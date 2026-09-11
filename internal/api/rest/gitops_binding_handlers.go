package rest

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/gitops/engine"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/service"
)

// discoverGitOpsApplications scans all ArgoCD and FluxCD connections for a tenant
// and creates bindings for discovered applications.
func discoverGitOpsApplications(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := getTenantID(c)

		engineClient := newEngineClient(deps)

		// Get all connections for reference (including kubernetes-type which the
		// engine resolves as a FluxCD/ArgoCD fallback).
		argoConns, _ := deps.Repos.Connection.List(ctx, tenantID, "argocd")
		fluxConns, _ := deps.Repos.Connection.List(ctx, tenantID, "fluxcd")
		k8sConns, _ := deps.Repos.Connection.List(ctx, tenantID, "kubernetes")

		// Build connection name lookup
		connNames := make(map[string]string)
		for _, conn := range argoConns {
			connNames[conn.ID.String()] = conn.Name
		}
		for _, conn := range fluxConns {
			connNames[conn.ID.String()] = conn.Name
		}
		for _, conn := range k8sConns {
			connNames[conn.ID.String()] = conn.Name
		}

		var discovered []*discoveredApp
		var created int

		// List all ArgoCD applications
		argoApps, err := engineClient.List(ctx, engine.ListOptions{
			TenantID:   tenantID,
			EngineType: "argocd",
		})
		if err != nil {
			slog.Warn("Failed to list ArgoCD apps", "error", err)
		}

		for _, app := range argoApps {
			connID, _ := uuid.Parse(app.ConnectionID)
			disc := &discoveredApp{
				ConnectionID:   connID,
				ConnectionName: connNames[app.ConnectionID],
				App:            app,
			}
			discovered = append(discovered, disc)

			// Create binding if it doesn't exist
			if deps.Repos.GitOpsBinding != nil {
				connUUID, _ := uuid.Parse(app.ConnectionID)
				_, err := deps.Repos.GitOpsBinding.FindByApp(ctx, connUUID, app.Namespace, app.Name, tenantID.String())
				if err != nil {
					// Binding doesn't exist, create it
					binding := &repository.GitOpsBinding{
						TenantID:         tenantID,
						Name:             app.Name,
						ArgoConnectionID: &connUUID,
						EngineType:       "argocd",
						AppName:          app.Name,
						AppNamespace:     app.Namespace,
						AppProject:       strPtr(app.Project),
						Environment:      strPtr(app.Environment),
						UpdateStrategy:   "kustomize_image",
						AutoBound:        true,
					}
					if err := deps.Repos.GitOpsBinding.Create(ctx, binding); err == nil {
						created++
						disc.Bound = true
						disc.BindingID = &binding.ID
					}
				} else {
					disc.Bound = true
				}
			}
		}

		// List all FluxCD applications
		fluxApps, err := engineClient.List(ctx, engine.ListOptions{
			TenantID:   tenantID,
			EngineType: "fluxcd",
		})
		if err != nil {
			slog.Warn("Failed to list FluxCD apps", "error", err)
		}

		for _, app := range fluxApps {
			connID, _ := uuid.Parse(app.ConnectionID)
			disc := &discoveredApp{
				ConnectionID:   connID,
				ConnectionName: connNames[app.ConnectionID],
				App:            app,
			}
			discovered = append(discovered, disc)

			// Create binding if it doesn't exist
			if deps.Repos.GitOpsBinding != nil {
				connUUID, _ := uuid.Parse(app.ConnectionID)
				_, err := deps.Repos.GitOpsBinding.FindByApp(ctx, connUUID, app.Namespace, app.Name, tenantID.String())
				if err != nil {
					// Binding doesn't exist, create it
					binding := &repository.GitOpsBinding{
						TenantID:         tenantID,
						Name:             app.Name,
						ArgoConnectionID: &connUUID,
						EngineType:       "fluxcd",
						AppName:          app.Name,
						AppNamespace:     app.Namespace,
						Environment:      strPtr(app.Environment),
						UpdateStrategy:   "kustomize_image",
						AutoBound:        true,
					}
					if err := deps.Repos.GitOpsBinding.Create(ctx, binding); err == nil {
						created++
						disc.Bound = true
						disc.BindingID = &binding.ID
					}
				} else {
					disc.Bound = true
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"discovered":  discovered,
			"total":       len(discovered),
			"newly_bound": created,
		})
	}
}

