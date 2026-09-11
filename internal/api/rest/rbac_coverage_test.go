package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/config"
	rbacengine "github.com/pepa/pepa/internal/rbac/engine"
)

// TestRouterRBACCoverageHasNoGaps is the regression guard for the class of bug
// that left /audit-logs, /rag and /executions reachable without a permission
// check: a route registered under /api/v1 whose first path segment is not in
// rbacResourceMap silently skips RBAC. Adding a route without mapping it makes
// this test fail.
func TestRouterRBACCoverageHasNoGaps(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deps := Dependencies{Config: &config.Config{}}
	deps.Config.Server.Env = "test"
	deps.Repos = &Repositories{}

	handler, shutdown := NewRouter(deps)
	if shutdown != nil {
		defer shutdown()
	}

	lister, ok := handler.(rbacRouteLister)
	if !ok {
		t.Fatalf("router does not expose routes: %T", handler)
	}

	if gaps := verifyRBACCoverage(lister); len(gaps) > 0 {
		t.Errorf("routes without RBAC coverage (add them to rbacResourceMap or rbacSkipPrefixes):\n%s", joinLines(gaps))
	}
}

// TestVerifyRBACCoverageFlagsUnmappedPrefix proves the checker actually reports
// a gap; without this, the "no gaps" assertion above could pass vacuously if the
// walk stopped matching any route at all.
func TestVerifyRBACCoverageFlagsUnmappedPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.GET("/entities", noopHandler)         // mapped resource
	v1.GET("/me/roles", noopHandler)         // explicitly exempt prefix
	v1.POST("/brand-new-thing", noopHandler) // unmapped → gap
	v1.GET("/brand-new-thing", noopHandler)  // unmapped → gap
	r.GET("/healthz", noopHandler)           // outside /api/v1 → ignored

	gaps := verifyRBACCoverage(r)
	if len(gaps) != 2 {
		t.Fatalf("expected 2 gaps, got %d: %v", len(gaps), gaps)
	}
	for _, g := range gaps {
		if !contains(g, "brand-new-thing") {
			t.Errorf("unexpected gap reported: %q", g)
		}
	}
}

// TestRBACResourceMapResourcesAreSeeded catches the opposite mistake: a prefix
// mapped to a resource that no role is ever granted. Every such route answers
// 403 to non-admins forever — what happened to /registry and /observability.
func TestRBACResourceMapResourcesAreSeeded(t *testing.T) {
	seeded := make(map[string]bool, len(rbacengine.AllRBACResources))
	for _, res := range rbacengine.AllRBACResources {
		seeded[res] = true
	}

	for prefix, resource := range rbacResourceMap {
		if !seeded[resource] {
			t.Errorf("rbacResourceMap[%q] = %q, but %q is not in AllRBACResources and is never granted to any role",
				prefix, resource, resource)
		}
	}
}

// TestRBACMiddlewareUnmappedPrefixFailsClosed pins the authorisation behaviour
// for routes that were added without updating rbacResourceMap: a write is always
// denied, and in production a read is denied too.
func TestRBACMiddlewareUnmappedPrefixFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	tenantID := uuid.New()

	newEngine := func(env string) *gin.Engine {
		deps := Dependencies{
			Config: &config.Config{},
			// Non-nil engine with no pool: reachable only for mapped resources,
			// which these tests never request.
			RBAC: rbacengine.New(nil),
		}
		deps.Config.Server.Env = env

		r := gin.New()
		v1 := r.Group("/api/v1", func(c *gin.Context) {
			c.Set(auth.CtxUserID, userID)
			c.Set(auth.CtxTenantID, tenantID)
			c.Set(auth.CtxRoles, []string{"developer"})
		}, rbacMiddleware(deps))
		v1.GET("/unknown-read", noopHandler)
		v1.POST("/unknown-write", noopHandler)
		return r
	}

	do := func(r *gin.Engine, method, path string) int {
		req := httptest.NewRequest(method, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	dev := newEngine("development")
	if code := do(dev, http.MethodPost, "/api/v1/unknown-write"); code != http.StatusForbidden {
		t.Errorf("unmapped write outside production: got %d, want %d", code, http.StatusForbidden)
	}
	if code := do(dev, http.MethodGet, "/api/v1/unknown-read"); code != http.StatusOK {
		t.Errorf("unmapped read in development: got %d, want %d", code, http.StatusOK)
	}

	prod := newEngine("production")
	if code := do(prod, http.MethodGet, "/api/v1/unknown-read"); code != http.StatusForbidden {
		t.Errorf("unmapped read in production: got %d, want %d", code, http.StatusForbidden)
	}
	if code := do(prod, http.MethodPost, "/api/v1/unknown-write"); code != http.StatusForbidden {
		t.Errorf("unmapped write in production: got %d, want %d", code, http.StatusForbidden)
	}
}

func noopHandler(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }

func joinLines(items []string) string {
	out := ""
	for _, item := range items {
		out += "\t" + item + "\n"
	}
	return out
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
