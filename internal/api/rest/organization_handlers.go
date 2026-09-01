package rest

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/internal/repository"
)

// registerOrganizationRoutes registers organization & workspace (tenant) API endpoints.
//
// Multi-workspace model:
//   - One Organization = the company using PEPA
//   - Multiple Tenants (workspaces) = project/team isolation boundaries (e.g. "payments-team", "frontend-app")
//   - Each Workspace has its own Environments (dev, staging, production) as deployment targets
//
// All resources belong to a workspace (tenant_id). The organization groups workspaces.
// Environments are scoped to a workspace and represent deployment targets (cluster/namespace).
func registerOrganizationRoutes(r *gin.RouterGroup, deps Dependencies) {
	// ── Organization (single) ────────────────────────────────
	// The org is created by init-db.sql as "Default Organization".
	// Setup wizard renames it; after that it can be updated from settings.
	org := r.Group("/organization")
	{
		org.GET("", getMyOrganization(deps))
		org.PUT("", requireAdminRole(deps), updateMyOrganization(deps))
	}

	// ── Setup: first-run org naming ──────────────────────────
	setup := r.Group("/setup")
	{
		setup.POST("/organization", setupOrganization(deps))
	}

	// ── Workspaces (tenants) ─────────────────────────────────
	ws := r.Group("/workspaces")
	{
		ws.GET("", listWorkspaces(deps))
		ws.GET("/:id", getWorkspace(deps))
		ws.POST("", requireAdminRole(deps), createWorkspace(deps))
		ws.PUT("/:id", requireAdminRole(deps), updateWorkspace(deps))
		ws.DELETE("/:id", requireAdminRole(deps), deleteWorkspace(deps))
		ws.POST("/:id/switch", switchWorkspace(deps))
		// Workspace member management (cross-workspace user access)
		ws.GET("/:id/members", requireAdminRole(deps), listWorkspaceMembers(deps))
		ws.POST("/:id/members", requireAdminRole(deps), addWorkspaceMember(deps))
		ws.DELETE("/:id/members/:userId", requireAdminRole(deps), removeWorkspaceMember(deps))
	}
}

// ─── Organization ─────────────────────────────────────────────────────────────

// getMyOrganization returns the single organization for this PEPA instance.
func getMyOrganization(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Organization == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "organization repository not available"})
			return
		}

		orgID := uuid.MustParse(database.DefaultOrganizationID)

		org, err := deps.Repos.Organization.GetOrganization(c.Request.Context(), orgID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
			return
		}

		workspaceCount, _ := deps.Repos.Organization.GetWorkspaceCount(c.Request.Context(), orgID)

		c.JSON(http.StatusOK, gin.H{
			"organization": gin.H{
				"id":         org.ID.String(),
				"name":       org.Name,
				"slug":       org.Slug,
				"plan":       org.Plan,
				"created_at": org.CreatedAt,
			},
			"workspace_count": workspaceCount,
		})
	}
}

// updateMyOrganization updates the single organization's name and slug.
func updateMyOrganization(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Organization == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "organization repository not available"})
			return
		}

		orgID := uuid.MustParse(database.DefaultOrganizationID)

		var req struct {
			Name string `json:"name" binding:"required"`
			Slug string `json:"slug" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx := c.Request.Context()
		if err := deps.Repos.Organization.UpdateOrganization(ctx, orgID, req.Name, req.Slug); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "organization", orgID.String(), nil, gin.H{
			"name": req.Name, "slug": req.Slug,
		})

		c.JSON(http.StatusOK, gin.H{"message": "organization updated"})
	}
}

// setupOrganization is called during the first-run setup wizard to name the org.
// It can only be used once — after that, use PUT /organization.
func setupOrganization(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := uuid.MustParse(database.DefaultOrganizationID)

		var req struct {
			Name string `json:"name" binding:"required"`
			Slug string `json:"slug" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx := c.Request.Context()

		// Only allow if still "Default Organization" (first-run)
		var currentName string
		_ = deps.DB.Pool.QueryRow(ctx,
			`SELECT name FROM organizations WHERE id = $1`, orgID).Scan(&currentName)
		if currentName != "Default Organization" {
			c.JSON(http.StatusConflict, gin.H{"error": "organization already configured"})
			return
		}

		_, err := deps.DB.Pool.Exec(ctx, `
			UPDATE organizations SET name = $2, slug = $3
			WHERE id = $1
		`, orgID, req.Name, req.Slug)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "setup", "organization", orgID.String(), nil, gin.H{
			"name": req.Name, "slug": req.Slug,
		})

		c.JSON(http.StatusOK, gin.H{
			"message": "organization configured",
			"name":    req.Name,
			"slug":    req.Slug,
		})
	}
}

