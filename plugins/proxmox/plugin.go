package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// ProxmoxPlugin implements provider.Provider for Proxmox VE integration.
type ProxmoxPlugin struct{}

var _ provider.Provider = (*ProxmoxPlugin)(nil)

func (p *ProxmoxPlugin) Name() string    { return "proxmox" }
func (p *ProxmoxPlugin) Version() string { return "0.1.0" }
func (p *ProxmoxPlugin) Description() string {
	return "Proxmox VE virtualization — VMs, LXC containers, nodes"
}
func (p *ProxmoxPlugin) PluginType() string { return "virtualization" }

func (p *ProxmoxPlugin) Actions() []string {
	return []string{
		"test_connection",
		"list_nodes",
		"get_node",
		"list_vms",
		"list_containers",
		"get_vm",
		"start_vm",
		"stop_vm",
		"shutdown_vm",
		"reboot_vm",
		"create_vm",
		"delete_vm",
		"start_container",
		"stop_container",
		"create_container",
		"delete_container",
		"cluster_resources",
		"list_pools",
		"list_storage",
		"get_permissions",
		"next_id",
		"list_os_templates",
		"list_storage_content",
		"node_syslog",
		"node_tasks",
		"task_log",
		"deploy_docker",
	}
}

func (p *ProxmoxPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	client, err := p.buildClient(config)
	if err != nil {
		return nil, err
	}

	switch action {
	case "test_connection":
		return p.testConnection(client)
	case "list_nodes":
		return p.listNodes(client)
	case "get_node":
		return p.getNode(client, params)
	case "list_vms":
		return p.listVMs(client)
	case "list_containers":
		return p.listContainers(client)
	case "get_vm":
		return p.getVM(client, params)
	case "start_vm":
		return p.vmAction(client, params, "start")
	case "stop_vm":
		return p.vmAction(client, params, "stop")
	case "shutdown_vm":
		return p.vmAction(client, params, "shutdown")
	case "reboot_vm":
		return p.vmAction(client, params, "reboot")
	case "create_vm":
		return p.createVM(client, params)
	case "delete_vm":
		return p.deleteVM(client, params)
	case "start_container":
		return p.containerAction(client, params, "start")
	case "stop_container":
		return p.containerAction(client, params, "stop")
	case "create_container":
		return p.createContainer(client, params)
	case "delete_container":
		return p.deleteContainer(client, params)
	case "cluster_resources":
		return p.clusterResources(client)
	case "list_pools":
		return p.listPools(client)
	case "list_storage":
		return p.listStorage(client)
	case "get_permissions":
		return p.getPermissions(client)
	case "next_id":
		return p.nextID(client)
	case "list_os_templates":
		return p.listOSTemplates(client, params)
	case "list_storage_content":
		return p.listStorageContent(client, params)
	case "node_syslog":
		return p.nodeSyslog(client, params)
	case "node_tasks":
		return p.nodeTasks(client, params)
	case "task_log":
		return p.taskLog(client, params)
	case "deploy_docker":
		return p.deployDocker(client, config, params)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *ProxmoxPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "Proxmox plugin ready — requires connection config (url, token_id, token_secret)",
	}, nil
}

// buildClient creates a Proxmox API client from the connection config.
func (p *ProxmoxPlugin) buildClient(config map[string]string) (*Client, error) {
	baseURL := config["url"]
	if baseURL == "" {
		return nil, fmt.Errorf("proxmox plugin requires 'url' in connection config")
	}
	tokenID := config["token_id"]
	if tokenID == "" {
		return nil, fmt.Errorf("proxmox plugin requires 'token_id' in connection config")
	}
	tokenSecret := config["token_secret"]
	if tokenSecret == "" {
		return nil, fmt.Errorf("proxmox plugin requires 'token_secret' in connection config")
	}
	insecureTLS := config["insecure_tls"] == "true" || config["insecure_tls"] == "1"
	return NewClient(baseURL, tokenID, tokenSecret, insecureTLS), nil
}

// ── Action implementations ────────────────────────────────────

