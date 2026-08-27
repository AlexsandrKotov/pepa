package rest

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	rbacengine "github.com/pepa/pepa/internal/rbac/engine"
)

// registerRBACRoutes registers RBAC API endpoints.
func registerRBACRoutes(r *gin.RouterGroup, deps Dependencies) {
	if deps.RBAC == nil {
		return
	}

	// Read-only endpoints are available to all authenticated users.
	r.GET("/roles", listRoles(deps))
	r.GET("/roles/:id/permissions", getRolePermissions(deps))
	r.GET("/role-assignments", listAssignments(deps))

	// Write endpoints require admin role.
	admin := r.Group("")
	admin.Use(requireAdminRole(deps))
	{
		admin.POST("/roles", createRole(deps))
		admin.PUT("/roles/:id", updateRole(deps))
		admin.DELETE("/roles/:id", deleteRole(deps))
		admin.POST("/roles/:id/permissions", addRolePermission(deps))
		admin.DELETE("/roles/:id/permissions/:permId", removeRolePermission(deps))
		admin.POST("/role-assignments", assignRole(deps))
		admin.DELETE("/role-assignments/:id", revokeAssignment(deps))
	}

	// Current user's roles and permission checks
	r.GET("/me/roles", myRoles(deps))
	r.GET("/me/check", checkMyPermission(deps))
	r.GET("/me/permissions", myPermissions(deps))
}

// listRoles returns all roles for the current tenant.
func listRoles(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)

		roles, err := deps.RBAC.ListRoles(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		if roles == nil {
			roles = []rbacengine.Role{}
		}

		c.JSON(http.StatusOK, gin.H{
			"roles": roles,
			"total": len(roles),
		})
	}
}

// createRole creates a new role.
func createRole(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)

		var req struct {
			Name        string `json:"name" binding:"required"`
			Slug        string `json:"slug" binding:"required"`
			Description string `json:"description"`
			Scope       string `json:"scope"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		role, err := deps.RBAC.CreateRole(c.Request.Context(), tenantID, req.Name, req.Slug, req.Description, req.Scope)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "create", "role", role.ID.String(), nil, role)
		c.JSON(http.StatusCreated, role)
	}
}

// updateRole updates a role's name, description, and scope.
func updateRole(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role ID"})
			return
		}

		tenantID := auth.GetTenantID(c)

		// Verify the role belongs to the current tenant.
		var roleTenantID uuid.UUID
		err = deps.DB.Pool.QueryRow(c.Request.Context(), `
			SELECT tenant_id FROM roles WHERE id = $1
		`, id).Scan(&roleTenantID)
		if err != nil || roleTenantID != tenantID {
			c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
			return
		}

		var req struct {
			Name        string `json:"name" binding:"required"`
			Description string `json:"description"`
			Scope       string `json:"scope"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := deps.RBAC.UpdateRole(c.Request.Context(), id, req.Name, req.Description, req.Scope); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "role", id.String(), nil, req)
		c.JSON(http.StatusOK, gin.H{"message": "role updated"})
	}
}

// deleteRole deletes a role by ID.
func deleteRole(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role ID"})
			return
		}

		// Verify the role belongs to the current tenant and is not a system role.
		tenantID := auth.GetTenantID(c)
		var roleTenantID uuid.UUID
		var isSystem bool
		err = deps.DB.Pool.QueryRow(c.Request.Context(), `
			SELECT tenant_id, is_system FROM roles WHERE id = $1
		`, id).Scan(&roleTenantID, &isSystem)
		if err != nil || roleTenantID != tenantID {
			c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
			return
		}
		if isSystem {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete a system role"})
			return
		}

		if err := deps.RBAC.DeleteRole(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "role", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "role deleted"})
	}
}

