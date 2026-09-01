package rest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/api/rest/dto"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/database"
)

// registerAuthRoutes registers authentication and user management endpoints.
func registerAuthRoutes(r *gin.Engine, deps Dependencies) {
	// Public routes (no JWT required)
	public := r.Group("/api/v1/auth")
	{
		public.POST("/login", loginHandler(deps))
		public.POST("/logout", logoutHandler(deps))
		public.GET("/bootstrap/status", bootstrapStatusHandler(deps))
		public.POST("/bootstrap/activate", bootstrapActivateHandler(deps))
		
		// OIDC routes (public, no JWT required)
		public.GET("/oidc/config", oidcConfigHandler(deps))
		public.GET("/oidc/login", oidcLoginHandler(deps))
		public.GET("/oidc/callback", oidcCallbackHandler(deps))

		// Azure AD routes (public, no JWT required)
		public.GET("/azure/config", azureConfigHandler(deps))
		public.GET("/azure/login", azureLoginHandler(deps))
		public.GET("/azure/callback", azureCallbackHandler(deps))

		// Google OAuth routes (public, no JWT required)
		public.GET("/google/config", googleConfigHandler(deps))
		public.GET("/google/login", googleLoginHandler(deps))
		public.GET("/google/callback", googleCallbackHandler(deps))

		// GitHub OAuth routes (public, no JWT required)
		public.GET("/github/config", githubConfigHandler(deps))
		public.GET("/github/login", githubLoginHandler(deps))
		public.GET("/github/callback", githubCallbackHandler(deps))

		// LDAP routes (public, no JWT required)
		public.POST("/ldap/login", ldapLoginHandler(deps))
		public.GET("/ldap/config", ldapConfigHandler(deps))
	}

	// Protected routes (JWT required)
	protected := r.Group("/api/v1/auth")
	protected.Use(auth.Middleware(deps.Config.Auth.JWTSecret))
	{
		protected.POST("/refresh", refreshTokenHandler(deps))
		protected.GET("/me", getMeHandler(deps))
		protected.POST("/me/reset-password", resetMyPasswordHandler(deps))

		// User management (admin only)
		admin := protected.Group("/users")
		admin.Use(requireAdminRole(deps))
		{
			admin.GET("", listUsersHandler(deps))
			admin.POST("", createUserHandler(deps))
			admin.GET("/:id", getUserHandler(deps))
			admin.PUT("/:id", updateUserHandler(deps))
			admin.DELETE("/:id", deactivateUserHandler(deps))
			admin.POST("/:id/reset-password", resetUserPasswordHandler(deps))
		}
	}
}

// secureCookie reports whether the auth cookie should carry the Secure flag.
// Only set in production AND when the request actually arrived over HTTPS
// (directly or via a reverse proxy). A Secure cookie served over plain HTTP
// is silently dropped by browsers, which breaks login entirely.
func secureCookie(c *gin.Context, deps Dependencies) bool {
	if deps.Config == nil || deps.Config.Server.Env != "production" {
		return false
	}
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

// setAuthCookie stores the JWT in an httpOnly cookie so it is not readable
// from JavaScript (XSS-resistant).
func setAuthCookie(c *gin.Context, deps Dependencies, token string, expiry time.Duration) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.CookieTokenName, token, int(expiry.Seconds()), "/", "", secureCookie(c, deps), true)
}

// clearAuthCookie removes the auth cookie.
func clearAuthCookie(c *gin.Context, deps Dependencies) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.CookieTokenName, "", -1, "/", "", secureCookie(c, deps), true)
}

// logoutHandler clears the auth cookie. Stateless JWTs remain valid until
// expiry, but cannot be refreshed once the cookie is gone.
func logoutHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		clearAuthCookie(c, deps)
		c.JSON(http.StatusOK, gin.H{"message": "logged out"})
	}
}

