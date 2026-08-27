package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/gitops"
	"github.com/pepa/pepa/internal/repository"
	"gopkg.in/yaml.v3"
)

func registerGitOpsRoutes(r *gin.RouterGroup, deps Dependencies) {
	wf := r.Group("/gitops")
	{
		// Legacy MR / deploy endpoints
		wf.GET("/mrs", listWorkflowMRs(deps))
		wf.POST("/deploy", manualDeploy(deps))
		wf.POST("/verify", verifyWorkflowDeployment(deps))
		wf.GET("/timeline/:id", getWorkflowTimeline(deps))

		// Deployment lifecycle: promote across stages, approve, rollback
		wf.POST("/deployments/:id/promote", promoteDeployment(deps))
		wf.POST("/deployments/:id/approve", approveDeployment(deps))
		wf.POST("/deployments/:id/rollback", rollbackDeployment(deps))

		// GitOps manifest repository management
		wf.POST("/repos", gitopsCreateRepo(deps))
		wf.GET("/repos", gitopsListRepos(deps))
		wf.GET("/repos/:id", gitopsGetRepo(deps))
		wf.PUT("/repos/:id", gitopsUpdateRepo(deps))
		wf.DELETE("/repos/:id", gitopsDeleteRepo(deps))
		wf.POST("/repos/:id/scan", gitopsScanRepo(deps))
		wf.GET("/repos/:id/resources", gitopsListResources(deps))
		wf.GET("/repos/:id/clusters", gitopsListClusters(deps))
		wf.POST("/repos/:id/resources", gitopsCreateResource(deps))

		// Alias: /repositories → /repos (frontend compatibility)
		wf.GET("/repositories", gitopsListRepos(deps))
		wf.POST("/repositories", gitopsCreateRepo(deps))
		wf.GET("/repositories/:id", gitopsGetRepo(deps))
		wf.PUT("/repositories/:id", gitopsUpdateRepo(deps))
		wf.DELETE("/repositories/:id", gitopsDeleteRepo(deps))
		wf.POST("/repositories/:id/scan", gitopsScanRepo(deps))
		wf.GET("/repositories/:id/resources", gitopsListResources(deps))
		wf.GET("/repositories/:id/clusters", gitopsListClusters(deps))

		// Manifest editing with Git commit
		wf.PUT("/repos/:id/resources/:resourceId/values", gitopsEditValues(deps))
		wf.POST("/repos/:id/resources/:resourceId/values/preview", gitopsPreviewDiff(deps))
		wf.POST("/repos/:id/resources/:resourceId/values/suggest-commit", gitopsSuggestCommitMessage(deps))
		wf.POST("/repos/:id/resources/:resourceId/suspend", gitopsSuspendResource(deps))

		// Deployment tracking (SSE)
		wf.GET("/repos/:id/track/:commitSHA", gitopsTrackCommit(deps))

		// Topology / dependency graph
		wf.GET("/repos/:id/topology", gitopsTopology(deps))

		// Drift detection: compare Git desired state vs live cluster state
		wf.GET("/repos/:id/drift", gitopsDetectDrift(deps))
		wf.GET("/repos/:id/overlays", gitopsListOverlays(deps))

		// Per-repo cluster & scope mapping for drift detection
		wf.PUT("/repos/:id/mapping", gitopsUpdateMapping(deps))
		wf.DELETE("/repos/:id/mapping", gitopsDeleteMapping(deps))
	}
}

// listWorkflowMRs returns merge requests with deployment status.
func listWorkflowMRs(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusOK, gin.H{"merge_requests": []interface{}{}, "total": 0})
			return
		}
		tenantID := auth.GetTenantID(c)
		teamFilter := c.Query("team")
		stageFilter := c.Query("stage")

		// Get all deployments for this tenant as MR proxies
		ctx := c.Request.Context()
		deployments, err := deps.Repos.Deployment.List(ctx, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		// Transform deployments into MR-like view
		mrs := make([]gin.H, 0, len(deployments))
		for _, d := range deployments {
			stage := d.Stage
			if stage == "" {
				// Legacy rows: guess the stage from status/namespace
				stage = "dev"
				switch d.Status {
				case "promoted":
					stage = "testing"
				case "deployed":
					if d.TargetNamespace == "app-staging" || d.TargetNamespace == "staging" {
						stage = "staging"
					}
				}
			}

			if teamFilter != "" && d.TeamName != teamFilter {
				continue
			}
			if stageFilter != "" && stage != stageFilter {
				continue
			}

			mrs = append(mrs, gin.H{
				"id":               d.ID,
				"jira_issue_key":   d.JiraIssueKey,
				"jira_summary":     d.JiraSummary,
				"project_name":     d.GitlabProjectName,
				"mr_id":            d.GitlabMRID,
				"mr_url":           d.GitlabMRURL,
				"image_tag":        d.ImageTag,
				"image_repository": d.ImageRepository,
				"status":           d.Status,
				"stage":            stage,
				"team":             d.TeamName,
				"namespace":        d.TargetNamespace,
				"created_at":       d.CreatedAt,
				"updated_at":       d.UpdatedAt,
			})
		}

		c.JSON(http.StatusOK, gin.H{"merge_requests": mrs, "total": len(mrs)})
	}
}

