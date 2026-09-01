package rest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/internal/repository"
)

// githubStates stores state for GitHub OAuth flow.
var (
	githubStates   = make(map[string]githubStateData)
	githubStatesMu = &oidcStatesMu // share the same mutex
)

type githubStateData = oidcStateData

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

		githubStatesMu.Lock()
		githubStates[state] = githubStateData{
			CreatedAt: time.Now(),
		}
		githubStatesMu.Unlock()

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
	return func(c *gin.Context) {
		if !deps.Config.Auth.GitHub.Enabled {
			c.JSON(http.StatusNotFound, gin.H{"error": "GitHub OAuth not enabled"})
			return
		}

		code := c.Query("code")
		state := c.Query("state")

		if code == "" || state == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing code or state"})
			return
		}

		// Verify state
		githubStatesMu.Lock()
		_, exists := githubStates[state]
		if exists {
			delete(githubStates, state)
		}
		githubStatesMu.Unlock()

		if !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
			return
		}

		// Exchange code for access token
		provider := auth.NewGitHubProvider(deps.Config.Auth.GitHub)
		accessToken, err := provider.ExchangeCode(c.Request.Context(), code)
		if err != nil {
			slog.Error("failed to exchange GitHub code", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to exchange code"})
			return
		}

		// Get GitHub user info
		githubInfo, err := provider.GetGitHubUserInfo(c.Request.Context(), accessToken)
		if err != nil {
			slog.Error("failed to get GitHub user info", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user info"})
			return
		}

		// Find or create user
		user, err := findOrCreateGitHubUser(c.Request.Context(), deps, githubInfo)
		if err != nil {
			slog.Error("failed to find/create GitHub user", "error", err)
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
			slog.Error("failed to generate JWT for GitHub user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		setAuthCookie(c, deps, token, tokenExpiry)
		slog.Info("GitHub login successful", "user_id", user.ID, "email", user.Email)

		c.Redirect(http.StatusTemporaryRedirect, "/")
	}
}

// findOrCreateGitHubUser finds an existing user by email or external_id,
// or creates a new one from GitHub user info.
func findOrCreateGitHubUser(ctx context.Context, deps Dependencies, info *auth.GitHubUserInfo) (*repository.User, error) {
	email := info.EffectiveEmail()
	externalID := info.ExternalID()

	if email == "" {
		return nil, fmt.Errorf("GitHub user has no email address")
	}

	// Try to find by external_id first
	if externalID != "" {
		user, err := findUserByExternalID(ctx, deps, externalID, "github")
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
				UPDATE users SET external_id = $1, auth_provider = 'github' WHERE id = $2
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
		VALUES ($1, $2, $3, true, $4, $5, 0, 'github', $6)
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

	slog.Info("created new user via GitHub", "user_id", createdUser.ID, "email", createdUser.Email)
	return createdUser, nil
}
