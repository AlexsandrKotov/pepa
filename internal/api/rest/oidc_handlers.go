package rest

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/internal/repository"
)

// oidcStateStore stores state and nonce for OIDC flow (in production, use Redis).
var (
	oidcStates   = make(map[string]oidcStateData)
	oidcStatesMu sync.Mutex
)

type oidcStateData struct {
	Nonce     string
	CreatedAt time.Time
}

// cleanupTicker fires periodically to remove expired OIDC states.
var cleanupTicker = time.NewTicker(5 * time.Minute)

func init() {
	go func() {
		for range cleanupTicker.C {
			cleanupOldStates()
		}
	}()
}

// oidcLoginHandler initiates the OIDC authentication flow.
func oidcLoginHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !deps.Config.Auth.OIDC.Enabled {
			c.JSON(http.StatusNotFound, gin.H{"error": "OIDC not enabled"})
			return
		}

		state, err := auth.GenerateState()
		if err != nil {
			slog.Error("failed to generate OIDC state", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state"})
			return
		}

		nonce, err := auth.GenerateNonce()
		if err != nil {
			slog.Error("failed to generate OIDC nonce", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate nonce"})
			return
		}

		// Store state and nonce
		oidcStatesMu.Lock()
		oidcStates[state] = oidcStateData{
			Nonce:     nonce,
			CreatedAt: time.Now(),
		}
		oidcStatesMu.Unlock()

		provider := auth.NewOIDCProvider(deps.Config.Auth.OIDC)
		authURL, err := provider.BuildAuthURL(c.Request.Context(), state, nonce)
		if err != nil {
			slog.Error("failed to build OIDC auth URL", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build auth URL"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"redirect_url": authURL,
		})
	}
}

// oidcCallbackHandler handles the OIDC callback after user authentication.
func oidcCallbackHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !deps.Config.Auth.OIDC.Enabled {
			c.JSON(http.StatusNotFound, gin.H{"error": "OIDC not enabled"})
			return
		}

		code := c.Query("code")
		state := c.Query("state")

		if code == "" || state == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing code or state"})
			return
		}

		// Verify state
		oidcStatesMu.Lock()
		_, exists := oidcStates[state]
		if exists {
			delete(oidcStates, state)
		}
		oidcStatesMu.Unlock()

		if !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
			return
		}

		// Exchange code for tokens
		provider := auth.NewOIDCProvider(deps.Config.Auth.OIDC)
		tokens, err := provider.ExchangeCode(c.Request.Context(), code)
		if err != nil {
			slog.Error("failed to exchange OIDC code", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to exchange code"})
			return
		}

		// Get user info
		userInfo, err := provider.GetUserInfo(c.Request.Context(), tokens.AccessToken)
		if err != nil {
			slog.Error("failed to get OIDC user info", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user info"})
			return
		}

		// Find or create user
		user, err := findOrCreateOIDCUser(c.Request.Context(), deps, userInfo)
		if err != nil {
			slog.Error("failed to find/create OIDC user", "error", err)
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
		
		// Generate JWT token
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
			slog.Error("failed to generate JWT", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		// Set auth cookie — use the same expiry as the JWT
		cookieExpiry := tokenExpiry
		setAuthCookie(c, deps, token, cookieExpiry)

		slog.Info("OIDC login successful", "user_id", user.ID, "email", user.Email)

		// Redirect to frontend
		c.Redirect(http.StatusTemporaryRedirect, "/")
	}
}

// oidcConfigHandler returns public OIDC configuration for the frontend.
func oidcConfigHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !deps.Config.Auth.OIDC.Enabled {
			c.JSON(http.StatusOK, gin.H{
				"enabled": false,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"enabled":      true,
			"issuer":       deps.Config.Auth.OIDC.Issuer,
			"client_id":    deps.Config.Auth.OIDC.ClientID,
			"redirect_url": deps.Config.Auth.OIDC.RedirectURL,
			"scopes":       deps.Config.Auth.OIDC.Scopes,
		})
	}
}

// findOrCreateOIDCUser finds an existing user or creates a new one from OIDC info.
func findOrCreateOIDCUser(ctx context.Context, deps Dependencies, userInfo *auth.OIDCUserInfo) (*repository.User, error) {
	// Try to find user by email
	user, err := deps.Repos.Auth.GetUserByEmail(ctx, userInfo.Email)
	if err == nil {
		// User exists
		return user, nil
	}

	// User doesn't exist, create new one
	displayName := userInfo.Name
	if displayName == "" {
		displayName = userInfo.PreferredUsername
	}
	if displayName == "" {
		displayName = userInfo.Email
	}

	// Create user in database
	userID := uuid.New()
	tenantID := uuid.MustParse(database.DefaultTenantID)
	orgID := uuid.MustParse(database.DefaultOrganizationID)

	_, err = deps.DB.Pool.Exec(ctx, `
		INSERT INTO users (id, email, name, is_active, tenant_id, organization_id, token_version)
		VALUES ($1, $2, $3, true, $4, $5, 0)
	`, userID, userInfo.Email, displayName, tenantID, orgID)
	if err != nil {
		return nil, err
	}

	// Auto-assign default viewer role so new users have minimal access.
	assignDefaultViewerRole(ctx, deps, userID)

	// Return created user
	createdUser, err := deps.Repos.Auth.GetUserByEmail(ctx, userInfo.Email)
	if err != nil {
		return nil, err
	}

	slog.Info("created new user via OIDC", "user_id", createdUser.ID, "email", createdUser.Email)
	return createdUser, nil
}

// cleanupOldStates removes OIDC states older than 10 minutes.
func cleanupOldStates() {
	oidcStatesMu.Lock()
	defer oidcStatesMu.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute)
	for state, data := range oidcStates {
		if data.CreatedAt.Before(cutoff) {
			delete(oidcStates, state)
		}
	}
	for state, data := range azureStates {
		if data.CreatedAt.Before(cutoff) {
			delete(azureStates, state)
		}
	}
	for state, data := range googleStates {
		if data.CreatedAt.Before(cutoff) {
			delete(googleStates, state)
		}
	}
	for state, data := range githubStates {
		if data.CreatedAt.Before(cutoff) {
			delete(githubStates, state)
		}
	}
}