// verifyRoleOwnership checks that the role belongs to the current tenant.
// Returns true if verified, false if the response was already sent (404).
func verifyRoleOwnership(c *gin.Context, deps Dependencies, roleID uuid.UUID) bool {
	tenantID := auth.GetTenantID(c)
	var roleTenantID uuid.UUID
	err := deps.DB.Pool.QueryRow(c.Request.Context(),
		`SELECT tenant_id FROM roles WHERE id = $1`, roleID).Scan(&roleTenantID)
	if err != nil || roleTenantID != tenantID {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return false
	}
	return true
}

// getRolePermissions returns all permissions for a role.
func getRolePermissions(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role ID"})
			return
		}

		if !verifyRoleOwnership(c, deps, id) {
			return
		}

		perms, err := deps.RBAC.GetRolePermissions(c.Request.Context(), id)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		if perms == nil {
			perms = []rbacengine.Permission{}
		}

		c.JSON(http.StatusOK, gin.H{
			"permissions": perms,
			"total":       len(perms),
		})
	}
}

// addRolePermission adds a permission to a role.
func addRolePermission(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role ID"})
			return
		}

		if !verifyRoleOwnership(c, deps, id) {
			return
		}

		var req struct {
			Resource string `json:"resource" binding:"required"`
			Action   string `json:"action" binding:"required"`
			Effect   string `json:"effect"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		perm, err := deps.RBAC.AddPermission(c.Request.Context(), id, req.Resource, req.Action, req.Effect)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "grant", "permission", id.String(), nil, perm)
		c.JSON(http.StatusCreated, perm)
	}
}

// removeRolePermission removes a permission from a role.
func removeRolePermission(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role ID"})
			return
		}

		if !verifyRoleOwnership(c, deps, roleID) {
			return
		}

		permID, err := uuid.Parse(c.Param("permId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid permission ID"})
			return
		}

		if err := deps.RBAC.RemovePermission(c.Request.Context(), permID); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "revoke", "permission", permID.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "permission removed"})
	}
}

// listAssignments returns all role assignments for the current tenant.
func listAssignments(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)

		assignments, err := deps.RBAC.ListAllAssignments(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		if assignments == nil {
			assignments = []rbacengine.RoleAssignment{}
		}

		c.JSON(http.StatusOK, gin.H{
			"assignments": assignments,
			"total":       len(assignments),
		})
	}
}

// assignRole assigns a role to a user or team (unified endpoint).
// Exactly one of user_id or team_id must be provided.
func assignRole(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		grantedBy := auth.GetUserID(c)

		var req struct {
			UserID *uuid.UUID `json:"user_id"`
			TeamID *uuid.UUID `json:"team_id"`
			RoleID uuid.UUID  `json:"role_id" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Exactly one of user_id or team_id must be set.
		if (req.UserID == nil) == (req.TeamID == nil) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "exactly one of user_id or team_id is required"})
			return
		}

		ctx := c.Request.Context()

		if req.TeamID != nil {
			// Validate the team exists.
			var teamExists int
			_ = deps.DB.Pool.QueryRow(ctx, `
				SELECT COUNT(*) FROM teams WHERE id = $1 AND tenant_id = $2
			`, *req.TeamID, tenantID).Scan(&teamExists)
			if teamExists == 0 {
				c.JSON(http.StatusNotFound, gin.H{"error": "team not found"})
				return
			}

			// Team-based assignment (best practice: assign roles to groups).
			assignment, err := deps.RBAC.AssignTeamRole(ctx, tenantID, *req.TeamID, req.RoleID, grantedBy)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			logAudit(deps, c, "assign", "role", req.RoleID.String(), nil, map[string]string{
				"team_id": req.TeamID.String(),
			})
			c.JSON(http.StatusCreated, assignment)
			return
		}

		// Direct user assignment (exception / override only).
		// Validate the target user exists and is active.
		var targetActive bool
		_ = deps.DB.Pool.QueryRow(ctx, `
			SELECT is_active FROM users WHERE id = $1
		`, *req.UserID).Scan(&targetActive)
		if !targetActive {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target user not found or is disabled"})
			return
		}

		assignment, err := deps.RBAC.AssignRole(ctx, tenantID, *req.UserID, req.RoleID, grantedBy)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "assign", "role", req.RoleID.String(), nil, map[string]string{
			"user_id": req.UserID.String(),
		})
		c.JSON(http.StatusCreated, assignment)
	}
}

