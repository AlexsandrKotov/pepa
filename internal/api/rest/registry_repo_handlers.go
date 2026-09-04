package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
)

func registerRegistryRepoRoutes(r *gin.RouterGroup, deps Dependencies) {
	regRepos := r.Group("/registry-repositories")
	{
		regRepos.GET("", listRegistryRepos(deps))
		regRepos.POST("", createRegistryRepo(deps))
		regRepos.GET("/:id", getRegistryRepo(deps))
		regRepos.PUT("/:id", updateRegistryRepo(deps))
		regRepos.DELETE("/:id", deleteRegistryRepo(deps))
		// Image/tag listing via Docker Registry HTTP API V2
		regRepos.GET("/:id/images", listRegistryImages(deps))
		// Use wildcard param to support image names with slashes (e.g. library/nginx)
		regRepos.GET("/:id/images/*imageName", listRegistryImageTags(deps))
	}
}

func listRegistryRepos(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Registry == nil {
			c.JSON(http.StatusOK, gin.H{"registry_repositories": []interface{}{}, "total": 0})
			return
		}
		tenantID := auth.GetTenantID(c)
		items, err := deps.Repos.Registry.List(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		for i := range items {
			items[i].Password = ""
			items[i].Token = ""
		}
		c.JSON(http.StatusOK, gin.H{"registry_repositories": items, "total": len(items)})
	}
}

func createRegistryRepo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Registry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "registry repository not available"})
			return
		}
		var req struct {
			Name         string `json:"name" binding:"required"`
			Description  string `json:"description"`
			RegistryType string `json:"registry_type" binding:"required"`
			URL          string `json:"url" binding:"required"`
			Username     string `json:"username"`
			Password     string `json:"password"`
			Token        string `json:"token"`
			IsDefault    bool   `json:"is_default"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		validTypes := map[string]bool{
			"docker": true, "ghcr": true, "harbor": true,
			"ecr": true, "gcr": true, "acr": true, "other": true,
		}
		if !validTypes[req.RegistryType] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "registry_type must be one of: docker, ghcr, harbor, ecr, gcr, acr, other"})
			return
		}

		if err := validateRegistryURL(req.URL); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		tenantID := auth.GetTenantID(c)
		repo := &repository.RegistryRepo{
			TenantID:     tenantID,
			Name:         req.Name,
			Description:  req.Description,
			RegistryType: req.RegistryType,
			URL:          req.URL,
			Username:     req.Username,
			Password:     req.Password,
			Token:        req.Token,
			IsDefault:    req.IsDefault,
			Status:       "active",
		}

		if err := deps.Repos.Registry.Create(c.Request.Context(), repo); err != nil {
			respondInternalError(c, err)
			return
		}

		repo.Password = ""
		repo.Token = ""
		logAudit(deps, c, "create", "registry_repository", repo.ID.String(), nil, gin.H{"name": repo.Name, "url": repo.URL})
		c.JSON(http.StatusCreated, repo)
	}
}

func getRegistryRepo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Registry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "registry repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		repo, err := deps.Repos.Registry.Get(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "registry repository not found"})
			return
		}
		repo.Password = ""
		repo.Token = ""
		c.JSON(http.StatusOK, repo)
	}
}

func updateRegistryRepo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Registry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "registry repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		existing, err := deps.Repos.Registry.Get(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "registry repository not found"})
			return
		}

		var req struct {
			Name         string  `json:"name"`
			Description  string  `json:"description"`
			RegistryType string  `json:"registry_type"`
			URL          string  `json:"url"`
			Username     *string `json:"username"`
			Password     *string `json:"password"`
			Token        *string `json:"token"`
			IsDefault    bool    `json:"is_default"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Name != "" {
			existing.Name = req.Name
		}
		if req.Description != "" {
			existing.Description = req.Description
		}
		if req.RegistryType != "" {
			existing.RegistryType = req.RegistryType
		}
		if req.URL != "" {
			if err := validateRegistryURL(req.URL); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			existing.URL = req.URL
		}
		if req.Username != nil {
			existing.Username = *req.Username
		}
		if req.Password != nil {
			existing.Password = *req.Password
		}
		if req.Token != nil {
			existing.Token = *req.Token
		}
		existing.IsDefault = req.IsDefault

		if err := deps.Repos.Registry.Update(c.Request.Context(), existing); err != nil {
			respondInternalError(c, err)
			return
		}
		existing.Password = ""
		existing.Token = ""
		logAudit(deps, c, "update", "registry_repository", existing.ID.String(), nil, gin.H{"name": existing.Name, "url": existing.URL})
		c.JSON(http.StatusOK, existing)
	}
}

