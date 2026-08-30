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

// azureStates stores state/nonce for Azure AD OIDC flow (reuses the same
// in-memory store as generic OIDC; in production, use Redis).
var (
	azureStates   = make(map[string]azureStateData)
	azureStatesMu = oidcStatesMu // share the same mutex
)

type azureStateData = oidcStateData

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

		azureStatesMu.Lock()
		azureStates[state] = azureStateData{
			Nonce:     nonce,
			CreatedAt: time.Now(),
		}
		azureStatesMu.Unlock()

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
	return func(c *gin.Context) {
		if !deps.Config.Auth.AzureAD.Enabled {
			c.JSON(http.StatusNotFound, gin.H{"error": "Azure AD not enabled"})
			return
		}

		code := c.Query("code")
		state := c.Query("state")

		if code == "" || state == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing code or state"})
			return
		}

		// Verify state
		azureStatesMu.Lock()
		_, exists := azureStates[state]
		if exists {
			delete(azureStates, state)
		}
		azureStatesMu.Unlock()

		if !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
			return
		}

		// Exchange code for tokens using the Azure OIDC config
		provider := auth.NewAzureProvider(deps.Config.Auth.AzureAD)
		tokens, err := provider.ExchangeCode(c.Request.Context(), code)
		if err != nil {
			slog.Error("failed to exchange Azure code", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to exchange code"})
			return
		}

		// Get Azure-specific user info
		azureInfo, err := provider.GetAzureUserInfo(c.Request.Context(), tokens.AccessToken)
		if err != nil {
			slog.Error("failed to get Azure user info", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user info"})
			return
		}

		// Find or create user
		user, err := findOrCreateAzureUser(c.Request.Context(), deps, azureInfo)
		if err != nil {
			slog.Error("failed to find/create Azure user", "error", err)
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
		if len(roles) == 0 {
			roles = []string{"user"}
		}

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
			slog.Error("failed to generate JWT for Azure user", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		setAuthCookie(c, deps, token, tokenExpiry)
		slog.Info("Azure AD login successful", "user_id", user.ID, "email", user.Email)

		c.Redirect(http.StatusTemporaryRedirect, "/")
	}
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
	email := info.EffectiveEmail()
	externalID := info.ExternalID()

	// Try to find by external_id first (most reliable for returning users)
	if externalID != "" {
		user, err := findUserByExternalID(ctx, deps, externalID, "azure")
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
				UPDATE users SET external_id = $1, auth_provider = 'azure' WHERE id = $2
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
		VALUES ($1, $2, $3, true, $4, $5, 0, 'azure', $6)
	`, userID, email, displayName, tenantID, orgID, externalID)
	if err != nil {
		return nil, err
	}

	createdUser, err := deps.Repos.Auth.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	slog.Info("created new user via Azure AD", "user_id", createdUser.ID, "email", createdUser.Email)
	return createdUser, nil
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
