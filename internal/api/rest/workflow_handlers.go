package rest

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/events"
	"github.com/pepa/pepa/pkg/models"
)

func registerWorkflowRoutes(r *gin.RouterGroup, deps Dependencies) {
	workflows := r.Group("/workflows")
	{
		workflows.GET("", listWorkflows(deps))
		workflows.POST("", createWorkflow(deps))
		workflows.GET("/:id", getWorkflow(deps))
		workflows.PUT("/:id", updateWorkflow(deps))
		workflows.DELETE("/:id", deleteWorkflow(deps))
		workflows.POST("/:id/execute", executeWorkflow(deps))
		workflows.GET("/:id/executions", listExecutions(deps))
	}
}

func listWorkflows(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

		workflows, total, err := deps.Repos.Workflow.List(c.Request.Context(), tenantID, page, perPage)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if workflows == nil {
			workflows = []models.Workflow{}
		}

		c.JSON(http.StatusOK, gin.H{
			"workflows": workflows,
			"total":     total,
			"page":      page,
			"per_page":  perPage,
		})
	}
}

func createWorkflow(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		userID := auth.GetUserID(c)

		var w models.Workflow
		if err := c.ShouldBindJSON(&w); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		w.TenantID = tenantID
		w.CreatedBy = userID
		w.IsEnabled = true

		if w.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
			return
		}

		if err := deps.Repos.Workflow.Create(c.Request.Context(), &w); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "create", "workflow", w.ID.String(), nil, gin.H{"name": w.Name})
		c.JSON(http.StatusCreated, w)
	}
}

func getWorkflow(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workflow ID"})
			return
		}

		w, err := deps.Repos.Workflow.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, w)
	}
}

func updateWorkflow(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workflow ID"})
			return
		}

		var w models.Workflow
		if err := c.ShouldBindJSON(&w); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := deps.Repos.Workflow.Update(c.Request.Context(), id, &w); err != nil {
			respondInternalError(c, err)
			return
		}

		updated, _ := deps.Repos.Workflow.Get(c.Request.Context(), id)
		logAudit(deps, c, "update", "workflow", id.String(), nil, gin.H{"name": updated.Name})
		c.JSON(http.StatusOK, updated)
	}
}

func deleteWorkflow(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workflow ID"})
			return
		}

		if err := deps.Repos.Workflow.Delete(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "workflow", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "workflow deleted", "id": id})
	}
}

func executeWorkflow(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workflow ID"})
			return
		}

		tenantID := auth.GetTenantID(c)

		var req models.ExecuteWorkflowRequest
		if bindErr := c.ShouldBindJSON(&req); bindErr != nil {
			// Body is optional — only reject if it's malformed JSON, not just empty.
			if bindErr.Error() != "EOF" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + bindErr.Error()})
				return
			}
		}

		paramsJSON, _ := json.Marshal(req.Parameters)

		exec := &models.WorkflowExecution{
			WorkflowID:     id,
			TenantID:       tenantID,
			TriggerType:    "manual",
			TriggerPayload: paramsJSON,
			TriggeredBy:    auth.GetUserID(c),
		}

		if err := deps.Repos.Workflow.CreateExecution(c.Request.Context(), exec); err != nil {
			respondInternalError(c, err)
			return
		}

		// Enqueue job for worker
		if deps.JobQueue != nil {
			_ = deps.JobQueue.Enqueue("workflow.execute", tenantID.String(), map[string]interface{}{
				"workflow_id":  id.String(),
				"execution_id": exec.ID.String(),
			})
		} else {
			slog.Info("JobQueue is nil, workflow execution will not be processed", "id", id, "id", exec.ID)
		}

		// Emit event
		if deps.EventBus != nil {
			_ = deps.EventBus.Publish(events.Event{
				Type:     "workflow.executing",
				TenantID: tenantID.String(),
				Payload: map[string]interface{}{
					"workflow_id":  id.String(),
					"execution_id": exec.ID.String(),
				},
			})
		}

		logAudit(deps, c, "execute", "workflow", id.String(), nil, exec)

		c.JSON(http.StatusAccepted, exec)
	}
}

func listExecutions(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workflow ID"})
			return
		}

		execs, err := deps.Repos.Workflow.ListExecutions(c.Request.Context(), id)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"executions": execs})
	}
}