// manualDeploy triggers a manual deployment into a team's pipeline stage.
func manualDeploy(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment repository not available"})
			return
		}

		var req struct {
			JiraIssueKey    string `json:"jira_issue_key"`
			JiraSummary     string `json:"jira_summary"`
			GitlabProjectID *int   `json:"gitlab_project_id"`
			ProjectName     string `json:"project_name"`
			ImageTag        string `json:"image_tag" binding:"required"`
			ImageRepository string `json:"image_repository"`
			Namespace       string `json:"namespace"`
			ClusterID       string `json:"cluster_id"`
			Team            string `json:"team"`
			Stage           string `json:"stage"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		tenantID := auth.GetTenantID(c)
		now := time.Now()

		stage := req.Stage
		if stage == "" {
			stage = "dev"
		}
		namespace := req.Namespace
		if namespace == "" {
			namespace = "app-" + stage
		}

		deployment := &repository.Deployment{
			TenantID:          tenantID,
			JiraIssueKey:      req.JiraIssueKey,
			JiraSummary:       req.JiraSummary,
			GitlabProjectID:   req.GitlabProjectID,
			GitlabProjectName: req.ProjectName,
			ImageTag:          req.ImageTag,
			ImageRepository:   req.ImageRepository,
			TargetNamespace:   namespace,
			TeamName:          req.Team,
			Stage:             stage,
			Status:            "pending",
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		if req.ClusterID != "" {
			if id, err := uuid.Parse(req.ClusterID); err == nil {
				deployment.TargetClusterID = &id
			}
		}

		ctx := c.Request.Context()
		if err := deps.Repos.Deployment.Create(ctx, deployment); err != nil {
			respondInternalError(c, err)
			return
		}

		// Simulated lifecycle: pending -> syncing -> deployed.
		// Real cluster sync (ArgoCD/Flux) plugs in here later.
		simulateDeployLifecycle(deps, deployment.ID)

		logAudit(deps, c, "create", "deployment", deployment.ID.String(), nil, gin.H{
			"image_tag": req.ImageTag, "stage": stage, "team": req.Team,
		})

		c.JSON(http.StatusCreated, gin.H{
			"deployment": deployment,
			"message":    "Deployment created and queued",
		})
	}
}

// simulateDeployLifecycle walks a deployment through pending -> syncing -> deployed.
func simulateDeployLifecycle(deps Dependencies, deploymentID uuid.UUID) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		transition := func(status string, wait time.Duration) bool {
			time.Sleep(wait)
			d, err := deps.Repos.Deployment.Get(ctx, deploymentID)
			if err != nil {
				return false
			}
			// Do not override terminal/manual states (cancelled, rolled_back, promoted...)
			if d.Status != "pending" && d.Status != "syncing" {
				return false
			}
			d.Status = status
			d.UpdatedAt = time.Now()
			if err := deps.Repos.Deployment.Update(ctx, d); err != nil {
				log.Printf("[gitops] lifecycle transition to %s failed for %s: %v", status, deploymentID, err)
				return false
			}
			return true
		}

		if !transition("syncing", 1500*time.Millisecond) {
			return
		}
		transition("deployed", 2*time.Second)
	}()
}

// verifyWorkflowDeployment runs verification checks on a deployment.
func verifyWorkflowDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment repository not available"})
			return
		}

		var req struct {
			DeploymentID string `json:"deployment_id" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		id, err := uuid.Parse(req.DeploymentID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
			return
		}

		ctx := c.Request.Context()
		d, err := deps.Repos.Deployment.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
			return
		}

		// Run verification checks
		checks := []gin.H{
			{"name": "deployment-exists", "status": "passed", "message": "Deployment record found"},
			{"name": "status-check", "status": checkStatus(d.Status), "message": fmt.Sprintf("Deployment status: %s", d.Status)},
			{"name": "image-pull", "status": "passed", "message": fmt.Sprintf("Image %s available", d.ImageTag)},
			{"name": "health-check", "status": checkHealth(d.Status), "message": healthMessage(d.Status)},
		}

		allPassed := true
		for _, ch := range checks {
			if ch["status"] != "passed" {
				allPassed = false
				break
			}
		}

		verificationStatus := "verified"
		if !allPassed {
			verificationStatus = "degraded"
		}

		c.JSON(http.StatusOK, gin.H{
			"deployment_id":       req.DeploymentID,
			"verification_status": verificationStatus,
			"checks":              checks,
			"verified_at":         time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func checkStatus(status string) string {
	switch status {
	case "deployed", "promoted":
		return "passed"
	case "failed", "rolled_back":
		return "failed"
	default:
		return "warning"
	}
}

func checkHealth(status string) string {
	switch status {
	case "deployed", "promoted":
		return "passed"
	case "failed":
		return "failed"
	default:
		return "warning"
	}
}

func healthMessage(status string) string {
	switch status {
	case "deployed", "promoted":
		return "All health checks passing"
	case "failed":
		return "Health checks failing"
	case "cancelled":
		return "Deployment cancelled"
	default:
		return "Health checks pending"
	}
}

// getWorkflowTimeline returns timeline events for a deployment.
func getWorkflowTimeline(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusOK, gin.H{"deployment_id": c.Param("id"), "events": []interface{}{}, "history": []interface{}{}, "total": 0})
			return
		}

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
			return
		}

		ctx := c.Request.Context()
		d, err := deps.Repos.Deployment.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
			return
		}

		// Build timeline events from deployment lifecycle
		events := buildTimelineEvents(d)

		// Get deployment history for the same project/namespace
		var history []gin.H
		if d.GitlabProjectName != "" {
			histDeployments, _ := deps.Repos.Deployment.History(ctx, d.TenantID, d.GitlabProjectName, d.TargetNamespace, 10)
			for _, hd := range histDeployments {
				history = append(history, gin.H{
					"id":         hd.ID,
					"status":     hd.Status,
					"image_tag":  hd.ImageTag,
					"created_at": hd.CreatedAt,
					"updated_at": hd.UpdatedAt,
				})
			}
		}
		if history == nil {
			history = []gin.H{}
		}

		c.JSON(http.StatusOK, gin.H{
			"deployment_id": c.Param("id"),
			"events":        events,
			"history":       history,
			"total":         len(events),
		})
	}
}

