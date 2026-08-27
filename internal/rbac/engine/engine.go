package engine

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Role represents a role in the system.
type Role struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	IsSystem    bool      `json:"is_system"`
	Scope       string    `json:"scope"`
}

// Permission represents a permission entry.
type Permission struct {
	ID         uuid.UUID              `json:"id"`
	RoleID     uuid.UUID              `json:"role_id"`
	Resource   string                 `json:"resource"`
	Action     string                 `json:"action"`
	Effect     string                 `json:"effect"`
	Conditions map[string]interface{} `json:"conditions"`
}

// RoleAssignment represents a role assignment to a user or team.
type RoleAssignment struct {
	ID        uuid.UUID  `json:"id"`
	TenantID  uuid.UUID  `json:"tenant_id"`
	UserID    *uuid.UUID `json:"user_id,omitempty"`
	TeamID    *uuid.UUID `json:"team_id,omitempty"`
	RoleID    uuid.UUID  `json:"role_id"`
	RoleName  string     `json:"role_name"`
	RoleSlug  string     `json:"role_slug,omitempty"`
	IsActive  bool       `json:"is_active"`
	GrantedBy *uuid.UUID `json:"granted_by,omitempty"`
	ExpiresAt *string    `json:"expires_at,omitempty"`
	UserEmail string     `json:"user_email,omitempty"`
	UserName  string     `json:"user_name,omitempty"`
	TeamName  string     `json:"team_name,omitempty"`
}

// Engine handles RBAC operations.
type Engine struct {
	db *pgxpool.Pool
}

// New creates a new RBAC engine.
func New(db *pgxpool.Pool) *Engine {
	return &Engine{db: db}
}

// ListRoles returns all roles for a tenant.
func (e *Engine) ListRoles(ctx context.Context, tenantID uuid.UUID) ([]Role, error) {
	rows, err := e.db.Query(ctx, `
		SELECT id, tenant_id, name, slug, COALESCE(description, ''), is_system, scope
		FROM roles
		WHERE tenant_id = $1
		ORDER BY name
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Slug, &r.Description, &r.IsSystem, &r.Scope); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		roles = append(roles, r)
	}
	return roles, nil
}

// CreateRole creates a new role.
func (e *Engine) CreateRole(ctx context.Context, tenantID uuid.UUID, name, slug, description, scope string) (*Role, error) {
	if scope == "" {
		scope = "tenant"
	}
	r := &Role{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Name:        name,
		Slug:        slug,
		Description: description,
		Scope:       scope,
	}

	_, err := e.db.Exec(ctx, `
		INSERT INTO roles (id, tenant_id, name, slug, description, scope)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, r.ID, r.TenantID, r.Name, r.Slug, r.Description, r.Scope)
	if err != nil {
		return nil, fmt.Errorf("create role: %w", err)
	}
	return r, nil
}

// UpdateRole updates a role's name, description, and scope.
func (e *Engine) UpdateRole(ctx context.Context, roleID uuid.UUID, name, description, scope string) error {
	_, err := e.db.Exec(ctx, `
		UPDATE roles SET name = $2, description = $3, scope = $4
		WHERE id = $1
	`, roleID, name, description, scope)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	return nil
}

// DeleteRole deletes a role.
func (e *Engine) DeleteRole(ctx context.Context, roleID uuid.UUID) error {
	// Prevent deletion of system roles.
	var isSystem bool
	err := e.db.QueryRow(ctx, `SELECT is_system FROM roles WHERE id = $1`, roleID).Scan(&isSystem)
	if err != nil {
		return fmt.Errorf("delete role: check system flag: %w", err)
	}
	if isSystem {
		return fmt.Errorf("cannot delete system role")
	}

	_, err = e.db.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	return nil
}

// AddPermission adds a permission to a role.
func (e *Engine) AddPermission(ctx context.Context, roleID uuid.UUID, resource, action, effect string) (*Permission, error) {
	if effect == "" {
		effect = "allow"
	}
	p := &Permission{
		ID:       uuid.New(),
		RoleID:   roleID,
		Resource: resource,
		Action:   action,
		Effect:   effect,
	}

	_, err := e.db.Exec(ctx, `
		INSERT INTO permissions (id, role_id, resource, action, effect)
		VALUES ($1, $2, $3, $4, $5)
	`, p.ID, p.RoleID, p.Resource, p.Action, p.Effect)
	if err != nil {
		return nil, fmt.Errorf("add permission: %w", err)
	}
	return p, nil
}

