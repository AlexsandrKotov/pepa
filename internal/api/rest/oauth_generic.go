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

// oauthCallbacks is the unified state store for all OAuth providers.
// Each provider uses a namespaced key (e.g. "oidc:state123", "google:state456").
var (
	oauthCallbacks   = make(map[string]time.Time)
	oauthCallbacksMu sync.Mutex
)

func init() {
	// Periodic cleanup of expired OAuth states to prevent memory leaks
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cleanupOAuthStates()
		}
	}()
}

// oauthProviderConfig captures the provider-specific parts of the OAuth flow.
type oauthProviderConfig struct {
	// name is the provider name for logging (e.g. "Google", "GitHub").
	name string
	// enabled indicates whether the provider is currently configured.
	enabled bool
	// exchangeAndFindUser exchanges the authorization code for user info and
	// finds or creates the corresponding user in the database.
	exchangeAndFindUser func(ctx context.Context, code string) (*repository.User, error)
}

// genericOAuthCallback handles the common OAuth callback flow:
// validate params → verify state → exchange code → find/create user → issue JWT.
func genericOAuthCallback(deps Dependencies, cfg oauthProviderConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.enabled {
			c.JSON(http.StatusNotFound, gin.H{"error": cfg.name + " OAuth not enabled"})
			return
		}

		code := c.Query("code")
		state := c.Query("state")

		if code == "" || state == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing code or state"})
			return
		}

		// Verify and consume state (namespaced by provider).
		stateKey := cfg.name + ":" + state
		oauthCallbacksMu.Lock()
		_, exists := oauthCallbacks[stateKey]
		if exists {
			delete(oauthCallbacks, stateKey)
		}
		oauthCallbacksMu.Unlock()

		if !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
			return
		}

		// Exchange code for tokens and find/create user.
		user, err := cfg.exchangeAndFindUser(c.Request.Context(), code)
		if err != nil {
			slog.Error("failed to authenticate via "+cfg.name, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authenticate user"})
			return
		}

		// Get user roles from RBAC.
		tenantID := uuid.MustParse(database.DefaultTenantID)
		orgID := uuid.MustParse(database.DefaultOrganizationID)
		var roles []string
		if deps.RBAC != nil {
			assignments, err := deps.RBAC.GetUserRoles(c.Request.Context(), tenantID, user.ID)
			if err == nil {
				roles = append(roles, jwtRolesFromAssignments(assignments)...)
			}
		}

		// Generate JWT.
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
			slog.Error("failed to generate JWT for "+cfg.name+" user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		setAuthCookie(c, deps, token, tokenExpiry)
		slog.Info(cfg.name+" login successful", "user_id", user.ID, "email", user.Email)

		c.Redirect(http.StatusTemporaryRedirect, "/")
	}
}

// storeOAuthState stores an OAuth state parameter for later verification.
// The state is namespaced by provider name.
func storeOAuthState(providerName, state string) {
	stateKey := providerName + ":" + state
	oauthCallbacksMu.Lock()
	oauthCallbacks[stateKey] = time.Now()
	oauthCallbacksMu.Unlock()
}

// cleanupOAuthStates removes states older than 10 minutes.
func cleanupOAuthStates() {
	oauthCallbacksMu.Lock()
	defer oauthCallbacksMu.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute)
	for key, createdAt := range oauthCallbacks {
		if createdAt.Before(cutoff) {
			delete(oauthCallbacks, key)
		}
	}
}

// findOrCreateOAuthUser is a generic helper that finds or creates a user from
// OAuth user info. It handles the common pattern: try external_id → try email → create.
func findOrCreateOAuthUser(ctx context.Context, deps Dependencies, email, displayName, externalID, providerName string) (*repository.User, error) {
	// Try to find by external_id first.
	if externalID != "" {
		user, err := findUserByExternalID(ctx, deps, externalID, providerName)
		if err == nil {
			return user, nil
		}
	}

	// Try to find by email.
	user, err := deps.Repos.Auth.GetUserByEmail(ctx, email)
	if err == nil {
		// Update external_id if not set.
		if externalID != "" {
			if _, err := deps.DB.Pool.Exec(ctx, `
				UPDATE users SET external_id = $1, auth_provider = $2 WHERE id = $3
			`, externalID, providerName, user.ID); err != nil {
				slog.Warn("failed to backfill OAuth external_id",
					"provider", providerName, "user_id", user.ID, "error", err)
			}
		}
		return user, nil
	}

	// Create new user.
	userID := uuid.New()
	tenantID := uuid.MustParse(database.DefaultTenantID)
	orgID := uuid.MustParse(database.DefaultOrganizationID)

	_, err = deps.DB.Pool.Exec(ctx, `
		INSERT INTO users (id, email, name, is_active, tenant_id, organization_id, token_version, auth_provider, external_id)
		VALUES ($1, $2, $3, true, $4, $5, 0, $6, $7)
	`, userID, email, displayName, tenantID, orgID, providerName, externalID)
	if err != nil {
		return nil, err
	}

	createdUser, err := deps.Repos.Auth.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	// Auto-assign default viewer role so new users have minimal access.
	assignDefaultViewerRole(ctx, deps, userID)

	slog.Info("created new user via "+providerName, "user_id", createdUser.ID, "email", createdUser.Email)
	return createdUser, nil
}
