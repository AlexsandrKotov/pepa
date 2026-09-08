package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Client is an HTTP client for the VMware vSphere REST API (vCenter 7.0+).
// Authentication is session-based: POST /api/session returns a session token
// that must be sent as the "vmware-api-session-id" header on every request.
type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client

	mu        sync.Mutex
	sessionID string
}

// NewClient creates a vSphere REST API client.
func NewClient(baseURL, username, password string, insecureTLS bool) *Client {
	transport := &http.Transport{}
	if insecureTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // #nosec // user-configured per-connection setting
	}
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   5 * time.Minute, // clone/reconfigure can take minutes
		},
	}
}

// ── HTTP helpers ─────────────────────────────────────────────

// ensureSession obtains or refreshes the session token.
func (c *Client) ensureSession() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessionID != "" {
		return nil
	}
	return c.login()
}

// login performs POST /api/session to obtain a new session token.
func (c *Client) login() error {
	reqURL := c.baseURL + "/api/session"
	req, err := http.NewRequest(http.MethodPost, reqURL, nil)
	if err != nil {
		return fmt.Errorf("vmware: build login request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vmware: login request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("vmware: authentication failed — check username and password")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vmware: login HTTP %d: %s", resp.StatusCode, string(body))
	}

	// The session ID is returned as a JSON string (quoted).
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("vmware: read login response: %w", err)
	}
	var sessionID string
	if err := json.Unmarshal(data, &sessionID); err != nil {
		return fmt.Errorf("vmware: parse session ID: %w", err)
	}
	if sessionID == "" {
		return fmt.Errorf("vmware: empty session ID returned")
	}
	c.sessionID = sessionID
	return nil
}

// invalidateSession clears the cached session so the next call re-authenticates.
func (c *Client) invalidateSession() {
	c.mu.Lock()
	c.sessionID = ""
	c.mu.Unlock()
}

// do executes an HTTP request with automatic session management.
// If the session expires (401), it re-authenticates and retries once.
func (c *Client) do(method, path string, body interface{}) (json.RawMessage, error) {
	if err := c.ensureSession(); err != nil {
		return nil, err
	}

	result, err := c.doOnce(method, path, body)
	if err != nil && strings.Contains(err.Error(), "HTTP 401") {
		// Session expired — re-login and retry.
		c.invalidateSession()
		if err2 := c.ensureSession(); err2 != nil {
			return nil, err2
		}
		return c.doOnce(method, path, body)
	}
	return result, err
}

// doOnce performs a single HTTP request with the current session token.
func (c *Client) doOnce(method, path string, body interface{}) (json.RawMessage, error) {
	reqURL := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("vmware: marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("vmware: build request: %w", err)
	}

	c.mu.Lock()
	sid := c.sessionID
	c.mu.Unlock()
	req.Header.Set("vmware-api-session-id", sid)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vmware: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vmware: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vmware: HTTP %d: %s", resp.StatusCode, string(data))
	}

	if len(data) == 0 {
		return json.RawMessage("null"), nil
	}
	return json.RawMessage(data), nil
}

// get performs a GET request.
func (c *Client) get(path string) (json.RawMessage, error) {
	return c.do(http.MethodGet, path, nil)
}

// RawGet performs a raw GET request (exposed for debugging).
func (c *Client) RawGet(path string) (json.RawMessage, error) {
	return c.get(path)
}

// RawPost performs a raw POST request with no body (exposed for debugging).
func (c *Client) RawPost(path string) (json.RawMessage, error) {
	return c.post(path, nil)
}

// RawPostWithBody performs a raw POST request with a body (exposed for debugging).
func (c *Client) RawPostWithBody(path string, body interface{}) (json.RawMessage, error) {
	return c.post(path, body)
}

// RawPut performs a raw PUT request (exposed for debugging).
func (c *Client) RawPut(path string, body interface{}) (json.RawMessage, error) {
	return c.do(http.MethodPut, path, body)
}

// RawPatchWithBody performs a raw PATCH request with a body (exposed for debugging).
func (c *Client) RawPatchWithBody(path string, body interface{}) (json.RawMessage, error) {
	return c.patch(path, body)
}

