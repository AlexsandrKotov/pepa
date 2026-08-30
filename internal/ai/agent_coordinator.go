package ai

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// CoordinatorResult holds the aggregated result from multiple specialist agents.
type CoordinatorResult struct {
	Query       string                    `json:"query"`
	Primary     string                    `json:"primary_specialist"`
	Responses   []SpecialistResponse      `json:"responses"`
	Synthesized string                    `json:"synthesized_answer"`
	Duration    time.Duration             `json:"duration_ms"`
}

// SpecialistResponse holds a single specialist's response.
type SpecialistResponse struct {
	Specialist SpecialistType `json:"specialist"`
	Answer     string         `json:"answer"`
	ToolCalls  int            `json:"tool_calls"`
	Error      string         `json:"error,omitempty"`
}

// AgentCoordinator coordinates multiple specialist agents for complex queries.
type AgentCoordinator struct {
	registry *SpecialistRegistry
	provider LLMProvider
}

// NewAgentCoordinator creates a new agent coordinator.
func NewAgentCoordinator(registry *SpecialistRegistry, provider LLMProvider) *AgentCoordinator {
	return &AgentCoordinator{
		registry: registry,
		provider: provider,
	}
}

// Route sends a query to the most appropriate specialist agent.
func (c *AgentCoordinator) Route(ctx context.Context, query string) (*CoordinatorResult, error) {
	start := time.Now()

	// Classify the intent
	specialist, err := ClassifyIntent(ctx, c.provider, query)
	if err != nil {
		slog.Warn("intent classification failed, using general agent", "error", err)
		specialist = SpecialistGeneral
	}

	agent, ok := c.registry.Get(specialist)
	if !ok {
		// Fall back to general agent
		agent, ok = c.registry.Get(SpecialistGeneral)
		if !ok {
			return nil, fmt.Errorf("no specialist agents available")
		}
		specialist = SpecialistGeneral
	}

	messages := []Message{
		{Role: "user", Content: query},
	}

	resp, err := agent.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("specialist %s failed: %w", specialist, err)
	}

	toolCalls := 0
	if resp != nil {
		toolCalls = len(resp.ToolCalls)
	}

	result := &CoordinatorResult{
		Query:       query,
		Primary:     string(specialist),
		Synthesized: resp.Answer,
		Duration:    time.Since(start),
		Responses: []SpecialistResponse{
			{
				Specialist: specialist,
				Answer:     resp.Answer,
				ToolCalls:  toolCalls,
			},
		},
	}

	slog.Info("query routed to specialist", "specialist", specialist, "duration", time.Since(start))
	return result, nil
}

// Coordinate sends a query to multiple specialists in parallel and synthesizes results.
// Used for complex, multi-domain queries.
func (c *AgentCoordinator) Coordinate(ctx context.Context, query string, specialists []SpecialistType) (*CoordinatorResult, error) {
	start := time.Now()

	if len(specialists) == 0 {
		// Auto-detect which specialists are needed
		specialists = c.detectNeededSpecialists(ctx, query)
	}

	if len(specialists) == 1 {
		// Single specialist is enough — just route
		return c.Route(ctx, query)
	}

	// Run specialists in parallel
	var mu sync.Mutex
	var wg sync.WaitGroup
	responses := make([]SpecialistResponse, 0, len(specialists))

	for _, spec := range specialists {
		agent, ok := c.registry.Get(spec)
		if !ok {
			continue
		}

		wg.Add(1)
		go func(s SpecialistType, a *SpecialistAgent) {
			defer wg.Done()

			messages := []Message{
				{Role: "user", Content: query},
			}

			resp, err := a.Chat(ctx, messages)
			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				responses = append(responses, SpecialistResponse{
					Specialist: s,
					Error:      err.Error(),
				})
				return
			}

			toolCalls := 0
			if resp != nil {
				toolCalls = len(resp.ToolCalls)
			}

			responses = append(responses, SpecialistResponse{
				Specialist: s,
				Answer:     resp.Answer,
				ToolCalls:  toolCalls,
			})
		}(spec, agent)
	}

	wg.Wait()

	// Synthesize results from all specialists
	synthesized, err := c.synthesize(ctx, query, responses)
	if err != nil {
		slog.Warn("synthesis failed, returning raw responses", "error", err)
		synthesized = c.simpleSynthesize(responses)
	}

	primary := ""
	if len(specialists) > 0 {
		primary = string(specialists[0])
	}

	result := &CoordinatorResult{
		Query:       query,
		Primary:     primary,
		Responses:   responses,
		Synthesized: synthesized,
		Duration:    time.Since(start),
	}

	slog.Info("multi-agent coordination complete",
		"specialists", len(specialists),
		"responses", len(responses),
		"duration", time.Since(start))

	return result, nil
}

