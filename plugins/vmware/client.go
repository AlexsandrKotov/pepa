package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	VM        string `json:"vm"`
	Name      string `json:"name"`
	Power     string `json:"power_state"`
	CPU       int    `json:"cpu_count"`
	MemoryMiB int64  `json:"memory_size_mib"`
	Host      string `json:"host,omitempty"`
	Cluster   string `json:"cluster,omitempty"`
	GuestOS   string `json:"guest_OS,omitempty"`
	IPAddress string `json:"ip_address,omitempty"`
	GuestHost string `json:"guest_host_name,omitempty"`
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
