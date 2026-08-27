package main

import (
	"context"
	"fmt"

	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// GitHubPlugin implements provider.Provider for GitHub integration.
type GitHubPlugin struct{}

var _ provider.Provider = (*GitHubPlugin)(nil)

func (p *GitHubPlugin) Name() string        { return "github" }
func (p *GitHubPlugin) Version() string     { return "0.1.0" }
func (p *GitHubPlugin) Description() string { return "GitHub source code and Actions integration" }
func (p *GitHubPlugin) PluginType() string  { return "git_provider" }

func (p *GitHubPlugin) Actions() []string {
	return []string{
		"list_groups",
		"list_repos",
		"get_branches",
		"list_pipelines",
		"trigger_pipeline",
		"parse_ci_config",
	}
}

func (p *GitHubPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	token := config["token"]
	if token == "" {
		return nil, fmt.Errorf("github plugin requires 'token' in connection config")
	}

	baseURL := config["url"]
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}

	switch action {
	case "list_groups":
		return p.listGroups(ctx, baseURL, token)
	case "list_repos":
		return p.listRepos(ctx, baseURL, token, params)
	case "get_branches":
		return p.getBranches(ctx, baseURL, token, params)
	case "list_pipelines":
		return p.listPipelines(ctx, baseURL, token, params)
	case "trigger_pipeline":
		return p.triggerPipeline(ctx, baseURL, token, params)
	case "parse_ci_config":
		return p.parseCIConfig(ctx, baseURL, token, params)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *GitHubPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "GitHub plugin ready — requires connection config (token)",
	}, nil
}

// actionOutput is a helper to encode action results.
func actionOutput(v interface{}) ([]byte, error) {
	return sdk.JSONMarshal(v)
}

// actionInput is a helper to decode action params.
func actionInput(data []byte, v interface{}) error {
	return sdk.JSONUnmarshal(data, v)
}
