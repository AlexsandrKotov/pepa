package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// quickIntentPattern maps a regex pattern to a tool name and default params.
// Matched before LLM classification — instant response for common queries.
type quickIntentPattern struct {
	pattern  *regexp.Regexp
	toolName string
	params   map[string]interface{}
}

// quickIntentPatterns is checked in order; first match wins.
var quickIntentPatterns = []quickIntentPattern{
	// Services
	{regexp.MustCompile(`(?i)^(list|show|get|all)\s+(all\s+)?services`), "list_services", map[string]interface{}{}},
	{regexp.MustCompile(`(?i)^(list|show|get)\s+(active|running)\s+services`), "list_services", map[string]interface{}{"status": "active"}},
	{regexp.MustCompile(`(?i)^(list|show|get)\s+docker\s+services`), "list_docker_services", map[string]interface{}{}},
	// Clusters
	{regexp.MustCompile(`(?i)^(list|show|get|all)\s+(all\s+)?clusters`), "list_clusters", map[string]interface{}{}},
	// Deployments
	{regexp.MustCompile(`(?i)^(list|show|get|all)\s+(all\s+)?deployments`), "list_deployments", map[string]interface{}{}},
	// Pipelines
	{regexp.MustCompile(`(?i)^(list|show|get|all)\s+(all\s+)?pipelines`), "list_pipelines", map[string]interface{}{}},
	{regexp.MustCompile(`(?i)^(list|show|get)\s+(pipeline\s+)?runs`), "list_pipeline_runs", map[string]interface{}{}},
	// Workflows
	{regexp.MustCompile(`(?i)^(list|show|get|all)\s+(all\s+)?workflows`), "list_workflows", map[string]interface{}{}},
	// Environments
	{regexp.MustCompile(`(?i)^(list|show|get|all)\s+(all\s+)?environments`), "list_environments", map[string]interface{}{}},
	// Connections
	{regexp.MustCompile(`(?i)^(list|show|get|all)\s+(all\s+)?connections`), "list_connections", map[string]interface{}{}},
	// Plugins
	{regexp.MustCompile(`(?i)^(list|show|get|all)\s+(all\s+)?plugins`), "list_plugins", map[string]interface{}{}},
	// Entities
	{regexp.MustCompile(`(?i)^(list|show|get|all)\s+(all\s+)?entities`), "list_entities", map[string]interface{}{}},
	// Jira
	{regexp.MustCompile(`(?i)^(list|show|get)\s+(jira\s+)?issues`), "list_jira_issues", map[string]interface{}{}},
	// Russian: сервисы
	{regexp.MustCompile(`(?i)^(покажи|показать|список|выведи|get|list)\s+(все\s+)?сервис(ы|ов)`), "list_services", map[string]interface{}{}},
	{regexp.MustCompile(`(?i)^(покажи|показать|список|выведи)\s+(все\s+)?docker\s+(сервис|контейнер)`), "list_docker_services", map[string]interface{}{}},
	// Russian: кластеры
	{regexp.MustCompile(`(?i)^(покажи|показать|список|выведи)\s+(все\s+)?кластер(а|ов)`), "list_clusters", map[string]interface{}{}},
	// Russian: деплои
	{regexp.MustCompile(`(?i)^(покажи|показать|список|выведи)\s+(все\s+)?депло(и|ев|оймент)`), "list_deployments", map[string]interface{}{}},
	// Russian: пайплайны
	{regexp.MustCompile(`(?i)^(покажи|показать|список|выведи)\s+(все\s+)?пайплайн(а|ов)`), "list_pipelines", map[string]interface{}{}},
	// Russian: окружения
	{regexp.MustCompile(`(?i)^(покажи|показать|список|выведи)\s+(все\s+)?окружени(я|й)`), "list_environments", map[string]interface{}{}},
	// Russian: подключения
	{regexp.MustCompile(`(?i)^(покажи|показать|список|выведи)\s+(все\s+)?подключени(я|й)`), "list_connections", map[string]interface{}{}},
	// Russian: workflow
	{regexp.MustCompile(`(?i)^(покажи|показать|список|выведи)\s+(все\s+)?(воркфлоу|workflow(а|ов)?)`), "list_workflows", map[string]interface{}{}},
	// Russian: сущности
	{regexp.MustCompile(`(?i)^(покажи|показать|список|выведи)\s+(все\s+)?сущност(и|ей)`), "list_entities", map[string]interface{}{}},
	// Greetings and conversational → __direct__
	{regexp.MustCompile(`(?i)^(привет|здравствуй|хай|хелло|добр(ый|ое|ая)\s+(день|вечер|утро))\s*[!.?]*$`), "__direct__", map[string]interface{}{}},
	{regexp.MustCompile(`(?i)^(hi|hello|hey|good\s+(morning|afternoon|evening))\s*[!.?]*$`), "__direct__", map[string]interface{}{}},
	{regexp.MustCompile(`(?i)^(как\s+дела|как\s+ты|what'?s?\s+up|how\s+are\s+you)\s*[!.?]*$`), "__direct__", map[string]interface{}{}},
	{regexp.MustCompile(`(?i)^(спасибо|thanks?|thank\s+you)\s*[!.?]*$`), "__direct__", map[string]interface{}{}},
}

