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

// cleanupTicker fires periodically to remove expired OAuth states.
var cleanupTicker = time.NewTicker(5 * time.Minute)

func init() {
	go func() {
		for range cleanupTicker.C {
			cleanupOAuthStates()
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

		// Store state for later verification.
		storeOAuthState("OIDC", state)

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
	cfg := oauthProviderConfig{
		name:    "OIDC",
		enabled: deps.Config.Auth.OIDC.Enabled,
		exchangeAndFindUser: func(ctx context.Context, code string) (*repository.User, error) {
			provider := auth.NewOIDCProvider(deps.Config.Auth.OIDC)
			tokens, err := provider.ExchangeCode(ctx, code)
			if err != nil {
				return nil, err
			}
			userInfo, err := provider.GetUserInfo(ctx, tokens.AccessToken)
			if err != nil {
				return nil, err
			}
			return findOrCreateOIDCUser(ctx, deps, userInfo)
		},
	}
	return genericOAuthCallback(deps, cfg)
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
