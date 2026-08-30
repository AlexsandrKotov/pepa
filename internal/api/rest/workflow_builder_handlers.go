package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/ai"
)

// WorkflowBuilderHandlers handles NL workflow builder endpoints.
type WorkflowBuilderHandlers struct {
	builder *ai.WorkflowBuilder
}

// NewWorkflowBuilderHandlers creates new workflow builder handlers.
func NewWorkflowBuilderHandlers(builder *ai.WorkflowBuilder) *WorkflowBuilderHandlers {
	return &WorkflowBuilderHandlers{builder: builder}
}

// BuildWorkflow generates a workflow from natural language.
func (h *WorkflowBuilderHandlers) BuildWorkflow(c *gin.Context) {
	if h.builder == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Workflow builder not initialized"})
		return
	}

	var req struct {
		Description string `json:"description" binding:"required"`
		Environment string `json:"environment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Environment == "" {
		req.Environment = "default"
	}

	workflow, err := h.builder.BuildWorkflow(c.Request.Context(), req.Description, req.Environment)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"workflow": workflow,
		"yaml":     workflow.ToYAML(),
	})
}

// PreviewWorkflow returns a YAML preview of an existing workflow definition.
func (h *WorkflowBuilderHandlers) PreviewWorkflow(c *gin.Context) {
	var wf ai.WorkflowDefinition
	if err := c.ShouldBindJSON(&wf); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"yaml":     wf.ToYAML(),
		"workflow": wf,
	})
}