// post performs a POST request.
func (c *Client) post(path string, body interface{}) (json.RawMessage, error) {
	return c.do(http.MethodPost, path, body)
}

// delete performs a DELETE request.
func (c *Client) delete(path string) (json.RawMessage, error) {
	return c.do(http.MethodDelete, path, nil)
}

// patch performs a PATCH request.
func (c *Client) patch(path string, body interface{}) (json.RawMessage, error) {
	return c.do(http.MethodPatch, path, body)
}

// ── Data models ──────────────────────────────────────────────

// VMwareVM represents a virtual machine from the vSphere REST API.
type VMwareVM struct {
	VM           string `json:"vm"`
	Name         string `json:"name"`
	Power        string `json:"power_state"`
	CPU          int    `json:"cpu_count"`
	MemoryMiB    int64  `json:"memory_size_mib"`
	Host         string `json:"host,omitempty"`
	Cluster      string `json:"cluster,omitempty"`
	GuestOS      string `json:"guest_OS,omitempty"`
	IPAddress    string `json:"ip_address,omitempty"`
	GuestHost    string `json:"guest_host_name,omitempty"`
	DiskBytes    int64  `json:"disk_capacity_bytes,omitempty"`
	InstanceUUID string `json:"instance_uuid,omitempty"`
}

// VMwareVMDetail is the detailed VM response from GET /api/vcenter/vm/{id}.
type VMwareVMDetail struct {
	Name      string `json:"name"`
	Power     string `json:"power_state"`
	CPU       struct {
		Count   int   `json:"count"`
		CoresPer int  `json:"cores_per_socket"`
	} `json:"cpu"`
	Memory    struct {
		SizeMiB int64 `json:"size_MiB"`
	} `json:"memory"`
	Guest     struct {
		OS         string `json:"os"`
		Name       string `json:"name"`
		IPAddress  string `json:"ip_address"`
		HostName   string `json:"host_name"`
	} `json:"guest"`
	Host      string `json:"host"`
	Cluster   string `json:"cluster"`
}

// VMwareHost represents an ESXi host.
type VMwareHost struct {
	Host       string `json:"host"`
	Name       string `json:"name"`
	Connection string `json:"connection_state"`
	Hardware   struct {
		CPUCores   int   `json:"cpu_cores"`
		MemoryMiB  int64 `json:"memory_size_mib"`
	} `json:"hardware,omitempty"`
	MemoryUsageMiB  int64 `json:"memory_usage_mib,omitempty"`
	MemoryUtilization float64 `json:"memory_utilization,omitempty"`
}

// VMwareDatacenter represents a vSphere datacenter.
type VMwareDatacenter struct {
	Datacenter string `json:"datacenter"`
	Name       string `json:"name"`
}

// VMwareCluster represents a vSphere cluster.
type VMwareCluster struct {
	Cluster string `json:"cluster"`
	Name    string `json:"name"`
}

// VMwareDatastore represents a datastore.
type VMwareDatastore struct {
	Datastore  string  `json:"datastore"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	FreeSpace  int64   `json:"free_space"`
	Capacity   int64   `json:"capacity"`
}

// VMwareNetwork represents a network.
type VMwareNetwork struct {
	Network string `json:"network"`
	Name    string `json:"name"`
	Type    string `json:"type"`
}

// VMwareResourcePool represents a resource pool.
type VMwareResourcePool struct {
	ResourcePool string `json:"resource_pool"`
	Name         string `json:"name"`
}

// VMwareSnapshot represents a VM snapshot.
type VMwareSnapshot struct {
	Snapshot    string `json:"snapshot"`
	DisplayName string `json:"display_name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"create_time,omitempty"`
	State       string `json:"state,omitempty"`
}

// VMwareVersion represents the vCenter version info.
type VMwareVersion struct {
	Version    string `json:"version"`
	Build      string `json:"build"`
	Product    string `json:"product"`
	InstanceUUID string `json:"instance_uuid"`
}

