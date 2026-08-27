package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	pepacrypto "github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/storage"
)

// UserCredential represents a user's external service credential.
type UserCredential struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"user_id"`
	TenantID     uuid.UUID  `json:"tenant_id"`
	Provider     string     `json:"provider"`
	ProviderURL  string     `json:"provider_url"`
	DisplayName  string     `json:"display_name"`
	TokenMasked  string     `json:"token_masked"`
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	IsDefault    bool       `json:"is_default"`
	LastVerified *time.Time `json:"last_verified,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// registerUserCredentialRoutes registers per-user credential endpoints.
func registerUserCredentialRoutes(r *gin.RouterGroup, deps Dependencies) {
	creds := r.Group("/my/credentials")
	{
		creds.GET("", listMyCredentials(deps))
		creds.POST("", createMyCredential(deps))
		creds.POST("/fetch-user-info", fetchUserInfoForCredential(deps))
		creds.PUT("/:id", updateMyCredential(deps))
		creds.DELETE("/:id", deleteMyCredential(deps))
		creds.POST("/:id/verify", verifyMyCredential(deps))

		// Sharing endpoints
		creds.GET("/shared", listSharedWithMe(deps))
		creds.POST("/:id/share", shareMyCredential(deps))
		creds.GET("/:id/shares", listMyCredentialShares(deps))
		creds.DELETE("/:id/shares/:shareId", revokeMyCredentialShare(deps))
	}
}

// listMyCredentials returns the current user's external credentials with masked tokens.
func listMyCredentials(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		tenantID := auth.GetTenantID(c)
		ctx := c.Request.Context()

		credModels, err := deps.Repos.UserCredential.ListByUser(ctx, *userID, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		creds := make([]UserCredential, len(credModels))
		for i, cm := range credModels {
			creds[i] = UserCredential{
				ID:           cm.ID,
				UserID:       cm.UserID,
				TenantID:     cm.TenantID,
				Provider:     cm.Provider,
				ProviderURL:  cm.ProviderURL,
				DisplayName:  cm.DisplayName,
				TokenMasked:  maskToken(cm.TokenEnc),
				Username:     cm.Username,
				Email:        cm.Email,
				IsDefault:    cm.IsDefault,
				LastVerified: cm.LastVerified,
				CreatedAt:    cm.CreatedAt,
				UpdatedAt:    cm.UpdatedAt,
			}
		}

		c.JSON(http.StatusOK, gin.H{"credentials": creds, "total": len(creds)})
	}
}

