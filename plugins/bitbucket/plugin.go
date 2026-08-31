package main

import (
	"context"
	"fmt"

	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// BitbucketPlugin implements provider.Provider for Bitbucket integration.
type BitbucketPlugin struct{}

var _ provider.Provider = (*BitbucketPlugin)(nil)

func (p *BitbucketPlugin) Name() string    { return "bitbucket" }
func (p *BitbucketPlugin) Version() string { return "0.1.0" }
func (p *BitbucketPlugin) Description() string {
	return "Bitbucket source code and pipeline integration"
}
func (p *BitbucketPlugin) PluginType() string { return "git_provider" }

func (p *BitbucketPlugin) Actions() []string {
	return []string{
		"list_groups",
		"list_repos",
		"get_branches",
		"list_pipelines",
		"parse_ci_config",
	}
}

func (p *BitbucketPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	token := config["token"]
	if token == "" {
		return nil, fmt.Errorf("bitbucket plugin requires 'token' in connection config")
	}

	baseURL := config["url"]
	if baseURL == "" {
		baseURL = "https://api.bitbucket.org/2.0"
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
	case "parse_ci_config":
		return p.parseCIConfig(ctx, baseURL, token, params)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *BitbucketPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "Bitbucket plugin ready — requires connection config (token)",
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
