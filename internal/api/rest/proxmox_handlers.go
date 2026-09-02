package rest

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// registerProxmoxRoutes registers REST API routes for the Proxmox virtualization plugin.
// These endpoints proxy requests to the proxmox gRPC plugin using credentials
// from the 'proxmox' connection type.
func registerProxmoxRoutes(r *gin.RouterGroup, deps Dependencies) {
	// Root virtualization endpoint — list available providers
	virt := r.Group("/virtualization")
	{
		virt.GET("", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"providers": []gin.H{
					{"id": "proxmox", "name": "Proxmox VE", "status": "available"},
					{"id": "vmware", "name": "VMware vCenter", "status": "available"},
				},
				"total": 2,
			})
		})
	}

	px := r.Group("/virtualization/proxmox")
	{
		px.POST("/test", proxmoxTestConnection(deps))
		px.GET("/nodes", proxmoxListNodes(deps))
		px.GET("/nodes/:node", proxmoxGetNode(deps))
		px.GET("/vms", proxmoxListVMs(deps))
		px.GET("/vms/:node/:vmid", proxmoxGetVM(deps))
		px.POST("/vms", proxmoxCreateVM(deps))
		px.DELETE("/vms/:node/:vmid", proxmoxDeleteVM(deps))
		px.POST("/vms/:node/:vmid/:action", proxmoxVMAction(deps))
		px.GET("/containers", proxmoxListContainers(deps))
		px.POST("/containers", proxmoxCreateContainer(deps))
		px.POST("/containers/docker", proxmoxDeployDocker(deps))
		px.DELETE("/containers/:node/:vmid", proxmoxDeleteContainer(deps))
		px.POST("/containers/:node/:vmid/:action", proxmoxContainerAction(deps))
		px.GET("/resources", proxmoxClusterResources(deps))
		px.GET("/pools", proxmoxListPools(deps))
		px.GET("/storage", proxmoxListStorage(deps))
		px.GET("/storage/:storage/content", proxmoxListStorageContent(deps))
		px.GET("/permissions", proxmoxGetPermissions(deps))
		px.GET("/connection-info", proxmoxConnectionInfo(deps))
		px.GET("/next-id", proxmoxNextID(deps))
		px.GET("/nodes/:node/templates", proxmoxListOSTemplates(deps))
		px.GET("/nodes/:node/syslog", proxmoxNodeSyslog(deps))
		px.GET("/nodes/:node/tasks", proxmoxNodeTasks(deps))
		px.GET("/nodes/:node/tasks/:upid/log", proxmoxTaskLog(deps))
	}
}

// proxmoxExec is a helper that executes a proxmox plugin action with merged config.
// It unwraps the plugin output so the frontend receives {"data": <action output>}.
func proxmoxExec(deps Dependencies, c *gin.Context, action string, params json.RawMessage) {
	if deps.ProviderRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provider registry not available"})
		return
	}

	// Merge stored plugin config from DB with request config
	mergedConfig := mergeStoredPluginConfig(deps, "proxmox", nil, c.Request.Context())

	resp, err := deps.ProviderRegistry.ExecuteAction(c.Request.Context(), "proxmox", action, params, mergedConfig)
	if err != nil {
		respondInternalError(c, err)
		// Log the failed action
		if entityType, isStateChange := stateChangingProxmoxActions[action]; isStateChange {
			logPluginActionAsync(deps, c, "proxmox", action, entityType, string(params), false, err.Error())
		}
		return
	}
	if !resp.Success {
		c.JSON(http.StatusBadGateway, gin.H{"error": resp.Error})
		// Log the failed action
		if entityType, isStateChange := stateChangingProxmoxActions[action]; isStateChange {
			logPluginActionAsync(deps, c, "proxmox", action, entityType, string(params), false, resp.Error)
		}
		return
	}
	out := json.RawMessage(resp.Output)
	if len(out) == 0 {
		out = json.RawMessage("null")
	}
	c.JSON(http.StatusOK, gin.H{"data": out})

	// Log successful state-changing actions
	if entityType, isStateChange := stateChangingProxmoxActions[action]; isStateChange {
		logPluginActionAsync(deps, c, "proxmox", action, entityType, string(params), true, "")
	}
}

func proxmoxTestConnection(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxmoxExec(deps, c, "test_connection", nil)
	}
}

func proxmoxListNodes(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxmoxExec(deps, c, "list_nodes", nil)
	}
}

func proxmoxGetNode(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		node := c.Param("node")
		params, _ := json.Marshal(map[string]string{"node": node})
		proxmoxExec(deps, c, "get_node", params)
	}
}

