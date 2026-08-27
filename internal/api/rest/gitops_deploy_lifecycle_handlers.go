package rest

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
)

// loadTeamStages returns the stage list for a team: saved config first,
// then the built-in defaults.
func loadTeamStages(deps Dependencies, c *gin.Context, tenantID uuid.UUID, team string) []repository.WorkflowStage {
	if team != "" && deps.Repos.TeamWorkflow != nil {
		if cfg, err := deps.Repos.TeamWorkflow.Get(c.Request.Context(), tenantID, team); err == nil && cfg != nil && len(cfg.Stages) > 0 {
			return cfg.Stages
		}
	}
	return defaultTeamWorkflow(team).Stages
}

// stageIndex finds the position of a stage key in the list (-1 if absent).
func stageIndex(stages []repository.WorkflowStage, key string) int {
	for i, s := range stages {
		if s.Key == key {
			return i
		}
	}
	return -1
}

// currentUserLabel returns a best-effort label for audit/promotion attribution.
func currentUserLabel(c *gin.Context) string {
	if uid := auth.GetUserID(c); uid != nil {
		return uid.String()
	}
	return "system"
}

// approveDeployment completes a promotion that was parked in "awaiting_approval".
func approveDeployment(deps Dependencies) gin.HandlerFunc {
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
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
			return
		}
		if d.TenantID != auth.GetTenantID(c) {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
			return
		}
		if d.Status != "awaiting_approval" {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("deployment is not awaiting approval (status %q)", d.Status)})
			return
		}

		stages := loadTeamStages(deps, c, d.TenantID, d.TeamName)
		idx := stageIndex(stages, d.Stage)
		if idx < 0 || idx >= len(stages)-1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no next stage available for approval"})
			return
		}

		completePromotion(deps, c, d, stages[idx+1])
	}
}

// completePromotion marks the source deployment as promoted and creates the
// deployment in the next stage, then starts its simulated lifecycle.
func completePromotion(deps Dependencies, c *gin.Context, d *repository.Deployment, next repository.WorkflowStage) {
	ctx := c.Request.Context()
	user := currentUserLabel(c)

	if err := deps.Repos.Deployment.Promote(ctx, d.ID, user); err != nil {
		respondInternalError(c, err)
		return
	}

	promoted := &repository.Deployment{
		TenantID:          d.TenantID,
		JiraIssueKey:      d.JiraIssueKey,
		JiraSummary:       d.JiraSummary,
		GitlabProjectID:   d.GitlabProjectID,
		GitlabProjectName: d.GitlabProjectName,
		GitlabMRID:        d.GitlabMRID,
		GitlabMRURL:       d.GitlabMRURL,
		TargetClusterID:   d.TargetClusterID,
		TargetNamespace:   "app-" + next.Key,
		ImageTag:          d.ImageTag,
		ImageRepository:   d.ImageRepository,
		TeamName:          d.TeamName,
		Stage:             next.Key,
		Status:            "pending",
		CreatedBy:         user,
	}
	if err := deps.Repos.Deployment.Create(ctx, promoted); err != nil {
		respondInternalError(c, err)
		return
	}
	simulateDeployLifecycle(deps, promoted.ID)

	logAudit(deps, c, "promote", "deployment", d.ID.String(), nil, gin.H{
		"from_stage": d.Stage, "to_stage": next.Key, "new_deployment": promoted.ID.String(),
	})

	c.JSON(http.StatusOK, gin.H{
		"deployment":        d,
		"new_deployment":    promoted,
		"promoted_to":       next.Key,
		"awaiting_approval": false,
		"message":           fmt.Sprintf("Promoted to %q", next.Label),
	})
}
