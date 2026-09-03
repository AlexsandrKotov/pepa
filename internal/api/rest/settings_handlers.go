package rest

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/logging"
)

func registerSettingsRoutes(r *gin.RouterGroup, deps Dependencies) {
	settings := r.Group("/settings")
	{
		settings.GET("", listSettings(deps))
		settings.GET("/oidc/config", getOIDCAdminConfig(deps))
		settings.GET("/azure/config", getAzureADAdminConfig(deps))
		settings.GET("/google/config", getGoogleAdminConfig(deps))
		settings.GET("/github/config", getGitHubAdminConfig(deps))
		settings.GET("/ldap/config", getLDAPAdminConfig(deps))
		settings.POST("/ldap/test", testLDAPConnection(deps))
		settings.GET("/:key", getSetting(deps))
		settings.PUT("/:key", updateSetting(deps))
		settings.DELETE("/:key", deleteSetting(deps))
		settings.POST("/ai/test", testAIProvider(deps))
	}
}

// listSettings returns all settings grouped by key.
func listSettings(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Settings == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "settings repository not available"})
			return
		}
		all, err := deps.Repos.Settings.GetAll(c.Request.Context())
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if all == nil {
			all = map[string]json.RawMessage{}
		}
		c.JSON(http.StatusOK, gin.H{"settings": all})
	}
}

// getSetting returns a single setting by key.
func getSetting(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Settings == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "settings repository not available"})
			return
		}
		key := c.Param("key")
		value, err := deps.Repos.Settings.Get(c.Request.Context(), key)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"key": key, "value": value})
	}
}

