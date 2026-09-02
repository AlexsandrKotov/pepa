package rest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
)

// stateChangingProxmoxActions lists Proxmox actions that modify infrastructure state.
var stateChangingProxmoxActions = map[string]string{
	"create_vm":         "vm",
	"delete_vm":         "vm",
	"start_vm":          "vm",
	"stop_vm":           "vm",
	"shutdown_vm":       "vm",
	"reboot_vm":         "vm",
	"create_container":  "container",
	"delete_container":  "container",
	"start_container":   "container",
	"stop_container":    "container",
	"deploy_docker":     "container",
}

// stateChangingVMwareActions lists VMware actions that modify infrastructure state.
var stateChangingVMwareActions = map[string]string{
	"create_vm":       "vm",
	"delete_vm":       "vm",
	"start_vm":        "vm",
	"stop_vm":         "vm",
	"shutdown_vm":     "vm",
	"reboot_vm":       "vm",
	"suspend_vm":      "vm",
	"clone_vm":        "vm",
	"reconfigure_vm":  "vm",
	"migrate_vm":      "vm",
	"create_snapshot": "snapshot",
	"delete_snapshot": "snapshot",
	"revert_snapshot": "snapshot",
}

// logPluginActionAsync logs a plugin action asynchronously.
// Called from proxmoxExec/vmwareExec after the action completes.
// IMPORTANT: all gin.Context values are captured synchronously before the goroutine
// is spawned, because Gin may recycle the context after the handler returns.
func logPluginActionAsync(deps Dependencies, c *gin.Context, pluginName, action, entityType string, params json.RawMessage, success bool, errMsg string) {
	if deps.Repos.PluginActivity == nil {
		return
	}

	// Capture context values synchronously — gin.Context is not safe to read from a goroutine.
	tenantID := auth.GetTenantID(c)
	userID := auth.GetUserID(c)
	ipAddress := c.ClientIP()

	status := "success"
	if !success {
		status = "error"
	}

	entry := &repository.PluginActionLog{
		TenantID:     tenantID,
		UserID:       userID,
		PluginName:   pluginName,
		Action:       action,
		EntityType:   entityType,
		Params:       params,
		IPAddress:    ipAddress,
		Status:       status,
		ErrorMessage: errMsg,
	}

	go func(e *repository.PluginActionLog) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := deps.Repos.PluginActivity.LogPluginAction(ctx, e); err != nil {
			slog.Debug("failed to log plugin action", "error", err, "plugin", pluginName, "action", action)
		}
	}(entry)
}

// registerPluginActivityRoutes registers API endpoints for querying SSH command and plugin action logs.
func registerPluginActivityRoutes(r *gin.RouterGroup, deps Dependencies) {
	pa := r.Group("/plugin-activity")
	{
		pa.GET("/ssh-commands", listSSHCommands(deps))
		pa.GET("/plugin-actions", listPluginActions(deps))
	}
}

func listSSHCommands(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.PluginActivity == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin activity repository not available"})
			return
		}

		tenantID := auth.GetTenantID(c)
		filter := map[string]string{
			"tenant_id": tenantID.String(),
			"user_id":   c.Query("user_id"),
			"host_id":   c.Query("host_id"),
			"host_name": c.Query("host_name"),
			"command":   c.Query("command"),
			"page":      c.DefaultQuery("page", "1"),
		}

		items, total, err := deps.Repos.PluginActivity.ListSSHCommands(c.Request.Context(), filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"items": items,
			"total": total,
		})
	}
}

func listPluginActions(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.PluginActivity == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin activity repository not available"})
			return
		}

		tenantID := auth.GetTenantID(c)
		filter := map[string]string{
			"tenant_id":   tenantID.String(),
			"user_id":     c.Query("user_id"),
			"plugin_name": c.Query("plugin_name"),
			"action":      c.Query("action"),
			"entity_type": c.Query("entity_type"),
			"page":        c.DefaultQuery("page", "1"),
		}

		items, total, err := deps.Repos.PluginActivity.ListPluginActions(c.Request.Context(), filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"items": items,
			"total": total,
		})
	}
}
