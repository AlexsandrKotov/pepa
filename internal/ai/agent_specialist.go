package ai

import (
	"context"
	"fmt"
	"log/slog"
)

// SpecialistType represents the domain of a specialist agent.
type SpecialistType string

const (
	SpecialistSRE     SpecialistType = "sre"
	SpecialistDevOps  SpecialistType = "devops"
	SpecialistSecurity SpecialistType = "security"
	SpecialistDoc     SpecialistType = "doc"
	SpecialistCost    SpecialistType = "cost"
	SpecialistGeneral SpecialistType = "general"
)

// SpecialistAgent is an agent specialized in a specific domain.
type SpecialistAgent struct {
	name         string
	specialist   SpecialistType
	agent        *Agent
	systemPrompt string
	tools        *ToolRegistry
}

// NewSpecialistAgent creates a new specialist agent.
func NewSpecialistAgent(name string, specialist SpecialistType, provider LLMProvider, baseTools *ToolRegistry, mode AgentMode) *SpecialistAgent {
	// Create a filtered tool registry with only relevant tools
	filteredTools := filterToolsForSpecialist(baseTools, specialist)

	agent := NewAgent(provider, filteredTools, NewAgentPolicy(), mode)

	systemPrompt := systemPromptForSpecialist(specialist)
	agent.SetSystemInstruction(systemPrompt)

	return &SpecialistAgent{
		name:         name,
		specialist:   specialist,
		agent:        agent,
		systemPrompt: systemPrompt,
		tools:        filteredTools,
	}
}

// Chat sends a message to the specialist agent.
func (s *SpecialistAgent) Chat(ctx context.Context, messages []Message) (*AgentResponse, error) {
	return s.agent.Run(ctx, nil, messages[len(messages)-1].Content)
}

// Stream sends a streaming message to the specialist agent.
func (s *SpecialistAgent) Stream(ctx context.Context, messages []Message) (<-chan *StreamChunk, error) {
	return s.agent.Stream(ctx, nil, messages[len(messages)-1].Content)
}

// Name returns the specialist agent's name.
func (s *SpecialistAgent) Name() string { return s.name }

// Specialist returns the specialist type.
func (s *SpecialistAgent) Specialist() SpecialistType { return s.specialist }

// systemPromptForSpecialist returns the system prompt for a given specialist.
func systemPromptForSpecialist(specialist SpecialistType) string {
	switch specialist {
	case SpecialistSRE:
		return `You are an SRE (Site Reliability Engineering) specialist for the PEPA platform.
Your expertise includes:
- Monitoring, metrics, and observability (Prometheus, Grafana)
- Incident response and post-mortem analysis
- Service health, SLIs/SLOs, and error budgets
- Log analysis and debugging
- Performance optimization and capacity planning
- Topology and service dependency analysis

When answering questions, focus on reliability, observability, and operational excellence.
Use the available tools to query real platform data about services, deployments, and metrics.`

	case SpecialistDevOps:
		return `You are a DevOps specialist for the PEPA platform.
Your expertise includes:
- CI/CD pipelines and deployment strategies
- Kubernetes cluster management and GitOps
- Helm charts, Docker, and container orchestration
- Infrastructure as Code (Terraform, Ansible)
- Environment management and configuration
- Pipeline optimization and automation

When answering questions, focus on delivery, automation, and infrastructure.
Use the available tools to query real platform data about pipelines, deployments, and clusters.`

	case SpecialistSecurity:
		return `You are a Security specialist for the PEPA platform.
Your expertise includes:
- Vulnerability assessment and scanning (Trivy)
- RBAC and access control
- Secret management (Vault)
- Compliance and audit trails
- Network policies and security best practices
- Certificate management

When answering questions, focus on security posture, compliance, and risk mitigation.
Use the available tools to query real platform data about security configurations.`

	case SpecialistDoc:
		return `You are a Documentation specialist for the PEPA platform.
Your expertise includes:
- Technical writing and documentation generation
- Knowledge base management (RAG)
- Service documentation and runbooks
- Architecture diagrams and API documentation
- Onboarding guides and tutorials

When answering questions, focus on clarity, completeness, and actionability.
Use the knowledge base (RAG) to provide accurate, cited information.`

	case SpecialistCost:
		return `You are a Cost Optimization specialist for the PEPA platform.
Your expertise includes:
- Cloud cost analysis and optimization
- Resource right-sizing
- Identifying idle and underutilized resources
- Budget planning and forecasting
- Compute, storage, and network cost optimization

When answering questions, focus on cost efficiency and resource optimization.
Use the available tools to query real platform data about services and resource usage.`

	default:
		return `You are a general-purpose AI assistant for the PEPA platform engineering system.
Help users with any platform-related question using the available tools and knowledge base.`
	}
}