// createMyCredential adds a new external credential for the current user.
func createMyCredential(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		var req struct {
			Provider    string `json:"provider" binding:"required"`
			ProviderURL string `json:"provider_url" binding:"required"`
			DisplayName string `json:"display_name"`
			Token       string `json:"token" binding:"required"`
			Username    string `json:"username"`
			Email       string `json:"email"`
			IsDefault   bool   `json:"is_default"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provider, provider_url, and token are required"})
			return
		}

		tenantID := auth.GetTenantID(c)
		ctx := c.Request.Context()

		// Resolve Vault reference if the token starts with "vault:"
		actualToken := req.Token
		if strings.HasPrefix(req.Token, "vault:") {
			resolved, err := resolveVaultRef(deps, ctx, req.Token, tenantID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to resolve Vault reference: %v", err)})
				return
			}
			actualToken = resolved
		}

		// Auto-fetch real name and email from the provider if not provided manually.
		if req.Username == "" || req.Email == "" {
			if name, email, fetchErr := fetchExternalUserInfo(ctx, req.Provider, req.ProviderURL, req.Username, actualToken); fetchErr == nil {
				if req.Username == "" && name != "" {
					req.Username = name
				}
				if req.Email == "" && email != "" {
					req.Email = email
				}
			} else {
				log.Printf("[credential] could not fetch user info from %s: %v", req.Provider, fetchErr)
			}
		}

		// Encrypt the token
		tokenEnc, err := pepacrypto.Encrypt(actualToken)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt token"})
			return
		}

		// If this is set as default, unset other defaults for this provider
		if req.IsDefault {
			if err := deps.Repos.UserCredential.UnsetDefaults(ctx, *userID, req.Provider); err != nil {
				log.Printf("[credential] failed to unset other defaults for user %s, provider %s: %v", *userID, req.Provider, err)
			}
		}

		credID := uuid.New()
		cred := &repository.UserCredential{
			ID:          credID,
			UserID:      *userID,
			TenantID:    tenantID,
			Provider:    req.Provider,
			ProviderURL: req.ProviderURL,
			DisplayName: req.DisplayName,
			TokenEnc:    tokenEnc,
			Username:    req.Username,
			Email:       req.Email,
			IsDefault:   req.IsDefault,
		}

		if err := deps.Repos.UserCredential.Create(ctx, cred); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create credential"})
			return
		}

		logAudit(deps, c, "create", "user_credential", credID.String(), nil, gin.H{"provider": req.Provider})

		c.JSON(http.StatusCreated, gin.H{
			"id":       credID,
			"provider": req.Provider,
			"message":  "credential saved",
		})
	}
}

// updateMyCredential updates an existing credential.
func updateMyCredential(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		credID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential ID"})
			return
		}

		var req struct {
			DisplayName string `json:"display_name"`
			Token       string `json:"token"`
			Username    string `json:"username"`
			Email       string `json:"email"`
			IsDefault   bool   `json:"is_default"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx := c.Request.Context()

		// Verify ownership
		owned, err := deps.Repos.UserCredential.VerifyOwnership(ctx, credID, *userID)
		if err != nil || !owned {
			c.JSON(http.StatusNotFound, gin.H{"error": "credential not found"})
			return
		}

		// Build update
		if req.IsDefault {
			// Unset other defaults for this provider
			provider, err := deps.Repos.UserCredential.GetProvider(ctx, credID)
			if err == nil {
				if err := deps.Repos.UserCredential.UnsetDefaults(ctx, *userID, provider); err != nil {
					log.Printf("[credential] failed to unset other defaults for user %s, provider %s: %v", *userID, provider, err)
				}
			}
		}

		cred := &repository.UserCredential{
			ID:          credID,
			DisplayName: req.DisplayName,
			Username:    req.Username,
			Email:       req.Email,
			IsDefault:   req.IsDefault,
		}

		if req.Token != "" {
			tokenEnc, err := pepacrypto.Encrypt(req.Token)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt token"})
				return
			}
			cred.TokenEnc = tokenEnc
			if err := deps.Repos.UserCredential.Update(ctx, cred); err != nil {
				respondInternalError(c, err)
				return
			}
		} else {
			if err := deps.Repos.UserCredential.UpdateWithoutToken(ctx, cred); err != nil {
				respondInternalError(c, err)
				return
			}
		}

		logAudit(deps, c, "update", "user_credential", credID.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "credential updated"})
	}
}

// deleteMyCredential removes a credential.
func deleteMyCredential(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		credID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential ID"})
			return
		}

		ctx := c.Request.Context()

		// Verify ownership
		owned, err := deps.Repos.UserCredential.VerifyOwnership(ctx, credID, *userID)
		if err != nil || !owned {
			c.JSON(http.StatusNotFound, gin.H{"error": "credential not found"})
			return
		}

		if err := deps.Repos.UserCredential.Delete(ctx, credID); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "user_credential", credID.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "credential deleted"})
	}
}

// verifyMyCredential tests the credential against the external provider.
func verifyMyCredential(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		credID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential ID"})
			return
		}

		ctx := c.Request.Context()

		// Get credential with decrypted token
		cred, err := deps.Repos.UserCredential.GetByID(ctx, credID)
		if err != nil || cred.UserID != *userID {
			c.JSON(http.StatusNotFound, gin.H{"error": "credential not found"})
			return
		}

		// Decrypt token
		token, err := pepacrypto.Decrypt(cred.TokenEnc)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decrypt token"})
			return
		}

		// Test the credential against the provider and fetch real user info.
		status, message := testExternalCredential(ctx, cred.Provider, cred.ProviderURL, cred.Username, token)

		if status == "connected" {
			// Auto-update username and email from the provider so git commits
			// are authored under the user's real identity (e.g. "Aleksandr Kotau"
			// instead of a manually entered login like "your-username").
			if name, email, fetchErr := fetchExternalUserInfo(ctx, cred.Provider, cred.ProviderURL, cred.Username, token); fetchErr == nil {
				if err := deps.Repos.UserCredential.UpdateVerification(ctx, credID, name, email); err != nil {
					log.Printf("[credential] failed to update user info for credential %s: %v", credID, err)
				}
			} else {
				if err := deps.Repos.UserCredential.UpdateVerification(ctx, credID, "", ""); err != nil {
					log.Printf("[credential] failed to update last_verified for credential %s: %v", credID, err)
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  status,
			"message": message,
		})
	}
}

