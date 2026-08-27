package rest

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/ai"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/gitops"
	"github.com/pepa/pepa/pkg/models"
	"gopkg.in/yaml.v3"
)

// AIHandlers handles AI chat endpoints
type AIHandlers struct {
	aiManager *ai.Manager
	deps      Dependencies
	mu        sync.RWMutex
	// history per user — keyed by user ID string
	history map[string][]chatHistoryEntry
}

type chatHistoryEntry struct {
	Type      string                 `json:"type"`
	Query     string                 `json:"query"`
	Response  string                 `json:"response"`
	ToolCalls []ai.AgentActionResult `json:"tool_calls,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	ModelUsed string                 `json:"model_used,omitempty"`
	TokensIn  int                    `json:"tokens_input,omitempty"`
	TokensOut int                    `json:"tokens_output,omitempty"`
}

const maxHistoryPerUser = 200

// NewAIHandlers creates new AI handlers
func NewAIHandlers(aiMgr *ai.Manager, deps Dependencies) *AIHandlers {
	return &AIHandlers{
		aiManager: aiMgr,
		deps:      deps,
		history:   make(map[string][]chatHistoryEntry),
	}
}

// userIDStr returns the user ID from context as a string, or "anonymous".
func userIDStr(c *gin.Context) string {
	uid := auth.GetUserID(c)
	if uid != nil {
		return uid.String()
	}
	return "anonymous"
}

// appendHistory adds an entry to the per-user history, enforcing limits.
func (h *AIHandlers) appendHistory(uid string, entry chatHistoryEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.history[uid] = append(h.history[uid], entry)
	if len(h.history[uid]) > maxHistoryPerUser {
		h.history[uid] = h.history[uid][len(h.history[uid])-maxHistoryPerUser:]
	}
}

// simpleChatSystemPrompt is used when tools/agent mode is disabled.
// It must explicitly prevent hallucination since the model has no live data access.
const simpleChatSystemPrompt = `You are the PEPA AI Assistant — a helpful conversational AI built into the PEPA platform engineering portal.

ABOUT PEPA — you are part of this application. PEPA is a self-hosted platform engineering portal that provides:
- Service catalog (registered microservices and applications)
- Docker service management (containers on Docker hosts)
- Kubernetes cluster management and deployments
- CI/CD pipeline tracking (GitLab CI, GitHub Actions, Gitea Actions)
- GitOps engine (FluxCD, ArgoCD)
- Automation workflows and pipeline builder
- RBAC with role-based dashboards
- Vault integration for secret management
- Plugin system (Slack, Telegram, Jira, Prometheus, Proxmox, etc.)
- AI-powered configuration generation (Helm, K8s manifests, Terraform, etc.)

WHAT YOU CAN DO:
- Explain PEPA features and guide users through the UI
- Answer DevOps, Kubernetes, Docker, CI/CD, GitOps, Helm, Terraform questions
- Generate configurations (Helm values, K8s manifests, docker-compose, Terraform, etc.)
- Share best practices for platform engineering

WHAT YOU CANNOT DO (in chat mode):
- Access live data from PEPA databases or APIs
- List services, deployments, clusters, etc.
For live data, the user needs to enable Agent mode (toggle in the toolbar).

RULES:
- NEVER fabricate service names, statuses, API endpoints, or CLI commands.
- If asked about live data, say: "Enable Agent mode to query real data from PEPA."
- Be concise and structured. Use bullet points and code blocks.
- Respond in the SAME LANGUAGE the user used.`

// ChatRequest
type ChatRequest struct {
	Message        string `json:"message"`
	ConversationID string `json:"conversation_id,omitempty"`
	Model          string `json:"model,omitempty"`
	EnableTools    bool   `json:"enable_tools"`
	// AgentMode allows overriding the auto-detected agent mode.
	// "native" = use LLM function calling, "prompt" = use prompt-based intent classification.
	// Empty = auto-detect based on model capabilities.
	AgentMode string `json:"agent_mode,omitempty"`
	// SystemInstruction overrides the default system prompt with a custom
	// instruction. Example: "Ты — умный ИИ-агент. Твоя задача — помогать пользователю."
	SystemInstruction string `json:"system_instruction,omitempty"`
	// History is the conversation history for multi-turn context.
	History []ai.Message `json:"history,omitempty"`
	// Provider selects which LLM provider to use. Empty = default provider.
	Provider string `json:"provider,omitempty"`
}

// Chat handles the AI chat endpoint.
// When enable_tools is true, it uses the Agent (ReAct loop with tools).
// When enable_tools is false, it uses simple chat without tools.
func (h *AIHandlers) Chat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Get the provider (selected or default)
	providerName := req.Provider
	provider, err := h.aiManager.DefaultProvider()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "AI provider not configured",
			"message": "No LLM provider is configured. Please add an AI Provider connection in Connections.",
		})
		return
	}
	if providerName != "" {
		provider, err = h.aiManager.GetProvider(providerName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Provider %q not found", providerName)})
			return
		}
	}

	var response string
	var modelUsed string
	var tokensUsed ai.TokenUsage
	var toolCalls []ai.AgentActionResult

	if req.EnableTools {
		// Agent mode: use ReAct loop with tools
		var agent *ai.Agent
		var err error

		// Allow frontend to override agent mode and provider.
		// SystemInstruction is passed via the task, not the agent, to avoid double-setting.
		switch req.AgentMode {
		case "native":
			agent, err = h.aiManager.CreateAgentWithProviderAndMode(req.Provider, ai.AgentModeNative)
		case "prompt":
			agent, err = h.aiManager.CreateAgentWithProviderAndMode(req.Provider, ai.AgentModePrompt)
		default:
			// Auto-detect based on model capabilities
			agent, err = h.aiManager.CreateAgentForProvider(req.Provider)
			if err == nil && req.SystemInstruction != "" {
				agent.SetSystemInstruction(req.SystemInstruction)
			}
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "AI provider not configured",
				"message": "No LLM provider is configured. Please add an AI Provider connection in Connections.",
			})
			return
		}

		task := &ai.AgentTask{
			Description:       req.Message,
			History:           req.History,
			SystemInstruction: req.SystemInstruction,
		}

		resp, err := agent.Run(c.Request.Context(), task, req.Message)
		if err != nil {
			log.Printf("[AI Chat] Agent error: %v", err)
			respondInternalError(c, err)
			return
		}

		response = resp.Answer
		modelUsed = resp.ModelUsed
		tokensUsed = resp.TokensUsed
		toolCalls = resp.ToolCalls
	} else {
		// Simple chat mode: no tools
		sysContent := simpleChatSystemPrompt
		if req.SystemInstruction != "" {
			sysContent = req.SystemInstruction
		}
		messages := []ai.Message{
			{Role: "system", Content: sysContent},
		}
		if len(req.History) > 0 {
			// Limit to last 10 messages to save tokens
			hist := req.History
			if len(hist) > 10 {
				hist = hist[len(hist)-10:]
			}
			for _, h := range hist {
				if h.Role == "user" || h.Role == "assistant" {
					messages = append(messages, h)
				}
			}
		}
		messages = append(messages, ai.Message{Role: "user", Content: req.Message})

		resp, err := provider.Chat(c.Request.Context(), messages, &ai.ChatOptions{
			Model:     req.Model,
			MaxTokens: 4096,
		})
		if err != nil {
			log.Printf("[AI Chat] Provider error: %v", err)
			respondInternalError(c, err)
			return
		}

		response = ai.StripThinkBlocks(resp.Content)
		modelUsed = resp.ModelUsed
		tokensUsed = resp.TokensUsed
	}

	// Store in per-user history
	uid := userIDStr(c)
	h.appendHistory(uid, chatHistoryEntry{
		Type:      "chat",
		Query:     req.Message,
		Response:  response,
		ToolCalls: toolCalls,
		Timestamp: time.Now(),
		ModelUsed: modelUsed,
		TokensIn:  tokensUsed.InputTokens,
		TokensOut: tokensUsed.OutputTokens,
	})

	// Audit log
	logAudit(h.deps, c, "ai_chat", "ai_message", uuid.NewString(), nil, map[string]interface{}{
		"type":       "chat",
		"query":      req.Message,
		"model":      modelUsed,
		"tokens_in":  tokensUsed.InputTokens,
		"tokens_out": tokensUsed.OutputTokens,
		"tools_used": len(toolCalls),
	})

	result := gin.H{
		"response":    response,
		"model":       modelUsed,
		"tokens_used": tokensUsed,
		"tool_calls":  toolCalls,
		"tools_used":  len(toolCalls),
	}

	c.JSON(http.StatusOK, result)
}

// ChatStream handles the AI chat endpoint (streaming SSE)
func (h *AIHandlers) ChatStream(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	if req.EnableTools {
		// Agent streaming: classify + execute tools + stream synthesis
		var agent *ai.Agent
		var err error

		switch req.AgentMode {
		case "native":
			agent, err = h.aiManager.CreateAgentWithProviderAndMode(req.Provider, ai.AgentModeNative)
		case "prompt":
			agent, err = h.aiManager.CreateAgentWithProviderAndMode(req.Provider, ai.AgentModePrompt)
		default:
			agent, err = h.aiManager.CreateAgentForProvider(req.Provider)
			if err == nil && req.SystemInstruction != "" {
				agent.SetSystemInstruction(req.SystemInstruction)
			}
		}
		if err != nil {
			_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", mustJSON(gin.H{"type": "error", "error": err.Error()}))
			flusher.Flush()
			return
		}

		// Build the task for agent streaming
		task := &ai.AgentTask{
			Description:       req.Message,
			History:           req.History,
			SystemInstruction: req.SystemInstruction,
		}

		// Stream from the agent (prompt mode uses RunStream, native mode falls back to blocking run)
		stream, sErr := agent.Stream(c.Request.Context(), task, req.Message)
		if sErr != nil {
			_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", mustJSON(gin.H{"type": "error", "error": sErr.Error()}))
			flusher.Flush()
			return
		}

		for chunk := range stream {
			// Filter out model reasoning blocks from agent streaming
			if chunk.Type == "text" && chunk.Content != "" {
				chunk.Content = ai.StripThinkBlocks(chunk.Content)
				if chunk.Content == "" {
					continue
				}
			}
			_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", mustJSON(chunk))
			flusher.Flush()
		}
		return
	}

	// Simple streaming chat (no tools)
	var provider ai.LLMProvider
	var err error
	if req.Provider != "" {
		provider, err = h.aiManager.GetProvider(req.Provider)
	} else {
		provider, err = h.aiManager.DefaultProvider()
	}
	if err != nil {
		fmt.Fprintf(c.Writer, "data: %s\n\n", mustJSON(gin.H{"type": "error", "error": "AI provider not configured"}))
		flusher.Flush()
		return
	}

	messages := []ai.Message{
		{
			Role:    "system",
			Content: simpleChatSystemPrompt,
		},
	}
	if req.SystemInstruction != "" {
		messages[0].Content = req.SystemInstruction
	}
	if len(req.History) > 0 {
		// Limit to last 10 messages to save tokens
		hist := req.History
		if len(hist) > 10 {
			hist = hist[len(hist)-10:]
		}
		for _, h := range hist {
			if h.Role == "user" || h.Role == "assistant" {
				messages = append(messages, h)
			}
		}
	}
	messages = append(messages, ai.Message{Role: "user", Content: req.Message})

	opts := &ai.ChatOptions{
		Model:     req.Model,
		MaxTokens: 4096,
	}

	stream, err := provider.Stream(c.Request.Context(), messages, opts)
	if err != nil {
		fmt.Fprintf(c.Writer, "data: %s\n\n", mustJSON(gin.H{"type": "error", "error": err.Error()}))
		flusher.Flush()
		return
	}

	for chunk := range stream {
		// Filter out model reasoning blocks from streaming output
		if chunk.Type == "text" && chunk.Content != "" {
			chunk.Content = ai.StripThinkBlocks(chunk.Content)
			if chunk.Content == "" {
				continue
			}
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", mustJSON(chunk))
		flusher.Flush()
	}
}

// AIStatus returns AI provider status
func (h *AIHandlers) AIStatus(c *gin.Context) {
	providers := h.aiManager.ListProviders()
	health := h.aiManager.HealthCheck(c.Request.Context())

	status := make([]gin.H, 0, len(providers))
	for _, name := range providers {
		err := health[name]
		s := gin.H{
			"name":      name,
			"available": err == nil,
		}
		if err != nil {
			s["error"] = err.Error()
		}
		status = append(status, s)
	}

	c.JSON(http.StatusOK, gin.H{
		"providers":        status,
		"default_provider": h.aiManager.DefaultProviderName(),
		"tools":            len(h.aiManager.ToolRegistry().List()),
		"enabled":          len(providers) > 0,
		"policy":           h.aiManager.AgentPolicy().ListLevels(),
	})
}

// SetDefaultProvider sets the default AI provider.
func (h *AIHandlers) SetDefaultProvider(c *gin.Context) {
	var req struct {
		Provider string `json:"provider" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if h.aiManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI manager not available"})
		return
	}
	providers := h.aiManager.ListProviders()
	found := false
	for _, p := range providers {
		if p == req.Provider {
			found = true
			break
		}
	}
	if !found {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider not found"})
		return
	}
	h.aiManager.SetDefaultProvider(req.Provider)
	logAudit(h.deps, c, "update", "setting", "ai_default_provider", nil, gin.H{"provider": req.Provider})
	c.JSON(http.StatusOK, gin.H{"default_provider": req.Provider, "message": "Default provider updated"})
}

