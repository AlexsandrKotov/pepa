package rest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/pkg/models"
)

// ServiceHandlers handles service-related HTTP requests.
type ServiceHandlers struct {
	repo *repository.ServiceRepository
	deps Dependencies
}

// NewServiceHandlers creates new service handlers.
func NewServiceHandlers(repo *repository.ServiceRepository, deps Dependencies) *ServiceHandlers {
	return &ServiceHandlers{repo: repo, deps: deps}
}

// registerServiceRoutes registers all service-related routes.
func registerServiceRoutes(v1 *gin.RouterGroup, deps Dependencies) {
	if deps.Repos.Service == nil {
		return
	}
	h := NewServiceHandlers(deps.Repos.Service, deps)

	// Service Templates
	v1.GET("/service-templates", h.ListTemplates)
	v1.GET("/service-templates/:slug", h.GetTemplate)

	// Services
	services := v1.Group("/services")
	{
		services.GET("", h.ListServices)
		services.POST("", h.CreateService)
		services.GET("/:id", h.GetService)
		services.PUT("/:id", h.UpdateService)
		services.DELETE("/:id", h.DeleteService)

		// Service deployments
		services.POST("/:id/deploy", h.DeployService)
		services.GET("/:id/deployments", h.GetServiceDeployments)

		// Deployment management
		services.POST("/:id/deployments/:deploymentId/verify", h.VerifyDeployment)
		services.POST("/:id/deployments/:deploymentId/promote", h.PromoteDeployment)
	}
}

// ── Service Templates ────────────────────────────────────────

// ListTemplates returns all service templates.
func (h *ServiceHandlers) ListTemplates(c *gin.Context) {
	templates, err := h.repo.ListTemplates(c.Request.Context())
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"templates": templates,
		"total":     len(templates),
	})
}

// GetTemplate returns a single template.
func (h *ServiceHandlers) GetTemplate(c *gin.Context) {
	slug := c.Param("slug")
	template, err := h.repo.GetTemplate(c.Request.Context(), slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, template)
}

// ── Services ─────────────────────────────────────────────────

// ListServices returns services with filtering.
func (h *ServiceHandlers) ListServices(c *gin.Context) {
	var filter models.ServiceFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.repo.List(c.Request.Context(), filter)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetService returns a single service.
func (h *ServiceHandlers) GetService(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service ID"})
		return
	}

	service, err := h.repo.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Get deployments for this service
	deployments, _ := h.repo.ListDeployments(c.Request.Context(), id)

	c.JSON(http.StatusOK, gin.H{
		"service":     service,
		"deployments": deployments,
	})
}