// validateProviderURL checks that the URL is safe (no SSRF via loopback/metadata IPs).
// Private/internal IPs are allowed — this is a DevOps platform where users connect
// to self-hosted GitLab, Gitea, etc. on internal networks.
func validateProviderURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("only http/https schemes allowed")
	}
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("cannot resolve host: %w", err)
	}
	for _, ip := range ips {
		// Block loopback (127.0.0.0/8) — prevents accessing the API server's own services
		if ip.IsLoopback() {
			return fmt.Errorf("host resolves to loopback address")
		}
		// Block link-local (169.254.0.0/16) — prevents cloud metadata attacks (e.g. AWS IMDS)
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("host resolves to a blocked IP range")
		}
	}
	return nil
}

// testExternalCredential tests a credential against an external service.
// For S3, username = access_key_id and token = secret_key.
func testExternalCredential(ctx context.Context, provider, providerURL, username, token string) (string, string) {
	// Validate URL to prevent SSRF
	if err := validateProviderURL(providerURL); err != nil {
		return "error", fmt.Sprintf("invalid provider URL: %v", err)
	}

	switch provider {
	case "gitlab":
		return testGitLabCredential(ctx, providerURL, token)
	case "github":
		return testGitHubCredential(ctx, providerURL, token)
	case "gitea":
		return testGiteaCredential(ctx, providerURL, token)
	case "s3":
		return testS3Credential(ctx, providerURL, username, token)
	default:
		return "unknown", fmt.Sprintf("verification not supported for provider: %s", provider)
	}
}