// ListTools returns available AI tools with their policy levels
func (h *AIHandlers) ListTools(c *gin.Context) {
	tools := h.aiManager.ToolRegistry().List()
	policyLevels := h.aiManager.AgentPolicy().ListLevels()

	type toolWithPolicy struct {
		ai.ToolDefinition
		Policy string `json:"policy"`
	}

	result := make([]toolWithPolicy, 0, len(tools))
	for _, t := range tools {
		policy := "observe"
		if level, ok := policyLevels[t.Name]; ok {
			policy = string(level)
		}
		result = append(result, toolWithPolicy{ToolDefinition: t, Policy: policy})
	}

	c.JSON(http.StatusOK, gin.H{
		"tools": result,
		"total": len(result),
	})
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// History returns chat interaction history for the current user
func (h *AIHandlers) History(c *gin.Context) {
	uid := userIDStr(c)
	h.mu.RLock()
	defer h.mu.RUnlock()
	entries := h.history[uid]
	items := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		item := gin.H{
			"type":      e.Type,
			"query":     e.Query,
			"response":  e.Response,
			"timestamp": e.Timestamp,
		}
		if e.ModelUsed != "" {
			item["model_used"] = e.ModelUsed
		}
		if e.TokensIn > 0 || e.TokensOut > 0 {
			item["tokens_input"] = e.TokensIn
			item["tokens_output"] = e.TokensOut
		}
		if len(e.ToolCalls) > 0 {
			item["tool_calls"] = e.ToolCalls
			item["tools_used"] = len(e.ToolCalls)
		}
		items = append(items, item)
	}
	// Return most recent first
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	c.JSON(http.StatusOK, gin.H{"history": items, "total": len(items)})
}

