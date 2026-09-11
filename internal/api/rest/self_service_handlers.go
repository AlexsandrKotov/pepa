package rest

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
)

// registerSelfServiceRoutes registers the developer self-service deployment
// endpoints used by the "Deploy My App" wizard.
func registerSelfServiceRoutes(v1 *gin.RouterGroup, deps Dependencies) {
	if deps.Repos.SelfService == nil {
		return
	}

	selfService := v1.Group("/self-service")
	{
		// POST /self-service/deploy - start a new self-service deployment
		selfService.POST("/deploy", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)
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

			envID, err := uuid.Parse(req.EnvironmentID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid environment_id"})
				return
			}

			// Verify the environment belongs to the caller's tenant
			env, err := deps.Repos.Environment.Get(c.Request.Context(), tenantID, envID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "environment not found"})
				return
			}

			// Developers may deploy to dev/staging; production requires admin
			if env.Type == "production" && !containsRole(auth.GetRoles(c), "admin") {
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
				respondInternalError(c, err)
				return
			}

			c.JSON(http.StatusCreated, deployment)
		})

		// GET /self-service/deployments - list deployments visible to the caller
		selfService.GET("/deployments", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)

			// Non-admins see only their own deployments
			var userIDFilter *uuid.UUID
			if !containsRole(auth.GetRoles(c), "admin") {
				userIDFilter = auth.GetUserID(c)
				if userIDFilter == nil {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
					return
				}
			}

			deployments, err := deps.Repos.SelfService.List(c.Request.Context(), tenantID, userIDFilter, c.Query("status"), 50)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			if deployments == nil {
				deployments = []repository.SelfServiceDeployment{}
			}

			c.JSON(http.StatusOK, gin.H{
				"deployments": deployments,
				"total":       len(deployments),
			})
		})

		// GET /self-service/deployments/:id - get a specific deployment
		selfService.GET("/deployments/:id", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)

			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
				return
			}

			deployment, err := deps.Repos.SelfService.Get(c.Request.Context(), tenantID, id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
				return
			}

			if !canAccessSelfServiceDeployment(c, deployment) {
				c.JSON(http.StatusForbidden, gin.H{"error": "cannot access deployment owned by another user"})
				return
			}

			c.JSON(http.StatusOK, deployment)
		})

		// POST /self-service/deployments/:id/cancel - cancel a pending deployment
		selfService.POST("/deployments/:id/cancel", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)

			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
				return
			}

			deployment, err := deps.Repos.SelfService.Get(c.Request.Context(), tenantID, id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
				return
			}

			if !canAccessSelfServiceDeployment(c, deployment) {
				c.JSON(http.StatusForbidden, gin.H{"error": "cannot cancel deployment owned by another user"})
				return
			}

			switch deployment.Status {
			case "pending", "detecting", "building", "deploying":
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "cannot cancel deployment in current status"})
				return
			}

			logs := deployment.Logs
			if logs != "" {
				logs += "\n"
			}
			if err := deps.Repos.SelfService.UpdateStatus(c.Request.Context(), tenantID, id, "cancelled", deployment.Progress, logs+"Cancelled by user", nil); err != nil {
				respondInternalError(c, err)
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "deployment cancelled"})
		})

		// DELETE /self-service/deployments/:id - delete a deployment record
		selfService.DELETE("/deployments/:id", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)

			id, err := uuid.Parse(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
				return
			}

			deployment, err := deps.Repos.SelfService.Get(c.Request.Context(), tenantID, id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
				return
			}

			if !canAccessSelfServiceDeployment(c, deployment) {
				c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete deployment owned by another user"})
				return
			}

			if err := deps.Repos.SelfService.Delete(c.Request.Context(), tenantID, id); err != nil {
				respondInternalError(c, err)
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "deployment deleted"})
		})

		// POST /self-service/detect - detect the project type of a git repo
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

			c.JSON(http.StatusOK, analyzeProjectType(req.GitRepoURL, req.GitBranch))
		})

		// GET /self-service/environments - environments the caller may deploy to
		selfService.GET("/environments", func(c *gin.Context) {
			tenantID := auth.GetTenantID(c)

			envs, err := deps.Repos.Environment.List(c.Request.Context(), tenantID)
			if err != nil {
				respondInternalError(c, err)
				return
			}

			// Production is hidden from everyone who is not an admin
			isAdmin := containsRole(auth.GetRoles(c), "admin")
			accessibleEnvs := make([]repository.Environment, 0, len(envs))
			for _, env := range envs {
				if env.Type == "production" && !isAdmin {
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

// canAccessSelfServiceDeployment reports whether the caller owns the record or
// holds the admin role.
func canAccessSelfServiceDeployment(c *gin.Context, d *repository.SelfServiceDeployment) bool {
	if containsRole(auth.GetRoles(c), "admin") {
		return true
	}
	userID := auth.GetUserID(c)
	return userID != nil && d.UserID == *userID
}

// analyzeProjectType guesses the project type of a repository from its name.
// It performs no network access — the result is only a suggestion the wizard
// lets the developer override.
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

	switch {
	case strings.Contains(lowerURL, "node") || strings.Contains(lowerURL, "next") || strings.Contains(lowerURL, "react"):
		result["detected_type"] = "nodejs"
		result["suggested_blueprint"] = "docker_compose"
		result["confidence"] = 0.7
		indicators = append(indicators, "Node.js/React detected from URL")
	case strings.Contains(lowerURL, "python") || strings.Contains(lowerURL, "django") || strings.Contains(lowerURL, "flask"):
		result["detected_type"] = "python"
		result["suggested_blueprint"] = "docker_compose"
		result["confidence"] = 0.7
		indicators = append(indicators, "Python detected from URL")
	case strings.Contains(lowerURL, "helm") || strings.Contains(lowerURL, "chart"):
		result["detected_type"] = "helm"
		result["suggested_blueprint"] = "helm"
		result["confidence"] = 0.8
		indicators = append(indicators, "Helm chart detected from URL")
	case strings.Contains(lowerURL, "go") || strings.Contains(lowerURL, "golang"):
		result["detected_type"] = "go"
		result["suggested_blueprint"] = "helm"
		result["confidence"] = 0.7
		indicators = append(indicators, "Go detected from URL")
	}

	result["indicators"] = indicators
	return result
}

// containsRole reports whether the caller's JWT role list contains role. The
// "admin" question is answered by auth.IsAdminRoles instead, so that a holder of
// "super_admin"/"platform_admin" is not treated as an ordinary developer here
// while being granted the platform-wide bypass by rbacMiddleware at the same time.
func containsRole(roles []string, role string) bool {
	if strings.EqualFold(role, "admin") {
		return auth.IsAdminRoles(roles)
	}
	for _, r := range roles {
		if strings.EqualFold(r, role) {
			return true
		}
	}
	return false
}
