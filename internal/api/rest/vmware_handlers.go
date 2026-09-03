package rest

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// vmwareIDRe validates vSphere managed-object IDs (e.g. vm-123, host-5, group-d1).
var vmwareIDRe = regexp.MustCompile(`^[a-z]+-\d+$`)

// registerVMwareRoutes registers REST API routes for the VMware vCenter virtualization plugin.
// These endpoints proxy requests to the vmware gRPC plugin using credentials
// from the 'vmware' connection type.
func registerVMwareRoutes(r *gin.RouterGroup, deps Dependencies) {
	vm := r.Group("/virtualization/vmware")
	{
		vm.POST("/test", vmwareTestConnection(deps))
		vm.GET("/debug-raw", vmwareDebugRaw(deps))
		vm.GET("/datacenters", vmwareListDatacenters(deps))
		vm.GET("/clusters", vmwareListClusters(deps))
		vm.GET("/hosts", vmwareListHosts(deps))
		vm.GET("/vms", vmwareListVMs(deps))
		vm.GET("/vms/:id", vmwareGetVM(deps))
		vm.POST("/vms", vmwareCreateVM(deps))
		vm.DELETE("/vms/:id", vmwareDeleteVM(deps))
		vm.POST("/vms/:id/:action", vmwareVMAction(deps))
		vm.GET("/datastores", vmwareListDatastores(deps))
		vm.GET("/networks", vmwareListNetworks(deps))
		vm.GET("/resource-pools", vmwareListResourcePools(deps))
		vm.GET("/vms/:id/snapshots", vmwareListSnapshots(deps))
		vm.POST("/vms/:id/snapshots", vmwareCreateSnapshot(deps))
		vm.DELETE("/vms/:id/snapshots/:snapId", vmwareDeleteSnapshot(deps))
		vm.POST("/vms/:id/snapshots/:snapId/revert", vmwareRevertSnapshot(deps))
		vm.POST("/vms/:id/clone", vmwareCloneVM(deps))
		vm.PATCH("/vms/:id", vmwareReconfigureVM(deps))
		vm.POST("/vms/:id/migrate", vmwareMigrateVM(deps))
		vm.GET("/connection-info", vmwareConnectionInfo(deps))
	}
}

// vmwareExec is a helper that executes a vmware plugin action with merged config.
func vmwareExec(deps Dependencies, c *gin.Context, action string, params json.RawMessage) {
	if deps.ProviderRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provider registry not available — VMware plugin may not be loaded yet"})
		return
	}

	mergedConfig := mergeStoredPluginConfig(deps, "vmware", nil, c.Request.Context())

	resp, err := deps.ProviderRegistry.ExecuteAction(c.Request.Context(), "vmware", action, params, mergedConfig)
	if err != nil {
		slog.Error("vmware plugin action failed", "action", action, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		if entityType, isStateChange := stateChangingVMwareActions[action]; isStateChange {
			logPluginActionAsync(deps, c, "vmware", action, entityType, params, false, err.Error())
		}
		return
	}
	if !resp.Success {
		slog.Error("vmware plugin action returned error", "action", action, "error", resp.Error)
		c.JSON(http.StatusBadGateway, gin.H{"error": resp.Error})
		if entityType, isStateChange := stateChangingVMwareActions[action]; isStateChange {
			logPluginActionAsync(deps, c, "vmware", action, entityType, params, false, resp.Error)
		}
		return
	}
	out := json.RawMessage(resp.Output)
	if len(out) == 0 {
		out = json.RawMessage("null")
	}
	c.JSON(http.StatusOK, gin.H{"data": out})

	if entityType, isStateChange := stateChangingVMwareActions[action]; isStateChange {
		logPluginActionAsync(deps, c, "vmware", action, entityType, params, true, "")
	}
}

// validVMwareID checks that an ID looks like a vSphere managed-object reference.
func validVMwareID(id string) bool {
	return vmwareIDRe.MatchString(id)
}

func vmwareTestConnection(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmwareExec(deps, c, "test_connection", nil)
	}
}

func vmwareDebugRaw(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmwareExec(deps, c, "debug_raw", nil)
	}
}

func vmwareListDatacenters(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmwareExec(deps, c, "list_datacenters", nil)
	}
}

func vmwareListClusters(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmwareExec(deps, c, "list_clusters", nil)
	}
}

func vmwareListHosts(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmwareExec(deps, c, "list_hosts", nil)
	}
}

func vmwareListVMs(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmwareExec(deps, c, "list_vms", nil)
	}
}

func vmwareGetVM(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmID := c.Param("id")
		if !validVMwareID(vmID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid VM ID format"})
			return
		}
		params, _ := json.Marshal(map[string]string{"vmid": vmID})
		vmwareExec(deps, c, "get_vm", params)
	}
}

func vmwareCreateVM(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body json.RawMessage
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		vmwareExec(deps, c, "create_vm", body)
	}
}

func vmwareDeleteVM(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmID := c.Param("id")
		if !validVMwareID(vmID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid VM ID format"})
			return
		}
		params, _ := json.Marshal(map[string]string{"vmid": vmID})
		vmwareExec(deps, c, "delete_vm", params)
	}
}

func vmwareVMAction(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmID := c.Param("id")
		if !validVMwareID(vmID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid VM ID format"})
			return
		}
		action := c.Param("action")

		pluginAction := ""
		switch action {
		case "start":
			pluginAction = "start_vm"
		case "stop":
			pluginAction = "stop_vm"
		case "shutdown":
			pluginAction = "shutdown_vm"
		case "reboot", "reset":
			pluginAction = "reboot_vm"
		case "suspend":
			pluginAction = "suspend_vm"
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown action: " + action})
			return
		}

		params, _ := json.Marshal(map[string]string{"vmid": vmID})
		vmwareExec(deps, c, pluginAction, params)
	}
}

