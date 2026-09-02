package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// vmNameRe validates VM names: alphanumeric, hyphens, underscores, dots; 1-80 chars.
var vmNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$`)

// VMwarePlugin implements provider.Provider for VMware vCenter integration.
type VMwarePlugin struct{}

var _ provider.Provider = (*VMwarePlugin)(nil)

func (p *VMwarePlugin) Name() string    { return "vmware" }
func (p *VMwarePlugin) Version() string { return "0.1.0" }
func (p *VMwarePlugin) Description() string {
	return "VMware vCenter — ESXi virtual machines, hosts, datastores, and networks"
}
func (p *VMwarePlugin) PluginType() string { return "virtualization" }

func (p *VMwarePlugin) Actions() []string {
	return []string{
		"test_connection",
		"debug_raw",
		"list_datacenters",
		"list_clusters",
		"list_hosts",
		"list_vms",
		"get_vm",
		"start_vm",
		"stop_vm",
		"shutdown_vm",
		"reboot_vm",
		"suspend_vm",
		"create_vm",
		"delete_vm",
		"list_datastores",
		"list_networks",
		"list_resource_pools",
		"list_snapshots",
		"create_snapshot",
		"delete_snapshot",
		"revert_snapshot",
		"clone_vm",
		"reconfigure_vm",
		"migrate_vm",
	}
}

func (p *VMwarePlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	client, err := p.buildClient(config)
	if err != nil {
		return nil, err
	}

	switch action {
	case "test_connection":
		return p.testConnection(client)
	case "debug_raw":
		return p.debugRaw(client)
	case "list_datacenters":
		return p.listDatacenters(client)
	case "list_clusters":
		return p.listClusters(client)
	case "list_hosts":
		return p.listHosts(client)
	case "list_vms":
		return p.listVMs(client)
	case "get_vm":
		return p.getVM(client, params)
	case "start_vm":
		return p.vmAction(client, params, "start")
	case "stop_vm":
		return p.vmAction(client, params, "stop")
	case "shutdown_vm":
		return p.vmAction(client, params, "shutdown")
	case "reboot_vm":
		return p.vmAction(client, params, "reset")
	case "suspend_vm":
		return p.vmAction(client, params, "suspend")
	case "create_vm":
		return p.createVM(client, params)
	case "delete_vm":
		return p.deleteVM(client, params)
	case "list_datastores":
		return p.listDatastores(client)
	case "list_networks":
		return p.listNetworks(client)
	case "list_resource_pools":
		return p.listResourcePools(client)
	case "list_snapshots":
		return p.listSnapshots(client, params)
	case "create_snapshot":
		return p.createSnapshot(client, params)
	case "delete_snapshot":
		return p.deleteSnapshot(client, params)
	case "revert_snapshot":
		return p.revertSnapshot(client, params)
	case "clone_vm":
		return p.cloneVM(client, params)
	case "reconfigure_vm":
		return p.reconfigureVM(client, params)
	case "migrate_vm":
		return p.migrateVM(client, params)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *VMwarePlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "VMware plugin ready — requires connection config (url, username, password)",
	}, nil
}

// buildClient creates a vSphere API client from the connection config.
func (p *VMwarePlugin) buildClient(config map[string]string) (*Client, error) {
	baseURL := config["url"]
	if baseURL == "" {
		return nil, fmt.Errorf("vmware plugin requires 'url' in connection config")
	}
	username := config["username"]
	if username == "" {
		return nil, fmt.Errorf("vmware plugin requires 'username' in connection config")
	}
	password := config["password"]
	if password == "" {
		return nil, fmt.Errorf("vmware plugin requires 'password' in connection config")
	}
	insecureTLS := config["insecure_tls"] == "true" || config["insecure_tls"] == "1"
	return NewClient(baseURL, username, password, insecureTLS), nil
}

// ── Action implementations ────────────────────────────────────

func (p *VMwarePlugin) testConnection(client *Client) ([]byte, error) {
	ver, err := client.TestConnection()
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{
		"status":  "connected",
		"version": ver,
	})
}

// debugRaw returns raw API responses for debugging.
func (p *VMwarePlugin) debugRaw(client *Client) ([]byte, error) {
	vmData, vmErr := client.RawGet("/api/vcenter/vm")
	hostsData, hostsErr := client.RawGet("/api/vcenter/host")
	rpData, rpErr := client.RawGet("/api/vcenter/resource-pool")

	// Find first VM and first host.
	var firstVM, firstHostID string
	if vmErr == nil {
		var vms []map[string]interface{}
		if json.Unmarshal(vmData, &vms) == nil {
			for _, v := range vms {
				if vmID, _ := v["vm"].(string); vmID != "" {
					firstVM = vmID
					break
				}
			}
		}
	}
	if hostsErr == nil {
		var hosts []map[string]interface{}
		if json.Unmarshal(hostsData, &hosts) == nil && len(hosts) > 0 {
			firstHostID, _ = hosts[0]["host"].(string)
		}
	}

	// Guest identity for first VM.
	var guestIdentity json.RawMessage
	var guestErr error
	if firstVM != "" {
		guestIdentity, guestErr = client.RawGet(fmt.Sprintf("/api/vcenter/vm/%s/guest/identity", firstVM))
	}

	result := map[string]interface{}{
		"vms_response":         string(vmData),
		"vms_error":            fmt.Sprintf("%v", vmErr),
		"hosts_summary":        string(hostsData),
		"hosts_error":          fmt.Sprintf("%v", hostsErr),
		"resource_pools":       string(rpData),
		"resource_pools_error": fmt.Sprintf("%v", rpErr),
		"guest_identity":       string(guestIdentity),
		"guest_identity_error": fmt.Sprintf("%v", guestErr),
		"first_vm":             firstVM,
		"first_host_id":        firstHostID,
	}
	return actionOutput(result)
}

func (p *VMwarePlugin) listDatacenters(client *Client) ([]byte, error) {
	dcs, err := client.ListDatacenters()
	if err != nil {
		return nil, err
	}
	return actionOutput(dcs)
}

func (p *VMwarePlugin) listClusters(client *Client) ([]byte, error) {
	clusters, err := client.ListClusters()
	if err != nil {
		return nil, err
	}
	return actionOutput(clusters)
}

func (p *VMwarePlugin) listHosts(client *Client) ([]byte, error) {
	hosts, err := client.ListHosts()
	if err != nil {
		return nil, err
	}
	return actionOutput(hosts)
}

func (p *VMwarePlugin) listVMs(client *Client) ([]byte, error) {
	vms, err := client.ListVMs()
	if err != nil {
		return nil, err
	}
	return actionOutput(vms)
}

func (p *VMwarePlugin) getVM(client *Client, params []byte) ([]byte, error) {
	var input struct {
		VMID string `json:"vmid"`
	}
	if err := actionInput(params, &input); err != nil || input.VMID == "" {
		return nil, fmt.Errorf("get_vm requires 'vmid' parameter")
	}
	detail, err := client.GetVM(input.VMID)
	if err != nil {
		return nil, err
	}
	return actionOutput(detail)
}

func (p *VMwarePlugin) vmAction(client *Client, params []byte, action string) ([]byte, error) {
	var input struct {
		VMID string `json:"vmid"`
	}
	if err := actionInput(params, &input); err != nil || input.VMID == "" {
		return nil, fmt.Errorf("%s requires 'vmid' parameter", action)
	}
	if err := client.VMAction(input.VMID, action); err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "ok", "action": action, "vmid": input.VMID})
}

func (p *VMwarePlugin) createVM(client *Client, params []byte) ([]byte, error) {
	var req struct {
		Name       string `json:"name"`
		Host       string `json:"host,omitempty"`
		Cluster    string `json:"cluster,omitempty"`
		Datastore  string `json:"datastore,omitempty"`
		ResourcePool string `json:"resource_pool,omitempty"`
		Cores      int    `json:"cores,omitempty"`
		MemoryMiB  int64  `json:"memory_mib,omitempty"`
		GuestOS    string `json:"guest_os,omitempty"`
		Network    string `json:"network,omitempty"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, fmt.Errorf("create_vm: parse params: %w", err)
	}
	if req.Name == "" {
		return nil, fmt.Errorf("create_vm requires 'name'")
	}

	spec := map[string]interface{}{
		"name": req.Name,
	}
	if req.Host != "" {
		spec["host"] = req.Host
	}
	if req.Cluster != "" {
		spec["cluster"] = req.Cluster
	}
	if req.Datastore != "" {
		spec["datastore"] = req.Datastore
	}
	if req.ResourcePool != "" {
		spec["resource_pool"] = req.ResourcePool
	}
	if req.Cores > 0 {
		spec["cpu"] = map[string]interface{}{
			"count":            req.Cores,
			"cores_per_socket": req.Cores,
		}
	}
	if req.MemoryMiB > 0 {
		spec["memory"] = map[string]interface{}{
			"size_MiB": req.MemoryMiB,
		}
	}
	if req.GuestOS != "" {
		spec["guest_OS"] = req.GuestOS
	}

	vmID, err := client.CreateVM(spec)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "created", "vmid": vmID})
}