func proxmoxListVMs(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxmoxExec(deps, c, "list_vms", nil)
	}
}

func proxmoxGetVM(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		node := c.Param("node")
		vmidStr := c.Param("vmid")
		vmid, err := strconv.Atoi(vmidStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vmid"})
			return
		}
		params, _ := json.Marshal(map[string]interface{}{"node": node, "vmid": vmid})
		proxmoxExec(deps, c, "get_vm", params)
	}
}

func proxmoxCreateVM(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body json.RawMessage
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		proxmoxExec(deps, c, "create_vm", body)
	}
}

func proxmoxDeleteVM(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		node := c.Param("node")
		vmidStr := c.Param("vmid")
		vmid, err := strconv.Atoi(vmidStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vmid"})
			return
		}
		params, _ := json.Marshal(map[string]interface{}{"node": node, "vmid": vmid})
		proxmoxExec(deps, c, "delete_vm", params)
	}
}

func proxmoxVMAction(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		node := c.Param("node")
		vmidStr := c.Param("vmid")
		action := c.Param("action")

		vmid, err := strconv.Atoi(vmidStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vmid"})
			return
		}

		// Map URL action to plugin action
		pluginAction := ""
		switch action {
		case "start":
			pluginAction = "start_vm"
		case "stop":
			pluginAction = "stop_vm"
		case "shutdown":
			pluginAction = "shutdown_vm"
		case "reboot":
			pluginAction = "reboot_vm"
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown action: " + action})
			return
		}

		params, _ := json.Marshal(map[string]interface{}{"node": node, "vmid": vmid})
		proxmoxExec(deps, c, pluginAction, params)
	}
}

func proxmoxListContainers(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxmoxExec(deps, c, "list_containers", nil)
	}
}

func proxmoxCreateContainer(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body json.RawMessage
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		proxmoxExec(deps, c, "create_container", body)
	}
}

// proxmoxDeployDocker creates an LXC container and provisions a Docker
// workload inside it (registry image, local folder, or local Docker image).
func proxmoxDeployDocker(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body json.RawMessage
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		proxmoxExec(deps, c, "deploy_docker", body)
	}
}

func proxmoxDeleteContainer(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		node := c.Param("node")
		vmidStr := c.Param("vmid")
		vmid, err := strconv.Atoi(vmidStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vmid"})
			return
		}
		params, _ := json.Marshal(map[string]interface{}{"node": node, "vmid": vmid})
		proxmoxExec(deps, c, "delete_container", params)
	}
}

func proxmoxContainerAction(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		node := c.Param("node")
		vmidStr := c.Param("vmid")
		action := c.Param("action")

		vmid, err := strconv.Atoi(vmidStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vmid"})
			return
		}

		pluginAction := ""
		switch action {
		case "start":
			pluginAction = "start_container"
		case "stop":
			pluginAction = "stop_container"
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown action: " + action})
			return
		}

		params, _ := json.Marshal(map[string]interface{}{"node": node, "vmid": vmid})
		proxmoxExec(deps, c, pluginAction, params)
	}
}

func proxmoxClusterResources(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxmoxExec(deps, c, "cluster_resources", nil)
	}
}

func proxmoxListPools(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxmoxExec(deps, c, "list_pools", nil)
	}
}

func proxmoxListStorage(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxmoxExec(deps, c, "list_storage", nil)
	}
}

func proxmoxGetPermissions(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxmoxExec(deps, c, "get_permissions", nil)
	}
}

// proxmoxConnectionInfo returns non-sensitive connection info (base URL)
// so the frontend can build "Open in Proxmox" links.
func proxmoxConnectionInfo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		config := mergeStoredPluginConfig(deps, "proxmox", nil, c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"url": config["url"]}})
	}
}

func proxmoxNextID(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		proxmoxExec(deps, c, "next_id", nil)
	}
}

func proxmoxListOSTemplates(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		params, _ := json.Marshal(map[string]string{"node": c.Param("node")})
		proxmoxExec(deps, c, "list_os_templates", params)
	}
}

func proxmoxListStorageContent(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		params, _ := json.Marshal(map[string]string{
			"storage": c.Param("storage"),
			"content": c.DefaultQuery("content", "vztmpl,iso"),
		})
		proxmoxExec(deps, c, "list_storage_content", params)
	}
}

func proxmoxNodeSyslog(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
		params, _ := json.Marshal(map[string]interface{}{"node": c.Param("node"), "limit": limit})
		proxmoxExec(deps, c, "node_syslog", params)
	}
}