// requireAdminRole is a middleware that checks if the user has the admin role.
func requireAdminRole(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		roles := auth.GetRoles(c)
		isAdmin := false
		for _, r := range roles {
			lower := strings.ToLower(r)
			// Match by slug ("admin", "super_admin") or by legacy role name ("platform admin").
			if lower == "admin" || lower == "super_admin" || lower == "platform admin" || lower == "platform_admin" {
				isAdmin = true
				break
			}
		}
		if !isAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// loginHandler authenticates a user with email and password, returning a JWT.
func loginHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, ok := bindAndValidate[dto.LoginRequest](c)
		if !ok {
			return
		}

		email := strings.ToLower(strings.TrimSpace(req.Email))

		// Rate limiting: check by both email and client IP.
		clientIP := extractClientIP(c)
		if deps.LoginLimiter != nil {
			// Check email-based rate limit.
			if allowed, retryAfter := deps.LoginLimiter.Allow(email); !allowed {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":       "too many failed login attempts, try again later",
					"retry_after": int(retryAfter.Seconds()),
				})
				return
			}
			// Check IP-based rate limit.
			if allowed, retryAfter := deps.LoginLimiter.Allow(clientIP); !allowed {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":       "too many login attempts from this IP, try again later",
					"retry_after": int(retryAfter.Seconds()),
				})
				return
			}
		}

		// Look up user by email.
		ctx := c.Request.Context()
		var user struct {
			ID                 uuid.UUID
			Email              string
			Name               string
			PasswordHash       string
			IsActive           bool
			MustChangePassword bool
			TokenVersion       int
			TenantID           uuid.UUID
			OrgID              uuid.UUID
		}

		userModel, err := deps.Repos.Auth.GetUserByEmail(ctx, email)
		if err != nil {
			// Record failure for both email and IP.
			if deps.LoginLimiter != nil {
				deps.LoginLimiter.RecordFailure(email)
				deps.LoginLimiter.RecordFailure(clientIP)
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
			return
		}

		// Map repository model to local user struct
		user.ID = userModel.ID
		user.Email = userModel.Email
		user.Name = userModel.Name
		user.PasswordHash = userModel.PasswordHash
		user.IsActive = userModel.IsActive
		user.TokenVersion = userModel.TokenVersion

		// Check must_change_password from database
		mustChangePassword, _ := deps.Repos.Auth.GetMustChangePassword(ctx, user.ID)
		user.MustChangePassword = mustChangePassword

		// Use default tenant/org.
		user.TenantID = uuid.MustParse(database.DefaultTenantID)
		user.OrgID = uuid.MustParse(database.DefaultOrganizationID)

		if !user.IsActive {
			c.JSON(http.StatusForbidden, gin.H{"error": "account is disabled"})
			return
		}

		if user.PasswordHash == "" {
			if deps.LoginLimiter != nil {
				deps.LoginLimiter.RecordFailure(email)
				deps.LoginLimiter.RecordFailure(clientIP)
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
			return
		}

		if err := auth.CheckPassword(user.PasswordHash, req.Password); err != nil {
			if deps.LoginLimiter != nil {
				deps.LoginLimiter.RecordFailure(email)
				deps.LoginLimiter.RecordFailure(clientIP)
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
			return
		}

		// Success: clear rate limit counters.
		if deps.LoginLimiter != nil {
			deps.LoginLimiter.RecordSuccess(email)
			deps.LoginLimiter.RecordSuccess(clientIP)
		}

		// Get user roles
		var roles []string
		if deps.RBAC != nil {
			assignments, err := deps.RBAC.GetUserRoles(ctx, user.TenantID, user.ID)
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
		// No hardcoded fallback — only explicit role_assignments grant access.

		// Generate JWT
		tokenExpiry := deps.Config.Auth.SessionDuration
		if tokenExpiry == 0 {
			tokenExpiry = 24 * time.Hour // fallback: 24 hours
		}
		token, err := auth.GenerateToken(
			deps.Config.Auth.JWTSecret,
			user.ID, user.TenantID, user.OrgID,
			user.Email, roles, user.TokenVersion, tokenExpiry,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		// Update last_login_at
		if err := deps.Repos.Auth.UpdateLastLogin(ctx, user.ID); err != nil {
			slog.Info("failed to update last_login_at for user ", "id", user.ID, "error", err)
		}

		logAudit(deps, c, "login", "user", user.ID.String(), nil, gin.H{"email": user.Email})

		// Set the httpOnly session cookie in addition to returning the token.
		setAuthCookie(c, deps, token, tokenExpiry)

		c.JSON(http.StatusOK, gin.H{
			"token":      token,
			"expires_in": int(tokenExpiry.Seconds()),
			"user": gin.H{
				"id":    user.ID,
				"email": user.Email,
				"name":  user.Name,
				"roles": roles,
			},
			"must_change_password": user.MustChangePassword,
		})
	}
}

// refreshTokenHandler issues a new JWT if the current one is still valid.
// It re-validates that the user is still active, re-fetches roles from the
// database (so role changes take effect without re-login), and verifies the
// token_version claim (so revoked sessions cannot be refreshed).
func refreshTokenHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		ctx := c.Request.Context()

		// Re-check user state from the database (catches deactivated accounts
		// and revoked token versions).
		isActive, tokenVersion, email, err := deps.Repos.Auth.GetUserAuthInfo(ctx, *userID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}
		if !isActive {
			c.JSON(http.StatusForbidden, gin.H{"error": "account is disabled"})
			return
		}
		// Reject tokens issued before the last revocation event
		// (password change, deactivation, role change).
		if auth.GetTokenVersion(c) != tokenVersion {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired, please log in again"})
			return
		}

		tenantID := auth.GetTenantID(c)
		orgID := auth.GetOrgID(c)

		// Re-fetch roles from the database so role changes apply on refresh.
		roles := auth.GetRoles(c)
		if deps.RBAC != nil {
			if assignments, err := deps.RBAC.GetUserRoles(ctx, tenantID, *userID); err == nil {
				fresh := make([]string, 0, len(assignments))
				for _, a := range assignments {
					slug := a.RoleSlug
					if slug == "" {
						slug = a.RoleName
					}
					fresh = append(fresh, slug)
				}
				if len(fresh) > 0 {
					roles = fresh
				}
			}
		}
		// No hardcoded fallback — only explicit role_assignments grant access.

		tokenExpiry := deps.Config.Auth.SessionDuration
		if tokenExpiry == 0 {
			tokenExpiry = 24 * time.Hour // fallback: 24 hours
		}

		token, err := auth.GenerateToken(
			deps.Config.Auth.JWTSecret,
			*userID, tenantID, orgID, email, roles, tokenVersion, tokenExpiry,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		setAuthCookie(c, deps, token, tokenExpiry)

		c.JSON(http.StatusOK, gin.H{
			"token":      token,
			"expires_in": int(tokenExpiry.Seconds()),
		})
	}
}

// getMeHandler returns the current authenticated user's info.
func getMeHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		ctx := c.Request.Context()
		var user struct {
			ID                 uuid.UUID `json:"id"`
			Email              string    `json:"email"`
			Name               string    `json:"name"`
			IsActive           bool      `json:"is_active"`
			MustChangePassword bool      `json:"must_change_password"`
		}

		userModel, err := deps.Repos.Auth.GetUserByID(ctx, *userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		user.ID = userModel.ID
		user.Email = userModel.Email
		user.Name = userModel.Name
		user.IsActive = userModel.IsActive
		user.MustChangePassword, _ = deps.Repos.Auth.GetMustChangePassword(ctx, *userID)

		roles := auth.GetRoles(c)

		// Fetch aggregated permissions for the user
		var permissions []string
		if deps.RBAC != nil {
			tenantID := auth.GetTenantID(c)
			permissions, err = deps.RBAC.GetUserPermissions(ctx, tenantID, *userID)
			if err != nil {
				// Non-fatal: log but still return user info
				permissions = []string{}
			}
		}
		if permissions == nil {
			permissions = []string{}
		}

		c.JSON(http.StatusOK, gin.H{
			"user":        user,
			"roles":       roles,
			"permissions": permissions,
		})
	}
}

// resetMyPasswordHandler lets the current user change their own password.
// If must_change_password is true (bootstrap flow), current_password is not required.
func resetMyPasswordHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		var req struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "new_password is required"})
			return
		}

		if err := auth.ValidatePasswordStrength(req.NewPassword); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx := c.Request.Context()

		// Check if user must change password (bootstrap flow)
		mustChange, _ := deps.Repos.Auth.GetMustChangePassword(ctx, *userID)

		if !mustChange {
			// Normal flow: require and verify current password
			if req.CurrentPassword == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "current_password is required"})
				return
			}

			userModel, err := deps.Repos.Auth.GetUserByID(ctx, *userID)
			currentHash := ""
			if err == nil {
				currentHash = userModel.PasswordHash
			}
			if currentHash == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "cannot verify current password"})
				return
			}

			if err := auth.CheckPassword(currentHash, req.CurrentPassword); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "current password is incorrect"})
				return
			}
		}

		// Hash and save new password
		newHash, err := auth.HashPassword(req.NewPassword, auth.DefaultBCryptCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}

		// Update password
		if err := deps.Repos.Auth.UpdatePassword(ctx, *userID, newHash); err != nil {
			slog.Info("resetMyPassword: update error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update password"})
			return
		}

		// Revoke all outstanding sessions (including other devices).
		bumpTokenVersion(deps, ctx, *userID)

		// Return updated user info so the frontend can refresh its stored state.
		var updatedUser struct {
			ID    uuid.UUID `json:"id"`
			Email string    `json:"email"`
			Name  string    `json:"name"`
		}
		if userModel, err := deps.Repos.Auth.GetUserByID(ctx, *userID); err == nil {
			updatedUser.ID = userModel.ID
			updatedUser.Email = userModel.Email
			updatedUser.Name = userModel.Name
		}

		roles := auth.GetRoles(c)

		// Re-issue a session with the new token version so the caller stays
		// logged in — their previous cookie was just revoked by the bump.
		newVersion, _ := deps.Repos.Auth.GetTokenVersion(ctx, *userID)
		tokenExpiry := deps.Config.Auth.SessionDuration
		if tokenExpiry == 0 {
			tokenExpiry = 24 * time.Hour // fallback: 24 hours
		}
		if token, err := auth.GenerateToken(
			deps.Config.Auth.JWTSecret,
			*userID, auth.GetTenantID(c), auth.GetOrgID(c),
			updatedUser.Email, roles, newVersion, tokenExpiry,
		); err == nil {
			setAuthCookie(c, deps, token, tokenExpiry)
		}

		logAudit(deps, c, "reset_password", "user", userID.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{
			"message": "password updated successfully",
			"user":    updatedUser,
			"roles":   roles,
		})
	}
}