func (p *VMwarePlugin) deleteVM(client *Client, params []byte) ([]byte, error) {
	var input struct {
		VMID string `json:"vmid"`
	}
	if err := actionInput(params, &input); err != nil || input.VMID == "" {
		return nil, fmt.Errorf("delete_vm requires 'vmid' parameter")
	}
	if err := client.DeleteVM(input.VMID); err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "deleted", "vmid": input.VMID})
}

func (p *VMwarePlugin) listDatastores(client *Client) ([]byte, error) {
	stores, err := client.ListDatastores()
	if err != nil {
		return nil, err
	}
	return actionOutput(stores)
}

func (p *VMwarePlugin) listNetworks(client *Client) ([]byte, error) {
	nets, err := client.ListNetworks()
	if err != nil {
		return nil, err
	}
	return actionOutput(nets)
}

func (p *VMwarePlugin) listResourcePools(client *Client) ([]byte, error) {
	pools, err := client.ListResourcePools()
	if err != nil {
		return nil, err
	}
	return actionOutput(pools)
}

func (p *VMwarePlugin) listSnapshots(client *Client, params []byte) ([]byte, error) {
	var input struct {
		VMID string `json:"vmid"`
	}
	if err := actionInput(params, &input); err != nil || input.VMID == "" {
		return nil, fmt.Errorf("list_snapshots requires 'vmid' parameter")
	}
	snaps, err := client.ListSnapshots(input.VMID)
	if err != nil {
		return nil, err
	}
	return actionOutput(snaps)
}