// detectNeededSpecialists analyzes a query to determine which specialists are needed.
func (c *AgentCoordinator) detectNeededSpecialists(ctx context.Context, query string) []SpecialistType {
	prompt := fmt.Sprintf(`Analyze this query and determine which specialist domains are relevant.

Query: %s

Available specialists: sre, devops, security, doc, cost

Respond with a comma-separated list of relevant specialists. Example: "sre,devops"
If only one specialist is needed, respond with just that one.`, query)

	messages := []Message{
		{Role: "system", Content: "You determine which AI specialists are needed for a query. Respond with ONLY a comma-separated list."},
		{Role: "user", Content: prompt},
	}

	resp, err := c.provider.Chat(ctx, messages, &ChatOptions{MaxTokens: 50})
	if err != nil {
		return []SpecialistType{SpecialistGeneral}
	}

	parts := strings.Split(resp.Content, ",")
	var result []SpecialistType
	seen := make(map[SpecialistType]bool)

	for _, p := range parts {
		// Strip non-alphabetic characters (newlines, extra text, etc.)
		p = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				return r
			}
			return -1
		}, p)
		p = strings.ToLower(strings.TrimSpace(p))
		switch SpecialistType(p) {
		case SpecialistSRE, SpecialistDevOps, SpecialistSecurity, SpecialistDoc, SpecialistCost:
			if !seen[SpecialistType(p)] {
				result = append(result, SpecialistType(p))
				seen[SpecialistType(p)] = true
			}
		}
	}

	if len(result) == 0 {
		result = []SpecialistType{SpecialistGeneral}
	}

	return result
}

// synthesize uses the LLM to combine multiple specialist responses into one answer.
func (c *AgentCoordinator) synthesize(ctx context.Context, query string, responses []SpecialistResponse) (string, error) {
	var sb strings.Builder
	sb.WriteString("Original query: ")
	sb.WriteString(query)
	sb.WriteString("\n\nResponses from specialist agents:\n\n")

	for _, r := range responses {
		if r.Error != "" {
			sb.WriteString(fmt.Sprintf("[%s Agent] (error: %s)\n\n", r.Specialist, r.Error))
			continue
		}
		sb.WriteString(fmt.Sprintf("[%s Agent]:\n%s\n\n", r.Specialist, r.Answer))
	}

	sb.WriteString("Synthesize these responses into a single, coherent answer. Resolve any contradictions. ")
	sb.WriteString("Cite which specialist provided each piece of information. Be concise but complete.")

	messages := []Message{
		{Role: "system", Content: "You are an AI coordinator synthesizing multiple specialist responses into one answer. Be clear and cite sources."},
		{Role: "user", Content: sb.String()},
	}

	resp, err := c.provider.Chat(ctx, messages, &ChatOptions{MaxTokens: 2048})
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// simpleSynthesize concatenates responses without LLM synthesis.
func (c *AgentCoordinator) simpleSynthesize(responses []SpecialistResponse) string {
	var parts []string
	for _, r := range responses {
		if r.Error != "" {
			parts = append(parts, fmt.Sprintf("**%s Agent**: (error: %s)", r.Specialist, r.Error))
		} else {
			parts = append(parts, fmt.Sprintf("**%s Agent**:\n%s", r.Specialist, r.Answer))
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}
