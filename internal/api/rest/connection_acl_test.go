package rest

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
)

// newTestContext creates a gin test context with tenant, user, and roles set.
func newTestContext(t *testing.T, tenantID, userID uuid.UUID, roles []string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set(auth.CtxTenantID, tenantID)
	c.Set(auth.CtxUserID, userID)
	c.Set(auth.CtxRoles, roles)
	return c
}

func TestCheckConnectionAccess_AdminBypass(t *testing.T) {
	deps := Dependencies{Repos: &Repositories{}}
	tenantID := uuid.New()
	conn := &repository.Connection{
		ID:         uuid.New(),
		TenantID:   tenantID,
		Restricted: true,
	}

	// Admin should always have access, even to restricted connections with no ACL.
	c := newTestContext(t, tenantID, uuid.New(), []string{"admin"})
	if !checkConnectionAccess(deps, c, conn, "read") {
		t.Fatal("admin bypass failed for read")
	}
	if !checkConnectionAccess(deps, c, conn, "use") {
		t.Fatal("admin bypass failed for use")
	}
}

func TestCheckConnectionAccess_OwnerBypass(t *testing.T) {
	deps := Dependencies{Repos: &Repositories{}}
	tenantID := uuid.New()
	ownerID := uuid.New()
	conn := &repository.Connection{
		ID:         uuid.New(),
		TenantID:   tenantID,
		OwnerID:    &ownerID,
		Restricted: true,
	}

	// Owner should have access even to restricted connections with no ACL.
	c := newTestContext(t, tenantID, ownerID, []string{"developer"})
	if !checkConnectionAccess(deps, c, conn, "read") {
		t.Fatal("owner bypass failed for read")
	}
	if !checkConnectionAccess(deps, c, conn, "use") {
		t.Fatal("owner bypass failed for use")
	}
}

func TestCheckConnectionAccess_NonRestrictedAllowsAll(t *testing.T) {
	deps := Dependencies{Repos: &Repositories{}}
	tenantID := uuid.New()
	userID := uuid.New()
	conn := &repository.Connection{
		ID:         uuid.New(),
		TenantID:   tenantID,
		Restricted: false, // not restricted
	}

	// Any authenticated user should have access to non-restricted connections.
	c := newTestContext(t, tenantID, userID, []string{"developer"})
	if !checkConnectionAccess(deps, c, conn, "read") {
		t.Fatal("non-restricted connection should be readable by any user")
	}
	if !checkConnectionAccess(deps, c, conn, "use") {
		t.Fatal("non-restricted connection should be usable by any user")
	}
}

func TestCheckConnectionAccess_RestrictedDeniesWithoutACL(t *testing.T) {
	deps := Dependencies{Repos: &Repositories{}}
	tenantID := uuid.New()
	userID := uuid.New()
	otherID := uuid.New() // not the owner
	conn := &repository.Connection{
		ID:         uuid.New(),
		TenantID:   tenantID,
		OwnerID:    &otherID,
		Restricted: true,
	}

	// Non-admin, non-owner user should be denied access to restricted connection
	// when no ACL repository is available.
	c := newTestContext(t, tenantID, userID, []string{"developer"})
	if checkConnectionAccess(deps, c, conn, "read") {
		t.Fatal("restricted connection should deny non-admin/non-owner without ACL")
	}
	if checkConnectionAccess(deps, c, conn, "use") {
		t.Fatal("restricted connection should deny use for non-admin/non-owner without ACL")
	}
}

func TestCheckConnectionAccess_NilUserID(t *testing.T) {
	deps := Dependencies{Repos: &Repositories{}}
	tenantID := uuid.New()
	conn := &repository.Connection{
		ID:         uuid.New(),
		TenantID:   tenantID,
		Restricted: true,
	}

	// Context without user ID should be denied.
	c := newTestContext(t, tenantID, uuid.Nil, []string{})
	// Remove the user ID from context to simulate missing auth.
	delete(c.Keys, auth.CtxUserID)
	if checkConnectionAccess(deps, c, conn, "read") {
		t.Fatal("missing user ID should be denied for restricted connection")
	}
}

func TestCheckConnectionAccess_NonOwnerNotBypassed(t *testing.T) {
	deps := Dependencies{Repos: &Repositories{}}
	tenantID := uuid.New()
	ownerID := uuid.New()
	regularUserID := uuid.New()
	conn := &repository.Connection{
		ID:         uuid.New(),
		TenantID:   tenantID,
		OwnerID:    &ownerID,
		Restricted: true,
	}

	// Non-owner, non-admin user should NOT get owner bypass.
	c := newTestContext(t, tenantID, regularUserID, []string{"developer"})
	// Without ACL repo, this should fail (restricted + no ACL grant).
	if checkConnectionAccess(deps, c, conn, "read") {
		t.Fatal("non-owner should not get owner bypass on restricted connection")
	}
}
