package main

import (
	"context"
	"fmt"

	"github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// JenkinsPlugin implements provider.Provider for Jenkins CI/CD integration.
type JenkinsPlugin struct{}

var _ provider.Provider = (*JenkinsPlugin)(nil)

func (p *JenkinsPlugin) Name() string    { return "jenkins" }
func (p *JenkinsPlugin) Version() string { return "1.0.0" }
func (p *JenkinsPlugin) Description() string {
	return "Jenkins CI/CD integration — jobs, builds, Groovy pipelines, agents, stages, and artifacts"
}
func (p *JenkinsPlugin) PluginType() string { return "ci_provider" }

func (p *JenkinsPlugin) Actions() []string {
	return []string{
		// Jobs & Folders
		"list_jobs",
		"list_folders",
		"get_job",
		"create_job",
		"update_job",
		"delete_job",
		"enable_job",
		"disable_job",
		// Builds & Parameters
		"list_builds",
		"get_build",
		"trigger_build",
		"rebuild",
		"stop_build",
		"get_build_parameters",
		// Groovy Pipeline Script
		"get_pipeline_script",
		"update_pipeline_script",
		"get_job_config_xml",
		"update_job_config_xml",
		// Logs & Artifacts
		"get_build_log",
		"get_build_log_progressive",
		"get_build_artifacts",
		"get_build_changes",
		"get_build_test_results",
		// Pipeline Stages
		"get_pipeline_stages",
		// Infrastructure
		"list_views",
		"list_nodes",
		"get_node",
		"get_queue",
		"list_credentials",
		// System
		"test_connection",
		"get_system_info",
	}
}

func (p *JenkinsPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	url := config["url"]
	if url == "" {
		return nil, fmt.Errorf("jenkins plugin requires 'url' in connection config")
	}

	username := config["username"]
	token := config["api_token"]
	if token == "" {
		token = config["token"]
	}
	if username == "" && token == "" {
		return nil, fmt.Errorf("jenkins plugin requires 'username' and 'api_token' (or 'token') in connection config")
	}

	insecure := config["insecure"] == "true"
	client, err := NewJenkinsClient(url, username, token, insecure)
	if err != nil {
		return nil, fmt.Errorf("create jenkins client: %w", err)
	}

	switch action {
	// Jobs & Folders
	case "list_jobs":
		return p.listJobs(ctx, client, params)
	case "list_folders":
		return p.listFolders(ctx, client, params)
	case "get_job":
		return p.getJob(ctx, client, params)
	case "create_job":
		return p.createJob(ctx, client, params)
	case "update_job":
		return p.updateJob(ctx, client, params)
	case "delete_job":
		return p.deleteJob(ctx, client, params)
	case "enable_job":
		return p.enableJob(ctx, client, params)
	case "disable_job":
		return p.disableJob(ctx, client, params)
	// Builds & Parameters
	case "list_builds":
		return p.listBuilds(ctx, client, params)
	case "get_build":
		return p.getBuild(ctx, client, params)
	case "trigger_build":
		return p.triggerBuild(ctx, client, params)
	case "rebuild":
		return p.rebuild(ctx, client, params)
	case "stop_build":
		return p.stopBuild(ctx, client, params)
	case "get_build_parameters":
		return p.getBuildParameters(ctx, client, params)
	// Groovy Pipeline Script
	case "get_pipeline_script":
		return p.getPipelineScript(ctx, client, params)
	case "update_pipeline_script":
		return p.updatePipelineScript(ctx, client, params)
	case "get_job_config_xml":
		return p.getJobConfigXML(ctx, client, params)
	case "update_job_config_xml":
		return p.updateJobConfigXML(ctx, client, params)
	// Logs & Artifacts
	case "get_build_log":
		return p.getBuildLog(ctx, client, params)
	case "get_build_log_progressive":
		return p.getBuildLogProgressive(ctx, client, params)
	case "get_build_artifacts":
		return p.getBuildArtifacts(ctx, client, params)
	case "get_build_changes":
		return p.getBuildChanges(ctx, client, params)
	case "get_build_test_results":
		return p.getBuildTestResults(ctx, client, params)
	// Pipeline Stages
	case "get_pipeline_stages":
		return p.getPipelineStages(ctx, client, params)
	// Infrastructure
	case "list_views":
		return p.listViews(ctx, client, params)
	case "list_nodes":
		return p.listNodes(ctx, client, params)
	case "get_node":
		return p.getNode(ctx, client, params)
	case "get_queue":
		return p.getQueue(ctx, client, params)
	case "list_credentials":
		return p.listCredentials(ctx, client, params)
	// System
	case "test_connection":
		return p.testConnection(ctx, client, params)
	case "get_system_info":
		return p.getSystemInfo(ctx, client, params)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *JenkinsPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "Jenkins plugin ready — requires connection config (url, username, api_token)",
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