// ─── Workspaces (Tenants) ─────────────────────────────────────────────────────

// Workspace is the API representation of a tenant.
type Workspace struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// listWorkspaces returns workspaces (tenants) accessible to the current user.
// Admins and super admins see all workspaces; regular users see only those
// where they have active role assignments.
func listWorkspaces(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Organization == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "organization repository not available"})
			return
		}

		orgID := uuid.MustParse(database.DefaultOrganizationID)
		ctx := c.Request.Context()
		userID := auth.GetUserID(c)

		// Determine if user is super admin or has admin role in any workspace
		isAdmin := false
		if userID != nil {
			isSuperAdmin := *userID == uuid.MustParse(database.SuperAdminUserID)
			if isSuperAdmin {
				isAdmin = true
			} else if deps.RBAC != nil {
				// Check if user has workspace:create permission (admin-level)
				canCreate, _ := deps.RBAC.CheckPermission(ctx, uuid.MustParse(database.DefaultTenantID), *userID, "workspace", "create")
				if canCreate {
					isAdmin = true
				}
			}
		}

		var workspacesList []repository.Workspace
		var err error

		if isAdmin {
			// Admins see all workspaces
			workspacesList, err = deps.Repos.Organization.ListWorkspaces(ctx, orgID)
		} else if userID != nil {
			// Regular users see only workspaces where they have role assignments
			workspacesList, err = deps.Repos.Organization.ListUserWorkspaces(ctx, orgID, *userID)
		} else {
			// No user context — return empty
			c.JSON(http.StatusOK, gin.H{"workspaces": []interface{}{}, "total": 0, "current_workspace": ""})
			return
		}

		if err != nil {
			respondInternalError(c, err)
			return
		}

		type workspaceWithCounts struct {
			Workspace
			Counts map[string]int `json:"counts"`
		}
		workspaces := []workspaceWithCounts{}
		for _, ws := range workspacesList {
			stats, _ := deps.Repos.Organization.GetWorkspaceStats(ctx, ws.ID)
			counts := map[string]int{
				"teams":        stats.Teams,
				"users":        stats.Users,
				"environments": stats.Environments,
				"services":     stats.Services,
			}
			workspaces = append(workspaces, workspaceWithCounts{
				Workspace: Workspace{
					ID:        ws.ID.String(),
					Name:      ws.Name,
					Slug:      ws.Slug,
					CreatedAt: ws.CreatedAt.Format(time.RFC3339),
				},
				Counts: counts,
			})
		}

		// Mark current workspace
		currentTenantID := auth.GetTenantID(c)

		c.JSON(http.StatusOK, gin.H{
			"workspaces":        workspaces,
			"total":             len(workspaces),
			"current_workspace": currentTenantID.String(),
		})
	}
}

// getWorkspace returns a single workspace.
func getWorkspace(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Organization == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "organization repository not available"})
			return
		}

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workspace ID"})
			return
		}

		ctx := c.Request.Context()
		ws, err := deps.Repos.Organization.GetWorkspace(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
			return
		}

		// Count resources in this workspace
		stats, _ := deps.Repos.Organization.GetWorkspaceStats(ctx, id)

		c.JSON(http.StatusOK, gin.H{
			"workspace": Workspace{
				ID:        ws.ID.String(),
				Name:      ws.Name,
				Slug:      ws.Slug,
				CreatedAt: ws.CreatedAt.Format(time.RFC3339),
			},
			"counts": gin.H{
				"services":     stats.Services,
				"connections":  stats.Connections,
				"teams":        stats.Teams,
			},
		})
	}
}

