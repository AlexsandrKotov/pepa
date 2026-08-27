package rest

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/pkg/models"
)

func registerEnvironmentRoutes(r *gin.RouterGroup, deps Dependencies) {
	envs := r.Group("/environments")
	{
		// Compare must be registered BEFORE /:id to avoid route conflict
		envs.GET("/compare", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)
			env1Slug := c.Query("env1")
			env2Slug := c.Query("env2")
			if env1Slug == "" || env2Slug == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "env1 and env2 query params are required"})
				return
			}

			env1, err := deps.Repos.Environment.GetBySlug(c.Request.Context(), tenantID, env1Slug)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "environment 1 not found"})
				return
			}
			env2, err := deps.Repos.Environment.GetBySlug(c.Request.Context(), tenantID, env2Slug)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "environment 2 not found"})
				return
			}

			vars1, err := deps.Repos.EnvVariable.ListByEnvID(c.Request.Context(), tenantID, env1.ID)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			vars2, err := deps.Repos.EnvVariable.ListByEnvID(c.Request.Context(), tenantID, env2.ID)
			if err != nil {
				respondInternalError(c, err)
				return
			}

			// Build comparison
			map1 := make(map[string]repository.EnvironmentVariable)
			for _, v := range vars1 {
				map1[v.Key] = v
			}
			map2 := make(map[string]repository.EnvironmentVariable)
			for _, v := range vars2 {
				map2[v.Key] = v
			}

			type diffEntry struct {
				Key      string `json:"key"`
				Value1   string `json:"value1"`
				Value2   string `json:"value2"`
				Status   string `json:"status"` // "only_env1", "only_env2", "different", "same"
				IsSecret bool   `json:"is_secret"`
			}

			// Initialize as empty slice so JSON marshals [] instead of null
			comparison := []diffEntry{}
			allKeys := make(map[string]bool)
			for k := range map1 {
				allKeys[k] = true
			}
			for k := range map2 {
				allKeys[k] = true
			}
			for k := range allKeys {
				v1, in1 := map1[k]
				v2, in2 := map2[k]
				entry := diffEntry{Key: k}
				switch {
				case in1 && in2:
					entry.Value1 = v1.Value
					entry.Value2 = v2.Value
					entry.IsSecret = v1.IsSecret || v2.IsSecret
					if v1.Value == v2.Value {
						entry.Status = "same"
					} else {
						entry.Status = "different"
					}
				case in1:
					entry.Value1 = v1.Value
					entry.IsSecret = v1.IsSecret
					entry.Status = "only_env1"
				case in2:
					entry.Value2 = v2.Value
					entry.IsSecret = v2.IsSecret
					entry.Status = "only_env2"
				}
				comparison = append(comparison, entry)
			}

			diffs := 0
			for _, e := range comparison {
				if e.Status != "same" {
					diffs++
				}
			}

			c.JSON(http.StatusOK, gin.H{
				"env1":        env1Slug,
				"env2":        env2Slug,
				"comparison":  comparison,
				"total":       len(comparison),
				"differences": diffs,
			})
		})

		envs.GET("", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)
			items, err := deps.Repos.Environment.List(c.Request.Context(), tenantID)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			if items == nil {
				items = []repository.Environment{}
			}
			c.JSON(http.StatusOK, gin.H{"environments": items, "total": len(items)})
		})

		envs.GET("/:id", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)
			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
				return
			}
			env, err := deps.Repos.Environment.Get(c.Request.Context(), tenantID, id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, env)
		})

		envs.POST("", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)

			var req struct {
				Name        string `json:"name" binding:"required"`
				Slug        string `json:"slug"`
				Type        string `json:"type"`
				Cluster     string `json:"cluster"`
				Namespace   string `json:"namespace"`
				Status      string `json:"status"`
				Description string `json:"description"`
				Color       string `json:"color"`
				IsDefault   bool   `json:"is_default"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			// Auto-generate slug from name if not provided
			slug := req.Slug
			if slug == "" {
				slug = slugify(req.Name)
			}
			if slug == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "name or slug is required"})
				return
			}

			// Validate color format
			color := req.Color
			if color == "" {
				color = "#6B7280"
			}
			if !regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`).MatchString(color) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "color must be a hex code like #3B82F6"})
				return
			}

			envType := req.Type
			if envType == "" {
				envType = strings.ToLower(req.Name)
			}

			status := req.Status
			if status == "" {
				status = "active"
			}

			env := &repository.Environment{
				TenantID:    tenantID,
				Name:        req.Name,
				Slug:        slug,
				Type:        envType,
				Cluster:     req.Cluster,
				Namespace:   req.Namespace,
				Status:      status,
				Description: req.Description,
				Color:       color,
				IsDefault:   req.IsDefault,
			}

			if err := deps.Repos.Environment.Create(c.Request.Context(), env); err != nil {
				if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique") {
					c.JSON(http.StatusConflict, gin.H{"error": "environment with slug '" + slug + "' already exists"})
					return
				}
				respondInternalError(c, err)
				return
			}

			logAudit(deps, c, "create", "environment", env.ID.String(), nil, map[string]string{
				"name": req.Name,
				"slug": slug,
			})
			c.JSON(http.StatusCreated, env)
		})

		envs.PUT("/:id", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)
			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
				return
			}

			existing, err := deps.Repos.Environment.Get(c.Request.Context(), tenantID, id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}

			var req struct {
				Name        string `json:"name"`
				Slug        string `json:"slug"`
				Type        string `json:"type"`
				Cluster     string `json:"cluster"`
				Namespace   string `json:"namespace"`
				Status      string `json:"status"`
				Description string `json:"description"`
				Color       string `json:"color"`
				IsDefault   *bool  `json:"is_default"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			if req.Name != "" {
				existing.Name = req.Name
			}
			if req.Slug != "" {
				existing.Slug = slugify(req.Slug)
			}
			if req.Type != "" {
				existing.Type = req.Type
			}
			if req.Cluster != "" {
				existing.Cluster = req.Cluster
			}
			if req.Namespace != "" {
				existing.Namespace = req.Namespace
			}
			if req.Status != "" {
				existing.Status = req.Status
			}
			if req.Description != "" {
				existing.Description = req.Description
			}
			if req.Color != "" {
				if !regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`).MatchString(req.Color) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "color must be a hex code like #3B82F6"})
					return
				}
				existing.Color = req.Color
			}
			if req.IsDefault != nil {
				existing.IsDefault = *req.IsDefault
			}

			if err := deps.Repos.Environment.Update(c.Request.Context(), existing); err != nil {
				respondInternalError(c, err)
				return
			}

			logAudit(deps, c, "update", "environment", existing.ID.String(), nil, map[string]string{
				"name": existing.Name,
			})
			c.JSON(http.StatusOK, existing)
		})

		envs.DELETE("/:id", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)
			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
				return
			}

			if err := deps.Repos.Environment.Delete(c.Request.Context(), tenantID, id); err != nil {
				respondInternalError(c, err)
				return
			}

			logAudit(deps, c, "delete", "environment", id.String(), nil, nil)
			c.JSON(http.StatusOK, gin.H{"message": "environment deleted"})
		})

		// ── Environment Variables ──────────────────────────────────────────

		envs.GET("/:id/variables", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)
			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
				return
			}

			// Verify environment exists
			_, err = deps.Repos.Environment.Get(c.Request.Context(), tenantID, id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}

			vars, err := deps.Repos.EnvVariable.ListByEnvID(c.Request.Context(), tenantID, id)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			if vars == nil {
				vars = []repository.EnvironmentVariable{}
			}

			c.JSON(http.StatusOK, gin.H{"variables": vars, "total": len(vars)})
		})

		envs.POST("/:id/variables", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)
			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
				return
			}

			// Verify environment exists
			_, err = deps.Repos.Environment.Get(c.Request.Context(), tenantID, id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}

			var req struct {
				Key      string `json:"key" binding:"required"`
				Value    string `json:"value"`
				IsSecret bool   `json:"is_secret"`
				Source   string `json:"source"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			source := req.Source
			if source == "" {
				source = "manual"
			}

			v := &repository.EnvironmentVariable{
				TenantID: tenantID,
				EnvID:    id,
				Key:      req.Key,
				Value:    req.Value,
				IsSecret: req.IsSecret,
				Source:   source,
			}

			if err := deps.Repos.EnvVariable.Set(c.Request.Context(), v); err != nil {
				respondInternalError(c, err)
				return
			}

			logAudit(deps, c, "set_variable", "environment", id.String(), nil, map[string]string{
				"key": req.Key,
			})
			c.JSON(http.StatusOK, gin.H{"variable": v, "message": "variable set"})
		})

		envs.DELETE("/:id/variables/:key", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)
			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
				return
			}
			key := c.Param("key")
			if key == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
				return
			}

			if err := deps.Repos.EnvVariable.DeleteByKey(c.Request.Context(), tenantID, id, key); err != nil {
				respondInternalError(c, err)
				return
			}

			logAudit(deps, c, "delete_variable", "environment", id.String(), nil, map[string]string{
				"key": key,
			})
			c.JSON(http.StatusOK, gin.H{"message": "variable deleted"})
		})

		// ── Environment Contents ───────────────────────────────────────────

		envs.GET("/:id/contents", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)
			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
				return
			}

			env, err := deps.Repos.Environment.Get(c.Request.Context(), tenantID, id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}

			// Get clusters in this environment (matched by environment slug)
			allClusters, err := deps.Repos.Cluster.List(c.Request.Context(), tenantID)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			var envClusters []repository.Cluster
			for _, cl := range allClusters {
				if cl.Environment == env.Slug {
					envClusters = append(envClusters, cl)
				}
			}
			if envClusters == nil {
				envClusters = []repository.Cluster{}
			}

			// Get service deployments in this environment
			deployments, err := deps.Repos.Service.ListDeploymentsByEnvironment(c.Request.Context(), tenantID, env.Slug)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			if deployments == nil {
				deployments = []models.ServiceDeployment{}
			}

			// Get variable count
			varCount, _ := deps.Repos.EnvVariable.CountByEnvID(c.Request.Context(), id)

			c.JSON(http.StatusOK, gin.H{
				"environment": env,
				"clusters":    envClusters,
				"deployments": deployments,
				"variables_count": varCount,
				"summary": gin.H{
					"cluster_count":   len(envClusters),
					"deployment_count": len(deployments),
					"variable_count":  varCount,
				},
			})
		})
	}
}

// slugify converts a string to a URL-safe slug.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