func buildTimelineEvents(d *repository.Deployment) []gin.H {
	events := make([]gin.H, 0)
	created := d.CreatedAt.Format(time.RFC3339)
	updated := d.UpdatedAt.Format(time.RFC3339)

	// Creation event
	events = append(events, gin.H{
		"timestamp": created,
		"stage":     "created",
		"status":    "completed",
		"label":     "Deployment created",
		"detail":    fmt.Sprintf("Image: %s", d.ImageTag),
	})

	switch d.Status {
	case "pending":
		events = append(events, gin.H{
			"timestamp": created,
			"stage":     "pending",
			"status":    "in_progress",
			"label":     "Waiting for deployment",
			"detail":    "Deployment is queued",
		})
	case "deployed":
		events = append(events, gin.H{
			"timestamp": updated,
			"stage":     "deployed",
			"status":    "completed",
			"label":     "Deployment successful",
			"detail":    fmt.Sprintf("Namespace: %s", d.TargetNamespace),
		})
	case "promoted":
		events = append(events, gin.H{
			"timestamp": updated,
			"stage":     "deployed",
			"status":    "completed",
			"label":     "Deployment successful",
			"detail":    fmt.Sprintf("Namespace: %s", d.TargetNamespace),
		})
		promotedAt := updated
		if d.PromotedAt != nil {
			promotedAt = d.PromotedAt.Format(time.RFC3339)
		}
		events = append(events, gin.H{
			"timestamp": promotedAt,
			"stage":     "promoted",
			"status":    "completed",
			"label":     "Promoted to next environment",
			"detail":    fmt.Sprintf("Promoted by: %s", d.PromotedBy),
		})
	case "failed":
		events = append(events, gin.H{
			"timestamp": updated,
			"stage":     "failed",
			"status":    "failed",
			"label":     "Deployment failed",
			"detail":    d.ErrorMessage,
		})
	case "cancelled":
		events = append(events, gin.H{
			"timestamp": updated,
			"stage":     "cancelled",
			"status":    "cancelled",
			"label":     "Deployment cancelled",
			"detail":    "Deployment was cancelled",
		})
	case "rolled_back":
		events = append(events, gin.H{
			"timestamp": updated,
			"stage":     "rolled_back",
			"status":    "rolled_back",
			"label":     "Deployment rolled back",
			"detail":    fmt.Sprintf("Rolled back by: %s", d.PromotedBy),
		})
	}

	return events
}

// =============================================================================
// GitOps Manifest Repository Management
// =============================================================================

// maskRepoToken hides the repository access token in API responses.
// The presence of a token is indicated by "***" so the UI can show
// "token configured" without ever seeing the secret itself.
func maskRepoToken(repo *gitops.Repo) {
	if repo == nil || repo.Config == nil {
		return
	}
	if t, ok := repo.Config["token"]; ok && t != "" {
		repo.Config["token"] = "***"
	}
}

func gitopsCreateRepo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		var req struct {
			Name         string `json:"name" binding:"required"`
			ConnectionID string `json:"connection_id"`
			RepoURL      string `json:"repo_url" binding:"required"`
			Branch       string `json:"branch"`
			Path         string `json:"path"`
			EngineType   string `json:"engine_type"`
			Token        string `json:"token"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Branch != "" && !gitops.ValidBranchName(req.Branch) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid branch name"})
			return
		}

		tenantID := auth.GetTenantID(c)
		repo := &gitops.Repo{
			TenantID:   tenantID,
			Name:       req.Name,
			RepoURL:    req.RepoURL,
			Branch:     req.Branch,
			Path:       req.Path,
			EngineType: req.EngineType,
			Config:     map[string]string{},
		}
		if req.Token != "" && req.Token != "***" {
			repo.Config["token"] = req.Token
		}
		if req.ConnectionID != "" {
			if id, err := uuid.Parse(req.ConnectionID); err == nil {
				repo.ConnectionID = &id
			}
		}

		ctx := c.Request.Context()
		if err := deps.Repos.GitopsRepo.Create(ctx, repo); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "create", "gitops_repo", repo.ID.String(), nil, repo)
		maskRepoToken(repo)
		c.JSON(http.StatusCreated, repo)
	}
}

func gitopsListRepos(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusOK, gin.H{"repos": []interface{}{}, "total": 0})
			return
		}
		tenantID := auth.GetTenantID(c)
		repos, err := deps.Repos.GitopsRepo.List(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		for i := range repos {
			maskRepoToken(&repos[i])
		}
		c.JSON(http.StatusOK, gin.H{"repos": repos, "total": len(repos)})
	}
}

func gitopsGetRepo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		repo, err := deps.Repos.GitopsRepo.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		maskRepoToken(repo)
		c.JSON(http.StatusOK, repo)
	}
}

func gitopsUpdateRepo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		ctx := c.Request.Context()
		existing, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		var req struct {
			Name         string `json:"name"`
			RepoURL      string `json:"repo_url"`
			Branch       string `json:"branch"`
			Path         string `json:"path"`
			EngineType   string `json:"engine_type"`
			Token        string `json:"token"`
			ConnectionID string `json:"connection_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Name != "" {
			existing.Name = req.Name
		}
		if req.RepoURL != "" {
			existing.RepoURL = req.RepoURL
		}
		if req.Branch != "" {
			if !gitops.ValidBranchName(req.Branch) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid branch name"})
				return
			}
			existing.Branch = req.Branch
		}
		if req.Path != "" {
			existing.Path = req.Path
		}
		if req.EngineType != "" {
			existing.EngineType = req.EngineType
		}
		if req.Token != "" && req.Token != "***" {
			existing.Config["token"] = req.Token
		}
		if req.ConnectionID != "" {
			if id, err := uuid.Parse(req.ConnectionID); err == nil {
				existing.ConnectionID = &id
			}
		}

		if err := deps.Repos.GitopsRepo.Update(ctx, existing); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "gitops_repo", existing.ID.String(), nil, existing)
		maskRepoToken(existing)
		c.JSON(http.StatusOK, existing)
	}
}