func (p *VMwarePlugin) createSnapshot(client *Client, params []byte) ([]byte, error) {
	var input struct {
		VMID        string `json:"vmid"`
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}
	if err := actionInput(params, &input); err != nil || input.VMID == "" || input.Name == "" {
		return nil, fmt.Errorf("create_snapshot requires 'vmid' and 'name' parameters")
	}
	snapID, err := client.CreateSnapshot(input.VMID, input.Name, input.Description)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "created", "snapshot_id": snapID})
}

func (p *VMwarePlugin) deleteSnapshot(client *Client, params []byte) ([]byte, error) {
	var input struct {
		VMID       string `json:"vmid"`
		SnapshotID string `json:"snapshot_id"`
	}
	if err := actionInput(params, &input); err != nil || input.VMID == "" || input.SnapshotID == "" {
		return nil, fmt.Errorf("delete_snapshot requires 'vmid' and 'snapshot_id' parameters")
	}
	if err := client.DeleteSnapshot(input.VMID, input.SnapshotID); err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "deleted"})
}

func (p *VMwarePlugin) revertSnapshot(client *Client, params []byte) ([]byte, error) {
	var input struct {
		VMID       string `json:"vmid"`
		SnapshotID string `json:"snapshot_id"`
	}
	if err := actionInput(params, &input); err != nil || input.VMID == "" || input.SnapshotID == "" {
		return nil, fmt.Errorf("revert_snapshot requires 'vmid' and 'snapshot_id' parameters")
	}
	if err := client.RevertSnapshot(input.VMID, input.SnapshotID); err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "reverted"})
}