// filterToolsForSpecialist returns a tool registry filtered for the specialist's domain.
func filterToolsForSpecialist(base *ToolRegistry, specialist SpecialistType) *ToolRegistry {
	filtered := NewToolRegistry()

	// Define which tool categories each specialist gets
	allowedPrefixes := map[SpecialistType][]string{
		SpecialistSRE:     {"get_services", "get_deployments", "get_pipeline", "get_entities", "get_docker"},
		SpecialistDevOps:  {"get_services", "get_deployments", "get_pipeline", "get_clusters", "get_environments", "get_helm", "get_docker", "get_gitops"},
		SpecialistSecurity: {"get_services", "get_entities", "get_vault", "get_audit", "get_roles"},
		SpecialistDoc:     {"get_services", "get_entities", "get_pipeline", "search_knowledge"},
		SpecialistCost:    {"get_services", "get_clusters", "get_docker", "get_environments"},
		SpecialistGeneral: {}, // empty = all tools
	}

	allowed, isFiltered := allowedPrefixes[specialist]

	// List() returns []ToolDefinition, but we need the actual Tool objects.
	// Use the internal tools map via a different approach.
	base.mu.RLock()
	for _, tool := range base.tools {
		def := tool.Definition()
		if !isFiltered {
			filtered.Register(tool)
			continue
		}
		for _, prefix := range allowed {
			if len(def.Name) >= len(prefix) && def.Name[:len(prefix)] == prefix {
				filtered.Register(tool)
				break
			}
		}
	}
	base.mu.RUnlock()

	return filtered
}

// SpecialistRegistry manages all specialist agents.
type SpecialistRegistry struct {
	specialists map[SpecialistType]*SpecialistAgent
}

// NewSpecialistRegistry creates a new specialist registry.
func NewSpecialistRegistry() *SpecialistRegistry {
	return &SpecialistRegistry{
		specialists: make(map[SpecialistType]*SpecialistAgent),
	}
}

// Register adds a specialist agent to the registry.
func (r *SpecialistRegistry) Register(agent *SpecialistAgent) {
	r.specialists[agent.specialist] = agent
	slog.Info("specialist agent registered", "name", agent.name, "type", agent.specialist)
}

// Get returns a specialist agent by type.
func (r *SpecialistRegistry) Get(specialist SpecialistType) (*SpecialistAgent, bool) {
	agent, ok := r.specialists[specialist]
	return agent, ok
}

// List returns all registered specialist types.
func (r *SpecialistRegistry) List() []SpecialistType {
	types := make([]SpecialistType, 0, len(r.specialists))
	for t := range r.specialists {
		types = append(types, t)
	}
	return types
}

// InitSpecialists creates all specialist agents using the given provider.
func InitSpecialists(provider LLMProvider, tools *ToolRegistry, mode AgentMode) *SpecialistRegistry {
	registry := NewSpecialistRegistry()

	specialists := []struct {
		name string
		typ  SpecialistType
	}{
		{"SRE Agent", SpecialistSRE},
		{"DevOps Agent", SpecialistDevOps},
		{"Security Agent", SpecialistSecurity},
		{"Doc Agent", SpecialistDoc},
		{"Cost Agent", SpecialistCost},
		{"General Agent", SpecialistGeneral},
	}

	for _, s := range specialists {
		agent := NewSpecialistAgent(s.name, s.typ, provider, tools, mode)
		registry.Register(agent)
	}

	slog.Info("specialist agents initialized", "count", len(specialists))
	return registry
}

// ClassifyIntent determines which specialist should handle a query.
func ClassifyIntent(ctx context.Context, provider LLMProvider, query string) (SpecialistType, error) {
	prompt := fmt.Sprintf(`Classify the following user query into exactly one specialist category.

Query: %s

Categories:
- sre: Questions about monitoring, incidents, health, metrics, logs, debugging, performance, SLIs/SLOs
- devops: Questions about deployments, pipelines, CI/CD, Kubernetes, Docker, Helm, GitOps, infrastructure, environments
- security: Questions about vulnerabilities, RBAC, secrets, compliance, audit, access control
- doc: Questions about documentation, how-to guides, service information, knowledge base
- cost: Questions about costs, resource optimization, budget, right-sizing, idle resources
- general: Anything that doesn't fit the above categories

Respond with ONLY the category name (sre, devops, security, doc, cost, or general). No explanation.`, query)

	messages := []Message{
		{Role: "system", Content: "You are an intent classifier. Respond with ONLY a single word: sre, devops, security, doc, cost, or general."},
		{Role: "user", Content: prompt},
	}

	resp, err := provider.Chat(ctx, messages, &ChatOptions{MaxTokens: 20})
	if err != nil {
		return SpecialistGeneral, fmt.Errorf("intent classification failed: %w", err)
	}

	// Parse the response
	result := SpecialistType(trimSpace(resp.Content))
	switch result {
	case SpecialistSRE, SpecialistDevOps, SpecialistSecurity, SpecialistDoc, SpecialistCost, SpecialistGeneral:
		slog.Debug("intent classified", "query", query[:min(50, len(query))], "specialist", result)
		return result, nil
	default:
		slog.Debug("intent classification returned unknown type, defaulting to general", "response", resp.Content)
		return SpecialistGeneral, nil
	}
}

// trimSpace trims leading/trailing whitespace.
func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\r' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
