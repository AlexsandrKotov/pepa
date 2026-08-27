package ai

import (
	"context"
	"fmt"
)

// AgentPolicyLevel defines what the agent is allowed to do.
type AgentPolicyLevel string

const (
	// LevelObserve — read-only tools, no side effects.
	LevelObserve AgentPolicyLevel = "observe"
	// LevelAutoFix — low-risk mutations the agent can do autonomously.
	LevelAutoFix AgentPolicyLevel = "auto_fix"
	// LevelApprove — high-risk actions that require explicit user approval.
	LevelApprove AgentPolicyLevel = "require_approval"
	// LevelForbidden — actions the agent can never perform.
	LevelForbidden AgentPolicyLevel = "forbidden"
)

// AgentPolicy maps tool names to their policy level.
type AgentPolicy struct {
	toolLevels map[string]AgentPolicyLevel
}

// NewAgentPolicy creates a policy with sensible defaults.
func NewAgentPolicy() *AgentPolicy {
	p := &AgentPolicy{toolLevels: make(map[string]AgentPolicyLevel)}
	// Register defaults
	for _, name := range observeTools {
		p.toolLevels[name] = LevelObserve
	}
	for _, name := range autoFixTools {
		p.toolLevels[name] = LevelAutoFix
	}
	for _, name := range approveTools {
		p.toolLevels[name] = LevelApprove
	}
	for _, name := range forbiddenTools {
		p.toolLevels[name] = LevelForbidden
	}
	return p
}

// Read-only tools — always allowed.
var observeTools = []string{
	"list_services",
	"get_service",
	"list_deployments",
	"get_deployment",
	"list_clusters",
	"get_cluster",
	"list_pipelines",
	"get_pipeline",
	"list_pipeline_runs",
	"list_workflows",
	"get_workflow",
	"list_environments",
	"list_connections",
	"list_plugins",
	"list_entities",
	"get_entity",
	"list_jira_issues",
	"get_agent_tasks",
	"list_docker_services",
	"get_docker_service",
	"get_docker_service_logs",
}

// Low-risk mutations — agent can do these autonomously.
var autoFixTools = []string{
	"update_service",
	"update_entity",
	"create_environment",
	"create_entity",
	"create_blueprint",
	"restart_docker_service",
	"stop_docker_service",
	"start_docker_service",
	"refresh_docker_service",
	"refresh_all_docker_services",
}

// High-risk — require user approval.
var approveTools = []string{
	"create_service",
	"deploy_service",
	"create_connection",
	"create_docker_service",
	"trigger_pipeline",
	"execute_workflow",
}

// Forbidden — agent can never do these.
var forbiddenTools = []string{
	"delete_service",
	"delete_deployment",
	"delete_cluster",
	"delete_pipeline",
	"delete_connection",
	"delete_environment",
	"delete_docker_service",
}

// Check returns nil if the tool is allowed, or an error describing the restriction.
func (p *AgentPolicy) Check(_ context.Context, toolName string) (AgentPolicyLevel, error) {
	level, ok := p.toolLevels[toolName]
	if !ok {
		// Unknown tools default to require_approval for safety
		return LevelApprove, nil
	}
	if level == LevelForbidden {
		return LevelForbidden, fmt.Errorf("tool %q is forbidden for the AI agent", toolName)
	}
	return level, nil
}

// SetLevel overrides the policy level for a specific tool.
func (p *AgentPolicy) SetLevel(toolName string, level AgentPolicyLevel) {
	p.toolLevels[toolName] = level
}

// ListLevels returns all tool policy levels.
func (p *AgentPolicy) ListLevels() map[string]AgentPolicyLevel {
	out := make(map[string]AgentPolicyLevel, len(p.toolLevels))
	for k, v := range p.toolLevels {
		out[k] = v
	}
	return out
}