func deleteRegistryRepo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Registry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "registry repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		if err := deps.Repos.Registry.Delete(c.Request.Context(), id, tenantID); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "delete", "registry_repository", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"status": "deleted"})
	}
}

// validateRegistryURL performs safety checks on a registry URL to prevent SSRF.
// Blocks cloud metadata endpoints, link-local, loopback, and private network ranges.
//
// Environment variables (consistent with Vault's VAULT_ALLOW_PRIVATE_IPS / VAULT_ALLOWED_CIDRS):
//   - REGISTRY_ALLOW_PRIVATE_IPS=true  — full bypass (dev/docker)
//   - REGISTRY_ALLOWED_CIDRS=10.0.0.0/8,172.16.0.0/12 — granular allowlist (production)
func validateRegistryURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http and https schemes are allowed")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL must include a hostname")
	}

	// Full bypass — dev/docker mode.
	if os.Getenv("REGISTRY_ALLOW_PRIVATE_IPS") == "true" {
		return nil
	}

	// Resolve hostname to IPs.
	ips, lookupErr := net.LookupIP(host)
	if lookupErr != nil {
		// DNS resolution failed — log but allow (the registry client will fail
		// later if truly unreachable). This matches the original behavior.
		slog.Debug("registry URL DNS lookup failed, allowing", "host", host, "error", lookupErr)
		return nil
	}

	// Parse granular CIDR allowlist (production-safe alternative to full bypass).
	var allowedNetworks []*net.IPNet
	if cidrs := os.Getenv("REGISTRY_ALLOWED_CIDRS"); cidrs != "" {
		for _, cidr := range strings.Split(cidrs, ",") {
			cidr = strings.TrimSpace(cidr)
			_, network, parseErr := net.ParseCIDR(cidr)
			if parseErr != nil {
				slog.Warn("invalid REGISTRY_ALLOWED_CIDRS entry", "cidr", cidr, "error", parseErr)
				continue
			}
			allowedNetworks = append(allowedNetworks, network)
		}
	}

	for _, ip := range ips {
		if ip == nil {
			continue
		}

		// Check if IP matches a granular allowlist entry — if so, skip block checks.
		if ipInNetworks(ip, allowedNetworks) {
			continue
		}

		// Always block cloud metadata IP (169.254.169.254) unless full bypass.
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("URL must not target link-local or metadata endpoints")
		}
		if ip.IsLoopback() {
			return fmt.Errorf("URL must not target loopback addresses (set REGISTRY_ALLOW_PRIVATE_IPS=true for local registries)")
		}
		if ip.IsPrivate() {
			return fmt.Errorf("URL must not target private network addresses (set REGISTRY_ALLOWED_CIDRS or REGISTRY_ALLOW_PRIVATE_IPS for internal registries)")
		}
	}
	return nil
}