// createWorkspace creates a new workspace (tenant).
func createWorkspace(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Organization == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "organization repository not available"})
			return
		}

		orgID := uuid.MustParse(database.DefaultOrganizationID)

		var req struct {
			Name string `json:"name" binding:"required"`
			Slug string `json:"slug" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		id := uuid.New()
		ctx := c.Request.Context()

		ws := &repository.Workspace{
			ID:             id,
			OrganizationID: orgID,
			Name:           req.Name,
			Slug:           req.Slug,
		}

		if err := deps.Repos.Organization.CreateWorkspaceWithEnvironments(ctx, ws); err != nil {
			respondInternalError(c, err)
			return
		}

		// Seed default RBAC roles for the new workspace (idempotent).
		if deps.RBAC != nil {
			if err := deps.RBAC.SeedDefaultRoles(ctx, id); err != nil {
				slog.Info("createWorkspace: failed to seed RBAC roles", "error", err)
				// Non-fatal: workspace is already created, roles can be seeded later.
			}
		}

		logAudit(deps, c, "create", "workspace", id.String(), nil, gin.H{
			"name": req.Name, "slug": req.Slug,
		})

		c.JSON(http.StatusCreated, gin.H{
			"id":   id,
			"name": req.Name,
			"slug": req.Slug,
		})
	}
}

// updateWorkspace updates a workspace's name and slug.
func updateWorkspace(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Organization == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "organization repository not available"})
			return
		}

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workspace ID"})
			return
		}

		// Prevent renaming the default workspace
		if id.String() == database.DefaultTenantID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot modify the default workspace"})
			return
		}

		var req struct {
			Name string `json:"name" binding:"required"`
			Slug string `json:"slug" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx := c.Request.Context()
		if err := deps.Repos.Organization.UpdateWorkspace(ctx, id, req.Name, req.Slug, nil); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "workspace", id.String(), nil, gin.H{
			"name": req.Name, "slug": req.Slug,
		})

		c.JSON(http.StatusOK, gin.H{"message": "workspace updated"})
	}
}

// deleteWorkspace deletes a workspace (only if empty and not default).
func deleteWorkspace(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Organization == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "organization repository not available"})
			return
		}

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workspace ID"})
			return
		}

		if id.String() == database.DefaultTenantID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the default workspace"})
			return
		}

		ctx := c.Request.Context()

		// Check for resources
		serviceCount, _ := deps.Repos.Organization.GetServiceCount(ctx, id)
		if serviceCount > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete workspace with existing services"})
			return
		}

		if err := deps.Repos.Organization.DeleteWorkspace(ctx, id); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "workspace", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "workspace deleted"})
	}
}

