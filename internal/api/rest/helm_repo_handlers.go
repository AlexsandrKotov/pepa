package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
	"gopkg.in/yaml.v3"
)

func registerHelmRepoRoutes(r *gin.RouterGroup, deps Dependencies) {
	helmRepos := r.Group("/helm-repositories")
	{
		helmRepos.GET("", listHelmRepos(deps))
		helmRepos.POST("", createHelmRepo(deps))
		helmRepos.GET("/:id", getHelmRepo(deps))
		helmRepos.PUT("/:id", updateHelmRepo(deps))
		helmRepos.DELETE("/:id", deleteHelmRepo(deps))
		// Chart listing endpoints
		helmRepos.GET("/:id/charts", listHelmCharts(deps))
		helmRepos.GET("/:id/charts/:chartName/versions", listHelmChartVersions(deps))
		helmRepos.GET("/:id/charts/:chartName/versions/:version/download", downloadHelmChart(deps))
	}
}

func listHelmRepos(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Helm == nil {
			c.JSON(http.StatusOK, gin.H{"helm_repositories": []interface{}{}, "total": 0})
			return
		}
		tenantID := auth.GetTenantID(c)
		items, err := deps.Repos.Helm.List(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		// Strip credentials from list response
		for i := range items {
			items[i].Password = ""
			items[i].Token = ""
			items[i].SSHKey = ""
		}
		c.JSON(http.StatusOK, gin.H{"helm_repositories": items, "total": len(items)})
	}
}