// matchQuickIntent tries to match the user message against known patterns.
// Returns nil if no match.
func matchQuickIntent(message string) *classifiedIntent {
	msg := strings.TrimSpace(message)
	for _, qp := range quickIntentPatterns {
		if qp.pattern.MatchString(msg) {
			// Copy params to avoid shared reference to the pattern's map
			params := make(map[string]interface{}, len(qp.params))
			for k, v := range qp.params {
				params[k] = v
			}
			return &classifiedIntent{
				ToolName:   qp.toolName,
				Params:     params,
				Confidence: "high",
				Reason:     "matched quick intent pattern",
			}
		}
	}
	return nil
}

// IntentRouter implements a prompt-based agent loop for models that do not
// support native function calling. It works by asking the LLM to classify the
// user's intent as a JSON object, executing the corresponding tool
// deterministically, and then synthesising a human-readable response.
type IntentRouter struct {
	provider    LLMProvider
	tools       *ToolRegistry
	policy      *AgentPolicy
	toolCatalog string          // cached category-grouped tool catalog for prompts
	toolNames   map[string]bool // cached tool name set for validation
}

// NewIntentRouter creates a new intent router.
func NewIntentRouter(provider LLMProvider, tools *ToolRegistry, policy *AgentPolicy) *IntentRouter {
	r := &IntentRouter{
		provider:  provider,
		tools:     tools,
		policy:    policy,
		toolNames: make(map[string]bool),
	}
	// Cache the category-grouped tool catalog and name set at creation time
	defs := tools.List()
	for _, t := range defs {
		r.toolNames[t.Name] = true
	}
	r.toolCatalog = buildCategoryCatalog(defs)
	return r
}

// toolCategory defines how tools are grouped for the classification prompt.
var toolCategoryOrder = []struct {
	prefix string
	label  string
}{
	{"list_services", "SERVICE CATALOG"},
	{"get_service", "SERVICE CATALOG"},
	{"update_service", "SERVICE CATALOG"},
	{"create_service", "SERVICE CATALOG"},
	{"list_docker", "DOCKER SERVICES"},
	{"get_docker", "DOCKER SERVICES"},
	{"restart_docker", "DOCKER SERVICES"},
	{"stop_docker", "DOCKER SERVICES"},
	{"start_docker", "DOCKER SERVICES"},
	{"refresh_docker", "DOCKER SERVICES"},
	{"refresh_all_docker", "DOCKER SERVICES"},
	{"create_docker", "DOCKER SERVICES"},
	{"list_deployments", "DEPLOYMENTS"},
	{"get_deployment", "DEPLOYMENTS"},
	{"list_clusters", "CLUSTERS"},
	{"get_cluster", "CLUSTERS"},
	{"list_pipelines", "PIPELINES & CI/CD"},
	{"get_pipeline", "PIPELINES & CI/CD"},
	{"list_pipeline_runs", "PIPELINES & CI/CD"},
	{"trigger_pipeline", "PIPELINES & CI/CD"},
	{"list_workflows", "WORKFLOWS"},
	{"get_workflow", "WORKFLOWS"},
	{"execute_workflow", "WORKFLOWS"},
	{"list_environments", "ENVIRONMENTS"},
	{"create_environment", "ENVIRONMENTS"},
	{"list_connections", "CONNECTIONS"},
	{"create_connection", "CONNECTIONS"},
	{"list_plugins", "PLUGINS"},
	{"list_entities", "ENTITY GRAPH"},
	{"get_entity", "ENTITY GRAPH"},
	{"create_entity", "ENTITY GRAPH"},
	{"update_entity", "ENTITY GRAPH"},
	{"list_jira", "JIRA"},
}

// buildCategoryCatalog groups tools by category for better small-model accuracy.
// It produces a compact catalog with parameter NAMES only (not full JSON Schema)
// to keep the prompt small enough for local models with limited context windows.
func buildCategoryCatalog(defs []ToolDefinition) string {
	categoryMap := make(map[string][]ToolDefinition)
	uncategorized := make([]ToolDefinition, 0)

	for _, t := range defs {
		found := false
		for _, cat := range toolCategoryOrder {
			if strings.HasPrefix(t.Name, cat.prefix) {
				categoryMap[cat.label] = append(categoryMap[cat.label], t)
				found = true
				break
			}
		}
		if !found {
			uncategorized = append(uncategorized, t)
		}
	}

	var sb strings.Builder
	// Output categories in defined order
	seen := make(map[string]bool)
	for _, cat := range toolCategoryOrder {
		if seen[cat.label] {
			continue
		}
		tools, ok := categoryMap[cat.label]
		if !ok {
			continue
		}
		seen[cat.label] = true
		sb.WriteString(fmt.Sprintf("\n=== %s ===\n", cat.label))
		for _, t := range tools {
			sb.WriteString(fmt.Sprintf("- %s: %s (params: %s)\n", t.Name, t.Description, compactParamNames(t.Parameters)))
		}
	}
	if len(uncategorized) > 0 {
		sb.WriteString("\n=== OTHER ===\n")
		for _, t := range uncategorized {
			sb.WriteString(fmt.Sprintf("- %s: %s (params: %s)\n", t.Name, t.Description, compactParamNames(t.Parameters)))
		}
	}
	return sb.String()
}

