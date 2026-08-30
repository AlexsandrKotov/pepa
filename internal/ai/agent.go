package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// AgentMode determines how the agent executes tasks.
type AgentMode string

const (
	// AgentModeNative uses the LLM's native function calling (tool_calls).
	AgentModeNative AgentMode = "native"
	// AgentModePrompt uses prompt-based intent classification for models
	// that do not support native function calling.
	AgentModePrompt AgentMode = "prompt"
)

// Agent implements a ReAct (Reason + Act) loop that uses LLM + tools to answer
// questions using real PEPA data. The agent NEVER goes beyond PEPA's service
// boundaries — it can only call registered tools.
//
// In native mode the LLM is expected to support OpenAI-style function calling.
// In prompt mode the agent delegates to an IntentRouter that classifies the
// user's intent via structured prompts, executes tools deterministically, and
// synthesises a response — working with ANY chat model.
type Agent struct {
	provider          LLMProvider
	tools             *ToolRegistry
	policy            *AgentPolicy
	mode              AgentMode
	router            *IntentRouter
	maxIterations     int
	systemInstruction string // custom system instruction
	mu                sync.RWMutex
}

// NewAgent creates a new agent with the given provider, tools, policy and mode.
func NewAgent(provider LLMProvider, tools *ToolRegistry, policy *AgentPolicy, mode AgentMode) *Agent {
	a := &Agent{
		provider:      provider,
		tools:         tools,
		policy:        policy,
		mode:          mode,
		maxIterations: 15,
	}
	// Always create the router — it serves as a fallback for native mode
	// when token limits are exceeded (413 error).
	a.router = NewIntentRouter(provider, tools, policy)
	return a
}

// AgentTask represents a single agent execution task.
type AgentTask struct {
	Description string
	UserRole    string // admin, viewer, etc.
	// History is the conversation history for multi-turn context.
	History []Message
	// SystemInstruction overrides the default system prompt with a custom
	// instruction. When empty, the default prompt is used.
	SystemInstruction string
}

// AgentActionResult records one tool call made by the agent.
type AgentActionResult struct {
	ToolName  string          `json:"tool_name"`
	ToolArgs  json.RawMessage `json:"tool_args"`
	Result    string          `json:"result"`
	Error     string          `json:"error,omitempty"`
	Policy    string          `json:"policy"` // observe, auto_fix, require_approval, forbidden
	Timestamp time.Time       `json:"timestamp"`
}

// AgentResponse is the final agent response returned to the user.
type AgentResponse struct {
	Answer        string              `json:"answer"`
	ToolCalls     []AgentActionResult `json:"tool_calls,omitempty"`
	ModelUsed     string              `json:"model_used"`
	TokensUsed    TokenUsage          `json:"tokens_used"`
	NeedsApproval *ApprovalRequest    `json:"needs_approval,omitempty"`
}

// ApprovalRequest is returned when the agent wants to perform a high-risk action.
type ApprovalRequest struct {
	ToolName    string          `json:"tool_name"`
	ToolArgs    json.RawMessage `json:"tool_args"`
	Description string          `json:"description"`
	Reason      string          `json:"reason"`
}

const agentSystemPrompt = `You are the PEPA AI Agent — a versatile assistant for the PEPA platform engineering portal. Answer in the SAME LANGUAGE the user used.

CAPABILITIES:
1. PLATFORM DATA — Use tools to fetch real PEPA data (services, deployments, clusters, pipelines, Docker, workflows). For infrastructure questions, call tools FIRST.
2. GENERAL KNOWLEDGE — Answer DevOps, Kubernetes, Docker, CI/CD, cloud, programming questions directly from your knowledge.
3. CONFIG GENERATION — Generate YAML, JSON, HCL configs when asked.
4. ACTIONS — Create blueprints (create_blueprint), services (create_service), deploy (deploy_service), manage Docker, trigger pipelines, execute workflows. When the user asks to create and deploy something, use create_blueprint to register the blueprint, then guide them to the Pipeline Builder to deploy it.

RULES:
- Be concise. Use bullet points, tables, code blocks.
- PEPA data questions → call tools. NEVER fabricate data.
- General knowledge → answer directly, no tools needed.
- Mixed questions → explain + call tools.
- Empty tool results → say "No data found".
- NEVER suggest shell commands when you have tools.
- Suggest next steps based on data found.
- When creating a blueprint, use create_blueprint tool with structured parameters (name, image, ports, etc.), not create_service.

ANSWER IN THE SAME LANGUAGE THE USER USED.`

