package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/pkg/models"
)

func registerPluginRoutes(r *gin.RouterGroup, deps Dependencies) {
	// Read-only endpoints — available to all authenticated users.
	plugins := r.Group("/plugins")
	{
		plugins.GET("", listPlugins(deps))
		plugins.GET("/:name", getPlugin(deps))
		plugins.GET("/:name/health", getPluginHealth(deps))
	}

	// Write endpoints — require admin role.
	adminPlugins := r.Group("/plugins")
	adminPlugins.Use(requireAdminRole(deps))
	{
		adminPlugins.POST("/install", installPlugin(deps))
		adminPlugins.POST("/:name/configure", configurePlugin(deps))
		adminPlugins.POST("/:name/enable", enablePlugin(deps))
		adminPlugins.POST("/:name/disable", disablePlugin(deps))
		adminPlugins.DELETE("/:name", uninstallPlugin(deps))
		adminPlugins.POST("/:name/execute", executePluginAction(deps))
	}

	// Provider registry endpoints
	providers := r.Group("/providers")
	{
		providers.GET("", listProviders(deps))
		providers.GET("/summary", providerSummary(deps))
		providers.GET("/:name", getProvider(deps))
		providers.GET("/:name/health", providerHealth(deps))
	}

	// Provider write endpoints — require admin role.
	adminProviders := r.Group("/providers")
	adminProviders.Use(requireAdminRole(deps))
	{
		adminProviders.POST("/:name/execute", executeProviderAction(deps))
	}
}

func listPlugins(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Plugin != nil {
			plugins, err := deps.Repos.Plugin.List(c.Request.Context())
			if err != nil {
				respondInternalError(c, err)
				return
			}
			if plugins == nil {
				plugins = []repository.Plugin{}
			}
			// Decrypt config and enrich with live data from the provider registry.
			result := make([]map[string]interface{}, 0, len(plugins))
			for i := range plugins {
				plugins[i].Config = repository.DecryptPluginConfig(plugins[i].Config)

				pMap := map[string]interface{}{
					"id":           plugins[i].ID,
					"name":         plugins[i].Name,
					"version":      plugins[i].Version,
					"type":         plugins[i].Type,
					"status":       plugins[i].Status,
					"config":       plugins[i].Config,
					"enabled":      plugins[i].Enabled,
					"installed_at": plugins[i].InstalledAt,
					"updated_at":   plugins[i].UpdatedAt,
				}

				if deps.ProviderRegistry != nil {
					if entry, ok := deps.ProviderRegistry.Get(plugins[i].Name); ok {
						// Merge enabled state from the live registry (source of truth at runtime).
						pMap["enabled"] = entry.Enabled
						if entry.Info != nil {
							actions := make([]string, 0, len(entry.Info.Actions))
							for _, a := range entry.Info.Actions {
								actions = append(actions, a.Name)
							}
							pMap["actions"] = actions
						}
					}
				}

				result = append(result, pMap)
			}
			c.JSON(http.StatusOK, gin.H{"plugins": result})
			return
		}
		// Fallback to in-memory manager
		plugins := deps.PluginMgr.ListPlugins()
		if plugins == nil {
			plugins = []*models.Plugin{}
		}
		c.JSON(http.StatusOK, gin.H{"plugins": plugins})
	}
}

func installPlugin(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Plugin == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin repository not available"})
			return
		}

		var req struct {
			Name    string          `json:"name" binding:"required"`
			Version string          `json:"version" binding:"required"`
			Type    string          `json:"type"`
			Config  json.RawMessage `json:"config"`
			Enabled bool            `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		p := &repository.Plugin{
			Name:    req.Name,
			Version: req.Version,
			Type:    req.Type,
			Config:  req.Config,
			Enabled: req.Enabled,
			Status:  "installed",
		}

		if err := deps.Repos.Plugin.Register(c.Request.Context(), p); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "install", "plugin", p.ID.String(), nil, gin.H{"name": p.Name, "version": p.Version})
		c.JSON(http.StatusCreated, p)
	}
}

func getPlugin(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")

		if deps.Repos.Plugin != nil {
			p, err := deps.Repos.Plugin.GetByName(c.Request.Context(), name)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, p)
			return
		}

		plugin := deps.PluginMgr.GetPlugin(name)
		if plugin == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
			return
		}
		c.JSON(http.StatusOK, plugin)
	}
}

func configurePlugin(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only platform admins can configure plugins (plugin configs are global)
		if !auth.IsPlatformAdmin(c) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only platform administrators can configure plugins"})
			return
		}

		name := c.Param("name")

		if deps.Repos.Plugin == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin repository not available"})
			return
		}

		var req struct {
			Config json.RawMessage `json:"config"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		p, err := deps.Repos.Plugin.GetByName(c.Request.Context(), name)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		p.Config = req.Config
		if err := deps.Repos.Plugin.Register(c.Request.Context(), p); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "configure", "plugin", p.ID.String(), nil, gin.H{"name": name})
		c.JSON(http.StatusOK, p)
	}
}

