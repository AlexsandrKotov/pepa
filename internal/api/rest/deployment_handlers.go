package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
)

func registerDeploymentRoutes(r *gin.RouterGroup, deps Dependencies) {
	deployments := r.Group("/deployments")
	{
		deployments.GET("", listDeployments(deps))
		deployments.POST("", createDeployment(deps))
		deployments.GET("/:id", getDeployment(deps))
		deployments.DELETE("/:id", removeDeployment(deps))
		deployments.POST("/:id/promote", promoteDeployment(deps))
		deployments.POST("/:id/rollback", rollbackDeployment(deps))
		deployments.POST("/:id/cancel", cancelDeployment(deps))
		deployments.GET("/:id/history", getDeploymentHistory(deps))
		deployments.GET("/:id/logs", getDeploymentLogs(deps))
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
		c.JSON(http.StatusOK, gin.H{"deployments": items, "total": len(items)})
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

		// If a target cluster is specified, perform the actual deployment
		if d.TargetClusterID != nil && deps.Repos.Cluster != nil {
			go performDeployment(d.ID, *d.TargetClusterID, d.TargetNamespace,
				d.GitlabProjectName, int32(d.Replicas), d.Spec, d.TimeoutSeconds, deps)
		}

		logAudit(deps, c, "create", "deployment", d.ID.String(), nil, gin.H{"deploy_type": d.DeployType, "status": d.Status})
		c.JSON(http.StatusCreated, d)
	}
}

// performDeployment runs the actual Kubernetes deployment in the background.
func performDeployment(deploymentID, clusterID uuid.UUID, namespace, releaseName string, replicas int32, specJSON json.RawMessage, timeoutSeconds int, deps Dependencies) {
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
		log.Printf("Deployment %s failed: %s", deploymentID, result.Message)
	}
}

// updateDeploymentStatus updates the deployment status in the database.
func updateDeploymentStatus(deps Dependencies, id uuid.UUID, status string) {
	updateDeploymentStatusWithError(deps, id, status, "", "")
}

// updateDeploymentStatusWithError updates the deployment status with error message.
func updateDeploymentStatusWithError(deps Dependencies, id uuid.UUID, status string, errMsg string, logs string) {
	updateDeploymentStatusWithLogs(deps, id, status, errMsg, logs)
}

// updateDeploymentStatusWithLogs updates the deployment status with error message and logs.
func updateDeploymentStatusWithLogs(deps Dependencies, id uuid.UUID, status string, errMsg string, logs string) {
	ctx := context.Background()
	d, err := deps.Repos.Deployment.Get(ctx, id)
	if err != nil {
		log.Printf("ERROR: update deployment status: get deployment %s: %v", id, err)
		return
	}
	d.Status = status
	d.ErrorMessage = errMsg
	d.Logs = logs
	if err := deps.Repos.Deployment.Update(ctx, d); err != nil {
		log.Printf("ERROR: update deployment status: update deployment %s: %v", id, err)
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
		rolledBackBy := currentUserLabel(c)
		if err := deps.Repos.Deployment.Rollback(c.Request.Context(), id, rolledBackBy); err != nil {
			if strings.Contains(err.Error(), "cannot be rolled back") {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			respondInternalError(c, err)
			return
		}
		d, _ := deps.Repos.Deployment.Get(c.Request.Context(), id)
		logAudit(deps, c, "rollback", "deployment", id.String(), nil, nil)
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
