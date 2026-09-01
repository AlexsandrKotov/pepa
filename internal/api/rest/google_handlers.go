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

// googleStates stores state/nonce for Google OAuth flow.
var (
	googleStates   = make(map[string]googleStateData)
	googleStatesMu = &oidcStatesMu // share the same mutex
)

type googleStateData = oidcStateData

// googleConfigHandler returns public Google OAuth configuration for the frontend.
func googleConfigHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !deps.Config.Auth.Google.Enabled {
			c.JSON(http.StatusOK, gin.H{"enabled": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"enabled": true})
	}
}

// googleLoginHandler initiates the Google OAuth authentication flow.
func googleLoginHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !deps.Config.Auth.Google.Enabled {
			c.JSON(http.StatusNotFound, gin.H{"error": "Google OAuth not enabled"})
			return
		}

		state, err := auth.GenerateState()
		if err != nil {
			slog.Error("failed to generate Google state", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state"})
			return
		}

		nonce, err := auth.GenerateNonce()
		if err != nil {
			slog.Error("failed to generate Google nonce", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate nonce"})
			return
		}

		googleStatesMu.Lock()
		googleStates[state] = googleStateData{
			Nonce:     nonce,
			CreatedAt: time.Now(),
		}
		googleStatesMu.Unlock()

		provider := auth.NewGoogleProvider(deps.Config.Auth.Google)
		authURL, err := provider.BuildAuthURL(c.Request.Context(), state, nonce)
		if err != nil {
			slog.Error("failed to build Google auth URL", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build auth URL"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"redirect_url": authURL})
	}
}

// googleCallbackHandler handles the Google OAuth callback.
func googleCallbackHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !deps.Config.Auth.Google.Enabled {
			c.JSON(http.StatusNotFound, gin.H{"error": "Google OAuth not enabled"})
			return
		}

		code := c.Query("code")
		state := c.Query("state")

		if code == "" || state == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing code or state"})
			return
		}

		// Verify state
		googleStatesMu.Lock()
		_, exists := googleStates[state]
		if exists {
			delete(googleStates, state)
		}
		googleStatesMu.Unlock()

		if !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
			return
		}

		// Exchange code for tokens
		provider := auth.NewGoogleProvider(deps.Config.Auth.Google)
		tokens, err := provider.ExchangeCode(c.Request.Context(), code)
		if err != nil {
			slog.Error("failed to exchange Google code", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to exchange code"})
			return
		}

		// Get Google user info
		googleInfo, err := provider.GetGoogleUserInfo(c.Request.Context(), tokens.AccessToken)
		if err != nil {
			slog.Error("failed to get Google user info", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user info"})
			return
		}

		// Google may return unverified emails — reject them to prevent
		// an attacker from claiming any email address.
		if !googleInfo.EmailVerified {
			slog.Warn("Google login rejected: email not verified", "email", googleInfo.Email)
			c.JSON(http.StatusForbidden, gin.H{"error": "Google email is not verified"})
			return
		}

		// Find or create user
		user, err := findOrCreateGoogleUser(c.Request.Context(), deps, googleInfo)
		if err != nil {
			slog.Error("failed to find/create Google user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authenticate user"})
			return
		}

		// Get user roles
		tenantID := uuid.MustParse(database.DefaultTenantID)
		orgID := uuid.MustParse(database.DefaultOrganizationID)
		var roles []string
		if deps.RBAC != nil {
			assignments, err := deps.RBAC.GetUserRoles(c.Request.Context(), tenantID, user.ID)
			if err == nil {
				for _, a := range assignments {
					slug := a.RoleSlug
					if slug == "admin" || slug == "super_admin" || slug == "platform_admin" {
						roles = append(roles, "admin")
					} else {
						roles = append(roles, slug)
					}
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
			slog.Error("failed to generate JWT for Google user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		setAuthCookie(c, deps, token, tokenExpiry)
		slog.Info("Google login successful", "user_id", user.ID, "email", user.Email)

		c.Redirect(http.StatusTemporaryRedirect, "/")
	}
}

// findOrCreateGoogleUser finds an existing user by email or external_id,
// or creates a new one from Google user info.
func findOrCreateGoogleUser(ctx context.Context, deps Dependencies, info *auth.GoogleUserInfo) (*repository.User, error) {
	email := info.EffectiveEmail()
	externalID := info.ExternalID()

	// Try to find by external_id first
	if externalID != "" {
		user, err := findUserByExternalID(ctx, deps, externalID, "google")
		if err == nil {
			return user, nil
		}
	}

	// Try to find by email
	user, err := deps.Repos.Auth.GetUserByEmail(ctx, email)
	if err == nil {
		// Update external_id if not set
		if externalID != "" {
			_ = deps.DB.Pool.QueryRow(ctx, `
				UPDATE users SET external_id = $1, auth_provider = 'google' WHERE id = $2
			`, externalID, user.ID)
		}
		return user, nil
	}

	// Create new user
	displayName := info.EffectiveName()
	userID := uuid.New()
	tenantID := uuid.MustParse(database.DefaultTenantID)
	orgID := uuid.MustParse(database.DefaultOrganizationID)

	_, err = deps.DB.Pool.Exec(ctx, `
		INSERT INTO users (id, email, name, is_active, tenant_id, organization_id, token_version, auth_provider, external_id)
		VALUES ($1, $2, $3, true, $4, $5, 0, 'google', $6)
	`, userID, email, displayName, tenantID, orgID, externalID)
	if err != nil {
		return nil, err
	}

	createdUser, err := deps.Repos.Auth.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	// Auto-assign default viewer role so new users have minimal access.
	assignDefaultViewerRole(ctx, deps, userID)

	slog.Info("created new user via Google", "user_id", createdUser.ID, "email", createdUser.Email)
	return createdUser, nil
}
