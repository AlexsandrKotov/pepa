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

// githubConfigHandler returns public GitHub OAuth configuration for the frontend.
func githubConfigHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !deps.Config.Auth.GitHub.Enabled {
			c.JSON(http.StatusOK, gin.H{"enabled": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"enabled": true})
	}
}

// githubLoginHandler initiates the GitHub OAuth authentication flow.
func githubLoginHandler(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !deps.Config.Auth.GitHub.Enabled {
			c.JSON(http.StatusNotFound, gin.H{"error": "GitHub OAuth not enabled"})
			return
		}

		state, err := auth.GenerateState()
		if err != nil {
			slog.Error("failed to generate GitHub state", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state"})
			return
		}

		// Store state for later verification.
		storeOAuthState("GitHub", state)

		provider := auth.NewGitHubProvider(deps.Config.Auth.GitHub)
		authURL, err := provider.BuildAuthURL(c.Request.Context(), state)
		if err != nil {
			slog.Error("failed to build GitHub auth URL", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build auth URL"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"redirect_url": authURL})
	}
}

// githubCallbackHandler handles the GitHub OAuth callback.
func githubCallbackHandler(deps Dependencies) gin.HandlerFunc {
	cfg := oauthProviderConfig{
		name:    "GitHub",
		enabled: deps.Config.Auth.GitHub.Enabled,
		exchangeAndFindUser: func(ctx context.Context, code string) (*repository.User, error) {
			provider := auth.NewGitHubProvider(deps.Config.Auth.GitHub)
			accessToken, err := provider.ExchangeCode(ctx, code)
			if err != nil {
				return nil, err
			}
			githubInfo, err := provider.GetGitHubUserInfo(ctx, accessToken)
			if err != nil {
				return nil, err
			}
			return findOrCreateGitHubUser(ctx, deps, githubInfo)
		},
	}
	return genericOAuthCallback(deps, cfg)
}

// findOrCreateGitHubUser finds an existing user by email or external_id,
// or creates a new one from GitHub user info.
func findOrCreateGitHubUser(ctx context.Context, deps Dependencies, info *auth.GitHubUserInfo) (*repository.User, error) {
	email := info.EffectiveEmail()
	if email == "" {
		return nil, fmt.Errorf("GitHub user has no email address")
	}
	return findOrCreateOAuthUser(ctx, deps, email, info.EffectiveName(), info.ExternalID(), "github")
}