func createHelmRepo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Helm == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "helm repository not available"})
			return
		}
		var req struct {
			Name        string `json:"name" binding:"required"`
			Description string `json:"description"`
			RepoType    string `json:"repo_type" binding:"required"`
			URL         string `json:"url" binding:"required"`
			Username    string `json:"username"`
			Password    string `json:"password"`
			Token       string `json:"token"`
			SSHKey      string `json:"ssh_key"`
			CACert      string `json:"ca_cert"`
			IsDefault   bool   `json:"is_default"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		tenantID := auth.GetTenantID(c)
		repo := &repository.HelmRepo{
			TenantID:    tenantID,
			Name:        req.Name,
			Description: req.Description,
			RepoType:    req.RepoType,
			URL:         req.URL,
			Username:    req.Username,
			Password:    req.Password,
			Token:       req.Token,
			SSHKey:      req.SSHKey,
			CACert:      req.CACert,
			IsDefault:   req.IsDefault,
			Status:      "active",
		}

		if err := deps.Repos.Helm.Create(c.Request.Context(), repo); err != nil {
			respondInternalError(c, err)
			return
		}

		// Strip credentials from response
		repo.Password = ""
		repo.Token = ""
		repo.SSHKey = ""
		logAudit(deps, c, "create", "helm_repository", repo.ID.String(), nil, gin.H{"name": repo.Name, "url": repo.URL})
		c.JSON(http.StatusCreated, repo)
	}
}

func getHelmRepo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Helm == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "helm repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		repo, err := deps.Repos.Helm.Get(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "helm repository not found"})
			return
		}
		repo.Password = ""
		repo.Token = ""
		repo.SSHKey = ""
		c.JSON(http.StatusOK, repo)
	}
}

func updateHelmRepo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Helm == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "helm repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		existing, err := deps.Repos.Helm.Get(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "helm repository not found"})
			return
		}

		var req struct {
			Name        string  `json:"name"`
			Description string  `json:"description"`
			RepoType    string  `json:"repo_type"`
			URL         string  `json:"url"`
			Username    *string `json:"username"`
			Password    *string `json:"password"`
			Token       *string `json:"token"`
			SSHKey      *string `json:"ssh_key"`
			CACert      *string `json:"ca_cert"`
			IsDefault   bool    `json:"is_default"`
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
		if req.RepoType != "" {
			existing.RepoType = req.RepoType
		}
		if req.URL != "" {
			existing.URL = req.URL
		}
		// Credentials: use pointers to distinguish "not sent" from "sent as empty"
		// nil = don't change, empty string = clear, non-empty = update
		if req.Username != nil {
			existing.Username = *req.Username
		}
		if req.Password != nil {
			existing.Password = *req.Password
		}
		if req.Token != nil {
			existing.Token = *req.Token
		}
		if req.SSHKey != nil {
			existing.SSHKey = *req.SSHKey
		}
		if req.CACert != nil {
			existing.CACert = *req.CACert
		}
		existing.IsDefault = req.IsDefault

		if err := deps.Repos.Helm.Update(c.Request.Context(), existing); err != nil {
			respondInternalError(c, err)
			return
		}
		existing.Password = ""
		existing.Token = ""
		existing.SSHKey = ""
		logAudit(deps, c, "update", "helm_repository", existing.ID.String(), nil, gin.H{"name": existing.Name, "url": existing.URL})
		c.JSON(http.StatusOK, existing)
	}
}

func deleteHelmRepo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Helm == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "helm repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		tenantID := auth.GetTenantID(c)
		if err := deps.Repos.Helm.Delete(c.Request.Context(), id, tenantID); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "delete", "helm_repository", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"status": "deleted"})
	}
}

// helmIndex represents the structure of a Helm repository index.yaml
type helmIndex struct {
	APIVersion string                 `json:"apiVersion" yaml:"apiVersion"`
	Entries    map[string][]helmChart `json:"entries" yaml:"entries"`
}

type helmChart struct {
	Name        string    `json:"name" yaml:"name"`
	Version     string    `json:"version" yaml:"version"`
	AppVersion  string    `json:"appVersion" yaml:"appVersion"`
	Description string    `json:"description" yaml:"description"`
	Deprecated  bool      `json:"deprecated" yaml:"deprecated"`
	Created     time.Time `json:"created" yaml:"created"`
	URLs        []string  `json:"urls" yaml:"urls"`
}

// applyHelmAuth adds authentication headers to an HTTP request for a Helm repository.
// GitLab's Helm registry requires HTTP Basic Auth. We send multiple auth headers
// to maximize compatibility with different registry implementations:
//   - PRIVATE-TOKEN header (GitLab API standard)
//   - Authorization: Bearer (OAuth2 standard)
//   - Basic Auth with username+token or username+password (Helm registry standard)
func applyHelmAuth(req *http.Request, repo *repository.HelmRepo) {
	// Set PRIVATE-TOKEN for GitLab API compatibility
	if repo.Token != "" {
		req.Header.Set("PRIVATE-TOKEN", repo.Token)
		req.Header.Set("Authorization", "Bearer "+repo.Token)
		// For Helm registries, also set Basic Auth with token as password
		// Use "gitlab-ci-token" as default username if none provided (GitLab convention)
		username := repo.Username
		if username == "" {
			username = "gitlab-ci-token"
		}
		req.SetBasicAuth(username, repo.Token)
	} else if repo.Username != "" && repo.Password != "" {
		// Username+Password auth
		req.Header.Set("PRIVATE-TOKEN", repo.Password)
		req.SetBasicAuth(repo.Username, repo.Password)
	}
}

// fetchHelmIndex fetches and parses the index.yaml from a Helm repository
func fetchHelmIndex(repo *repository.HelmRepo) (*helmIndex, error) {
	url := strings.TrimSuffix(repo.URL, "/") + "/index.yaml"

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	applyHelmAuth(req, repo)

	// Debug: log auth headers being sent
	log.Printf("DEBUG: Fetching helm index from %s", url)
	log.Printf("DEBUG: Has PRIVATE-TOKEN: %v", req.Header.Get("PRIVATE-TOKEN") != "")
	log.Printf("DEBUG: Has Authorization: %v", req.Header.Get("Authorization") != "")
	log.Printf("DEBUG: Has BasicAuth: %v", req.Header.Get("Authorization") != "" && strings.HasPrefix(req.Header.Get("Authorization"), "Basic"))
	// Masked token for debugging
	if repo.Token != "" {
		maskedToken := repo.Token
		if len(maskedToken) > 8 {
			maskedToken = maskedToken[:4] + "..." + maskedToken[len(maskedToken)-4:]
		}
		log.Printf("DEBUG: Token (masked): %s", maskedToken)
	} else {
		log.Printf("DEBUG: Token is EMPTY")
	}
	if repo.Username != "" {
		log.Printf("DEBUG: Username: %s", repo.Username)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch index from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read error response body for debugging
		errorBody, _ := io.ReadAll(resp.Body)
		log.Printf("DEBUG: Helm repo error response (%s): %s", resp.Status, string(errorBody))
		return nil, fmt.Errorf("fetch index from %s returned %s: %s", url, resp.Status, string(errorBody))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var index helmIndex
	// Try YAML first (standard helm format), fall back to JSON
	if err := yaml.Unmarshal(body, &index); err != nil {
		if err := json.Unmarshal(body, &index); err != nil {
			return nil, fmt.Errorf("parse index: failed as YAML (%v) and JSON (%v)", err, err)
		}
	}

	return &index, nil
}

// listHelmCharts returns all charts available in a Helm repository
func listHelmCharts(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Helm == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "helm repository not available"})
			return
		}

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		tenantID := auth.GetTenantID(c)
		repo, err := deps.Repos.Helm.GetDecrypted(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "helm repository not found"})
			return
		}

		index, err := fetchHelmIndex(repo)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to fetch helm index: %v", err)})
			return
		}

		// Build chart list with latest version info
		type chartInfo struct {
			Name          string `json:"name"`
			Description   string `json:"description"`
			LatestVersion string `json:"latest_version"`
			AppVersion    string `json:"app_version"`
			Deprecated    bool   `json:"deprecated"`
			VersionCount  int    `json:"version_count"`
		}

		var charts []chartInfo
		for name, versions := range index.Entries {
			if len(versions) == 0 {
				continue
			}
			// First entry is typically the latest version
			latest := versions[0]
			charts = append(charts, chartInfo{
				Name:          name,
				Description:   latest.Description,
				LatestVersion: latest.Version,
				AppVersion:    latest.AppVersion,
				Deprecated:    latest.Deprecated,
				VersionCount:  len(versions),
			})
		}

		// Sort by name
		sort.Slice(charts, func(i, j int) bool {
			return charts[i].Name < charts[j].Name
		})

		c.JSON(http.StatusOK, gin.H{"charts": charts, "total": len(charts)})
	}
}

// listHelmChartVersions returns all versions for a specific chart
func listHelmChartVersions(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Helm == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "helm repository not available"})
			return
		}

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		chartName := c.Param("chartName")
		if chartName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "chart name required"})
			return
		}

		tenantID := auth.GetTenantID(c)
		repo, err := deps.Repos.Helm.GetDecrypted(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "helm repository not found"})
			return
		}

		index, err := fetchHelmIndex(repo)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to fetch helm index: %v", err)})
			return
		}

		versions, ok := index.Entries[chartName]
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("chart '%s' not found", chartName)})
			return
		}

		// Build version list
		type versionInfo struct {
			Version    string    `json:"version"`
			AppVersion string    `json:"app_version"`
			Deprecated bool      `json:"deprecated"`
			Created    time.Time `json:"created"`
			URLs       []string  `json:"urls"`
		}

		var versionList []versionInfo
		for _, v := range versions {
			versionList = append(versionList, versionInfo{
				Version:    v.Version,
				AppVersion: v.AppVersion,
				Deprecated: v.Deprecated,
				Created:    v.Created,
				URLs:       v.URLs,
			})
		}

		c.JSON(http.StatusOK, gin.H{"versions": versionList, "total": len(versionList)})
	}
}

// downloadHelmChart downloads a specific chart version .tgz file
func downloadHelmChart(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Helm == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "helm repository not available"})
			return
		}

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		chartName := c.Param("chartName")
		version := c.Param("version")
		if chartName == "" || version == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "chart name and version required"})
			return
		}

		tenantID := auth.GetTenantID(c)
		repo, err := deps.Repos.Helm.GetDecrypted(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "helm repository not found"})
			return
		}

		index, err := fetchHelmIndex(repo)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to fetch helm index: %v", err)})
			return
		}

		versions, ok := index.Entries[chartName]
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("chart '%s' not found", chartName)})
			return
		}

		// Find the specific version
		var chartURL string
		for _, v := range versions {
			if v.Version == version && len(v.URLs) > 0 {
				chartURL = v.URLs[0]
				break
			}
		}

		if chartURL == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("version '%s' not found for chart '%s'", version, chartName)})
			return
		}

		// If URL is relative, prepend repo URL
		if !strings.HasPrefix(chartURL, "http://") && !strings.HasPrefix(chartURL, "https://") {
			chartURL = strings.TrimSuffix(repo.URL, "/") + "/" + strings.TrimPrefix(chartURL, "/")
		}

		// Download the chart .tgz
		client := &http.Client{Timeout: 60 * time.Second}
		req, err := http.NewRequest("GET", chartURL, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("create download request: %v", err)})
			return
		}

		// Add authentication
		applyHelmAuth(req, repo)

		resp, err := client.Do(req)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("download failed: %v", err)})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("chart download returned: %s", resp.Status)})
			return
		}

		// Stream the chart to the client
		filename := fmt.Sprintf("%s-%s.tgz", chartName, version)
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		c.Header("Content-Type", "application/gzip")
		c.Status(http.StatusOK)
		io.Copy(c.Writer, resp.Body)
	}
}
