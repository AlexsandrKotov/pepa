package rest

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/k8s"
	"github.com/pepa/pepa/internal/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func registerConnectionRoutes(r *gin.RouterGroup, deps Dependencies) {
	conns := r.Group("/connections")
	{
		conns.GET("", listConnections(deps))
		conns.POST("", createConnection(deps))
		conns.GET("/summary", connectionSummary(deps))
		conns.GET("/plugin-status", connectionPluginStatus(deps))
		conns.POST("/parse-kubeconfig", parseKubeconfig(deps))
		conns.GET("/:id", getConnection(deps))
		conns.PUT("/:id", updateConnection(deps))
		conns.DELETE("/:id", deleteConnection(deps))
		conns.POST("/:id/test", testConnection(deps))
		conns.GET("/:id/browse", browseConnection(deps))
		conns.POST("/:id/execute", executeConnectionAction(deps))
	}
}

func listConnections(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Connection == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "connection repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)
		connType := c.Query("type")

		items, err := deps.Repos.Connection.List(c.Request.Context(), tenantID, connType)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if items == nil {
			items = []repository.Connection{}
		}
		// Mask sensitive config values in list response to reduce payload and avoid leaking secrets
		for i := range items {
			sanitized := make(map[string]any, len(items[i].Config))
			for k, v := range items[i].Config {
				if isSensitiveConfigKey(k) {
					sanitized[k] = "***"
				} else {
					sanitized[k] = v
				}
			}
			items[i].Config = sanitized
		}
		c.JSON(http.StatusOK, gin.H{"connections": items, "total": len(items)})
	}
}

func getConnection(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Connection == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "connection repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection ID"})
			return
		}
		tenantID := auth.GetTenantID(c)
		conn, err := deps.Repos.Connection.Get(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
			return
		}
		// Mask sensitive config values to prevent leaking secrets
		sanitized := make(map[string]any, len(conn.Config))
		for k, v := range conn.Config {
			if isSensitiveConfigKey(k) {
				sanitized[k] = "***"
			} else {
				sanitized[k] = v
			}
		}
		conn.Config = sanitized
		c.JSON(http.StatusOK, conn)
	}
}

func createConnection(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Connection == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "connection repository not available"})
			return
		}
		var req struct {
			Type        repository.ConnectionType `json:"type" binding:"required"`
			Name        string                    `json:"name" binding:"required"`
			Description string                    `json:"description"`
			Config      map[string]any            `json:"config"`
			Labels      map[string]string         `json:"labels"`
			Notes       string                    `json:"notes"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validate that the required plugin is installed and enabled
		if deps.ProviderRegistry != nil {
			pluginName := requiredPluginForConnection(string(req.Type), req.Config)
			if pluginName != "" {
				entry, ok := deps.ProviderRegistry.Get(pluginName)
				if !ok {
					c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Cannot create %s connection: plugin %q is not installed. Install it from the Marketplace first.", req.Type, pluginName)})
					return
				}
				if !entry.Enabled {
					c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Cannot create %s connection: plugin %q is disabled. Enable it in the Plugins page first.", req.Type, pluginName)})
					return
				}
			}
		}

		conn := &repository.Connection{
			TenantID:    auth.GetTenantID(c),
			Type:        req.Type,
			Name:        req.Name,
			Description: req.Description,
			Config:      req.Config,
			Labels:      req.Labels,
			Notes:       req.Notes,
			Status:      "disconnected",
		}
		if conn.Config == nil {
			conn.Config = map[string]any{}
		}
		if conn.Labels == nil {
			conn.Labels = map[string]string{}
		}

		if err := deps.Repos.Connection.Create(c.Request.Context(), conn); err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "duplicate key") || strings.Contains(errMsg, "unique constraint") {
				c.JSON(http.StatusConflict, gin.H{"error": "A connection with this name already exists"})
				return
			}
			respondInternalError(c, err)
			return
		}

		// Auto-sync: create cluster entry for kubernetes connections
		syncClusterFromConnection(deps, c, conn)

		// Auto-sync: register AI provider connections with the AI manager
		applyAIConnection(deps, c.Request.Context(), conn)

		logAudit(deps, c, "create", "connection", conn.ID.String(), nil, gin.H{"name": conn.Name, "type": string(conn.Type)})
		c.JSON(http.StatusCreated, conn)
	}
}

func updateConnection(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Connection == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "connection repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection ID"})
			return
		}
		tenantID := auth.GetTenantID(c)
		conn, err := deps.Repos.Connection.Get(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
			return
		}

		var req struct {
			Name        string            `json:"name"`
			Description string            `json:"description"`
			Config      map[string]any    `json:"config"`
			Labels      map[string]string `json:"labels"`
			Notes       string            `json:"notes"`
			Status      string            `json:"status"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Name != "" {
			conn.Name = req.Name
		}
		conn.Description = req.Description
		if req.Config != nil {
			conn.Config = req.Config
		}
		if req.Labels != nil {
			conn.Labels = req.Labels
		}
		if req.Status != "" {
			conn.Status = req.Status
		}
		conn.Notes = req.Notes

		if err := deps.Repos.Connection.Update(c.Request.Context(), conn); err != nil {
			respondInternalError(c, err)
			return
		}

		// Auto-sync: update cluster entry for kubernetes connections
		syncClusterFromConnection(deps, c, conn)

		// Auto-sync: re-register AI provider connections with the AI manager
		applyAIConnection(deps, c.Request.Context(), conn)

		logAudit(deps, c, "update", "connection", conn.ID.String(), nil, gin.H{"name": conn.Name, "status": conn.Status})
		c.JSON(http.StatusOK, conn)
	}
}

func deleteConnection(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Connection == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "connection repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection ID"})
			return
		}

		// Capture the connection before deletion to clean up linked resources
		tenantID := auth.GetTenantID(c)
		existing, _ := deps.Repos.Connection.Get(c.Request.Context(), id, tenantID)

		// Clean up linked cluster before deleting connection
		if deps.Repos.Cluster != nil {
			_ = deps.Repos.Cluster.DeleteByConnectionID(c.Request.Context(), id)
		}

		if err := deps.Repos.Connection.Delete(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}

		// Auto-sync: unregister AI provider if no other connection backs it
		if existing != nil && existing.Type == repository.ConnectionAI {
			provider, _ := existing.Config["provider"].(string)
			resyncAIProviderAfterDelete(deps, c.Request.Context(), provider)
		}

		logAudit(deps, c, "delete", "connection", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "connection deleted"})
	}
}

