package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WorkflowDefinition represents a generated workflow.
type WorkflowDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Trigger     map[string]interface{} `json:"trigger"`
	Steps       []WorkflowStep         `json:"steps"`
	Variables   map[string]string      `json:"variables,omitempty"`
	GeneratedAt time.Time              `json:"generated_at"`
	Model       string                 `json:"model"`
	Valid       bool                   `json:"valid"`
	Errors      []string               `json:"errors,omitempty"`
}

// WorkflowStep represents a single step in a workflow.
type WorkflowStep struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Plugin   string                 `json:"plugin"`
	Action   string                 `json:"action"`
	Inputs   map[string]interface{} `json:"inputs,omitempty"`
	Condition string                `json:"condition,omitempty"`
	OnError  string                 `json:"on_error,omitempty"`
}

// WorkflowBuilder generates workflow definitions from natural language.
type WorkflowBuilder struct {
	pool     *pgxpool.Pool
	provider LLMProvider
	tenantID uuid.UUID
}

// NewWorkflowBuilder creates a new NL workflow builder.
func NewWorkflowBuilder(pool *pgxpool.Pool, provider LLMProvider, tenantID uuid.UUID) *WorkflowBuilder {
	return &WorkflowBuilder{pool: pool, provider: provider, tenantID: tenantID}
}

// BuildWorkflow generates a workflow from a natural language description.
func (b *WorkflowBuilder) BuildWorkflow(ctx context.Context, description string, environment string) (*WorkflowDefinition, error) {
	// Gather available plugins and environments for context
	pluginsContext, err := b.getAvailablePlugins(ctx)
	if err != nil {
		slog.Warn("could not load plugins for workflow builder", "error", err)
		pluginsContext = "Unable to load plugins list."
	}

	envContext, err := b.getAvailableEnvironments(ctx)
	if err != nil {
		slog.Warn("could not load environments for workflow builder", "error", err)
		envContext = "Unable to load environments list."
	}

	prompt := fmt.Sprintf(`You are a DevOps workflow builder. Generate a workflow definition from the following natural language description.

Description: %s
Target environment: %s

Available plugins (use these for steps):
%s

Available environments:
%s

Generate a workflow as JSON with this exact structure:
{
  "name": "workflow-name",
  "description": "What this workflow does",
  "trigger": {
    "type": "manual|webhook|schedule|event",
    "config": {}
  },
  "steps": [
    {
      "id": "step-1",
      "name": "Step Name",
      "plugin": "plugin_name",
      "action": "action_name",
      "inputs": {},
      "condition": "optional condition expression",
      "on_error": "abort|continue|retry"
    }
  ],
  "variables": {
    "VAR_NAME": "default_value"
  }
}

Rules:
1. Use ONLY the plugins listed above
2. Include proper error handling (on_error field)
3. Add conditions when steps should be conditional
4. Use environment variables for sensitive values
5. Make the workflow production-ready
6. Respond ONLY with valid JSON, no markdown or explanation`, description, environment, pluginsContext, envContext)

	messages := []Message{
		{Role: "system", Content: "You are a DevOps workflow architect. Generate valid JSON workflow definitions. Respond ONLY with JSON."},
		{Role: "user", Content: prompt},
	}

	resp, err := b.provider.Chat(ctx, messages, &ChatOptions{
		MaxTokens:      4096,
		ResponseFormat: "json_object",
	})
	if err != nil {
		return nil, fmt.Errorf("workflow generation failed: %w", err)
	}

	workflow := &WorkflowDefinition{
		GeneratedAt: time.Now(),
		Model:       resp.ModelUsed,
	}

	if err := json.Unmarshal([]byte(resp.Content), workflow); err != nil {
		// If parsing fails, wrap the raw response
		workflow.Name = "generated-workflow"
		workflow.Description = description
		workflow.Valid = false
		workflow.Errors = []string{fmt.Sprintf("Failed to parse workflow JSON: %v", err)}
		workflow.Steps = []WorkflowStep{
			{
				ID:     "raw-output",
				Name:   "Raw AI Output",
				Plugin: "core",
				Action: "log",
				Inputs: map[string]interface{}{"message": resp.Content},
			},
		}
		return workflow, nil
	}

	// Validate the generated workflow
	workflow.Valid = true
	validationErrors := b.validateWorkflow(ctx, workflow)
	if len(validationErrors) > 0 {
		workflow.Valid = false
		workflow.Errors = validationErrors
	}

	workflow.GeneratedAt = time.Now()
	workflow.Model = resp.ModelUsed

	slog.Info("workflow generated from NL description",
		"name", workflow.Name,
		"steps", len(workflow.Steps),
		"valid", workflow.Valid,
		"model", workflow.Model)

	return workflow, nil
}

