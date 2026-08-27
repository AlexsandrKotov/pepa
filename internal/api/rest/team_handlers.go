package rest

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
)

// Team represents a team/group with optional parent for hierarchy.
type Team struct {
	ID           uuid.UUID      `json:"id"`
	TenantID     uuid.UUID      `json:"tenant_id"`
	Name         string         `json:"name"`
	Slug         string         `json:"slug"`
	Description  string         `json:"description"`
	ParentTeamID *uuid.UUID     `json:"parent_team_id,omitempty"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
}

// TeamMember represents a user's membership in a team.
type TeamMember struct {
	ID       uuid.UUID `json:"id"`
	TeamID   uuid.UUID `json:"team_id"`
	UserID   uuid.UUID `json:"user_id"`
	Email    string    `json:"email,omitempty"`
	Name     string    `json:"name,omitempty"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// registerTeamRoutes registers team management endpoints.
func registerTeamRoutes(r *gin.RouterGroup, deps Dependencies) {
	teams := r.Group("/teams")
	{
		teams.GET("", listTeams(deps))
		teams.POST("", createTeam(deps))
		teams.GET("/:id", getTeam(deps))
		teams.PUT("/:id", updateTeam(deps))
		teams.DELETE("/:id", deleteTeam(deps))

		// Members
		teams.GET("/:id/members", listTeamMembers(deps))
		teams.POST("/:id/members", addTeamMember(deps))
		teams.DELETE("/:id/members/:userId", removeTeamMember(deps))

		// Team roles
		teams.GET("/:id/roles", getTeamRoles(deps))
		teams.POST("/:id/roles", assignTeamRole(deps))
		teams.DELETE("/:id/roles/:roleId", removeTeamRole(deps))
	}
}

// verifyTeamOwnership checks that the team belongs to the current tenant.
// Returns true if verified, false if the response was already sent (404).
func verifyTeamOwnership(c *gin.Context, deps Dependencies, teamID uuid.UUID) bool {
	tenantID := auth.GetTenantID(c)
	var teamTenantID uuid.UUID
	err := deps.DB.Pool.QueryRow(c.Request.Context(),
		`SELECT tenant_id FROM teams WHERE id = $1`, teamID).Scan(&teamTenantID)
	if err != nil || teamTenantID != tenantID {
		c.JSON(http.StatusNotFound, gin.H{"error": "team not found"})
		return false
	}
	return true
}

// listTeams returns all teams for the current tenant, optionally as a tree.
func listTeams(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		ctx := c.Request.Context()

		rows, err := deps.DB.Pool.Query(ctx, `
			SELECT id, tenant_id, name, slug, COALESCE(description,''),
			       parent_team_id, COALESCE(metadata,'{}'::jsonb), created_at
			FROM teams
			WHERE tenant_id = $1
			ORDER BY parent_team_id NULLS FIRST, name
		`, tenantID)
		if err != nil {
			log.Printf("listTeams: query error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list teams"})
			return
		}
		defer rows.Close()

		var teams []Team
		for rows.Next() {
			var t Team
			var metadataJSON []byte
			if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Slug, &t.Description,
				&t.ParentTeamID, &metadataJSON, &t.CreatedAt); err != nil {
				log.Printf("listTeams: scan error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read team data"})
				return
			}
			teams = append(teams, t)
		}

		if teams == nil {
			teams = []Team{}
		}

		c.JSON(http.StatusOK, gin.H{"teams": teams, "total": len(teams)})
	}
}

// createTeam creates a new team or subgroup.
func createTeam(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)

		var req struct {
			Name         string     `json:"name" binding:"required"`
			Slug         string     `json:"slug" binding:"required"`
			Description  string     `json:"description"`
			ParentTeamID *uuid.UUID `json:"parent_team_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validate parent exists if specified
		if req.ParentTeamID != nil {
			var exists int
			_ = deps.DB.Pool.QueryRow(c.Request.Context(),
				`SELECT COUNT(*) FROM teams WHERE id = $1 AND tenant_id = $2`,
				*req.ParentTeamID, tenantID).Scan(&exists)
			if exists == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "parent team not found"})
				return
			}
		}

		teamID := uuid.New()
		ctx := c.Request.Context()
		_, err := deps.DB.Pool.Exec(ctx, `
			INSERT INTO teams (id, tenant_id, name, slug, description, parent_team_id)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, teamID, tenantID, req.Name, req.Slug, req.Description, req.ParentTeamID)
		if err != nil {
			log.Printf("createTeam: insert error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create team"})
			return
		}

		logAudit(deps, c, "create", "team", teamID.String(), nil, gin.H{"name": req.Name, "slug": req.Slug})

		c.JSON(http.StatusCreated, gin.H{
			"id":   teamID,
			"name": req.Name,
			"slug": req.Slug,
		})
	}
}