// updateSetting saves a setting and applies runtime changes for known keys.
func updateSetting(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Settings == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "settings repository not available"})
			return
		}
		key := c.Param("key")

		var req struct {
			Value json.RawMessage `json:"value" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Apply runtime changes for known keys (before any sanitisation)
		switch key {
		case "ai":
			tenantID := auth.GetTenantID(c)
			applyAISettings(deps, c.Request.Context(), req.Value, tenantID)
		case "oidc":
			applyOIDCSettings(deps, req.Value)
		case "azure_ad":
			applyAzureADSettings(deps, req.Value)
		case "google":
			applyGoogleSettings(deps, req.Value)
		case "github":
			applyGitHubSettings(deps, req.Value)
		case "ldap":
			applyLDAPSettings(deps, req.Value)
		case "general":
			applyGeneralSettings(deps, req.Value)
		}

		// Strip secrets from values persisted in the DB so they are never
		// stored in plaintext. The runtime config holds the real secrets;
		// admin endpoints re-mask them on read.
		storeValue := req.Value
		switch key {
		case "oidc":
			storeValue = stripOIDCSecret(req.Value)
		case "azure_ad":
			storeValue = stripAzureSecret(req.Value)
		case "google":
			storeValue = stripGoogleSecret(req.Value)
		case "github":
			storeValue = stripGitHubSecret(req.Value)
		case "ldap":
			storeValue = stripLDAPSecret(req.Value)
		}

		// Persist to DB
		if err := deps.Repos.Settings.Set(c.Request.Context(), key, storeValue); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "setting", key, nil, gin.H{"key": key})

		c.JSON(http.StatusOK, gin.H{"key": key, "value": storeValue, "message": "saved"})
	}
}

// deleteSetting removes a setting.
func deleteSetting(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Settings == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "settings repository not available"})
			return
		}
		key := c.Param("key")
		if err := deps.Repos.Settings.Delete(c.Request.Context(), key); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "delete", "setting", key, nil, gin.H{"key": key})
		c.JSON(http.StatusOK, gin.H{"message": "deleted", "key": key})
	}
}

// testAIProvider tests an AI provider configuration without saving.
func testAIProvider(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Provider string `json:"provider" binding:"required"`
			APIKey   string `json:"api_key"`
			BaseURL  string `json:"base_url"`
			Model    string `json:"model"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		// Resolve vault references in the API key
		apiKey := req.APIKey
		if strings.HasPrefix(apiKey, "vault:") {
			tenantID := auth.GetTenantID(c)
			resolved, err := resolveVaultRef(deps, ctx, apiKey, tenantID)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"status": "error", "message": fmt.Sprintf("Cannot resolve vault reference: %v", err), "provider": req.Provider})
				return
			}
			apiKey = resolved
		}

		status := "connected"
		var message string

		switch req.Provider {
		case "openai":
			if apiKey == "" {
				status = "error"
				message = "API key required for OpenAI"
			} else {
				status, message = testOpenAIConnection(ctx, apiKey)
			}
		case "anthropic":
			if apiKey == "" {
				status = "error"
				message = "API key required for Anthropic"
			} else {
				message = "Anthropic configuration valid"
			}
		case "groq":
			if apiKey == "" {
				status = "error"
				message = "API key required for Groq"
			} else {
				status, message = testGroqConnection(ctx, apiKey)
			}
		case "qoder":
			if apiKey == "" {
				status = "error"
				message = "API key required for Qoder"
			} else {
				baseURL := req.BaseURL
				if baseURL == "" {
					baseURL = "https://api.qoder.com/v1"
				}
				client := &http.Client{Timeout: 10 * time.Second}
				httpReq, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
				httpReq.Header.Set("Authorization", "Bearer "+apiKey)
				resp, err := client.Do(httpReq)
				if err != nil {
					status = "error"
					message = fmt.Sprintf("Cannot reach Qoder: %v", err)
				} else {
					_ = resp.Body.Close()
					if resp.StatusCode == 200 {
						message = "Successfully connected to Qoder"
					} else if resp.StatusCode == 401 {
						status = "error"
						message = "Invalid API key"
					} else {
						status = "error"
						message = fmt.Sprintf("Qoder returned status %d", resp.StatusCode)
					}
				}
			}
		case "ollama":
			if req.BaseURL == "" {
				status = "error"
				message = "Base URL required for Ollama"
			} else {
				client := &http.Client{Timeout: 5 * time.Second}
				httpReq, _ := http.NewRequestWithContext(ctx, "GET", req.BaseURL+"/api/tags", nil)
				if apiKey != "" {
					httpReq.Header.Set("Authorization", "Bearer "+apiKey)
				}
				resp, err := client.Do(httpReq)
				if err != nil {
					status = "error"
					message = fmt.Sprintf("Cannot reach Ollama: %v", err)
				} else {
					_ = resp.Body.Close()
					if resp.StatusCode == 200 {
						message = "Successfully connected to Ollama"
					} else {
						status = "error"
						message = fmt.Sprintf("Ollama returned status %d", resp.StatusCode)
					}
				}
			}
		case "lmstudio":
			baseURL := req.BaseURL
			if baseURL == "" {
				baseURL = "http://host.docker.internal:1234/v1"
			}
			client := &http.Client{Timeout: 5 * time.Second}
			httpReq, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
			if apiKey != "" {
				httpReq.Header.Set("Authorization", "Bearer "+apiKey)
			}
			resp, err := client.Do(httpReq)
			if err != nil {
				status = "error"
				message = fmt.Sprintf("Cannot reach LM Studio: %v", err)
			} else {
				_ = resp.Body.Close()
				if resp.StatusCode == 200 {
					message = "Successfully connected to LM Studio"
				} else {
					status = "error"
					message = fmt.Sprintf("LM Studio returned status %d", resp.StatusCode)
				}
			}
		default:
			status = "error"
			message = fmt.Sprintf("Unknown provider: %s", req.Provider)
		}

		c.JSON(http.StatusOK, gin.H{
			"status":   status,
			"message":  message,
			"provider": req.Provider,
		})
	}
}

// testGroqConnection validates a Groq API key by calling the models endpoint.
func testGroqConnection(ctx context.Context, apiKey string) (string, string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.groq.com/openai/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("Cannot reach Groq: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 401 {
		return "error", "Invalid API key"
	}
	if resp.StatusCode == 200 {
		return "connected", "Successfully authenticated with Groq"
	}
	return "error", fmt.Sprintf("Groq returned status %d", resp.StatusCode)
}