// applyAIConnection registers an AI-type connection's provider with the AI
// manager so it becomes available to the AI Assistant. The applied connection
// becomes the default provider. Connections are the single place to configure
// AI providers (Settings → AI was removed).
func applyAIConnection(deps Dependencies, ctx context.Context, conn *repository.Connection) {
	if deps.AIManager == nil || conn == nil || conn.Type != repository.ConnectionAI {
		return
	}
	provider, _ := conn.Config["provider"].(string)
	if provider == "" {
		return
	}
	apiKey, _ := conn.Config["api_key"].(string)
	baseURL, _ := conn.Config["base_url"].(string)
	model, _ := conn.Config["model"].(string)

	// Resolve vault references in the API key
	if strings.HasPrefix(apiKey, "vault:") {
		if resolved, err := resolveVaultRef(deps, ctx, apiKey, conn.TenantID); err == nil {
			apiKey = resolved
		} else {
			log.Printf("Warning: cannot resolve vault reference for AI connection %q: %v", conn.Name, err)
		}
	}

	if err := deps.AIManager.ConfigureProvider(provider, apiKey, baseURL, model); err != nil {
		log.Printf("Warning: failed to apply AI connection %q: %v", conn.Name, err)
		return
	}
	deps.AIManager.SetDefaultProvider(provider)
	log.Printf("AI provider %q configured from connection %q", provider, conn.Name)
}

// resyncAIProviderAfterDelete keeps the AI manager in sync after an AI
// connection was deleted: if another connection backs the same provider it is
// applied, otherwise the provider is unregistered.
func resyncAIProviderAfterDelete(deps Dependencies, ctx context.Context, provider string) {
	if deps.AIManager == nil || provider == "" || deps.Repos.Connection == nil {
		return
	}
	conns, err := deps.Repos.Connection.FindByTypeDecrypted(ctx, string(repository.ConnectionAI))
	if err == nil {
		for i := range conns {
			if p, _ := conns[i].Config["provider"].(string); p == provider {
				applyAIConnection(deps, ctx, &conns[i])
				return
			}
		}
	}
	deps.AIManager.UnregisterProvider(provider)
	log.Printf("AI provider %q unregistered (connection deleted)", provider)
}

func testConnection(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Connection == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "connection repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection ID"})
			return
		}
		tenantID := auth.GetTenantID(c)
		conn, err := deps.Repos.Connection.GetDecrypted(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
			return
		}

		// Test connection based on type with real validation
		status := "connected"
		message := "Connection test passed"

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		switch conn.Type {
		case repository.ConnectionKubernetes:
			kubeconfig, ok := conn.Config["kubeconfig"].(string)
			if ok && kubeconfig != "" {
				result := deps.Services.Connection.TestKubernetesConnection(ctx, kubeconfig, conn.Config)
				status, message = result.Status, result.Message
			} else if server, srvOk := conn.Config["server"].(string); srvOk && server != "" {
				// Minimal config: just a server URL (e.g. k3s)
				result := deps.Services.Connection.TestKubernetesServerConnection(ctx, server, conn.Config)
				status, message = result.Status, result.Message
			} else {
				status = "error"
				message = "No kubeconfig or server URL provided"
			}
		case repository.ConnectionGit, repository.ConnectionGitLab:
			url, urlOk := conn.Config["url"].(string)
			if !urlOk || url == "" {
				status = "error"
				message = "No URL configured"
			} else {
				token, tokenOk := conn.Config["token"].(string)
				username, _ := conn.Config["username"].(string)
				password, _ := conn.Config["password"].(string)
				provider, _ := conn.Config["provider"].(string)
				if tokenOk && token != "" {
					result := deps.Services.Connection.TestGitConnection(ctx, url, token, provider)
					status, message = result.Status, result.Message
				} else if username != "" && password != "" {
					result := deps.Services.Connection.TestGitBasicAuthConnection(ctx, url, username, password, provider)
					status, message = result.Status, result.Message
				} else {
					status = "error"
					message = "No token or username/password configured"
				}
			}
		case repository.ConnectionJira:
			url, urlOk := conn.Config["url"].(string)
			token, tokenOk := conn.Config["token"].(string)
			if !urlOk || url == "" {
				status = "error"
				message = "No URL configured"
			} else if !tokenOk || token == "" {
				status = "error"
				message = "No token configured"
			} else {
				result := deps.Services.Connection.TestJiraConnection(ctx, url, token)
				status, message = result.Status, result.Message
			}
		case repository.ConnectionAI:
			provider, ok := conn.Config["provider"].(string)
			if !ok || provider == "" {
				status = "error"
				message = "No provider configured"
			} else {
				result := deps.Services.Connection.TestAIConnection(ctx, conn.Config)
				status, message = result.Status, result.Message
			}
		case repository.ConnectionStorage:
			endpoint, ok := conn.Config["endpoint"].(string)
			if !ok || endpoint == "" {
				status = "error"
				message = "No endpoint configured"
			} else {
				result := deps.Services.Connection.TestStorageConnection(ctx, endpoint)
				status, message = result.Status, result.Message
			}
		case repository.ConnectionCI:
			url, urlOk := conn.Config["url"].(string)
			if !urlOk || url == "" {
				status = "error"
				message = "No URL configured"
			} else {
				result := deps.Services.Connection.TestCIConnection(ctx, url, conn.Config)
				status, message = result.Status, result.Message
			}
		case repository.ConnectionProxmox:
			status, message = testProxmoxConnection(deps, c, conn.Config)
		case repository.ConnectionDocker:
			host, _ := conn.Config["host"].(string)
			result := deps.Services.Connection.TestDockerConnection(ctx, host)
			status, message = result.Status, result.Message
		case repository.ConnectionSecret:
			address, _ := conn.Config["address"].(string)
			token, _ := conn.Config["token"].(string)
			result := deps.Services.Connection.TestVaultConnection(ctx, address, token)
			status, message = result.Status, result.Message
		case repository.ConnectionNotification:
			result := deps.Services.Connection.TestNotificationConnection(ctx, conn.Config)
			status, message = result.Status, result.Message
		default:
			status = "disconnected"
			message = "Unknown connection type"
		}

		// Update status in DB
		now := time.Now()
		conn.LastCheckAt = &now
		conn.Status = status
		deps.Repos.Connection.Update(c.Request.Context(), conn)

		// Sync cluster from connection for kubernetes type
		if conn.Type == repository.ConnectionKubernetes && status == "connected" {
			syncClusterFromConnection(deps, c, conn)
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  status,
			"message": message,
			"type":    conn.Type,
			"name":    conn.Name,
		})
	}
}

func connectionSummary(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Connection == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "connection repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)
		counts, err := deps.Repos.Connection.CountByType(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if counts == nil {
			counts = map[string]int{}
		}

		// Define expected types
		types := []string{"kubernetes", "gitlab", "git", "jira", "ci", "ai", "storage", "notification"}
		summary := make([]gin.H, 0, len(types))
		for _, t := range types {
			summary = append(summary, gin.H{
				"type":  t,
				"count": counts[t],
			})
		}

		c.JSON(http.StatusOK, gin.H{"summary": summary})
	}
}