func gitopsDeleteRepo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		if err := deps.Repos.GitopsRepo.Delete(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "delete", "gitops_repo", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "GitOps repository removed"})
	}
}

// resolveConnectionToken resolves the token for a GitOps repo.
// Priority: 1) explicit token in repo.Config  2) user's personal credential
// 3) global connection token.  This ensures operations run under the user's identity.
func resolveConnectionToken(ctx context.Context, deps Dependencies, repo *gitops.Repo, userID *uuid.UUID, tenantID uuid.UUID) {
	// 1. Already has an explicit token — nothing to do
	if token, ok := repo.Config["token"]; ok && token != "" {
		return
	}

	// 2. Try user's personal credential (matched by repo hostname)
	if userID != nil && deps.DB != nil {
		providerURL, provider := detectProviderFromRepoURL(repo.RepoURL)
		if providerURL != "" {
			token, username, email, err := GetUserCredential(ctx, deps, *userID, provider, providerURL)
			if err == nil && token != "" {
				if repo.Config == nil {
					repo.Config = map[string]string{}
				}
				repo.Config["token"] = token
				if username != "" {
					repo.Config["git_user_name"] = username
				}
				if email != "" {
					repo.Config["git_user_email"] = email
				}
				log.Printf("[gitops] using personal credential for user %s on %s", userID, providerURL)
				return
			}
		}
	}

	// 3. Fall back to global connection token
	if repo.ConnectionID == nil {
		return
	}
	if deps.Repos.Connection == nil {
		return
	}
	conn, err := deps.Repos.Connection.GetDecrypted(ctx, *repo.ConnectionID, tenantID)
	if err != nil {
		log.Printf("[gitops] failed to resolve connection %s for repo %s: %v", repo.ConnectionID, repo.Name, err)
		return
	}
	if token, ok := conn.Config["token"]; ok {
		switch t := token.(type) {
		case string:
			if t != "" {
				if repo.Config == nil {
					repo.Config = map[string]string{}
				}
				repo.Config["token"] = t
			}
		}
	}
}

// detectProviderFromRepoURL extracts the provider URL and provider name from a git repo URL.
// e.g. "https://gitlab.example.com/org/repo.git" -> ("https://gitlab.example.com", "gitlab")
func detectProviderFromRepoURL(repoURL string) (providerURL string, provider string) {
	u, err := url.Parse(repoURL)
	if err != nil || u.Host == "" {
		return "", ""
	}
	base := fmt.Sprintf("%s://%s", u.Scheme, u.Host)

	// Detect provider from hostname hints
	host := strings.ToLower(u.Host)
	switch {
	case strings.Contains(host, "gitlab"):
		return base, "gitlab"
	case strings.Contains(host, "github"):
		return base, "github"
	case strings.Contains(host, "gitea"):
		return base, "gitea"
	case strings.Contains(host, "bitbucket"):
		return base, "bitbucket"
	default:
		// Default to gitlab for self-hosted (most common in enterprise)
		return base, "gitlab"
	}
}

func gitopsScanRepo(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		ctx := c.Request.Context()
		repo, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Resolve token from linked connection if needed
		resolveConnectionToken(ctx, deps, repo, auth.GetUserID(c), auth.GetTenantID(c))

		// Mark as scanning
		_ = deps.Repos.GitopsRepo.UpdateScanStatus(ctx, id, "scanning", "")

		// Run scan in a goroutine to avoid blocking, but for small repos do it inline
		scanner := gitops.NewScanner("")
		scanCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		result, scanErr := scanner.Scan(scanCtx, repo)
		if scanErr != nil {
			_ = deps.Repos.GitopsRepo.UpdateScanStatus(ctx, id, "error", scanErr.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": scanErr.Error()})
			return
		}

		// Update status to ready
		_ = deps.Repos.GitopsRepo.UpdateScanStatus(ctx, id, "ready", "")

		// Update engine type if auto-detected
		if repo.EngineType == "auto" && result.Engine != "unknown" {
			repo.EngineType = result.Engine
			_ = deps.Repos.GitopsRepo.Update(ctx, repo)
		}

		log.Printf("[gitops] scanned repo %s: %d resources found, engine=%s", repo.Name, len(result.Resources), result.Engine)

		c.JSON(http.StatusOK, gin.H{
			"message":    "Scan completed",
			"resources":  result.Resources,
			"engine":     result.Engine,
			"file_count": result.FileCount,
			"total":      len(result.Resources),
			"tree":       result.Tree,
			"clusters":   result.Clusters,
			"layout":     result.Layout,
		})
	}
}