func (p *ProxmoxPlugin) testConnection(client *Client) ([]byte, error) {
	data, err := client.TestConnection()
	if err != nil {
		return nil, err
	}
	// Detect low-privilege tokens: /access/permissions returns the effective
	// permission set; without Sys.Audit the token cannot list VMs/nodes.
	warning := ""
	if perms, perr := client.GetPermissions(); perr == nil && !permsIncludeAudit(perms) {
		warning = "Token has no usable permissions: assign a role (e.g. PVEAdministrator or PVEAuditor) to the API token in Proxmox under Datacenter → Permissions"
	}
	return actionOutput(map[string]interface{}{"status": "connected", "version": jsonRaw(data), "warning": warning})
}

// permsIncludeAudit checks whether the /access/permissions payload grants any
// meaningful access (Sys.Audit or VM.* / VM.Allocate on /).
func permsIncludeAudit(raw json.RawMessage) bool {
	var perms map[string]map[string]int
	if err := json.Unmarshal(raw, &perms); err != nil {
		return false
	}
	for path, acl := range perms {
		_ = path
		for perm, granted := range acl {
			if granted == 1 && (perm == "Sys.Audit" || strings.HasPrefix(perm, "VM.")) {
				return true
			}
		}
	}
	return false
}

func (p *ProxmoxPlugin) getPermissions(client *Client) ([]byte, error) {
	data, err := client.GetPermissions()
	if err != nil {
		return nil, err
	}
	return actionOutput(jsonRaw(data))
}

func (p *ProxmoxPlugin) listNodes(client *Client) ([]byte, error) {
	nodes, err := client.ListNodes()
	if err != nil {
		return nil, err
	}
	return actionOutput(nodes)
}

func (p *ProxmoxPlugin) getNode(client *Client, params []byte) ([]byte, error) {
	var input struct {
		Node string `json:"node"`
	}
	if err := actionInput(params, &input); err != nil || input.Node == "" {
		return nil, fmt.Errorf("get_node requires 'node' parameter")
	}
	data, err := client.GetNodeStatus(input.Node)
	if err != nil {
		return nil, err
	}
	return actionOutput(jsonRaw(data))
}

func (p *ProxmoxPlugin) listVMs(client *Client) ([]byte, error) {
	nodes, err := client.ListNodes()
	if err != nil {
		return nil, err
	}
	var allVMs []ProxmoxVM
	for _, node := range nodes {
		if node.Status != "online" {
			continue
		}
		vms, err := client.ListVMs(node.Node)
		if err != nil {
			continue
		}
		for i := range vms {
			vms[i].Node = node.Node
		}
		allVMs = append(allVMs, vms...)
	}
	return actionOutput(allVMs)
}

func (p *ProxmoxPlugin) listContainers(client *Client) ([]byte, error) {
	nodes, err := client.ListNodes()
	if err != nil {
		return nil, err
	}
	var allContainers []ProxmoxVM
	for _, node := range nodes {
		if node.Status != "online" {
			continue
		}
		containers, err := client.ListContainers(node.Node)
		if err != nil {
			continue
		}
		for i := range containers {
			containers[i].Node = node.Node
		}
		allContainers = append(allContainers, containers...)
	}
	return actionOutput(allContainers)
}

func (p *ProxmoxPlugin) getVM(client *Client, params []byte) ([]byte, error) {
	var input struct {
		Node string `json:"node"`
		VMID int    `json:"vmid"`
	}
	if err := actionInput(params, &input); err != nil || input.Node == "" || input.VMID == 0 {
		return nil, fmt.Errorf("get_vm requires 'node' and 'vmid' parameters")
	}
	data, err := client.GetVMConfig(input.Node, input.VMID)
	if err != nil {
		return nil, err
	}
	return actionOutput(jsonRaw(data))
}

func (p *ProxmoxPlugin) vmAction(client *Client, params []byte, action string) ([]byte, error) {
	var input struct {
		Node string `json:"node"`
		VMID int    `json:"vmid"`
	}
	if err := actionInput(params, &input); err != nil || input.Node == "" || input.VMID == 0 {
		return nil, fmt.Errorf("%s requires 'node' and 'vmid' parameters", action)
	}
	data, err := client.VMAction(input.Node, input.VMID, action)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "ok", "upid": jsonRaw(data)})
}

