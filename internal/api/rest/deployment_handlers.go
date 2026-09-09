package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/events"
	"github.com/pepa/pepa/internal/k8s"
	"github.com/pepa/pepa/internal/repository"
)

func registerDeploymentRoutes(r *gin.RouterGroup, deps Dependencies) {
	deployments := r.Group("/deployments")
	{
		deployments.GET("", listDeployments(deps))
		deployments.POST("", createDeployment(deps))
		deployments.POST("/dry-run", dryRunDeployment(deps))
		deployments.GET("/pipeline", getDeploymentPipeline(deps))
		deployments.GET("/metrics", getDeploymentMetrics(deps))
		deployments.GET("/:id", getDeployment(deps))
		deployments.DELETE("/:id", removeDeployment(deps))
		deployments.POST("/:id/promote", promoteDeployment(deps))
		deployments.POST("/:id/rollback", rollbackDeployment(deps))
		deployments.POST("/:id/cancel", cancelDeployment(deps))
		deployments.POST("/:id/retry", retryDeployment(deps))
		deployments.GET("/:id/history", getDeploymentHistory(deps))
		deployments.GET("/:id/logs", getDeploymentLogs(deps))
		deployments.GET("/:id/diff", getDeploymentDiff(deps))
		deployments.GET("/:id/resources", getDeploymentResources(deps))
		deployments.GET("/:id/events", getDeploymentTimelineEvents(deps))
	}
}

func listDeployments(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusOK, gin.H{"deployments": []interface{}{}, "total": 0})
			return
		}
		tenantID := auth.GetTenantID(c)
		items, err := deps.Repos.Deployment.List(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		// Apply optional query filters.
		statusFilter := c.Query("status")
		teamFilter := c.Query("team")
		serviceFilter := c.Query("service")
		limitStr := c.DefaultQuery("limit", "0")
		limit := 0
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}

		filtered := make([]repository.Deployment, 0, len(items))
		for _, d := range items {
			if statusFilter != "" && d.Status != statusFilter {
				continue
			}
			if teamFilter != "" && d.TeamName != teamFilter {
				continue
			}
			if serviceFilter != "" && d.GitlabProjectName != serviceFilter {
				continue
			}
			filtered = append(filtered, d)
		}

		if limit > 0 && len(filtered) > limit {
			filtered = filtered[:limit]
		}

		c.JSON(http.StatusOK, gin.H{"deployments": filtered, "total": len(filtered)})
	}
}

func createDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment repository not available"})
			return
		}
		var req struct {
			JiraIssueKey      string          `json:"jira_issue_key"`
			JiraSummary       string          `json:"jira_summary"`
			GitlabProjectID   *int            `json:"gitlab_project_id"`
			GitlabProjectName string          `json:"gitlab_project_name"`
			GitlabMRID        *int            `json:"gitlab_mr_id"`
			GitlabMRURL       string          `json:"gitlab_mr_url"`
			TargetClusterID   *uuid.UUID      `json:"target_cluster_id"`
			TargetNamespace   string          `json:"target_namespace"`
			ImageTag          string          `json:"image_tag"`
			ImageRepository   string          `json:"image_repository"`
			DeployType        string          `json:"deploy_type"`
			Replicas          int             `json:"replicas"`
			Strategy          string          `json:"strategy"`
			Spec              json.RawMessage `json:"spec"`
			CreatedBy         string          `json:"created_by"`
			Status            string          `json:"status"`
			TimeoutSeconds    int             `json:"timeout_seconds"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		deployType := req.DeployType
		if deployType == "" {
			deployType = "helm"
		}
		replicas := req.Replicas
		if replicas <= 0 {
			replicas = 1
		}
		strategy := req.Strategy
		if strategy == "" {
			strategy = "rolling"
		}
		spec := req.Spec
		if len(spec) == 0 {
			spec = json.RawMessage("{}")
		}
		timeoutSeconds := req.TimeoutSeconds
		if timeoutSeconds <= 0 {
			timeoutSeconds = 300 // default 5 minutes
		}

		d := &repository.Deployment{
			TenantID:          auth.GetTenantID(c),
			JiraIssueKey:      req.JiraIssueKey,
			JiraSummary:       req.JiraSummary,
			GitlabProjectID:   req.GitlabProjectID,
			GitlabProjectName: req.GitlabProjectName,
			GitlabMRID:        req.GitlabMRID,
			GitlabMRURL:       req.GitlabMRURL,
			TargetClusterID:   req.TargetClusterID,
			TargetNamespace:   req.TargetNamespace,
			ImageTag:          req.ImageTag,
			ImageRepository:   req.ImageRepository,
			DeployType:        deployType,
			Replicas:          replicas,
			Strategy:          strategy,
			Spec:              spec,
			Status:            "pending",
			CreatedBy:         req.CreatedBy,
			TimeoutSeconds:    timeoutSeconds,
		}
		if err := deps.Repos.Deployment.Create(c.Request.Context(), d); err != nil {
			respondInternalError(c, err)
			return
		}

		// If a target cluster is specified, enqueue the deployment job
		if d.TargetClusterID != nil && deps.Repos.Cluster != nil {
			if deps.JobQueue != nil {
				// Enqueue to Redis for async processing by worker
				var specMap interface{}
				_ = json.Unmarshal(d.Spec, &specMap)
				err := deps.JobQueue.Enqueue("deployment.execute", d.TenantID.String(), map[string]interface{}{
					"deployment_id":   d.ID.String(),
					"cluster_id":      d.TargetClusterID.String(),
					"namespace":       d.TargetNamespace,
					"release_name":    d.GitlabProjectName,
					"replicas":        d.Replicas,
					"timeout_seconds": d.TimeoutSeconds,
					"spec":            specMap,
				})
				if err != nil {
					slog.Info("Failed to enqueue deployment job", "id", d.ID, "error", err)
					// Fallback to goroutine if queue fails
					go performDeployment(d.ID, *d.TargetClusterID, d.TargetNamespace,
						d.GitlabProjectName, d.Replicas, d.Spec, d.TimeoutSeconds, deps)
				}
			} else {
				// No queue available, fallback to goroutine
				go performDeployment(d.ID, *d.TargetClusterID, d.TargetNamespace,
					d.GitlabProjectName, d.Replicas, d.Spec, d.TimeoutSeconds, deps)
			}
		}

		logAudit(deps, c, "create", "deployment", d.ID.String(), nil, gin.H{"deploy_type": d.DeployType, "status": d.Status})

		// Publish deployment.created event
		publishDeploymentEvent(deps, "deployment.created", d, nil)

		c.JSON(http.StatusCreated, d)
	}
}

// performDeployment runs the actual Kubernetes deployment in the background.
func performDeployment(deploymentID, clusterID uuid.UUID, namespace, releaseName string, replicas int, specJSON json.RawMessage, timeoutSeconds int, deps Dependencies) {
	if deps.Services == nil || deps.Services.Deployment == nil {
		slog.Info("Deployment skipped: deployment service not available", "id", deploymentID)
		return
	}

	// Use the service layer for deployment logic
	result := deps.Services.Deployment.PerformDeployment(
		context.Background(),
		deploymentID,
		clusterID,
		namespace,
		releaseName,
		replicas,
		specJSON,
		timeoutSeconds,
	)

	if !result.Success {
		slog.Info("Deployment failed", "id", deploymentID, "error", result.Message)
		// Publish deployment.failed event
		if deps.EventBus != nil {
			dep, _ := deps.Repos.Deployment.Get(context.Background(), deploymentID)
			if dep != nil {
				publishDeploymentEvent(deps, "deployment.failed", dep, map[string]interface{}{"error": result.Message})
			}
		}
	} else {
		// Publish deployment.succeeded event
		if deps.EventBus != nil {
			dep, _ := deps.Repos.Deployment.Get(context.Background(), deploymentID)
			if dep != nil {
				publishDeploymentEvent(deps, "deployment.succeeded", dep, nil)
			}
		}
	}
}

func getDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
			return
		}
		d, err := deps.Repos.Deployment.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, d)
	}
}

func promoteDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment repository not available"})
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
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if d.Status != "deployed" {
			c.JSON(http.StatusConflict, gin.H{"error": "deployment must be in 'deployed' status to promote, current: " + d.Status})
			return
		}

		stages := loadTeamStages(deps, c, d.TenantID, d.TeamName)
		idx := stageIndex(stages, d.Stage)
		if idx < 0 || idx >= len(stages)-1 {
			// Unknown or final stage — legacy behavior: just mark as promoted.
			promotedBy := currentUserLabel(c)
			if err := deps.Repos.Deployment.Promote(ctx, id, promotedBy); err != nil {
				respondInternalError(c, err)
				return
			}
			d, _ = deps.Repos.Deployment.Get(ctx, id)
			logAudit(deps, c, "promote", "deployment", id.String(), nil, nil)
			publishDeploymentEvent(deps, "deployment.promoted", d, nil)
			c.JSON(http.StatusOK, gin.H{"deployment": d, "awaiting_approval": false})
			return
		}
		next := stages[idx+1]

		// Target stage requires approval — park the deployment and wait.
		if next.Approval {
			d.Status = "awaiting_approval"
			d.UpdatedAt = time.Now()
			if err := deps.Repos.Deployment.Update(ctx, d); err != nil {
				respondInternalError(c, err)
				return
			}
			logAudit(deps, c, "promote.request", "deployment", d.ID.String(), nil, gin.H{"target_stage": next.Key})
			c.JSON(http.StatusOK, gin.H{
				"deployment":        d,
				"awaiting_approval": true,
				"target_stage":      next.Key,
				"message":           fmt.Sprintf("Promotion to %q requires approval", next.Label),
			})
			return
		}

		completePromotion(deps, c, d, next)
	}
}

func rollbackDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
			return
		}

		// Validate the deployment exists and can be rolled back
		d, err := deps.Repos.Deployment.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if d.Status != "deployed" && d.Status != "promoted" {
			c.JSON(http.StatusConflict, gin.H{"error": "deployment cannot be rolled back: must be in 'deployed' or 'promoted' status"})
			return
		}

		rolledBackBy := currentUserLabel(c)

		// Use the DeploymentService for real rollback if available
		if deps.Services.Deployment != nil && d.TargetClusterID != nil {
			go func() {
				result := deps.Services.Deployment.PerformRollback(context.Background(), id, rolledBackBy)
				dep, _ := deps.Repos.Deployment.Get(context.Background(), id)
				if !result.Success {
					slog.Info("Rollback failed", "id", id, "error", result.Message)
					if dep != nil {
						publishDeploymentEvent(deps, "deployment.rollback_failed", dep, map[string]interface{}{"error": result.Message})
					}
				} else if dep != nil {
					publishDeploymentEvent(deps, "deployment.rolled_back", dep, nil)
				}
			}()
			logAudit(deps, c, "rollback", "deployment", id.String(), nil, gin.H{"rolled_back_by": rolledBackBy})
			publishDeploymentEvent(deps, "deployment.rollback_initiated", d, nil)
			c.JSON(http.StatusOK, gin.H{"deployment": d, "message": "rollback initiated"})
			return
		}

		// Fallback: DB-only rollback (no target cluster)
		if err := deps.Repos.Deployment.Rollback(c.Request.Context(), id, rolledBackBy); err != nil {
			if strings.Contains(err.Error(), "cannot be rolled back") {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			respondInternalError(c, err)
			return
		}
		d, _ = deps.Repos.Deployment.Get(c.Request.Context(), id)
		logAudit(deps, c, "rollback", "deployment", id.String(), nil, nil)
		publishDeploymentEvent(deps, "deployment.rolled_back", d, nil)
		c.JSON(http.StatusOK, gin.H{"deployment": d, "message": "rollback initiated"})
	}
}

func cancelDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
			return
		}
		if err := deps.Repos.Deployment.Cancel(c.Request.Context(), id); err != nil {
			if strings.Contains(err.Error(), "cannot be cancelled") {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			respondInternalError(c, err)
			return
		}
		d, _ := deps.Repos.Deployment.Get(c.Request.Context(), id)
		logAudit(deps, c, "cancel", "deployment", id.String(), nil, nil)
		publishDeploymentEvent(deps, "deployment.cancelled", d, nil)
		c.JSON(http.StatusOK, gin.H{"deployment": d, "message": "deployment cancelled"})
	}
}

func getDeploymentHistory(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusOK, gin.H{"history": []interface{}{}, "total": 0})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
			return
		}
		// Get the deployment to find project/namespace context
		d, err := deps.Repos.Deployment.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		tenantID := auth.GetTenantID(c)
		history, err := deps.Repos.Deployment.History(c.Request.Context(), tenantID, d.GitlabProjectName, d.TargetNamespace, 20)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"history": history, "total": len(history)})
	}
}

func getDeploymentLogs(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
			return
		}
		d, err := deps.Repos.Deployment.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Build structured log entries from the deployment's stored logs
		logEntries := make([]map[string]string, 0)
		if d.Logs != "" {
			for _, line := range strings.Split(d.Logs, "\n") {
				if line == "" {
					continue
				}
				level := "info"
				if strings.HasPrefix(line, "ERROR:") {
					level = "error"
				} else if strings.HasPrefix(line, "WARN:") {
					level = "warn"
				} else if strings.HasPrefix(line, "SUCCESS:") {
					level = "info"
				}
				logEntries = append(logEntries, map[string]string{
					"timestamp": d.UpdatedAt.Format(time.RFC3339),
					"level":     level,
					"message":   line,
				})
			}
		}

		// If deployment failed, add the error message as a log entry
		if d.Status == "failed" && d.ErrorMessage != "" {
			logEntries = append(logEntries, map[string]string{
				"timestamp": d.UpdatedAt.Format(time.RFC3339),
				"level":     "error",
				"message":   "DEPLOYMENT FAILED: " + d.ErrorMessage,
			})
		}

		// If deployment is still in progress, add a status entry
		if d.Status == "pending" || d.Status == "syncing" {
			logEntries = append(logEntries, map[string]string{
				"timestamp": d.UpdatedAt.Format(time.RFC3339),
				"level":     "info",
				"message":   fmt.Sprintf("Deployment is %s... (timeout: %ds)", d.Status, d.TimeoutSeconds),
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"logs":          logEntries,
			"deployment_id": d.ID.String(),
			"status":        d.Status,
			"error_message": d.ErrorMessage,
		})
	}
}

func retryDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
			return
		}

		ctx := c.Request.Context()
		original, err := deps.Repos.Deployment.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Only failed deployments can be retried
		if original.Status != "failed" {
			c.JSON(http.StatusConflict, gin.H{"error": "only failed deployments can be retried, current status: " + original.Status})
			return
		}

		// Create a new deployment record with the same spec
		newDeploy := &repository.Deployment{
			TenantID:          original.TenantID,
			JiraIssueKey:      original.JiraIssueKey,
			JiraSummary:       original.JiraSummary,
			GitlabProjectID:   original.GitlabProjectID,
			GitlabProjectName: original.GitlabProjectName,
			TargetClusterID:   original.TargetClusterID,
			TargetNamespace:   original.TargetNamespace,
			ImageTag:          original.ImageTag,
			ImageRepository:   original.ImageRepository,
			DeployType:        original.DeployType,
			Replicas:          original.Replicas,
			Strategy:          original.Strategy,
			Spec:              original.Spec,
			Status:            "pending",
			CreatedBy:         currentUserLabel(c),
			TimeoutSeconds:    original.TimeoutSeconds,
			TeamName:          original.TeamName,
			Stage:             original.Stage,
		}
		if err := deps.Repos.Deployment.Create(ctx, newDeploy); err != nil {
			respondInternalError(c, err)
			return
		}

		// Enqueue the deployment for execution
		if newDeploy.TargetClusterID != nil && deps.Repos.Cluster != nil {
			if deps.JobQueue != nil {
				var specMap interface{}
				_ = json.Unmarshal(newDeploy.Spec, &specMap)
				_ = deps.JobQueue.Enqueue("deployment.execute", newDeploy.TenantID.String(), map[string]interface{}{
					"deployment_id":   newDeploy.ID.String(),
					"cluster_id":      newDeploy.TargetClusterID.String(),
					"namespace":       newDeploy.TargetNamespace,
					"release_name":    newDeploy.GitlabProjectName,
					"replicas":        newDeploy.Replicas,
					"timeout_seconds": newDeploy.TimeoutSeconds,
					"spec":            specMap,
				})
			} else if deps.Services.Deployment != nil {
				go performDeployment(newDeploy.ID, *newDeploy.TargetClusterID, newDeploy.TargetNamespace,
					newDeploy.GitlabProjectName, newDeploy.Replicas, newDeploy.Spec, newDeploy.TimeoutSeconds, deps)
			}
		}

		logAudit(deps, c, "retry", "deployment", newDeploy.ID.String(), nil, gin.H{"original_id": id.String()})
		publishDeploymentEvent(deps, "deployment.created", newDeploy, nil)

		c.JSON(http.StatusAccepted, gin.H{
			"deployment":  newDeploy,
			"original_id": id.String(),
			"message":     "retry deployment created",
		})
	}
}

func getDeploymentDiff(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
			return
		}
		compareWithStr := c.Query("compare_with")
		if compareWithStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "compare_with query parameter is required"})
			return
		}
		compareWithID, err := uuid.Parse(compareWithStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid compare_with ID"})
			return
		}

		ctx := c.Request.Context()
		d1, err := deps.Repos.Deployment.Get(ctx, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found: " + err.Error()})
			return
		}
		d2, err := deps.Repos.Deployment.Get(ctx, compareWithID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "comparison deployment not found: " + err.Error()})
			return
		}

		// Build field-level diff
		type diffEntry struct {
			Field    string      `json:"field"`
			OldValue interface{} `json:"old_value"`
			NewValue interface{} `json:"new_value"`
		}
		var diffs []diffEntry

		if d1.ImageTag != d2.ImageTag {
			diffs = append(diffs, diffEntry{"image_tag", d2.ImageTag, d1.ImageTag})
		}
		if d1.ImageRepository != d2.ImageRepository {
			diffs = append(diffs, diffEntry{"image_repository", d2.ImageRepository, d1.ImageRepository})
		}
		if d1.Replicas != d2.Replicas {
			diffs = append(diffs, diffEntry{"replicas", d2.Replicas, d1.Replicas})
		}
		if d1.Strategy != d2.Strategy {
			diffs = append(diffs, diffEntry{"strategy", d2.Strategy, d1.Strategy})
		}
		if d1.DeployType != d2.DeployType {
			diffs = append(diffs, diffEntry{"deploy_type", d2.DeployType, d1.DeployType})
		}
		if d1.TargetNamespace != d2.TargetNamespace {
			diffs = append(diffs, diffEntry{"target_namespace", d2.TargetNamespace, d1.TargetNamespace})
		}
		if d1.Stage != d2.Stage {
			diffs = append(diffs, diffEntry{"stage", d2.Stage, d1.Stage})
		}
		if d1.Status != d2.Status {
			diffs = append(diffs, diffEntry{"status", d2.Status, d1.Status})
		}
		if string(d1.Spec) != string(d2.Spec) {
			diffs = append(diffs, diffEntry{"spec", string(d2.Spec), string(d1.Spec)})
		}

		c.JSON(http.StatusOK, gin.H{
			"deployment_a":  d1,
			"deployment_b":  d2,
			"diffs":         diffs,
			"total_changes": len(diffs),
		})
	}
}

func getDeploymentPipeline(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusOK, gin.H{"stages": []interface{}{}, "projects": []string{}})
			return
		}
		tenantID := auth.GetTenantID(c)
		projectFilter := c.Query("project")

		items, err := deps.Repos.Deployment.List(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		// Group by project name
		projectMap := make(map[string][]repository.Deployment)
		projectSet := make(map[string]bool)
		for _, d := range items {
			if projectFilter != "" && d.GitlabProjectName != projectFilter {
				continue
			}
			projectSet[d.GitlabProjectName] = true
			projectMap[d.GitlabProjectName] = append(projectMap[d.GitlabProjectName], d)
		}

		// Build pipeline view: for each project, find the latest deployment per stage
		type stageInfo struct {
			Stage      string `json:"stage"`
			Status     string `json:"status"`
			ImageTag   string `json:"image_tag"`
			DeployedAt string `json:"deployed_at"`
			DeployID   string `json:"deployment_id"`
		}
		type pipelineEntry struct {
			Project string      `json:"project"`
			Stages  []stageInfo `json:"stages"`
		}

		var pipelines []pipelineEntry
		for project, deploys := range projectMap {
			// Find latest deployment per stage
			stageMap := make(map[string]*repository.Deployment)
			for i := range deploys {
				d := &deploys[i]
				existing, ok := stageMap[d.Stage]
				if !ok || d.CreatedAt.After(existing.CreatedAt) {
					stageMap[d.Stage] = d
				}
			}

			var stages []stageInfo
			for stageName, d := range stageMap {
				stages = append(stages, stageInfo{
					Stage:      stageName,
					Status:     d.Status,
					ImageTag:   d.ImageTag,
					DeployedAt: d.CreatedAt.Format(time.RFC3339),
					DeployID:   d.ID.String(),
				})
			}
			pipelines = append(pipelines, pipelineEntry{Project: project, Stages: stages})
		}

		var projects []string
		for p := range projectSet {
			projects = append(projects, p)
		}

		c.JSON(http.StatusOK, gin.H{
			"pipelines": pipelines,
			"projects":  projects,
		})
	}
}

func getDeploymentMetrics(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusOK, gin.H{})
			return
		}
		tenantID := auth.GetTenantID(c)
		period := c.DefaultQuery("period", "30d")

		items, err := deps.Repos.Deployment.List(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		// Parse period
		days := 30
		if strings.HasSuffix(period, "d") {
			if _, err := fmt.Sscanf(period, "%dd", &days); err != nil {
				days = 30
			}
		}
		cutoff := time.Now().AddDate(0, 0, -days)

		// Filter by period
		var filtered []repository.Deployment
		for _, d := range items {
			if d.CreatedAt.After(cutoff) {
				filtered = append(filtered, d)
			}
		}

		total := len(filtered)
		deployed := 0
		failed := 0
		var totalLeadTime time.Duration
		leadTimeCount := 0
		var totalMTTR time.Duration
		mttrCount := 0

		// Group by project for MTTR calculation
		projectFailures := make(map[string]time.Time)
		for _, d := range filtered {
			switch d.Status {
			case "deployed", "promoted":
				deployed++
				leadTime := d.UpdatedAt.Sub(d.CreatedAt)
				if leadTime > 0 {
					totalLeadTime += leadTime
					leadTimeCount++
				}
			case "failed":
				failed++
				projectFailures[d.GitlabProjectName] = d.CreatedAt
			case "rolled_back":
				// Check if there was a previous failure for MTTR
				if failTime, ok := projectFailures[d.GitlabProjectName]; ok {
					mttr := d.UpdatedAt.Sub(failTime)
					if mttr > 0 {
						totalMTTR += mttr
						mttrCount++
					}
					delete(projectFailures, d.GitlabProjectName)
				}
			}
		}

		// Calculate metrics
		deployFrequency := float64(deployed) / float64(days)
		avgLeadTime := 0.0
		if leadTimeCount > 0 {
			avgLeadTime = totalLeadTime.Seconds() / float64(leadTimeCount)
		}
		failureRate := 0.0
		if total > 0 {
			failureRate = float64(failed) / float64(total) * 100
		}
		avgMTTR := 0.0
		if mttrCount > 0 {
			avgMTTR = totalMTTR.Seconds() / float64(mttrCount)
		}

		c.JSON(http.StatusOK, gin.H{
			"period_days":          days,
			"total_deployments":    total,
			"deployment_frequency": fmt.Sprintf("%.1f/day", deployFrequency),
			"avg_lead_time_hours":  fmt.Sprintf("%.1f", avgLeadTime/3600),
			"change_failure_rate":  fmt.Sprintf("%.1f%%", failureRate),
			"avg_mttr_minutes":     fmt.Sprintf("%.1f", avgMTTR/60),
			"successful":           deployed,
			"failed":               failed,
		})
	}
}

func removeDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
			return
		}
		if err := deps.Repos.Deployment.Delete(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "delete", "deployment", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "deployment deleted"})
	}
}

// publishDeploymentEvent publishes a deployment-related event to the event bus.
func publishDeploymentEvent(deps Dependencies, eventType string, d *repository.Deployment, extra map[string]interface{}) {
	if deps.EventBus == nil || d == nil {
		return
	}
	payload := map[string]interface{}{
		"deployment_id":   d.ID.String(),
		"service_name":    d.GitlabProjectName,
		"environment":     d.TargetNamespace,
		"stage":           d.Stage,
		"team_name":       d.TeamName,
		"image_tag":       d.ImageTag,
		"image_repository": d.ImageRepository,
		"user":            d.CreatedBy,
		"status":          d.Status,
		"url":             "/deployments/" + d.ID.String(),
	}
	for k, v := range extra {
		payload[k] = v
	}
	_ = deps.EventBus.Publish(events.Event{
		Type:     eventType,
		TenantID: d.TenantID.String(),
		EntityID: d.ID.String(),
		Payload:  payload,
	})
}

// dryRunDeployment handles POST /deployments/dry-run — preview what would be deployed
// without creating a record or applying changes.
func dryRunDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			TargetClusterID *uuid.UUID      `json:"target_cluster_id"`
			TargetNamespace string          `json:"target_namespace"`
			GitlabProjectName string        `json:"gitlab_project_name"`
			Replicas        int             `json:"replicas"`
			Spec            json.RawMessage `json:"spec"`
			TimeoutSeconds  int             `json:"timeout_seconds"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		replicas := req.Replicas
		if replicas <= 0 {
			replicas = 1
		}
		spec := req.Spec
		if len(spec) == 0 {
			spec = json.RawMessage("{}")
		}
		releaseName := req.GitlabProjectName
		if releaseName == "" {
			releaseName = "pepa-release"
		}

		var clusterID uuid.UUID
		if req.TargetClusterID != nil {
			clusterID = *req.TargetClusterID
		}

		if deps.Services.Deployment == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment service not available"})
			return
		}

		result, err := deps.Services.Deployment.PerformDryRun(
			c.Request.Context(),
			clusterID,
			req.TargetNamespace,
			releaseName,
			replicas,
			spec,
			req.TimeoutSeconds,
		)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// getDeploymentResources handles GET /deployments/:id/resources — lists Kubernetes