// revokeAssignment deactivates a role assignment.
func revokeAssignment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid assignment ID"})
			return
		}

		// Verify the assignment belongs to the current tenant.
		tenantID := auth.GetTenantID(c)
		var assignTenantID uuid.UUID
		err = deps.DB.Pool.QueryRow(c.Request.Context(), `
			SELECT tenant_id FROM role_assignments WHERE id = $1
		`, id).Scan(&assignTenantID)
		if err != nil || assignTenantID != tenantID {
			c.JSON(http.StatusNotFound, gin.H{"error": "assignment not found"})
			return
		}

		if err := deps.RBAC.RevokeAssignment(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "revoke", "role", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "assignment revoked"})
	}
}

// myRoles returns the current user's roles.
func myRoles(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		userID := auth.GetUserID(c)

		if userID == nil {
			// Dev mode: no user in context
			c.JSON(http.StatusOK, gin.H{
				"roles": []string{"admin"},
				"note":  "dev mode — all permissions granted",
			})
			return
		}

		assignments, err := deps.RBAC.GetUserRoles(c.Request.Context(), tenantID, *userID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		roles := make([]string, 0, len(assignments))
		for _, a := range assignments {
			roles = append(roles, a.RoleName)
		}

		c.JSON(http.StatusOK, gin.H{
			"roles":       roles,
			"assignments": assignments,
		})
	}
}

// myPermissions returns all aggregated permissions for the current user.
func myPermissions(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		userID := auth.GetUserID(c)

		if userID == nil {
			// Dev mode: return wildcard admin permissions
			c.JSON(http.StatusOK, gin.H{
				"permissions": []string{"*:*"},
				"note":        "dev mode — all permissions granted",
			})
			return
		}

		perms, err := deps.RBAC.GetUserPermissions(c.Request.Context(), tenantID, *userID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		if perms == nil {
			perms = []string{}
		}

		c.JSON(http.StatusOK, gin.H{
			"permissions": perms,
			"total":       len(perms),
		})
	}
}

// requirePermission returns a middleware that checks if the user has a specific permission.
// If the user lacks the permission, a 403 Forbidden response is returned.
func requirePermission(deps Dependencies, resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			// Dev mode: allow everything
			c.Next()
			return
		}

		tenantID := auth.GetTenantID(c)

		allowed, err := deps.RBAC.CheckPermission(c.Request.Context(), tenantID, *userID, resource, action)
		if err != nil {
			respondInternalError(c, err)
			c.Abort()
			return
		}

		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{
				"error":    "forbidden",
				"message":  "you do not have permission to perform this action",
				"resource": resource,
				"action":   action,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// requireAnyPermission returns a middleware that checks if the user has at least one of the given permissions.
func requireAnyPermission(deps Dependencies, perms ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.Next()
			return
		}

		tenantID := auth.GetTenantID(c)

		for _, perm := range perms {
			parts := strings.SplitN(perm, ":", 2)
			if len(parts) != 2 {
				continue
			}
			allowed, err := deps.RBAC.CheckPermission(c.Request.Context(), tenantID, *userID, parts[0], parts[1])
			if err != nil {
				respondInternalError(c, err)
				c.Abort()
				return
			}
			if allowed {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "you do not have permission to perform this action",
		})
		c.Abort()
	}
}

// checkMyPermission checks if the current user has a specific permission.
func checkMyPermission(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		userID := auth.GetUserID(c)
		resource := c.Query("resource")
		action := c.Query("action")

		if resource == "" || action == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "resource and action query params are required"})
			return
		}

		if userID == nil {
			// Dev mode: allow everything
			c.JSON(http.StatusOK, gin.H{"allowed": true, "reason": "dev mode"})
			return
		}

		allowed, err := deps.RBAC.CheckPermission(c.Request.Context(), tenantID, *userID, resource, action)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"allowed": allowed})
	}
}