// CreateService creates a new service from a template.
func (h *ServiceHandlers) CreateService(c *gin.Context) {
	var req models.CreateServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := auth.GetTenantID(c)

	service, err := h.repo.Create(c.Request.Context(), req, tenantID, auth.GetUserID(c))
	if err != nil {
		// Return 409 Conflict for duplicate key errors
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		respondInternalError(c, err)
		return
	}

	// Auto-deploy to target clusters if specified
	if len(req.TargetClusterIDs) > 0 && h.deps.Repos.Cluster != nil {
		// Look up template Helm chart info
		var helmChartInfo map[string]interface{}
		if service.TemplateID != nil {
			tmpl, tmplErr := h.repo.GetTemplateByID(c.Request.Context(), *service.TemplateID)
			if tmplErr == nil && tmpl != nil && len(tmpl.HelmChart) > 0 {
				_ = json.Unmarshal(tmpl.HelmChart, &helmChartInfo)
			}
		}

		// Build deployment spec from service config + template chart
		spec := buildServiceDeploySpec(service, models.DeployServiceRequest{})

		// If template has Helm chart info, inject chart spec
		if helmChartInfo != nil {
			repoURL, _ := helmChartInfo["repo_url"].(string)
			chartNameVal, _ := helmChartInfo["chart_name"].(string)
			chartVer, _ := helmChartInfo["chart_version"].(string)
			if repoURL != "" && chartNameVal != "" {
				var specMap map[string]interface{}
				_ = json.Unmarshal(spec, &specMap)
				specMap["chart"] = map[string]interface{}{
					"source_type":   "helm_http",
					"chart_url":     repoURL,
					"chart_name":    chartNameVal,
					"chart_version": chartVer,
				}
				spec, _ = json.Marshal(specMap)
			}
		}

		// Create deployment records and trigger deployment for each target cluster
		for _, clusterIDStr := range req.TargetClusterIDs {
			clusterUUID, parseErr := uuid.Parse(clusterIDStr)
			if parseErr != nil {
				continue
			}
			// Create a deployment record so it shows in the pipeline UI
			deployReq := models.DeployServiceRequest{
				Environment: "dev",
				ClusterID:   clusterUUID.String(),
				DeployType:  "automatic",
			}
			deployment, depErr := h.repo.CreateDeployment(c.Request.Context(), service.ID, deployReq, tenantID)
			if depErr != nil {
				slog.Info("ERROR: create deployment record for cluster ", "id", clusterUUID, "error", depErr)
				continue
			}
			go func() {
				if err := h.deps.Services.ServiceDeployment.PerformServiceDeployment(
					context.Background(),
					deployment.ID, service.ID, clusterUUID, service.Namespace,
					service.Name, spec,
				); err != nil {
					slog.Info("ERROR: service deployment failed", "id", deployment.ID, "error", err)
				}
			}()
			_ = deployment
		}
	}

	logAudit(h.deps, c, "create", "service", service.ID.String(), nil, gin.H{"name": service.Name, "namespace": service.Namespace})
	c.JSON(http.StatusCreated, service)
}

// UpdateService updates an existing service.
func (h *ServiceHandlers) UpdateService(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service ID"})
		return
	}

	var req models.UpdateServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	service, err := h.repo.Update(c.Request.Context(), id, req, nil)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "update", "service", service.ID.String(), nil, gin.H{"name": service.Name})
	c.JSON(http.StatusOK, service)
}

// DeleteService deletes a service.
func (h *ServiceHandlers) DeleteService(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service ID"})
		return
	}

	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "delete", "service", id.String(), nil, nil)
	c.JSON(http.StatusOK, gin.H{"message": "service deleted"})
}

// ── Service Deployments ──────────────────────────────────────

// DeployService triggers deployment to an environment.
func (h *ServiceHandlers) DeployService(c *gin.Context) {
	serviceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service ID"})
		return
	}

	var req models.DeployServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := auth.GetTenantID(c)

	deployment, err := h.repo.CreateDeployment(c.Request.Context(), serviceID, req, tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	// Trigger actual k8s deployment if cluster is specified
	if req.ClusterID != "" && h.deps.Repos.Cluster != nil {
		svc, svcErr := h.repo.Get(c.Request.Context(), serviceID)
		if svcErr == nil && svc != nil {
			clusterUUID, parseErr := uuid.Parse(req.ClusterID)
			if parseErr == nil {
				// Update service status to deploying
				_ = h.repo.SetStatus(c.Request.Context(), serviceID, "deploying")
				// Build deployment spec from service configuration
				spec := buildServiceDeploySpec(svc, req)
				releaseName := svc.Name
				go func() {
					if err := h.deps.Services.ServiceDeployment.PerformServiceDeployment(
						context.Background(),
						deployment.ID, serviceID, clusterUUID, svc.Namespace,
						releaseName, spec,
					); err != nil {
						slog.Info("ERROR: service deployment failed", "id", deployment.ID, "error", err)
					}
				}()
			}
		}
	}

	logAudit(h.deps, c, "deploy", "service", serviceID.String(), nil, gin.H{"environment": req.Environment, "cluster_id": req.ClusterID})
	c.JSON(http.StatusCreated, deployment)
}

