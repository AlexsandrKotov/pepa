package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
)

// listTeamWorkflows returns all team workflow configs for the tenant.
func listTeamWorkflows(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.TeamWorkflow == nil {
			c.JSON(http.StatusOK, gin.H{"workflows": []interface{}{}, "total": 0})
			return
		}
		tenantID := auth.GetTenantID(c)

		configs, err := deps.Repos.TeamWorkflow.List(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if configs == nil {
			configs = []repository.TeamWorkflowConfig{}
		}

		c.JSON(http.StatusOK, gin.H{
			"workflows": configs,
			"total":     len(configs),
		})
	}
}

// getTeamWorkflow returns a team's workflow config (or a default).
func getTeamWorkflow(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		team := c.Param("team")
		if deps.Repos.TeamWorkflow == nil {
			c.JSON(http.StatusOK, defaultTeamWorkflow(team))
			return
		}
		tenantID := auth.GetTenantID(c)

		cfg, err := deps.Repos.TeamWorkflow.Get(c.Request.Context(), tenantID, team)
		if err != nil || cfg == nil || len(cfg.Stages) == 0 {
			// No saved config (or an empty one) — fall back to defaults.
			c.JSON(http.StatusOK, defaultTeamWorkflow(team))
			return
		}

		c.JSON(http.StatusOK, cfg)
	}
}

// saveTeamWorkflow creates or updates a team workflow config.
func saveTeamWorkflow(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.TeamWorkflow == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "team workflow repository not available"})
			return
		}
		team := c.Param("team")
		tenantID := auth.GetTenantID(c)

		var req repository.TeamWorkflowConfig
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		req.TeamName = team
		if len(req.Stages) == 0 {
			req.Stages = defaultTeamWorkflow(team).Stages
		}

		if err := deps.Repos.TeamWorkflow.Upsert(c.Request.Context(), tenantID, &req); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "team_workflow", team, nil, &req)

		c.JSON(http.StatusOK, gin.H{
			"message": "Team workflow saved",
			"team":    team,
			"config":  &req,
		})
	}
}

// deleteTeamWorkflow removes a team workflow config.
func deleteTeamWorkflow(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.TeamWorkflow == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "team workflow repository not available"})
			return
		}
		team := c.Param("team")
		tenantID := auth.GetTenantID(c)

		if err := deps.Repos.TeamWorkflow.Delete(c.Request.Context(), tenantID, team); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "team_workflow", team, nil, nil)

		c.JSON(http.StatusOK, gin.H{
			"message": "Team workflow deleted",
			"team":    team,
		})
	}
}

// defaultTeamWorkflow returns a sensible default workflow for a team.
func defaultTeamWorkflow(team string) *repository.TeamWorkflowConfig {
	return &repository.TeamWorkflowConfig{
		TeamName: team,
		Stages: []repository.WorkflowStage{
			{Key: "dev", Label: "Development", Color: "bg-green-500", AutoPromote: true, Approval: false},
			{Key: "testing", Label: "Testing", Color: "bg-blue-500", AutoPromote: false, Approval: false},
			{Key: "staging", Label: "Staging", Color: "bg-yellow-500", AutoPromote: false, Approval: true},
			{Key: "production", Label: "Production", Color: "bg-red-500", AutoPromote: false, Approval: true},
		},
		GitOps: repository.GitOpsConfig{
			Provider: "argocd",
			RepoURL:  "https://git.example.com/" + team + "/gitops",
			Branch:   "main",
			Path:     "manifests/",
		},
		CI: repository.CIConfig{
			Provider: "gitlab",
			Pipeline: ".gitlab-ci.yml",
		},
		Verification: repository.VerifyConfig{
			Checks: []string{"health-check", "smoke-test"},
		},
	}
}
