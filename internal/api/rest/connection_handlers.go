package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/k8s"
	"github.com/pepa/pepa/internal/repository"
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
			slog.Warn("cannot resolve vault reference for AI connection ", "name", conn.Name, "error", err)
		}
	}

	if err := deps.AIManager.ConfigureProvider(provider, apiKey, baseURL, model); err != nil {
		slog.Warn("failed to apply AI connection ", "name", conn.Name, "error", err)
		return
	}
	deps.AIManager.SetDefaultProvider(provider)
	slog.Info("AI provider configured from connection", "id", provider, "name", conn.Name)
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
	slog.Info("AI provider unregistered (connection deleted)", "id", provider)
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
		var status, message string

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
		case repository.ConnectionVMware:
			status, message = testVMwareConnection(deps, c, conn.Config)
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
		_ = deps.Repos.Connection.Update(c.Request.Context(), conn)

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
		slog.Warn("failed to find cluster by connection ID ", "id", conn.ID, "error", err)
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
			slog.Warn("failed to update cluster from connection", "id", existing.ID, "error", err)
		}

		// Transfer kubeconfig from connection to cluster and detect GitOps engines
		if kubeconfig, ok := conn.Config["kubeconfig"].(string); ok && kubeconfig != "" {
			if err := deps.Repos.Cluster.SaveKubeconfig(ctx, existing.ID, kubeconfig); err != nil {
				slog.Warn("failed to save kubeconfig to cluster ", "id", existing.ID, "error", err)
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
			slog.Warn("failed to create cluster from connection ", "id", conn.ID, "error", err)
			return
		}

		// Transfer kubeconfig from connection to cluster and detect GitOps engines
		if kubeconfig, ok := conn.Config["kubeconfig"].(string); ok && kubeconfig != "" {
			if err := deps.Repos.Cluster.SaveKubeconfig(ctx, cluster.ID, kubeconfig); err != nil {
				slog.Warn("failed to save kubeconfig to cluster ", "id", cluster.ID, "error", err)
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
			slog.Info("browseConnection: failed to load credentials for connection ", "id", id, "error", err)
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
					slog.Info("using personal credential for user on", "id", userID, "id", providerURL)
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
					slog.Info("using personal credential for user on", "id", userID, "id", providerURL)
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