// ipInNetworks returns true if ip is contained in any of the given networks.
func ipInNetworks(ip net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// ── Docker Registry HTTP API V2 ───────────────────────────────

// registryAuth sets authentication headers on a request for a Docker registry.
// Priority: token (PAT, designed for API auth) > username+password (fallback).
func registryAuth(req *http.Request, repo *repository.RegistryRepo) {
	if repo.Token != "" {
		if repo.Username != "" {
			// Token + username: use Basic Auth (e.g. GitLab PAT, GHCR PAT)
			req.SetBasicAuth(repo.Username, repo.Token)
		} else {
			// Token only: use Bearer
			req.Header.Set("Authorization", "Bearer "+repo.Token)
		}
	} else if repo.Username != "" && repo.Password != "" {
		// Username + password: standard Basic Auth (same as docker login)
		req.SetBasicAuth(repo.Username, repo.Password)
	}
}

// authChallenge holds the parsed Www-Authenticate challenge from a registry.
type authChallenge struct {
	realm   string
	service string
}

// getAuthChallenge pings /v2/ and parses the Www-Authenticate header.
// Returns nil if no token exchange is needed (registry accepts credentials directly).
func getAuthChallenge(repo *repository.RegistryRepo) (*authChallenge, error) {
	baseURL := strings.TrimSuffix(repo.URL, "/")
	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequest("GET", baseURL+"/v2/", nil)
	if err != nil {
		return nil, fmt.Errorf("create ping request: %w", err)
	}
	registryAuth(req, repo)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registry ping failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil, nil // No token needed
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	wwwAuth := resp.Header.Get("Www-Authenticate")
	if wwwAuth == "" {
		return nil, fmt.Errorf("no Www-Authenticate header in 401 response")
	}

	realm := extractAuthParam(wwwAuth, "realm")
	if realm == "" {
		return nil, fmt.Errorf("no realm in Www-Authenticate header")
	}

	return &authChallenge{
		realm:   realm,
		service: extractAuthParam(wwwAuth, "service"),
	}, nil
}

// maxBodyRead limits response body reads from external registries to prevent
// resource exhaustion from malicious or misconfigured servers.
const maxBodyRead = 1 << 20 // 1 MB

// validateRealmURL checks that an auth realm URL is safe to request.
// Prevents SSRF via a malicious Www-Authenticate realm pointing to internal services.
func validateRealmURL(realm string) error {
	u, err := url.Parse(realm)
	if err != nil {
		return fmt.Errorf("invalid realm URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("realm scheme must be http or https, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("realm must include a hostname")
	}
	// In dev/docker mode, skip IP checks.
	if os.Getenv("REGISTRY_ALLOW_PRIVATE_IPS") == "true" {
		return nil
	}
	ips, lookupErr := net.LookupIP(host)
	if lookupErr != nil {
		// DNS failure — allow (the HTTP client will fail anyway).
		return nil
	}
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("realm must not target link-local or metadata endpoints")
		}
		if ip.IsLoopback() {
			return fmt.Errorf("realm must not target loopback addresses")
		}
		if ip.IsPrivate() {
			return fmt.Errorf("realm must not target private network addresses")
		}
	}
	return nil
}

// getScopedToken requests a JWT token from the registry auth endpoint with a specific scope.
func getScopedToken(challenge *authChallenge, repo *repository.RegistryRepo, scope string) (string, error) {
	// Validate the realm URL to prevent SSRF via malicious Www-Authenticate headers.
	if err := validateRealmURL(challenge.realm); err != nil {
		return "", fmt.Errorf("unsafe auth realm: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}

	tokenURL := challenge.realm + "?"
	if challenge.service != "" {
		tokenURL += "service=" + url.QueryEscape(challenge.service) + "&"
	}
	if scope != "" {
		tokenURL += "scope=" + url.QueryEscape(scope)
	}

	tokenReq, err := http.NewRequest("GET", tokenURL, nil)
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	registryAuth(tokenReq, repo)

	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer tokenResp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(tokenResp.Body, maxBodyRead))
	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d: %s", tokenResp.StatusCode, string(body)[:min(len(body), 200)])
	}

	var tokenData struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenData); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if tokenData.Token != "" {
		return tokenData.Token, nil
	}
	return tokenData.AccessToken, nil
}

// getRegistryToken performs Docker Registry v2 authentication.
// Returns a JWT token for accessing the registry, or empty string if no token needed.
func getRegistryToken(repo *repository.RegistryRepo) (string, error) {
	challenge, err := getAuthChallenge(repo)
	if err != nil {
		return "", err
	}
	if challenge == nil {
		return "", nil // No token needed
	}
	return getScopedToken(challenge, repo, "")
}

