package rest

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
)

// registerSelfServiceRoutes registers the developer self-service deployment endpoints.
func registerSelfServiceRoutes(v1 *gin.RouterGroup, deps Dependencies) {
	if deps.Repos.SelfService == nil {
		return
	}

	selfService := v1.Group("/self-service")
	{
		// POST /self-service/deploy - Start a new self-service deployment
		selfService.POST("/deploy", func(c *gin.Context) {
			tenantID := getTenantID(c)
			userID := auth.GetUserID(c)
			if userID == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
				return
			}

			var req struct {
				GitRepoURL      string                 `json:"git_repo_url" binding:"required"`
				GitBranch       string                 `json:"git_branch"`
				EnvironmentID   string                 `json:"environment_id" binding:"required"`
				BlueprintType   string                 `json:"blueprint_type"`
				BlueprintConfig map[string]interface{} `json:"blueprint_config"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			// Validate environment ID
			envID, err := uuid.Parse(req.EnvironmentID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid environment_id"})
				return
			}

			// Verify environment exists
			env, err := deps.Repos.Environment.Get(c.Request.Context(), tenantID, envID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "environment not found"})
				return
			}

			// Developers can deploy to dev/staging; production requires admin
			userRoles := auth.GetRoles(c)
			if env.Type == "production" && !containsRole(userRoles, "admin") {
				c.JSON(http.StatusForbidden, gin.H{"error": "production deployments require admin approval"})
				return
			}

			if req.GitBranch == "" {
				req.GitBranch = "main"
			}
			if req.BlueprintType == "" {
				req.BlueprintType = "auto"
			}

			deployment := &repository.SelfServiceDeployment{
				TenantID:        tenantID,
				UserID:          *userID,
				GitRepoURL:      req.GitRepoURL,
				GitBranch:       req.GitBranch,
				EnvironmentID:   &envID,
				BlueprintType:   req.BlueprintType,
				BlueprintConfig: req.BlueprintConfig,
				Status:          "pending",
				Progress:        0,
			}

			if err := deps.Repos.SelfService.Create(c.Request.Context(), deployment); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusCreated, deployment)
		})

		// GET /self-service/deployments - List self-service deployments
		selfService.GET("/deployments", func(c *gin.Context) {
			tenantID := getTenantID(c)

			// Non-admins see only their own deployments
			userRoles := auth.GetRoles(c)
			var userIDFilter *uuid.UUID
			if !containsRole(userRoles, "admin") {
				userIDFilter = auth.GetUserID(c)
			}

			status := c.Query("status")
			deployments, err := deps.Repos.SelfService.List(c.Request.Context(), tenantID, userIDFilter, status, 50)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"deployments": deployments,
				"total":       len(deployments),
			})
		})

		// GET /self-service/deployments/:id - Get a specific self-service deployment
		selfService.GET("/deployments/:id", func(c *gin.Context) {
			tenantID := getTenantID(c)

			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
				return
			}

			deployment, err := deps.Repos.SelfService.Get(c.Request.Context(), tenantID, id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, deployment)
		})

		// POST /self-service/deployments/:id/cancel - Cancel a pending deployment
		selfService.POST("/deployments/:id/cancel", func(c *gin.Context) {
			tenantID := getTenantID(c)

			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
				return
			}

			deployment, err := deps.Repos.SelfService.Get(c.Request.Context(), tenantID, id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}

			// Ownership check: only the owner or an admin can cancel
			userID := auth.GetUserID(c)
			userRoles := auth.GetRoles(c)
			if userID != nil && !containsRole(userRoles, "admin") && deployment.UserID != *userID {
				c.JSON(http.StatusForbidden, gin.H{"error": "cannot cancel deployment owned by another user"})
				return
			}

			// Can only cancel pending or in-progress deployments
			if deployment.Status != "pending" && deployment.Status != "detecting" && deployment.Status != "building" && deployment.Status != "deploying" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "cannot cancel deployment in current status"})
				return
			}

			if err := deps.Repos.SelfService.UpdateStatus(c.Request.Context(), tenantID, id, "cancelled", deployment.Progress, deployment.Logs+"\nCancelled by user", nil); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "deployment cancelled"})
		})

		// DELETE /self-service/deployments/:id - Delete a self-service deployment record
		selfService.DELETE("/deployments/:id", func(c *gin.Context) {
			tenantID := getTenantID(c)

			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
				return
			}

			// Ownership check: only the owner or an admin can delete
			deployment, err := deps.Repos.SelfService.Get(c.Request.Context(), tenantID, id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}

			userID := auth.GetUserID(c)
			userRoles := auth.GetRoles(c)
			if userID != nil && !containsRole(userRoles, "admin") && deployment.UserID != *userID {
				c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete deployment owned by another user"})
				return
			}

			if err := deps.Repos.SelfService.Delete(c.Request.Context(), tenantID, id); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "deployment deleted"})
		})

		// POST /self-service/detect - Detect project type from a git repo
		selfService.POST("/detect", func(c *gin.Context) {
			var req struct {
				GitRepoURL string `json:"git_repo_url" binding:"required"`
				GitBranch  string `json:"git_branch"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			if req.GitBranch == "" {
				req.GitBranch = "main"
			}

			detection := analyzeProjectType(req.GitRepoURL, req.GitBranch)
			c.JSON(http.StatusOK, detection)
		})

		// GET /self-service/environments - Get environments available for self-service deployment
		selfService.GET("/environments", func(c *gin.Context) {
			tenantID := getTenantID(c)

			envs, err := deps.Repos.Environment.List(c.Request.Context(), tenantID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			// Filter: developers can access dev/staging, admins can access all
			userRoles := auth.GetRoles(c)
			accessibleEnvs := make([]repository.Environment, 0)
			for _, env := range envs {
				if env.Type == "production" && !containsRole(userRoles, "admin") {
					continue
				}
				accessibleEnvs = append(accessibleEnvs, env)
			}

			c.JSON(http.StatusOK, gin.H{
				"environments": accessibleEnvs,
				"total":        len(accessibleEnvs),
			})
		})
	}
}

// analyzeProjectType analyzes a git repository to detect the project type and suggest a blueprint.
func analyzeProjectType(repoURL, branch string) map[string]interface{} {
	result := map[string]interface{}{
		"git_repo_url":        repoURL,
		"git_branch":          branch,
		"detected_type":       "unknown",
		"suggested_blueprint": "docker_compose",
		"confidence":          0.5,
		"indicators":          []string{},
	}

	lowerURL := strings.ToLower(repoURL)
	indicators := []string{}

	if strings.Contains(lowerURL, "node") || strings.Contains(lowerURL, "next") || strings.Contains(lowerURL, "react") {
		result["detected_type"] = "nodejs"
		result["suggested_blueprint"] = "docker_compose"
		result["confidence"] = 0.7
		indicators = append(indicators, "Node.js/React detected from URL")
	} else if strings.Contains(lowerURL, "python") || strings.Contains(lowerURL, "django") || strings.Contains(lowerURL, "flask") {
		result["detected_type"] = "python"
		result["suggested_blueprint"] = "docker_compose"
		result["confidence"] = 0.7
		indicators = append(indicators, "Python detected from URL")
	} else if strings.Contains(lowerURL, "go") || strings.Contains(lowerURL, "golang") {
		result["detected_type"] = "go"
		result["suggested_blueprint"] = "helm"
		result["confidence"] = 0.7
		indicators = append(indicators, "Go detected from URL")
	} else if strings.Contains(lowerURL, "helm") || strings.Contains(lowerURL, "chart") {
		result["detected_type"] = "helm"
		result["suggested_blueprint"] = "helm"
		result["confidence"] = 0.8
		indicators = append(indicators, "Helm chart detected from URL")
	}

	result["indicators"] = indicators
	return result
}

func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if strings.EqualFold(r, role) {
			return true
		}
	}
	return false
}