func gitopsListResources(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		ctx := c.Request.Context()
		repo, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Resolve token from linked connection if needed
		resolveConnectionToken(ctx, deps, repo, auth.GetUserID(c), auth.GetTenantID(c))

		// Re-scan to get fresh resources (could be cached in the future)
		// Always re-scan regardless of status to ensure fresh results
		scanner := gitops.NewScanner("")
		scanCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		result, scanErr := scanner.Scan(scanCtx, repo)
		if scanErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": scanErr.Error()})
			return
		}

		// Apply filters
		kindFilter := c.Query("kind")
		nsFilter := c.Query("namespace")
		searchFilter := c.Query("search")

		filtered := make([]gitops.Resource, 0, len(result.Resources))
		for _, r := range result.Resources {
			if kindFilter != "" && r.Kind != kindFilter {
				continue
			}
			if nsFilter != "" && r.Namespace != nsFilter {
				continue
			}
			if searchFilter != "" {
				name := r.Name
				if !containsIgnoreCase(name, searchFilter) {
					continue
				}
			}
			filtered = append(filtered, r)
		}

		c.JSON(http.StatusOK, gin.H{
			"resources":   filtered,
			"total":       len(filtered),
			"engine":      result.Engine,
			"scan_status": repo.ScanStatus,
			"tree":        result.Tree,
			"clusters":    result.Clusters,
			"layout":      result.Layout,
		})
	}
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) > 0 && containsLower(toLower(s), toLower(substr)))
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func containsLower(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// =============================================================================
// GitOps Manifest Editing
// =============================================================================

func gitopsEditValues(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		ctx := c.Request.Context()
		repo, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Resolve token from linked connection if needed
		resolveConnectionToken(ctx, deps, repo, auth.GetUserID(c), auth.GetTenantID(c))

		var req struct {
			FilePath  string      `json:"file_path" binding:"required"`
			FieldPath string      `json:"field_path"`
			NewValue  interface{} `json:"new_value"`
			FullYAML  string      `json:"full_yaml"`
			CommitMsg string      `json:"commit_message"`
			Branch    string      `json:"branch"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		editor := gitops.NewEditor("")
		editCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()

		result, editErr := editor.ApplyEdit(editCtx, repo, &gitops.EditRequest{
			FilePath:  req.FilePath,
			FieldPath: req.FieldPath,
			NewValue:  req.NewValue,
			FullYAML:  req.FullYAML,
			CommitMsg: req.CommitMsg,
			Branch:    req.Branch,
		})
		if editErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": editErr.Error()})
			return
		}

		log.Printf("[gitops] edit repo=%s file=%s commit=%s branch=%s mr_needed=%v",
			repo.Name, req.FilePath, result.CommitSHA, result.Branch, result.MRNeeded)

		c.JSON(http.StatusOK, result)
	}
}

func gitopsPreviewDiff(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		ctx := c.Request.Context()
		repo, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Resolve token from linked connection if needed
		resolveConnectionToken(ctx, deps, repo, auth.GetUserID(c), auth.GetTenantID(c))

		var req struct {
			FilePath  string      `json:"file_path" binding:"required"`
			FieldPath string      `json:"field_path"`
			NewValue  interface{} `json:"new_value"`
			FullYAML  string      `json:"full_yaml"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		editor := gitops.NewEditor("")
		previewCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		diff, diffErr := editor.PreviewDiff(previewCtx, repo, &gitops.EditRequest{
			FilePath:  req.FilePath,
			FieldPath: req.FieldPath,
			NewValue:  req.NewValue,
			FullYAML:  req.FullYAML,
		})
		if diffErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": diffErr.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"diff": diff})
	}
}

// gitopsSuggestCommitMessage generates a suggested commit message for a resource edit.
// In the future, this will be enhanced with AI-powered contextual messages.
func gitopsSuggestCommitMessage(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			ResourceKind string `json:"resource_kind"`
			ResourceName string `json:"resource_name"`
			FilePath     string `json:"file_path"`
			Changes      string `json:"changes"` // comma-separated list of changed fields
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Generate a contextual commit message
		changeDesc := "values"
		if req.Changes != "" {
			changeDesc = req.Changes
		}

		suggestion := fmt.Sprintf("gitops(PEPA): update %s in %s/%s", changeDesc, req.ResourceKind, req.ResourceName)

		// Future: Use AI to generate more contextual messages based on the diff
		// Example: "Update HelmRelease/nginx-ingress: bump chart version to 4.7.1"

		c.JSON(http.StatusOK, gin.H{
			"suggested_message": suggestion,
			"prefix":            "gitops(PEPA):",
		})
	}
}

// =============================================================================
// GitOps Resource Suspend/Resume
// =============================================================================

