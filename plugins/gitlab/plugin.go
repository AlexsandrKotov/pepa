package main

import (
	"context"
	"fmt"

	"github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// GitLabPlugin implements provider.Provider for GitLab integration.
type GitLabPlugin struct{}

var _ provider.Provider = (*GitLabPlugin)(nil)

func (p *GitLabPlugin) Name() string        { return "gitlab" }
func (p *GitLabPlugin) Version() string     { return "0.1.0" }
func (p *GitLabPlugin) Description() string { return "GitLab source code and CI/CD integration" }
func (p *GitLabPlugin) PluginType() string  { return "git_provider" }

func (p *GitLabPlugin) Actions() []string {
	return []string{
		"list_groups",
		"list_repos",
		"get_branches",
		"create_branch",
		"create_mr",
		"get_mr",
		"list_pipelines",
		"trigger_pipeline",
		"parse_ci_config",
		"get_pipeline_jobs",
		"get_job_log",
	}
}

func (p *GitLabPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	url := config["url"]
	token := config["token"]
	if url == "" {
		return nil, fmt.Errorf("gitlab plugin requires 'url' in connection config")
	}
	if token == "" {
		return nil, fmt.Errorf("gitlab plugin requires 'token' (Personal Access Token with 'api' scope) in connection config. Username/password authentication is not supported by the GitLab API")
	}

	switch action {
	case "list_groups":
		return p.listGroups(ctx, url, token, params)
	case "list_repos":
		return p.listRepos(ctx, url, token, params)
	case "get_branches":
		return p.getBranches(ctx, url, token, params)
	case "create_branch":
		return p.createBranch(ctx, url, token, params)
	case "create_mr":
		return p.createMR(ctx, url, token, params)
	case "get_mr":
		return p.getMR(ctx, url, token, params)
	case "list_pipelines":
		return p.listPipelines(ctx, url, token, params)
	case "trigger_pipeline":
		return p.triggerPipeline(ctx, url, token, params)
	case "parse_ci_config":
		return p.parseCIConfig(ctx, url, token, params)
	case "get_pipeline_jobs":
		return p.getPipelineJobs(ctx, url, token, params)
	case "get_job_log":
		return p.getJobLog(ctx, url, token, params)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *GitLabPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	// We can't do a full health check without config, so just report ready
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "GitLab plugin ready — requires connection config (url, token)",
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