// VMwareVMDisk represents a virtual disk attached to a VM.
type VMwareVMDisk struct {
	Key       string `json:"key"`
	Type      string `json:"type"`
	Capacity  int64  `json:"capacity"`
	Label     string `json:"label,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Datastore string `json:"datastore,omitempty"`
}

// VMwareVMNIC represents a network adapter attached to a VM.
type VMwareVMNIC struct {
	Key        string `json:"key"`
	Type       string `json:"type"`
	Network    string `json:"network"`
	MacAddress string `json:"mac_address"`
	Connected  bool   `json:"connected"`
}

// VMwareVMExpanded is a comprehensive VM detail response.
type VMwareVMExpanded struct {
	VMwareVMDetail
	Disks     []VMwareVMDisk   `json:"disks"`
	NICs      []VMwareVMNIC    `json:"nics"`
	Snapshots []VMwareSnapshot `json:"snapshots"`
	DiskError string           `json:"disk_error,omitempty"`
	NICError  string           `json:"nic_error,omitempty"`
}

// ── API operations ───────────────────────────────────────────

// TestConnection authenticates and retrieves vCenter version info.
func (c *Client) TestConnection() (*VMwareVersion, error) {
	// Force a fresh login to verify credentials.
	c.invalidateSession()
	if err := c.ensureSession(); err != nil {
		return nil, err
	}
	// Try to get product info from the about endpoint.
	data, err := c.get("/api/vcenter/")
	if err != nil {
		// Fallback: just return that we connected.
		return &VMwareVersion{Product: "vCenter", Version: "unknown"}, nil
	}
	var ver VMwareVersion
	if err := json.Unmarshal(data, &ver); err != nil {
		return &VMwareVersion{Product: "vCenter"}, nil
	}
	return &ver, nil
}

// ListDatacenters returns all datacenters.
func (c *Client) ListDatacenters() ([]VMwareDatacenter, error) {
	data, err := c.get("/api/vcenter/datacenter")
	if err != nil {
		return nil, err
	}
	var dcs []VMwareDatacenter
	if err := json.Unmarshal(data, &dcs); err != nil {
		return nil, fmt.Errorf("vmware: parse datacenters: %w", err)
	}
	return dcs, nil
}

// ListClusters returns all clusters.
func (c *Client) ListClusters() ([]VMwareCluster, error) {
	data, err := c.get("/api/vcenter/cluster")
	if err != nil {
		return nil, err
	}
	var clusters []VMwareCluster
	if err := json.Unmarshal(data, &clusters); err != nil {
		return nil, fmt.Errorf("vmware: parse clusters: %w", err)
	}
	return clusters, nil
}

// ListHosts returns all ESXi hosts.
func (c *Client) ListHosts() ([]VMwareHost, error) {
	data, err := c.get("/api/vcenter/host")
	if err != nil {
		return nil, err
	}
	var hosts []VMwareHost
	if err := json.Unmarshal(data, &hosts); err != nil {
		return nil, fmt.Errorf("vmware: parse hosts: %w", err)
	}
	return hosts, nil
}


// ListVMs returns all virtual machines, enriched with host mapping and guest identity.
func (c *Client) ListVMs() ([]VMwareVM, error) {
	// Step 1: Build VM-to-host mapping by querying VMs per host.
	// The vSphere REST API supports ?hosts=<hostID> to filter VMs by host.
	vmToHost := make(map[string]string) // vmID -> hostID
	hosts, err := c.ListHosts()
	if err == nil {
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, h := range hosts {
			wg.Add(1)
			go func(hostID string) {
				defer wg.Done()
				data, err := c.get(fmt.Sprintf("/api/vcenter/vm?hosts=%s", hostID))
				if err != nil {
					return
				}
				var vms []map[string]interface{}
				if json.Unmarshal(data, &vms) != nil {
					return
				}
				mu.Lock()
				for _, v := range vms {
					if vmID, ok := v["vm"].(string); ok {
						vmToHost[vmID] = hostID
					}
				}
				mu.Unlock()
			}(h.Host)
		}
		wg.Wait()
	}

	// Step 2: Fetch the full VM list.
	data, err := c.get("/api/vcenter/vm")
	if err != nil {
		return nil, err
	}
	var rawVMs []map[string]interface{}
	if err := json.Unmarshal(data, &rawVMs); err != nil {
		return nil, fmt.Errorf("vmware: parse VMs: %w", err)
	}
	allVMs := make([]VMwareVM, len(rawVMs))
	for i, raw := range rawVMs {
		if v, ok := raw["vm"].(string); ok {
			allVMs[i].VM = v
		}
		if v, ok := raw["name"].(string); ok {
			allVMs[i].Name = v
		}
		if v, ok := raw["power_state"].(string); ok {
			allVMs[i].Power = v
		}
		if v, ok := raw["cpu_count"].(float64); ok {
			allVMs[i].CPU = int(v)
		}
		if v, ok := raw["memory_size_MiB"].(float64); ok {
			allVMs[i].MemoryMiB = int64(v)
		}
		if v, ok := raw["cluster"].(string); ok {
			allVMs[i].Cluster = v
		}
		if v, ok := raw["instance_uuid"].(string); ok && v != "" {
			allVMs[i].InstanceUUID = v
		} else if v, ok := raw["uuid"].(string); ok && v != "" {
			allVMs[i].InstanceUUID = v
		}
		// Assign host from the mapping built in step 1.
		if hostID, ok := vmToHost[allVMs[i].VM]; ok {
			allVMs[i].Host = hostID
		}
	}

	// Step 3: Enrich powered-on VMs with guest identity in parallel.
	type vmEnrich struct {
		idx int
		vm  VMwareVM
	}
	ch := make(chan vmEnrich, len(allVMs))
	var wg sync.WaitGroup
	for i, vm := range allVMs {
		wg.Add(1)
		go func(idx int, v VMwareVM) {
			defer wg.Done()
			if v.Power == "POWERED_ON" {
				if identity, err := c.getVMGuestIdentity(v.VM); err == nil {
					if identity.Name != "" {
						v.GuestOS = identity.Name
					}
					if identity.IPAddress != "" {
						v.IPAddress = identity.IPAddress
					}
					if identity.HostName != "" {
						v.GuestHost = identity.HostName
					}
				}
			}
			// Fetch disk capacity for all VMs.
			v.DiskBytes = c.getVMDiskCapacity(v.VM)
			// Fetch instance UUID if not present (needed for "Open in vCenter" deep links).
			if v.InstanceUUID == "" {
				v.InstanceUUID = c.getVMInstanceUUID(v.VM)
			}
			ch <- vmEnrich{idx: idx, vm: v}
		}(i, vm)
	}
	wg.Wait()
	close(ch)

	enriched := make([]VMwareVM, len(allVMs))
	for e := range ch {
		enriched[e.idx] = e.vm
	}
	return enriched, nil
}

// VMwareGuestIdentity represents guest OS identity info.
type VMwareGuestIdentity struct {
	Name      string `json:"name"`
	HostName  string `json:"host_name"`
	IPAddress string `json:"ip_address"`
	Family    string `json:"family"`
}

// getVMDiskCapacity returns the total provisioned disk capacity (bytes) for a VM.
// The vSphere API requires two steps: list disk IDs, then fetch each disk's details.
func (c *Client) getVMDiskCapacity(vmID string) int64 {
	listData, err := c.get(fmt.Sprintf("/api/vcenter/vm/%s/hardware/disk", vmID))
	if err != nil {
		return 0
	}
	diskIDs := extractDiskIDs(listData)
	if len(diskIDs) == 0 {
		return 0
	}
	var total int64
	for _, diskID := range diskIDs {
		diskData, err := c.get(fmt.Sprintf("/api/vcenter/vm/%s/hardware/disk/%s", vmID, diskID))
		if err != nil {
			continue
		}
		var raw map[string]interface{}
		if json.Unmarshal(diskData, &raw) != nil {
			continue
		}
		if cap, ok := raw["capacity"].(float64); ok {
			total += int64(cap)
		}
	}
	return total
}

// getVMGuestIdentity returns guest identity info for a VM.
func (c *Client) getVMGuestIdentity(vmID string) (*VMwareGuestIdentity, error) {
	data, err := c.get(fmt.Sprintf("/api/vcenter/vm/%s/guest/identity", vmID))
	if err != nil {
		return nil, err
	}
	var identity VMwareGuestIdentity
	if err := json.Unmarshal(data, &identity); err != nil {
		return nil, fmt.Errorf("vmware: parse guest identity: %w", err)
	}
	return &identity, nil
}

// getVMInstanceUUID fetches the instance UUID (BIOS UUID) for a VM.
// This is required for building vCenter UI deep links.
// It tries multiple field names since different vCenter versions may use different keys.
func (c *Client) getVMInstanceUUID(vmID string) string {
	data, err := c.get(fmt.Sprintf("/api/vcenter/vm/%s", vmID))
	if err != nil {
		slog.Warn("vmware: fetch VM detail for instance_uuid failed", "vm", vmID, "error", err)
		return ""
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		slog.Warn("vmware: parse VM detail for instance_uuid failed", "vm", vmID, "error", err)
		return ""
	}
	// Try multiple field names used by different vCenter versions.
	for _, key := range []string{"instance_uuid", "uuid", "bios_uuid"} {
		if uuid, ok := raw[key].(string); ok && uuid != "" {
			return uuid
		}
	}
	slog.Warn("vmware: instance_uuid not found in VM detail response", "vm", vmID, "keys", mapKeys(raw))
	return ""
}

// mapKeys returns the keys of a map as a sorted slice (for debugging).
func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// GetVM returns detailed info about a VM.
func (c *Client) GetVM(vmID string) (*VMwareVMDetail, error) {
	data, err := c.get(fmt.Sprintf("/api/vcenter/vm/%s", vmID))
	if err != nil {
		return nil, err
	}
	var detail VMwareVMDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return nil, fmt.Errorf("vmware: parse VM detail: %w", err)
	}
	return &detail, nil
}

// GetVMExpanded returns comprehensive VM info including disks, NICs, and snapshots.
func (c *Client) GetVMExpanded(vmID string) (*VMwareVMExpanded, error) {
	// Base detail.
	detail, err := c.GetVM(vmID)
	if err != nil {
		return nil, fmt.Errorf("vmware: get VM detail: %w", err)
	}

	expanded := &VMwareVMExpanded{
		VMwareVMDetail: *detail,
		Disks:          make([]VMwareVMDisk, 0),
		NICs:           make([]VMwareVMNIC, 0),
		Snapshots:      make([]VMwareSnapshot, 0),
	}

	// Fetch guest identity (for powered-on VMs with VMware Tools).
	if detail.Power == "POWERED_ON" {
		if identity, err := c.getVMGuestIdentity(vmID); err == nil {
			expanded.Guest.Name = identity.Name
			expanded.Guest.HostName = identity.HostName
			expanded.Guest.IPAddress = identity.IPAddress
			expanded.Guest.OS = identity.Family
		}
	}

	// Resolve host ID to name (the detail response returns host as an ID like "host-123").
	if expanded.Host != "" && strings.HasPrefix(expanded.Host, "host-") {
		if hosts, err := c.ListHosts(); err == nil {
			for _, h := range hosts {
				if h.Host == expanded.Host {
					expanded.Host = h.Name
					break
				}
			}
		}
	}

	// Resolve cluster ID to name.
	if expanded.Cluster != "" && strings.HasPrefix(expanded.Cluster, "group-") {
		if clusters, err := c.ListClusters(); err == nil {
			for _, cl := range clusters {
				if cl.Cluster == expanded.Cluster {
					expanded.Cluster = cl.Name
					break
				}
			}
		}
	}

	// Also fetch the raw VM detail to get guest_OS (top-level field).
	rawDetail, err := c.get(fmt.Sprintf("/api/vcenter/vm/%s", vmID))
	if err == nil {
		var rawMap map[string]interface{}
		if json.Unmarshal(rawDetail, &rawMap) == nil {
			if gos, ok := rawMap["guest_OS"].(string); ok && gos != "" {
				expanded.Guest.OS = gos
			}
		}
	}

	// Disks: first fetch list of disk IDs, then fetch each disk's details.
	diskListData, err := c.get(fmt.Sprintf("/api/vcenter/vm/%s/hardware/disk", vmID))
	if err != nil {
		slog.Warn("vmware: fetch disk list failed", "vm", vmID, "error", err)
		expanded.DiskError = err.Error()
	} else {
		diskIDs := extractDiskIDs(diskListData)
		if len(diskIDs) > 0 {
			for _, diskID := range diskIDs {
				diskDetail, err := c.get(fmt.Sprintf("/api/vcenter/vm/%s/hardware/disk/%s", vmID, diskID))
				if err != nil {
					slog.Warn("vmware: fetch disk detail failed", "vm", vmID, "disk", diskID, "error", err)
					continue
				}
				var raw map[string]interface{}
				if json.Unmarshal(diskDetail, &raw) != nil {
					continue
				}
				disk := VMwareVMDisk{Key: diskID}
				if v, ok := raw["type"].(string); ok {
					disk.Type = v
				}
				if v, ok := raw["capacity"].(float64); ok {
					disk.Capacity = int64(v)
				}
				if v, ok := raw["label"].(string); ok {
					disk.Label = v
				}
				if backing, ok := raw["backing"].(map[string]interface{}); ok {
					if v, ok := backing["vmdk_file"].(string); ok {
						disk.Summary = v
					}
					if v, ok := backing["datastore"].(string); ok {
						disk.Datastore = v
					}
				}
				expanded.Disks = append(expanded.Disks, disk)
			}
		} else {
			// Check if the response was an error or empty.
			slog.Warn("vmware: could not parse disk list", "vm", vmID, "response", string(diskListData))
			expanded.DiskError = "unexpected disk list format from vCenter"
		}
	}

	// Network adapters: try multiple endpoint patterns.
	var nicListData json.RawMessage
	var nicBasePath string
	for _, path := range []string{
		fmt.Sprintf("/api/vcenter/vm/%s/hardware/ethernet", vmID),
		fmt.Sprintf("/api/vcenter/vm/%s/hardware/adapter/ethernet", vmID),
	} {
		nicListData, err = c.get(path)
		if err == nil {
			// Extract base path for detail requests.
			if strings.Contains(path, "/hardware/ethernet") {
				nicBasePath = fmt.Sprintf("/api/vcenter/vm/%s/hardware/ethernet", vmID)
			} else {
				nicBasePath = fmt.Sprintf("/api/vcenter/vm/%s/hardware/adapter/ethernet", vmID)
			}
			break
		}
	}
	if err != nil {
		slog.Warn("vmware: fetch NIC list failed", "vm", vmID, "error", err)
		expanded.NICError = err.Error()
	} else {
		// Parse NIC ID list: [{"nic": "4000"}, ...] or map format
		var nicRefs []struct {
			NIC string `json:"nic"`
		}
		if json.Unmarshal(nicListData, &nicRefs) == nil && len(nicRefs) > 0 {
			for _, ref := range nicRefs {
				nicDetail, err := c.get(fmt.Sprintf("%s/%s", nicBasePath, ref.NIC))
				if err != nil {
					slog.Warn("vmware: fetch NIC detail failed", "vm", vmID, "nic", ref.NIC, "error", err)
					continue
				}
				var raw map[string]interface{}
				if json.Unmarshal(nicDetail, &raw) != nil {
					continue
				}
				nic := VMwareVMNIC{Key: ref.NIC}
				if v, ok := raw["type"].(string); ok {
					nic.Type = v
				}
				if v, ok := raw["mac_address"].(string); ok {
					nic.MacAddress = v
				}
				if backing, ok := raw["backing"].(map[string]interface{}); ok {
					// Prefer network_name (human-readable) over network (ID).
					if v, ok := backing["network_name"].(string); ok && v != "" {
						nic.Network = v
					} else if v, ok := backing["network"].(string); ok {
						nic.Network = v
					}
				}
				if state, ok := raw["state"].(string); ok {
					nic.Connected = state == "CONNECTED"
				}
				if startConn, ok := raw["start_connected"].(bool); ok && !nic.Connected {
					nic.Connected = startConn
				}
				expanded.NICs = append(expanded.NICs, nic)
			}
		} else {
			// Try map format: {"4000": {...}, ...}
			var rawNICs map[string]map[string]interface{}
			if json.Unmarshal(nicListData, &rawNICs) == nil {
				for key, n := range rawNICs {
					nic := VMwareVMNIC{Key: key}
					if v, ok := n["type"].(string); ok {
						nic.Type = v
					}
					if v, ok := n["mac_address"].(string); ok {
						nic.MacAddress = v
					}
					if backing, ok := n["backing"].(map[string]interface{}); ok {
						if v, ok := backing["network"].(string); ok {
							nic.Network = v
						}
					}
					if state, ok := n["state"].(string); ok {
						nic.Connected = state == "CONNECTED"
					}
					expanded.NICs = append(expanded.NICs, nic)
				}
			} else {
				slog.Warn("vmware: could not parse NIC list", "vm", vmID, "response", string(nicListData))
			}
		}
	}

	// Snapshots.
	snapData, err := c.get(fmt.Sprintf("/api/vcenter/vm/%s/snapshots", vmID))
	if err == nil {
		_ = json.Unmarshal(snapData, &expanded.Snapshots)
	}

	return expanded, nil
}

// extractDiskIDs parses disk IDs from the vSphere disk list response.
// It handles both array format ([{"disk":"2000"}]) and map format ({"2000":{...}}).
func extractDiskIDs(data json.RawMessage) []string {
	// Try array format first: [{"disk": "2000"}, ...]
	var diskRefs []struct {
		Disk string `json:"disk"`
	}
	if json.Unmarshal(data, &diskRefs) == nil && len(diskRefs) > 0 {
		ids := make([]string, len(diskRefs))
		for i, ref := range diskRefs {
			ids[i] = ref.Disk
		}
		return ids
	}
	// Try map format: {"2000": {...}, ...}
	var rawDisks map[string]json.RawMessage
	if json.Unmarshal(data, &rawDisks) == nil && len(rawDisks) > 0 {
		ids := make([]string, 0, len(rawDisks))
		for k := range rawDisks {
			ids = append(ids, k)
		}
		return ids
	}
	return nil
}

// VMAction performs a power action on a VM.
// The vSphere REST API uses POST /api/vcenter/vm/{vm}/power?action={action}
// where action is one of: start, stop (graceful), reset (hard), suspend.
// We map friendly names: shutdown→stop, reboot→reset.
func (c *Client) VMAction(vmID, action string) error {
	// Map friendly action names to vSphere API action names.
	switch action {
	case "shutdown":
		action = "stop" // graceful guest shutdown
	case "reboot":
		action = "reset" // hard reset
	}
	_, err := c.post(fmt.Sprintf("/api/vcenter/vm/%s/power?action=%s", vmID, action), nil)
	return err
}

// CreateVM creates a new VM with the given spec.
func (c *Client) CreateVM(spec map[string]interface{}) (string, error) {
	data, err := c.post("/api/vcenter/vm", spec)
	if err != nil {
		return "", err
	}
	var vmID string
	if err := json.Unmarshal(data, &vmID); err != nil {
		return "", fmt.Errorf("vmware: parse created VM ID: %w", err)
	}
	return vmID, nil
}

// DeleteVM deletes a virtual machine.
func (c *Client) DeleteVM(vmID string) error {
	_, err := c.delete(fmt.Sprintf("/api/vcenter/vm/%s", vmID))
	return err
}

// ListDatastores returns all datastores.
func (c *Client) ListDatastores() ([]VMwareDatastore, error) {
	data, err := c.get("/api/vcenter/datastore")
	if err != nil {
		return nil, err
	}
	var stores []VMwareDatastore
	if err := json.Unmarshal(data, &stores); err != nil {
		return nil, fmt.Errorf("vmware: parse datastores: %w", err)
	}
	return stores, nil
}

// ListNetworks returns all networks.
func (c *Client) ListNetworks() ([]VMwareNetwork, error) {
	data, err := c.get("/api/vcenter/network")
	if err != nil {
		return nil, err
	}
	var nets []VMwareNetwork
	if err := json.Unmarshal(data, &nets); err != nil {
		return nil, fmt.Errorf("vmware: parse networks: %w", err)
	}
	return nets, nil
}

// ListResourcePools returns all resource pools.
func (c *Client) ListResourcePools() ([]VMwareResourcePool, error) {
	data, err := c.get("/api/vcenter/resource-pool")
	if err != nil {
		return nil, err
	}
	var pools []VMwareResourcePool
	if err := json.Unmarshal(data, &pools); err != nil {
		return nil, fmt.Errorf("vmware: parse resource pools: %w", err)
	}
	return pools, nil
}

// ListSnapshots returns the snapshot tree of a VM.
func (c *Client) ListSnapshots(vmID string) ([]VMwareSnapshot, error) {
	data, err := c.get(fmt.Sprintf("/api/vcenter/vm/%s/snapshots", vmID))
	if err != nil {
		return nil, err
	}
	var snaps []VMwareSnapshot
	if err := json.Unmarshal(data, &snaps); err != nil {
		return nil, fmt.Errorf("vmware: parse snapshots: %w", err)
	}
	return snaps, nil
}

// CreateSnapshot creates a snapshot for a VM.
func (c *Client) CreateSnapshot(vmID, name, description string) (string, error) {
	spec := map[string]interface{}{
		"name": name,
	}
	if description != "" {
		spec["description"] = description
	}
	data, err := c.post(fmt.Sprintf("/api/vcenter/vm/%s/snapshots", vmID), spec)
	if err != nil {
		return "", err
	}
	var snapID string
	if err := json.Unmarshal(data, &snapID); err != nil {
		return "", fmt.Errorf("vmware: parse snapshot ID: %w", err)
	}
	return snapID, nil
}

// DeleteSnapshot removes a snapshot from a VM.
func (c *Client) DeleteSnapshot(vmID, snapshotID string) error {
	_, err := c.delete(fmt.Sprintf("/api/vcenter/vm/%s/snapshots/%s", vmID, snapshotID))
	return err
}

// RevertSnapshot reverts a VM to the given snapshot.
func (c *Client) RevertSnapshot(vmID, snapshotID string) error {
	_, err := c.post(fmt.Sprintf("/api/vcenter/vm/%s/snapshots/%s/revert", vmID, snapshotID), nil)
	return err
}

// CloneVM clones an existing VM into a new VM.
func (c *Client) CloneVM(sourceVMID string, spec map[string]interface{}) (string, error) {
	spec["source"] = sourceVMID
	data, err := c.post("/api/vcenter/vm?action=clone", spec)
	if err != nil {
		return "", err
	}
	var vmID string
	if err := json.Unmarshal(data, &vmID); err != nil {
		return "", fmt.Errorf("vmware: parse cloned VM ID: %w", err)
	}
	return vmID, nil
}

// ReconfigureVM updates VM configuration (CPU, memory).
// The vSphere REST API requires separate PATCH calls to hardware sub-endpoints:
//   PATCH /api/vcenter/vm/{vm}/hardware/cpu   {"count":N,"cores_per_socket":N}
//   PATCH /api/vcenter/vm/{vm}/hardware/memory {"size_MiB":N}
func (c *Client) ReconfigureVM(vmID string, spec map[string]interface{}) error {
	if cpu, ok := spec["cpu"]; ok {
		if _, err := c.patch(fmt.Sprintf("/api/vcenter/vm/%s/hardware/cpu", vmID), cpu); err != nil {
			return fmt.Errorf("vmware: update CPU: %w", err)
		}
	}
	if mem, ok := spec["memory"]; ok {
		if _, err := c.patch(fmt.Sprintf("/api/vcenter/vm/%s/hardware/memory", vmID), mem); err != nil {
			return fmt.Errorf("vmware: update memory: %w", err)
		}
	}
	if _, hasCPU := spec["cpu"]; !hasCPU {
		if _, hasMem := spec["memory"]; !hasMem {
			return fmt.Errorf("vmware: reconfigure requires at least 'cpu' or 'memory' in spec")
		}
	}
	return nil
}

// MigrateVM relocates a VM to another host.
func (c *Client) MigrateVM(vmID, targetHost string) error {
	spec := map[string]interface{}{
		"target": map[string]interface{}{
			"host": targetHost,
		},
	}
	_, err := c.post(fmt.Sprintf("/api/vcenter/vm/%s?action=relocate", vmID), spec)
	return err
}