// SetSystemInstruction sets a custom system instruction that overrides the
// default prompts used for classification and synthesis.
func (a *Agent) SetSystemInstruction(instruction string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.systemInstruction = instruction
}

// Run executes the agent using the configured mode.
func (a *Agent) Run(ctx context.Context, task *AgentTask, userMessage string) (*AgentResponse, error) {
	if a.mode == AgentModePrompt {
		slog.Info("Running in PROMPT mode (intent classification + synthesis)")
		var history []Message
		var sysInstruction string
		if task != nil {
			history = task.History
			sysInstruction = task.SystemInstruction
		}
		if sysInstruction == "" {
			a.mu.RLock()
			sysInstruction = a.systemInstruction
			a.mu.RUnlock()
		}
		return a.router.Run(ctx, userMessage, &RunOptions{History: history, SystemInstruction: sysInstruction})
	}

	slog.Info("Running in NATIVE mode (function calling)")
	resp, err := a.runNative(ctx, task, userMessage)
	if err != nil {
		// Auto-fallback: if native mode fails with 413 (token overflow) and we
		// have a router available, retry in prompt mode which doesn't send tool
		// definitions and thus uses far fewer tokens.
		errMsg := err.Error()
		if strings.Contains(errMsg, "too large") && a.router != nil {
			slog.Info("Native mode failed with 413, falling back to prompt mode")
			var history []Message
			var sysInstruction string
			if task != nil {
				history = task.History
				sysInstruction = task.SystemInstruction
			}
			return a.router.Run(ctx, userMessage, &RunOptions{History: history, SystemInstruction: sysInstruction})
		}
		return nil, err
	}
	return resp, nil
}

// nativeToolLoopResult holds the state after the tool execution loop.
type nativeToolLoopResult struct {
	messages    []Message
	results     []AgentActionResult
	modelUsed   string
	tokensUsed  TokenUsage
	finalAnswer string // set when the LLM gives a direct answer without tools
}

