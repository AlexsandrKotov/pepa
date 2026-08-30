package rest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/pkg/models"
)
// auditLogCh is a bounded channel for async audit log writes.
// It prevents unbounded goroutine creation when the DB is overloaded.
var auditLogCh = make(chan *models.AuditLog, 256)

func init() {
	// Start a pool of audit log writer goroutines.
	for i := 0; i < 4; i++ {
		go func() {
			for entry := range auditLogCh {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				// The repo is set on the entry via a small wrapper; we use a package-level ref.
				if auditRepo != nil {
					if err := auditRepo.Create(ctx, entry); err != nil {
						slog.Info("audit log write failed", "error", err)
					}
				}
				cancel()
			}
		}()
	}
}

// auditRepo is set during init to allow the worker goroutines to write audit logs.
var auditRepo interface{ Create(context.Context, *models.AuditLog) error }

// initAuditWorkers sets the audit repository for the background worker pool.
// Called once during router setup.
func initAuditWorkers(repo interface{ Create(context.Context, *models.AuditLog) error }) {
	auditRepo = repo
}
// registerAuditRoutes registers audit log API endpoints.
func registerAuditRoutes(r *gin.RouterGroup, deps Dependencies) {
	audit := r.Group("/audit")
	{
		audit.GET("", listAuditLogs(deps))
		audit.GET("/stats", auditStats(deps))
	}
	// Alias: /audit-logs → /audit (frontend compatibility)
	auditLogs := r.Group("/audit-logs")
	{
		auditLogs.GET("", listAuditLogs(deps))
		auditLogs.GET("/stats", auditStats(deps))
	}
}

// logAudit is a helper to write audit entries from handlers.
func logAudit(deps Dependencies, c *gin.Context, action, entityType, entityID string, oldValues, newValues interface{}) {
	if deps.Repos.Audit == nil {
		return
	}

	tenantID := auth.GetTenantID(c)
	userID := auth.GetUserID(c)

	entry := &models.AuditLog{
		TenantID:   tenantID,
		UserID:     userID,
		Action:     action,
		EntityType: entityType,
		IPAddress:  net.ParseIP(c.ClientIP()),
		UserAgent:  c.Request.UserAgent(),
	}

	if entityID != "" {
		if id, err := uuid.Parse(entityID); err == nil {
			entry.EntityID = &id
		}
	}

	if oldValues != nil {
		data, _ := json.Marshal(oldValues)
		entry.OldValues = data
	}
	if newValues != nil {
		data, _ := json.Marshal(newValues)
		entry.NewValues = data
	}

	// Send to bounded worker pool instead of spawning an unbounded goroutine.
	select {
	case auditLogCh <- entry:
	default:
		// Channel full — log synchronously as fallback to avoid losing audit entries.
		slog.Info("audit log channel full, writing synchronously")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if auditRepo != nil {
			if err := auditRepo.Create(ctx, entry); err != nil {
				slog.Info("audit log write failed", "error", err)
			}
		}
	}
}

func listAuditLogs(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var filter models.AuditFilter
		if err := c.ShouldBindQuery(&filter); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		result, err := deps.Repos.Audit.List(c.Request.Context(), filter)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

func auditStats(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		byAction, err := deps.Repos.Audit.CountByAction(c.Request.Context())
		if err != nil {
			respondInternalError(c, err)
			return
		}

		byResource, err := deps.Repos.Audit.CountByResource(c.Request.Context())
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"by_action":   byAction,
			"by_resource": byResource,
		})
	}
}

// apiAuditMiddleware logs all state-changing API requests (POST/PUT/PATCH/DELETE)
// to the audit log. This acts as a catch-all so that no mutation goes unrecorded,
// even if the handler itself forgets to call logAudit explicitly.
func apiAuditMiddleware(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only log write operations
		method := c.Request.Method
		if method != "POST" && method != "PUT" && method != "PATCH" && method != "DELETE" {
			c.Next()
			return
		}

		// Skip health and system info endpoints
		path := c.Request.URL.Path
		if path == "/api/v1/system/info" {
			c.Next()
			return
		}

		// Process request
		c.Next()

		// Only log if the audit repo is available and response was successful (2xx)
		if deps.Repos.Audit == nil {
			return
		}
		status := c.Writer.Status()
		if status < 200 || status >= 300 {
			return
		}

		// Derive action from HTTP method
		action := methodToAction(method)

		// Derive entity type from URL path
		entityType := pathToEntityType(path)
		if entityType == "" {
			return
		}

		tenantID := auth.GetTenantID(c)
		userID := auth.GetUserID(c)

		entry := &models.AuditLog{
			TenantID:   tenantID,
			UserID:     userID,
			Action:     action,
			EntityType: entityType,
			IPAddress:  net.ParseIP(c.ClientIP()),
			UserAgent:  c.Request.UserAgent(),
		}

		// Try to extract entity ID from path params
		if entityID := c.Param("id"); entityID != "" {
			if id, err := uuid.Parse(entityID); err == nil {
				entry.EntityID = &id
			}
		} else if entityID := c.Param("name"); entityID != "" {
			// For settings and other named resources
			entry.EntityID = nil
		}

		// Fire-and-forget
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := deps.Repos.Audit.Create(ctx, entry); err != nil {
				slog.Info("api audit middleware write failed", "error", err)
			}
		}()
	}
}

// methodToAction maps HTTP method to an audit action verb.
func methodToAction(method string) string {
	switch method {
	case "POST":
		return "api_create"
	case "PUT":
		return "api_update"
	case "PATCH":
		return "api_patch"
	case "DELETE":
		return "api_delete"
	default:
		return "api_request"
	}
}

// pathToEntityType extracts a human-readable entity type from the API path.
func pathToEntityType(path string) string {
	// Strip /api/v1/ prefix
	trimmed := path
	if len(trimmed) > 7 && trimmed[:7] == "/api/v1" {
		trimmed = trimmed[7:]
	}
	// Get the first path segment
	for i := 1; i < len(trimmed); i++ {
		if trimmed[i] == '/' {
			segment := trimmed[1:i]
			return pathSegmentToType(segment)
		}
	}
	if len(trimmed) > 1 {
		return pathSegmentToType(trimmed[1:])
	}
	return ""
}

func pathSegmentToType(segment string) string {
	// Map common API path segments to entity types
	mapping := map[string]string{
		"entities":          "entity",
		"workflows":         "workflow",
		"plugins":           "plugin",
		"scorecards":        "scorecard",
		"clusters":          "cluster",
		"deployments":       "deployment",
		"jira":              "jira",
		"connections":       "connection",
		"services":          "service",
		"catalog":           "catalog",
		"gitops":            "gitops",
		"settings":          "setting",
		"environments":      "environment",
		"marketplace":       "marketplace",
		"discovery":         "discovery",
		"docker-hosts":      "docker_host",
		"docker-services":   "docker_service",
		"helm-repositories": "helm_repository",
		"pipeline-sources":  "pipeline_source",
		"pipeline-runs":     "pipeline_run",
		"vault":             "vault",
		"teams":             "team",
		"roles":             "role",
		"users":             "user",
		"credentials":       "credential",
		"audit":             "audit",
		"storage":           "storage",
		"ai":                "ai",
	}
	if t, ok := mapping[segment]; ok {
		return t
	}
	return segment
}
