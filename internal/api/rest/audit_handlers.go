package rest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/pkg/models"
)
// auditLogCh is a bounded channel for async audit log writes.
// It prevents unbounded goroutine creation when the DB is overloaded.
// Capacity increased to 1024 since we now log ALL requests (including GET).
var auditLogCh = make(chan *models.AuditLog, 1024)

func init() {
	// Start a pool of audit log writer goroutines.
	// More workers since we log every request now.
	for i := 0; i < 8; i++ {
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
		IPAddress:  c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
	}

	// Build metadata with user info
	meta := map[string]interface{}{}
	if userID != nil {
		meta["user_id"] = userID.String()
	}
	if email := auth.GetEmail(c); email != "" {
		meta["user_email"] = email
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
		// Merge user info into newValues for richer context
		if nvMap, ok := newValues.(map[string]interface{}); ok {
			for k, v := range meta {
				if _, exists := nvMap[k]; !exists {
					nvMap[k] = v
				}
			}
			data, _ := json.Marshal(nvMap)
			entry.NewValues = data
		} else {
			data, _ := json.Marshal(newValues)
			entry.NewValues = data
		}
	} else if len(meta) > 0 {
		data, _ := json.Marshal(meta)
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

// apiAuditMiddleware logs ALL API requests to the audit log with full detail.
// Every request (GET, POST, PUT, PATCH, DELETE) is recorded so we can see
// exactly what each user did: which page they visited, what they clicked,
// what they launched, etc.
func apiAuditMiddleware(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip health and system info endpoints to reduce noise
		path := c.Request.URL.Path
		if path == "/api/v1/system/info" || path == "/health" || path == "/healthz" || path == "/metrics" {
			c.Next()
			return
		}
		// Skip audit and observability read endpoints to avoid self-referential log spam
		if (path == "/api/v1/audit" || path == "/api/v1/audit-logs" ||
			path == "/api/v1/audit/stats" || path == "/api/v1/audit-logs/stats" ||
			path == "/api/v1/observability/logs" || path == "/api/v1/observability/overview" ||
			path == "/api/v1/observability/settings") && c.Request.Method == "GET" {
			c.Next()
			return
		}

		// Process request
		c.Next()

		// Only log if the audit repo is available
		if deps.Repos.Audit == nil {
			return
		}

		status := c.Writer.Status()

		// Derive action from HTTP method
		method := c.Request.Method
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
			IPAddress:  c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
		}

		// Try to extract entity ID from path params
		if entityID := c.Param("id"); entityID != "" {
			if id, err := uuid.Parse(entityID); err == nil {
				entry.EntityID = &id
			}
		} else if entityID := c.Param("name"); entityID != "" {
			entry.EntityID = nil
		}

		// Build detailed metadata for the audit entry
		meta := map[string]interface{}{
			"method":      method,
			"path":        path,
			"status_code": status,
		}
		if qs := c.Request.URL.RawQuery; qs != "" {
			meta["query"] = qs
		}
		if userID != nil {
			meta["user_id"] = userID.String()
		}
		if email := auth.GetEmail(c); email != "" {
			meta["user_email"] = email
		}
		meta["ip"] = c.ClientIP()
		meta["user_agent"] = c.Request.UserAgent()

		// For state-changing requests, include content metadata
		if (method == "POST" || method == "PUT" || method == "PATCH") && status >= 200 && status < 300 {
			if c.Request.ContentLength > 0 && c.Request.ContentLength < 10240 {
				meta["content_type"] = c.ContentType()
				meta["content_length"] = c.Request.ContentLength
			}
		}

		metaJSON, _ := json.Marshal(meta)
		entry.NewValues = metaJSON

		// Send to bounded worker pool
		select {
		case auditLogCh <- entry:
		default:
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
}

// methodToAction maps HTTP method to an audit action verb.
func methodToAction(method string) string {
	switch method {
	case "GET":
		return "view"
	case "POST":
		return "create"
	case "PUT":
		return "update"
	case "PATCH":
		return "patch"
	case "DELETE":
		return "delete"
	default:
		return "request"
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
		"registry-repositories": "registry_repository",
		"pipeline-sources":  "pipeline_source",
		"pipeline-runs":     "pipeline_run",
		"vault":             "vault",
		"teams":             "team",
		"roles":             "role",
		"users":             "user",
		"credentials":       "credential",
		"audit":             "audit",
		"audit-logs":        "audit",
		"storage":           "storage",
		"ai":                "ai",
		"observability":     "observability",
		"organization":      "organization",
		"workspaces":        "workspace",
		"blueprints":        "blueprint",
		"blueprint-groups":  "blueprint_group",
		"s3-browser":        "s3",
		"virtualization":    "virtualization",
		"rbac":              "rbac",
		"auth":              "auth",
	}
	if t, ok := mapping[segment]; ok {
		return t
	}
	return segment
}