// runNativeToolLoop executes the ReAct tool-calling loop and returns the full
// message history plus collected results. If the LLM answers without calling
// tools (and no tools were previously called), finalAnswer is set.
func (a *Agent) runNativeToolLoop(ctx context.Context, task *AgentTask, userMessage string) (*nativeToolLoopResult, error) {
	systemPrompt := agentSystemPrompt
	if task != nil && task.SystemInstruction != "" {
		systemPrompt += "\n\nADDITIONAL USER GUIDANCE:\n" + task.SystemInstruction
	} else {
		a.mu.RLock()
		if a.systemInstruction != "" {
			systemPrompt += "\n\nADDITIONAL USER GUIDANCE:\n" + a.systemInstruction
		}
		a.mu.RUnlock()
	}

	messages := []Message{
		{Role: "system", Content: systemPrompt},
	}

	// Limit history to last 6 messages to save tokens (each message = input tokens)
	const maxHistoryMsgs = 6
	if task != nil && len(task.History) > 0 {
		start := 0
		if len(task.History) > maxHistoryMsgs {
			start = len(task.History) - maxHistoryMsgs
		}
		for _, h := range task.History[start:] {
			if h.Role == "user" || h.Role == "assistant" {
				messages = append(messages, h)
			}
		}
	}

	messages = append(messages, Message{Role: "user", Content: userMessage})

	// Dynamic tool selection: only send tools relevant to the user's message.
	// This dramatically reduces token usage (e.g. "what is K8s?" → 0 tools,
	// "list services" → 4 tools instead of 30+).
	selectedTools := selectToolsForMessage(a.tools, userMessage)
	slog.Info("dynamic tool selection", "selected", len(selectedTools), "total", len(a.tools.List()))

	opts := &ChatOptions{
		MaxTokens:  4096,
		ToolChoice: "auto",
	}
	if len(selectedTools) > 0 {
		opts.Tools = selectedTools
	}

	var results []AgentActionResult
	toolCallsMade := false
	var totalTokens TokenUsage
	var modelUsed string

	for iteration := 0; iteration < a.maxIterations; iteration++ {
		resp, err := a.provider.Chat(ctx, messages, opts)
		if err != nil {
			errMsg := err.Error()
			// Provide actionable messages for common provider errors
			if strings.Contains(errMsg, "413") || strings.Contains(errMsg, "too large") || strings.Contains(errMsg, "rate_limit") {
				slog.Info("Token limit exceeded", "error", err)
				return nil, fmt.Errorf("the request is too large for this model. Try switching to 'Prompt' mode in the toolbar above, or start a new chat to clear conversation history")
			}
			if strings.Contains(errMsg, "404") {
				slog.Info("Model not found (404)", "error", err)
				return nil, fmt.Errorf("the selected model or endpoint was not found. Please check your AI provider settings in Connections")
			}
			return nil, fmt.Errorf("agent LLM call failed: %w", err)
		}

		totalTokens.InputTokens += resp.TokensUsed.InputTokens
		totalTokens.OutputTokens += resp.TokensUsed.OutputTokens
		totalTokens.TotalTokens += resp.TokensUsed.TotalTokens
		modelUsed = resp.ModelUsed

		slog.Info("Iteration : LLM returned tool calls, finish_reason=, content_len=", "arg1", iteration, "count", len(resp.ToolCalls), resp.FinishReason, len(resp.Content))

		// If no tool calls — we have the final answer
		if len(resp.ToolCalls) == 0 {
			if !toolCallsMade {
				// The updated prompt allows general knowledge answers.
				// Only re-prompt if the message looks like it needs platform data.
				needsData := looksLikeDataQuery(userMessage)
				if needsData {
					slog.Info("WARNING: LLM answered without calling tools for data query — re-prompting")
					messages = append(messages, Message{Role: "assistant", Content: resp.Content})
					messages = append(messages, Message{
						Role:    "user",
						Content: "CRITICAL: You MUST call the available tools to fetch real data from PEPA. Do NOT respond with shell commands, generic instructions, or made-up data. Call the appropriate tool NOW by using the function calling interface.",
					})
					resp2, err2 := a.provider.Chat(ctx, messages, opts)
					if err2 != nil {
						return nil, fmt.Errorf("agent LLM re-prompt failed: %w", err2)
					}
					totalTokens.InputTokens += resp2.TokensUsed.InputTokens
					totalTokens.OutputTokens += resp2.TokensUsed.OutputTokens
					totalTokens.TotalTokens += resp2.TokensUsed.TotalTokens
					modelUsed = resp2.ModelUsed
					if len(resp2.ToolCalls) == 0 {
						slog.Info("WARNING: Model did not call tools after re-prompt (model=)", "arg1", resp2.ModelUsed)
						return &nativeToolLoopResult{
							messages:    messages,
							results:     results,
							modelUsed:   modelUsed,
							tokensUsed:  totalTokens,
							finalAnswer: "I was unable to fetch data from PEPA. Try switching the agent mode from 'Auto' to 'Prompt' in the toolbar above — prompt mode works with any chat model.",
						}, nil
					}
					resp = resp2
				} else {
					// General knowledge question — return the LLM's answer directly.
					return &nativeToolLoopResult{
						messages:    messages,
						results:     results,
						modelUsed:   modelUsed,
						tokensUsed:  totalTokens,
						finalAnswer: resp.Content,
					}, nil
				}
			} else {
				// Tools were called previously, this is the final synthesis.
				return &nativeToolLoopResult{
					messages:    messages,
					results:     results,
					modelUsed:   modelUsed,
					tokensUsed:  totalTokens,
					finalAnswer: resp.Content,
				}, nil
			}
		}

		toolCallsMade = true

		messages = append(messages, Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute all tool calls in parallel for speed.
		type toolOutcome struct {
			result AgentActionResult
			msg    Message
		}
		outcomes := make([]toolOutcome, len(resp.ToolCalls))

		var wg sync.WaitGroup
		for i, tc := range resp.ToolCalls {
			wg.Add(1)
			go func(idx int, tc ToolCall) {
				defer wg.Done()
				toolArgs := json.RawMessage(tc.Function.Arguments)

				level, policyErr := a.policy.Check(ctx, tc.Function.Name)
				if policyErr != nil {
					outcomes[idx] = toolOutcome{
						result: AgentActionResult{
							ToolName: tc.Function.Name, ToolArgs: toolArgs,
							Error: policyErr.Error(), Policy: string(level), Timestamp: time.Now(),
						},
						msg: Message{Role: "tool", Content: "Error: " + policyErr.Error(), ToolCallID: tc.ID},
					}
					return
				}

				if level == LevelApprove {
					outcomes[idx] = toolOutcome{
						result: AgentActionResult{
							ToolName: tc.Function.Name, ToolArgs: toolArgs,
							Error: "requires_approval", Policy: string(level), Timestamp: time.Now(),
						},
						msg: Message{Role: "tool", Content: "This action requires user approval.", ToolCallID: tc.ID},
					}
					return
				}

				slog.Info("Calling tool: with args", "name", tc.Function.Name, "arg2", string(toolArgs))
				toolResult, execErr := a.tools.Execute(ctx, tc.Function.Name, toolArgs)

				r := AgentActionResult{
					ToolName: tc.Function.Name, ToolArgs: toolArgs,
					Result: toolResult, Policy: string(level), Timestamp: time.Now(),
				}
				content := toolResult
				if execErr != nil {
					r.Error = execErr.Error()
					content = "Error: " + execErr.Error()
					slog.Info("Tool failed", "name", tc.Function.Name, "error", execErr)
				} else {
					slog.Info("Tool returned bytes", "name", tc.Function.Name, "count", len(toolResult))
				}
				outcomes[idx] = toolOutcome{
					result: r,
					msg:    Message{Role: "tool", Content: content, ToolCallID: tc.ID},
				}
			}(i, tc)
		}
		wg.Wait()

		// Collect results in order.
		for _, o := range outcomes {
			results = append(results, o.result)
			messages = append(messages, o.msg)
		}
	}

	return &nativeToolLoopResult{
		messages:    messages,
		results:     results,
		modelUsed:   modelUsed,
		tokensUsed:  totalTokens,
		finalAnswer: "I've reached the maximum number of tool calls. Here's what I found so far based on the data gathered.",
	}, nil
}

// ── Dynamic tool selection ────────────────────────────────────────────────────
// Instead of sending all 30+ tool definitions to the LLM (which wastes tokens
// and causes 413 errors on small-context models), we analyze the user message
// and send only the relevant subset.

// toolCategoryGroups maps category names to tool name prefixes.
var toolCategoryGroups = map[string][]string{
	"service":    {"list_services", "get_service", "update_service", "create_service"},
	"docker":     {"list_docker_services", "get_docker_service", "get_docker_service_logs", "restart_docker_service", "stop_docker_service", "start_docker_service", "refresh_docker_service", "refresh_all_docker_services", "create_docker_service"},
	"cluster":    {"list_clusters", "get_cluster"},
	"deployment": {"list_deployments", "get_deployment", "deploy_service"},
	"pipeline":   {"list_pipelines", "get_pipeline", "list_pipeline_runs", "trigger_pipeline"},
	"workflow":   {"list_workflows", "get_workflow", "execute_workflow"},
	"infra":      {"list_environments", "create_environment", "list_connections", "create_connection"},
	"entity":     {"list_entities", "get_entity", "create_entity", "update_entity"},
	"plugin":     {"list_plugins"},
	"jira":       {"list_jira_issues"},
	"blueprint":  {"create_blueprint"},
}

// categoryKeywords maps keywords (English + Russian) to tool categories.
var categoryKeywords = map[string][]string{
	// English
	"service": {"service"}, "catalog": {"service"},
	"docker": {"docker"}, "container": {"docker"},
	"cluster": {"cluster"}, "kubernetes": {"cluster"}, "k8s": {"cluster"},
	"deploy": {"deployment"}, "deployment": {"deployment"},
	"pipeline": {"pipeline"}, "ci": {"pipeline"}, "cicd": {"pipeline"}, "gitlab": {"pipeline"},
	"workflow": {"workflow"}, "automation": {"workflow"},
	"environment": {"infra"}, "env": {"infra"}, "connection": {"infra"},
	"entity": {"entity"},
	"plugin": {"plugin"},
	"jira": {"jira"}, "issue": {"jira"},
	"blueprint": {"blueprint"}, "blueprints": {"blueprint"},
	// Russian
	"сервис": {"service"}, "сервисы": {"service"}, "каталог": {"service"},
	"докер": {"docker"}, "контейнер": {"docker"}, "контейнеры": {"docker"},
	"кластер": {"cluster"}, "кластеры": {"cluster"},
	"деплой": {"deployment"}, "деплоймент": {"deployment"}, "развертывание": {"deployment"},
	"пайплайн": {"pipeline"}, "пайплайны": {"pipeline"},
	"воркфлоу": {"workflow"}, "автоматизац": {"workflow"},
	"окружен": {"infra"}, "подключен": {"infra"},
	"сущност": {"entity"},
	"плагин": {"plugin"},
	"джира": {"jira"}, "задач": {"jira"},
	"блюпринт": {"blueprint"}, "шаблон": {"blueprint"},
}

// selectToolsForMessage analyzes the user message and returns only the relevant
// tool definitions. Returns nil if no tools are needed (general knowledge).
func selectToolsForMessage(registry *ToolRegistry, message string) []ToolDefinition {
	msg := strings.ToLower(message)

	// Detect which categories are needed
	needed := make(map[string]bool)
	for keyword, categories := range categoryKeywords {
		if strings.Contains(msg, keyword) {
			for _, cat := range categories {
				needed[cat] = true
			}
		}
	}

	// If nothing matched, check for generic list/show/get verbs
	if len(needed) == 0 {
		genericVerbs := []string{"list", "show", "get", "create", "build", "покажи", "показать", "список", "выведи", "все", "создай", "создать", "создайте"}
		for _, v := range genericVerbs {
			if strings.Contains(msg, v) {
				// Generic query — send all tools (the LLM needs to see them)
				return registry.List()
			}
		}
		// No data keywords and no list verbs → general knowledge, no tools needed
		return nil
	}

	// Collect tool definitions for matched categories
	allDefs := registry.List()
	defMap := make(map[string]ToolDefinition, len(allDefs))
	for _, d := range allDefs {
		defMap[d.Name] = d
	}

	seen := make(map[string]bool)
	var result []ToolDefinition
	for cat := range needed {
		for _, toolName := range toolCategoryGroups[cat] {
			if !seen[toolName] {
				if def, ok := defMap[toolName]; ok {
					result = append(result, def)
					seen[toolName] = true
				}
			}
		}
	}

	// If too many tools selected (>15), fall back to all tools
	if len(result) > 15 {
		return allDefs
	}

	return result
}

// looksLikeDataQuery returns true if the message seems to need PEPA platform data.
// Uses specific phrases rather than single common words to reduce false positives
// (e.g. "How to start with K8s?" should NOT trigger this).
func looksLikeDataQuery(msg string) bool {
	msg = strings.ToLower(msg)
	dataPhrases := []string{
		// English: specific data-access phrases
		"list all", "list my", "list the", "list services", "list clusters",
		"list deployments", "list pipelines", "list workflows", "list environments",
		"list connections", "list plugins", "list entities", "list docker",
		"list jira", "list pipeline runs",
		"show all", "show my", "show me the", "show services", "show clusters",
		"show deployments", "show pipelines", "show workflows", "show environments",
		"show connections", "show plugins", "show docker",
		"get service", "get deployment", "get cluster",
		"what is running", "what services are", "what clusters are",
		"what deployments are", "what pipelines",
		"restart the", "restart docker", "stop the docker",
		"start the docker", "deploy the", "create a service",
		"create a connection", "trigger the pipeline",
		"execute the workflow",
		// Russian: specific data-access phrases
		"покажи все", "покажи мои", "показать все", "показать мои",
		"список сервис", "список кластер", "список депло",
		"список пайплайн", "список воркфлоу", "список окружен",
		"список подключен", "список плагин", "список сущност",
		"выведи все", "выведи мои",
		"перезапусти", "запусти docker", "останови docker",
		"создай сервис", "создай подключен",
	}
	for _, phrase := range dataPhrases {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}

// runNative executes the ReAct loop with native function calling.
func (a *Agent) runNative(ctx context.Context, task *AgentTask, userMessage string) (*AgentResponse, error) {
	lr, err := a.runNativeToolLoop(ctx, task, userMessage)
	if err != nil {
		return nil, err
	}
	return &AgentResponse{
		Answer:     StripThinkBlocks(lr.finalAnswer),
		ToolCalls:  lr.results,
		ModelUsed:  lr.modelUsed,
		TokensUsed: lr.tokensUsed,
	}, nil
}

// Router returns the intent router for prompt-mode streaming.
func (a *Agent) Router() *IntentRouter {
	return a.router
}

// Stream executes the agent and returns a streaming channel.
// In both prompt and native modes, the final synthesis is streamed token-by-token.
func (a *Agent) Stream(ctx context.Context, task *AgentTask, userMessage string) (<-chan *StreamChunk, error) {
	if a.mode == AgentModePrompt && a.router != nil {
		var history []Message
		var sysInstruction string
		if task != nil {
			history = task.History
			sysInstruction = task.SystemInstruction
		}
		if sysInstruction == "" {
			a.mu.RLock()
			sysInstruction = a.systemInstruction
			a.mu.RUnlock()
		}
		return a.router.RunStream(ctx, userMessage, &RunOptions{
			History:           history,
			Stream:            true,
			SystemInstruction: sysInstruction,
		})
	}

	// Native mode: run the tool loop, then stream the final synthesis.
	ch := make(chan *StreamChunk, 32)
	go func() {
		defer close(ch)

		lr, err := a.runNativeToolLoop(ctx, task, userMessage)
		if err != nil {
			// Auto-fallback: if native mode fails with 413, retry via prompt mode streaming
			errMsg := err.Error()
			if strings.Contains(errMsg, "too large") && a.router != nil {
				slog.Info("Native streaming failed with 413, falling back to prompt mode")
				var history []Message
				var sysInstruction string
				if task != nil {
					history = task.History
					sysInstruction = task.SystemInstruction
				}
				promptStream, sErr := a.router.RunStream(ctx, userMessage, &RunOptions{
					History:           history,
					Stream:            true,
					SystemInstruction: sysInstruction,
				})
				if sErr != nil {
					sendChunk(ch, ctx, &StreamChunk{Type: "text", Content: fmt.Sprintf("Error: %v", sErr)})
					return
				}
				for chunk := range promptStream {
					sendChunk(ch, ctx, chunk)
				}
				return
			}
			sendChunk(ch, ctx, &StreamChunk{Type: "text", Content: fmt.Sprintf("Error: %v", err)})
			return
		}

		// Emit tool_call and tool_result events for the UI.
		for _, r := range lr.results {
			sendChunk(ch, ctx, &StreamChunk{
				Type: "tool_call",
				Metadata: map[string]interface{}{
					"tool_name": r.ToolName,
					"params":    json.RawMessage(r.ToolArgs),
				},
			})
			resultChunk := &StreamChunk{
				Type: "tool_result",
				ToolResult: &ToolResult{
					Content: r.Result,
				},
				Metadata: map[string]interface{}{
					"tool_name": r.ToolName,
					"policy":    r.Policy,
				},
			}
			if r.Error != "" {
				resultChunk.ToolResult.Error = r.Error
			}
			sendChunk(ch, ctx, resultChunk)
		}

		// If we already have a final answer, send it directly.
		// In native mode the tool loop already produced the complete answer;
		// re-streaming via the provider would risk the LLM generating a
		// different response since lr.messages lacks the final assistant turn.
		if lr.finalAnswer != "" {
			sendChunk(ch, ctx, &StreamChunk{Type: "text", Content: StripThinkBlocks(lr.finalAnswer)})
		}
	}()
	return ch, nil
}
