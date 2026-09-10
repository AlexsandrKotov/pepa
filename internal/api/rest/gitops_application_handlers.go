package rest

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/gitops"
	"github.com/pepa/pepa/internal/gitops/engine"
)

// registerGitOpsApplicationRoutes registers the unified GitOps application endpoints.
// These provide a single API surface for both ArgoCD and FluxCD applications.
func registerGitOpsApplicationRoutes(r *gin.RouterGroup, deps Dependencies) {
	apps := r.Group("/gitops/applications")
	{
		apps.GET("", listGitOpsApplications(deps))
		apps.GET("/:connection_id/:namespace/:name", getGitOpsApplication(deps))
		apps.GET("/:connection_id/:namespace/:name/history", getGitOpsApplicationHistory(deps))
		apps.GET("/:connection_id/:namespace/:name/tree", getGitOpsApplicationTree(deps))
		apps.GET("/:connection_id/:namespace/:name/events", getGitOpsApplicationEvents(deps))
		apps.POST("/:connection_id/:namespace/:name/refresh", refreshGitOpsApplication(deps))
		apps.POST("/:connection_id/:namespace/:name/sync", syncGitOpsApplication(deps))
		apps.POST("/:connection_id/:namespace/:name/rollback", rollbackGitOpsApplication(deps))
		apps.POST("/:connection_id/:namespace/:name/terminate", terminateGitOpsApplication(deps))
		apps.POST("/:connection_id/:namespace/:name/auto-sync", setAutoSyncGitOpsApplication(deps))
	}
}

// newEngineClient creates an engine client from dependencies.
func newEngineClient(deps Dependencies) *engine.Client {
	credResolver := gitops.NewCredentialResolver(deps.Repos.Connection, deps.Repos.Vault)
	return engine.NewClient(deps.ProviderRegistry, credResolver)
}

// parseAppRef extracts an AppRef from URL parameters.
func parseAppRef(c *gin.Context) (engine.AppRef, bool) {
	connIDStr := c.Param("connection_id")
	connID, err := uuid.Parse(connIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection_id"})
		return engine.AppRef{}, false
	}
	return engine.AppRef{
		ConnectionID: connID,
		TenantID:     getTenantID(c),
		Namespace:    c.Param("namespace"),
		AppName:      c.Param("name"),
	}, true
}

// getTenantID extracts the tenant ID from the Gin context.
// Delegates to auth.GetTenantID which reads the verified JWT claim.
func getTenantID(c *gin.Context) uuid.UUID {
	return auth.GetTenantID(c)
}

// listGitOpsApplications lists all GitOps applications from both engines.
// GET /api/v1/gitops/applications
func listGitOpsApplications(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		client := newEngineClient(deps)
		tenantID := getTenantID(c)

		opts := engine.ListOptions{
			TenantID:   tenantID,
			EngineType: c.Query("engine"),
			Health:     c.Query("health"),
			SyncStatus: c.Query("sync_status"),
			Environment: c.Query("environment"),
			Project:    c.Query("project"),
		}

		apps, err := client.List(c.Request.Context(), opts)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if apps == nil {
			apps = []engine.AppSummary{}
		}

		c.JSON(http.StatusOK, gin.H{
			"applications": apps,
			"total":        len(apps),
		})
	}
}

// getGitOpsApplication returns detailed information about a specific application.
// GET /api/v1/gitops/applications/:connection_id/:namespace/:name
func getGitOpsApplication(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ref, ok := parseAppRef(c)
		if !ok {
			return
		}

		client := newEngineClient(deps)
		detail, err := client.Get(c.Request.Context(), ref)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, detail)
	}
}

// getGitOpsApplicationHistory returns the deployment history.
// GET /api/v1/gitops/applications/:connection_id/:namespace/:name/history
func getGitOpsApplicationHistory(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ref, ok := parseAppRef(c)
		if !ok {
			return
		}

		client := newEngineClient(deps)
		history, err := client.History(c.Request.Context(), ref)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if history == nil {
			history = []engine.HistoryEntry{}
		}

		c.JSON(http.StatusOK, gin.H{
			"history": history,
			"total":   len(history),
		})
	}
}

