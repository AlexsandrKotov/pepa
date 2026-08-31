package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a minimal HTTP client for the Proxmox VE REST API.
// It uses API token authentication: Authorization: PVEAPIToken={token_id}={token_secret}
type Client struct {
	baseURL     string
	tokenID     string
	tokenSecret string
	httpClient  *http.Client
}

// NewClient creates a Proxmox API client from connection config.
func NewClient(baseURL, tokenID, tokenSecret string, insecureTLS bool) *Client {
	transport := &http.Transport{}
	if insecureTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		tokenID:     tokenID,
		tokenSecret: tokenSecret,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// proxmoxResponse wraps the standard Proxmox API response envelope.
type proxmoxResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors json.RawMessage `json:"errors,omitempty"`
}

// get performs a GET request and returns the data field.
func (c *Client) get(path string) (json.RawMessage, error) {
	return c.do(http.MethodGet, path, nil)
}

// post performs a POST request and returns the data field.
func (c *Client) post(path string, body url.Values) (json.RawMessage, error) {
	return c.do(http.MethodPost, path, body)
}

// delete performs a DELETE request and returns the data field.
func (c *Client) delete(path string) (json.RawMessage, error) {
	return c.do(http.MethodDelete, path, nil)
}

// do executes an HTTP request to the Proxmox API.
func (c *Client) do(method, path string, body url.Values) (json.RawMessage, error) {
	reqURL := c.baseURL + "/api2/json" + path

	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(body.Encode())
	}

	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("proxmox: build request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenID, c.tokenSecret))
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("proxmox: request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("proxmox: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("proxmox: HTTP %d: %s", resp.StatusCode, string(data))
	}

	var pr proxmoxResponse
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, fmt.Errorf("proxmox: parse response: %w", err)
	}

	if pr.Errors != nil && len(pr.Errors) > 2 {
		return nil, fmt.Errorf("proxmox: API errors: %s", string(pr.Errors))
	}

	return pr.Data, nil
}

// ── Node operations ──────────────────────────────────────────

type ProxmoxNode struct {
	Node    string  `json:"node"`
	Status  string  `json:"status"`
	CPU     float64 `json:"cpu"`
	MaxCPU  int     `json:"maxcpu"`
	Mem     uint64  `json:"mem"`
	MaxMem  uint64  `json:"maxmem"`
	Uptime  int64   `json:"uptime"`
	Version string  `json:"version"`
}

func (c *Client) ListNodes() ([]ProxmoxNode, error) {
	data, err := c.get("/nodes")
	if err != nil {
		return nil, err
	}
	var nodes []ProxmoxNode
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, fmt.Errorf("proxmox: parse nodes: %w", err)
	}
	return nodes, nil
}

func (c *Client) GetNodeStatus(node string) (json.RawMessage, error) {
	return c.get(fmt.Sprintf("/nodes/%s/status", node))
}

// ── VM operations ────────────────────────────────────────────

type ProxmoxVM struct {
	VMID     int     `json:"vmid"`
	Name     string  `json:"name"`
	Node     string  `json:"node"`
	Status   string  `json:"status"`
	CPU      float64 `json:"cpu"`
	MaxCPU   int     `json:"maxcpu"`
	Mem      uint64  `json:"mem"`
	MaxMem   uint64  `json:"maxmem"`
	Disk     uint64  `json:"disk"`
	MaxDisk  uint64  `json:"maxdisk"`
	Uptime   int64   `json:"uptime"`
	Template int     `json:"template"`
	Tags     string  `json:"tags"`
}

func (c *Client) ListVMs(node string) ([]ProxmoxVM, error) {
	data, err := c.get(fmt.Sprintf("/nodes/%s/qemu", node))
	if err != nil {
		return nil, err
	}
	var vms []ProxmoxVM
	if err := json.Unmarshal(data, &vms); err != nil {
		return nil, fmt.Errorf("proxmox: parse VMs: %w", err)
	}
	return vms, nil
}

func (c *Client) GetVMConfig(node string, vmid int) (json.RawMessage, error) {
	return c.get(fmt.Sprintf("/nodes/%s/qemu/%d/config", node, vmid))
}

func (c *Client) VMAction(node string, vmid int, action string) (json.RawMessage, error) {
	return c.post(fmt.Sprintf("/nodes/%s/qemu/%d/status/%s", node, vmid, action), nil)
}

func (c *Client) CreateVM(node string, params url.Values) (json.RawMessage, error) {
	return c.post(fmt.Sprintf("/nodes/%s/qemu", node), params)
}

func (c *Client) DeleteVM(node string, vmid int) (json.RawMessage, error) {
	return c.delete(fmt.Sprintf("/nodes/%s/qemu/%d", node, vmid))
}