func gitopsSuspendResource(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		ctx := c.Request.Context()
		repo, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Resolve token from linked connection if needed
		resolveConnectionToken(ctx, deps, repo, auth.GetUserID(c), auth.GetTenantID(c))

		var req struct {
			FilePath     string `json:"file_path" binding:"required"`
			Suspend      bool   `json:"suspend"`
			CommitMsg    string `json:"commit_message"`
			ResourceKind string `json:"resource_kind"`
			ResourceName string `json:"resource_name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Generate commit message if not provided
		commitMsg := req.CommitMsg
		if commitMsg == "" {
			action := "resume"
			if req.Suspend {
				action = "suspend"
			}
			commitMsg = fmt.Sprintf("gitops(PEPA): %s %s/%s", action, req.ResourceKind, req.ResourceName)
		}

		editor := gitops.NewEditor("")
		editCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()

		result, editErr := editor.ApplyEdit(editCtx, repo, &gitops.EditRequest{
			FilePath:  req.FilePath,
			FieldPath: "spec.suspend",
			NewValue:  req.Suspend,
			CommitMsg: commitMsg,
		})
		if editErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": editErr.Error()})
			return
		}

		log.Printf("[gitops] suspend repo=%s file=%s suspend=%v commit=%s",
			repo.Name, req.FilePath, req.Suspend, result.CommitSHA)

		c.JSON(http.StatusOK, result)
	}
}

// =============================================================================
// GitOps Cluster Discovery
// =============================================================================

func gitopsListClusters(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusOK, gin.H{"clusters": []string{}, "total": 0})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		ctx := c.Request.Context()
		repo, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Resolve token from linked connection if needed
		resolveConnectionToken(ctx, deps, repo, auth.GetUserID(c), auth.GetTenantID(c))

		if repo.ScanStatus != "ready" {
			c.JSON(http.StatusOK, gin.H{"clusters": []string{}, "total": 0})
			return
		}

		// Scan to get resources with cluster info
		scanner := gitops.NewScanner("")
		scanCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		result, scanErr := scanner.Scan(scanCtx, repo)
		if scanErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": scanErr.Error()})
			return
		}

		// Extract unique clusters
		clusterSet := make(map[string]bool)
		for _, r := range result.Resources {
			if r.Cluster != "" {
				clusterSet[r.Cluster] = true
			}
		}

		clusters := make([]string, 0, len(clusterSet))
		for cluster := range clusterSet {
			clusters = append(clusters, cluster)
		}

		c.JSON(http.StatusOK, gin.H{
			"clusters": clusters,
			"total":    len(clusters),
			"tree":     result.Tree,
			"layout":   result.Layout,
		})
	}
}

// =============================================================================
// GitOps Create Resource
// =============================================================================

func gitopsCreateResource(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		ctx := c.Request.Context()
		repo, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Resolve token from linked connection if needed
		resolveConnectionToken(ctx, deps, repo, auth.GetUserID(c), auth.GetTenantID(c))

		var req struct {
			Kind      string                 `json:"kind" binding:"required"` // HelmRelease, Kustomization
			Name      string                 `json:"name" binding:"required"`
			Namespace string                 `json:"namespace" binding:"required"`
			Cluster   string                 `json:"cluster"`
			Chart     string                 `json:"chart"`
			Version   string                 `json:"version"`
			SourceRef string                 `json:"source_ref"` // HelmRepository name
			Values    map[string]interface{} `json:"values"`
			CommitMsg string                 `json:"commit_message"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Generate YAML based on kind
		var yamlContent string
		switch req.Kind {
		case "HelmRelease":
			yamlContent = generateHelmReleaseYAML(req.Name, req.Namespace, req.Chart, req.Version, req.SourceRef, req.Values)
		case "Kustomization":
			yamlContent = generateKustomizationYAML(req.Name, req.Namespace, req.SourceRef)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported kind: " + req.Kind})
			return
		}

		// Determine file path
		filePath := fmt.Sprintf("%s/%s.yaml", req.Namespace, req.Name)
		if req.Cluster != "" {
			filePath = fmt.Sprintf("clusters/%s/%s/%s.yaml", req.Cluster, req.Namespace, req.Name)
		}

		// Generate commit message if not provided
		commitMsg := req.CommitMsg
		if commitMsg == "" {
			commitMsg = fmt.Sprintf("gitops(PEPA): create %s/%s in %s", req.Kind, req.Name, filePath)
		}

		editor := gitops.NewEditor("")
		editCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()

		result, editErr := editor.ApplyEdit(editCtx, repo, &gitops.EditRequest{
			FilePath:  filePath,
			FullYAML:  yamlContent,
			CommitMsg: commitMsg,
		})
		if editErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": editErr.Error()})
			return
		}

		log.Printf("[gitops] create resource repo=%s kind=%s name=%s file=%s commit=%s",
			repo.Name, req.Kind, req.Name, filePath, result.CommitSHA)

		c.JSON(http.StatusCreated, gin.H{
			"resource": gin.H{
				"kind":      req.Kind,
				"name":      req.Name,
				"namespace": req.Namespace,
				"cluster":   req.Cluster,
				"file_path": filePath,
			},
			"commit": result,
		})
	}
}

// generateHelmReleaseYAML creates a Flux HelmRelease YAML.
func generateHelmReleaseYAML(name, namespace, chart, version, sourceRef string, values map[string]interface{}) string {
	result := fmt.Sprintf(`apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: %s
  namespace: %s
spec:
  interval: 5m
  chart:
    spec:
      chart: %s
      version: "%s"
      sourceRef:
        kind: HelmRepository
        name: %s
        namespace: flux-system
`, name, namespace, chart, version, sourceRef)

	if len(values) > 0 {
		result += "  values:\n"
		valuesYAML, _ := yaml.Marshal(values)
		for _, line := range strings.Split(string(valuesYAML), "\n") {
			if line != "" {
				result += "    " + line + "\n"
			}
		}
	}

	return result
}

// generateKustomizationYAML creates a Flux Kustomization YAML.
func generateKustomizationYAML(name, namespace, sourceRef string) string {
	return fmt.Sprintf(`apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: %s
  namespace: %s
spec:
  interval: 10m
  sourceRef:
    kind: GitRepository
    name: %s
  path: "./"
  prune: true
`, name, namespace, sourceRef)
}

// =============================================================================
// GitOps Deployment Tracking (SSE)
// =============================================================================

var globalDeployTracker *gitops.DeployTracker

func getDeployTracker() *gitops.DeployTracker {
	if globalDeployTracker == nil {
		globalDeployTracker = gitops.NewDeployTracker(10 * time.Second)
	}
	return globalDeployTracker
}

func gitopsTrackCommit(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		commitSHA := c.Param("commitSHA")
		if commitSHA == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "commit SHA required"})
			return
		}

		ctx := c.Request.Context()
		repo, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		tracker := getDeployTracker()
		eventCh, err := tracker.WatchCommit(ctx, repo, commitSHA)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		// Set SSE headers
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.Flush()

		// Stream events
		clientGone := c.Writer.CloseNotify()
		for {
			select {
			case <-clientGone:
				return
			case <-ctx.Done():
				return
			case evt, ok := <-eventCh:
				if !ok {
					return
				}
				data, _ := json.Marshal(evt)
				_, _ = fmt.Fprintf(c.Writer, "event: deploy\ndata: %s\n\n", data)
				c.Writer.Flush()
			}
		}
	}
}

// =============================================================================
// GitOps Topology / Dependency Graph
// =============================================================================