// switchWorkspace issues a new JWT for the requested workspace.
// The user stays the same — only the tenant_id in the token changes.
// Users can only switch to workspaces where they have active role assignments,
// unless they are admins/super admins.
func switchWorkspace(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workspace ID"})
			return
		}

		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		// Re-check that the user is still active.
		var isActive bool
		_ = deps.DB.Pool.QueryRow(c.Request.Context(), `SELECT is_active FROM users WHERE id = $1`, *userID).Scan(&isActive)
		if !isActive {
			c.JSON(http.StatusForbidden, gin.H{"error": "account is disabled"})
			return
		}

		// Verify the workspace exists and belongs to our org
		orgID := uuid.MustParse(database.DefaultOrganizationID)
		var exists int
		_ = deps.DB.Pool.QueryRow(c.Request.Context(), `
			SELECT COUNT(*) FROM tenants WHERE id = $1 AND organization_id = $2
		`, id, orgID).Scan(&exists)
		if exists == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
			return
		}

		// Check if user is super admin
		isSuperAdmin := *userID == uuid.MustParse(database.SuperAdminUserID)

		// Check if user has access to this workspace
		hasAccess := false
		if isSuperAdmin {
			hasAccess = true
		} else if deps.RBAC != nil {
			// Check if user has admin-level permission (workspace:create) — they see all
			canCreate, _ := deps.RBAC.CheckPermission(c.Request.Context(), uuid.MustParse(database.DefaultTenantID), *userID, "workspace", "create")
			if canCreate {
				hasAccess = true
			} else {
				// Check if user has any role assignment in this specific workspace
				var assignmentCount int
				_ = deps.DB.Pool.QueryRow(c.Request.Context(), `
					SELECT COUNT(*) FROM role_assignments
					WHERE tenant_id = $1 AND user_id = $2 AND is_active = true
				`, id, *userID).Scan(&assignmentCount)
				hasAccess = assignmentCount > 0
			}
		}

		if !hasAccess {
			c.JSON(http.StatusForbidden, gin.H{"error": "you do not have access to this workspace"})
			return
		}

		// Get user roles for the new workspace
		var roles []string
		if deps.RBAC != nil {
			assignments, err := deps.RBAC.GetUserRoles(c.Request.Context(), id, *userID)
			if err == nil {
				for _, a := range assignments {
					roles = append(roles, a.RoleName)
				}
			}
		}
		if len(roles) == 0 {
			// Super admin or users with workspace:create permission get admin access
			if isSuperAdmin {
				roles = []string{"admin"}
			} else if deps.RBAC != nil {
				// Check if user has workspace:create in any workspace (org-level permission)
				canCreate, _ := deps.RBAC.CheckPermission(c.Request.Context(), id, *userID, "workspace", "create")
				if canCreate {
					roles = []string{"admin"}
				}
			}
			// No fallback — users without explicit role_assignments get empty roles.
		}

		email := auth.GetEmail(c)

		// Generate new token with the new tenant
		tokenExpiry := deps.Config.Auth.SessionDuration
		if tokenExpiry == 0 {
			tokenExpiry = 8 * time.Hour
		}

		token, err := auth.GenerateToken(
			deps.Config.Auth.JWTSecret,
			*userID, id, orgID,
			email, roles, auth.GetTokenVersion(c), tokenExpiry,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		// Update the session cookie so the browser picks up the new workspace.
		setAuthCookie(c, deps, token, tokenExpiry)

		c.JSON(http.StatusOK, gin.H{
			"token":        token,
			"workspace_id": id.String(),
			"expires_in":   int(tokenExpiry.Seconds()),
		})
	}
}

// ─── Workspace Member Management ─────────────────────────────────────────────
// These endpoints allow admins to grant/revoke user access to specific workspaces,
// enabling one user to have access to multiple workspaces.

// listWorkspaceMembers returns all users with their access status for a specific workspace.
func listWorkspaceMembers(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		wsID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workspace ID"})
			return
		}

		ctx := c.Request.Context()

		// Get all active users
		rows, err := deps.DB.Pool.Query(ctx, `
			SELECT u.id, u.email, u.name,
			       COALESCE(u.last_login_at, '1970-01-01'::timestamptz) as last_login
			FROM users u
			WHERE u.is_active = true
			ORDER BY u.name ASC
		`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
			return
		}
		defer rows.Close()

		type memberInfo struct {
			UserID       uuid.UUID `json:"user_id"`
			Email        string    `json:"email"`
			Name         string    `json:"name"`
			IsSuperAdmin bool      `json:"is_super_admin"`
			HasAccess    bool      `json:"has_access"`
			Roles        []string  `json:"roles"`
		}

		superAdminID := uuid.MustParse(database.SuperAdminUserID)

		var members []memberInfo
		var userIDs []uuid.UUID
		for rows.Next() {
			var m memberInfo
			var lastLogin time.Time
			if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &lastLogin); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scan user"})
				return
			}
			m.IsSuperAdmin = (m.UserID == superAdminID)
			m.Roles = []string{}
			members = append(members, m)
			userIDs = append(userIDs, m.UserID)
		}

		// Check role assignments for all users in this workspace
		if deps.RBAC != nil && len(userIDs) > 0 {
			for i := range members {
				if members[i].IsSuperAdmin {
					members[i].HasAccess = true
					members[i].Roles = []string{"admin"}
					continue
				}
				assignments, err := deps.RBAC.GetUserRoles(ctx, wsID, members[i].UserID)
				if err == nil && len(assignments) > 0 {
					members[i].HasAccess = true
					for _, a := range assignments {
						slug := a.RoleSlug
						if slug == "" {
							slug = a.RoleName
						}
						members[i].Roles = append(members[i].Roles, slug)
					}
				}
			}
		} else {
			for i := range members {
				if members[i].IsSuperAdmin {
					members[i].HasAccess = true
					members[i].Roles = []string{"admin"}
				}
			}
		}

		if members == nil {
			members = []memberInfo{}
		}

		c.JSON(http.StatusOK, gin.H{
			"members": members,
			"total":   len(members),
		})
	}
}