// listUsersHandler returns all users (admin only).
func listUsersHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		search := c.Query("search")
		tenantID := auth.GetTenantID(c)

		query := `
			SELECT DISTINCT u.id, u.email, u.name, u.is_active, u.auth_provider,
			       u.last_login_at, u.created_at
			FROM users u
			LEFT JOIN role_assignments ra ON ra.user_id = u.id AND ra.tenant_id = $1
		`
		args := []interface{}{tenantID}
		whereClauses := []string{}
		argIdx := 2
		if search != "" {
			escaped := strings.ReplaceAll(strings.ReplaceAll(search, "\\", "\\\\"), "%", "\\%")
			escaped = strings.ReplaceAll(escaped, "_", "\\_")
			whereClauses = append(whereClauses, fmt.Sprintf("(u.email ILIKE $%d OR u.name ILIKE $%d)", argIdx, argIdx))
			args = append(args, "%"+escaped+"%")
		}
		if len(whereClauses) > 0 {
			query += " WHERE " + strings.Join(whereClauses, " AND ")
		}
		query += ` ORDER BY u.created_at DESC LIMIT 100`

		rows, err := deps.DB.Pool.Query(ctx, query, args...)
		if err != nil {
			slog.Info("listUsers: query error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
			return
		}
		defer rows.Close()

		type userInfo struct {
			ID           uuid.UUID  `json:"id"`
			Email        string     `json:"email"`
			Name         string     `json:"name"`
			IsActive     bool       `json:"is_active"`
			AuthProvider string     `json:"auth_provider"`
			IsSuperAdmin bool       `json:"is_super_admin"`
			Roles        []string   `json:"roles"`
			LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
			CreatedAt    time.Time  `json:"created_at"`
		}

		superAdminID := uuid.MustParse(database.SuperAdminUserID)
		var users []userInfo
		var userIDs []uuid.UUID
		for rows.Next() {
			var u userInfo
			if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.IsActive, &u.AuthProvider, &u.LastLoginAt, &u.CreatedAt); err != nil {
				slog.Info("listUsers: scan error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read user data"})
				return
			}
			u.IsSuperAdmin = (u.ID == superAdminID)
			u.Roles = []string{}
			userIDs = append(userIDs, u.ID)
			users = append(users, u)
		}

		// Batch-fetch roles for all users in a single query (avoids N+1).
		if deps.RBAC != nil && len(userIDs) > 0 {
			bulkRoles, err := deps.RBAC.GetBulkUserRoles(ctx, tenantID, userIDs)
			if err != nil {
				slog.Info("listUsers: bulk role fetch error", "error", err)
			} else {
				for i := range users {
					if assignments, ok := bulkRoles[users[i].ID]; ok {
						for _, a := range assignments {
							slug := a.RoleSlug
							if slug == "" {
								slug = a.RoleName
							}
							users[i].Roles = append(users[i].Roles, slug)
						}
					}
				}
			}
		}

		if users == nil {
			users = []userInfo{}
		}

		c.JSON(http.StatusOK, gin.H{"users": users, "total": len(users)})
	}
}