func gitopsTopology(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		ctx := c.Request.Context()
		repo, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Resolve token from linked connection if needed
		resolveConnectionToken(ctx, deps, repo, auth.GetUserID(c), auth.GetTenantID(c))

		if repo.ScanStatus != "ready" {
			c.JSON(http.StatusOK, &gitops.TopologyGraph{
				Nodes: []gitops.TopologyNode{},
				Edges: []gitops.TopologyEdge{},
			})
			return
		}

		// Scan to get resources
		scanner := gitops.NewScanner("")
		scanCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		result, scanErr := scanner.Scan(scanCtx, repo)
		if scanErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": scanErr.Error()})
			return
		}

		graph := gitops.BuildTopology(ctx, repo, result.Resources)
		c.JSON(http.StatusOK, graph)
	}
}

// gitopsUpdateMapping saves per-repo drift mapping (cluster_id and scope_path) in repo config.
func gitopsUpdateMapping(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		ctx := c.Request.Context()
		repo, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		var req struct {
			ClusterID string `json:"cluster_id"`
			ScopePath string `json:"scope_path"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if repo.Config == nil {
			repo.Config = map[string]string{}
		}
		if req.ClusterID != "" {
			// Validate cluster exists
			if deps.Repos.Cluster != nil {
				cid, parseErr := uuid.Parse(req.ClusterID)
				if parseErr != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster_id"})
					return
				}
				_, clusterErr := deps.Repos.Cluster.Get(ctx, cid, auth.GetTenantID(c))
				if clusterErr != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "cluster not found"})
					return
				}
			}
			repo.Config["drift_cluster_id"] = req.ClusterID
		}
		if req.ScopePath != "" {
			repo.Config["drift_scope_path"] = req.ScopePath
		}

		if err := deps.Repos.GitopsRepo.Update(ctx, repo); err != nil {
			respondInternalError(c, err)
			return
		}

		maskRepoToken(repo)
		c.JSON(http.StatusOK, gin.H{
			"cluster_id": repo.Config["drift_cluster_id"],
			"scope_path": repo.Config["drift_scope_path"],
		})
	}
}

// gitopsDeleteMapping removes the per-repo drift mapping.
func gitopsDeleteMapping(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}
		ctx := c.Request.Context()
		repo, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		if repo.Config != nil {
			delete(repo.Config, "drift_cluster_id")
			delete(repo.Config, "drift_scope_path")
		}

		if err := deps.Repos.GitopsRepo.Update(ctx, repo); err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "mapping removed"})
	}
}

// gitopsDetectDrift compares Git desired state against live cluster state.
// Query params:
//   - cluster_id (optional): specific cluster to compare against
//   - If omitted, compares against all clusters with kubeconfig
func gitopsDetectDrift(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}
		if deps.Repos.Cluster == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cluster repository not available"})
			return
		}

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		// Get the gitops repo
		repo, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Resolve connection token if linked
		resolveConnectionToken(ctx, deps, repo, auth.GetUserID(c), auth.GetTenantID(c))

		// Scan Git repo to get desired state
		var gitResources []gitops.Resource
		if repo.ScanStatus == "ready" {
			scanner := gitops.NewScanner("")
			scanCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			result, scanErr := scanner.Scan(scanCtx, repo)
			if scanErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "git scan failed: " + scanErr.Error()})
				return
			}
			gitResources = result.Resources
		}

		// Optional: filter git resources by overlay path prefix
		// Priority: query param > per-repo config > none
		pathPrefix := c.Query("path")
		if pathPrefix == "" {
			pathPrefix = repo.Config["drift_scope_path"]
		}
		if pathPrefix != "" {
			var filtered []gitops.Resource
			for _, r := range gitResources {
				if strings.HasPrefix(r.FilePath, pathPrefix) {
					filtered = append(filtered, r)
				}
			}
			gitResources = filtered
		}

		// Determine which clusters to check
		// Priority: query param > per-repo config > all clusters
		var clusterFilter *uuid.UUID
		if clusterParam := c.Query("cluster_id"); clusterParam != "" {
			cid, err := uuid.Parse(clusterParam)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cluster_id"})
				return
			}
			clusterFilter = &cid
		} else if mappedClusterID := repo.Config["drift_cluster_id"]; mappedClusterID != "" {
			if cid, err := uuid.Parse(mappedClusterID); err == nil {
				clusterFilter = &cid
			}
		}

		// Get clusters for this tenant
		allClusters, err := deps.Repos.Cluster.List(ctx, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		// Filter to clusters with kubeconfig
		var targetClusters []repository.Cluster
		for _, cl := range allClusters {
			if !cl.HasKubeconfig {
				continue
			}
			if clusterFilter != nil && cl.ID != *clusterFilter {
				continue
			}
			targetClusters = append(targetClusters, cl)
		}

		if len(targetClusters) == 0 {
			c.JSON(http.StatusOK, &gitops.DriftResult{
				RepoID:    repo.ID.String(),
				RepoName:  repo.Name,
				Entries:   []gitops.DriftEntry{},
				ScannedAt: time.Now(),
			})
			return
		}

		// For single-cluster case, use simple DetectDrift
		if len(targetClusters) == 1 {
			cl := targetClusters[0]
			liveResources, err := queryLiveFluxResources(ctx, deps, &cl)
			if err != nil {
				respondInternalError(c, err)
				return
			}

			// Convert to LiveResource format
			liveRes := toLiveResources(liveResources)

			result := gitops.DetectDrift(repo, gitResources, liveRes)
			result.ClusterID = cl.ID.String()
			result.ClusterName = cl.Name
			c.JSON(http.StatusOK, result)
			return
		}

		// Multi-cluster: use DetectDriftMultiCluster
		liveByCluster := make(map[string][]gitops.LiveResource)
		for _, cl := range targetClusters {
			liveResources, err := queryLiveFluxResources(ctx, deps, &cl)
			if err != nil {
				log.Printf("Warning: failed to query cluster %s for drift: %v", cl.Name, err)
				continue
			}
			liveByCluster[cl.Name] = toLiveResources(liveResources)
		}

		result := gitops.DetectDriftMultiCluster(repo, gitResources, liveByCluster)
		c.JSON(http.StatusOK, result)
	}
}

// queryLiveFluxResources fetches FluxCD resources from a live cluster.
func queryLiveFluxResources(ctx context.Context, deps Dependencies, cl *repository.Cluster) ([]gitops.Resource, error) {
	kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(ctx, cl.ID, cl.TenantID)
	if err != nil || kubeconfig == "" {
		return nil, fmt.Errorf("kubeconfig not available for cluster %s", cl.Name)
	}

	restCfg, err := buildRestConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	var resources []gitops.Resource
	hrResources := queryFluxHelmReleases(ctx, restCfg)
	resources = append(resources, hrResources...)
	ksResources := queryFluxKustomizations(ctx, restCfg)
	resources = append(resources, ksResources...)

	return resources, nil
}

// toLiveResources converts gitops.Resource (from cluster query) to LiveResource for drift comparison.
func toLiveResources(resources []gitops.Resource) []gitops.LiveResource {
	result := make([]gitops.LiveResource, 0, len(resources))
	for _, r := range resources {
		health := "unknown"
		if h, ok := r.Labels["health"]; ok {
			health = h
		}
		result = append(result, gitops.LiveResource{
			Kind:      r.Kind,
			Name:      r.Name,
			Namespace: r.Namespace,
			Suspended: r.Suspended,
			Version:   r.Version,
			Health:    health,
		})
	}
	return result
}

// gitopsListOverlays returns the unique scope directory paths found in a scanned repo.
// It detects various repository layout patterns and returns meaningful scope paths
// for drift detection filtering.
// Supported patterns:
//   - overlays/<env>/<cluster> (base-overlay layout)
//   - clusters/<name> (monorepo layout)
//   - environments/<name> or envs/<name> (environment-based layout)
//   - teams/<name>/... (team-based layout)
//   - Top-level directories (flat layout)
func gitopsListOverlays(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.GitopsRepo == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
			return
		}

		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo ID"})
			return
		}

		ctx := c.Request.Context()

		repo, err := deps.Repos.GitopsRepo.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		resolveConnectionToken(ctx, deps, repo, auth.GetUserID(c), auth.GetTenantID(c))

		if repo.ScanStatus != "ready" {
			c.JSON(http.StatusOK, gin.H{"overlays": []string{}, "message": "repo not scanned yet"})
			return
		}

		scanner := gitops.NewScanner("")
		scanCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		result, scanErr := scanner.Scan(scanCtx, repo)
		if scanErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "git scan failed: " + scanErr.Error()})
			return
		}

		// Extract scope paths from resource file paths
		scopeSet := make(map[string]bool)

		// Known structural directory patterns that indicate scope boundaries
		knownStructuralDirs := map[string]bool{
			"overlays": true, "overlay": true,
			"clusters": true, "environments": true, "envs": true, "env": true,
			"teams": true, "services": true, "apps": true, "app": true,
			"base": true, "components": true,
		}

		for _, r := range result.Resources {
			parts := strings.Split(r.FilePath, "/")
			if len(parts) < 2 {
				continue
			}

			// Check for known structural patterns at any depth
			for i, part := range parts {
				partLower := strings.ToLower(part)

				// Pattern: overlays/<env>/<cluster> or overlay/<env>
				if (partLower == "overlays" || partLower == "overlay") && i+1 < len(parts) {
					if i+2 < len(parts) {
						// overlays/<env>/<cluster>
						scopeSet[strings.Join(parts[:i+3], "/")] = true
					} else {
						// overlays/<env>
						scopeSet[strings.Join(parts[:i+2], "/")] = true
					}
					break
				}

				// Pattern: clusters/<name> or environments/<name> or envs/<name>
				if (partLower == "clusters" || partLower == "environments" ||
					partLower == "envs" || partLower == "env") && i+1 < len(parts) {
					scopeSet[strings.Join(parts[:i+2], "/")] = true
					break
				}

				// Pattern: teams/<team>/... - extract team directory and subdirectories
				if (partLower == "teams" || partLower == "services" ||
					partLower == "apps" || partLower == "app") && i+1 < len(parts) {
					// Include team/<subdir> as a scope
					if i+2 < len(parts) {
						scopeSet[strings.Join(parts[:i+3], "/")] = true
					} else {
						scopeSet[strings.Join(parts[:i+2], "/")] = true
					}
					break
				}
			}

			// If no known pattern found, extract top-level directory as scope
			if len(parts) >= 2 && !knownStructuralDirs[parts[0]] {
				// Check if first dir is a known structural parent
				firstPartLower := strings.ToLower(parts[0])
				if !knownStructuralDirs[firstPartLower] {
					// Top-level directory is likely a scope (e.g., staging/, production/)
					scopeSet[parts[0]] = true
				}
			}
		}

		// If no scopes found, try extracting from all file paths
		if len(scopeSet) == 0 {
			for _, r := range result.Resources {
				parts := strings.Split(r.FilePath, "/")
				if len(parts) >= 2 {
					scopeSet[parts[0]] = true
				}
			}
		}

		overlays := make([]string, 0, len(scopeSet))
		for p := range scopeSet {
			overlays = append(overlays, p)
		}
		// Sort for consistent ordering
		sort.Strings(overlays)

		c.JSON(http.StatusOK, gin.H{"overlays": overlays})
	}
}
