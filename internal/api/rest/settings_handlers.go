package rest

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
)

func registerSettingsRoutes(r *gin.RouterGroup, deps Dependencies) {
	settings := r.Group("/settings")
	{
		settings.GET("", listSettings(deps))
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

		// Persist to DB
		if err := deps.Repos.Settings.Set(c.Request.Context(), key, req.Value); err != nil {
			respondInternalError(c, err)
			return
		}

		// Apply runtime changes for known keys
		switch key {
		case "ai":
			tenantID := auth.GetTenantID(c)
			applyAISettings(deps, c.Request.Context(), req.Value, tenantID)
		}

		logAudit(deps, c, "update", "setting", key, nil, gin.H{"key": key})

		c.JSON(http.StatusOK, gin.H{"key": key, "value": req.Value, "message": "saved"})
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
					resp.Body.Close()
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
					resp.Body.Close()
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
				resp.Body.Close()
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