// ValidateWorkflow checks a workflow definition for errors.
func (b *WorkflowBuilder) validateWorkflow(ctx context.Context, wf *WorkflowDefinition) []string {
	var errors []string

	if wf.Name == "" {
		errors = append(errors, "Workflow name is required")
	}

	if len(wf.Steps) == 0 {
		errors = append(errors, "Workflow must have at least one step")
	}

	// Check for duplicate step IDs
	stepIDs := make(map[string]bool)
	for _, step := range wf.Steps {
		if step.ID == "" {
			errors = append(errors, "All steps must have an ID")
		}
		if stepIDs[step.ID] {
			errors = append(errors, fmt.Sprintf("Duplicate step ID: %s", step.ID))
		}
		stepIDs[step.ID] = true

		if step.Plugin == "" {
			errors = append(errors, fmt.Sprintf("Step %s: plugin is required", step.ID))
		}
		if step.Action == "" {
			errors = append(errors, fmt.Sprintf("Step %s: action is required", step.ID))
		}
	}

	return errors
}

// ToYAML converts a workflow definition to YAML format.
func (wf *WorkflowDefinition) ToYAML() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("name: %s\n", wf.Name))
	sb.WriteString(fmt.Sprintf("description: %q\n", wf.Description))

	if wf.Trigger != nil {
		sb.WriteString("trigger:\n")
		if t, ok := wf.Trigger["type"]; ok {
			sb.WriteString(fmt.Sprintf("  type: %s\n", t))
		}
		if cfg, ok := wf.Trigger["config"]; ok {
			sb.WriteString(fmt.Sprintf("  config: %v\n", cfg))
		}
	}

	if len(wf.Variables) > 0 {
		sb.WriteString("variables:\n")
		for k, v := range wf.Variables {
			sb.WriteString(fmt.Sprintf("  %s: %q\n", k, v))
		}
	}

	sb.WriteString("steps:\n")
	for _, step := range wf.Steps {
		sb.WriteString(fmt.Sprintf("  - id: %s\n", step.ID))
		sb.WriteString(fmt.Sprintf("    name: %s\n", step.Name))
		sb.WriteString(fmt.Sprintf("    plugin: %s\n", step.Plugin))
		sb.WriteString(fmt.Sprintf("    action: %s\n", step.Action))
		if len(step.Inputs) > 0 {
			sb.WriteString("    inputs:\n")
			for k, v := range step.Inputs {
				sb.WriteString(fmt.Sprintf("      %s: %v\n", k, v))
			}
		}
		if step.Condition != "" {
			sb.WriteString(fmt.Sprintf("    condition: %q\n", step.Condition))
		}
		if step.OnError != "" {
			sb.WriteString(fmt.Sprintf("    on_error: %s\n", step.OnError))
		}
	}

	return sb.String()
}

// getAvailablePlugins returns a formatted list of available plugins.
func (b *WorkflowBuilder) getAvailablePlugins(ctx context.Context) (string, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT name, plugin_type, COALESCE(description, '')
		FROM plugins WHERE status = 'running' AND tenant_id = $1
		ORDER BY name
	`, b.tenantID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var sb strings.Builder
	for rows.Next() {
		var name, ptype, desc string
		_ = rows.Scan(&name, &ptype, &desc)
		sb.WriteString(fmt.Sprintf("  - %s (%s): %s\n", name, ptype, desc))
	}

	if sb.Len() == 0 {
		return "No plugins currently available. Use generic steps like: shell, http_request, log.", nil
	}
	return sb.String(), nil
}

// getAvailableEnvironments returns a formatted list of environments.
func (b *WorkflowBuilder) getAvailableEnvironments(ctx context.Context) (string, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT name, COALESCE(description, '')
		FROM environments WHERE tenant_id = $1
		ORDER BY name
	`, b.tenantID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var sb strings.Builder
	for rows.Next() {
		var name, desc string
		_ = rows.Scan(&name, &desc)
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", name, desc))
	}

	if sb.Len() == 0 {
		return "No environments configured.", nil
	}
	return sb.String(), nil
}