// compactParamNames extracts just the parameter names and types from a JSON
// Schema, producing a short string like "name:string, status:string, limit:int".
// This keeps the classification prompt small for local models.
func compactParamNames(schema json.RawMessage) string {
	var obj struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &obj); err != nil || len(obj.Properties) == 0 {
		return "none"
	}
	required := make(map[string]bool, len(obj.Required))
	for _, r := range obj.Required {
		required[r] = true
	}
	parts := make([]string, 0, len(obj.Properties))
	for name, prop := range obj.Properties {
		suffix := ""
		if required[name] {
			suffix = "*"
		}
		parts = append(parts, name+suffix+":"+prop.Type)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// classifiedIntent is the JSON structure the LLM returns during classification.
type classifiedIntent struct {
	ToolName   string                 `json:"tool_name"`
	Params     map[string]interface{} `json:"params"`
	Confidence string                 `json:"confidence"`
	Reason     string                 `json:"reason"`
}

// intentClassification is the full classification response.
// It supports either a single intent or a multi-tool array.
type intentClassification struct {
	Intent *classifiedIntent  `json:"intent"`
	Multi  []classifiedIntent `json:"multi"`
}

// ── Prompts ──────────────────────────────────────────────────────────────────

const intentClassifySystemPrompt = `You classify PEPA user requests into tool calls. Respond with ONLY a JSON object.

TOOLS:
%s

FORMAT: {"intent":{"tool_name":"NAME","params":{},"confidence":"high","reason":"brief"}}
Multiple tools: {"multi":[{"tool_name":"NAME","params":{}}],"confidence":"high","reason":"brief"}
General conversation (greetings, opinions, explanations, follow-ups): {"intent":{"tool_name":"__direct__","params":{},"confidence":"high","reason":"brief"}}
General knowledge (facts, how-to, concepts): {"intent":{"tool_name":"__knowledge__","params":{},"confidence":"high","reason":"brief"}}
Unknown: {"intent":{"tool_name":"unknown","params":{},"confidence":"low","reason":"brief"}}

Rules: pick most specific tool(s), extract params from message, use defaults if missing, respond VALID JSON only.

Examples:
"Show services" → {"intent":{"tool_name":"list_services","params":{},"confidence":"high","reason":"listing services"}}
"List services and clusters" → {"multi":[{"tool_name":"list_services","params":{}},{"tool_name":"list_clusters","params":{}}],"confidence":"high","reason":"listing both"}}
"What is K8s?" → {"intent":{"tool_name":"__knowledge__","params":{},"confidence":"high","reason":"general knowledge"}}
"Restart payment-api" → {"intent":{"tool_name":"restart_docker_service","params":{"service_name":"payment-api"},"confidence":"high","reason":"restarting service"}}
"Покажи сервисы" → {"intent":{"tool_name":"list_services","params":{},"confidence":"high","reason":"список сервисов"}}
"Что такое Kubernetes?" → {"intent":{"tool_name":"__knowledge__","params":{},"confidence":"high","reason":"общий вопрос"}}
"Привет!" → {"intent":{"tool_name":"__direct__","params":{},"confidence":"high","reason":"greeting"}}
"Как дела?" → {"intent":{"tool_name":"__direct__","params":{},"confidence":"high","reason":"conversation"}}
"Что думаешь о микросервисах?" → {"intent":{"tool_name":"__direct__","params":{},"confidence":"high","reason":"opinion"}}
"Расскажи подробнее" → {"intent":{"tool_name":"__direct__","params":{},"confidence":"high","reason":"follow-up"}}`

const responseSynthSystemPrompt = `You are the PEPA AI Assistant — a knowledgeable, friendly platform engineering expert.

Based on the tool results and/or your own knowledge, answer the user's question.

CRITICAL RULES:
1. Answer ONLY based on the actual data provided by tools — NEVER fabricate platform data.
2. If the data is empty or shows "no data found", say so clearly.
3. For general knowledge questions (no tool data), answer from your expertise confidently.
4. Be concise and structured — use bullet points, tables, or sections as appropriate.
5. Highlight key findings and actionable recommendations.
6. If the data contains errors, explain what went wrong and suggest fixes.
7. ALWAYS respond in the SAME LANGUAGE the user used. If the user writes in Russian, respond in Russian. If in English, respond in English.
8. Proactively suggest next steps based on the data.
9. Use code blocks for YAML, JSON, or config examples.
10. If a tool result contains "requires_approval" or "requires your approval", the action was NOT executed. Do NOT claim it succeeded. Do NOT fabricate IDs, names, or any data. Tell the user the action requires their approval and has not been performed yet.`

// ── Main entry point ─────────────────────────────────────────────────────────

// RunOptions holds optional parameters for the intent router.
type RunOptions struct {
	// History is the conversation history for multi-turn context.
	History []Message
	// Stream, if true, returns results via the streaming channel instead of
	// blocking until synthesis completes.
	Stream bool
	// SystemInstruction overrides the default system prompts for classification
	// and synthesis. When set, it replaces both the classifier and synthesizer
	// system prompts with a single custom instruction.
	SystemInstruction string
}

// Run executes the prompt-based agent loop:
//
//	quick-match → classify → validate → execute → synthesize
func (r *IntentRouter) Run(ctx context.Context, userMessage string, opts *RunOptions) (*AgentResponse, error) {
	var history []Message
	var sysInstruction string
	if opts != nil {
		history = opts.History
		sysInstruction = opts.SystemInstruction
	}

	var allResults []AgentActionResult
	var totalTokens TokenUsage
	knowledgeMode := false

	// Step A: Try quick intent match first
	if quick := matchQuickIntent(userMessage); quick != nil {
		intents := []classifiedIntent{*quick}
		log.Printf("[IntentRouter] Quick intent match: %s (reason: %s)", quick.ToolName, quick.Reason)

		// Process the matched intent
		for _, intent := range intents {
			log.Printf("[IntentRouter] Step 0: classified as %s (confidence: %s, reason: %s)", intent.ToolName, intent.Confidence, intent.Reason)
			if intent.ToolName == "__knowledge__" || intent.ToolName == "__direct__" {
				knowledgeMode = true
				continue
			}
			if intent.ToolName == "" || intent.ToolName == "unknown" {
				break
			}
			if !r.toolNames[intent.ToolName] {
				log.Printf("[IntentRouter] Model hallucinated tool name: %s — skipping", intent.ToolName)
				allResults = append(allResults, AgentActionResult{
					ToolName: intent.ToolName, ToolArgs: mustMarshalJSON(intent.Params),
					Error: "This tool does not exist.", Policy: "forbidden", Timestamp: time.Now(),
				})
				continue
			}
			paramsJSON := mustMarshalJSON(intent.Params)
			level, policyErr := r.policy.Check(ctx, intent.ToolName)
			if policyErr != nil {
				allResults = append(allResults, AgentActionResult{
					ToolName: intent.ToolName, ToolArgs: paramsJSON,
					Error: policyErr.Error(), Policy: string(level), Timestamp: time.Now(),
				})
				if level == LevelForbidden {
					break
				}
				if level == LevelApprove {
					return &AgentResponse{
						Answer:    fmt.Sprintf("The action %q requires your approval.", intent.ToolName),
						ToolCalls: allResults,
						NeedsApproval: &ApprovalRequest{
							ToolName: intent.ToolName, ToolArgs: paramsJSON,
							Description: fmt.Sprintf("The action %q requires approval.", intent.ToolName),
							Reason:      "This action modifies platform state.",
						},
						TokensUsed: totalTokens,
					}, nil
				}
				continue
			}
			log.Printf("[IntentRouter] Step 0: executing tool %s with params: %s", intent.ToolName, string(paramsJSON))
			toolResult, execErr := r.tools.Execute(ctx, intent.ToolName, paramsJSON)
			result := AgentActionResult{
				ToolName: intent.ToolName, ToolArgs: paramsJSON,
				Result: toolResult, Policy: string(level), Timestamp: time.Now(),
			}
			if execErr != nil {
				result.Error = execErr.Error()
				log.Printf("[IntentRouter] Tool %s failed: %v", intent.ToolName, execErr)
			} else {
				log.Printf("[IntentRouter] Tool %s returned %d bytes", intent.ToolName, len(toolResult))
			}
			allResults = append(allResults, result)
		}
	} else {
		// Step B: Classify via LLM
		intents, classifyTokens, err := r.classifyWithRetry(ctx, userMessage, nil, 0, history, sysInstruction)
		totalTokens.InputTokens += classifyTokens.InputTokens
		totalTokens.OutputTokens += classifyTokens.OutputTokens
		totalTokens.TotalTokens += classifyTokens.TotalTokens

		if err != nil {
			log.Printf("[IntentRouter] Classification failed: %v", err)
			return &AgentResponse{
				Answer:     fmt.Sprintf("I couldn't determine what you're asking for. Please try rephrasing your question. Error: %v", err),
				ToolCalls:  allResults,
				TokensUsed: totalTokens,
			}, nil
		} else {
			// Process all intents from classification
			for _, intent := range intents {
				log.Printf("[IntentRouter] Step 0: classified as %s (confidence: %s, reason: %s)", intent.ToolName, intent.Confidence, intent.Reason)

				if intent.ToolName == "__knowledge__" || intent.ToolName == "__direct__" {
					knowledgeMode = true
					log.Printf("[IntentRouter] Knowledge/direct mode: will synthesize from LLM knowledge")
					continue
				}

				if intent.ToolName == "" || intent.ToolName == "unknown" {
					if len(allResults) > 0 || knowledgeMode {
						break
					}
					// Fall back to direct LLM response instead of error
					knowledgeMode = true
					break
				}

				if !r.toolNames[intent.ToolName] {
					log.Printf("[IntentRouter] Model hallucinated tool name: %s — skipping", intent.ToolName)
					allResults = append(allResults, AgentActionResult{
						ToolName:  intent.ToolName,
						ToolArgs:  mustMarshalJSON(intent.Params),
						Error:     "This tool does not exist.",
						Policy:    "forbidden",
						Timestamp: time.Now(),
					})
					continue
				}

				paramsJSON := mustMarshalJSON(intent.Params)
				level, policyErr := r.policy.Check(ctx, intent.ToolName)
				if policyErr != nil {
					result := AgentActionResult{
						ToolName: intent.ToolName, ToolArgs: paramsJSON,
						Error: policyErr.Error(), Policy: string(level), Timestamp: time.Now(),
					}
					allResults = append(allResults, result)
					if level == LevelForbidden {
						break
					}
					if level == LevelApprove {
						return &AgentResponse{
							Answer:    fmt.Sprintf("The action %q requires your approval.", intent.ToolName),
							ToolCalls: allResults,
							NeedsApproval: &ApprovalRequest{
								ToolName: intent.ToolName, ToolArgs: paramsJSON,
								Description: fmt.Sprintf("The action %q requires approval.", intent.ToolName),
								Reason:      "This action modifies platform state.",
							},
							TokensUsed: totalTokens,
						}, nil
					}
					continue
				}

				log.Printf("[IntentRouter] Step 0: executing tool %s with params: %s", intent.ToolName, string(paramsJSON))
				toolResult, execErr := r.tools.Execute(ctx, intent.ToolName, paramsJSON)

				result := AgentActionResult{
					ToolName: intent.ToolName, ToolArgs: paramsJSON,
					Result: toolResult, Policy: string(level), Timestamp: time.Now(),
				}
				if execErr != nil {
					result.Error = execErr.Error()
					log.Printf("[IntentRouter] Tool %s failed: %v", intent.ToolName, execErr)
				} else {
					log.Printf("[IntentRouter] Tool %s returned %d bytes", intent.ToolName, len(toolResult))
				}
				allResults = append(allResults, result)
			}
		}
	}

	// Step E: Synthesize final response
	return r.synthesize(ctx, userMessage, allResults, totalTokens, history, sysInstruction, knowledgeMode)
}

// RunStream executes the agent loop and streams the synthesis response.
// Tool execution results are sent as "tool_result" chunks, and the final
// synthesis is streamed as "text" chunks.
func (r *IntentRouter) RunStream(ctx context.Context, userMessage string, opts *RunOptions) (<-chan *StreamChunk, error) {
	var history []Message
	var sysInstruction string
	if opts != nil {
		history = opts.History
		sysInstruction = opts.SystemInstruction
	}

	ch := make(chan *StreamChunk, 32)

	go func() {
		defer close(ch)
		defer func() {
			if rv := recover(); rv != nil {
				log.Printf("[IntentRouter] PANIC in RunStream goroutine: %v", rv)
				ch <- &StreamChunk{Type: "text", Content: fmt.Sprintf("Internal error: %v", rv)}
			}
		}()

		var allResults []AgentActionResult
		var totalTokens TokenUsage
		knowledgeMode := false

		// Step A: Try quick intent match
		var intents []classifiedIntent
		if quick := matchQuickIntent(userMessage); quick != nil {
			intents = []classifiedIntent{*quick}
		}

		// Step B: LLM classification (if no quick match)
		if intents == nil {
			var classifyTokens TokenUsage
			var err error
			intents, classifyTokens, err = r.classifyWithRetry(ctx, userMessage, nil, 0, history, sysInstruction)
			totalTokens.InputTokens += classifyTokens.InputTokens
			totalTokens.OutputTokens += classifyTokens.OutputTokens
			totalTokens.TotalTokens += classifyTokens.TotalTokens
			if err != nil {
				log.Printf("[IntentRouter] Classification failed, falling back to direct response: %v", err)
				knowledgeMode = true
				intents = nil // ensure we skip intent processing
			}
		}

		// Process all intents with parallel tool execution
		if intents != nil {
			type streamOutcome struct {
				result    AgentActionResult
				toolChunk *StreamChunk
				callChunk *StreamChunk
			}
			outcomes := make([]streamOutcome, len(intents))

			var wg sync.WaitGroup
			for i, intent := range intents {
				wg.Add(1)
				go func(idx int, intent classifiedIntent) {
					defer wg.Done()

					if intent.ToolName == "__knowledge__" || intent.ToolName == "__direct__" {
						knowledgeMode = true
						return
					}

					if intent.ToolName == "" || intent.ToolName == "unknown" {
						// Fall back to direct LLM response
						knowledgeMode = true
						return
					}

					if !r.toolNames[intent.ToolName] {
						outcomes[idx] = streamOutcome{
							callChunk: &StreamChunk{Type: "text", Content: fmt.Sprintf("Tool %q does not exist, skipping.", intent.ToolName)},
						}
						return
					}

					paramsJSON := mustMarshalJSON(intent.Params)
					level, policyErr := r.policy.Check(ctx, intent.ToolName)
					if policyErr != nil {
						if level == LevelForbidden {
							outcomes[idx] = streamOutcome{
								callChunk: &StreamChunk{Type: "text", Content: fmt.Sprintf("Tool %q is forbidden by policy.", intent.ToolName)},
							}
							return
						}
						if level == LevelApprove {
							blockedResult := AgentActionResult{
								ToolName: intent.ToolName, ToolArgs: paramsJSON,
								Error: "requires_approval", Policy: string(level), Timestamp: time.Now(),
							}
							outcomes[idx] = streamOutcome{
								result: blockedResult,
								callChunk: &StreamChunk{
									Type:     "text",
									Content:  fmt.Sprintf("The action %q requires your approval.", intent.ToolName),
									Metadata: map[string]interface{}{"needs_approval": true, "tool_name": intent.ToolName},
								},
							}
							return
						}
					}

					callChunk := &StreamChunk{
						Type:     "tool_call",
						Metadata: map[string]interface{}{"tool_name": intent.ToolName, "params": intent.Params},
					}

					toolResult, execErr := r.tools.Execute(ctx, intent.ToolName, paramsJSON)

					resultChunk := &StreamChunk{
						Type:       "tool_result",
						ToolResult: &ToolResult{Content: toolResult},
						Metadata:   map[string]interface{}{"tool_name": intent.ToolName, "policy": string(level)},
					}
					if execErr != nil {
						resultChunk.ToolResult.Error = execErr.Error()
					}

					outcomes[idx] = streamOutcome{
						result: AgentActionResult{
							ToolName: intent.ToolName, ToolArgs: paramsJSON,
							Result: toolResult, Policy: string(level), Timestamp: time.Now(),
						},
						toolChunk: resultChunk,
						callChunk: callChunk,
					}
				}(i, intent)
			}
			wg.Wait()

			// Emit chunks and collect results in order
			for _, o := range outcomes {
				if o.callChunk != nil {
					sendChunk(ch, ctx, o.callChunk)
				}
				if o.toolChunk != nil {
					sendChunk(ch, ctx, o.toolChunk)
				}
				if o.result.ToolName != "" {
					allResults = append(allResults, o.result)
				}
			}
		} else {
			// Check if any intent was __knowledge__
			// (intents is nil only on classification error; knowledgeMode may still be false)
		}

		// Stream synthesis
		if len(allResults) > 0 || knowledgeMode {
			r.synthesizeStream(ctx, userMessage, allResults, history, sysInstruction, knowledgeMode, ch)
		} else if intents == nil {
			// Classification failed and no results — error already sent above
		} else {
			sendChunk(ch, ctx, &StreamChunk{Type: "text", Content: "I couldn't retrieve any data to answer your question."})
		}
	}()

	return ch, nil
}

// sendChunk sends a chunk to the stream channel, respecting context cancellation.
func sendChunk(ch chan<- *StreamChunk, ctx context.Context, chunk *StreamChunk) {
	select {
	case ch <- chunk:
	case <-ctx.Done():
	}
}

// classifyWithRetry tries to classify the user's intent, retrying once on parse
// failure with a stronger prompt. Returns a slice of intents for multi-tool support.
func (r *IntentRouter) classifyWithRetry(ctx context.Context, userMessage string, prevResults []AgentActionResult, step int, history []Message, systemInstruction string) ([]classifiedIntent, TokenUsage, error) {
	intents, tokens, err := r.classify(ctx, userMessage, prevResults, step, history, systemInstruction)
	if err == nil {
		return intents, tokens, nil
	}

	log.Printf("[IntentRouter] Classification parse failed, retrying with correction prompt")
	failedInput := tokens.InputTokens
	failedOutput := tokens.OutputTokens
	failedTotal := tokens.TotalTokens

	correctedMsg := userMessage + "\n\nIMPORTANT: Respond with ONLY a valid JSON object. No text before or after the JSON."
	if step > 0 && len(prevResults) > 0 {
		correctedMsg = fmt.Sprintf("Original request: %s\n\nPrevious results collected:\n%s\n\nIMPORTANT: Respond with ONLY a valid JSON object. No text before or after the JSON.", userMessage, formatResultsBrief(prevResults))
	}

	intents, retryTokens, retryErr := r.classify(ctx, correctedMsg, nil, 0, nil, systemInstruction)
	tokens.InputTokens = failedInput + retryTokens.InputTokens
	tokens.OutputTokens = failedOutput + retryTokens.OutputTokens
	tokens.TotalTokens = failedTotal + retryTokens.TotalTokens

	if retryErr != nil {
		return nil, tokens, fmt.Errorf("classification failed after retry: %w", retryErr)
	}
	return intents, tokens, nil
}

// ── Classification ───────────────────────────────────────────────────────────

func (r *IntentRouter) classify(ctx context.Context, userMessage string, prevResults []AgentActionResult, step int, history []Message, systemInstruction string) ([]classifiedIntent, TokenUsage, error) {
	systemPrompt := fmt.Sprintf(intentClassifySystemPrompt, r.toolCatalog)
	if systemInstruction != "" {
		systemPrompt += "\n\nADDITIONAL USER GUIDANCE:\n" + systemInstruction
	}

	messages := []Message{
		{Role: "system", Content: systemPrompt},
	}

	if len(history) > 0 {
		// Limit to last 2 messages to save tokens (classification needs minimal context)
		start := 0
		if len(history) > 2 {
			start = len(history) - 2
		}
		for _, h := range history[start:] {
			if h.Role == "user" || h.Role == "assistant" {
				messages = append(messages, h)
			}
		}
	}

	if step > 0 && len(prevResults) > 0 {
		messages = append(messages, Message{
			Role:    "user",
			Content: fmt.Sprintf("Original request: %s\n\nResults so far:\n%s\n\nBased on the original request and results above, what is the next tool to call? If no more tools are needed, respond with {\"intent\":{\"tool_name\":\"\",\"params\":{},\"confidence\":\"high\",\"reason\":\"data collection complete\"}}", userMessage, formatResultsBrief(prevResults)),
		})
	} else {
		messages = append(messages, Message{
			Role:    "user",
			Content: userMessage,
		})
	}

	resp, err := r.provider.Chat(ctx, messages, &ChatOptions{
		MaxTokens:   1024,
		Temperature: 0.1,
	})
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "413") || strings.Contains(errMsg, "too large") || strings.Contains(errMsg, "rate_limit") {
			return nil, TokenUsage{}, fmt.Errorf("request too large for this model. Try starting a new chat or use a model with higher limits")
		}
		if strings.Contains(errMsg, "404") {
			return nil, TokenUsage{}, fmt.Errorf("model or endpoint not found. Check your AI provider settings in Connections")
		}
		return nil, TokenUsage{}, fmt.Errorf("classification LLM call failed: %w", err)
	}

	// Try to parse JSON from the response content (supports single and multi-tool)
	intents, err := r.parseClassification(resp.Content)
	if err != nil {
		if len(resp.ToolCalls) > 0 {
			result := make([]classifiedIntent, 0, len(resp.ToolCalls))
			for _, tc := range resp.ToolCalls {
				var params map[string]interface{}
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &params)
				result = append(result, classifiedIntent{
					ToolName:   tc.Function.Name,
					Params:     params,
					Confidence: "high",
					Reason:     "extracted from native tool_call",
				})
			}
			return result, resp.TokensUsed, nil
		}
		log.Printf("[IntentRouter] Failed to parse classification: %v (content: %s)", err, truncate(resp.Content, 200))
		return nil, resp.TokensUsed, fmt.Errorf("failed to parse intent from model response: %w", err)
	}

	return intents, resp.TokensUsed, nil
}