type discoveredApp struct {
	ConnectionID   uuid.UUID        `json:"connection_id"`
	ConnectionName string           `json:"connection_name"`
	App            engine.AppSummary `json:"app"`
	Bound          bool             `json:"bound"`
	BindingID      *uuid.UUID       `json:"binding_id,omitempty"`
}

// listGitOpsBindings returns all bindings for the tenant with joined environment info.
func listGitOpsBindings(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := getTenantID(c)

		if deps.Repos.GitOpsBinding == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "binding repository not available"})
			return
		}

		bindings, err := deps.Repos.GitOpsBinding.ListWithEnvironment(ctx, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"bindings": bindings,
			"total":    len(bindings),
		})
	}
}

// getGitOpsBinding returns a single binding by ID.
func getGitOpsBinding(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := getTenantID(c)

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid binding ID"})
			return
		}

		if deps.Repos.GitOpsBinding == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "binding repository not available"})
			return
		}

		binding, err := deps.Repos.GitOpsBinding.Get(ctx, id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "binding not found"})
			return
		}

		c.JSON(http.StatusOK, binding)
	}
}

// createGitOpsBinding creates a new binding manually.
func createGitOpsBinding(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := getTenantID(c)

		if deps.Repos.GitOpsBinding == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "binding repository not available"})
			return
		}

		var req struct {
			Name             string     `json:"name" binding:"required"`
			ServiceID        *uuid.UUID `json:"service_id"`
			EntityID         *uuid.UUID `json:"entity_id"`
			RepoID           *uuid.UUID `json:"repo_id"`
			ClusterID        *uuid.UUID `json:"cluster_id"`
			ArgoConnectionID *uuid.UUID `json:"argo_connection_id" binding:"required"`
			EngineType       string     `json:"engine_type" binding:"required"`
			AppName          string     `json:"app_name" binding:"required"`
			AppNamespace     string     `json:"app_namespace" binding:"required"`
			AppProject       *string    `json:"app_project"`
			Environment      *string    `json:"environment"`
			EnvironmentID    *uuid.UUID `json:"environment_id"`
			ManifestPath     *string    `json:"manifest_path"`
			UpdateStrategy   string     `json:"update_strategy"`
			UpdatePath       *string    `json:"update_path"`
			VerifyURL        *string    `json:"verify_url"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		strategy := req.UpdateStrategy
		if strategy == "" {
			strategy = "kustomize_image"
		}

		binding := &repository.GitOpsBinding{
			TenantID:         tenantID,
			ServiceID:        req.ServiceID,
			EntityID:         req.EntityID,
			Name:             req.Name,
			RepoID:           req.RepoID,
			ClusterID:        req.ClusterID,
			ArgoConnectionID: req.ArgoConnectionID,
			EngineType:       req.EngineType,
			AppName:          req.AppName,
			AppNamespace:     req.AppNamespace,
			AppProject:       req.AppProject,
			Environment:      req.Environment,
			EnvironmentID:    req.EnvironmentID,
			ManifestPath:     req.ManifestPath,
			UpdateStrategy:   strategy,
			UpdatePath:       req.UpdatePath,
			VerifyURL:        req.VerifyURL,
			AutoBound:        false,
		}

		if err := deps.Repos.GitOpsBinding.Create(ctx, binding); err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusCreated, binding)
	}
}

// updateGitOpsBinding updates an existing binding.
func updateGitOpsBinding(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := getTenantID(c)

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid binding ID"})
			return
		}

		if deps.Repos.GitOpsBinding == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "binding repository not available"})
			return
		}

		// Get existing binding
		existing, err := deps.Repos.GitOpsBinding.Get(ctx, id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "binding not found"})
			return
		}

		var req struct {
			Name              string     `json:"name"`
			ServiceID         *uuid.UUID `json:"service_id"`
			EntityID          *uuid.UUID `json:"entity_id"`
			RepoID            *uuid.UUID `json:"repo_id"`
			ClusterID         *uuid.UUID `json:"cluster_id"`
			ArgoConnectionID  *uuid.UUID `json:"argo_connection_id"`
			EngineType        string     `json:"engine_type"`
			AppName           string     `json:"app_name"`
			AppNamespace      string     `json:"app_namespace"`
			AppProject        *string    `json:"app_project"`
			Environment       *string    `json:"environment"`
			EnvironmentID     *uuid.UUID `json:"environment_id"`
			ClearEnvironment  bool       `json:"clear_environment"`
			ManifestPath      *string    `json:"manifest_path"`
			UpdateStrategy    string     `json:"update_strategy"`
			UpdatePath        *string    `json:"update_path"`
			VerifyURL         *string    `json:"verify_url"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Update fields if provided
		if req.Name != "" {
			existing.Name = req.Name
		}
		if req.ServiceID != nil {
			existing.ServiceID = req.ServiceID
		}
		if req.EntityID != nil {
			existing.EntityID = req.EntityID
		}
		if req.RepoID != nil {
			existing.RepoID = req.RepoID
		}
		if req.ClusterID != nil {
			existing.ClusterID = req.ClusterID
		}
		if req.ArgoConnectionID != nil {
			existing.ArgoConnectionID = req.ArgoConnectionID
		}
		if req.EngineType != "" {
			existing.EngineType = req.EngineType
		}
		if req.AppName != "" {
			existing.AppName = req.AppName
		}
		if req.AppNamespace != "" {
			existing.AppNamespace = req.AppNamespace
		}
		if req.AppProject != nil {
			existing.AppProject = req.AppProject
		}
		if req.ClearEnvironment {
			existing.Environment = nil
			existing.EnvironmentID = nil
		} else {
			if req.Environment != nil {
				existing.Environment = req.Environment
			}
			if req.EnvironmentID != nil {
				existing.EnvironmentID = req.EnvironmentID
			}
		}
		if req.ManifestPath != nil {
			existing.ManifestPath = req.ManifestPath
		}
		if req.UpdateStrategy != "" {
			existing.UpdateStrategy = req.UpdateStrategy
		}
		if req.UpdatePath != nil {
			existing.UpdatePath = req.UpdatePath
		}
		if req.VerifyURL != nil {
			existing.VerifyURL = req.VerifyURL
		}

		if err := deps.Repos.GitOpsBinding.Update(ctx, existing); err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, existing)
	}
}