func (p *ProxmoxPlugin) createVM(client *Client, params []byte) ([]byte, error) {
	var req provider.CreateVMRequest
	if err := actionInput(params, &req); err != nil {
		return nil, fmt.Errorf("create_vm: parse params: %w", err)
	}
	if req.Node == "" || req.Name == "" {
		return nil, fmt.Errorf("create_vm requires 'node' and 'name'")
	}

	form := url.Values{}
	form.Set("name", req.Name)
	if req.VMID > 0 {
		form.Set("vmid", strconv.Itoa(req.VMID))
	}
	if req.Cores > 0 {
		form.Set("cores", strconv.Itoa(req.Cores))
	}
	if req.Memory > 0 {
		form.Set("memory", strconv.Itoa(req.Memory))
	}
	if req.DiskSize != "" {
		storage := req.Storage
		if storage == "" {
			storage = "local-lvm"
		}
		form.Set("scsi0", fmt.Sprintf("%s:%s", storage, req.DiskSize))
	}
	if req.ISO != "" {
		form.Set("ide2", fmt.Sprintf("%s,media=cdrom", req.ISO))
	}
	if req.Network != "" {
		form.Set("net0", fmt.Sprintf("virtio,bridge=%s", req.Network))
	}
	if req.Template > 0 {
		form.Set("clone", strconv.Itoa(req.Template))
	}
	if req.Start {
		form.Set("start", "1")
	}

	data, err := client.CreateVM(req.Node, form)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "created", "upid": jsonRaw(data)})
}

func (p *ProxmoxPlugin) deleteVM(client *Client, params []byte) ([]byte, error) {
	var input struct {
		Node string `json:"node"`
		VMID int    `json:"vmid"`
	}
	if err := actionInput(params, &input); err != nil || input.Node == "" || input.VMID == 0 {
		return nil, fmt.Errorf("delete_vm requires 'node' and 'vmid' parameters")
	}
	data, err := client.DeleteVM(input.Node, input.VMID)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "deleted", "upid": jsonRaw(data)})
}

func (p *ProxmoxPlugin) containerAction(client *Client, params []byte, action string) ([]byte, error) {
	var input struct {
		Node string `json:"node"`
		VMID int    `json:"vmid"`
	}
	if err := actionInput(params, &input); err != nil || input.Node == "" || input.VMID == 0 {
		return nil, fmt.Errorf("%s requires 'node' and 'vmid' parameters", action)
	}
	data, err := client.ContainerAction(input.Node, input.VMID, action)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "ok", "upid": jsonRaw(data)})
}

func (p *ProxmoxPlugin) createContainer(client *Client, params []byte) ([]byte, error) {
	var req provider.CreateContainerRequest
	if err := actionInput(params, &req); err != nil {
		return nil, fmt.Errorf("create_container: parse params: %w", err)
	}
	if req.Node == "" || req.Hostname == "" || req.Template == "" {
		return nil, fmt.Errorf("create_container requires 'node', 'hostname', and 'template'")
	}

	form := url.Values{}
	form.Set("hostname", req.Hostname)
	form.Set("ostemplate", req.Template)
	if req.VMID > 0 {
		form.Set("vmid", strconv.Itoa(req.VMID))
	}
	if req.Cores > 0 {
		form.Set("cores", strconv.Itoa(req.Cores))
	}
	if req.Memory > 0 {
		form.Set("memory", strconv.Itoa(req.Memory))
	}
	if req.DiskSize != "" {
		storage := req.Storage
		if storage == "" {
			storage = "local-lvm"
		}
		form.Set("rootfs", fmt.Sprintf("%s:%s", storage, req.DiskSize))
	}
	if req.Network != "" {
		form.Set("net0", fmt.Sprintf("name=eth0,bridge=%s", req.Network))
	}
	if req.Password != "" {
		form.Set("password", req.Password)
	}
	if req.SSHKeys != "" {
		form.Set("ssh-public-keys", strings.ReplaceAll(req.SSHKeys, "\n", "\\n"))
	}
	if req.Start {
		form.Set("start", "1")
	}

	data, err := client.CreateContainer(req.Node, form)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "created", "upid": jsonRaw(data)})
}