// resources for a deployed application in the target cluster.
func getDeploymentResources(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deployment repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
			return
		}

		d, err := deps.Repos.Deployment.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		if d.TargetClusterID == nil || deps.Repos.Cluster == nil {
			c.JSON(http.StatusOK, gin.H{"resources": []interface{}{}, "message": "no target cluster configured"})
			return
		}

		// Get kubeconfig via the cluster repo
		kubeconfig, err := deps.Repos.Cluster.GetKubeconfig(c.Request.Context(), *d.TargetClusterID, uuid.Nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get kubeconfig: %v", err)})
			return
		}

		// Create k8s client with server override if available
		var k8sClient *k8s.Client
		clusterObj, cerr := deps.Repos.Cluster.Get(c.Request.Context(), *d.TargetClusterID, uuid.Nil)
		if cerr == nil && clusterObj != nil && clusterObj.APIServerURL != "" {
			k8sClient, err = k8s.NewClientWithServerOverride(kubeconfig, clusterObj.APIServerURL)
		} else {
			k8sClient, err = k8s.NewClient(kubeconfig)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create k8s client: %v", err)})
			return
		}

		namespace := d.TargetNamespace
		if namespace == "" {
			namespace = "default"
		}
		resources, err := k8sClient.ListResources(c.Request.Context(), namespace)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list resources: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"resources": resources, "total": len(resources), "namespace": namespace})
	}
}