// deleteGitOpsBinding removes a binding.
func deleteGitOpsBinding(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := getTenantID(c)

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid binding ID"})
			return
		}

		if deps.Repos.GitOpsBinding == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "binding repository not available"})
			return
		}

		if err := deps.Repos.GitOpsBinding.Delete(ctx, id, tenantID); err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "binding deleted"})
	}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// listBindingsByEnvironment returns all bindings for a specific environment.
func listBindingsByEnvironment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := getTenantID(c)

		envID, err := uuid.Parse(c.Param("envId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid environment ID"})
			return
		}

		if deps.Repos.GitOpsBinding == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "binding repository not available"})
			return
		}

		bindings, err := deps.Repos.GitOpsBinding.FindByEnvironment(ctx, envID, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"bindings": bindings,
			"total":    len(bindings),
		})
	}
}

// listBindingsByService returns all bindings for a specific service.
func listBindingsByService(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := getTenantID(c)

		serviceID, err := uuid.Parse(c.Param("serviceId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service ID"})
			return
		}

		if deps.Repos.GitOpsBinding == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "binding repository not available"})
			return
		}

		bindings, err := deps.Repos.GitOpsBinding.FindByService(ctx, serviceID, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"bindings": bindings,
			"total":    len(bindings),
		})
	}
}