// createUserHandler creates a new user account (admin only).
func createUserHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, ok := bindAndValidate[dto.CreateUserRequest](c)
		if !ok {
			return
		}

		if err := auth.ValidatePasswordStrength(req.Password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		email := strings.ToLower(strings.TrimSpace(req.Email))
		ctx := c.Request.Context()

		// Check if user already exists
		exists, _ := deps.Repos.Auth.UserExistsByEmail(ctx, email)
		if exists {
			c.JSON(http.StatusConflict, gin.H{"error": "user with this email already exists"})
			return
		}

		// Hash password
		hash, err := auth.HashPassword(req.Password, auth.DefaultBCryptCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}

		userID := uuid.New()
		if err := deps.Repos.Auth.CreateUser(ctx, userID, email, req.Name, hash); err != nil {
			slog.Info("createUser: insert error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
			return
		}

		// Assign roles if provided
		tenantID := auth.GetTenantID(c)
		if len(req.Roles) > 0 && deps.RBAC != nil {
			for _, roleName := range req.Roles {
				// Look up role by slug
				var roleID uuid.UUID
				err := deps.DB.Pool.QueryRow(ctx, `
					SELECT id FROM roles WHERE tenant_id = $1 AND slug = $2
				`, tenantID, roleName).Scan(&roleID)
				if err == nil {
					_, _ = deps.RBAC.AssignRole(ctx, tenantID, userID, roleID, auth.GetUserID(c))
				}
			}
		} else {
			// No roles specified: auto-assign default viewer role
			assignDefaultViewerRole(ctx, deps, userID)
		}

		logAudit(deps, c, "create", "user", userID.String(), nil, gin.H{"email": email, "name": req.Name})

		c.JSON(http.StatusCreated, gin.H{
			"id":    userID,
			"email": email,
			"name":  req.Name,
		})
	}
}