// testOpenAIConnection validates an OpenAI API key by calling the models endpoint.
func testOpenAIConnection(ctx context.Context, apiKey string) (string, string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.openai.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("Cannot reach OpenAI: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 401 {
		return "error", "Invalid API key"
	}
	if resp.StatusCode == 200 {
		return "connected", "Successfully authenticated with OpenAI"
	}
	return "error", fmt.Sprintf("OpenAI returned status %d", resp.StatusCode)
}

// applyAISettings reads the AI settings JSON and configures the AI manager.
func applyAISettings(deps Dependencies, ctx context.Context, value json.RawMessage, tenantID uuid.UUID) {
	if deps.AIManager == nil {
		return
	}
	var aiSettings struct {
		Enabled         bool   `json:"enabled"`
		DefaultProvider string `json:"default_provider"`
		Providers       map[string]struct {
			APIKey  string `json:"api_key"`
			BaseURL string `json:"base_url"`
			Model   string `json:"model"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(value, &aiSettings); err != nil {
		return
	}
	if !aiSettings.Enabled {
		return
	}
	for name, cfg := range aiSettings.Providers {
		if cfg.APIKey != "" || cfg.BaseURL != "" {
			apiKey := cfg.APIKey
			// Resolve vault references
			if strings.HasPrefix(apiKey, "vault:") {
				resolved, err := resolveVaultRef(deps, ctx, apiKey, tenantID)
				if err != nil {
					continue // skip this provider if vault resolution fails
				}
				apiKey = resolved
			}
			_ = deps.AIManager.ConfigureProvider(name, apiKey, cfg.BaseURL, cfg.Model)
		}
	}
	if aiSettings.DefaultProvider != "" {
		deps.AIManager.SetDefaultProvider(aiSettings.DefaultProvider)
	}
}

// applyOIDCSettings reads the OIDC settings JSON and updates the runtime config.
func applyOIDCSettings(deps Dependencies, value json.RawMessage) {
	var oidcSettings struct {
		Enabled       bool     `json:"enabled"`
		Issuer        string   `json:"issuer"`
		ClientID      string   `json:"client_id"`
		ClientSecret  string   `json:"client_secret"`
		RedirectURL   string   `json:"redirect_url"`
		Scopes        []string `json:"scopes"`
	}
	if err := json.Unmarshal(value, &oidcSettings); err != nil {
		slog.Error("failed to unmarshal OIDC settings", "error", err)
		return
	}
	deps.Config.Auth.OIDC.Enabled = oidcSettings.Enabled
	deps.Config.Auth.OIDC.Issuer = oidcSettings.Issuer
	deps.Config.Auth.OIDC.ClientID = oidcSettings.ClientID
	// If the client_secret contains mask characters, keep the existing secret
	if oidcSettings.ClientSecret != "" && !strings.Contains(oidcSettings.ClientSecret, "\u2022") {
		deps.Config.Auth.OIDC.ClientSecret = oidcSettings.ClientSecret
	}
	deps.Config.Auth.OIDC.RedirectURL = oidcSettings.RedirectURL
	if len(oidcSettings.Scopes) > 0 {
		deps.Config.Auth.OIDC.Scopes = oidcSettings.Scopes
	}
	slog.Info("OIDC settings updated at runtime", "enabled", oidcSettings.Enabled, "issuer", oidcSettings.Issuer)
}

// stripOIDCSecret removes client_secret from the JSON so it is not persisted in the DB.
func stripOIDCSecret(value json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(value, &m); err != nil {
		return value
	}
	delete(m, "client_secret")
	out, err := json.Marshal(m)
	if err != nil {
		return value
	}
	return out
}

// getOIDCAdminConfig returns the current OIDC configuration for the admin settings page.
// The client_secret is masked for security.
func getOIDCAdminConfig(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		oidc := deps.Config.Auth.OIDC
		maskedSecret := ""
		if oidc.ClientSecret != "" {
			if len(oidc.ClientSecret) > 4 {
				maskedSecret = oidc.ClientSecret[:4] + "••••••••"
			} else {
				maskedSecret = "••••••••" //nolint:gosec // #nosec // G101: mask string, not a credential
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"enabled":       oidc.Enabled,
			"issuer":        oidc.Issuer,
			"client_id":     oidc.ClientID,
			"client_secret": maskedSecret,
			"redirect_url":  oidc.RedirectURL,
			"scopes":        oidc.Scopes,
		})
	}
}

// ── Azure AD settings ────────────────────────────────────────────────────────

// applyAzureADSettings reads the Azure AD settings JSON and updates the runtime config.
func applyAzureADSettings(deps Dependencies, value json.RawMessage) {
	var azureSettings struct {
		Enabled      bool   `json:"enabled"`
		TenantID     string `json:"tenant_id"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RedirectURL  string `json:"redirect_url"`
	}
	if err := json.Unmarshal(value, &azureSettings); err != nil {
		slog.Error("failed to unmarshal Azure AD settings", "error", err)
		return
	}
	deps.Config.Auth.AzureAD.Enabled = azureSettings.Enabled
	deps.Config.Auth.AzureAD.TenantID = azureSettings.TenantID
	deps.Config.Auth.AzureAD.ClientID = azureSettings.ClientID
	// If the client_secret contains mask characters, keep the existing secret
	if azureSettings.ClientSecret != "" && !strings.Contains(azureSettings.ClientSecret, "\u2022") {
		deps.Config.Auth.AzureAD.ClientSecret = azureSettings.ClientSecret
	}
	deps.Config.Auth.AzureAD.RedirectURL = azureSettings.RedirectURL
	slog.Info("Azure AD settings updated at runtime", "enabled", azureSettings.Enabled, "tenant_id", azureSettings.TenantID)
}

// stripAzureSecret removes client_secret from the JSON so it is not persisted in the DB.
func stripAzureSecret(value json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(value, &m); err != nil {
		return value
	}
	delete(m, "client_secret")
	out, err := json.Marshal(m)
	if err != nil {
		return value
	}
	return out
}

// getAzureADAdminConfig returns the current Azure AD configuration for the admin settings page.
// The client_secret is masked for security.
func getAzureADAdminConfig(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		azure := deps.Config.Auth.AzureAD
		maskedSecret := ""
		if azure.ClientSecret != "" {
			if len(azure.ClientSecret) > 4 {
				maskedSecret = azure.ClientSecret[:4] + "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022"
			} else {
				maskedSecret = "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022" //nolint:gosec // #nosec // G101: mask string, not a credential
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"enabled":       azure.Enabled,
			"tenant_id":     azure.TenantID,
			"client_id":     azure.ClientID,
			"client_secret": maskedSecret,
			"redirect_url":  azure.RedirectURL,
		})
	}
}