func vmwareListDatastores(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmwareExec(deps, c, "list_datastores", nil)
	}
}

func vmwareListNetworks(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmwareExec(deps, c, "list_networks", nil)
	}
}

func vmwareListResourcePools(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmwareExec(deps, c, "list_resource_pools", nil)
	}
}

func vmwareListSnapshots(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmID := c.Param("id")
		if !validVMwareID(vmID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid VM ID format"})
			return
		}
		params, _ := json.Marshal(map[string]string{"vmid": vmID})
		vmwareExec(deps, c, "list_snapshots", params)
	}
}

func vmwareCreateSnapshot(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmID := c.Param("id")
		if !validVMwareID(vmID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid VM ID format"})
			return
		}
		var body struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		params, _ := json.Marshal(map[string]interface{}{
			"vmid":        vmID,
			"name":        body.Name,
			"description": body.Description,
		})
		vmwareExec(deps, c, "create_snapshot", params)
	}
}

func vmwareDeleteSnapshot(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmID := c.Param("id")
		if !validVMwareID(vmID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid VM ID format"})
			return
		}
		snapID := c.Param("snapId")
		if !validVMwareID(snapID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid snapshot ID format"})
			return
		}
		params, _ := json.Marshal(map[string]string{"vmid": vmID, "snapshot_id": snapID})
		vmwareExec(deps, c, "delete_snapshot", params)
	}
}

func vmwareRevertSnapshot(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmID := c.Param("id")
		if !validVMwareID(vmID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid VM ID format"})
			return
		}
		snapID := c.Param("snapId")
		if !validVMwareID(snapID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid snapshot ID format"})
			return
		}
		params, _ := json.Marshal(map[string]string{"vmid": vmID, "snapshot_id": snapID})
		vmwareExec(deps, c, "revert_snapshot", params)
	}
}

// vmwareConnectionInfo returns non-sensitive connection info (base URL)
// so the frontend can build "Open in vCenter" links.
func vmwareConnectionInfo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		config := mergeStoredPluginConfig(deps, "vmware", nil, c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"url": config["url"]}})
	}
}

func vmwareCloneVM(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmID := c.Param("id")
		if !validVMwareID(vmID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid VM ID format"})
			return
		}
		var body json.RawMessage
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// Merge source_vmid from URL param into the request body.
		var reqMap map[string]interface{}
		if err := json.Unmarshal(body, &reqMap); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		reqMap["source_vmid"] = vmID
		params, err := json.Marshal(reqMap)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build request"})
			return
		}
		vmwareExec(deps, c, "clone_vm", params)
	}
}

func vmwareReconfigureVM(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmID := c.Param("id")
		if !validVMwareID(vmID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid VM ID format"})
			return
		}
		var body json.RawMessage
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var reqMap map[string]interface{}
		if err := json.Unmarshal(body, &reqMap); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		reqMap["vmid"] = vmID
		params, err := json.Marshal(reqMap)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build request"})
			return
		}
		vmwareExec(deps, c, "reconfigure_vm", params)
	}
}

func vmwareMigrateVM(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		vmID := c.Param("id")
		if !validVMwareID(vmID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid VM ID format"})
			return
		}
		var body json.RawMessage
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		var reqMap map[string]interface{}
		if err := json.Unmarshal(body, &reqMap); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		reqMap["vmid"] = vmID
		params, err := json.Marshal(reqMap)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build request"})
			return
		}
		vmwareExec(deps, c, "migrate_vm", params)
	}
}

// testVMwareConnection tests a VMware vCenter connection by authenticating via the REST API.
func testVMwareConnection(deps Dependencies, c *gin.Context, connConfig map[string]any) (string, string) {
	baseURL, _ := connConfig["url"].(string)
	if baseURL == "" {
		return "error", "No vCenter URL configured"
	}
	username, _ := connConfig["username"].(string)
	if username == "" {
		return "error", "No vCenter username configured"
	}
	password, _ := connConfig["password"].(string)
	if password == "" {
		return "error", "No vCenter password configured"
	}

	baseURL = strings.TrimRight(baseURL, "/")

	// Build HTTP client
	transport := &http.Transport{}
	if insecure, _ := connConfig["insecure_tls"].(string); insecure == "true" || insecure == "1" {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // #nosec
	}
	if insecure, _ := connConfig["insecure"].(string); insecure == "true" || insecure == "1" {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // #nosec
	}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}

	// Authenticate: POST /api/session
	sessionURL := baseURL + "/api/session"
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, sessionURL, nil)
	if err != nil {
		return "error", fmt.Sprintf("Invalid URL: %v", err)
	}
	req.SetBasicAuth(username, password)

	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("Connection failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read the response body once and reuse it.
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		return "error", "Authentication failed: check vCenter username and password"
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "error", fmt.Sprintf("Authentication failed (HTTP %d): %s", resp.StatusCode, string(data))
	}

	var sessionID string
	if err := json.Unmarshal(data, &sessionID); err != nil || sessionID == "" {
		return "error", "Authentication succeeded but no session ID returned"
	}

	// Verify access by listing VMs
	vmReq, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, baseURL+"/api/vcenter/vm", nil)
	vmReq.Header.Set("vmware-api-session-id", sessionID)
	vmResp, err := client.Do(vmReq)
	if err != nil {
		return "connected", "Authenticated but cannot list VMs: " + err.Error()
	}
	defer func() { _ = vmResp.Body.Close() }()

	if vmResp.StatusCode == http.StatusUnauthorized || vmResp.StatusCode == http.StatusForbidden {
		return "connected", "Authenticated but token has no VM read access — check vCenter role permissions"
	}

	return "connected", "Connected to VMware vCenter successfully"
}