// gitlabAPIBaseURL extracts the GitLab API base URL from the JWT realm.
// e.g., "https://gitlab.example.com/jwt/auth" → "https://gitlab.example.com"
func gitlabAPIBaseURL(realm string) string {
	u, err := url.Parse(realm)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}

// gitlabAuth sets authentication on a GitLab API request.
// Tries PRIVATE-TOKEN header first (for PAT), falls back to Basic Auth.
func gitlabAuth(req *http.Request, repo *repository.RegistryRepo) {
	if repo.Token != "" {
		req.Header.Set("PRIVATE-TOKEN", repo.Token)
	} else if repo.Username != "" && repo.Password != "" {
		req.SetBasicAuth(repo.Username, repo.Password)
	}
}

// listGitLabContainerRepos uses the GitLab API to discover container repositories.
// This is needed because GitLab restricts /v2/_catalog to admin users.
func listGitLabContainerRepos(realm string, repo *repository.RegistryRepo) ([]string, error) {
	gitlabBase := gitlabAPIBaseURL(realm)
	if gitlabBase == "" {
		return nil, fmt.Errorf("cannot derive GitLab URL from realm: %s", realm)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	var allRepos []string

	// Step 1: Get user's projects
	page := 1
	for {
		projectsURL := fmt.Sprintf("%s/api/v4/projects?per_page=100&page=%d&simple=true&order_by=name", gitlabBase, page)
		req, _ := http.NewRequest("GET", projectsURL, nil)
		gitlabAuth(req, repo)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("gitlab projects request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyRead))
			resp.Body.Close()
			return nil, fmt.Errorf("gitlab projects returned %d: %s", resp.StatusCode, string(body)[:min(len(body), 200)])
		}

		var projects []struct {
			ID int `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("parse projects: %w", err)
		}
		resp.Body.Close()

		if len(projects) == 0 {
			break
		}

		// Step 2: For each project, get container repositories
		for _, proj := range projects {
			reposURL := fmt.Sprintf("%s/api/v4/projects/%d/registry/repositories", gitlabBase, proj.ID)
			req, _ := http.NewRequest("GET", reposURL, nil)
			gitlabAuth(req, repo)

			resp, err := client.Do(req)
			if err != nil {
				slog.Warn("gitlab registry repos request failed", "project_id", proj.ID, "error", err)
				continue
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				continue
			}

			var repos []struct {
				Path string `json:"path"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&repos); err == nil {
				for _, r := range repos {
					if r.Path != "" {
						allRepos = append(allRepos, r.Path)
					}
				}
			}
			resp.Body.Close()
		}

		if len(projects) < 100 {
			break
		}
		page++
	}

	return allRepos, nil
}

// extractAuthParam extracts a parameter value from a Www-Authenticate header.
// e.g., extractAuthParam(`Bearer realm="https://auth.docker.io/token",service="registry.docker.io"`, "realm")
// returns "https://auth.docker.io/token"
func extractAuthParam(header, param string) string {
	// Look for param="value"
	search := param + `="`
	idx := strings.Index(header, search)
	if idx < 0 {
		return ""
	}
	start := idx + len(search)
	end := strings.Index(header[start:], `"`)
	if end < 0 {
		return ""
	}
	return header[start : start+end]
}