func proxmoxNodeTasks(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		params, _ := json.Marshal(map[string]interface{}{"node": c.Param("node"), "limit": limit})
		proxmoxExec(deps, c, "node_tasks", params)
	}
}

func proxmoxTaskLog(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		params, _ := json.Marshal(map[string]string{"node": c.Param("node"), "upid": c.Param("upid")})
		proxmoxExec(deps, c, "task_log", params)
	}
}

// testProxmoxConnection tests a Proxmox connection by calling the Proxmox API version endpoint.
func testProxmoxConnection(deps Dependencies, c *gin.Context, connConfig map[string]any) (string, string) {
	baseURL, _ := connConfig["url"].(string)
	if baseURL == "" {
		return "error", "No Proxmox URL configured"
	}

	// Build HTTP client
	transport := &http.Transport{}
	if insecure, _ := connConfig["insecure_tls"].(string); insecure == "true" || insecure == "1" {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	if insecure, _ := connConfig["insecure"].(string); insecure == "true" || insecure == "1" {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	// Try token-based auth first
	tokenID, _ := connConfig["token_id"].(string)
	tokenSecret, _ := connConfig["token_secret"].(string)

	reqURL := strings.TrimRight(baseURL, "/") + "/api2/json/version"
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, reqURL, nil)
	if err != nil {
		return "error", fmt.Sprintf("Invalid URL: %v", err)
	}

	if tokenID != "" && tokenSecret != "" {
		req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", tokenID, tokenSecret))
	} else if username, _ := connConfig["username"].(string); username != "" {
		// Username/password: first get a ticket
		password, _ := connConfig["password"].(string)
		ticketReq, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodPost,
			strings.TrimRight(baseURL, "/")+"/api2/json/access/ticket",
			strings.NewReader(url.Values{"username": {username}, "password": {password}}.Encode()))
		ticketReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ticketResp, err := client.Do(ticketReq)
		if err != nil {
			return "error", fmt.Sprintf("Connection failed: %v", err)
		}
		defer func() { _ = ticketResp.Body.Close() }()
		if ticketResp.StatusCode == http.StatusUnauthorized || ticketResp.StatusCode == http.StatusForbidden {
			return "error", "Authentication failed: check Proxmox username and password"
		}
		if ticketResp.StatusCode >= 200 && ticketResp.StatusCode < 300 {
			var ticketResult struct {
				Data struct {
					Ticket string `json:"ticket"`
				} `json:"data"`
			}
			if err := json.NewDecoder(ticketResp.Body).Decode(&ticketResult); err == nil && ticketResult.Data.Ticket != "" {
				// Re-create the version request with the ticket
				req, _ = http.NewRequestWithContext(c.Request.Context(), http.MethodGet, reqURL, nil)
				req.Header.Set("Cookie", "PVEAuthCookie="+ticketResult.Data.Ticket)
			}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("Connection failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "error", "Authentication failed: check credentials"
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "error", fmt.Sprintf("Unexpected HTTP status %d", resp.StatusCode)
	}

	var version struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return "error", fmt.Sprintf("Invalid API response: %v", err)
	}

	message := "Connection successful"
	if version.Data.Version != "" {
		message = "Connected to Proxmox VE " + version.Data.Version
	}

	// Warn if the token has no usable permissions (common misconfiguration).
	permReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet,
		strings.TrimRight(baseURL, "/")+"/api2/json/access/permissions", nil)
	if err == nil {
		permReq.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", tokenID, tokenSecret))
		if permResp, err := client.Do(permReq); err == nil {
			defer func() { _ = permResp.Body.Close() }()
			if permResp.StatusCode >= 200 && permResp.StatusCode < 300 {
				var permEnvelope struct {
					Data map[string]map[string]int `json:"data"`
				}
				if json.NewDecoder(permResp.Body).Decode(&permEnvelope) == nil && !hasUsableProxmoxPerms(permEnvelope.Data) {
					message += " — WARNING: token has no permissions; assign a role (e.g. PVEAdministrator) to the token in Datacenter → Permissions"
				}
			}
		}
	}
	return "connected", message
}

// hasUsableProxmoxPerms reports whether the permission map grants any
// meaningful access (Sys.Audit or VM.* privileges).
func hasUsableProxmoxPerms(perms map[string]map[string]int) bool {
	for _, acl := range perms {
		for perm, granted := range acl {
			if granted == 1 && (perm == "Sys.Audit" || strings.HasPrefix(perm, "VM.")) {
				return true
			}
		}
	}
	return false
}
