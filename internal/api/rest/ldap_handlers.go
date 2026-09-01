package rest

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/internal/repository"
)

// ldapLoginHandler authenticates a user via LDAP credentials.
func ldapLoginHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !deps.Config.Auth.LDAP.Enabled {
			c.JSON(http.StatusNotFound, gin.H{"error": "LDAP not enabled"})
			return
		}

		var req struct {
			Email    string `json:"email" binding:"required"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "email and password are required"})
			return
		}

		ldapProvider := auth.NewLDAPProvider(deps.Config.Auth.LDAP)
		ldapInfo, err := ldapProvider.Authenticate(c.Request.Context(), req.Email, req.Password)
		if err != nil {
			slog.Info("LDAP authentication failed", "email", req.Email, "error", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
			return
		}

		// Find or create user in local database
		user, err := findOrCreateLDAPUser(c.Request.Context(), deps, ldapInfo)
		if err != nil {
			slog.Error("failed to find/create LDAP user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authenticate user"})
			return
		}

		if !user.IsActive {
			c.JSON(http.StatusForbidden, gin.H{"error": "account is disabled"})
			return
		}

		// Get user roles — start with LDAP group mapping, then add DB roles
		tenantID := uuid.MustParse(database.DefaultTenantID)
		orgID := uuid.MustParse(database.DefaultOrganizationID)
		var roles []string

		// Map LDAP groups to PEPA roles
		if mappedRoles := ldapProvider.MapGroupsToRoles(ldapInfo.Groups); len(mappedRoles) > 0 {
			roles = append(roles, mappedRoles...)
		}

		// Also get roles from the database
		if deps.RBAC != nil {
			assignments, err := deps.RBAC.GetUserRoles(c.Request.Context(), tenantID, user.ID)
			if err == nil {
				for _, a := range assignments {
					slug := a.RoleSlug
					if slug == "" {
						slug = a.RoleName
					}
					roles = append(roles, slug)
				}
			}
		}
		// No hardcoded fallback roles — only explicit role_assignments grant access.

		// Generate JWT
		tokenExpiry := deps.Config.Auth.TokenExpiry
		if tokenExpiry == 0 {
			tokenExpiry = 24 * time.Hour
		}
		token, err := auth.GenerateToken(
			deps.Config.Auth.JWTSecret,
			user.ID, tenantID, orgID,
			user.Email, roles, user.TokenVersion, tokenExpiry,
		)
		if err != nil {
			slog.Error("failed to generate JWT for LDAP user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		setAuthCookie(c, deps, token, tokenExpiry)
		slog.Info("LDAP login successful", "user_id", user.ID, "email", user.Email)

		c.JSON(http.StatusOK, gin.H{
			"token":      token,
			"expires_in": int(tokenExpiry.Seconds()),
			"user": gin.H{
				"id":    user.ID,
				"email": user.Email,
				"name":  user.Name,
				"roles": roles,
			},
		})
	}
}

// ldapConfigHandler returns public LDAP configuration for the frontend.
func ldapConfigHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"enabled": deps.Config.Auth.LDAP.Enabled,
		})
	}
}

// findOrCreateLDAPUser finds an existing user by email or creates a new one from LDAP info.
func findOrCreateLDAPUser(ctx context.Context, deps Dependencies, info *auth.LDAPUserInfo) (*repository.User, error) {
	// Try to find by email
	user, err := deps.Repos.Auth.GetUserByEmail(ctx, info.Email)
	if err == nil {
		// Update auth_provider if it was local (user now authenticates via LDAP)
		if user.Email != "" {
			_ = deps.DB.Pool.QueryRow(ctx, `
				UPDATE users SET auth_provider = 'ldap' WHERE id = $1 AND (auth_provider IS NULL OR auth_provider = 'local')
			`, user.ID)
		}
		return user, nil
	}

	// Create new user
	displayName := info.Name
	if displayName == "" {
		displayName = info.Email
	}

	userID := uuid.New()
	tenantID := uuid.MustParse(database.DefaultTenantID)
	orgID := uuid.MustParse(database.DefaultOrganizationID)

	_, err = deps.DB.Pool.Exec(ctx, `
		INSERT INTO users (id, email, name, is_active, tenant_id, organization_id, token_version, auth_provider, external_id)
		VALUES ($1, $2, $3, true, $4, $5, 0, 'ldap', $6)
	`, userID, info.Email, displayName, tenantID, orgID, info.DN)
	if err != nil {
		return nil, err
	}

	createdUser, err := deps.Repos.Auth.GetUserByEmail(ctx, info.Email)
	if err != nil {
		return nil, err
	}

	// Auto-assign default viewer role so new users have minimal access.
	assignDefaultViewerRole(ctx, deps, userID)

	slog.Info("created new user via LDAP", "user_id", createdUser.ID, "email", createdUser.Email)
	return createdUser, nil
}
