package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/pkg/models"
)

func registerCatalogRoutes(r *gin.RouterGroup, deps Dependencies) {
	catalog := r.Group("/catalog")
	{
		catalog.GET("", listCatalog(deps))
		catalog.GET("/:id", getCatalogItem(deps))
		catalog.GET("/:id/health", getCatalogHealth(deps))
		catalog.PUT("/:id", updateCatalogItem(deps))
	}
}

// listCatalog returns all services with enriched status info for the catalog view.
func listCatalog(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Service == nil {
			c.JSON(http.StatusOK, gin.H{"items": []interface{}{}, "total": 0})
			return
		}

		// Parse query params
		search := c.Query("search")
		status := c.Query("status")
		category := c.Query("category")

		filter := models.ServiceFilter{
			Search:  search,
			Status:  status,
			Page:    1,
			PerPage: 100,
		}

		result, err := deps.Repos.Service.List(c.Request.Context(), filter)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		services := result.Items
		total := int(result.Total)

		// Enrich with template info and deployment counts
		type catalogItem struct {
			ID                 string   `json:"id"`
			Name               string   `json:"name"`
			Slug               string   `json:"slug"`
			Description        string   `json:"description"`
			Language           string   `json:"language"`
			Framework          string   `json:"framework"`
			Namespace          string   `json:"namespace"`
			Status             string   `json:"status"`
			DeploymentStrategy string   `json:"deployment_strategy"`
			TemplateName       string   `json:"template_name"`
			Category           string   `json:"category"`
			Tags               []string `json:"tags"`
			DeploymentCount    int      `json:"deployment_count"`
			ActiveEnvironments []string `json:"active_environments"`
			CreatedAt          string   `json:"created_at"`
			UpdatedAt          string   `json:"updated_at"`
		}

		items := make([]catalogItem, 0, len(services))
		for _, svc := range services {
			item := catalogItem{
				ID:                 svc.ID.String(),
				Name:               svc.Name,
				Slug:               svc.Slug,
				Description:        svc.Description,
				Language:           svc.Language,
				Framework:          svc.Framework,
				Namespace:          svc.Namespace,
				Status:             svc.Status,
				DeploymentStrategy: svc.DeploymentStrategy,
				CreatedAt:          svc.CreatedAt.String(),
				UpdatedAt:          svc.UpdatedAt.String(),
			}

			// Get template info if available
			if svc.TemplateID != nil {
				tmpl, err := deps.Repos.Service.GetTemplateByID(c.Request.Context(), *svc.TemplateID)
				if err == nil && tmpl != nil {
					item.TemplateName = tmpl.Name
					item.Category = tmpl.Category
					item.Tags = tmpl.Tags
					if item.Language == "" {
						item.Language = tmpl.Language
					}
				}
			}

			// Get deployments to count active environments
			deployments, err := deps.Repos.Service.ListDeployments(c.Request.Context(), svc.ID)
			if err == nil {
				item.DeploymentCount = len(deployments)
				for _, d := range deployments {
					if d.Status == "deployed" || d.Status == "deploying" {
						item.ActiveEnvironments = append(item.ActiveEnvironments, d.Environment)
					}
				}
			}

			items = append(items, item)
		}

		// Filter by category if specified
		if category != "" {
			filtered := make([]catalogItem, 0)
			for _, item := range items {
				if item.Category == category {
					filtered = append(filtered, item)
				}
			}
			items = filtered
			total = len(items)
		}

		c.JSON(http.StatusOK, gin.H{
			"items": items,
			"total": total,
		})
	}
}

// getCatalogItem returns detailed info about a single catalog entry.
func getCatalogItem(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Service == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service ID"})
			return
		}

		svc, err := deps.Repos.Service.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		deployments, _ := deps.Repos.Service.ListDeployments(c.Request.Context(), id)

		// Get template info
		var templateName string
		var category string
		var tags []string
		if svc.TemplateID != nil {
			tmpl, err := deps.Repos.Service.GetTemplateByID(c.Request.Context(), *svc.TemplateID)
			if err == nil && tmpl != nil {
				templateName = tmpl.Name
				category = tmpl.Category
				tags = tmpl.Tags
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"service":       svc,
			"deployments":   deployments,
			"template_name": templateName,
			"category":      category,
			"tags":          tags,
		})
	}
}

// getCatalogHealth returns health status for a service.
func getCatalogHealth(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Service == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service ID"})
			return
		}

		svc, err := deps.Repos.Service.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		deployments, _ := deps.Repos.Service.ListDeployments(c.Request.Context(), id)

		// Calculate health based on deployment status
		totalPods := 0
		readyPods := 0
		healthyEnvs := 0
		totalEnvs := 0

		for _, d := range deployments {
			if d.Status == "deployed" || d.Status == "deploying" {
				totalEnvs++
				totalPods += d.PodsTotal
				readyPods += d.PodsReady
				if d.Status == "deployed" && d.PodsReady == d.PodsTotal && d.PodsTotal > 0 {
					healthyEnvs++
				}
			}
		}

		var healthStatus string
		if totalEnvs == 0 {
			healthStatus = "not_deployed"
		} else if healthyEnvs == totalEnvs {
			healthStatus = "healthy"
		} else if healthyEnvs > 0 || readyPods > 0 {
			healthStatus = "degraded"
		} else {
			healthStatus = "unhealthy"
		}

		c.JSON(http.StatusOK, gin.H{
			"service_id":   svc.ID.String(),
			"service_name": svc.Name,
			"status":       healthStatus,
			"total_envs":   totalEnvs,
			"healthy_envs": healthyEnvs,
			"total_pods":   totalPods,
			"ready_pods":   readyPods,
			"deployments":  len(deployments),
		})
	}
}

// updateCatalogItem updates service metadata.
func updateCatalogItem(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Service == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service ID"})
			return
		}

		userID := auth.GetUserID(c)
		req := models.UpdateServiceRequest{}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		svc, err := deps.Repos.Service.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		updated, err := deps.Repos.Service.Update(c.Request.Context(), svc.ID, req, userID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "catalog_item", svc.ID.String(), nil, gin.H{"name": svc.Name})

		c.JSON(http.StatusOK, updated)
	}
}