// getTeam returns a team with its member count.
func getTeam(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
			return
		}

		if !verifyTeamOwnership(c, deps, id) {
			return
		}

		ctx := c.Request.Context()
		var t Team
		var metadataJSON []byte
		err = deps.DB.Pool.QueryRow(ctx, `
			SELECT id, tenant_id, name, slug, COALESCE(description,''),
			       parent_team_id, COALESCE(metadata,'{}'::jsonb), created_at
			FROM teams WHERE id = $1
		`, id).Scan(&t.ID, &t.TenantID, &t.Name, &t.Slug, &t.Description,
			&t.ParentTeamID, &metadataJSON, &t.CreatedAt)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "team not found"})
			return
		}

		// Get member count
		var memberCount int
		_ = deps.DB.Pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM team_memberships WHERE team_id = $1
		`, id).Scan(&memberCount)

		c.JSON(http.StatusOK, gin.H{
			"team":         t,
			"member_count": memberCount,
		})
	}
}

// updateTeam updates a team's name, description, or parent.
func updateTeam(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
			return
		}

		if !verifyTeamOwnership(c, deps, id) {
			return
		}

		var req struct {
			Name         string     `json:"name"`
			Description  string     `json:"description"`
			ParentTeamID *uuid.UUID `json:"parent_team_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx := c.Request.Context()
		_, err = deps.DB.Pool.Exec(ctx, `
			UPDATE teams SET name = COALESCE($2, name),
			       description = COALESCE($3, description),
			       parent_team_id = $4,
			       updated_at = NOW()
			WHERE id = $1
		`, id, nilIfEmpty(req.Name), nilIfEmpty(req.Description), req.ParentTeamID)
		if err != nil {
			log.Printf("updateTeam: exec error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update team"})
			return
		}

		logAudit(deps, c, "update", "team", id.String(), nil, req)
		c.JSON(http.StatusOK, gin.H{"message": "team updated"})
	}
}

// deleteTeam deletes a team and its memberships.
func deleteTeam(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
			return
		}

		if !verifyTeamOwnership(c, deps, id) {
			return
		}

		ctx := c.Request.Context()
		// Delete memberships first (in case ON DELETE CASCADE is not set)
		if _, err := deps.DB.Pool.Exec(ctx, `DELETE FROM team_memberships WHERE team_id = $1`, id); err != nil {
			log.Printf("deleteTeam: membership delete error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete team memberships"})
			return
		}
		_, err = deps.DB.Pool.Exec(ctx, `DELETE FROM teams WHERE id = $1`, id)
		if err != nil {
			log.Printf("deleteTeam: exec error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete team"})
			return
		}

		logAudit(deps, c, "delete", "team", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "team deleted"})
	}
}

// listTeamMembers returns all members of a team.
func listTeamMembers(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
			return
		}

		if !verifyTeamOwnership(c, deps, id) {
			return
		}

		ctx := c.Request.Context()
		rows, err := deps.DB.Pool.Query(ctx, `
			SELECT tm.id, tm.team_id, tm.user_id, COALESCE(u.email,''), COALESCE(u.name,''),
			       tm.role, tm.joined_at
			FROM team_memberships tm
			JOIN users u ON u.id = tm.user_id
			WHERE tm.team_id = $1
			ORDER BY tm.joined_at
		`, id)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		defer rows.Close()

		var members []TeamMember
		for rows.Next() {
			var m TeamMember
			if err := rows.Scan(&m.ID, &m.TeamID, &m.UserID, &m.Email, &m.Name, &m.Role, &m.JoinedAt); err != nil {
				log.Printf("listTeamMembers: scan error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read member data"})
				return
			}
			members = append(members, m)
		}

		if members == nil {
			members = []TeamMember{}
		}

		c.JSON(http.StatusOK, gin.H{"members": members, "total": len(members)})
	}
}