// RemovePermission removes a permission from a role.
func (e *Engine) RemovePermission(ctx context.Context, permID uuid.UUID) error {
	_, err := e.db.Exec(ctx, `DELETE FROM permissions WHERE id = $1`, permID)
	if err != nil {
		return fmt.Errorf("remove permission: %w", err)
	}
	return nil
}

// GetRolePermissions returns all permissions for a role.
func (e *Engine) GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]Permission, error) {
	rows, err := e.db.Query(ctx, `
		SELECT id, role_id, resource, action, effect
		FROM permissions
		WHERE role_id = $1
	`, roleID)
	if err != nil {
		return nil, fmt.Errorf("get role permissions: %w", err)
	}
	defer rows.Close()

	var perms []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.RoleID, &p.Resource, &p.Action, &p.Effect); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		perms = append(perms, p)
	}
	return perms, nil
}

// AssignRole assigns a role to a user.
func (e *Engine) AssignRole(ctx context.Context, tenantID, userID, roleID uuid.UUID, grantedBy *uuid.UUID) (*RoleAssignment, error) {
	a := &RoleAssignment{
		ID:        uuid.New(),
		TenantID:  tenantID,
		UserID:    &userID,
		RoleID:    roleID,
		IsActive:  true,
		GrantedBy: grantedBy,
	}

	_, err := e.db.Exec(ctx, `
		INSERT INTO role_assignments (id, tenant_id, user_id, role_id, is_active, granted_by)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, a.ID, a.TenantID, a.UserID, a.RoleID, a.IsActive, a.GrantedBy)
	if err != nil {
		return nil, fmt.Errorf("assign role: %w", err)
	}
	return a, nil
}

// AssignTeamRole assigns a role to a team.
func (e *Engine) AssignTeamRole(ctx context.Context, tenantID, teamID, roleID uuid.UUID, grantedBy *uuid.UUID) (*RoleAssignment, error) {
	a := &RoleAssignment{
		ID:        uuid.New(),
		TenantID:  tenantID,
		TeamID:    &teamID,
		RoleID:    roleID,
		IsActive:  true,
		GrantedBy: grantedBy,
	}

	_, err := e.db.Exec(ctx, `
		INSERT INTO role_assignments (id, tenant_id, team_id, role_id, scope_type, scope_value, is_active, granted_by)
		VALUES ($1, $2, $3, $4, 'team', $5, $6, $7)
	`, a.ID, a.TenantID, a.TeamID, a.RoleID, teamID.String(), a.IsActive, a.GrantedBy)
	if err != nil {
		return nil, fmt.Errorf("assign team role: %w", err)
	}
	return a, nil
}

// GetUserRoles returns all roles assigned to a user, including roles inherited
// through team membership. This is the canonical source for "what roles does
// this user have" and is used during login to populate the JWT.
func (e *Engine) GetUserRoles(ctx context.Context, tenantID, userID uuid.UUID) ([]RoleAssignment, error) {
	rows, err := e.db.Query(ctx, `
		SELECT DISTINCT ON (r.id)
		       ra.id, ra.tenant_id, ra.user_id, ra.role_id, r.name, r.slug, ra.is_active,
		       COALESCE(t.name, '') AS team_name
		FROM role_assignments ra
		JOIN roles r ON r.id = ra.role_id
		LEFT JOIN team_memberships tm ON tm.team_id = ra.team_id AND tm.user_id = $2
		LEFT JOIN teams t ON t.id = ra.team_id
		WHERE ra.tenant_id = $1
		  AND ra.is_active = true
		  AND (
		      ra.user_id = $2
		      OR (ra.team_id IS NOT NULL AND tm.user_id IS NOT NULL)
		  )
		ORDER BY r.id, ra.user_id DESC NULLS LAST
	`, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("get user roles: %w", err)
	}
	defer rows.Close()

	var assignments []RoleAssignment
	for rows.Next() {
		var a RoleAssignment
		var teamName string
		if err := rows.Scan(&a.ID, &a.TenantID, &a.UserID, &a.RoleID, &a.RoleName, &a.RoleSlug, &a.IsActive, &teamName); err != nil {
			return nil, fmt.Errorf("scan assignment: %w", err)
		}
		a.TeamName = teamName
		assignments = append(assignments, a)
	}
	return assignments, nil
}

// GetBulkUserRoles returns role assignments for multiple users in a single query.
// Returns a map of userID -> []RoleAssignment.
func (e *Engine) GetBulkUserRoles(ctx context.Context, tenantID uuid.UUID, userIDs []uuid.UUID) (map[uuid.UUID][]RoleAssignment, error) {
	if len(userIDs) == 0 {
		return map[uuid.UUID][]RoleAssignment{}, nil
	}

	rows, err := e.db.Query(ctx, `
		SELECT ra.id, ra.tenant_id, ra.user_id, ra.role_id, r.name, r.slug, ra.is_active,
		       COALESCE(t.name, '') AS team_name,
		       COALESCE(ra.user_id, tm.user_id) AS effective_user_id
		FROM role_assignments ra
		JOIN roles r ON r.id = ra.role_id
		LEFT JOIN team_memberships tm ON tm.team_id = ra.team_id
		LEFT JOIN teams t ON t.id = ra.team_id
		WHERE ra.tenant_id = $1
		  AND ra.is_active = true
		  AND (
		      ra.user_id = ANY($2)
		      OR (ra.team_id IS NOT NULL AND tm.user_id = ANY($2))
		  )
		ORDER BY r.id, COALESCE(ra.user_id, tm.user_id)
	`, tenantID, userIDs)
	if err != nil {
		return nil, fmt.Errorf("get bulk user roles: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]RoleAssignment)
	seen := make(map[uuid.UUID]map[uuid.UUID]bool) // userID -> roleID -> seen
	for rows.Next() {
		var a RoleAssignment
		var teamName string
		var effectiveUserID uuid.UUID
		if err := rows.Scan(&a.ID, &a.TenantID, &a.UserID, &a.RoleID, &a.RoleName, &a.RoleSlug, &a.IsActive, &teamName, &effectiveUserID); err != nil {
			return nil, fmt.Errorf("scan bulk assignment: %w", err)
		}
		a.TeamName = teamName
		// Deduplicate: skip if we've already seen this role for this user
		if seen[effectiveUserID] == nil {
			seen[effectiveUserID] = make(map[uuid.UUID]bool)
		}
		if seen[effectiveUserID][a.RoleID] {
			continue
		}
		seen[effectiveUserID][a.RoleID] = true
		result[effectiveUserID] = append(result[effectiveUserID], a)
	}
	return result, nil
}

// CheckPermission checks if a user has a specific permission.
// It checks both direct user assignments and team-based assignments.
func (e *Engine) CheckPermission(ctx context.Context, tenantID, userID uuid.UUID, resource, action string) (bool, error) {
	// Check direct user assignment
	var count int
	err := e.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM role_assignments ra
		JOIN permissions p ON p.role_id = ra.role_id
		WHERE ra.tenant_id = $1
		  AND ra.user_id = $2
		  AND ra.is_active = true
		  AND p.resource = $3
		  AND p.action = $4
		  AND p.effect = 'allow'
	`, tenantID, userID, resource, action).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check permission: %w", err)
	}
	if count > 0 {
		return true, nil
	}

	// Check via team membership
	err = e.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM role_assignments ra
		JOIN permissions p ON p.role_id = ra.role_id
		JOIN team_memberships tm ON tm.team_id = ra.team_id
		WHERE ra.tenant_id = $1
		  AND tm.user_id = $2
		  AND ra.is_active = true
		  AND p.resource = $3
		  AND p.action = $4
		  AND p.effect = 'allow'
	`, tenantID, userID, resource, action).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check team permission: %w", err)
	}
	return count > 0, nil
}

// ListAllAssignments returns all role assignments (user and team) for a tenant.
func (e *Engine) ListAllAssignments(ctx context.Context, tenantID uuid.UUID) ([]RoleAssignment, error) {
	rows, err := e.db.Query(ctx, `
		SELECT ra.id, ra.tenant_id, ra.user_id, ra.team_id, ra.role_id, r.name, ra.is_active,
		       COALESCE(u.email, '') as user_email, COALESCE(u.name, '') as user_name,
		       COALESCE(t.name, '') as team_name
		FROM role_assignments ra
		JOIN roles r ON r.id = ra.role_id
		LEFT JOIN users u ON u.id = ra.user_id
		LEFT JOIN teams t ON t.id = ra.team_id
		WHERE ra.tenant_id = $1
		ORDER BY ra.created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list assignments: %w", err)
	}
	defer rows.Close()

	var assignments []RoleAssignment
	for rows.Next() {
		var a RoleAssignment
		var userEmail, userName, teamName string
		if err := rows.Scan(&a.ID, &a.TenantID, &a.UserID, &a.TeamID, &a.RoleID, &a.RoleName, &a.IsActive,
			&userEmail, &userName, &teamName); err != nil {
			return nil, fmt.Errorf("scan assignment: %w", err)
		}
		a.UserEmail = userEmail
		a.UserName = userName
		a.TeamName = teamName
		assignments = append(assignments, a)
	}
	return assignments, nil
}