// getGitOpsApplicationTree returns the resource tree.
// GET /api/v1/gitops/applications/:connection_id/:namespace/:name/tree
func getGitOpsApplicationTree(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ref, ok := parseAppRef(c)
		if !ok {
			return
		}

		client := newEngineClient(deps)
		nodes, err := client.ResourceTree(c.Request.Context(), ref)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if nodes == nil {
			nodes = []engine.ResourceNode{}
		}

		c.JSON(http.StatusOK, gin.H{"nodes": nodes, "total": len(nodes)})
	}
}

// getGitOpsApplicationEvents returns Kubernetes events.
// GET /api/v1/gitops/applications/:connection_id/:namespace/:name/events
func getGitOpsApplicationEvents(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ref, ok := parseAppRef(c)
		if !ok {
			return
		}

		client := newEngineClient(deps)
		events, err := client.Events(c.Request.Context(), ref)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if events == nil {
			events = []map[string]interface{}{}
		}

		c.JSON(http.StatusOK, gin.H{
			"events": events,
			"total":  len(events),
		})
	}
}

// refreshGitOpsApplication triggers a refresh/reconcile.
// POST /api/v1/gitops/applications/:connection_id/:namespace/:name/refresh
func refreshGitOpsApplication(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ref, ok := parseAppRef(c)
		if !ok {
			return
		}

		client := newEngineClient(deps)
		hard := c.Query("hard") == "true"
		if err := client.Refresh(c.Request.Context(), ref, hard); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "refresh triggered",
		})
	}
}

// syncGitOpsApplication triggers a sync.
// POST /api/v1/gitops/applications/:connection_id/:namespace/:name/sync
func syncGitOpsApplication(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ref, ok := parseAppRef(c)
		if !ok {
			return
		}

		var opts engine.SyncOptions
		if err := c.ShouldBindJSON(&opts); err != nil {
			// Use defaults if no body
			opts = engine.SyncOptions{}
		}

		client := newEngineClient(deps)
		if err := client.Sync(c.Request.Context(), ref, opts); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "sync triggered",
		})
	}
}

// rollbackGitOpsApplication rolls back to a previous revision.
// POST /api/v1/gitops/applications/:connection_id/:namespace/:name/rollback
func rollbackGitOpsApplication(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ref, ok := parseAppRef(c)
		if !ok {
			return
		}

		var body struct {
			HistoryID int64 `json:"history_id"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "history_id is required"})
			return
		}

		client := newEngineClient(deps)
		if err := client.Rollback(c.Request.Context(), ref, body.HistoryID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "rollback initiated",
		})
	}
}

// terminateGitOpsApplication terminates a running operation or suspends reconciliation.
// POST /api/v1/gitops/applications/:connection_id/:namespace/:name/terminate
func terminateGitOpsApplication(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ref, ok := parseAppRef(c)
		if !ok {
			return
		}

		client := newEngineClient(deps)
		if err := client.Terminate(c.Request.Context(), ref); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "terminate initiated",
		})
	}
}

// setAutoSyncGitOpsApplication enables or disables auto-sync.
// POST /api/v1/gitops/applications/:connection_id/:namespace/:name/auto-sync
func setAutoSyncGitOpsApplication(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ref, ok := parseAppRef(c)
		if !ok {
			return
		}

		var body struct {
			Enabled  bool `json:"enabled"`
			Prune    bool `json:"prune"`
			SelfHeal bool `json:"self_heal"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		client := newEngineClient(deps)
		if err := client.SetAutoSync(c.Request.Context(), ref, body.Enabled, body.Prune, body.SelfHeal); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		state := "disabled"
		if body.Enabled {
			state = "enabled"
		}
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": fmt.Sprintf("auto-sync %s", state),
		})
	}
}