// parseClassification extracts intent(s) from the model's response.
// Supports both single {"intent": ...} and multi-tool {"multi": [...]} formats.
func (r *IntentRouter) parseClassification(content string) ([]classifiedIntent, error) {
	content = strings.TrimSpace(content)

	// Try direct parse first
	var classification intentClassification
	if err := json.Unmarshal([]byte(content), &classification); err == nil {
		// Multi-tool format
		if len(classification.Multi) > 0 {
			return classification.Multi, nil
		}
		// Single intent format
		if classification.Intent != nil {
			return []classifiedIntent{*classification.Intent}, nil
		}
	}

	// Try to find JSON block in the response
	jsonStr := extractJSON(content)
	if jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), &classification); err == nil {
			if len(classification.Multi) > 0 {
				return classification.Multi, nil
			}
			if classification.Intent != nil {
				return []classifiedIntent{*classification.Intent}, nil
			}
		}
	}

	// Try parsing as a bare intent object
	var intent classifiedIntent
	if err := json.Unmarshal([]byte(content), &intent); err == nil && intent.ToolName != "" {
		return []classifiedIntent{intent}, nil
	}

	return nil, fmt.Errorf("no valid intent JSON found in response")
}

// ── Synthesis ────────────────────────────────────────────────────────────────

func (r *IntentRouter) synthesize(ctx context.Context, userMessage string, results []AgentActionResult, accumulatedTokens TokenUsage, history []Message, systemInstruction string, knowledgeMode bool) (*AgentResponse, error) {
	if len(results) == 0 && !knowledgeMode {
		// No tool results and not knowledge mode — try a direct LLM response as fallback
		knowledgeMode = true
	}

	messages := r.buildSynthesisMessages(userMessage, results, history, systemInstruction, knowledgeMode)

	resp, err := r.provider.Chat(ctx, messages, &ChatOptions{
		MaxTokens:   4096,
		Temperature: 0.3,
	})
	if err != nil {
		return nil, fmt.Errorf("synthesis LLM call failed: %w", err)
	}

	accumulatedTokens.InputTokens += resp.TokensUsed.InputTokens
	accumulatedTokens.OutputTokens += resp.TokensUsed.OutputTokens
	accumulatedTokens.TotalTokens += resp.TokensUsed.TotalTokens

	return &AgentResponse{
		Answer:     StripThinkBlocks(resp.Content),
		ToolCalls:  results,
		ModelUsed:  resp.ModelUsed,
		TokensUsed: accumulatedTokens,
	}, nil
}