func testGitLabCredential(ctx context.Context, baseURL, token string) (string, string) {
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/v4/user", nil)
	if err != nil {
		return "error", err.Error()
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("cannot reach GitLab: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 401 {
		return "error", "invalid token"
	}
	if resp.StatusCode == 200 {
		return "connected", "GitLab token valid"
	}
	return "error", fmt.Sprintf("GitLab returned status %d", resp.StatusCode)
}

func testGitHubCredential(ctx context.Context, baseURL, token string) (string, string) {
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/user", nil)
	if err != nil {
		return "error", err.Error()
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("cannot reach GitHub: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 401 {
		return "error", "invalid token"
	}
	if resp.StatusCode == 200 {
		return "connected", "GitHub token valid"
	}
	return "error", fmt.Sprintf("GitHub returned status %d", resp.StatusCode)
}

func testGiteaCredential(ctx context.Context, baseURL, token string) (string, string) {
	if baseURL == "" {
		return "error", "provider_url is required for Gitea"
	}
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/v1/user", nil)
	if err != nil {
		return "error", err.Error()
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("cannot reach Gitea: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 401 {
		return "error", "invalid token"
	}
	if resp.StatusCode == 200 {
		return "connected", "Gitea token valid"
	}
	return "error", fmt.Sprintf("Gitea returned status %d", resp.StatusCode)
}

// testS3Credential verifies S3/MinIO credentials by attempting to list buckets.
// username = access_key_id, token = secret_key.
func testS3Credential(ctx context.Context, endpoint, accessKey, secretKey string) (string, string) {
	if endpoint == "" {
		return "error", "S3 endpoint is required"
	}
	if accessKey == "" || secretKey == "" {
		return "error", "access key and secret key are required"
	}

	// Try with SSL first, then without
	for _, useSSL := range []bool{true, false} {
		s3Client, err := storage.NewS3ClientFromCredentials(endpoint, accessKey, secretKey, useSSL)
		if err != nil {
			return "error", err.Error()
		}
		if _, err := s3Client.ListBuckets(ctx); err == nil {
			return "connected", fmt.Sprintf("S3 credentials valid (ssl=%v)", useSSL)
		}
	}
	return "error", "S3 credentials rejected by server"
}

// fetchExternalUserInfo retrieves the user's real display name and email from
// the external provider (GitLab, GitHub, Gitea). For S3, it validates the
// credentials and returns the access_key as the username.
func fetchExternalUserInfo(ctx context.Context, provider, providerURL, username, token string) (name string, email string, err error) {
	if err := validateProviderURL(providerURL); err != nil {
		return "", "", err
	}

	client := &http.Client{Timeout: 10 * time.Second}

	switch provider {
	case "s3":
		// For S3, validate credentials by listing buckets. Return access_key as username.
		s3Client, s3Err := storage.NewS3ClientFromCredentials(providerURL, username, token, true)
		if s3Err != nil {
			return "", "", s3Err
		}
		if _, s3Err = s3Client.ListBuckets(ctx); s3Err != nil {
			// Try without SSL
			s3Client, s3Err = storage.NewS3ClientFromCredentials(providerURL, username, token, false)
			if s3Err != nil {
				return "", "", s3Err
			}
			if _, s3Err = s3Client.ListBuckets(ctx); s3Err != nil {
				return "", "", s3Err
			}
		}
		return username, "", nil
	case "gitlab":
		if providerURL == "" {
			providerURL = "https://gitlab.com"
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", providerURL+"/api/v4/user", nil)
		req.Header.Set("PRIVATE-TOKEN", token)
		resp, err := client.Do(req)
		if err != nil {
			return "", "", err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != 200 {
			return "", "", fmt.Errorf("gitlab returned status %d", resp.StatusCode)
		}
		var info struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&info); err == nil {
			return info.Name, info.Email, nil
		}
		return "", "", err

	case "github":
		if providerURL == "" {
			providerURL = "https://api.github.com"
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", providerURL+"/user", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			return "", "", err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != 200 {
			return "", "", fmt.Errorf("github returned status %d", resp.StatusCode)
		}
		var info struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Login string `json:"login"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&info); err == nil {
			n := info.Name
			if n == "" {
				n = info.Login
			}
			return n, info.Email, nil
		}
		return "", "", err

	case "gitea":
		if providerURL == "" {
			return "", "", fmt.Errorf("provider_url is required for Gitea")
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", providerURL+"/api/v1/user", nil)
		req.Header.Set("Authorization", "token "+token)
		resp, err := client.Do(req)
		if err != nil {
			return "", "", err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != 200 {
			return "", "", fmt.Errorf("gitea returned status %d", resp.StatusCode)
		}
		var info struct {
			FullName string `json:"full_name"`
			Email    string `json:"email"`
			Login    string `json:"login"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&info); err == nil {
			n := info.FullName
			if n == "" {
				n = info.Login
			}
			return n, info.Email, nil
		}
		return "", "", err

	default:
		return "", "", fmt.Errorf("user info fetch not supported for provider: %s", provider)
	}
}

// fetchUserInfoForCredential fetches user info from an external provider using the
// provided token, without saving anything. Used by the frontend to auto-populate
// username and email fields when adding a credential.
func fetchUserInfoForCredential(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		var req struct {
			Provider    string `json:"provider" binding:"required"`
			ProviderURL string `json:"provider_url" binding:"required"`
			Token       string `json:"token" binding:"required"`
			Username    string `json:"username"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provider, provider_url, and token are required"})
			return
		}

		// Resolve Vault reference if the token starts with "vault:"
		resolvedToken := req.Token
		if strings.HasPrefix(req.Token, "vault:") {
			tenantID := auth.GetTenantID(c)
			resolved, err := resolveVaultRef(deps, c.Request.Context(), req.Token, tenantID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to resolve Vault reference: %v", err)})
				return
			}
			resolvedToken = resolved
		}

		name, email, err := fetchExternalUserInfo(c.Request.Context(), req.Provider, req.ProviderURL, req.Username, resolvedToken)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to fetch user info: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"username": name,
			"email":    email,
		})
	}
}

// maskToken masks a token for display, showing only first 4 and last 4 chars.
func maskToken(tokenEnc string) string {
	// Try to decrypt first
	token, err := pepacrypto.Decrypt(tokenEnc)
	if err != nil {
		return "****"
	}
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "••••••••" + token[len(token)-4:]
}

// GetUserCredential retrieves a user's decrypted credential for a given provider.
// This is used by other handlers when they need to make external API calls on behalf of the user.
func GetUserCredential(ctx context.Context, deps Dependencies, userID uuid.UUID, provider, providerURL string) (token string, username string, email string, err error) {
	cred, err := deps.Repos.UserCredential.GetByProvider(ctx, userID, provider, providerURL)
	if err != nil {
		return "", "", "", fmt.Errorf("no credential found for user %s, provider %s: %w", userID, provider, err)
	}

	token, err = pepacrypto.Decrypt(cred.TokenEnc)
	if err != nil {
		return "", "", "", fmt.Errorf("decrypt credential: %w", err)
	}
	return token, cred.Username, cred.Email, nil
}