// listRegistryImages returns images (repositories) from a container registry.
// For GitLab registries, falls back to GitLab API when /v2/_catalog fails
// (GitLab restricts catalog listing to admin users).
func listRegistryImages(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Registry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "registry repository not available"})
			return
		}

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		tenantID := auth.GetTenantID(c)
		repo, err := deps.Repos.Registry.GetDecrypted(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("registry repository not found: %v", err)})
			return
		}

		baseURL := strings.TrimSuffix(repo.URL, "/")
		client := &http.Client{Timeout: 30 * time.Second}

		// Step 1: Get auth challenge from registry
		challenge, challengeErr := getAuthChallenge(repo)
		if challengeErr != nil {
			slog.Warn("registry auth challenge failed", "id", id, "error", challengeErr.Error())
		}

		// Step 2: Try /v2/_catalog with a catalog-scoped token
		var repositories []string
		catalogOK := false

		if challenge != nil {
			token, tokenErr := getScopedToken(challenge, repo, "registry:catalog:*")
			if tokenErr == nil && token != "" {
				req, _ := http.NewRequest("GET", baseURL+"/v2/_catalog", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				resp, err := client.Do(req)
				if err == nil {
					if resp.StatusCode == http.StatusOK {
						var catalog struct {
							Repositories []string `json:"repositories"`
						}
						if err := json.NewDecoder(resp.Body).Decode(&catalog); err == nil {
							repositories = catalog.Repositories
							catalogOK = true
						}
					}
					resp.Body.Close()
				}
			}
		} else if challengeErr == nil {
			// No auth needed — try /v2/_catalog directly
			req, _ := http.NewRequest("GET", baseURL+"/v2/_catalog", nil)
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				var catalog struct {
					Repositories []string `json:"repositories"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&catalog); err == nil {
					repositories = catalog.Repositories
					catalogOK = true
				}
				resp.Body.Close()
			}
		}

		// Step 3: Fallback — use GitLab API to discover container repositories
		if !catalogOK && challenge != nil {
			slog.Debug("catalog listing failed, trying GitLab API fallback", "id", id)
			gitlabRepos, err := listGitLabContainerRepos(challenge.realm, repo)
			if err != nil {
				slog.Warn("gitlab API fallback failed", "id", id, "error", err.Error())
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("registry catalog failed and GitLab API fallback: %v", err)})
				return
			}
			repositories = gitlabRepos
		}

		if repositories == nil {
			repositories = []string{}
		}
		sort.Strings(repositories)
		c.JSON(http.StatusOK, gin.H{"images": repositories, "total": len(repositories)})
	}
}

// listRegistryImageTags returns tags for a specific image in a registry.
func listRegistryImageTags(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Registry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "registry repository not available"})
			return
		}

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		// Gin wildcard params include the leading '/', strip it
		imageName := strings.TrimPrefix(c.Param("imageName"), "/")
		if imageName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image name required"})
			return
		}

		tenantID := auth.GetTenantID(c)
		repo, err := deps.Repos.Registry.GetDecrypted(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "registry repository not found"})
			return
		}

		baseURL := strings.TrimSuffix(repo.URL, "/")
		client := &http.Client{Timeout: 30 * time.Second}

		// Get auth challenge and request a token scoped to this specific repository
		var authHeader string
		challenge, err := getAuthChallenge(repo)
		if err != nil {
			slog.Warn("registry auth challenge failed for tags", "id", id, "error", err.Error())
		}
		if challenge != nil {
			scope := fmt.Sprintf("repository:%s:pull", imageName)
			token, tokenErr := getScopedToken(challenge, repo, scope)
			if tokenErr != nil {
				slog.Warn("scoped token request failed", "id", id, "scope", scope, "error", tokenErr.Error())
			}
			if token != "" {
				authHeader = "Bearer " + token
			}
		}
		if authHeader == "" {
			// Fallback: use Basic Auth or no auth
			req, _ := http.NewRequest("GET", fmt.Sprintf("%s/v2/%s/tags/list", baseURL, imageName), nil)
			registryAuth(req, repo)
			authHeader = req.Header.Get("Authorization")
		}

		req, err := http.NewRequest("GET", fmt.Sprintf("%s/v2/%s/tags/list", baseURL, imageName), nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("create request: %v", err)})
			return
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}

		resp, err := client.Do(req)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("registry request failed: %v", err)})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyRead))
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("registry returned %s: %s", resp.Status, string(body)[:min(len(body), 500)])})
			return
		}

		var tagData struct {
			Name string   `json:"name"`
			Tags []string `json:"tags"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&tagData); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("parse tags: %v", err)})
			return
		}

		if tagData.Tags == nil {
			tagData.Tags = []string{}
		}
		// Sort tags descending (newest first convention)
		sort.Sort(sort.Reverse(sort.StringSlice(tagData.Tags)))

		c.JSON(http.StatusOK, gin.H{"name": tagData.Name, "tags": tagData.Tags, "total": len(tagData.Tags)})
	}
}