// verifyUserInTenant checks that the target user has role assignments in the current tenant.
// Returns true if verified, false if the response was already sent (404).
func verifyUserInTenant(c *gin.Context, deps Dependencies, userID uuid.UUID) bool {
	tenantID := auth.GetTenantID(c)
	var inTenant int
	_ = deps.DB.Pool.QueryRow(c.Request.Context(), `
		SELECT COUNT(*) FROM role_assignments WHERE user_id = $1 AND tenant_id = $2
	`, userID, tenantID).Scan(&inTenant)
	if inTenant == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found in this workspace"})
		return false
	}
	return true
}

// getUserHandler returns a single user by ID.
func getUserHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
			return
		}

		if !verifyUserInTenant(c, deps, id) {
			return
		}

		ctx := c.Request.Context()
		var user struct {
			ID           uuid.UUID  `json:"id"`
			Email        string     `json:"email"`
			Name         string     `json:"name"`
			IsActive     bool       `json:"is_active"`
			AuthProvider string     `json:"auth_provider"`
			LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
			CreatedAt    time.Time  `json:"created_at"`
		}

		err = deps.DB.Pool.QueryRow(ctx, `
			SELECT id, email, name, is_active, COALESCE(auth_provider,'local'), last_login_at, created_at
			FROM users WHERE id = $1
		`, id).Scan(&user.ID, &user.Email, &user.Name, &user.IsActive, &user.AuthProvider, &user.LastLoginAt, &user.CreatedAt)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		// Get user roles
		var roles []string
		if deps.RBAC != nil {
			tenantID := auth.GetTenantID(c)
			assignments, err := deps.RBAC.GetUserRoles(ctx, tenantID, user.ID)
			if err == nil {
				for _, a := range assignments {
					roles = append(roles, a.RoleName)
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"user":           user,
			"roles":          roles,
			"is_super_admin": user.ID == uuid.MustParse(database.SuperAdminUserID),
		})
	}
}

// updateUserHandler updates a user's name, email, or active status.
func updateUserHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
			return
		}

		// Protect the super admin account from modification.
		superAdminID := uuid.MustParse(database.SuperAdminUserID)
		if id == superAdminID {
			c.JSON(http.StatusForbidden, gin.H{"error": "the super admin account cannot be modified"})
			return
		}

		if !verifyUserInTenant(c, deps, id) {
			return
		}

		var req struct {
			Name     string   `json:"name"`
			Email    string   `json:"email"`
			IsActive *bool    `json:"is_active"`
			Roles    []string `json:"roles"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		ctx := c.Request.Context()

		// Build dynamic update
		setClauses := []string{"updated_at = NOW()"}
		args := []interface{}{}
		argIdx := 1

		if req.Name != "" {
			setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
			args = append(args, req.Name)
			argIdx++
		}
		if req.Email != "" {
			newEmail := strings.ToLower(strings.TrimSpace(req.Email))
			// Basic email format validation.
			if !strings.Contains(newEmail, "@") || !strings.Contains(newEmail, ".") || len(newEmail) < 5 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email format"})
				return
			}
			// Check that the new email is not already taken by another user.
			emailOwnerID, err := deps.Repos.Auth.GetUserIDByEmail(ctx, newEmail)
			if err == nil && emailOwnerID != id {
				c.JSON(http.StatusConflict, gin.H{"error": "email is already in use by another user"})
				return
			}
			setClauses = append(setClauses, fmt.Sprintf("email = $%d", argIdx))
			args = append(args, newEmail)
			argIdx++
		}
		if req.IsActive != nil {
			setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", argIdx))
			args = append(args, *req.IsActive)
			argIdx++
		}

		if len(setClauses) > 1 {
			query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d",
				strings.Join(setClauses, ", "), argIdx)
			args = append(args, id)
			_, err := deps.DB.Pool.Exec(ctx, query, args...)
			if err != nil {
				slog.Info("updateUser: exec error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
				return
			}
		}

		// Update roles if provided — atomically (revoke + assign in one
		// transaction so the user is never left without roles on failure).
		if req.Roles != nil && deps.RBAC != nil {
			tenantID := auth.GetTenantID(c)

			tx, err := deps.DB.Pool.Begin(ctx)
			if err != nil {
				slog.Info("updateUser: begin tx error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update roles"})
				return
			}
			defer func() { _ = tx.Rollback(ctx) }()

			// Revoke existing assignments
			if _, err := tx.Exec(ctx, `
				UPDATE role_assignments SET is_active = false
				WHERE tenant_id = $1 AND user_id = $2 AND is_active = true
			`, tenantID, id); err != nil {
				slog.Info("updateUser: revoke error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update roles"})
				return
			}

			// Assign new roles (unknown slugs are collected for feedback)
			var unknown []string
			for _, roleName := range req.Roles {
				var roleID uuid.UUID
				err := tx.QueryRow(ctx, `
					SELECT id FROM roles WHERE tenant_id = $1 AND slug = $2
				`, tenantID, roleName).Scan(&roleID)
				if err != nil {
					unknown = append(unknown, roleName)
					continue
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO role_assignments (id, tenant_id, user_id, role_id, is_active, granted_by)
					VALUES ($1, $2, $3, $4, true, $5)
				`, uuid.New(), tenantID, id, roleID, auth.GetUserID(c)); err != nil {
					slog.Info("updateUser: assign error", "error", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update roles"})
					return
				}
			}

			if err := tx.Commit(ctx); err != nil {
				slog.Info("updateUser: commit error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update roles"})
				return
			}

			// Role change invalidates outstanding sessions.
			bumpTokenVersion(deps, ctx, id)

			if len(unknown) > 0 {
				logAudit(deps, c, "update", "user", id.String(), nil, req)
				c.JSON(http.StatusOK, gin.H{
					"message":       "user updated",
					"unknown_roles": unknown,
				})
				return
			}
		} else if req.IsActive != nil {
			// Status change invalidates outstanding sessions when disabling.
			if !*req.IsActive {
				bumpTokenVersion(deps, ctx, id)
			}
		}

		logAudit(deps, c, "update", "user", id.String(), nil, req)
		c.JSON(http.StatusOK, gin.H{"message": "user updated"})
	}
}