// addWorkspaceMember grants a user access to a workspace by assigning a role.
func addWorkspaceMember(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		wsID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workspace ID"})
			return
		}

		var req struct {
			UserID   uuid.UUID `json:"user_id" binding:"required"`
			RoleSlug string    `json:"role_slug"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
			return
		}

		ctx := c.Request.Context()

		// Verify user exists
		var userEmail string
		err = deps.DB.Pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, req.UserID).Scan(&userEmail)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		// Default role slug
		roleSlug := req.RoleSlug
		if roleSlug == "" {
			roleSlug = "viewer"
		}

		// Look up the role in this workspace
		var roleID uuid.UUID
		err = deps.DB.Pool.QueryRow(ctx, `
			SELECT id FROM roles WHERE tenant_id = $1 AND slug = $2
		`, wsID, roleSlug).Scan(&roleID)
		if err != nil {
			// If role doesn't exist in this workspace, try to seed default roles
			if deps.RBAC != nil {
				_ = deps.RBAC.SeedDefaultRoles(ctx, wsID)
				err = deps.DB.Pool.QueryRow(ctx, `
					SELECT id FROM roles WHERE tenant_id = $1 AND slug = $2
				`, wsID, roleSlug).Scan(&roleID)
			}
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "role not found in this workspace"})
				return
			}
		}

		// Check if user already has access
		var existingCount int
		_ = deps.DB.Pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM role_assignments
			WHERE tenant_id = $1 AND user_id = $2 AND is_active = true
		`, wsID, req.UserID).Scan(&existingCount)

		if existingCount > 0 {
			c.JSON(http.StatusOK, gin.H{"message": "user already has access to this workspace"})
			return
		}

		// Assign the role
		if deps.RBAC != nil {
			_, err = deps.RBAC.AssignRole(ctx, wsID, req.UserID, roleID, auth.GetUserID(c))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to assign role"})
				return
			}
		}

		// Get workspace name for audit
		var wsName string
		_ = deps.DB.Pool.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, wsID).Scan(&wsName)

		logAudit(deps, c, "add_member", "workspace", wsID.String(), nil, gin.H{
			"user_id": req.UserID.String(), "workspace": wsName, "role": roleSlug,
		})

		c.JSON(http.StatusOK, gin.H{
			"message": "user granted access to workspace",
		})
	}
}

// removeWorkspaceMember revokes a user's access from a workspace.
func removeWorkspaceMember(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		wsID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workspace ID"})
			return
		}

		userID, err := uuid.Parse(c.Param("userId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
			return
		}

		// Prevent removing access from super admin
		superAdminID := uuid.MustParse(database.SuperAdminUserID)
		if userID == superAdminID {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot remove super admin access"})
			return
		}

		ctx := c.Request.Context()

		// Revoke all active role assignments for this user in this workspace
		if deps.RBAC != nil {
			assignments, err := deps.RBAC.GetUserRoles(ctx, wsID, userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user roles"})
				return
			}
			for _, a := range assignments {
				_ = deps.RBAC.RevokeAssignment(ctx, a.ID)
			}
		}

		// Get workspace name for audit
		var wsName string
		_ = deps.DB.Pool.QueryRow(ctx, `SELECT name FROM tenants WHERE id = $1`, wsID).Scan(&wsName)

		logAudit(deps, c, "remove_member", "workspace", wsID.String(), nil, gin.H{
			"user_id": userID.String(), "workspace": wsName,
		})

		c.JSON(http.StatusOK, gin.H{
			"message": "user access revoked from workspace",
		})
	}
}
