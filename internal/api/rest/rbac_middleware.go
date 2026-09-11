package rest

import (
	"fmt"
	"net/http"
	"sort"
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
	"entities":              "entities",
	"entity-types":          "entities",
	"plugins":               "plugins",
	"providers":             "plugins",
	"marketplace":           "plugins",
	"storage":               "plugins",
	"workflows":             "workflows",
	"team-workflows":        "workflows",
	"scorecards":            "scorecards",
	"audit":                 "audit",
	"roles":                 "roles",
	"role-assignments":      "roles",
	"teams":                 "roles",
	"organization":          "roles",
	"workspaces":            "roles",
	"clusters":              "clusters",
	"deployments":           "deployments",
	"jira":                  "jira",
	"connections":           "connections",
	"services":              "services",
	"catalog":               "services",
	"blueprints":            "services",
	"gitops":                "gitops",
	"settings":              "settings",
	"setup":                 "settings",
	"environments":          "environments",
	"discovery":             "discovery",
	"docker-hosts":          "docker",
	"docker-services":       "docker",
	"helm-repositories":     "helm",
	"registry-repositories": "registry",
	"pipeline-sources":      "pipelines",
	"vault":                 "vault",
	"ai":                    "ai",
	"credentials":           "credentials",
	"user-credentials":      "credentials",
	"service-blueprints":    "services",
	"blueprint-groups":      "services",
	"pipeline-blueprints":   "pipelines",
	"virtualization":        "virtualization",
	"s3-browser":            "connections",
	"observability":         "observability",
	"plugin-activity":       "audit",
	"ssh-hosts":             "audit",
	"ssh-host-groups":       "audit",
	"ssh-terminal":          "audit",
	"security":              "security",
	"deployment-windows":    "deployments",
	"batch-operations":      "deployments",
	"compliance-policies":   "deployments",
	"security-findings":     "security",
	"secret-rotations":      "vault",
	"deployment-audit":      "audit",
	"pre-deploy-gate":       "deployments",
	"notifications":         "notifications",
	"self-service":          "self_service",
	"auto-deploy-rules":     "auto_deploy_rules",
	"webhooks":              "auto_deploy_rules",
	// Step executions of a workflow run — same resource as workflows. This
	// prefix was previously absent, so GET /executions/:id/steps skipped RBAC.
	"executions":        "workflows",
	"service-templates": "services",
	// RAG knowledge base lives next to the AI assistant; documents are
	// tenant-scoped and writable, so they must not fall through unmapped.
	"rag": "ai",
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

const rbacCacheTTL = 30 * time.Second

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
	// In production an unmapped prefix is treated as a hard denial for every
	// method, so a newly added endpoint can never be silently readable.
	// Outside production it stays read-permissive to avoid breaking local
	// development while `verifyRBACCoverage` reports the gap at startup.
	failClosedReads := deps.Config != nil && deps.Config.Server.Env == "production"

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
		if auth.IsPlatformAdmin(c) {
			c.Next()
			return
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
			// Fail closed for unknown resources. Writes always; reads too in
			// production, so an unmapped prefix can never become a data leak.
			isRead := c.Request.Method == http.MethodGet ||
				c.Request.Method == http.MethodHead ||
				c.Request.Method == http.MethodOptions
			if !isRead || failClosedReads {
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
		orgID := auth.GetOrgID(c)
		cacheKey := tenantID.String() + "|" + orgID.String() + "|" + userID.String() + "|" + resource + "|" + action
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

// rbacAuthOnlyPrefixes lists first path segments under /api/v1 that are served
// without the RBAC middleware (they are registered directly on the engine).
// They are exempt from the coverage check below.
var rbacAuthOnlyPrefixes = map[string]bool{
	"events": true, // SSE group has its own auth middleware, no RBAC
}

// rbacRouteLister is the part of gin.Engine this check needs; taking the
// interface keeps the startup call and the unit test on the same code path.
type rbacRouteLister interface {
	Routes() gin.RoutesInfo
}

// verifyRBACCoverage walks every registered route and returns the
// "/api/v1/..." routes whose first path segment is neither mapped to an RBAC
// resource nor explicitly exempt. An unmapped prefix means the request bypasses
// the permission check, which is how /audit-logs and /rag silently leaked data.
//
// It is called once at startup (any gap is logged as an error so CI/review
// catches it before a deployment does) and from the coverage unit test.
func verifyRBACCoverage(r rbacRouteLister) []string {
	var gaps []string
	seen := map[string]bool{}
	for _, rt := range r.Routes() {
		path := rt.Path
		if !strings.HasPrefix(path, "/api/v1/") {
			continue
		}
		// Authentication endpoints live on the engine itself, not in the RBAC
		// group: the public ones cannot require a permission (there is no token
		// yet) and the protected ones carry their own auth + admin middleware.
		if path == "/api/v1/auth" || strings.HasPrefix(path, "/api/v1/auth/") {
			continue
		}
		rel := strings.TrimPrefix(path, "/api/v1/")
		segments := strings.Split(strings.Trim(rel, "/"), "/")
		if len(segments) == 0 || segments[0] == "" {
			continue
		}
		prefix := segments[0]
		if rbacAuthOnlyPrefixes[prefix] || rbacSkipPrefixes[prefix] {
			continue
		}
		if _, ok := rbacResourceMap[prefix]; ok {
			continue
		}
		key := prefix + "|" + rt.Method
		if seen[key] {
			continue
		}
		seen[key] = true
		gaps = append(gaps, fmt.Sprintf("%s %s (unmapped prefix %q)", rt.Method, path, prefix))
	}
	sort.Strings(gaps)
	return gaps
}