// deactivateUserHandler soft-deletes a user by setting is_active = false.
func deactivateUserHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
			return
		}

		// Prevent deactivating yourself.
		currentUser := auth.GetUserID(c)
		if currentUser != nil && *currentUser == id {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot deactivate your own account"})
			return
		}

		// Protect the super admin account from deactivation.
		superAdminID := uuid.MustParse(database.SuperAdminUserID)
		if id == superAdminID {
			c.JSON(http.StatusForbidden, gin.H{"error": "the super admin account cannot be deactivated"})
			return
		}

		if !verifyUserInTenant(c, deps, id) {
			return
		}

		ctx := c.Request.Context()

		// Prevent deactivating the last admin user.
		if deps.RBAC != nil {
			tenantID := auth.GetTenantID(c)
			targetRoles, err := deps.RBAC.GetUserRoles(ctx, tenantID, id)
			if err == nil {
				targetIsAdmin := false
				for _, r := range targetRoles {
					name := strings.ToLower(r.RoleName)
					if name == "admin" || name == "super_admin" {
						targetIsAdmin = true
						break
					}
				}
				if targetIsAdmin {
					// Count total active admins in this tenant.
					var adminCount int
					err = deps.DB.Pool.QueryRow(ctx, `
						SELECT COUNT(*) FROM role_assignments ra
						JOIN roles r ON r.id = ra.role_id
						JOIN users u ON u.id = ra.user_id
						WHERE r.slug = 'admin' AND ra.is_active = true AND u.is_active = true
						  AND ra.tenant_id = $1
					`, tenantID).Scan(&adminCount)
					if err == nil && adminCount <= 1 {
						c.JSON(http.StatusBadRequest, gin.H{"error": "cannot deactivate the last admin user"})
						return
					}
				}
			}
		}

		if err := deps.Repos.Auth.DeactivateUser(ctx, id); err != nil {
			slog.Info("deactivateUser: update error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to deactivate user"})
			return
		}

		// Revoke all sessions of the deactivated user.
		bumpTokenVersion(deps, ctx, id)

		logAudit(deps, c, "deactivate", "user", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "user deactivated"})
	}
}

// extractClientIP returns the client IP from the request.
// It delegates to Gin's c.ClientIP() which respects the trusted proxy
// configuration set via r.SetTrustedProxies(). This prevents IP spoofing
// via fake X-Forwarded-For headers from untrusted sources.
func extractClientIP(c *gin.Context) string {
	ip := c.ClientIP()
	if ip == "" {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(ip)
	if err != nil {
		return ip
	}
	return host
}

// resetUserPasswordHandler lets an admin reset another user's password.
func resetUserPasswordHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
			return
		}

		// Protect the super admin account from password reset by others.
		superAdminID := uuid.MustParse(database.SuperAdminUserID)
		if id == superAdminID {
			c.JSON(http.StatusForbidden, gin.H{"error": "the super admin password can only be changed by the super admin"})
			return
		}

		var req struct {
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password is required"})
			return
		}

		if err := auth.ValidatePasswordStrength(req.Password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		hash, err := auth.HashPassword(req.Password, auth.DefaultBCryptCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}

		ctx := c.Request.Context()
		if err := deps.Repos.Auth.ResetUserPassword(ctx, id, hash); err != nil {
			slog.Info("resetUserPassword: update error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset password"})
			return
		}

		// Revoke all sessions of the target user.
		bumpTokenVersion(deps, ctx, id)

		logAudit(deps, c, "reset_password", "user", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "password reset successfully"})
	}
}

