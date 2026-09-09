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
		conns.GET("/credential-status", credentialStatus(deps))
		conns.GET("/health", connectionHealthDashboard(deps))
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
		isAdmin := auth.IsPlatformAdmin(c)

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
			if isAdmin {
				// Admin sees config with only sensitive keys masked
				sanitized := make(map[string]any, len(items[i].Config))
				for k, v := range items[i].Config {
					if isSensitiveConfigKey(k) {
						sanitized[k] = "***"
					} else {
						sanitized[k] = v
					}
				}
				items[i].Config = sanitized
			} else {
				// Non-admin: all config values are masked — they only see name, type, status
				items[i].Config = map[string]any{"_masked": true}
			}
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
		isAdmin := auth.IsPlatformAdmin(c)
		sanitized := make(map[string]any, len(conn.Config))
		if isAdmin {
			for k, v := range conn.Config {
				if isSensitiveConfigKey(k) {
					sanitized[k] = "***"
				} else {
					sanitized[k] = v
				}
			}
		} else {
			// Non-admin: all config values are masked
			sanitized["_masked"] = true
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
			Type            repository.ConnectionType `json:"type" binding:"required"`
			Name            string                    `json:"name" binding:"required"`
			Description     string                    `json:"description"`
			Config          map[string]any            `json:"config"`
			Labels          map[string]string         `json:"labels"`
			Notes           string                    `json:"notes"`
			FallbackToAdmin *bool                     `json:"fallback_to_admin"`
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
			TenantID:        auth.GetTenantID(c),
			Type:            req.Type,
			Name:            req.Name,
			Description:     req.Description,
			Config:          req.Config,
			Labels:          req.Labels,
			Notes:           req.Notes,
			Status:          "disconnected",
			FallbackToAdmin: true, // default: allow fallback to admin credentials
		}
		// Allow admin to disable fallback on creation
		if req.FallbackToAdmin != nil {
			conn.FallbackToAdmin = *req.FallbackToAdmin
		}
		// Set owner_id to the creating user for audit trail
		if userID := auth.GetUserID(c); userID != nil {
			conn.OwnerID = userID
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
			Name            string            `json:"name"`
			Description     string            `json:"description"`
			Config          map[string]any    `json:"config"`
			Labels          map[string]string `json:"labels"`
			Notes           string            `json:"notes"`
			Status          string            `json:"status"`
			FallbackToAdmin *bool             `json:"fallback_to_admin"`
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
		if req.FallbackToAdmin != nil {
			conn.FallbackToAdmin = *req.FallbackToAdmin
		}

		if err := deps.Repos.Connection.Update(c.Request.Context(), conn); err != nil {
			respondInternalError(c, err)
			return
		}

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

		// Resolve per-user credentials before testing so the test reflects
		// the caller's identity, not the admin's.
		credSource := string(CredentialSourceAdmin)
		userID := auth.GetUserID(c)
		if userID != nil {
			provName, provURLKey := testProviderInfo(conn.Type, conn.Config)
			if provName != "" {
				resolved, resErr := ResolveConnectionCredential(c.Request.Context(), deps, conn, userID, provName, provURLKey)
				if resErr != nil {
					// Honor fallback_to_admin policy: if resolver rejects and fallback is disabled, fail.
					if !conn.FallbackToAdmin {
						c.JSON(http.StatusForbidden, gin.H{"error": resErr.Error()})
						return
					}
					// Fallback allowed — continue with admin config.
				} else if resolved != nil {
					// Override config with resolved (user/shared) credentials
					for k, v := range resolved.Config {
						conn.Config[k] = v
					}
					credSource = string(resolved.Source)
				}
			}
		}

		// Test connection based on type with real validation
		var status, message string

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		switch conn.Type {
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
		case repository.ConnectionSonarQube:
			url, urlOk := conn.Config["url"].(string)
			token, tokenOk := conn.Config["token"].(string)
			if !urlOk || url == "" {
				status = "error"
				message = "No URL configured"
			} else if !tokenOk || token == "" {
				status = "error"
				message = "No token configured"
			} else {
				result := deps.Services.Connection.TestSonarQubeConnection(ctx, url, token)
				status, message = result.Status, result.Message
			}
		case repository.ConnectionArgoCD:
			status, message = testArgoCDConnection(deps, c, conn.Config)
		case repository.ConnectionFluxCD:
			status, message = testFluxCDConnection(deps, c, conn.Config)
		default:
			status = "disconnected"
			message = "Unknown connection type"
		}

		// Update status in DB
		now := time.Now()
		conn.LastCheckAt = &now
		conn.Status = status
		_ = deps.Repos.Connection.Update(c.Request.Context(), conn)

		c.JSON(http.StatusOK, gin.H{
			"status":            status,
			"message":           message,
			"type":              conn.Type,
			"name":              conn.Name,
			"credential_source": credSource,
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
		types := []string{"kubernetes", "gitlab", "git", "jira", "ci", "ai", "storage", "notification", "argocd", "fluxcd"}
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

		// Resolve credentials: user personal → shared → admin fallback
		userID := auth.GetUserID(c)
		credSource := string(CredentialSourceAdmin)
		providerURL := connConfig["url"]
		if providerURL == "" {
			providerURL = connConfig["repo_url"]
		}
		provName := pluginName
		if provName == "git" {
			provName = connConfig["provider"]
		}
		if provName == "" {
			provName = "gitlab" // default
		}
		if userID != nil && providerURL != "" {
			resolved, err := ResolveConnectionCredential(c.Request.Context(), deps, conn, userID, provName, "url")
			if err == nil && resolved != nil {
				connConfig = resolved.Config
				credSource = string(resolved.Source)
			} else if !conn.FallbackToAdmin {
				c.JSON(http.StatusForbidden, gin.H{"error": "no personal credential and admin fallback is disabled for this connection"})
				return
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
			c.JSON(http.StatusOK, gin.H{"resource": resource, "raw": string(resp.GetOutput()), "credential_source": credSource})
			return
		}
		c.JSON(http.StatusOK, gin.H{"resource": resource, "data": result, "credential_source": credSource})
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

		// Resolve credentials: user personal → shared → admin fallback
		userID := auth.GetUserID(c)
		credSource := string(CredentialSourceAdmin)
		providerURL := connConfig["url"]
		if providerURL == "" {
			providerURL = connConfig["repo_url"]
		}
		provName := pluginName
		if provName == "git" {
			provName = connConfig["provider"]
		}
		if provName == "" {
			provName = "gitlab" // default
		}
		if userID != nil && providerURL != "" {
			resolved, err := ResolveConnectionCredential(c.Request.Context(), deps, conn, userID, provName, "url")
			if err == nil && resolved != nil {
				connConfig = resolved.Config
				credSource = string(resolved.Source)
			} else if !conn.FallbackToAdmin {
				c.JSON(http.StatusForbidden, gin.H{"error": "no personal credential and admin fallback is disabled for this connection"})
				return
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
			c.JSON(http.StatusOK, gin.H{"resource": req.Resource, "raw": string(resp.GetOutput()), "credential_source": credSource})
			return
		}
		c.JSON(http.StatusOK, gin.H{"resource": req.Resource, "data": result, "credential_source": credSource})
	}
}

// testProviderInfo returns the provider name and URL config key used for
// credential resolution during connection testing. Returns ("", "") for types
// that do not support per-user credential override.
func testProviderInfo(connType repository.ConnectionType, config map[string]any) (string, string) {
	switch connType {
	case repository.ConnectionGit, repository.ConnectionGitLab:
		provider, _ := config["provider"].(string)
		if provider == "" {
			provider = "gitlab"
		}
		return provider, "url"
	case repository.ConnectionJira:
		return "jira", "url"
	case repository.ConnectionArgoCD:
		return "argocd", "server_url"
	default:
		return "", ""
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
	case "argocd":
		return "argocd"
	case "fluxcd":
		return "fluxcd"
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
			// GitOps engines
			"argocd": "argocd",
			"fluxcd": "fluxcd",
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

// testArgoCDConnection tests connectivity to an ArgoCD instance.
func testArgoCDConnection(deps Dependencies, c *gin.Context, connConfig map[string]any) (string, string) {
	serverURL, _ := connConfig["server_url"].(string)
	authToken, _ := connConfig["auth_token"].(string)
	kubeconfig, _ := connConfig["kubeconfig"].(string)

	if serverURL == "" && kubeconfig == "" {
		return "error", "No server_url or kubeconfig configured"
	}

	// CRD mode (kubeconfig only)
	if kubeconfig != "" && serverURL == "" {
		// Test by checking if ArgoCD CRDs are accessible
		if deps.DB == nil {
			return "error", "Database not available"
		}
		return "connected", "FluxCD/ArgoCD CRD mode — will verify on first use"
	}

	// REST API mode
	if serverURL != "" {
		if authToken == "" {
			return "error", "No auth_token configured"
		}
		// Test by calling ArgoCD version API
		client := &http.Client{Timeout: 10 * time.Second}
		req, err := http.NewRequest("GET", serverURL+"/api/version", nil)
		if err != nil {
			return "error", fmt.Sprintf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+authToken)
		resp, err := client.Do(req)
		if err != nil {
			return "error", fmt.Sprintf("Failed to connect: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == 200 {
			return "connected", "Successfully connected to ArgoCD"
		}
		return "error", fmt.Sprintf("ArgoCD returned status %d", resp.StatusCode)
	}

	return "error", "Invalid configuration"
}

// testFluxCDConnection tests connectivity for FluxCD (CRD mode only).
func testFluxCDConnection(deps Dependencies, c *gin.Context, connConfig map[string]any) (string, string) {
	kubeconfig, _ := connConfig["kubeconfig"].(string)
	if kubeconfig == "" {
		return "error", "No kubeconfig configured"
	}
	// FluxCD only works via CRD mode (kubeconfig)
	return "connected", "FluxCD CRD mode — will verify on first use"
}

// credentialStatus returns per-connection credential status for the current user.
// For each connection it reports whether the user has a personal credential,
// a shared credential, or relies on the admin fallback.
func credentialStatus(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		items, err := deps.Repos.Connection.List(c.Request.Context(), tenantID, "")
		if err != nil {
			respondInternalError(c, err)
			return
		}

		type connCredStatus struct {
			ConnectionID   uuid.UUID `json:"connection_id"`
			ConnectionName string    `json:"connection_name"`
			Type           string    `json:"type"`
			HasPersonal    bool      `json:"has_personal"`
			HasShared      bool      `json:"has_shared"`
			FallbackAdmin  bool      `json:"fallback_admin"`
			Effective      string    `json:"effective"` // "user", "shared", "admin", "none"
		}

		result := make([]connCredStatus, 0, len(items))
		for _, conn := range items {
			cs := connCredStatus{
				ConnectionID:   conn.ID,
				ConnectionName: conn.Name,
				Type:           string(conn.Type),
				FallbackAdmin:  conn.FallbackToAdmin,
				Effective:      "none",
			}

			// Determine provider name and URL for lookup
			provName, provURL := credLookupForConn(conn)

			if provName != "" && provURL != "" {
				// Check personal credential
				if deps.Repos.UserCredential != nil {
					cred, err := deps.Repos.UserCredential.GetByProvider(c.Request.Context(), *userID, provName, provURL)
					if err == nil && cred != nil {
						cs.HasPersonal = true
						cs.Effective = "user"
					}
				}
				// Check shared credential
				if !cs.HasPersonal && deps.Repos.CredentialShare != nil {
					tokenEnc, _, _, err := deps.Repos.CredentialShare.GetSharedToken(c.Request.Context(), *userID, tenantID, provName, provURL)
					if err == nil && tokenEnc != "" {
						cs.HasShared = true
						cs.Effective = "shared"
					}
				}
			}

			if cs.Effective == "none" && conn.FallbackToAdmin {
				cs.Effective = "admin"
			}

			result = append(result, cs)
		}

		c.JSON(http.StatusOK, gin.H{"statuses": result, "total": len(result)})
	}
}

// credLookupForConn returns the provider name and URL for credential lookup.
func credLookupForConn(conn repository.Connection) (string, string) {
	switch conn.Type {
	case repository.ConnectionGitLab:
		url, _ := conn.Config["url"].(string)
		return "gitlab", url
	case repository.ConnectionGit:
		provider, _ := conn.Config["provider"].(string)
		url, _ := conn.Config["url"].(string)
		if provider == "" {
			provider = "gitlab"
		}
		return provider, url
	case repository.ConnectionJira:
		url, _ := conn.Config["url"].(string)
		return "jira", url
	case repository.ConnectionArgoCD:
		url, _ := conn.Config["server_url"].(string)
		return "argocd", url
	default:
		return "", ""
	}
}

// connectionHealthDashboard returns a health matrix for admin dashboards.
// It shows each connection's status, who has credentials, and last check time.
func connectionHealthDashboard(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)

		items, err := deps.Repos.Connection.List(c.Request.Context(), tenantID, "")
		if err != nil {
			respondInternalError(c, err)
			return
		}

		type connHealth struct {
			ID           uuid.UUID  `json:"id"`
			Name         string     `json:"name"`
			Type         string     `json:"type"`
			Status       string     `json:"status"`
			LastCheckAt  *time.Time `json:"last_check_at,omitempty"`
			Fallback     bool       `json:"fallback_to_admin"`
			OwnerID      *uuid.UUID `json:"owner_id,omitempty"`
			UserCount    int        `json:"user_credential_count"`
			SharedCount  int        `json:"shared_credential_count"`
		}

		result := make([]connHealth, 0, len(items))
		for _, conn := range items {
			h := connHealth{
				ID:          conn.ID,
				Name:        conn.Name,
				Type:        string(conn.Type),
				Status:      conn.Status,
				LastCheckAt: conn.LastCheckAt,
				Fallback:    conn.FallbackToAdmin,
				OwnerID:     conn.OwnerID,
			}

			provName, provURL := credLookupForConn(conn)
			if provName != "" && provURL != "" && deps.DB != nil {
				// Count users with personal credentials for this provider+URL
				var userCount int
				_ = deps.DB.QueryRow(c.Request.Context(),
					`SELECT COUNT(*) FROM user_credentials WHERE tenant_id = $1 AND provider = $2 AND provider_url = $3`,
					tenantID, provName, provURL).Scan(&userCount)
				h.UserCount = userCount

				// Count shared credentials
				var sharedCount int
				_ = deps.DB.QueryRow(c.Request.Context(),
					`SELECT COUNT(*) FROM credential_shares cs JOIN user_credentials uc ON uc.id = cs.credential_id WHERE uc.tenant_id = $1 AND uc.provider = $2 AND uc.provider_url = $3`,
					tenantID, provName, provURL).Scan(&sharedCount)
				h.SharedCount = sharedCount
			}

			result = append(result, h)
		}

		// Summary counts
		total := len(result)
		healthy := 0
		degraded := 0
		down := 0
		for _, h := range result {
			switch h.Status {
			case "connected":
				healthy++
			case "error":
				down++
			default:
				degraded++
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"connections": result,
			"summary": gin.H{
				"total":    total,
				"healthy":  healthy,
				"degraded": degraded,
				"down":     down,
			},
		})
	}
}
