package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
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
		helmRepos.GET("/:id/charts/:chartName/versions/:version/values", getHelmChartValues(deps))
		helmRepos.GET("/:id/charts/:chartName/versions/:version/metadata", getHelmChartMetadata(deps))
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

// helmAuthType returns a human-readable label for the auth method on the
// request. Used only for structured logging — no secrets are exposed.
func helmAuthType(req *http.Request) string {
	if req.Header.Get("PRIVATE-TOKEN") != "" {
		return "token"
	}
	if auth := req.Header.Get("Authorization"); strings.HasPrefix(auth, "Basic") {
		return "basic"
	}
	return "none"
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

	slog.Debug("fetching helm index", "url", url, "auth_type", helmAuthType(req))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch index from %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("helm repo returned non-OK status", "url", url, "status", resp.Status)
		return nil, fmt.Errorf("fetch index from %s returned %s", url, resp.Status)
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

// getHelmChartValues fetches the default values.yaml for a specific chart version
// by running `helm show values` and returns both parsed JSON and raw YAML.
func getHelmChartValues(deps Dependencies) gin.HandlerFunc {
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

		// Determine chart reference and build helm command args
		var chartRef string
		var preArgs []string // args before "show values" (e.g. repo add)

		switch repo.RepoType {
		case "oci":
			chartRef = strings.TrimSuffix(repo.URL, "/") + "/" + chartName
		default: // helm_http, helm_git
			repoName := "pepa-values-" + strings.ReplaceAll(chartName, "/", "-")
			addArgs := []string{"repo", "add", repoName, repo.URL, "--force-update"}
			if repo.Username != "" && repo.Password != "" {
				addArgs = append(addArgs, "--username", repo.Username, "--password", repo.Password)
			} else if repo.Token != "" {
				addArgs = append(addArgs, "--username", "gitlab-ci-token", "--password", repo.Token)
			}
			preArgs = addArgs
			chartRef = repoName + "/" + chartName
		}

		// Add the repo first if needed (for HTTP/Git repos)
		if len(preArgs) > 0 {
			cmdAdd := exec.CommandContext(c.Request.Context(), "helm", preArgs...) //nolint:gosec // #nosec // G204: helm is an admin-configured binary
			if out, err := cmdAdd.CombinedOutput(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("helm repo add failed: %s: %v", strings.TrimSpace(string(out)), err)})
				return
			}
		}

		// Run helm show values
		showArgs := []string{"show", "values", chartRef, "--version", version}
		cmd := exec.CommandContext(c.Request.Context(), "helm", showArgs...) //nolint:gosec // #nosec // G204: helm is an admin-configured binary
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("helm show values failed: %s: %v", strings.TrimSpace(stderr.String()), err)})
			return
		}

		rawYAML := stdout.String()

		// Parse YAML into a generic map for JSON response
		var values map[string]interface{}
		if err := yaml.Unmarshal([]byte(rawYAML), &values); err != nil {
			// If parsing fails, return raw YAML only
			c.JSON(http.StatusOK, gin.H{"values": nil, "raw_yaml": rawYAML})
			return
		}

		c.JSON(http.StatusOK, gin.H{"values": values, "raw_yaml": rawYAML})
	}
}

// downloadHelmChart downloads a specific chart version .tgz file.
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
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("chart download returned: %s", resp.Status)})
			return
		}

		// Stream the chart to the client
		filename := fmt.Sprintf("%s-%s.tgz", chartName, version)
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		c.Header("Content-Type", "application/gzip")
		c.Status(http.StatusOK)
		_, _ = io.Copy(c.Writer, resp.Body)
	}
}

// helmChartMetadata represents parsed common fields from a Helm chart's values.yaml.
type helmChartMetadata struct {
	Replicas    *int               `json:"replicas,omitempty"`
	Image       *helmImageMeta     `json:"image,omitempty"`
	Service     *helmServiceMeta   `json:"service,omitempty"`
	Ingress     *helmIngressMeta   `json:"ingress,omitempty"`
	Resources   *helmResourcesMeta `json:"resources,omitempty"`
	Ports       []helmPortMeta     `json:"ports,omitempty"`
	Autoscaling *helmAutoscalingMeta `json:"autoscaling,omitempty"`
}

type helmImageMeta struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	PullPolicy string `json:"pull_policy"`
}

type helmServiceMeta struct {
	Type string `json:"type"`
	Port int    `json:"port"`
}