// addTeamMember adds a user to a team.
func addTeamMember(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		teamID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
			return
		}

		if !verifyTeamOwnership(c, deps, teamID) {
			return
		}

		var req struct {
			UserID string `json:"user_id" binding:"required"`
			Role   string `json:"role"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
			return
		}

		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
			return
		}

		role := req.Role
		if role == "" {
			role = "member"
		}

		ctx := c.Request.Context()
		membershipID := uuid.New()
		_, err = deps.DB.Pool.Exec(ctx, `
			INSERT INTO team_memberships (id, team_id, user_id, role)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (team_id, user_id) DO UPDATE SET role = $4
		`, membershipID, teamID, userID, role)
		if err != nil {
			log.Printf("addTeamMember: insert error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add team member"})
			return
		}

		logAudit(deps, c, "add_member", "team", teamID.String(), nil, gin.H{"user_id": userID})
		c.JSON(http.StatusCreated, gin.H{"message": "member added to team"})
	}
}

// removeTeamMember removes a user from a team.
func removeTeamMember(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		teamID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
			return
		}

		if !verifyTeamOwnership(c, deps, teamID) {
			return
		}

		userID, err := uuid.Parse(c.Param("userId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
			return
		}

		ctx := c.Request.Context()
		_, err = deps.DB.Pool.Exec(ctx, `
			DELETE FROM team_memberships WHERE team_id = $1 AND user_id = $2
		`, teamID, userID)
		if err != nil {
			log.Printf("removeTeamMember: exec error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove team member"})
			return
		}

		logAudit(deps, c, "remove_member", "team", teamID.String(), nil, gin.H{"user_id": userID})
		c.JSON(http.StatusOK, gin.H{"message": "member removed from team"})
	}
}

// getTeamRoles returns roles assigned to a team via role_assignments.
func getTeamRoles(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		teamID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
			return
		}

		if !verifyTeamOwnership(c, deps, teamID) {
			return
		}

		if deps.RBAC == nil {
			c.JSON(http.StatusOK, gin.H{"roles": []interface{}{}})
			return
		}

		ctx := c.Request.Context()
		rows, err := deps.DB.Pool.Query(ctx, `
			SELECT ra.id, ra.role_id, r.name, r.slug
			FROM role_assignments ra
			JOIN roles r ON r.id = ra.role_id
			WHERE ra.team_id = $1 AND ra.is_active = true
		`, teamID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		defer rows.Close()

		var roles []gin.H
		for rows.Next() {
			var assignID, roleID uuid.UUID
			var name, slug string
			if err := rows.Scan(&assignID, &roleID, &name, &slug); err != nil {
				log.Printf("getTeamRoles: scan error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read role data"})
				return
			}
			roles = append(roles, gin.H{
				"assignment_id": assignID,
				"role_id":       roleID,
				"name":          name,
				"slug":          slug,
			})
		}

		if roles == nil {
			roles = []gin.H{}
		}

		c.JSON(http.StatusOK, gin.H{"roles": roles})
	}
}

// assignTeamRole assigns a role to all members of a team.
func assignTeamRole(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		teamID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
			return
		}

		if !verifyTeamOwnership(c, deps, teamID) {
			return
		}

		if deps.RBAC == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RBAC not available"})
			return
		}

		var req struct {
			RoleID string `json:"role_id" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "role_id is required"})
			return
		}

		roleID, err := uuid.Parse(req.RoleID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role_id"})
			return
		}

		tenantID := auth.GetTenantID(c)
		ctx := c.Request.Context()
		userID := auth.GetUserID(c)

		// Insert role assignment with team_id and scope
		assignID := uuid.New()
		_, err = deps.DB.Pool.Exec(ctx, `
			INSERT INTO role_assignments (id, tenant_id, team_id, role_id, scope_type, scope_value, is_active, granted_by)
			VALUES ($1, $2, $3, $4, 'team', $5, true, $6)
		`, assignID, tenantID, teamID, roleID, teamID.String(), userID)
		if err != nil {
			log.Printf("assignTeamRole: insert error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to assign role"})
			return
		}

		logAudit(deps, c, "assign_role", "team", teamID.String(), nil, gin.H{"role_id": roleID})
		c.JSON(http.StatusCreated, gin.H{"message": "role assigned to team"})
	}
}

// removeTeamRole removes a role assignment from a team.
func removeTeamRole(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		teamID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid team ID"})
			return
		}

		if !verifyTeamOwnership(c, deps, teamID) {
			return
		}

		roleID, err := uuid.Parse(c.Param("roleId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role ID"})
			return
		}

		ctx := c.Request.Context()
		_, err = deps.DB.Pool.Exec(ctx, `
			DELETE FROM role_assignments WHERE team_id = $1 AND role_id = $2 AND is_active = true
		`, teamID, roleID)
		if err != nil {
			log.Printf("removeTeamRole: exec error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove role"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "role removed from team"})
	}
}

// nilIfEmpty returns nil if the string is empty, otherwise returns a pointer.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