// ── Google OAuth settings ─────────────────────────────────────────────────────

// applyGoogleSettings reads the Google OAuth settings JSON and updates the runtime config.
func applyGoogleSettings(deps Dependencies, value json.RawMessage) {
	var googleSettings struct {
		Enabled      bool   `json:"enabled"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RedirectURL  string `json:"redirect_url"`
	}
	if err := json.Unmarshal(value, &googleSettings); err != nil {
		slog.Error("failed to unmarshal Google settings", "error", err)
		return
	}
	deps.Config.Auth.Google.Enabled = googleSettings.Enabled
	deps.Config.Auth.Google.ClientID = googleSettings.ClientID
	// If the client_secret contains mask characters, keep the existing secret
	if googleSettings.ClientSecret != "" && !strings.Contains(googleSettings.ClientSecret, "\u2022") {
		deps.Config.Auth.Google.ClientSecret = googleSettings.ClientSecret
	}
	deps.Config.Auth.Google.RedirectURL = googleSettings.RedirectURL
	slog.Info("Google OAuth settings updated at runtime", "enabled", googleSettings.Enabled)
}

// stripGoogleSecret removes client_secret from the JSON so it is not persisted in the DB.
func stripGoogleSecret(value json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(value, &m); err != nil {
		return value
	}
	delete(m, "client_secret")
	out, err := json.Marshal(m)
	if err != nil {
		return value
	}
	return out
}

// getGoogleAdminConfig returns the current Google OAuth configuration for the admin settings page.
// The client_secret is masked for security.
func getGoogleAdminConfig(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		google := deps.Config.Auth.Google
		maskedSecret := ""
		if google.ClientSecret != "" {
			if len(google.ClientSecret) > 4 {
				maskedSecret = google.ClientSecret[:4] + "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022"
			} else {
				maskedSecret = "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022" //nolint:gosec // #nosec // G101: mask string, not a credential
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"enabled":       google.Enabled,
			"client_id":     google.ClientID,
			"client_secret": maskedSecret,
			"redirect_url":  google.RedirectURL,
		})
	}
}

// ── GitHub OAuth settings ─────────────────────────────────────────────────────

// applyGitHubSettings reads the GitHub OAuth settings JSON and updates the runtime config.
func applyGitHubSettings(deps Dependencies, value json.RawMessage) {
	var githubSettings struct {
		Enabled      bool   `json:"enabled"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RedirectURL  string `json:"redirect_url"`
	}
	if err := json.Unmarshal(value, &githubSettings); err != nil {
		slog.Error("failed to unmarshal GitHub settings", "error", err)
		return
	}
	deps.Config.Auth.GitHub.Enabled = githubSettings.Enabled
	deps.Config.Auth.GitHub.ClientID = githubSettings.ClientID
	// If the client_secret contains mask characters, keep the existing secret
	if githubSettings.ClientSecret != "" && !strings.Contains(githubSettings.ClientSecret, "\u2022") {
		deps.Config.Auth.GitHub.ClientSecret = githubSettings.ClientSecret
	}
	deps.Config.Auth.GitHub.RedirectURL = githubSettings.RedirectURL
	slog.Info("GitHub OAuth settings updated at runtime", "enabled", githubSettings.Enabled)
}