// getDeploymentTimelineEvents handles GET /deployments/:id/events — returns real
// deployment timeline events from the deployment_events table.
func getDeploymentTimelineEvents(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Deployment == nil {
			c.JSON(http.StatusOK, gin.H{"events": []interface{}{}})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment ID"})
			return
		}

		// Verify deployment exists
		_, err = deps.Repos.Deployment.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Query deployment_events table
		if deps.DB == nil {
			c.JSON(http.StatusOK, gin.H{"events": []interface{}{}})
			return
		}

		type deployEvent struct {
			ID           uuid.UUID  `json:"id"`
			DeploymentID uuid.UUID  `json:"deployment_id"`
			EventType    string     `json:"event_type"`
			Message      string     `json:"message"`
			CreatedAt    time.Time  `json:"created_at"`
		}

		rows, err := deps.DB.Pool.Query(c.Request.Context(), `
			SELECT id, deployment_id, event_type, COALESCE(message,''), created_at
			FROM deployment_events
			WHERE deployment_id = $1
			ORDER BY created_at ASC
		`, id)
		if err != nil {
			// Table might not exist yet — return empty
			slog.Info("Failed to query deployment_events", "error", err)
			c.JSON(http.StatusOK, gin.H{"events": []interface{}{}, "deployment_id": id.String()})
			return
		}
		defer rows.Close()

		var evts []deployEvent
		for rows.Next() {
			var e deployEvent
			if err := rows.Scan(&e.ID, &e.DeploymentID, &e.EventType, &e.Message, &e.CreatedAt); err != nil {
				continue
			}
			evts = append(evts, e)
		}
		if evts == nil {
			evts = make([]deployEvent, 0)
		}
		if err := rows.Err(); err != nil {
			slog.Info("deployment_events rows iteration error", "error", err)
		}

		c.JSON(http.StatusOK, gin.H{"events": evts, "deployment_id": id.String(), "total": len(evts)})
	}
}