func (p *ProxmoxPlugin) deleteContainer(client *Client, params []byte) ([]byte, error) {
	var input struct {
		Node string `json:"node"`
		VMID int    `json:"vmid"`
	}
	if err := actionInput(params, &input); err != nil || input.Node == "" || input.VMID == 0 {
		return nil, fmt.Errorf("delete_container requires 'node' and 'vmid' parameters")
	}
	data, err := client.DeleteContainer(input.Node, input.VMID)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "deleted", "upid": jsonRaw(data)})
}

func (p *ProxmoxPlugin) clusterResources(client *Client) ([]byte, error) {
	resources, err := client.ClusterResources()
	if err != nil {
		return nil, err
	}
	return actionOutput(resources)
}

func (p *ProxmoxPlugin) listPools(client *Client) ([]byte, error) {
	data, err := client.ListPools()
	if err != nil {
		return nil, err
	}
	return actionOutput(jsonRaw(data))
}

func (p *ProxmoxPlugin) listStorage(client *Client) ([]byte, error) {
	data, err := client.ListStorage()
	if err != nil {
		return nil, err
	}
	return actionOutput(jsonRaw(data))
}

func (p *ProxmoxPlugin) nextID(client *Client) ([]byte, error) {
	data, err := client.NextID()
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"vmid": jsonRaw(data)})
}

func (p *ProxmoxPlugin) listOSTemplates(client *Client, params []byte) ([]byte, error) {
	var input struct {
		Node string `json:"node"`
	}
	if err := actionInput(params, &input); err != nil || input.Node == "" {
		return nil, fmt.Errorf("list_os_templates requires 'node' parameter")
	}
	data, err := client.ListOSTemplates(input.Node)
	if err != nil {
		return nil, err
	}
	return actionOutput(jsonRaw(data))
}

func (p *ProxmoxPlugin) listStorageContent(client *Client, params []byte) ([]byte, error) {
	var input struct {
		Storage string `json:"storage"`
		Content string `json:"content"` // e.g. "vztmpl,iso"
	}
	if err := actionInput(params, &input); err != nil || input.Storage == "" {
		return nil, fmt.Errorf("list_storage_content requires 'storage' parameter")
	}
	data, err := client.ListStorageContent(input.Storage, input.Content)
	if err != nil {
		return nil, err
	}
	return actionOutput(jsonRaw(data))
}

func (p *ProxmoxPlugin) nodeSyslog(client *Client, params []byte) ([]byte, error) {
	var input struct {
		Node  string `json:"node"`
		Limit int    `json:"limit"`
		Start int    `json:"start"`
	}
	if err := actionInput(params, &input); err != nil || input.Node == "" {
		return nil, fmt.Errorf("node_syslog requires 'node' parameter")
	}
	data, err := client.NodeSyslog(input.Node, input.Start, input.Limit)
	if err != nil {
		return nil, err
	}
	return actionOutput(jsonRaw(data))
}

func (p *ProxmoxPlugin) nodeTasks(client *Client, params []byte) ([]byte, error) {
	var input struct {
		Node  string `json:"node"`
		Limit int    `json:"limit"`
	}
	if err := actionInput(params, &input); err != nil || input.Node == "" {
		return nil, fmt.Errorf("node_tasks requires 'node' parameter")
	}
	data, err := client.NodeTasks(input.Node, input.Limit)
	if err != nil {
		return nil, err
	}
	return actionOutput(jsonRaw(data))
}

func (p *ProxmoxPlugin) taskLog(client *Client, params []byte) ([]byte, error) {
	var input struct {
		Node string `json:"node"`
		UPID string `json:"upid"`
	}
	if err := actionInput(params, &input); err != nil || input.Node == "" || input.UPID == "" {
		return nil, fmt.Errorf("task_log requires 'node' and 'upid' parameters")
	}
	data, err := client.TaskLog(input.Node, input.UPID)
	if err != nil {
		return nil, err
	}
	return actionOutput(jsonRaw(data))
}

// ── Helpers ──────────────────────────────────────────────────

func actionOutput(v interface{}) ([]byte, error) {
	return sdk.JSONMarshal(v)
}

func actionInput(data []byte, v interface{}) error {
	return sdk.JSONUnmarshal(data, v)
}

// jsonRaw is a helper to wrap raw JSON for output.
func jsonRaw(data []byte) json.RawMessage {
	return json.RawMessage(data)
}