// ── Container operations ─────────────────────────────────────

func (c *Client) ListContainers(node string) ([]ProxmoxVM, error) {
	data, err := c.get(fmt.Sprintf("/nodes/%s/lxc", node))
	if err != nil {
		return nil, err
	}
	var containers []ProxmoxVM
	if err := json.Unmarshal(data, &containers); err != nil {
		return nil, fmt.Errorf("proxmox: parse containers: %w", err)
	}
	return containers, nil
}

func (c *Client) ContainerAction(node string, vmid int, action string) (json.RawMessage, error) {
	return c.post(fmt.Sprintf("/nodes/%s/lxc/%d/status/%s", node, vmid, action), nil)
}

func (c *Client) CreateContainer(node string, params url.Values) (json.RawMessage, error) {
	return c.post(fmt.Sprintf("/nodes/%s/lxc", node), params)
}

func (c *Client) DeleteContainer(node string, vmid int) (json.RawMessage, error) {
	return c.delete(fmt.Sprintf("/nodes/%s/lxc/%d", node, vmid))
}

// GetContainerStatus returns live status (including IP) of an LXC container.
func (c *Client) GetContainerStatus(node string, vmid int) (json.RawMessage, error) {
	return c.get(fmt.Sprintf("/nodes/%s/lxc/%d/status/current", node, vmid))
}

// ── Cluster operations ───────────────────────────────────────

type ProxmoxClusterResource struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	Node     string  `json:"node"`
	Status   string  `json:"status"`
	CPU      float64 `json:"cpu"`
	MaxCPU   int     `json:"maxcpu"`
	Mem      uint64  `json:"mem"`
	MaxMem   uint64  `json:"maxmem"`
	Disk     uint64  `json:"disk"`
	MaxDisk  uint64  `json:"maxdisk"`
	Uptime   int64   `json:"uptime"`
	Pool     string  `json:"pool"`
	Template int     `json:"template"`
}

func (c *Client) ClusterResources() ([]ProxmoxClusterResource, error) {
	data, err := c.get("/cluster/resources")
	if err != nil {
		return nil, err
	}
	var resources []ProxmoxClusterResource
	if err := json.Unmarshal(data, &resources); err != nil {
		return nil, fmt.Errorf("proxmox: parse cluster resources: %w", err)
	}
	return resources, nil
}

func (c *Client) ListPools() (json.RawMessage, error) {
	return c.get("/pools")
}

// GetPermissions returns the effective API permissions of the current token.
func (c *Client) GetPermissions() (json.RawMessage, error) {
	return c.get("/access/permissions")
}

func (c *Client) ListStorage() (json.RawMessage, error) {
	return c.get("/storage")
}

// NextID returns the next free VMID from the cluster.
func (c *Client) NextID() (json.RawMessage, error) {
	return c.get("/cluster/nextid")
}

// ListOSTemplates returns the downloadable OS template catalog of a node.
func (c *Client) ListOSTemplates(node string) (json.RawMessage, error) {
	return c.get(fmt.Sprintf("/nodes/%s/aplinfo", node))
}

// ListStorageContent lists the content of a storage (vztmpl, iso, images, ...).
func (c *Client) ListStorageContent(storage, content string) (json.RawMessage, error) {
	if content == "" {
		content = "vztmpl,iso"
	}
	return c.get(fmt.Sprintf("/storage/%s/content?content=%s", url.PathEscape(storage), url.QueryEscape(content)))
}

// NodeSyslog returns the syslog lines of a node (newest first).
func (c *Client) NodeSyslog(node string, start, limit int) (json.RawMessage, error) {
	path := fmt.Sprintf("/nodes/%s/syslog?", node)
	if limit > 0 {
		path += fmt.Sprintf("limit=%d&", limit)
	}
	if start > 0 {
		path += fmt.Sprintf("start=%d&", start)
	}
	return c.get(strings.TrimRight(path, "&?"))
}

// NodeTasks returns recent tasks (operations) of a node.
func (c *Client) NodeTasks(node string, limit int) (json.RawMessage, error) {
	path := fmt.Sprintf("/nodes/%s/tasks?errors=1", node)
	if limit > 0 {
		path += fmt.Sprintf("&limit=%d", limit)
	}
	return c.get(path)
}

// TaskLog returns the log output of a single task by UPID.
func (c *Client) TaskLog(node, upid string) (json.RawMessage, error) {
	return c.get(fmt.Sprintf("/nodes/%s/tasks/%s/log", node, url.PathEscape(upid)))
}

func (c *Client) TestConnection() (json.RawMessage, error) {
	return c.get("/version")
}