// writeBackBinding updates the image tag in the GitOps manifest repo based on
// the binding's update_strategy. This enables GitOps-native deploys: PEPA writes
// the new image tag to Git, and FluxCD/ArgoCD reconciles the change.
func writeBackBinding(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := getTenantID(c)

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid binding ID"})
			return
		}

		var req struct {
			ImageTag      string `json:"image_tag" binding:"required"`
			ImageName     string `json:"image_name"`
			CommitMessage string `json:"commit_message"`
			Branch        string `json:"branch"`
			DryRun        bool   `json:"dry_run"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if deps.Repos.GitopsRepo == nil || deps.Repos.GitOpsBinding == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops subsystem not configured"})
			return
		}

		writer := service.NewManifestWriter(deps.Repos.GitopsRepo, deps.Repos.GitOpsBinding)
		writeCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()

		result, writeErr := writer.WriteBack(writeCtx, tenantID, &service.WriteBackRequest{
			BindingID:     id,
			ImageTag:      req.ImageTag,
			ImageName:     req.ImageName,
			CommitMessage: req.CommitMessage,
			Branch:        req.Branch,
			DryRun:        req.DryRun,
		})
		if writeErr != nil {
			slog.Error("write-back failed", "binding_id", id, "error", writeErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": writeErr.Error()})
			return
		}

		// Resolve connection token for the repo (used for MR URL if needed)
		if result.MRNeeded && deps.Repos.GitopsRepo != nil {
			binding, bErr := deps.Repos.GitOpsBinding.Get(ctx, id, tenantID)
			if bErr == nil && binding != nil && binding.RepoID != nil {
				repo, rErr := deps.Repos.GitopsRepo.Get(ctx, *binding.RepoID)
				if rErr == nil && repo != nil {
					resolveConnectionToken(ctx, deps, repo, auth.GetUserID(c), tenantID)
				}
			}
		}

		slog.Info("write-back complete", "binding_id", id, "commit", result.CommitSHA, "branch", result.Branch, "strategy", result.Strategy)

		c.JSON(http.StatusOK, gin.H{
			"result":  result,
			"message": "image tag written to GitOps manifest repository",
		})
	}
}

// previewWriteBackDiff returns a diff preview without committing.
func previewWriteBackDiff(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		tenantID := getTenantID(c)

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid binding ID"})
			return
		}

		var req struct {
			ImageTag  string `json:"image_tag" binding:"required"`
			ImageName string `json:"image_name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if deps.Repos.GitopsRepo == nil || deps.Repos.GitOpsBinding == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops subsystem not configured"})
			return
		}

		writer := service.NewManifestWriter(deps.Repos.GitopsRepo, deps.Repos.GitOpsBinding)
		previewCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		result, previewErr := writer.WriteBack(previewCtx, tenantID, &service.WriteBackRequest{
			BindingID: id,
			ImageTag:  req.ImageTag,
			ImageName: req.ImageName,
			DryRun:    true,
		})
		if previewErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": previewErr.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"diff":      result.Diff,
			"file_path": result.FilePath,
			"strategy":  result.Strategy,
		})
	}
}

// registerGitOpsBindingRoutes registers all GitOps binding routes.
func registerGitOpsBindingRoutes(v1 *gin.RouterGroup, deps Dependencies) {
	bindings := v1.Group("/gitops/bindings")
	{
		// Discovery
		bindings.POST("/discover", discoverGitOpsApplications(deps))

		// By environment / service (before /:id to avoid route conflict)
		bindings.GET("/by-environment/:envId", listBindingsByEnvironment(deps))
		bindings.GET("/by-service/:serviceId", listBindingsByService(deps))

		// CRUD
		bindings.GET("", listGitOpsBindings(deps))
		bindings.GET("/:id", getGitOpsBinding(deps))
		bindings.POST("", createGitOpsBinding(deps))
		bindings.PUT("/:id", updateGitOpsBinding(deps))
		bindings.DELETE("/:id", deleteGitOpsBinding(deps))

		// Write-back (image-tag → Git)
		bindings.POST("/:id/write-back", writeBackBinding(deps))
		bindings.POST("/:id/write-back/preview", previewWriteBackDiff(deps))
	}
}