func (p *VMwarePlugin) cloneVM(client *Client, params []byte) ([]byte, error) {
	var req struct {
		SourceVMID   string `json:"source_vmid"`
		Name         string `json:"name"`
		Host         string `json:"host,omitempty"`
		Datastore    string `json:"datastore,omitempty"`
		ResourcePool string `json:"resource_pool,omitempty"`
		PowerOn      bool   `json:"power_on,omitempty"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, fmt.Errorf("clone_vm: parse params: %w", err)
	}
	if req.SourceVMID == "" || req.Name == "" {
		return nil, fmt.Errorf("clone_vm requires 'source_vmid' and 'name'")
	}
	if !vmNameRe.MatchString(req.Name) {
		return nil, fmt.Errorf("clone_vm: invalid VM name %q (alphanumeric, hyphens, underscores, dots; 1-80 chars)", req.Name)
	}

	spec := map[string]interface{}{
		"name": req.Name,
	}
	if req.Host != "" {
		spec["host"] = req.Host
	}
	if req.Datastore != "" {
		spec["datastore"] = req.Datastore
	}
	if req.ResourcePool != "" {
		spec["resource_pool"] = req.ResourcePool
	}
	if req.PowerOn {
		spec["power_on"] = true
	}

	vmID, err := client.CloneVM(req.SourceVMID, spec)
	if err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "cloned", "vmid": vmID})
}

func (p *VMwarePlugin) reconfigureVM(client *Client, params []byte) ([]byte, error) {
	var req struct {
		VMID      string `json:"vmid"`
		Cores     int    `json:"cores,omitempty"`
		MemoryMiB int64  `json:"memory_mib,omitempty"`
	}
	if err := actionInput(params, &req); err != nil || req.VMID == "" {
		return nil, fmt.Errorf("reconfigure_vm requires 'vmid' parameter")
	}

	spec := make(map[string]interface{})
	if req.Cores > 0 {
		spec["cpu"] = map[string]interface{}{
			"count":            req.Cores,
			"cores_per_socket": req.Cores,
		}
	}
	if req.MemoryMiB > 0 {
		spec["memory"] = map[string]interface{}{
			"size_MiB": req.MemoryMiB,
		}
	}
	if len(spec) == 0 {
		return nil, fmt.Errorf("reconfigure_vm requires at least 'cores' or 'memory_mib'")
	}

	if err := client.ReconfigureVM(req.VMID, spec); err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "reconfigured", "vmid": req.VMID})
}

func (p *VMwarePlugin) migrateVM(client *Client, params []byte) ([]byte, error) {
	var req struct {
		VMID       string `json:"vmid"`
		TargetHost string `json:"target_host"`
	}
	if err := actionInput(params, &req); err != nil || req.VMID == "" || req.TargetHost == "" {
		return nil, fmt.Errorf("migrate_vm requires 'vmid' and 'target_host' parameters")
	}
	if err := client.MigrateVM(req.VMID, req.TargetHost); err != nil {
		return nil, err
	}
	return actionOutput(map[string]interface{}{"status": "migrated", "vmid": req.VMID, "target_host": req.TargetHost})
}

// ── Helpers ──────────────────────────────────────────────────

func actionOutput(v interface{}) ([]byte, error) {
	return sdk.JSONMarshal(v)
}

func actionInput(data []byte, v interface{}) error {
	return sdk.JSONUnmarshal(data, v)
}