// Suggestions returns AI usage suggestions
func (h *AIHandlers) Suggestions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"suggestions": []gin.H{
			{"title": "List all services", "description": "Show all registered services with their status"},
			{"title": "Check deployments", "description": "Show recent deployments and their status"},
			{"title": "Cluster health", "description": "List connected clusters and their status"},
			{"title": "Pipeline status", "description": "Show pipeline sources and their recent runs"},
			{"title": "Active workflows", "description": "List automation workflows"},
			{"title": "Environments", "description": "Show all configured environments"},
			{"title": "Что такое Kubernetes?", "description": "Объяснение архитектуры и основных концепций"},
			{"title": "Лучшие практики CI/CD", "description": "Рекомендации по настройке пайплайнов"},
			{"title": "Почему поды перезапускаются?", "description": "Диагностика CrashLoopBackOff и OOMKilled"},
			{"title": "How to scale microservices?", "description": "Best practices for horizontal and vertical scaling"},
		},
		"total": 10,
	})
}

// Generate uses the LLM to generate configuration files
func (h *AIHandlers) Generate(c *gin.Context) {
	var req struct {
		Type   string            `json:"type"`
		Name   string            `json:"name"`
		Params map[string]string `json:"params,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	provider, err := h.aiManager.DefaultProvider()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI provider not configured"})
		return
	}

	// Generate PEPA-specific prompts based on type
	prompt := h.getGeneratePrompt(req.Type, req.Name, req.Params)

	messages := []ai.Message{
		{Role: "system", Content: "You are a PEPA platform configuration generator. Return only valid YAML/JSON configurations that can be directly imported into PEPA."},
		{Role: "user", Content: prompt},
	}

	resp, err := provider.Chat(c.Request.Context(), messages, &ai.ChatOptions{MaxTokens: 4096})
	if err != nil {
		respondInternalError(c, err)
		return
	}

	h.appendHistory(userIDStr(c), chatHistoryEntry{Type: "generate", Query: fmt.Sprintf("%s: %s", req.Type, req.Name), Response: resp.Content, Timestamp: time.Now(), ModelUsed: resp.ModelUsed, TokensIn: resp.TokensUsed.InputTokens, TokensOut: resp.TokensUsed.OutputTokens})

	logAudit(h.deps, c, "ai_generate", "ai_message", uuid.NewString(), nil, map[string]interface{}{
		"type":  req.Type,
		"name":  req.Name,
		"model": resp.ModelUsed,
	})

	c.JSON(http.StatusOK, gin.H{"type": req.Type, "name": req.Name, "content": resp.Content, "format": "yaml"})
}

// getGeneratePrompt returns a specific prompt based on the configuration type
func (h *AIHandlers) getGeneratePrompt(configType, name string, params map[string]string) string {
	switch configType {
	// PEPA Platform configurations
	case "pepa_service_blueprint":
		return fmt.Sprintf(`Generate a PEPA Service Blueprint YAML for '%s'.
Return a flat YAML mapping (no nested "spec" or "service" wrapper) with these exact top-level keys:
- name: "%s"
- description: short description
- category: one of backend, frontend, data, infrastructure
- source_type: one of container, helm_oci
- image: container image (e.g. nginx:1.25-alpine) when source_type is container
- chart_name: helm chart name when source_type is helm_oci
- chart_version: chart version
- namespace: default
- cpu: "100m"
- memory: "128Mi"
- replicas: 1
- ports: [8080]
- default_values: key-value map of environment variables
- tags: [web-server, http]
Do NOT wrap in markdown code fences. Return only raw YAML.`, name, name)

	case "pepa_workflow":
		return fmt.Sprintf(`Generate a PEPA Automation Workflow YAML for '%s'.
Return a flat YAML mapping with these exact top-level keys:
- name: "%s"
- description: short description
- steps: array of step objects, each with:
  - name: step name
  - plugin: plugin name (e.g. "builtin:shell")
  - action: action name (e.g. "run")
  - params: object with step-specific parameters
Example step: {name: "Build", plugin: "builtin:shell", action: "run", params: {script: "echo building"}}
Do NOT wrap in markdown code fences. Return only raw YAML.`, name, name)

	case "pepa_scorecard":
		return fmt.Sprintf(`Generate a PEPA Scorecard YAML for evaluating '%s'.
Return a flat YAML mapping with these exact top-level keys:
- name: "%s"
- description: short description
- enabled: true
- rules: array of rule objects, each with:
  - name: rule name
  - description: what it checks
  - expression: CEL-like expression (e.g. "entity.labels.has('owner')")
  - weight: integer 1-10
  - severity: one of critical, warning, info
  - pass_message: message when rule passes
  - fail_message: message when rule fails
Do NOT wrap in markdown code fences. Return only raw YAML.`, name, name)

	case "pepa_pipeline_source":
		return fmt.Sprintf(`Generate a PEPA Pipeline Source configuration YAML for '%s'.
Return a flat YAML mapping with these exact top-level keys:
- name: "%s"
- description: short description
- source_type: one of gitlab_ci, ansible, terraform
- config: object with source-specific settings, e.g.:
  - for gitlab_ci: {project_id: "123", ref: "main"}
  - for ansible: {playbook: "deploy.yml", inventory: "hosts"}
  - for terraform: {module_path: ".", workspace: "default"}
Do NOT wrap in markdown code fences. Return only raw YAML.`, name, name)

	case "pepa_rbac_policy":
		return fmt.Sprintf(`Generate a PEPA RBAC Policy YAML for '%s'.
Return a flat YAML mapping with these exact top-level keys:
- name: "%s"
- slug: url-friendly slug (e.g. "%s")
- description: short description
- scope: one of global, tenant
- permissions: array of permission objects, each with:
  - resource: resource type (services, deployments, pipelines, workflows, blueprints, scorecards)
  - action: one of read, write, delete, execute
  - effect: "allow"
Do NOT wrap in markdown code fences. Return only raw YAML.`, name, name, strings.ToLower(strings.ReplaceAll(name, " ", "-")))

	case "pepa_gitops":
		return fmt.Sprintf(`Generate a PEPA GitOps Repository configuration YAML for '%s'.
Return a flat YAML mapping with these exact top-level keys:
- name: "%s"
- repo_url: Git repository URL (e.g. "https://github.com/org/repo.git")
- branch: "main"
- path: "."
- engine_type: one of fluxcd, argocd, auto
Do NOT wrap in markdown code fences. Return only raw YAML.`, name, name)

	default:
		return fmt.Sprintf("Generate a %s configuration for a service called '%s'. Return only the YAML content, no explanations.", configType, name)
	}
}

// Analyze uses the LLM to analyze a target
func (h *AIHandlers) Analyze(c *gin.Context) {
	var req struct {
		Target  string `json:"target"`
		Context string `json:"context,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	provider, err := h.aiManager.DefaultProvider()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI provider not configured"})
		return
	}

	prompt := fmt.Sprintf("Analyze the following for potential issues and provide recommendations: %s", req.Target)
	messages := []ai.Message{
		{Role: "system", Content: "You are a platform engineering analyst. Provide structured analysis with severity levels."},
		{Role: "user", Content: prompt},
	}

	resp, err := provider.Chat(c.Request.Context(), messages, &ai.ChatOptions{MaxTokens: 4096})
	if err != nil {
		respondInternalError(c, err)
		return
	}

	h.appendHistory(userIDStr(c), chatHistoryEntry{Type: "analyze", Query: req.Target, Response: resp.Content, Timestamp: time.Now(), ModelUsed: resp.ModelUsed, TokensIn: resp.TokensUsed.InputTokens, TokensOut: resp.TokensUsed.OutputTokens})

	logAudit(h.deps, c, "ai_analyze", "ai_message", uuid.NewString(), nil, map[string]interface{}{
		"target": req.Target,
		"model":  resp.ModelUsed,
	})

	c.JSON(http.StatusOK, gin.H{"summary": resp.Content, "issues": []gin.H{}, "target": req.Target})
}

