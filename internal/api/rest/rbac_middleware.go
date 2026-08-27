package rest

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/auth"
)

// rbacResourceMap maps the first URL path segment after /api/v1 to the RBAC
// resource name used in the permissions table (must be PLURAL, matching the
// seeded permissions in the RBAC engine).
var rbacResourceMap = map[string]string{
	"entities":            "entities",
	"entity-types":        "entities",
	"plugins":             "plugins",
	"providers":           "plugins",
	"marketplace":         "plugins",
	"storage":             "plugins",
	"workflows":           "workflows",
	"team-workflows":      "workflows",
	"scorecards":          "scorecards",
	"audit":               "audit",
	"roles":               "roles",
	"role-assignments":    "roles",
	"teams":               "roles",
	"organization":        "roles",
	"workspaces":          "roles",
	"clusters":            "clusters",
	"deployments":         "deployments",
	"jira":                "jira",
	"connections":         "connections",
	"services":            "services",
	"catalog":             "services",
	"blueprints":          "services",
	"gitops":              "gitops",
	"settings":            "settings",
	"setup":               "settings",
	"environments":        "environments",
	"discovery":           "discovery",
	"docker-hosts":        "docker",
	"docker-services":     "docker",
	"helm-repositories":   "helm",
	"pipeline-sources":    "pipelines",
	"vault":               "vault",
	"ai":                  "ai",
	"credentials":         "credentials",
	"user-credentials":    "credentials",
	"service-blueprints":  "services",
	"pipeline-blueprints": "pipelines",
	"virtualization":      "virtualization",
	"s3-browser":          "connections",
	"observability":       "observability",
}

// rbacSkipPrefixes are paths that only require authentication, not a
// permission check (self-service endpoints, system info, SSE stream).
var rbacSkipPrefixes = map[string]bool{
	"my":     true, // /my/credentials — users manage their own credentials
	"me":     true, // /me/roles, /me/permissions, /me/check
	"system": true, // /system/info
	"events": true, // SSE stream (read-only, already auth-gated)
}

// rbacCache caches permission decisions to avoid hitting the database on
// every request. Entries expire after rbacCacheTTL.
type rbacCache struct {
	mu      sync.Mutex
	entries map[string]rbacCacheEntry
}

type rbacCacheEntry struct {
	allowed bool
	expires time.Time
}

const rbacCacheTTL = 5 * time.Second

func newRBACCache() *rbacCache {
	return &rbacCache{entries: make(map[string]rbacCacheEntry)}
}

func (rc *rbacCache) get(key string) (bool, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	e, ok := rc.entries[key]
	if !ok || time.Now().After(e.expires) {
		return false, false
	}
	return e.allowed, true
}

func (rc *rbacCache) set(key string, allowed bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	// Opportunistic cleanup to bound memory usage.
	if len(rc.entries) > 4096 {
		now := time.Now()
		for k, e := range rc.entries {
			if now.After(e.expires) {
				delete(rc.entries, k)
			}
		}
	}
	rc.entries[key] = rbacCacheEntry{allowed: allowed, expires: time.Now().Add(rbacCacheTTL)}
}

// rbacMiddleware enforces permission checks on all /api/v1 routes.
//
// The resource is derived from the first path segment; the action from the
// HTTP method (POST to a sub-path counts as "update", e.g. /clusters/:id/test).
// Admins (by JWT role) bypass the database check. All decisions are cached
// for a short TTL.
func rbacMiddleware(deps Dependencies) gin.HandlerFunc {
	cache := newRBACCache()

	return func(c *gin.Context) {
		// No RBAC engine configured — fail closed on writes, allow reads.
		if deps.RBAC == nil {
			switch c.Request.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				c.Next()
			default:
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authorization unavailable"})
				c.Abort()
			}
			return
		}

		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			c.Abort()
			return
		}

		// Admin bypass — role comes from the verified JWT.
		for _, r := range auth.GetRoles(c) {
			lower := strings.ToLower(r)
			if lower == "admin" || lower == "super_admin" || lower == "platform admin" || lower == "platform_admin" {
				c.Next()
				return
			}
		}

		// Derive resource from the first path segment after /api/v1.
		rel := strings.TrimPrefix(c.FullPath(), "/api/v1/")
		if rel == "" {
			rel = strings.TrimPrefix(c.Request.URL.Path, "/api/v1/")
		}
		segments := strings.Split(strings.Trim(rel, "/"), "/")
		if len(segments) == 0 || segments[0] == "" {
			c.Next()
			return
		}
		prefix := segments[0]

		if rbacSkipPrefixes[prefix] {
			c.Next()
			return
		}

		resource, ok := rbacResourceMap[prefix]
		if !ok {
			// Fail closed for unknown resources on mutating methods.
			if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
				c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
				c.Abort()
				return
			}
			c.Next()
			return
		}

		// Derive action from HTTP method.
		var action string
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			action = "read"
		case http.MethodPut, http.MethodPatch:
			action = "update"
		case http.MethodDelete:
			action = "delete"
		case http.MethodPost:
			// POST to a sub-path (e.g. /:id/test, /:id/execute) is an
			// operation on an existing resource → "update".
			if len(segments) > 1 {
				action = "update"
			} else {
				action = "create"
			}
		default:
			action = "update"
		}

		tenantID := auth.GetTenantID(c)
		cacheKey := tenantID.String() + "|" + userID.String() + "|" + resource + "|" + action
		if allowed, hit := cache.get(cacheKey); hit {
			if !allowed {
				c.JSON(http.StatusForbidden, gin.H{"error": "permission denied: requires " + resource + ":" + action})
				c.Abort()
				return
			}
			c.Next()
			return
		}

		allowed, err := deps.RBAC.CheckPermission(c.Request.Context(), tenantID, *userID, resource, action)
		if err != nil {
			// Fail closed on permission-check errors.
			c.JSON(http.StatusInternalServerError, gin.H{"error": "permission check failed"})
			c.Abort()
			return
		}
		cache.set(cacheKey, allowed)

		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied: requires " + resource + ":" + action})
			c.Abort()
			return
		}
		c.Next()
	}
}