// stripGitHubSecret removes client_secret from the JSON so it is not persisted in the DB.
func stripGitHubSecret(value json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(value, &m); err != nil {
		return value
	}
	delete(m, "client_secret")
	out, err := json.Marshal(m)
	if err != nil {
		return value
	}
	return out
}

// getGitHubAdminConfig returns the current GitHub OAuth configuration for the admin settings page.
// The client_secret is masked for security.
func getGitHubAdminConfig(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		github := deps.Config.Auth.GitHub
		maskedSecret := ""
		if github.ClientSecret != "" {
			if len(github.ClientSecret) > 4 {
				maskedSecret = github.ClientSecret[:4] + "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022"
			} else {
				maskedSecret = "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022" //nolint:gosec // #nosec // G101: mask string, not a credential
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"enabled":       github.Enabled,
			"client_id":     github.ClientID,
			"client_secret": maskedSecret,
			"redirect_url":  github.RedirectURL,
		})
	}
}

// ── LDAP settings ────────────────────────────────────────────────────────────

// applyLDAPSettings reads the LDAP settings JSON and updates the runtime config.
func applyLDAPSettings(deps Dependencies, value json.RawMessage) {
	var ldapSettings struct {
		Enabled            bool              `json:"enabled"`
		URL                string            `json:"url"`
		BindDN             string            `json:"bind_dn"`
		BindPassword       string            `json:"bind_password"`
		BaseDN             string            `json:"base_dn"`
		UserFilter         string            `json:"user_filter"`
		GroupFilter        string            `json:"group_filter"`
		EmailAttr          string            `json:"email_attr"`
		NameAttr           string            `json:"name_attr"`
		StartTLS           bool              `json:"start_tls"`
		InsecureSkipVerify bool              `json:"insecure_skip_verify"`
		CACertificate      string            `json:"ca_certificate"`
		GroupMapping       map[string]string `json:"group_mapping"`
	}
	if err := json.Unmarshal(value, &ldapSettings); err != nil {
		slog.Error("failed to unmarshal LDAP settings", "error", err)
		return
	}
	deps.Config.Auth.LDAP.Enabled = ldapSettings.Enabled
	deps.Config.Auth.LDAP.URL = ldapSettings.URL
	deps.Config.Auth.LDAP.BindDN = ldapSettings.BindDN
	// If the bind_password contains mask characters, keep the existing password
	if ldapSettings.BindPassword != "" && !strings.Contains(ldapSettings.BindPassword, "•") {
		deps.Config.Auth.LDAP.BindPassword = ldapSettings.BindPassword
	}
	deps.Config.Auth.LDAP.BaseDN = ldapSettings.BaseDN
	deps.Config.Auth.LDAP.UserFilter = ldapSettings.UserFilter
	deps.Config.Auth.LDAP.GroupFilter = ldapSettings.GroupFilter
	deps.Config.Auth.LDAP.EmailAttr = ldapSettings.EmailAttr
	deps.Config.Auth.LDAP.NameAttr = ldapSettings.NameAttr
	deps.Config.Auth.LDAP.StartTLS = ldapSettings.StartTLS
	deps.Config.Auth.LDAP.InsecureSkipVerify = ldapSettings.InsecureSkipVerify
	// If the ca_certificate contains mask characters, keep the existing certificate
	if ldapSettings.CACertificate != "" && !strings.Contains(ldapSettings.CACertificate, "•") {
		deps.Config.Auth.LDAP.CACertificate = ldapSettings.CACertificate
	}
	if ldapSettings.GroupMapping != nil {
		deps.Config.Auth.LDAP.GroupMapping = ldapSettings.GroupMapping
	}
	slog.Info("LDAP settings updated at runtime", "enabled", ldapSettings.Enabled, "url", ldapSettings.URL)
}