// Recommend uses the LLM to provide optimization recommendations
func (h *AIHandlers) Recommend(c *gin.Context) {
	var req struct {
		Category string `json:"category,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	provider, err := h.aiManager.DefaultProvider()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI provider not configured"})
		return
	}

	category := req.Category
	if category == "" {
		category = "general platform engineering"
	}
	prompt := fmt.Sprintf("Provide 5 actionable optimization recommendations for %s. For each, include title, description, and impact level (high/medium/low).", category)
	messages := []ai.Message{
		{Role: "system", Content: "You are a platform optimization advisor. Return recommendations as a numbered list with title, description, and impact."},
		{Role: "user", Content: prompt},
	}

	resp, err := provider.Chat(c.Request.Context(), messages, &ai.ChatOptions{MaxTokens: 4096})
	if err != nil {
		respondInternalError(c, err)
		return
	}

	h.appendHistory(userIDStr(c), chatHistoryEntry{Type: "recommend", Query: category, Response: resp.Content, Timestamp: time.Now(), ModelUsed: resp.ModelUsed, TokensIn: resp.TokensUsed.InputTokens, TokensOut: resp.TokensUsed.OutputTokens})

	logAudit(h.deps, c, "ai_recommend", "ai_message", uuid.NewString(), nil, map[string]interface{}{
		"category": category,
		"model":    resp.ModelUsed,
	})

	c.JSON(http.StatusOK, gin.H{
		"recommendations": []gin.H{
			{"title": "Review resource requests", "description": resp.Content, "impact": "high"},
		},
		"total": 1,
	})
}

// Apply parses AI-generated YAML and creates the corresponding PEPA resource.
func (h *AIHandlers) Apply(c *gin.Context) {
	var req struct {
		Type    string `json:"type" binding:"required"`
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type and content are required"})
		return
	}

	// Strip markdown code fences that LLMs often wrap around output
	req.Content = stripCodeFences(req.Content)

	switch req.Type {
	case "pepa_service_blueprint":
		h.applyBlueprint(c, req.Content)
	case "pepa_workflow":
		h.applyWorkflow(c, req.Content)
	case "pepa_scorecard":
		h.applyScorecard(c, req.Content)
	case "pepa_pipeline_source":
		h.applyPipelineSource(c, req.Content)
	case "pepa_rbac_policy":
		h.applyRBACPolicy(c, req.Content)
	case "pepa_gitops":
		h.applyGitOps(c, req.Content)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unsupported apply type: %s", req.Type)})
	}
}

// applyBlueprint parses YAML content and creates a service blueprint.
func (h *AIHandlers) applyBlueprint(c *gin.Context, content string) {
	// Parse the YAML into a generic map first
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid YAML: %v", err)})
		return
	}

	userID := auth.GetUserID(c)

	// Extract fields with sensible defaults
	name := strVal(raw, "name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "YAML must contain a 'name' field"})
		return
	}
	description := strVal(raw, "description")
	sourceType := strVal(raw, "source_type")
	if sourceType == "" {
		sourceType = "container"
	}
	image := strVal(raw, "image")
	chartURL := strVal(raw, "chart_url")
	chartName := strVal(raw, "chart_name")
	chartVersion := strVal(raw, "chart_version")
	chartPath := strVal(raw, "chart_path")
	namespace := strVal(raw, "namespace")
	if namespace == "" {
		namespace = "default"
	}
	cpu := strVal(raw, "cpu")
	if cpu == "" {
		cpu = "100m"
	}
	memory := strVal(raw, "memory")
	if memory == "" {
		memory = "128Mi"
	}
	replicas := intVal(raw, "replicas")
	if replicas < 1 {
		replicas = 1
	}
	category := strVal(raw, "category")
	if category == "" {
		category = "general"
	}

	// Build values_yaml from default_values or values_yaml field
	valuesYAML := ""
	if v, ok := raw["values_yaml"]; ok {
		if s, ok := v.(string); ok {
			valuesYAML = s
		} else {
			b, _ := yaml.Marshal(v)
			valuesYAML = string(b)
		}
	} else if dv, ok := raw["default_values"]; ok {
		b, _ := yaml.Marshal(dv)
		valuesYAML = string(b)
	}

	// Parse ports
	var ports []int
	if p, ok := raw["ports"]; ok {
		switch pv := p.(type) {
		case []interface{}:
			for _, item := range pv {
				switch n := item.(type) {
				case int:
					ports = append(ports, n)
				case float64:
					ports = append(ports, int(n))
				}
			}
		}
	}
	if ports == nil {
		ports = []int{}
	}

	var bp serviceBlueprintRow
	var helmRepoID *string
	err := h.deps.DB.Pool.QueryRow(c.Request.Context(), `
		INSERT INTO service_blueprints
			(name, description, source_type, helm_repo_id, image, chart_url,
			 chart_name, chart_version, chart_path, namespace, values_yaml,
			 cpu, memory, replicas, ports, category, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING id, name, COALESCE(description,''), source_type,
		          helm_repo_id, COALESCE(image,''), COALESCE(chart_url,''),
		          COALESCE(chart_name,''), COALESCE(chart_version,''),
		          COALESCE(chart_path,''), COALESCE(namespace,'default'),
		          COALESCE(values_yaml,''), COALESCE(cpu,'100m'),
		          COALESCE(memory,'128Mi'), COALESCE(replicas,1),
		          COALESCE(ports,'{}'), COALESCE(category,'general'),
		          created_at
	`,
		name, description, sourceType, helmRepoID,
		image, chartURL, chartName, chartVersion,
		chartPath, namespace, valuesYAML,
		cpu, memory, replicas, ports,
		category, userID,
	).Scan(
		&bp.ID, &bp.Name, &bp.Description, &bp.SourceType,
		&helmRepoID, &bp.Image, &bp.ChartURL,
		&bp.ChartName, &bp.ChartVersion, &bp.ChartPath,
		&bp.Namespace, &bp.ValuesYAML, &bp.CPU,
		&bp.Memory, &bp.Replicas, &bp.Ports, &bp.Category,
		&bp.CreatedAt,
	)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	if helmRepoID != nil {
		bp.HelmRepoID = helmRepoID
	}
	if bp.Ports == nil {
		bp.Ports = []int{}
	}

	logAudit(h.deps, c, "ai_apply", "service_blueprint", bp.ID, nil, bp)

	c.JSON(http.StatusCreated, gin.H{
		"blueprint": bp,
		"message":   fmt.Sprintf("Blueprint '%s' created successfully!", bp.Name),
	})
}

// parseYAMLContent strips fences and parses YAML into a generic map.
func parseYAMLContent(c *gin.Context, content string) (map[string]interface{}, bool) {
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid YAML: %v", err)})
		return nil, false
	}
	return raw, true
}

// applyWorkflow parses YAML content and creates a workflow.
func (h *AIHandlers) applyWorkflow(c *gin.Context, content string) {
	raw, ok := parseYAMLContent(c, content)
	if !ok {
		return
	}

	name := strVal(raw, "name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "YAML must contain a 'name' field"})
		return
	}

	tenantID := auth.GetTenantID(c)
	userID := auth.GetUserID(c)

	// Build the spec from the YAML — extract steps, triggers, settings
	spec := map[string]interface{}{}
	if steps, ok := raw["steps"]; ok {
		spec["steps"] = steps
	}
	if triggers, ok := raw["triggers"]; ok {
		spec["triggers"] = triggers
	}
	if settings, ok := raw["settings"]; ok {
		spec["settings"] = settings
	}
	if ctx, ok := raw["context"]; ok {
		spec["context"] = ctx
	}
	// Ensure steps is never nil for the DB constraint
	if _, hasSteps := spec["steps"]; !hasSteps {
		spec["steps"] = []interface{}{}
	}

	specJSON, err := json.Marshal(spec)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid spec: %v", err)})
		return
	}

	var w models.Workflow
	err = h.deps.DB.Pool.QueryRow(c.Request.Context(), `
		INSERT INTO workflows (name, tenant_id, spec, version, source, is_enabled, created_by)
		VALUES ($1, $2, $3, 1, 'yaml', true, $4)
		RETURNING id, name, tenant_id, spec, version, source, COALESCE(git_path,''),
		          is_enabled, is_locked, created_by, created_at, updated_at
	`,
		name, tenantID, specJSON, userID,
	).Scan(
		&w.ID, &w.Name, &w.TenantID, &w.Spec, &w.Version, &w.Source,
		&w.GitPath, &w.IsEnabled, &w.IsLocked, &w.CreatedBy,
		&w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "ai_apply", "workflow", w.ID.String(), nil, gin.H{"name": w.Name})
	c.JSON(http.StatusCreated, gin.H{
		"workflow": w,
		"message":  fmt.Sprintf("Workflow '%s' created successfully!", w.Name),
	})
}

// applyScorecard parses YAML content and creates a scorecard with rules.
func (h *AIHandlers) applyScorecard(c *gin.Context, content string) {
	raw, ok := parseYAMLContent(c, content)
	if !ok {
		return
	}

	name := strVal(raw, "name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "YAML must contain a 'name' field"})
		return
	}

	tenantID := auth.GetTenantID(c)
	userID := auth.GetUserID(c)
	description := strVal(raw, "description")
	enabled := true
	if v, ok := raw["enabled"]; ok {
		if b, ok := v.(bool); ok {
			enabled = b
		}
	}

	configJSON, _ := json.Marshal(map[string]interface{}{})

	var sc models.Scorecard
	err := h.deps.DB.Pool.QueryRow(c.Request.Context(), `
		INSERT INTO scorecards (tenant_id, name, description, enabled, config, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, name, COALESCE(description,''), enabled, COALESCE(config,'{}'),
		          created_by, created_at, updated_at
	`,
		tenantID, name, description, enabled, configJSON, userID,
	).Scan(
		&sc.ID, &sc.TenantID, &sc.Name, &sc.Description, &sc.Enabled,
		&sc.Config, &sc.CreatedBy, &sc.CreatedAt, &sc.UpdatedAt,
	)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	// Create rules from YAML
	rulesCreated := 0
	if rulesRaw, ok := raw["rules"]; ok {
		if rulesList, ok := rulesRaw.([]interface{}); ok {
			for _, r := range rulesList {
				ruleMap, ok := r.(map[string]interface{})
				if !ok {
					continue
				}
				ruleName := strVal(ruleMap, "name")
				if ruleName == "" {
					continue
				}
				expression := strVal(ruleMap, "expression")
				if expression == "" {
					continue
				}
				weight := intVal(ruleMap, "weight")
				if weight < 1 {
					weight = 5
				}
				severity := strVal(ruleMap, "severity")
				if severity == "" {
					severity = "warning"
				}

				_, err = h.deps.DB.Pool.Exec(c.Request.Context(), `
					INSERT INTO scorecard_rules
						(scorecard_id, name, description, expression, weight, pass_message, fail_message, severity)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				`,
					sc.ID, ruleName, strVal(ruleMap, "description"), expression,
					weight, strVal(ruleMap, "pass_message"), strVal(ruleMap, "fail_message"), severity,
				)
				if err == nil {
					rulesCreated++
				}
			}
		}
	}

	logAudit(h.deps, c, "ai_apply", "scorecard", sc.ID.String(), nil, gin.H{"name": sc.Name, "rules": rulesCreated})
	c.JSON(http.StatusCreated, gin.H{
		"scorecard": sc,
		"message":   fmt.Sprintf("Scorecard '%s' created with %d rules!", sc.Name, rulesCreated),
	})
}

// applyPipelineSource parses YAML content and creates a pipeline source.
func (h *AIHandlers) applyPipelineSource(c *gin.Context, content string) {
	raw, ok := parseYAMLContent(c, content)
	if !ok {
		return
	}

	name := strVal(raw, "name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "YAML must contain a 'name' field"})
		return
	}

	tenantID := auth.GetTenantID(c)
	userID := auth.GetUserID(c)
	sourceType := strVal(raw, "source_type")
	if sourceType == "" {
		sourceType = "gitlab_ci"
	}
	description := strVal(raw, "description")

	// Build config JSON from the config key or empty
	configJSON, _ := json.Marshal(map[string]interface{}{})
	if cfg, ok := raw["config"]; ok {
		if b, err := json.Marshal(cfg); err == nil {
			configJSON = b
		}
	}

	var ps models.PipelineSource
	err := h.deps.DB.Pool.QueryRow(c.Request.Context(), `
		INSERT INTO pipeline_sources (tenant_id, name, source_type, description, config, status, created_by)
		VALUES ($1, $2, $3, $4, $5, 'active', $6)
		RETURNING id, tenant_id, name, source_type, COALESCE(description,''),
		          COALESCE(config,'{}'), COALESCE(status,'active'), created_by, created_at, updated_at
	`,
		tenantID, name, sourceType, description, configJSON, userID,
	).Scan(
		&ps.ID, &ps.TenantID, &ps.Name, &ps.SourceType,
		&ps.Description, &ps.Config, &ps.Status, &ps.CreatedBy,
		&ps.CreatedAt, &ps.UpdatedAt,
	)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "ai_apply", "pipeline_source", ps.ID.String(), nil, gin.H{"name": ps.Name})
	c.JSON(http.StatusCreated, gin.H{
		"pipeline_source": ps,
		"message":         fmt.Sprintf("Pipeline source '%s' created successfully!", ps.Name),
	})
}

// applyRBACPolicy parses YAML content and creates a role with permissions.
func (h *AIHandlers) applyRBACPolicy(c *gin.Context, content string) {
	raw, ok := parseYAMLContent(c, content)
	if !ok {
		return
	}

	name := strVal(raw, "name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "YAML must contain a 'name' field"})
		return
	}

	tenantID := auth.GetTenantID(c)
	slug := strVal(raw, "slug")
	if slug == "" {
		slug = strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	}
	description := strVal(raw, "description")
	scope := strVal(raw, "scope")
	if scope == "" {
		scope = "tenant"
	}

	role, err := h.deps.RBAC.CreateRole(c.Request.Context(), tenantID, name, slug, description, scope)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	// Create permissions from YAML
	permsCreated := 0
	if permsRaw, ok := raw["permissions"]; ok {
		if permsList, ok := permsRaw.([]interface{}); ok {
			for _, p := range permsList {
				permMap, ok := p.(map[string]interface{})
				if !ok {
					continue
				}
				resource := strVal(permMap, "resource")
				action := strVal(permMap, "action")
				if resource == "" || action == "" {
					continue
				}
				effect := strVal(permMap, "effect")
				if effect == "" {
					effect = "allow"
				}
				_, err = h.deps.RBAC.AddPermission(c.Request.Context(), role.ID, resource, action, effect)
				if err == nil {
					permsCreated++
				}
			}
		}
	}

	logAudit(h.deps, c, "ai_apply", "role", role.ID.String(), nil, gin.H{"name": name, "permissions": permsCreated})
	c.JSON(http.StatusCreated, gin.H{
		"role":    role,
		"message": fmt.Sprintf("Role '%s' created with %d permissions!", name, permsCreated),
	})
}

// applyGitOps parses YAML content and creates a GitOps repository.
func (h *AIHandlers) applyGitOps(c *gin.Context, content string) {
	raw, ok := parseYAMLContent(c, content)
	if !ok {
		return
	}

	name := strVal(raw, "name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "YAML must contain a 'name' field"})
		return
	}

	tenantID := auth.GetTenantID(c)
	repoURL := strVal(raw, "repo_url")
	if repoURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "YAML must contain a 'repo_url' field"})
		return
	}
	branch := strVal(raw, "branch")
	if branch == "" {
		branch = "main"
	}
	path := strVal(raw, "path")
	if path == "" {
		path = "."
	}
	engineType := strVal(raw, "engine_type")
	if engineType == "" {
		engineType = "auto"
	}

	repo := &gitops.Repo{
		TenantID:   tenantID,
		Name:       name,
		RepoURL:    repoURL,
		Branch:     branch,
		Path:       path,
		EngineType: engineType,
		Config:     map[string]string{},
	}

	if h.deps.Repos.GitopsRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gitops repository not available"})
		return
	}

	if err := h.deps.Repos.GitopsRepo.Create(c.Request.Context(), repo); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "ai_apply", "gitops_repo", repo.ID.String(), nil, gin.H{"name": name})
	c.JSON(http.StatusCreated, gin.H{
		"gitops_repo": repo,
		"message":     fmt.Sprintf("GitOps repository '%s' created successfully!", name),
	})
}

// stripCodeFences removes markdown code fences (```yaml ... ``` or ``` ... ```)
// and any leading/trailing non-YAML text that LLMs often include.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	// If the content starts with a code fence, find the opening and closing markers
	if strings.HasPrefix(s, "```") {
		// Remove the opening fence line (e.g. ```yaml or ```)
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		// Remove the closing fence if present
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
}

// strVal safely extract a string from a map
func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// intVal safely extract an int from a map
func intVal(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case float64:
			return int(n)
		}
	}
	return 0
}
