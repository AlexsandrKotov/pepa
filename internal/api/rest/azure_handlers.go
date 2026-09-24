package rest

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
)

// azureLoginHandler initiates the Azure AD OIDC authentication flow.
func azureLoginHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !deps.Config.Auth.AzureAD.Enabled {
			c.JSON(http.StatusNotFound, gin.H{"error": "Azure AD not enabled"})
			return
		}

		state, err := auth.GenerateState()
		if err != nil {
			slog.Error("failed to generate Azure state", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state"})
			return
		}

		nonce, err := auth.GenerateNonce()
		if err != nil {
			slog.Error("failed to generate Azure nonce", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate nonce"})
			return
		}

		// Store state for later verification.
		storeOAuthState("AzureAD", state)

		provider := auth.NewAzureProvider(deps.Config.Auth.AzureAD)
		authURL, err := provider.BuildAuthURL(c.Request.Context(), state, nonce)
		if err != nil {
			slog.Error("failed to build Azure auth URL", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build auth URL"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"redirect_url": authURL})
	}
}

// azureCallbackHandler handles the Azure AD OIDC callback.
func azureCallbackHandler(deps Dependencies) gin.HandlerFunc {
	cfg := oauthProviderConfig{
		name:    "AzureAD",
		enabled: deps.Config.Auth.AzureAD.Enabled,
		exchangeAndFindUser: func(ctx context.Context, code string) (*repository.User, error) {
			provider := auth.NewAzureProvider(deps.Config.Auth.AzureAD)
			tokens, err := provider.ExchangeCode(ctx, code)
			if err != nil {
				return nil, err
			}
			azureInfo, err := provider.GetAzureUserInfo(ctx, tokens.AccessToken)
			if err != nil {
				return nil, err
			}
			return findOrCreateAzureUser(ctx, deps, azureInfo)
		},
	}
	return genericOAuthCallback(deps, cfg)
}

// azureConfigHandler returns public Azure AD configuration for the frontend.
func azureConfigHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !deps.Config.Auth.AzureAD.Enabled {
			c.JSON(http.StatusOK, gin.H{"enabled": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"enabled": true,
		})
	}
}

// findOrCreateAzureUser finds an existing user by email or external_id,
// or creates a new one from Azure AD user info.
func findOrCreateAzureUser(ctx context.Context, deps Dependencies, info *auth.AzureUserInfo) (*repository.User, error) {
	return findOrCreateOAuthUser(ctx, deps, info.EffectiveEmail(), info.EffectiveName(), info.ExternalID(), "azure")
}

// findUserByExternalID looks up a user by their external_id and auth_provider.
func findUserByExternalID(ctx context.Context, deps Dependencies, externalID, provider string) (*repository.User, error) {
	var user repository.User
	err := deps.DB.Pool.QueryRow(ctx, `
		SELECT id, email, name, is_active, token_version
		FROM users WHERE external_id = $1 AND auth_provider = $2 AND is_active = true
	`, externalID, provider).Scan(
		&user.ID, &user.Email, &user.Name, &user.IsActive, &user.TokenVersion,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