// applyGeneralSettings reads the general platform settings and applies runtime changes.
func applyGeneralSettings(deps Dependencies, value json.RawMessage) {
	var generalSettings struct {
		PlatformName string `json:"platform_name"`
		BaseURL      string `json:"base_url"`
		LogLevel     string `json:"log_level"`
		CORSOrigins  string `json:"cors_origins"`
	}
	if err := json.Unmarshal(value, &generalSettings); err != nil {
		slog.Error("failed to unmarshal general settings", "error", err)
		return
	}

	// Apply platform name and base URL to runtime config
	if generalSettings.PlatformName != "" {
		deps.Config.Server.PlatformName = generalSettings.PlatformName
	}
	if generalSettings.BaseURL != "" {
		deps.Config.Server.BaseURL = generalSettings.BaseURL
	}

	// Apply log level change at runtime (validate against accepted values)
	if generalSettings.LogLevel != "" && generalSettings.LogLevel != deps.Config.Server.LogLevel {
		switch strings.ToLower(generalSettings.LogLevel) {
		case "debug", "info", "warn", "error":
			deps.Config.Server.LogLevel = generalSettings.LogLevel
			logging.SetLevel(generalSettings.LogLevel)
			slog.Info("log level changed at runtime", "level", generalSettings.LogLevel)
		default:
			slog.Warn("invalid log level rejected", "level", generalSettings.LogLevel)
		}
	}

	// Apply CORS origins change at runtime (trim whitespace from each origin)
	if generalSettings.CORSOrigins != "" {
		rawOrigins := strings.Split(generalSettings.CORSOrigins, ",")
		newOrigins := make([]string, 0, len(rawOrigins))
		for _, o := range rawOrigins {
			trimmed := strings.TrimSpace(o)
			if trimmed != "" {
				newOrigins = append(newOrigins, trimmed)
			}
		}
		deps.Config.CORS.Origins = newOrigins
		slog.Info("CORS origins updated at runtime", "origins", newOrigins)
	}
}

// stripLDAPSecret removes bind_password and ca_certificate from the JSON so they are not persisted in the DB.
func stripLDAPSecret(value json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(value, &m); err != nil {
		return value
	}
	delete(m, "bind_password")
	delete(m, "ca_certificate")
	out, err := json.Marshal(m)
	if err != nil {
		return value
	}
	return out
}