func enablePlugin(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only platform admins can enable plugins (plugins are global)
		if !auth.IsPlatformAdmin(c) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only platform administrators can enable plugins"})
			return
		}

		name := c.Param("name")

		if deps.Repos.Plugin != nil {
			p, err := deps.Repos.Plugin.GetByName(c.Request.Context(), name)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			p.Enabled = true
			p.Status = "installed"
			if err := deps.Repos.Plugin.Register(c.Request.Context(), p); err != nil {
				respondInternalError(c, err)
				return
			}
			logAudit(deps, c, "enable", "plugin", p.ID.String(), nil, gin.H{"name": name})
		}

		// Also enable in the live Manager (gRPC subprocess model).
		// If the binary was unloaded (e.g. after uninstall), load it back.
		if err := loadPluginBinary(deps, name); err != nil {
			slog.Warn("could not activate plugin ", "name", name, "error", err)
		}

		// Also enable in provider registry if available.
		if deps.ProviderRegistry != nil {
			if entry, ok := deps.ProviderRegistry.Get(name); ok {
				entry.Enabled = true
			}
		}

		c.JSON(http.StatusOK, gin.H{"message": "plugin enabled", "name": name})
	}
}

func disablePlugin(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only platform admins can disable plugins (plugins are global)
		if !auth.IsPlatformAdmin(c) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only platform administrators can disable plugins"})
			return
		}

		name := c.Param("name")

		if deps.Repos.Plugin != nil {
			p, err := deps.Repos.Plugin.GetByName(c.Request.Context(), name)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			p.Enabled = false
			if err := deps.Repos.Plugin.Register(c.Request.Context(), p); err != nil {
				respondInternalError(c, err)
				return
			}
			logAudit(deps, c, "disable", "plugin", p.ID.String(), nil, gin.H{"name": name})
		}

		// Also disable in the live Manager (gRPC subprocess model).
		if deps.PluginMgr != nil {
			_ = deps.PluginMgr.Disable(name)
		}

		// Also disable in provider registry if available.
		if deps.ProviderRegistry != nil {
			if entry, ok := deps.ProviderRegistry.Get(name); ok {
				entry.Enabled = false
			}
		}

		c.JSON(http.StatusOK, gin.H{"message": "plugin disabled", "name": name})
	}
}

func uninstallPlugin(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")

		if deps.Repos.Plugin != nil {
			p, err := deps.Repos.Plugin.GetByName(c.Request.Context(), name)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			// Soft-delete by disabling and marking as uninstalled.
			p.Enabled = false
			p.Status = "uninstalled"
			if err := deps.Repos.Plugin.Register(c.Request.Context(), p); err != nil {
				respondInternalError(c, err)
				return
			}
			logAudit(deps, c, "uninstall", "plugin", p.ID.String(), nil, gin.H{"name": name})
		}

		// Disable in the live Manager so the gRPC subprocess model is consistent.
		if deps.PluginMgr != nil {
			_ = deps.PluginMgr.Disable(name)
		}

		// Disable in provider registry so no further actions are dispatched.
		if deps.ProviderRegistry != nil {
			if entry, ok := deps.ProviderRegistry.Get(name); ok {
				entry.Enabled = false
			}
		}

		c.JSON(http.StatusOK, gin.H{"message": "plugin uninstalled", "name": name})
	}
}

func getPluginHealth(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")

		// Check if the plugin is disabled in the provider registry.
		if deps.ProviderRegistry != nil {
			if entry, ok := deps.ProviderRegistry.Get(name); ok && !entry.Enabled {
				c.JSON(http.StatusOK, gin.H{
					"plugin_name": name,
					"status":      "disabled",
					"message":     "plugin is disabled",
				})
				return
			}
		}

		health := deps.PluginMgr.HealthCheck(name)
		c.JSON(http.StatusOK, health)
	}
}