// synthesizeStream performs streaming synthesis via SSE.
func (r *IntentRouter) synthesizeStream(ctx context.Context, userMessage string, results []AgentActionResult, history []Message, systemInstruction string, knowledgeMode bool, ch chan<- *StreamChunk) {
	messages := r.buildSynthesisMessages(userMessage, results, history, systemInstruction, knowledgeMode)

	stream, err := r.provider.Stream(ctx, messages, &ChatOptions{
		MaxTokens:   4096,
		Temperature: 0.3,
	})
	if err != nil {
		sendChunk(ch, ctx, &StreamChunk{Type: "text", Content: fmt.Sprintf("Synthesis failed: %v", err)})
		return
	}

	for chunk := range stream {
		sendChunk(ch, ctx, chunk)
	}
}

// buildSynthesisMessages constructs the message slice for synthesis.
func (r *IntentRouter) buildSynthesisMessages(userMessage string, results []AgentActionResult, history []Message, systemInstruction string, knowledgeMode bool) []Message {
	synthPrompt := responseSynthSystemPrompt
	if systemInstruction != "" {
		synthPrompt += "\n\nADDITIONAL USER GUIDANCE:\n" + systemInstruction
	}

	messages := []Message{
		{Role: "system", Content: synthPrompt},
	}
	if len(history) > 0 {
		// Limit to last 6 messages to save tokens
		start := 0
		if len(history) > 6 {
			start = len(history) - 6
		}
		for _, h := range history[start:] {
			if h.Role == "user" || h.Role == "assistant" {
				messages = append(messages, h)
			}
		}
	}

	// In knowledge mode, no tool data — just answer from LLM knowledge.
	if knowledgeMode && len(results) == 0 {
		messages = append(messages, Message{
			Role:    "user",
			Content: userMessage,
		})
		return messages
	}

	contextParts := make([]string, 0, len(results))
	for _, res := range results {
		if res.Error != "" {
			contextParts = append(contextParts, fmt.Sprintf("[Tool: %s] Error: %s", res.ToolName, res.Error))
		} else {
			preview := res.Result
			if len(preview) > 2000 {
				preview = preview[:2000] + "\n... (data truncated for brevity)"
			}
			contextParts = append(contextParts, fmt.Sprintf("[Tool: %s] Result:\n%s", res.ToolName, preview))
		}
	}

	userContent := fmt.Sprintf("User's question: %s\n\nData from tools:\n%s", userMessage, strings.Join(contextParts, "\n\n"))
	if knowledgeMode {
		userContent += "\n\nNote: The user also asked a general knowledge question. Answer from your expertise where appropriate, combining with the tool data above."
	}

	messages = append(messages, Message{
		Role:    "user",
		Content: userContent,
	})
	return messages
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func formatResultsBrief(results []AgentActionResult) string {
	parts := make([]string, 0, len(results))
	for _, res := range results {
		if res.Error != "" {
			parts = append(parts, fmt.Sprintf("[Tool: %s] Error: %s", res.ToolName, res.Error))
		} else {
			preview := res.Result
			if len(preview) > 500 {
				preview = preview[:500] + "... (truncated)"
			}
			parts = append(parts, fmt.Sprintf("[Tool: %s] %s", res.ToolName, preview))
		}
	}
	return strings.Join(parts, "\n")
}

// extractJSON tries to find a JSON object in arbitrary text.
func extractJSON(text string) string {
	// First try to find JSON wrapped in markdown code blocks
	if idx := strings.Index(text, "```json"); idx != -1 {
		start := idx + 7
		if end := strings.Index(text[start:], "```"); end != -1 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	if idx := strings.Index(text, "```"); idx != -1 {
		start := idx + 3
		if end := strings.Index(text[start:], "```"); end != -1 {
			candidate := strings.TrimSpace(text[start : start+end])
			if strings.HasPrefix(candidate, "{") {
				return candidate
			}
		}
	}

	// Find the first { and try to extract a balanced JSON object
	start := strings.Index(text, "{")
	if start == -1 {
		return ""
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		c := text[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// mustMarshalJSON marshals v to JSON, returning nil on error.
func mustMarshalJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
