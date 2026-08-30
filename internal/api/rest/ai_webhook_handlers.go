package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/ai"
)

// AIWebhookHandlers handles external IDE integration endpoints.
type AIWebhookHandlers struct {
	aiManager *ai.Manager
}

// NewAIWebhookHandlers creates new AI webhook handlers.
func NewAIWebhookHandlers(aiMgr *ai.Manager) *AIWebhookHandlers {
	return &AIWebhookHandlers{aiManager: aiMgr}
}

// Suggest provides AI suggestions for a file or code context.
func (h *AIWebhookHandlers) Suggest(c *gin.Context) {
	if h.aiManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI manager not initialized"})
		return
	}

	var req struct {
		FilePath    string `json:"file_path"`
		Language    string `json:"language"`
		Code        string `json:"code"`
		Context     string `json:"context"`     // surrounding code or project info
		ProjectType string `json:"project_type"` // e.g., "kubernetes", "go", "python"
		Action      string `json:"action"`       // suggest, explain, fix, review
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Action == "" {
		req.Action = "suggest"
	}

	var prompt string
	switch req.Action {
	case "explain":
		prompt = buildExplainPrompt(req.FilePath, req.Language, req.Code, req.Context)
	case "fix":
		prompt = buildFixPrompt(req.FilePath, req.Language, req.Code, req.Context)
	case "review":
		prompt = buildReviewPrompt(req.FilePath, req.Language, req.Code, req.Context)
	default: // suggest
		prompt = buildSuggestPrompt(req.FilePath, req.Language, req.Code, req.Context, req.ProjectType)
	}

	agent, err := h.aiManager.CreateAgent()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI agent not available"})
		return
	}

	resp, err := agent.Run(c.Request.Context(), nil, prompt)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"suggestion": resp.Answer,
		"action":     req.Action,
		"file_path":  req.FilePath,
		"language":   req.Language,
	})
}

// Status returns AI webhook API status and capabilities.
func (h *AIWebhookHandlers) Status(c *gin.Context) {
	if h.aiManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI manager not initialized"})
		return
	}
	providers := h.aiManager.ListProviders()
	c.JSON(http.StatusOK, gin.H{
		"status":     "active",
		"providers":  providers,
		"actions":    []string{"suggest", "explain", "fix", "review"},
		"endpoint":   "/api/v1/ai/webhook/suggest",
		"ide_plugin": "Available for VS Code and JetBrains via OpenAPI spec",
	})
}

func buildSuggestPrompt(filePath, language, code, context, projectType string) string {
	prompt := "You are an AI coding assistant integrated into a platform engineering system (PEPA).\n\n"
	if filePath != "" {
		prompt += "File: " + filePath + "\n"
	}
	if language != "" {
		prompt += "Language: " + language + "\n"
	}
	if projectType != "" {
		prompt += "Project type: " + projectType + "\n"
	}
	prompt += "\nCode:\n```\n" + code + "\n```\n\n"
	if context != "" {
		prompt += "Context:\n" + context + "\n\n"
	}
	prompt += "Provide suggestions for improving this code. Focus on:\n"
	prompt += "- Platform engineering best practices (Kubernetes, CI/CD, GitOps)\n"
	prompt += "- Security, performance, and maintainability\n"
	prompt += "- PEPA platform integration opportunities\n"
	prompt += "Be concise and actionable."
	return prompt
}

func buildExplainPrompt(filePath, language, code, context string) string {
	prompt := "Explain the following code:\n\n"
	if filePath != "" {
		prompt += "File: " + filePath + "\n"
	}
	prompt += "Language: " + language + "\n\n"
	prompt += "```\n" + code + "\n```\n\n"
	if context != "" {
		prompt += "Context: " + context + "\n\n"
	}
	prompt += "Provide a clear, concise explanation of what this code does and how it works."
	return prompt
}

func buildFixPrompt(filePath, language, code, context string) string {
	prompt := "Review and fix any issues in the following code:\n\n"
	if filePath != "" {
		prompt += "File: " + filePath + "\n"
	}
	prompt += "Language: " + language + "\n\n"
	prompt += "```\n" + code + "\n```\n\n"
	if context != "" {
		prompt += "Context: " + context + "\n\n"
	}
	prompt += "Identify bugs, security issues, and anti-patterns. Provide the fixed code."
	return prompt
}

func buildReviewPrompt(filePath, language, code, context string) string {
	prompt := "Perform a code review on the following:\n\n"
	if filePath != "" {
		prompt += "File: " + filePath + "\n"
	}
	prompt += "Language: " + language + "\n\n"
	prompt += "```\n" + code + "\n```\n\n"
	if context != "" {
		prompt += "Context: " + context + "\n\n"
	}
	prompt += "Review for:\n"
	prompt += "- Correctness and potential bugs\n"
	prompt += "- Security vulnerabilities\n"
	prompt += "- Performance issues\n"
	prompt += "- Code style and best practices\n"
	prompt += "- Platform engineering considerations\n"
	prompt += "Provide specific, actionable feedback."
	return prompt
}