// bumpTokenVersion increments the user's token_version, revoking all
// outstanding JWT sessions for that user (they can no longer be refreshed).
func bumpTokenVersion(deps Dependencies, ctx context.Context, userID uuid.UUID) {
	if _, err := deps.DB.Pool.Exec(ctx, `
		UPDATE users SET token_version = COALESCE(token_version, 0) + 1, updated_at = NOW() WHERE id = $1
	`, userID); err != nil {
		slog.Info("bumpTokenVersion: failed for user ", "id", userID, "error", err)
	}
}

// ── Bootstrap token handlers ────────────────────────────────────────────────

// GenerateBootstrapToken creates a cryptographically random token string.
func GenerateBootstrapToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashBootstrapToken returns a SHA-256 hash of the token.
func HashBootstrapToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// bootstrapStatusHandler returns whether the system needs bootstrap (first-run).
// Results are cached for 30 seconds to avoid hitting the database on every
// page load — bootstrap status changes at most once (first-run activation).
var (
	bootstrapStatusMu    sync.Mutex
	bootstrapStatusCache *struct {
		needed      bool
		inProgress  bool
		expiresAt   time.Time
	}
)

func bootstrapStatusHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check cache first
		bootstrapStatusMu.Lock()
		if bootstrapStatusCache != nil && time.Now().Before(bootstrapStatusCache.expiresAt) {
			cached := bootstrapStatusCache
			bootstrapStatusMu.Unlock()
			c.JSON(http.StatusOK, gin.H{
				"needed":      cached.needed,
				"in_progress": cached.inProgress,
			})
			return
		}
		bootstrapStatusMu.Unlock()

		ctx := c.Request.Context()

		// Check if there are any non-default users (real users besides the seeded admin).
		var userCount int
		err := deps.DB.Pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM users WHERE id != '00000000-0000-0000-0000-000000000010'
		`).Scan(&userCount)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check bootstrap status"})
			return
		}

		// If real users exist, bootstrap is complete.
		if userCount > 0 {
			c.JSON(http.StatusOK, gin.H{
				"needed":      false,
				"in_progress": false,
			})
			return
		}

		// Check if any bootstrap token has been used — this is the definitive
		// signal that the bootstrap flow was already completed (token activated
		// + password changed), even though must_change_password has since been
		// reset to false.
		var hasUsedToken bool
		_ = deps.DB.Pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM bootstrap_tokens WHERE used_at IS NOT NULL
			)
		`).Scan(&hasUsedToken)
		if hasUsedToken {
			c.JSON(http.StatusOK, gin.H{
				"needed":      false,
				"in_progress": false,
			})
			return
		}

		// Check if there's an unused, non-expired bootstrap token.
		var hasValidToken bool
		err = deps.DB.Pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM bootstrap_tokens
				WHERE used_at IS NULL AND expires_at > NOW()
			)
		`).Scan(&hasValidToken)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check bootstrap status"})
			return
		}

		// Bootstrap is needed if no valid token exists yet (fresh install).
		needed := !hasValidToken
		// If there's a valid token, bootstrap was initiated but not yet completed.
		inProgress := hasValidToken

		c.JSON(http.StatusOK, gin.H{
			"needed":      needed,
			"in_progress": inProgress,
		})

		// Update cache
		bootstrapStatusMu.Lock()
		bootstrapStatusCache = &struct {
			needed      bool
			inProgress  bool
			expiresAt   time.Time
		}{needed: needed, inProgress: inProgress, expiresAt: time.Now().Add(30 * time.Second)}
		bootstrapStatusMu.Unlock()
	}
}

// bootstrapActivateHandler validates a bootstrap token, sets the admin password,
// and returns a JWT for the admin user. This is the one-time first-run setup flow.
func bootstrapActivateHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Token       string `json:"token" binding:"required"`
			NewPassword string `json:"new_password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "token and new_password are required"})
			return
		}

		// Validate password strength
		if err := auth.ValidatePasswordStrength(req.NewPassword); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx := c.Request.Context()
		tokenHash := HashBootstrapToken(req.Token)

		// Atomically find and consume the matching unused, non-expired token.
		// A single UPDATE ... RETURNING prevents concurrent requests from
		// redeeming the same token twice (race between SELECT and UPDATE).
		var tokenID uuid.UUID
		err := deps.DB.Pool.QueryRow(ctx, `
			UPDATE bootstrap_tokens
			SET used_at = NOW()
			WHERE token_hash = $1 AND used_at IS NULL AND expires_at > NOW()
			RETURNING id
		`, tokenHash).Scan(&tokenID)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "invalid or expired bootstrap token"})
			return
		}

		// Hash the new password
		adminID := uuid.MustParse(database.SuperAdminUserID)
		newHash, err := auth.HashPassword(req.NewPassword, auth.DefaultBCryptCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}

		// Set the admin password, clear must_change_password
		if _, err := deps.DB.Pool.Exec(ctx, `
			UPDATE users
			SET password_hash = $2, must_change_password = false, updated_at = NOW()
			WHERE id = $1
		`, adminID, newHash); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set admin password"})
			return
		}

		// Bump token_version to revoke any outstanding sessions
		bumpTokenVersion(deps, ctx, adminID)

		// Get admin roles.
		tenantID := uuid.MustParse(database.DefaultTenantID)
		orgID := uuid.MustParse(database.DefaultOrganizationID)
		var roles []string
		if deps.RBAC != nil {
			assignments, err := deps.RBAC.GetUserRoles(ctx, tenantID, adminID)
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
		if len(roles) == 0 {
			roles = []string{"admin"}
		}

		// Read actual admin user data from DB.
		var adminUser struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		}
		_ = deps.DB.Pool.QueryRow(ctx, `SELECT email, name FROM users WHERE id = $1`, adminID).Scan(&adminUser.Email, &adminUser.Name)

		// Read the admin's current token version (after bump).
		var adminTokenVersion int
		_ = deps.DB.Pool.QueryRow(ctx, `SELECT COALESCE(token_version, 0) FROM users WHERE id = $1`, adminID).Scan(&adminTokenVersion)

		// Generate JWT.
		tokenExpiry := deps.Config.Auth.SessionDuration
		if tokenExpiry == 0 {
			tokenExpiry = 24 * time.Hour // fallback: 24 hours
		}
		jwt, err := auth.GenerateToken(
			deps.Config.Auth.JWTSecret,
			adminID, tenantID, orgID,
			adminUser.Email, roles, adminTokenVersion, tokenExpiry,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		logAudit(deps, c, "bootstrap_activate", "system", "", nil, nil)

		// Invalidate bootstrap status cache — bootstrap is now complete
		bootstrapStatusMu.Lock()
		bootstrapStatusCache = nil
		bootstrapStatusMu.Unlock()

		setAuthCookie(c, deps, jwt, tokenExpiry)

		c.JSON(http.StatusOK, gin.H{
			"token":      jwt,
			"expires_in": int(tokenExpiry.Seconds()),
			"user": gin.H{
				"id":    adminID,
				"email": adminUser.Email,
				"name":  adminUser.Name,
				"roles": roles,
			},
			"must_change_password": false,
		})
	}
}