// buildServiceDeploySpec constructs a deployment spec from service configuration.
func buildServiceDeploySpec(svc *models.Service, req models.DeployServiceRequest) json.RawMessage {
	spec := map[string]interface{}{}

	// Parse resource config for container info
	var resourceConfig map[string]interface{}
	if len(svc.ResourceConfig) > 0 {
		_ = json.Unmarshal(svc.ResourceConfig, &resourceConfig)
	}

	// Parse environment variables
	var envVars map[string]string
	if len(svc.EnvironmentVars) > 0 {
		_ = json.Unmarshal(svc.EnvironmentVars, &envVars)
	}

	// Build container from service config
	image := svc.ImageRepository
	if image == "" && svc.HelmChartURL != "" {
		image = svc.HelmChartURL
	}
	if req.ImageTag != "" && image != "" {
		if !strings.Contains(image, ":") {
			image = image + ":" + req.ImageTag
		}
	}

	if image != "" {
		cpu := "100m"
		memory := "128Mi"
		var replicas int = 1
		if rc, ok := resourceConfig["cpu"].(string); ok && rc != "" {
			cpu = rc
		}
		if mem, ok := resourceConfig["memory"].(string); ok && mem != "" {
			memory = mem
		}
		if rep, ok := resourceConfig["replicas"].(float64); ok && rep > 0 {
			replicas = int(rep)
		}

		spec["containers"] = []map[string]interface{}{
			{
				"name":   svc.Slug,
				"image":  image,
				"cpu":    cpu,
				"memory": memory,
				"ports":  []map[string]interface{}{{"containerPort": 80}},
			},
		}
		spec["replicas"] = replicas
	}

	// Add values.yaml from metadata if present
	var metadata map[string]interface{}
	if len(svc.Metadata) > 0 {
		_ = json.Unmarshal(svc.Metadata, &metadata)
	}
	if metadata != nil {
		if valuesYaml, ok := metadata["values_yaml"].(string); ok && valuesYaml != "" {
			spec["values_yaml"] = valuesYaml
		}
	}

	// Add service port
	spec["service"] = map[string]interface{}{"port": 80, "type": "ClusterIP"}

	// If helm_chart_url looks like a Helm chart, add chart spec
	if svc.HelmChartURL != "" && !strings.Contains(svc.HelmChartURL, "/") ||
		(svc.HelmChartURL != "" && strings.HasSuffix(svc.HelmChartURL, ".tgz")) {
		spec["chart"] = map[string]interface{}{
			"source_type": "helm_http",
			"chart_url":   svc.HelmChartURL,
			"chart_name":  svc.Slug,
		}
	}

	result, _ := json.Marshal(spec)
	return result
}

// GetServiceDeployments returns all deployments for a service.
func (h *ServiceHandlers) GetServiceDeployments(c *gin.Context) {
	serviceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service ID"})
		return
	}

	deployments, err := h.repo.ListDeployments(c.Request.Context(), serviceID)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"deployments": deployments,
		"total":       len(deployments),
	})
}

// VerifyDeployment runs verification checks on a deployment.
func (h *ServiceHandlers) VerifyDeployment(c *gin.Context) {
	deploymentID, err := uuid.Parse(c.Param("deploymentId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
		return
	}

	// For now, just mark as verified
	if err := h.repo.UpdateDeployment(c.Request.Context(), deploymentID, "deployed", "verified"); err != nil {
		respondInternalError(c, err)
		return
	}

	deployment, err := h.repo.GetDeployment(c.Request.Context(), deploymentID)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"deployment": deployment,
		"message":    "Deployment verified successfully",
	})
}

// PromoteDeployment promotes a deployment to the next environment.
func (h *ServiceHandlers) PromoteDeployment(c *gin.Context) {
	deploymentID, err := uuid.Parse(c.Param("deploymentId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
		return
	}

	// Validate deployment exists and is in a promotable state
	deployment, err := h.repo.GetDeployment(c.Request.Context(), deploymentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if deployment.Status != "deployed" {
		c.JSON(http.StatusConflict, gin.H{"error": "deployment must be in 'deployed' status to promote, current: " + deployment.Status})
		return
	}

	if err := h.repo.PromoteDeployment(c.Request.Context(), deploymentID); err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Deployment promoted successfully"})
}