// parseKubeconfig parses a kubeconfig file and extracts all clusters
func parseKubeconfig(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Kubeconfig string `json:"kubeconfig" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Parse kubeconfig
		config, err := clientcmd.Load([]byte(req.Kubeconfig))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Failed to parse kubeconfig: %v", err)})
			return
		}

		// Extract all clusters
		type ParsedCluster struct {
			Name       string `json:"name"`
			Server     string `json:"server"`
			Kubeconfig string `json:"kubeconfig"`
		}
		var clusters []ParsedCluster

		for name, cluster := range config.Clusters {
			// Find associated context and auth info
			var contextName string
			var authInfoName string
			for ctxName, ctx := range config.Contexts {
				if ctx.Cluster == name {
					contextName = ctxName
					authInfoName = ctx.AuthInfo
					break
				}
			}

			// Extract single-cluster kubeconfig
			singleConfig := clientcmdapi.NewConfig()
			singleConfig.Clusters[name] = cluster
			if contextName != "" {
				singleConfig.Contexts[contextName] = config.Contexts[contextName]
				singleConfig.CurrentContext = contextName
			}
			if authInfoName != "" && config.AuthInfos[authInfoName] != nil {
				singleConfig.AuthInfos[authInfoName] = config.AuthInfos[authInfoName]
			}

			// Convert to YAML
			yamlData, err := clientcmd.Write(*singleConfig)
			if err != nil {
				continue // Skip this cluster if we can't serialize it
			}

			clusters = append(clusters, ParsedCluster{
				Name:       name,
				Server:     cluster.Server,
				Kubeconfig: string(yamlData),
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"clusters": clusters,
			"count":    len(clusters),
		})
	}
}

// testKubernetesConnection attempts to parse and validate a kubeconfig using client-go
func testKubernetesConnection(ctx context.Context, kubeconfig string, connConfig map[string]any) (string, string) {
	// Detect if kubeconfig is still encrypted (decryption failed)
	if strings.HasPrefix(kubeconfig, "enc:") {
		return "error", "Kubeconfig is still encrypted — decryption failed. The encryption key (ENCRYPTION_KEY or AUTH_JWT_SECRET) may have changed since this connection was created. Please re-enter the kubeconfig in the connection settings."
	}

	// Parse kubeconfig using client-go
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return "error", fmt.Sprintf("Invalid kubeconfig: %v", err)
	}

	// Set timeout
	config.Timeout = 10 * time.Second

	// Support insecure TLS (self-signed certs, SAN mismatch)
	if insecure, _ := connConfig["insecure"].(string); insecure == "true" || insecure == "1" {
		config.Insecure = true
		config.CAFile = ""
		config.CAData = nil
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "error", fmt.Sprintf("Failed to create K8s client: %v", err)
	}

	// Test 1: Check API Server connectivity and get version
	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return "error", fmt.Sprintf("Cannot reach API Server: %v. Check if cluster is accessible from PEPA network.", err)
	}

	// Test 2: Check node status
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "error", fmt.Sprintf("Cannot list nodes: %v", err)
	}

	readyNodes := 0
	notReadyNodes := 0
	for _, node := range nodes.Items {
		isReady := false
		for _, condition := range node.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "True" {
				isReady = true
				break
			}
		}
		if isReady {
			readyNodes++
		} else {
			notReadyNodes++
		}
	}

	// Test 3: Check system pods in kube-system namespace
	systemPods, err := clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	if err != nil {
		return "error", fmt.Sprintf("Cannot list system pods: %v", err)
	}

	runningPods := 0
	for _, pod := range systemPods.Items {
		if pod.Status.Phase == "Running" {
			runningPods++
		}
	}

	// Test 4: Check recent events
	events, err := clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		Limit: 10,
	})
	if err != nil {
		return "error", fmt.Sprintf("Cannot list events: %v", err)
	}

	// Build comprehensive status message
	status := "connected"
	message := fmt.Sprintf(
		"Connected successfully. K8s %s. Nodes: %d Ready, %d NotReady. System pods: %d/%d running. Recent events: %d.",
		version.String(),
		readyNodes,
		notReadyNodes,
		runningPods,
		len(systemPods.Items),
		len(events.Items),
	)

	// If any nodes are NotReady, warn but still mark as connected
	if notReadyNodes > 0 {
		message += fmt.Sprintf(" WARNING: %d nodes are NotReady - check CNI plugin.", notReadyNodes)
	}

	return status, message
}

// testGitConnection attempts to connect to a generic git provider API.
// TLS verification is controlled by the skipTLSVerify parameter.
func testGitConnection(ctx context.Context, rawURL, token, provider string) (string, string) {
	url := strings.TrimRight(rawURL, "/")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}

	switch provider {
	case "github":
		return testGitHubConnection(ctx, client, url, token)
	case "gitea":
		return testGiteaConnection(ctx, client, url, token)
	case "bitbucket":
		return testBitbucketConnection(ctx, client, url, token)
	case "local":
		return testLocalGitConnection(url)
	case "gitlab":
		return testGitLabConnection(ctx, url, token)
	default:
		// Generic git: try common API endpoints
		return testGenericGitConnection(ctx, client, url, token)
	}
}

// testGitHubConnection tests connectivity to GitHub or GitHub Enterprise
func testGitHubConnection(ctx context.Context, client *http.Client, rawURL, token string) (string, string) {
	apiURL := rawURL
	if !strings.Contains(rawURL, "github.com") && !strings.HasSuffix(rawURL, "/api/v3") {
		apiURL = rawURL + "/api/v3"
	} else if strings.HasSuffix(rawURL, "github.com") || strings.Contains(rawURL, "github.com") {
		apiURL = "https://api.github.com"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"/user", nil)
	if err != nil {
		return "error", fmt.Sprintf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "x509") {
			return "error", fmt.Sprintf("TLS certificate error: %v. Check that the GitHub server certificate is trusted.", err)
		}
		if strings.Contains(err.Error(), "no such host") {
			return "error", fmt.Sprintf("Cannot resolve GitHub host: %v", err)
		}
		return "error", fmt.Sprintf("Cannot reach GitHub: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return "error", "Invalid token - authentication failed. Verify the token is a valid GitHub Personal Access Token."
	}
	if resp.StatusCode == 403 {
		return "error", "Token is valid but access is forbidden. Check token scopes."
	}
	if resp.StatusCode != 200 {
		return "error", fmt.Sprintf("GitHub returned status %d", resp.StatusCode)
	}

	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err == nil {
		username, _ := userInfo["login"].(string)
		return "connected", fmt.Sprintf("Authenticated as %s. GitHub token valid.", username)
	}
	return "connected", "Successfully authenticated with GitHub"
}

// testGiteaConnection tests connectivity to a Gitea instance
func testGiteaConnection(ctx context.Context, client *http.Client, rawURL, token string) (string, string) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL+"/api/v1/user", nil)
	if err != nil {
		return "error", fmt.Sprintf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "x509") {
			return "error", fmt.Sprintf("TLS certificate error: %v. Check that the Gitea server certificate is trusted.", err)
		}
		return "error", fmt.Sprintf("Cannot reach Gitea: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return "error", "Invalid token - authentication failed. Verify the Gitea API token."
	}
	if resp.StatusCode != 200 {
		return "error", fmt.Sprintf("Gitea returned status %d", resp.StatusCode)
	}

	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err == nil {
		username, _ := userInfo["login"].(string)
		return "connected", fmt.Sprintf("Authenticated as %s. Gitea token valid.", username)
	}
	return "connected", "Successfully authenticated with Gitea"
}

// testBitbucketConnection tests connectivity to Bitbucket Cloud or Server
func testBitbucketConnection(ctx context.Context, client *http.Client, rawURL, token string) (string, string) {
	// Try Bitbucket Cloud API first
	apiURL := rawURL
	if strings.Contains(rawURL, "bitbucket.org") {
		apiURL = "https://api.bitbucket.org/2.0"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"/user", nil)
	if err != nil {
		return "error", fmt.Sprintf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("Cannot reach Bitbucket: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return "error", "Invalid token - authentication failed. Verify the Bitbucket App Password or OAuth token."
	}
	if resp.StatusCode != 200 {
		return "error", fmt.Sprintf("Bitbucket returned status %d", resp.StatusCode)
	}

	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err == nil {
		username, _ := userInfo["username"].(string)
		if username == "" {
			username, _ = userInfo["display_name"].(string)
		}
		return "connected", fmt.Sprintf("Authenticated as %s. Bitbucket token valid.", username)
	}
	return "connected", "Successfully authenticated with Bitbucket"
}

// testLocalGitConnection tests if a local git repository path exists
func testLocalGitConnection(path string) (string, string) {
	if path == "" {
		return "error", "No repository path provided"
	}
	// Check if path looks like a git URL (not a local path)
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "git@") {
		return "error", "Local git provider requires a filesystem path, not a URL"
	}
	// For local git, we just validate the path is provided
	return "connected", fmt.Sprintf("Local git repository path configured: %s", path)
}

// testGenericGitConnection tries common git API patterns
func testGenericGitConnection(ctx context.Context, client *http.Client, rawURL, token string) (string, string) {
	// Try to reach the URL with the token
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return "error", fmt.Sprintf("Invalid URL: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("Cannot reach git server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return "error", "Invalid token - authentication failed"
	}
	// Any response means endpoint is reachable
	return "connected", fmt.Sprintf("Git server is reachable (status %d). Connection configured.", resp.StatusCode)
}

// testGitLabConnection attempts to connect to GitLab API.
// TLS verification is disabled for self-hosted instances that may use self-signed
// certificates. For production, consider providing a CA bundle instead.
func testGitLabConnection(ctx context.Context, rawURL, token string) (string, string) {
	// Normalize URL: trim trailing slash to avoid double-slash in API paths
	url := strings.TrimRight(rawURL, "/")

	// NOTE: InsecureSkipVerify is intentionally enabled here because many
	// self-hosted GitLab instances use self-signed certificates or internal CAs.
	// This is a known trade-off between usability and security.
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	// Test user authentication
	req, err := http.NewRequestWithContext(ctx, "GET", url+"/api/v4/user", nil)
	if err != nil {
		return "error", fmt.Sprintf("Failed to create request: %v", err)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := client.Do(req)
	if err != nil {
		// Provide specific hints for common TLS/network errors
		if strings.Contains(err.Error(), "x509") {
			return "error", fmt.Sprintf("TLS certificate error: %v. Check that the GitLab server certificate is trusted.", err)
		}
		if strings.Contains(err.Error(), "no such host") {
			return "error", fmt.Sprintf("Cannot resolve GitLab host: %v", err)
		}
		return "error", fmt.Sprintf("Cannot reach GitLab: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return "error", "Invalid token - authentication failed. Verify the token is a valid Personal Access Token with 'api' scope."
	}
	if resp.StatusCode == 403 {
		return "error", "Token is valid but access is forbidden. Check token scopes and GitLab permissions."
	}
	if resp.StatusCode != 200 {
		return "error", fmt.Sprintf("GitLab returned status %d", resp.StatusCode)
	}

	// Parse user info
	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err == nil {
		username, _ := userInfo["username"].(string)
		name, _ := userInfo["name"].(string)

		// Test project access
		projReq, _ := http.NewRequestWithContext(ctx, "GET", url+"/api/v4/projects?membership=true&per_page=1", nil)
		projReq.Header.Set("PRIVATE-TOKEN", token)
		if projResp, err := client.Do(projReq); err == nil {
			defer projResp.Body.Close()
			if projResp.StatusCode == 200 {
				return "connected", fmt.Sprintf("Authenticated as %s (%s). Token has project access.", username, name)
			}
		}

		return "connected", fmt.Sprintf("Authenticated as %s (%s). Token valid.", username, name)
	}

	return "connected", "Successfully authenticated with GitLab"
}

// testJiraConnection attempts to connect to Jira API
func testJiraConnection(ctx context.Context, url, token string) (string, string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url+"/rest/api/2/myself", nil)
	if err != nil {
		return "error", fmt.Sprintf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("Cannot reach Jira: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return "error", "Invalid token"
	}
	if resp.StatusCode == 200 {
		return "connected", "Successfully authenticated with Jira"
	}
	return "error", fmt.Sprintf("Jira returned status %d", resp.StatusCode)
}

// testAIConnection validates AI provider configuration
func testAIConnection(ctx context.Context, config map[string]any) (string, string) {
	provider, _ := config["provider"].(string)
	apiKey, _ := config["api_key"].(string)

	switch provider {
	case "openai":
		if apiKey == "" {
			return "error", "API key required for OpenAI"
		}
		return "connected", "OpenAI configuration valid"
	case "ollama":
		baseURL, _ := config["base_url"].(string)
		if baseURL == "" {
			return "error", "Base URL required for Ollama"
		}
		// Try to reach Ollama
		client := &http.Client{Timeout: 5 * time.Second}
		req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/tags", nil)
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, err := client.Do(req)
		if err != nil {
			return "error", fmt.Sprintf("Cannot reach Ollama: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			return "connected", "Successfully connected to Ollama"
		}
		return "error", fmt.Sprintf("Ollama returned status %d", resp.StatusCode)
	case "anthropic":
		if apiKey == "" {
			return "error", "API key required for Anthropic"
		}
		return "connected", "Anthropic configuration valid"
	case "groq":
		if apiKey == "" {
			return "error", "API key required for Groq"
		}
		return "connected", "Groq configuration valid"
	case "qoder":
		if apiKey == "" {
			return "error", "API key required for Qoder"
		}
		baseURL, _ := config["base_url"].(string)
		if baseURL == "" {
			baseURL = "https://api.qoder.com/v1"
		}
		client := &http.Client{Timeout: 10 * time.Second}
		req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := client.Do(req)
		if err != nil {
			return "error", fmt.Sprintf("Cannot reach Qoder: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			return "connected", "Successfully connected to Qoder"
		} else if resp.StatusCode == 401 {
			return "error", "Invalid API key"
		}
		return "error", fmt.Sprintf("Qoder returned status %d", resp.StatusCode)
	case "lmstudio":
		baseURL, _ := config["base_url"].(string)
		if baseURL == "" {
			baseURL, _ = config["url"].(string)
		}
		if baseURL == "" {
			baseURL = "http://host.docker.internal:1234/v1"
		}
		client := &http.Client{Timeout: 5 * time.Second}
		req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, err := client.Do(req)
		if err != nil {
			return "error", fmt.Sprintf("Cannot reach LM Studio: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			return "connected", "Successfully connected to LM Studio"
		}
		return "error", fmt.Sprintf("LM Studio returned status %d", resp.StatusCode)
	default:
		return "error", fmt.Sprintf("Unknown provider: %s", provider)
	}
}

// testStorageConnection attempts to reach storage endpoint
func testStorageConnection(ctx context.Context, endpoint string) (string, string) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}

	req, err := http.NewRequestWithContext(ctx, "HEAD", endpoint, nil)
	if err != nil {
		return "error", fmt.Sprintf("Invalid endpoint: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("Cannot reach storage: %v", err)
	}
	defer resp.Body.Close()

	// Any response means endpoint is reachable
	return "connected", "Storage endpoint is reachable"
}

// testCIConnection attempts to reach CI/CD endpoint
func testCIConnection(ctx context.Context, url string, config map[string]any) (string, string) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "error", fmt.Sprintf("Invalid URL: %v", err)
	}

	// Add token if provided
	if token, ok := config["token"].(string); ok && token != "" {
		ciType, _ := config["ci_type"].(string)
		switch ciType {
		case "gitlab_ci":
			req.Header.Set("PRIVATE-TOKEN", token)
		case "github_actions":
			req.Header.Set("Authorization", "token "+token)
		case "jenkins":
			req.SetBasicAuth("", token)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("Cannot reach CI: %v", err)
	}
	defer resp.Body.Close()

	// Any response means endpoint is reachable
	return "connected", "CI endpoint is reachable"
}

// syncClusterFromConnection creates or updates a cluster entry when a kubernetes connection is created/updated.
func syncClusterFromConnection(deps Dependencies, c *gin.Context, conn *repository.Connection) {
	if deps.Repos.Cluster == nil {
		return
	}
	if conn.Type != repository.ConnectionKubernetes {
		return
	}

	ctx := c.Request.Context()

	// Extract API server URL from connection config
	apiServer := ""
	if server, ok := conn.Config["api_server"].(string); ok {
		apiServer = server
	} else if server, ok := conn.Config["server"].(string); ok {
		apiServer = server
	} else if kubeconfig, ok := conn.Config["kubeconfig"].(string); ok && kubeconfig != "" {
		// Try to parse kubeconfig to extract server URL
		if cfg, err := clientcmd.Load([]byte(kubeconfig)); err == nil {
			for _, cluster := range cfg.Clusters {
				if cluster.Server != "" {
					apiServer = cluster.Server
					break
				}
			}
		}
	}

	// Determine cluster status from connection status
	clusterStatus := "disconnected"
	isActive := false
	if conn.Status == "connected" {
		clusterStatus = "connected"
		isActive = true
	}

	// Check if a cluster already exists for this connection
	existing, err := deps.Repos.Cluster.FindByConnectionID(ctx, conn.ID)
	if err != nil {
		log.Printf("Warning: failed to find cluster by connection ID %s: %v", conn.ID, err)
		return
	}

	if existing != nil {
		// Update existing cluster
		existing.Name = conn.Name
		existing.Description = conn.Description
		existing.APIServerURL = apiServer
		existing.Status = clusterStatus
		existing.IsActive = isActive
		if conn.Labels != nil {
			existing.Labels = conn.Labels
		}
		existing.Notes = conn.Notes
		// Set default node_count if not set
		if existing.NodeCount <= 0 {
			existing.NodeCount = 3
		}
		if err := deps.Repos.Cluster.Update(ctx, existing); err != nil {
			log.Printf("Warning: failed to update cluster %s from connection: %v", existing.ID, err)
		}

		// Transfer kubeconfig from connection to cluster and detect GitOps engines
		if kubeconfig, ok := conn.Config["kubeconfig"].(string); ok && kubeconfig != "" {
			if err := deps.Repos.Cluster.SaveKubeconfig(ctx, existing.ID, kubeconfig); err != nil {
				log.Printf("Warning: failed to save kubeconfig to cluster %s: %v", existing.ID, err)
			}
			// Detect GitOps engines from kubeconfig
			if client, err := k8s.NewClient(kubeconfig); err == nil {
				detectCtx, detectCancel := context.WithTimeout(context.Background(), 5*time.Second)
				if engine, err := client.DetectGitOpsEngine(detectCtx); err == nil {
					existing.FluxInstalled = engine.FluxCD
					if engine.ArgoCD {
						if existing.Labels == nil {
							existing.Labels = make(map[string]string)
						}
						existing.Labels["argocd_detected"] = "true"
					}
					_ = deps.Repos.Cluster.Update(context.Background(), existing)
				}
				detectCancel()
			}
		}
	} else {
		// Create new cluster linked to this connection
		cluster := &repository.Cluster{
			TenantID:     conn.TenantID,
			Name:         conn.Name,
			Description:  conn.Description,
			Environment:  "dev",
			APIServerURL: apiServer,
			Status:       clusterStatus,
			IsActive:     isActive,
			Labels:       conn.Labels,
			Notes:        conn.Notes,
			ConnectionID: &conn.ID,
			NodeCount:    3,
		}
		if cluster.Labels == nil {
			cluster.Labels = map[string]string{}
		}
		// Check if environment is in labels or config
		if env, ok := conn.Config["environment"].(string); ok && env != "" {
			cluster.Environment = env
		}
		if err := deps.Repos.Cluster.Create(ctx, cluster); err != nil {
			log.Printf("Warning: failed to create cluster from connection %s: %v", conn.ID, err)
			return
		}

		// Transfer kubeconfig from connection to cluster and detect GitOps engines
		if kubeconfig, ok := conn.Config["kubeconfig"].(string); ok && kubeconfig != "" {
			if err := deps.Repos.Cluster.SaveKubeconfig(ctx, cluster.ID, kubeconfig); err != nil {
				log.Printf("Warning: failed to save kubeconfig to cluster %s: %v", cluster.ID, err)
			}
			// Detect GitOps engines from kubeconfig
			if client, err := k8s.NewClient(kubeconfig); err == nil {
				detectCtx, detectCancel := context.WithTimeout(context.Background(), 5*time.Second)
				if engine, err := client.DetectGitOpsEngine(detectCtx); err == nil {
					cluster.FluxInstalled = engine.FluxCD
					if engine.ArgoCD {
						if cluster.Labels == nil {
							cluster.Labels = make(map[string]string)
						}
						cluster.Labels["argocd_detected"] = "true"
					}
					_ = deps.Repos.Cluster.Update(context.Background(), cluster)
				}
				detectCancel()
			}
		}
	}
}

// browseConnection lists available resources for a connection using the associated plugin.
func browseConnection(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		conn, err := deps.Repos.Connection.GetDecrypted(c.Request.Context(), id, tenantID)
		if err != nil {
			log.Printf("browseConnection: failed to load credentials for connection %s: %v", id, err)
			c.JSON(http.StatusNotFound, gin.H{"error": "failed to load connection credentials"})
			return
		}

		// Map connection type to plugin name
		pluginName := ""
		switch conn.Type {
		case "gitlab":
			pluginName = "gitlab"
		case "git":
			// Route to the appropriate plugin based on the git provider
			switch provider, _ := conn.Config["provider"].(string); provider {
			case "gitlab":
				pluginName = "gitlab"
			case "github":
				pluginName = "github"
			case "gitea":
				pluginName = "gitea"
			case "bitbucket":
				pluginName = "bitbucket"
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "browse not supported for this git provider. Supported: gitlab, github, gitea, bitbucket"})
				return
			}
		case "jira":
			pluginName = "jira"
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("browse not supported for type: %s", conn.Type)})
			return
		}

		if deps.ProviderRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provider registry not available"})
			return
		}

		provider, ok := deps.ProviderRegistry.GetEnabled(pluginName)
		if !ok || provider == nil {
			if entry, exists := deps.ProviderRegistry.Get(pluginName); exists && !entry.Enabled {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("plugin %q is disabled. Enable it in Plugins page to browse this connection.", pluginName)})
			} else {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("plugin %q is not installed. Install it from Marketplace to browse this connection.", pluginName)})
			}
			return
		}

		// Build connection config from connection's config
		connConfig := make(map[string]string)
		for k, v := range conn.Config {
			if s, ok := v.(string); ok {
				connConfig[k] = s
			}
		}

		// Try user's personal credential first (so browsing uses their identity)
		userID := auth.GetUserID(c)
		if userID != nil && deps.DB != nil {
			providerURL := connConfig["url"]
			if providerURL == "" {
				// Try to extract from repo_url or other fields
				providerURL = connConfig["repo_url"]
			}
			if providerURL != "" {
				provName := pluginName
				if provName == "git" {
					provName = connConfig["provider"]
				}
				if provName == "" {
					provName = "gitlab" // default
				}
				token, username, email, err := GetUserCredential(c.Request.Context(), deps, *userID, provName, providerURL)
				if err == nil && token != "" {
					connConfig["token"] = token
					if username != "" {
						connConfig["username"] = username
					}
					if email != "" {
						connConfig["email"] = email
					}
					log.Printf("[browse] using personal credential for user %s on %s", userID, providerURL)
				}
			}
		}

		resource := c.DefaultQuery("resource", "list_repos")

		// Build params from query parameters for hierarchical browsing
		browseParams := make(map[string]string)
		if v := c.Query("group_id"); v != "" {
			browseParams["group_id"] = v
		}
		if v := c.Query("parent_id"); v != "" {
			browseParams["parent_id"] = v
		}
		if v := c.Query("repo_id"); v != "" {
			browseParams["repo_id"] = v
		}
		paramsBytes, _ := json.Marshal(browseParams)
		params := json.RawMessage(paramsBytes)

		// Execute the action via the provider
		resp, err := provider.Executor.Execute(c.Request.Context(), resource, params, "", connConfig)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if !resp.GetSuccess() {
			c.JSON(http.StatusInternalServerError, gin.H{"error": resp.GetError()})
			return
		}

		// Parse and return the output
		var result interface{}
		if err := json.Unmarshal(resp.GetOutput(), &result); err != nil {
			c.JSON(http.StatusOK, gin.H{"raw": string(resp.GetOutput())})
			return
		}
		c.JSON(http.StatusOK, gin.H{"resource": resource, "data": result})
	}
}

// executeConnectionAction executes a plugin action on a connection using a JSON body.
// This allows passing complex parameters (e.g. variables map) unlike the GET browse endpoint.
func executeConnectionAction(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection id"})
			return
		}

		var req struct {
			Resource string                 `json:"resource"`
			Params   map[string]interface{} `json:"params,omitempty"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		if req.Resource == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "resource is required"})
			return
		}

		tenantID := auth.GetTenantID(c)
		conn, err := deps.Repos.Connection.GetDecrypted(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "connection not found or credentials unavailable"})
			return
		}

		// Map connection type to plugin name
		pluginName := ""
		switch conn.Type {
		case "gitlab":
			pluginName = "gitlab"
		case "git":
			switch provider, _ := conn.Config["provider"].(string); provider {
			case "gitlab":
				pluginName = "gitlab"
			case "github":
				pluginName = "github"
			case "gitea":
				pluginName = "gitea"
			case "bitbucket":
				pluginName = "bitbucket"
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported git provider"})
				return
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("execute not supported for type: %s", conn.Type)})
			return
		}

		if deps.ProviderRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provider registry not available"})
			return
		}

		provider, ok := deps.ProviderRegistry.GetEnabled(pluginName)
		if !ok || provider == nil {
			if entry, exists := deps.ProviderRegistry.Get(pluginName); exists && !entry.Enabled {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("plugin %q is disabled. Enable it in Plugins page to use this connection.", pluginName)})
			} else {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("plugin %q is not installed. Install it from Marketplace to use this connection.", pluginName)})
			}
			return
		}

		// Build connection config
		connConfig := make(map[string]string)
		for k, v := range conn.Config {
			if s, ok := v.(string); ok {
				connConfig[k] = s
			}
		}

		// Try user's personal credential first (so actions use their identity)
		userID := auth.GetUserID(c)
		if userID != nil && deps.DB != nil {
			providerURL := connConfig["url"]
			if providerURL == "" {
				providerURL = connConfig["repo_url"]
			}
			if providerURL != "" {
				provName := pluginName
				if provName == "git" {
					provName = connConfig["provider"]
				}
				if provName == "" {
					provName = "gitlab" // default
				}
				token, username, email, err := GetUserCredential(c.Request.Context(), deps, *userID, provName, providerURL)
				if err == nil && token != "" {
					connConfig["token"] = token
					if username != "" {
						connConfig["username"] = username
					}
					if email != "" {
						connConfig["email"] = email
					}
					log.Printf("[execute] using personal credential for user %s on %s", userID, providerURL)
				}
			}
		}

		// Marshal params to JSON for the plugin
		paramsBytes, _ := json.Marshal(req.Params)
		params := json.RawMessage(paramsBytes)

		resp, err := provider.Executor.Execute(c.Request.Context(), req.Resource, params, "", connConfig)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if !resp.GetSuccess() {
			c.JSON(http.StatusInternalServerError, gin.H{"error": resp.GetError()})
			return
		}

		var result interface{}
		if err := json.Unmarshal(resp.GetOutput(), &result); err != nil {
			c.JSON(http.StatusOK, gin.H{"raw": string(resp.GetOutput())})
			return
		}
		c.JSON(http.StatusOK, gin.H{"resource": req.Resource, "data": result})
	}
}

