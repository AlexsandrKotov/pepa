package rest

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	rbacengine "github.com/pepa/pepa/internal/rbac/engine"
)

// jwtRolesFromAssignments converts DB role assignments into the role strings that
// go into the JWT "roles" claim.
//
// Every authentication path (login, refresh, LDAP, Google, GitHub, Azure, OIDC,
// bootstrap) needs exactly this conversion, and each one carried its own copy
// that disagreed with the others: some read RoleSlug, some RoleName, and only
// the OAuth ones folded the admin slugs onto the single "admin" string that the
// authorisation layer checks. Normalising here means a system role renamed to
// "Platform Admin" can no longer be recognised by one code path and missed by
// another.
func jwtRolesFromAssignments(assignments []rbacengine.RoleAssignment) []string {
	roles := make([]string, 0, len(assignments))
	for _, a := range assignments {
		slug := a.RoleSlug
		if slug == "" {
			slug = a.RoleName
		}
		if rbacengine.IsReservedRoleSlug(slug) {
			roles = append(roles, "admin")
			continue
		}
		roles = append(roles, slug)
	}
	return roles
}

// userRoleNames returns the effective role list of userID in tenantID.
//
// Failure is deliberately silent-but-empty: no RBAC engine, no assignments or a
// database error all produce "no roles", which every authorisation check treats
// as deny. A fallback to previously issued roles is only ever correct for a
// refresh request (see refreshSession), never for a fresh token.
func userRoleNames(ctx context.Context, deps Dependencies, tenantID, userID uuid.UUID) []string {
	if deps.RBAC == nil {
		return nil
	}
	assignments, err := deps.RBAC.GetUserRoles(ctx, tenantID, userID)
	if err != nil {
		slog.Warn("failed to load role assignments for token", "user_id", userID.String(), "error", err)
		return nil
	}
	return jwtRolesFromAssignments(assignments)
}