// SeedBootstrapToken generates and stores a bootstrap token if this is a fresh install.
// It returns the raw token to be printed to the console.
func SeedBootstrapToken(deps Dependencies) (string, error) {
	// Check if there are any non-default users.
	var userCount int
	err := deps.DB.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM users WHERE id != '00000000-0000-0000-0000-000000000010'
	`).Scan(&userCount)
	if err != nil {
		return "", err
	}
	if userCount > 0 {
		return "", nil // not a fresh install
	}

	// Check if a valid token already exists.
	var hasValidToken bool
	err = deps.DB.Pool.QueryRow(context.Background(), `
		SELECT EXISTS(
			SELECT 1 FROM bootstrap_tokens
			WHERE used_at IS NULL AND expires_at > NOW()
		)
	`).Scan(&hasValidToken)
	if err != nil {
		return "", err
	}
	if hasValidToken {
		return "", nil // token already generated
	}

	// Generate a new token.
	rawToken, err := GenerateBootstrapToken()
	if err != nil {
		return "", err
	}
	tokenHash := HashBootstrapToken(rawToken)
	expiresAt := time.Now().Add(1 * time.Hour)

	_, err = deps.DB.Pool.Exec(context.Background(), `
		INSERT INTO bootstrap_tokens (token_hash, expires_at) VALUES ($1, $2)
	`, tokenHash, expiresAt)
	if err != nil {
		return "", err
	}

	return rawToken, nil
}

// assignDefaultViewerRole assigns the viewer role in the default workspace to
// a newly created user. This ensures new users have minimal read-only access
// until an administrator grants additional permissions.
func assignDefaultViewerRole(ctx context.Context, deps Dependencies, userID uuid.UUID) {
	if deps.RBAC == nil {
		return
	}
	tenantID := uuid.MustParse(database.DefaultTenantID)
	var viewerRoleID uuid.UUID
	err := deps.DB.Pool.QueryRow(ctx,
		`SELECT id FROM roles WHERE tenant_id = $1 AND slug = 'viewer'`,
		tenantID,
	).Scan(&viewerRoleID)
	if err != nil {
		slog.Info("assignDefaultViewerRole: viewer role not found, skipping", "error", err)
		return
	}
	if _, err := deps.RBAC.AssignRole(ctx, tenantID, userID, viewerRoleID, nil); err != nil {
		slog.Info("assignDefaultViewerRole: failed to assign viewer role", "user_id", userID, "error", err)
	}
}