// isSensitiveConfigKey returns true for config keys that contain secrets.
func isSensitiveConfigKey(key string) bool {
	sensitive := []string{"token", "password", "kubeconfig", "api_token", "secret", "ssh_key"}
	lower := strings.ToLower(key)
	for _, s := range sensitive {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// testKubernetesServerConnection tests connectivity to a Kubernetes API server using just a server URL.
func testKubernetesServerConnection(ctx context.Context, server string, config map[string]any) (string, string) {
	url := strings.TrimRight(server, "/")
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", url+"/version", nil)
	// Add Bearer token if provided
	if token, ok := config["token"].(string); ok && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("Cannot reach API Server: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return "connected", "Successfully connected to Kubernetes API server"
	}
	return "error", fmt.Sprintf("API Server returned status %d", resp.StatusCode)
}

// testGitBasicAuthConnection tests git connectivity using username/password basic auth.
func testGitBasicAuthConnection(ctx context.Context, rawURL, username, password, provider string) (string, string) {
	url := strings.TrimRight(rawURL, "/")
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	switch provider {
	case "gitea":
		req, _ := http.NewRequestWithContext(ctx, "GET", url+"/api/v1/user", nil)
		req.SetBasicAuth(username, password)
		resp, err := client.Do(req)
		if err != nil {
			return "error", fmt.Sprintf("Cannot reach Gitea: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var info map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&info); err == nil {
				login, _ := info["login"].(string)
				return "connected", fmt.Sprintf("Authenticated as %s. Gitea credentials valid.", login)
			}
			return "connected", "Successfully authenticated with Gitea"
		}
		return "error", fmt.Sprintf("Gitea returned status %d — check credentials", resp.StatusCode)

	case "gitlab":
		req, _ := http.NewRequestWithContext(ctx, "GET", url+"/api/v4/user", nil)
		req.SetBasicAuth(username, password)
		resp, err := client.Do(req)
		if err != nil {
			return "error", fmt.Sprintf("Cannot reach GitLab: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var info map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&info); err == nil {
				login, _ := info["username"].(string)
				return "connected", fmt.Sprintf("Authenticated as %s. GitLab credentials valid.", login)
			}
			return "connected", "Successfully authenticated with GitLab"
		}
		return "error", fmt.Sprintf("GitLab returned status %d — check credentials", resp.StatusCode)

	default:
		// Generic git: just try to reach the server
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		resp, err := client.Do(req)
		if err != nil {
			return "error", fmt.Sprintf("Cannot reach git server: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 500 {
			return "connected", fmt.Sprintf("Git server reachable (status %d)", resp.StatusCode)
		}
		return "error", fmt.Sprintf("Git server returned status %d", resp.StatusCode)
	}
}

// testDockerConnection tests connectivity to a Docker daemon.
func testDockerConnection(ctx context.Context, host string) (string, string) {
	if host == "" || host == "unix:///var/run/docker.sock" {
		// Inside a container, we can't reach the host Docker socket directly.
		// Just validate the configuration is present.
		return "connected", "Docker socket configured (local connection)"
	}
	// TCP-based Docker host
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, "GET", host+"/_ping", nil)
	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("Cannot reach Docker: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return "connected", "Successfully connected to Docker daemon"
	}
	return "error", fmt.Sprintf("Docker returned status %d", resp.StatusCode)
}

// testVaultConnection tests connectivity to a HashiCorp Vault server.
func testVaultConnection(ctx context.Context, address, token string) (string, string) {
	if address == "" {
		return "error", "No Vault address configured"
	}
	url := strings.TrimRight(address, "/")
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", url+"/v1/sys/health", nil)
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("Cannot reach Vault: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 || resp.StatusCode == 429 {
		return "connected", "Successfully connected to Vault"
	}
	if resp.StatusCode == 403 {
		return "error", "Vault token is invalid or expired"
	}
	return "error", fmt.Sprintf("Vault returned status %d", resp.StatusCode)
}

// testNotificationConnection validates a notification service connection (Slack, Telegram, Teams).
func testNotificationConnection(ctx context.Context, config map[string]any) (string, string) {
	provider, _ := config["provider"].(string)
	if provider == "" {
		return "error", "No notification provider configured"
	}

	switch provider {
	case "slack":
		webhookURL, _ := config["webhook_url"].(string)
		botToken, _ := config["bot_token"].(string)
		if webhookURL == "" && botToken == "" {
			return "error", "Either webhook_url or bot_token is required for Slack"
		}
		if webhookURL != "" {
			if err := validateSlackWebhookURL(webhookURL); err != nil {
				return "error", err.Error()
			}
			// Send a test ping to the webhook
			client := &http.Client{Timeout: 5 * time.Second}
			payload := []byte(`{"text":"PEPA connection test"}`)
			req, _ := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				return "error", fmt.Sprintf("Cannot reach Slack webhook: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				return "connected", "Slack webhook is reachable"
			}
			return "error", fmt.Sprintf("Slack webhook returned status %d", resp.StatusCode)
		}
		return "connected", "Slack bot_token configured"

	case "telegram":
		botToken, _ := config["bot_token"].(string)
		chatID, _ := config["chat_id"].(string)
		if botToken == "" {
			return "error", "bot_token is required for Telegram"
		}
		if chatID == "" {
			return "error", "chat_id is required for Telegram"
		}
		// Test by calling getMe
		client := &http.Client{Timeout: 5 * time.Second}
		apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", botToken)
		req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		resp, err := client.Do(req)
		if err != nil {
			return "error", fmt.Sprintf("Cannot reach Telegram API: %v", err)
		}
		defer resp.Body.Close()
		var result struct {
			OK     bool `json:"ok"`
			Result struct {
				Username string `json:"username"`
			} `json:"result"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		if result.OK {
			return "connected", fmt.Sprintf("Telegram bot @%s is reachable", result.Result.Username)
		}
		return "error", "Telegram bot token is invalid"

	case "teams":
		webhookURL, _ := config["webhook_url"].(string)
		if webhookURL == "" {
			return "error", "webhook_url is required for Microsoft Teams"
		}
		if err := validateTeamsWebhookURL(webhookURL); err != nil {
			return "error", err.Error()
		}
		// Send a test card to the webhook
		client := &http.Client{Timeout: 5 * time.Second}
		payload := []byte(`{"@type":"MessageCard","text":"PEPA connection test"}`)
		req, _ := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return "error", fmt.Sprintf("Cannot reach Teams webhook: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 || resp.StatusCode == 202 {
			return "connected", "Teams webhook is reachable"
		}
		return "error", fmt.Sprintf("Teams webhook returned status %d", resp.StatusCode)

	case "email":
		smtpHost, _ := config["smtp_host"].(string)
		smtpPort, _ := config["smtp_port"].(string)
		username, _ := config["username"].(string)
		password, _ := config["password"].(string)
		insecureTLS, _ := config["insecure_tls"].(string)
		if smtpHost == "" {
			return "error", "smtp_host is required for Email"
		}
		if smtpPort == "" {
			smtpPort = "587"
		}
		addr := net.JoinHostPort(smtpHost, smtpPort)
		tlsConfig := &tls.Config{
			ServerName:         smtpHost,
			InsecureSkipVerify: insecureTLS == "true",
		}
		// Try to connect and handshake with the SMTP server
		if smtpPort == "465" {
			conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, tlsConfig)
			if err != nil {
				return "error", fmt.Sprintf("Cannot reach SMTP server (TLS): %v", err)
			}
			conn.Close()
		} else {
			c, err := smtp.Dial(addr)
			if err != nil {
				return "error", fmt.Sprintf("Cannot reach SMTP server: %v", err)
			}
			if ok, _ := c.Extension("STARTTLS"); ok {
				if err := c.StartTLS(tlsConfig); err != nil {
					c.Close()
					return "error", fmt.Sprintf("STARTTLS failed: %v", err)
				}
			}
			// Try auth if credentials provided
			if username != "" && password != "" {
				if err := c.Auth(smtp.PlainAuth("", username, password, smtpHost)); err != nil {
					c.Close()
					return "error", fmt.Sprintf("SMTP auth failed: %v", err)
				}
			}
			c.Close()
		}
		from := smtpHost
		if v, ok := config["from"].(string); ok && v != "" {
			from = v
		} else if username != "" {
			from = username
		}
		return "connected", fmt.Sprintf("SMTP server %s:%s reachable (from: %s)", smtpHost, smtpPort, from)

	case "webhook":
		webhookURL, _ := config["webhook_url"].(string)
		if webhookURL == "" {
			return "error", "webhook_url is required for Webhook"
		}
		u, err := url.Parse(webhookURL)
		if err != nil || u.Host == "" {
			return "error", "Invalid webhook URL"
		}
		if u.Scheme != "https" && u.Scheme != "http" {
			return "error", "Webhook URL must use http or https"
		}
		// Send a test POST to verify the endpoint is reachable
		client := &http.Client{Timeout: 5 * time.Second}
		payload := []byte(`{"source":"pepa","event_type":"connection_test","message":"PEPA webhook connection test"}`)
		req, _ := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "PEPA-Webhook/0.1.0")
		// Apply custom headers from config
		for k, v := range config {
			if strings.HasPrefix(k, "header.") {
				headerName := strings.TrimPrefix(k, "header.")
				if sv, ok := v.(string); ok {
					req.Header.Set(headerName, sv)
				}
			}
		}
		if secret, ok := config["secret"].(string); ok && secret != "" {
			req.Header.Set("X-PEPA-Secret", secret)
		}
		resp, err := client.Do(req)
		if err != nil {
			return "error", fmt.Sprintf("Cannot reach webhook: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return "connected", fmt.Sprintf("Webhook endpoint reachable (status %d)", resp.StatusCode)
		}
		return "error", fmt.Sprintf("Webhook returned status %d", resp.StatusCode)

	default:
		return "error", fmt.Sprintf("Unknown notification provider: %s", provider)
	}
}

// validateSlackWebhookURL ensures the URL points to a legitimate Slack endpoint (SSRF protection).
func validateSlackWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid Slack webhook url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("Slack webhook url must use https, got %q", u.Scheme)
	}
	host := strings.ToLower(u.Host)
	if !strings.HasSuffix(host, ".slack.com") && host != "hooks.slack.com" {
		return fmt.Errorf("Slack webhook url must point to hooks.slack.com, got %q", host)
	}
	return nil
}

// validateTeamsWebhookURL ensures the URL points to a legitimate Teams endpoint (SSRF protection).
func validateTeamsWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid Teams webhook url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("Teams webhook url must use https, got %q", u.Scheme)
	}
	host := strings.ToLower(u.Host)
	if !strings.HasSuffix(host, ".office.com") && !strings.HasSuffix(host, ".office365.com") {
		return fmt.Errorf("Teams webhook url must point to a Microsoft Teams endpoint, got %q", host)
	}
	return nil
}

// resolveGitPluginName maps a connection type and git provider to the plugin name
// that handles browsing/execution for that provider.
func resolveGitPluginName(connType, provider string) string {
	if connType == "gitlab" {
		return "gitlab"
	}
	switch provider {
	case "github":
		return "github"
	case "gitlab":
		return "gitlab"
	case "gitea":
		return "gitea"
	case "bitbucket":
		return "bitbucket"
	}
	return ""
}

// requiredPluginForConnection returns the plugin name that must be installed and
// enabled for a given connection type + config. Returns "" if no plugin is required.
func requiredPluginForConnection(connType string, config map[string]any) string {
	switch connType {
	case "git":
		provider, _ := config["provider"].(string)
		return resolveGitPluginName(connType, provider)
	case "gitlab":
		return "gitlab"
	case "jira":
		return "jira"
	case "proxmox":
		return "proxmox"
	case "notification":
		provider, _ := config["provider"].(string)
		switch provider {
		case "slack":
			return "slack"
		case "telegram":
			return "telegram"
		case "teams":
			return "teams"
		}
		// email and webhook are built-in, no plugin required
	}
	return ""
}

// connectionPluginStatus returns the installation/enabled status of each git
// provider plugin so the frontend can show availability indicators.
func connectionPluginStatus(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		providers := map[string]string{
			// Git providers
			"github":    "github",
			"gitlab":    "gitlab",
			"gitea":     "gitea",
			"bitbucket": "bitbucket",
			// Notification providers
			"email":    "email",
			"webhook":  "webhook",
			"slack":    "slack",
			"telegram": "telegram",
			"teams":    "teams",
			// Other plugin-backed connection types
			"jira":    "jira",
			"proxmox": "proxmox",
		}

		if deps.ProviderRegistry == nil {
			// No registry at all – everything unavailable
			status := make(map[string]map[string]interface{})
			for prov := range providers {
				status[prov] = map[string]interface{}{"installed": false, "enabled": false}
			}
			c.JSON(http.StatusOK, status)
			return
		}

		status := make(map[string]map[string]interface{})
		for prov, plugin := range providers {
			entry, ok := deps.ProviderRegistry.Get(plugin)
			switch {
			case !ok:
				status[prov] = map[string]interface{}{"installed": false, "enabled": false}
			case !entry.Enabled:
				status[prov] = map[string]interface{}{"installed": true, "enabled": false}
			default:
				status[prov] = map[string]interface{}{"installed": true, "enabled": true}
			}
		}
		c.JSON(http.StatusOK, status)
	}
}