type helmIngressMeta struct {
	Enabled bool     `json:"enabled"`
	Hosts   []string `json:"hosts"`
}

type helmResourcesMeta struct {
	LimitsCPU      string `json:"limits_cpu"`
	LimitsMemory   string `json:"limits_memory"`
	RequestsCPU    string `json:"requests_cpu"`
	RequestsMemory string `json:"requests_memory"`
}

type helmPortMeta struct {
	Name         string `json:"name"`
	ContainerPort int   `json:"container_port"`
	Protocol     string `json:"protocol"`
}

type helmAutoscalingMeta struct {
	Enabled      bool `json:"enabled"`
	MinReplicas  int  `json:"min_replicas"`
	MaxReplicas  int  `json:"max_replicas"`
	TargetCPU    int  `json:"target_cpu_utilization"`
}

// extractChartMetadata parses common fields from a Helm chart's values.yaml.
func extractChartMetadata(values map[string]interface{}) helmChartMetadata {
	meta := helmChartMetadata{}

	// Replicas: check replicaCount, replicas
	if v, ok := getIntValue(values, "replicaCount"); ok {
		meta.Replicas = &v
	} else if v, ok := getIntValue(values, "replicas"); ok {
		meta.Replicas = &v
	}

	// Image: image.repository, image.tag, image.pullPolicy
	if img, ok := getMapValue(values, "image"); ok {
		imageMeta := &helmImageMeta{}
		if v, ok := getStringValue(img, "repository"); ok {
			imageMeta.Repository = v
		}
		if v, ok := getStringValue(img, "tag"); ok {
			imageMeta.Tag = v
		}
		if v, ok := getStringValue(img, "pullPolicy"); ok {
			imageMeta.PullPolicy = v
		}
		if imageMeta.Repository != "" || imageMeta.Tag != "" {
			meta.Image = imageMeta
		}
	}

	// Service: service.type, service.port
	if svc, ok := getMapValue(values, "service"); ok {
		serviceMeta := &helmServiceMeta{}
		if v, ok := getStringValue(svc, "type"); ok {
			serviceMeta.Type = v
		}
		if v, ok := getIntValue(svc, "port"); ok {
			serviceMeta.Port = v
		}
		if serviceMeta.Type != "" || serviceMeta.Port > 0 {
			meta.Service = serviceMeta
		}
	}

	// Ingress: ingress.enabled, ingress.hosts
	if ing, ok := getMapValue(values, "ingress"); ok {
		ingressMeta := &helmIngressMeta{}
		if v, ok := ing["enabled"].(bool); ok {
			ingressMeta.Enabled = v
		}
		// Check for hosts array
		if hosts, ok := ing["hosts"].([]interface{}); ok {
			for _, h := range hosts {
				switch hv := h.(type) {
				case string:
					ingressMeta.Hosts = append(ingressMeta.Hosts, hv)
				case map[string]interface{}:
					if host, ok := hv["host"].(string); ok {
						ingressMeta.Hosts = append(ingressMeta.Hosts, host)
					}
				}
			}
		}
		// Check for hostname (some charts use ingress.hostname)
		if len(ingressMeta.Hosts) == 0 {
			if host, ok := getStringValue(ing, "hostname"); ok {
				ingressMeta.Hosts = append(ingressMeta.Hosts, host)
			}
		}
		meta.Ingress = ingressMeta
	}

	// Resources: resources.limits.cpu, resources.limits.memory, etc.
	if res, ok := getMapValue(values, "resources"); ok {
		resMeta := &helmResourcesMeta{}
		if limits, ok := getMapValue(res, "limits"); ok {
			if v, ok := getStringValue(limits, "cpu"); ok {
				resMeta.LimitsCPU = v
			}
			if v, ok := getStringValue(limits, "memory"); ok {
				resMeta.LimitsMemory = v
			}
		}
		if requests, ok := getMapValue(res, "requests"); ok {
			if v, ok := getStringValue(requests, "cpu"); ok {
				resMeta.RequestsCPU = v
			}
			if v, ok := getStringValue(requests, "memory"); ok {
				resMeta.RequestsMemory = v
			}
		}
		if resMeta.LimitsCPU != "" || resMeta.LimitsMemory != "" || resMeta.RequestsCPU != "" || resMeta.RequestsMemory != "" {
			meta.Resources = resMeta
		}
	}

	// Container ports: service.ports or containerPort
	if svc, ok := getMapValue(values, "service"); ok {
		if ports, ok := svc["ports"].([]interface{}); ok {
			for _, p := range ports {
				if pm, ok := p.(map[string]interface{}); ok {
					port := helmPortMeta{Protocol: "TCP"}
					if v, ok := getStringValue(pm, "name"); ok {
						port.Name = v
					}
					if v, ok := getIntValue(pm, "port"); ok {
						port.ContainerPort = v
					} else if v, ok := getIntValue(pm, "containerPort"); ok {
						port.ContainerPort = v
					}
					if v, ok := getStringValue(pm, "protocol"); ok {
						port.Protocol = v
					}
					if port.ContainerPort > 0 {
						meta.Ports = append(meta.Ports, port)
					}
				}
			}
		}
	}
	// Fallback: check for containerPort at top level or in common locations
	if len(meta.Ports) == 0 {
		if v, ok := getIntValue(values, "containerPort"); ok {
			meta.Ports = append(meta.Ports, helmPortMeta{Name: "http", ContainerPort: v, Protocol: "TCP"})
		}
	}

	// Autoscaling: autoscaling.enabled, autoscaling.minReplicas, etc.
	if as, ok := getMapValue(values, "autoscaling"); ok {
		asMeta := &helmAutoscalingMeta{}
		if v, ok := as["enabled"].(bool); ok {
			asMeta.Enabled = v
		}
		if v, ok := getIntValue(as, "minReplicas"); ok {
			asMeta.MinReplicas = v
		}
		if v, ok := getIntValue(as, "maxReplicas"); ok {
			asMeta.MaxReplicas = v
		}
		if v, ok := getIntValue(as, "targetCPUUtilizationPercentage"); ok {
			asMeta.TargetCPU = v
		}
		meta.Autoscaling = asMeta
	}

	return meta
}