// getLDAPAdminConfig returns the current LDAP configuration for the admin settings page.
// The bind_password and ca_certificate are masked for security.
func getLDAPAdminConfig(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ldap := deps.Config.Auth.LDAP
		maskedPassword := ""
		if ldap.BindPassword != "" {
			if len(ldap.BindPassword) > 4 {
				maskedPassword = ldap.BindPassword[:4] + "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022"
			} else {
				maskedPassword = "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022" //nolint:gosec // #nosec // G101: mask string, not a credential
			}
		}
		maskedCACert := ""
		if ldap.CACertificate != "" {
			maskedCACert = "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022" //nolint:gosec // #nosec // G101: mask string, not a credential
		}
		c.JSON(http.StatusOK, gin.H{
			"enabled":             ldap.Enabled,
			"url":                 ldap.URL,
			"bind_dn":             ldap.BindDN,
			"bind_password":       maskedPassword,
			"base_dn":             ldap.BaseDN,
			"user_filter":         ldap.UserFilter,
			"group_filter":        ldap.GroupFilter,
			"email_attr":          ldap.EmailAttr,
			"name_attr":           ldap.NameAttr,
			"start_tls":           ldap.StartTLS,
			"insecure_skip_verify": ldap.InsecureSkipVerify,
			"ca_certificate":      maskedCACert,
			"group_mapping":       ldap.GroupMapping,
		})
	}
}

// testLDAPConnection tests an LDAP connection with the provided or current configuration.
func testLDAPConnection(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			URL                string `json:"url"`
			BindDN             string `json:"bind_dn"`
			BindPassword       string `json:"bind_password"`
			BaseDN             string `json:"base_dn"`
			StartTLS           bool   `json:"start_tls"`
			InsecureSkipVerify bool   `json:"insecure_skip_verify"`
			CACertificate      string `json:"ca_certificate"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Use provided values or fall back to runtime config
		cfg := deps.Config.Auth.LDAP
		if req.URL != "" {
			cfg.URL = req.URL
		}
		if req.BindDN != "" {
			cfg.BindDN = req.BindDN
		}
		if req.BindPassword != "" && !strings.Contains(req.BindPassword, "\u2022") {
			cfg.BindPassword = req.BindPassword
		}
		if req.BaseDN != "" {
			cfg.BaseDN = req.BaseDN
		}
		cfg.StartTLS = req.StartTLS
		cfg.InsecureSkipVerify = req.InsecureSkipVerify
		if req.CACertificate != "" && !strings.Contains(req.CACertificate, "\u2022") {
			cfg.CACertificate = req.CACertificate
		}

		if cfg.URL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "LDAP server URL is required"})
			return
		}

		ldapProvider := auth.NewLDAPProvider(cfg)
		if err := ldapProvider.TestConnection(c.Request.Context()); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("LDAP connection failed: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "connected",
			"message": "Successfully connected to LDAP server",
		})
	}
}

// resolveVaultRef resolves a vault reference (e.g. "vault:path/to/secret/key") to its actual value.
func resolveVaultRef(deps Dependencies, ctx context.Context, ref string, tenantID uuid.UUID) (string, error) {
	// Strip "vault:" prefix
	raw := strings.TrimPrefix(ref, "vault:")
	if raw == "" {
		return "", fmt.Errorf("empty vault reference")
	}

	// Split into path and key: last segment is the key
	idx := strings.LastIndex(raw, "/")
	if idx < 0 {
		return "", fmt.Errorf("invalid vault reference: %s", ref)
	}
	secretPath := raw[:idx]
	secretKey := raw[idx+1:]

	vaultCfg := getVaultConfig(deps, ctx)
	if vaultCfg.Mode == "remote" {
		client := newRemoteClient(vaultCfg)
		if client == nil {
			return "", fmt.Errorf("remote vault not configured")
		}
		secret, err := client.GetSecret(ctx, secretPath)
		if err != nil {
			return "", fmt.Errorf("cannot read vault secret: %w", err)
		}
		val, ok := secret.Data[secretKey]
		if !ok {
			return "", fmt.Errorf("key %q not found in vault secret %q", secretKey, secretPath)
		}
		return val, nil
	}

	// Local mode
	if deps.Repos.Vault == nil {
		return "", fmt.Errorf("vault not configured")
	}
	secret, err := deps.Repos.Vault.Get(ctx, tenantID, secretPath)
	if err != nil {
		return "", fmt.Errorf("cannot read vault secret: %w", err)
	}
	val, ok := secret.Data[secretKey]
	if !ok {
		return "", fmt.Errorf("key %q not found in vault secret %q", secretKey, secretPath)
	}
	return val, nil
}