// ListUserAssignments returns all role assignments for a tenant.
func (e *Engine) ListUserAssignments(ctx context.Context, tenantID uuid.UUID) ([]RoleAssignment, error) {
	rows, err := e.db.Query(ctx, `
		SELECT ra.id, ra.tenant_id, ra.user_id, ra.role_id, r.name, ra.is_active
		FROM role_assignments ra
		JOIN roles r ON r.id = ra.role_id
		WHERE ra.tenant_id = $1
		ORDER BY ra.created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list assignments: %w", err)
	}
	defer rows.Close()

	var assignments []RoleAssignment
	for rows.Next() {
		var a RoleAssignment
		if err := rows.Scan(&a.ID, &a.TenantID, &a.UserID, &a.RoleID, &a.RoleName, &a.IsActive); err != nil {
			return nil, fmt.Errorf("scan assignment: %w", err)
		}
		assignments = append(assignments, a)
	}
	return assignments, nil
}

// GetUserPermissions returns all aggregated permissions for a user
// across all assigned roles (direct and team-based), deduplicated.
// Returns a list of "resource:action" strings.
func (e *Engine) GetUserPermissions(ctx context.Context, tenantID, userID uuid.UUID) ([]string, error) {
	rows, err := e.db.Query(ctx, `
		SELECT DISTINCT p.resource, p.action
		FROM permissions p
		JOIN role_assignments ra ON ra.role_id = p.role_id
		WHERE ra.tenant_id = $1
		  AND ra.is_active = true
		  AND p.effect = 'allow'
		  AND (
		      ra.user_id = $2
		      OR ra.team_id IN (
		          SELECT tm.team_id FROM team_memberships tm WHERE tm.user_id = $2
		      )
		  )
	`, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("get user permissions: %w", err)
	}
	defer rows.Close()

	var perms []string
	for rows.Next() {
		var resource, action string
		if err := rows.Scan(&resource, &action); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		perms = append(perms, resource+":"+action)
	}
	return perms, nil
}

// RevokeAssignment deactivates a role assignment.
func (e *Engine) RevokeAssignment(ctx context.Context, assignmentID uuid.UUID) error {
	_, err := e.db.Exec(ctx, `UPDATE role_assignments SET is_active = false WHERE id = $1`, assignmentID)
	if err != nil {
		return fmt.Errorf("revoke assignment: %w", err)
	}
	return nil
}

// SeedDefaultRoles creates default admin and viewer roles if none exist.
func (e *Engine) SeedDefaultRoles(ctx context.Context, tenantID uuid.UUID) error {
	var count int
	err := e.db.QueryRow(ctx, `SELECT COUNT(*) FROM roles WHERE tenant_id = $1`, tenantID).Scan(&count)
	if err != nil {
		return fmt.Errorf("count roles: %w", err)
	}
	if count > 0 {
		return nil // already seeded
	}

	// Create admin role
	adminRole := &Role{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Name:        "Admin",
		Slug:        "admin",
		Description: "Full platform access — manage all resources",
		IsSystem:    true,
		Scope:       "tenant",
	}
	_, err = e.db.Exec(ctx, `
		INSERT INTO roles (id, tenant_id, name, slug, description, is_system, scope)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, adminRole.ID, adminRole.TenantID, adminRole.Name, adminRole.Slug, adminRole.Description, adminRole.IsSystem, adminRole.Scope)
	if err != nil {
		return fmt.Errorf("seed admin role: %w", err)
	}

	// Create viewer role
	viewerRole := &Role{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Name:        "Viewer",
		Slug:        "viewer",
		Description: "Read-only access to all resources",
		IsSystem:    true,
		Scope:       "tenant",
	}
	_, err = e.db.Exec(ctx, `
		INSERT INTO roles (id, tenant_id, name, slug, description, is_system, scope)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, viewerRole.ID, viewerRole.TenantID, viewerRole.Name, viewerRole.Slug, viewerRole.Description, viewerRole.IsSystem, viewerRole.Scope)
	if err != nil {
		return fmt.Errorf("seed viewer role: %w", err)
	}

	// Admin gets all permissions
	// NOTE: resource names must be PLURAL to match the frontend permission checks (e.g. "services", not "service").
	resources := []string{
		"entities", "services", "deployments", "workflows", "clusters", "connections", "scorecards", "plugins", "roles", "audit", "settings", "policies", "vault",
		"pipelines", "gitops", "docker", "helm", "environments", "discovery", "import", "ai", "jira", "credentials", "virtualization", "observability",
	}
	actions := []string{"read", "create", "update", "delete"}
	for _, res := range resources {
		for _, act := range actions {
			_, err = e.db.Exec(ctx, `
				INSERT INTO permissions (id, role_id, resource, action, effect)
				VALUES ($1, $2, $3, $4, $5)
			`, uuid.New(), adminRole.ID, res, act, "allow")
			if err != nil {
				return fmt.Errorf("seed admin permission %s/%s: %w", res, act, err)
			}
		}
	}

	// Viewer gets read-only
	for _, res := range resources {
		_, err = e.db.Exec(ctx, `
			INSERT INTO permissions (id, role_id, resource, action, effect)
			VALUES ($1, $2, $3, $4, $5)
		`, uuid.New(), viewerRole.ID, res, "read", "allow")
		if err != nil {
			return fmt.Errorf("seed viewer permission %s/read: %w", res, err)
		}
	}

	return nil
}

// EnsureBasePermissions guarantees that the core system roles (admin, developer, viewer)
// have the expected base permissions, regardless of whether they were created by
// migrations or by SeedDefaultRoles. This is idempotent and safe to call on every startup.
func (e *Engine) EnsureBasePermissions(ctx context.Context, tenantID uuid.UUID) error {
	// All resources that the frontend checks permissions for (must be PLURAL).
	allResources := []string{
		"entities", "services", "deployments", "workflows", "clusters", "connections",
		"scorecards", "plugins", "roles", "audit", "settings", "policies", "vault",
		"pipelines", "gitops", "docker", "helm", "environments", "discovery", "import",
		"ai", "jira", "credentials", "virtualization", "observability",
	}

	// Find system roles for this tenant.
	rows, err := e.db.Query(ctx, `
		SELECT id, slug FROM roles
		WHERE tenant_id = $1 AND is_system = true
		AND slug IN ('admin', 'developer', 'viewer')
	`, tenantID)
	if err != nil {
		return fmt.Errorf("query system roles: %w", err)
	}
	defer rows.Close()

	type roleInfo struct {
		ID   uuid.UUID
		Slug string
	}
	var roles []roleInfo
	for rows.Next() {
		var r roleInfo
		if err := rows.Scan(&r.ID, &r.Slug); err != nil {
			return fmt.Errorf("scan role: %w", err)
		}
		roles = append(roles, r)
	}

	for _, r := range roles {
		switch r.Slug {
		case "admin":
			// Admin gets full CRUD on all resources
			for _, res := range allResources {
				for _, act := range []string{"create", "read", "update", "delete"} {
					if _, err := e.db.Exec(ctx, `
						INSERT INTO permissions (role_id, resource, action, effect)
						VALUES ($1, $2, $3, 'allow')
						ON CONFLICT DO NOTHING
					`, r.ID, res, act); err != nil {
						return fmt.Errorf("ensure admin permission %s/%s: %w", res, act, err)
					}
				}
			}
		case "developer":
			// Developer gets CRUD on core resources
			devResources := []string{"entities", "services", "deployments", "workflows", "pipelines", "connections", "credentials"}
			for _, res := range devResources {
				for _, act := range []string{"create", "read", "update"} {
					if _, err := e.db.Exec(ctx, `
						INSERT INTO permissions (role_id, resource, action, effect)
						VALUES ($1, $2, $3, 'allow')
						ON CONFLICT DO NOTHING
					`, r.ID, res, act); err != nil {
						return fmt.Errorf("ensure developer permission %s/%s: %w", res, act, err)
					}
				}
			}
			// Read-only on remaining resources
			for _, res := range allResources {
				if _, err := e.db.Exec(ctx, `
					INSERT INTO permissions (role_id, resource, action, effect)
					VALUES ($1, $2, 'read', 'allow')
					ON CONFLICT DO NOTHING
				`, r.ID, res); err != nil {
					return fmt.Errorf("ensure developer read permission %s: %w", res, err)
				}
			}
		case "viewer":
			// Viewer gets read on all resources
			for _, res := range allResources {
				if _, err := e.db.Exec(ctx, `
					INSERT INTO permissions (role_id, resource, action, effect)
					VALUES ($1, $2, 'read', 'allow')
					ON CONFLICT DO NOTHING
				`, r.ID, res); err != nil {
					return fmt.Errorf("ensure viewer read permission %s: %w", res, err)
				}
			}
		}
	}

	return nil
}
