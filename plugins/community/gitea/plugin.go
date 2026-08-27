package main

import (
	"context"
	"fmt"

	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// GiteaPlugin implements provider.Provider for Gitea integration.
type GiteaPlugin struct{}

var _ provider.Provider = (*GiteaPlugin)(nil)

func (p *GiteaPlugin) Name() string        { return "gitea" }
func (p *GiteaPlugin) Version() string     { return "0.1.0" }
func (p *GiteaPlugin) Description() string { return "Gitea source code and pipeline integration" }
func (p *GiteaPlugin) PluginType() string  { return "git_provider" }

func (p *GiteaPlugin) Actions() []string {
	return []string{
		"list_groups",
		"list_repos",
		"get_branches",
		"list_pipelines",
		"trigger_pipeline",
		"parse_ci_config",
		"get_pipeline_jobs",
		"get_job_log",
	}
}

func (p *GiteaPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	url := config["url"]
	token := config["token"]
	if url == "" {
		return nil, fmt.Errorf("gitea plugin requires 'url' in connection config")
	}
	if token == "" {
		return nil, fmt.Errorf("gitea plugin requires 'token' in connection config")
	}

	switch action {
	case "list_groups":
		return p.listGroups(ctx, url, token)
	case "list_repos":
		return p.listRepos(ctx, url, token, params)
	case "get_branches":
		return p.getBranches(ctx, url, token, params)
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

func (p *GiteaPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "Gitea plugin ready — requires connection config (url, token)",
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