func executePluginAction(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		var req struct {
			Action string            `json:"action" binding:"required"`
			Params json.RawMessage   `json:"params"`
			Config map[string]string `json:"config"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Merge stored plugin config from DB with request config (request takes priority)
		mergedConfig := mergeStoredPluginConfig(deps, name, req.Config, c.Request.Context())

		resp, err := deps.PluginMgr.Execute(c.Request.Context(), name, req.Action, req.Params, mergedConfig)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ── Provider Registry handlers ───────────────────────────────

func listProviders(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.ProviderRegistry == nil {
			c.JSON(http.StatusOK, gin.H{"providers": []interface{}{}})
			return
		}
		providers := deps.ProviderRegistry.List()
		result := make([]map[string]interface{}, 0, len(providers))
		for _, p := range providers {
			actions := make([]string, 0)
			if p.Info != nil {
				for _, a := range p.Info.Actions {
					actions = append(actions, a.Name)
				}
			}
			result = append(result, map[string]interface{}{
				"name":          p.Name,
				"type":          p.Type,
				"enabled":       p.Enabled,
				"actions":       actions,
				"connection_id": p.ConnectionID,
			})
		}
		c.JSON(http.StatusOK, gin.H{"providers": result, "total": len(result)})
	}
}

func providerSummary(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.ProviderRegistry == nil {
			c.JSON(http.StatusOK, gin.H{"providers": []interface{}{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"providers": deps.ProviderRegistry.Summary()})
	}
}

func getProvider(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if deps.ProviderRegistry == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "provider registry not available"})
			return
		}
		entry, ok := deps.ProviderRegistry.Get(name)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
			return
		}
		actions := make([]string, 0)
		if entry.Info != nil {
			for _, a := range entry.Info.Actions {
				actions = append(actions, a.Name)
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"name":          entry.Name,
			"type":          entry.Type,
			"enabled":       entry.Enabled,
			"actions":       actions,
			"connection_id": entry.ConnectionID,
		})
	}
}

func executeProviderAction(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if deps.ProviderRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provider registry not available"})
			return
		}

		var req struct {
			Action string            `json:"action" binding:"required"`
			Params json.RawMessage   `json:"params"`
			Config map[string]string `json:"config"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Merge stored plugin config from DB with request config (request takes priority)
		mergedConfig := mergeStoredPluginConfig(deps, name, req.Config, c.Request.Context())

		resp, err := deps.ProviderRegistry.ExecuteAction(c.Request.Context(), name, req.Action, req.Params, mergedConfig)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": resp.Success,
			"output":  json.RawMessage(resp.Output),
			"error":   resp.Error,
		})
	}
}

func providerHealth(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if deps.ProviderRegistry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provider registry not available"})
			return
		}
		health, err := deps.ProviderRegistry.HealthCheck(c.Request.Context(), name)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, health)
	}
}

// registerStepExecutionRoutes adds step execution endpoints.
func registerStepExecutionRoutes(r *gin.RouterGroup, deps Dependencies) {
	r.GET("/executions/:execId/steps", listStepExecutions(deps))
}

// pluginToConnType maps plugin names to the connection type that provides their credentials.
var pluginToConnType = map[string]string{
	"gitlab":    "gitlab",
	"github":    "git",
	"jira":      "jira",
	"bitbucket": "git",
	"gitea":     "git",
	"fluxcd":    "kubernetes",
	"argocd":    "kubernetes",
	"proxmox":   "proxmox",
	"vmware":    "vmware",
}

// mergeStoredPluginConfig builds the final config for a plugin action by merging
// three sources (lowest to highest priority):
//  1. Connection config (from Connections page — the single source of truth for credentials)
//  2. Plugin stored config (from Plugins page Configuration section)
//  3. Request config (per-call overrides from Test Actions UI)
func mergeStoredPluginConfig(deps Dependencies, name string, reqConfig map[string]string, requestCtx context.Context) map[string]string {
	merged := make(map[string]string)

	// 1. Pull from matching connection (single source of truth)
	if connType, ok := pluginToConnType[name]; ok && deps.Repos.Connection != nil {
		if conns, err := deps.Repos.Connection.FindByTypeDecrypted(requestCtx, connType); err == nil && len(conns) > 0 {
			// For git-type connections, match by provider in config
			var conn *repository.Connection
			if connType == "git" {
				for i := range conns {
					if provider, _ := conns[i].Config["provider"].(string); provider == name {
						conn = &conns[i]
						break
					}
				}
			} else if len(conns) > 0 {
				conn = &conns[0]
			}
			if conn != nil {
				for k, v := range conn.Config {
					switch val := v.(type) {
					case string:
						merged[k] = val
					case float64:
						merged[k] = fmt.Sprintf("%v", val)
					case bool:
						merged[k] = fmt.Sprintf("%v", val)
					default:
						if b, err := json.Marshal(val); err == nil {
							merged[k] = string(b)
						}
					}
				}
			}
		}
	}

	// 2. Override with plugin's own stored config from DB (decrypted)
	if deps.Repos.Plugin != nil {
		if p, err := deps.Repos.Plugin.GetByNameDecrypted(requestCtx, name); err == nil && p != nil && len(p.Config) > 0 {
			var raw map[string]interface{}
			if json.Unmarshal(p.Config, &raw) == nil {
				for k, v := range raw {
					switch val := v.(type) {
					case string:
						merged[k] = val
					case float64:
						merged[k] = fmt.Sprintf("%v", val)
					case bool:
						merged[k] = fmt.Sprintf("%v", val)
					default:
						if b, err := json.Marshal(val); err == nil {
							merged[k] = string(b)
						}
					}
				}
			}
		}
	}

	// 3. Override with per-request config (highest priority)
	for k, v := range reqConfig {
		merged[k] = v
	}

	return merged
}

func listStepExecutions(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		execID, err := uuid.Parse(c.Param("execId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid execution ID"})
			return
		}

		steps, err := deps.Repos.Workflow.ListStepExecutions(c.Request.Context(), execID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"step_executions": steps})
	}
}