// Helper functions for extracting typed values from maps.
func getMapValue(m map[string]interface{}, key string) (map[string]interface{}, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	result, ok := v.(map[string]interface{})
	return result, ok
}

func getStringValue(m map[string]interface{}, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	result, ok := v.(string)
	return result, ok
}

func getIntValue(m map[string]interface{}, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case int:
		return val, true
	case float64:
		return int(val), true
	case int64:
		return int(val), true
	}
	return 0, false
}

// getHelmChartMetadata fetches chart default values and returns both the raw values
// and parsed metadata with extracted common fields (image, replicas, service, ingress, etc.).
func getHelmChartMetadata(deps Dependencies) gin.HandlerFunc {
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

		// Determine chart reference and build helm command args
		var chartRef string
		var preArgs []string

		switch repo.RepoType {
		case "oci":
			chartRef = strings.TrimSuffix(repo.URL, "/") + "/" + chartName
		default:
			repoName := "pepa-meta-" + strings.ReplaceAll(chartName, "/", "-")
			addArgs := []string{"repo", "add", repoName, repo.URL, "--force-update"}
			if repo.Username != "" && repo.Password != "" {
				addArgs = append(addArgs, "--username", repo.Username, "--password", repo.Password)
			} else if repo.Token != "" {
				addArgs = append(addArgs, "--username", "gitlab-ci-token", "--password", repo.Token)
			}
			preArgs = addArgs
			chartRef = repoName + "/" + chartName
		}

		if len(preArgs) > 0 {
			cmdAdd := exec.CommandContext(c.Request.Context(), "helm", preArgs...) //nolint:gosec
			if out, err := cmdAdd.CombinedOutput(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("helm repo add failed: %s: %v", strings.TrimSpace(string(out)), err)})
				return
			}
		}

		// Run helm show values
		showArgs := []string{"show", "values", chartRef, "--version", version}
		cmd := exec.CommandContext(c.Request.Context(), "helm", showArgs...) //nolint:gosec
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("helm show values failed: %s: %v", strings.TrimSpace(stderr.String()), err)})
			return
		}

		rawYAML := stdout.String()

		// Parse YAML into a generic map
		var values map[string]interface{}
		if err := yaml.Unmarshal([]byte(rawYAML), &values); err != nil {
			c.JSON(http.StatusOK, gin.H{"values": nil, "raw_yaml": rawYAML, "metadata": nil})
			return
		}

		// Extract common metadata fields
		metadata := extractChartMetadata(values)

		c.JSON(http.StatusOK, gin.H{
			"values":   values,
			"raw_yaml": rawYAML,
			"metadata": metadata,
		})
	}
}
