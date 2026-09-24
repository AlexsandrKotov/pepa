package rest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
)

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

		// Store state for later verification.
		storeOAuthState("Google", state)

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
	cfg := oauthProviderConfig{
		name:    "Google",
		enabled: deps.Config.Auth.Google.Enabled,
		exchangeAndFindUser: func(ctx context.Context, code string) (*repository.User, error) {
			provider := auth.NewGoogleProvider(deps.Config.Auth.Google)
			tokens, err := provider.ExchangeCode(ctx, code)
			if err != nil {
				return nil, err
			}
			googleInfo, err := provider.GetGoogleUserInfo(ctx, tokens.AccessToken)
			if err != nil {
				return nil, err
			}
			// Google may return unverified emails — reject them.
			if !googleInfo.EmailVerified {
				return nil, fmt.Errorf("Google email is not verified")
			}
			return findOrCreateGoogleUser(ctx, deps, googleInfo)
		},
	}
	return genericOAuthCallback(deps, cfg)
}

// findOrCreateGoogleUser finds an existing user by email or external_id,
// or creates a new one from Google user info.
func findOrCreateGoogleUser(ctx context.Context, deps Dependencies, info *auth.GoogleUserInfo) (*repository.User, error) {
	return findOrCreateOAuthUser(ctx, deps, info.EffectiveEmail(), info.EffectiveName(), info.ExternalID(), "google")
}
