package pipeline

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/xanzy/go-gitlab"
	"gopkg.in/yaml.v3"
)

// GitLabConfig is the expected shape of PipelineSource.Config for gitlab_ci sources.
type GitLabConfig struct {
	ProjectID string `json:"project_id"` // numeric project ID as string
	BaseURL   string `json:"base_url"`   // e.g. "https://gitlab.com"
	Token     string `json:"token"`      // personal access token (or from connection)
	Ref       string `json:"ref"`        // default branch/ref
}

func parseGitLabConfig(raw json.RawMessage) (*GitLabConfig, error) {
	var cfg GitLabConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid gitlab config: %w", err)
	}
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("gitlab config: project_id is required")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("gitlab config: token is required")
	}
	if cfg.Ref == "" {
		cfg.Ref = "main"
	}
	return &cfg, nil
}

func newGitLabClient(baseURL, token string) (*gitlab.Client, error) {
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid gitlab url: %w", err)
	}
	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(u.String()))
	if err != nil {
		return nil, fmt.Errorf("create gitlab client: %w", err)
	}
	return client, nil
}

// GitLabAdapter implements Provider for GitLab CI pipelines.
type GitLabAdapter struct{}

// NewGitLabAdapter creates a new GitLab CI adapter.
func NewGitLabAdapter() *GitLabAdapter {
	return &GitLabAdapter{}
}

func (a *GitLabAdapter) Name() string { return "gitlab_ci" }

// ResolveSchema fetches .gitlab-ci.yml and extracts variables to build a JSON Schema.
// Handles both simple (KEY: value) and expanded (KEY: {value, description, options}) variable formats.
// Also parses spec.inputs for component-based pipelines (Terraform, Ansible, etc.).
func (a *GitLabAdapter) ResolveSchema(ctx context.Context, raw json.RawMessage) (*ParameterSchema, error) {
	cfg, err := parseGitLabConfig(raw)
	if err != nil {
		return nil, err
	}

	client, err := newGitLabClient(cfg.BaseURL, cfg.Token)
	if err != nil {
		return nil, err
	}

	pid, _ := strconv.Atoi(cfg.ProjectID)
	props := make(map[string]PropertyDef)

	// Try to fetch and parse .gitlab-ci.yml to extract variables
	file, _, err := client.RepositoryFiles.GetFile(pid, ".gitlab-ci.yml", &gitlab.GetFileOptions{
		Ref: gitlab.Ptr(cfg.Ref),
	}, gitlab.WithContext(ctx))
	if err == nil && file.Content != "" {
		decoded, decErr := base64.StdEncoding.DecodeString(file.Content)
		if decErr == nil {
			var ciConfig map[string]interface{}
			if yaml.Unmarshal(decoded, &ciConfig) == nil {
				// Parse spec.inputs (component pipeline inputs for Terraform, Ansible, etc.)
				specInputsToProperties(ciConfig, props)

				if vars, ok := ciConfig["variables"].(map[string]interface{}); ok {
					for k, v := range vars {
						props[k] = gitlabVarToProperty(k, v)
					}
				}
				// Also extract workflow:rules:variables
				if workflow, ok := ciConfig["workflow"].(map[string]interface{}); ok {
					if rules, ok := workflow["rules"].([]interface{}); ok {
						for _, rule := range rules {
							if ruleMap, ok := rule.(map[string]interface{}); ok {
								if ruleVars, ok := ruleMap["variables"].(map[string]interface{}); ok {
									for k, v := range ruleVars {
										if _, exists := props[k]; !exists {
											props[k] = PropertyDef{
												Type:        "string",
												Description: fmt.Sprintf("CI variable from workflow:rules:variables: %s", k),
												Default:     fmt.Sprintf("%v", v),
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Also fetch project-level CI variables as hints
	vars, _, err := client.ProjectVariables.ListVariables(pid, &gitlab.ListProjectVariablesOptions{}, gitlab.WithContext(ctx))
	if err == nil {
		for _, v := range vars {
			if _, exists := props[v.Key]; !exists {
				pd := PropertyDef{
					Type:        "string",
					Description: fmt.Sprintf("CI variable: %s", v.Key),
				}
				if v.Value != "" {
					pd.Default = v.Value
				}
				if v.Description != "" {
					pd.Description = v.Description
				}
				props[v.Key] = pd
			}
		}
	}

	// Fallback: if no variables found at all, at least provide ref
	if len(props) == 0 {
		props["ref"] = PropertyDef{Type: "string", Description: "Branch or tag to run", Default: cfg.Ref}
	}

	// Always include ref
	if _, ok := props["ref"]; !ok {
		props["ref"] = PropertyDef{Type: "string", Description: "Branch or tag to run", Default: cfg.Ref}
	}

	return &ParameterSchema{
		Type:       "object",
		Properties: props,
		Required:   []string{"ref"},
	}, nil
}

// gitlabVarToProperty converts a GitLab CI variable (simple or expanded) to a PropertyDef.
func gitlabVarToProperty(key string, raw interface{}) PropertyDef {
	switch v := raw.(type) {
	case map[string]interface{}:
		// Expanded format: {value: "...", description: "...", options: [...]}
		pd := PropertyDef{Type: "string"}
		if val, ok := v["value"]; ok && val != nil {
			pd.Default = fmt.Sprintf("%v", val)
		}
		if desc, ok := v["description"]; ok && desc != nil {
			pd.Description = fmt.Sprintf("%v", desc)
		} else {
			pd.Description = fmt.Sprintf("CI variable from .gitlab-ci.yml: %s", key)
		}
		if opts, ok := v["options"]; ok {
			if optList, ok := opts.([]interface{}); ok {
				for _, o := range optList {
					pd.Enum = append(pd.Enum, fmt.Sprintf("%v", o))
				}
				if len(pd.Enum) > 0 {
					pd.Type = "enum"
				}
			}
		}
		return pd
	default:
		// Simple format: KEY: "value"
		pd := PropertyDef{
			Type:        "string",
			Description: fmt.Sprintf("CI variable from .gitlab-ci.yml: %s", key),
		}
		if raw != nil {
			pd.Default = fmt.Sprintf("%v", raw)
		}
		return pd
	}
}

// Trigger starts a new GitLab CI pipeline.
// It separates spec.inputs (marked with IsInput in the schema) from regular
// CI variables and passes them via the "inputs" field in the API request body,
// which is supported since GitLab 17.10 (GA in 18.1).
func (a *GitLabAdapter) Trigger(ctx context.Context, raw json.RawMessage, params map[string]any) (*TriggerResult, error) {
	cfg, err := parseGitLabConfig(raw)
	if err != nil {
		return nil, err
	}

	// Extract embedded inputs injected by the handler (from schema IsInput metadata).
	var inputs map[string]any
	if embedded, ok := params["__gitlab_inputs__"]; ok {
		if m, ok := embedded.(map[string]any); ok {
			inputs = m
		}
	}

	// Build CI variables from params (exclude ref and the embedded inputs key).
	variables := make([]*gitlab.PipelineVariableOptions, 0)
	for k, v := range params {
		if k == "ref" || k == "__gitlab_inputs__" {
			continue
		}
		// Skip keys that are in the inputs map (they are passed via the inputs field).
		if inputs != nil {
			if _, isInput := inputs[k]; isInput {
				continue
			}
		}
		variables = append(variables, &gitlab.PipelineVariableOptions{
			Key:   gitlab.Ptr(k),
			Value: gitlab.Ptr(fmt.Sprintf("%v", v)),
		})
	}

	ref := cfg.Ref
	if r, ok := params["ref"]; ok && r != "" {
		ref = fmt.Sprintf("%v", r)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Build the API request body with both variables and inputs.
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/pipeline", baseURL, url.PathEscape(cfg.ProjectID))
	body := map[string]interface{}{
		"ref": ref,
	}
	if len(variables) > 0 {
		vars := make([]map[string]string, 0, len(variables))
		for _, v := range variables {
			vars = append(vars, map[string]string{
				"key":   *v.Key,
				"value": *v.Value,
			})
		}
		body["variables"] = vars
	}
	if len(inputs) > 0 {
		body["inputs"] = inputs
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal pipeline request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create pipeline request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("PRIVATE-TOKEN", cfg.Token)

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("trigger gitlab pipeline: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read pipeline response: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, fmt.Errorf("trigger gitlab pipeline (%d): %s", httpResp.StatusCode, string(respBody))
	}

	var result struct {
		ID     int    `json:"id"`
		WebURL string `json:"web_url"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse pipeline response: %w", err)
	}

	return &TriggerResult{
		ExternalRunID: strconv.Itoa(result.ID),
		ExternalURL:   result.WebURL,
		Status:        result.Status,
	}, nil
}

// Status returns the current status of a GitLab pipeline.
func (a *GitLabAdapter) Status(ctx context.Context, raw json.RawMessage, externalRunID string) (*RunStatus, error) {
	cfg, err := parseGitLabConfig(raw)
	if err != nil {
		return nil, err
	}

	client, err := newGitLabClient(cfg.BaseURL, cfg.Token)
	if err != nil {
		return nil, err
	}

	pid, _ := strconv.Atoi(cfg.ProjectID)
	runID, _ := strconv.Atoi(externalRunID)

	pipeline, _, err := client.Pipelines.GetPipeline(pid, runID, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get pipeline status: %w", err)
	}

	var durationMs *int
	if pipeline.Duration > 0 {
		d := int(pipeline.Duration * 1000)
		durationMs = &d
	}

	return &RunStatus{
		ExternalRunID: externalRunID,
		Status:        mapGitLabStatus(string(pipeline.Status)),
		ExternalURL:   pipeline.WebURL,
		DurationMs:    durationMs,
	}, nil
}

// Jobs returns the jobs for a GitLab pipeline.
func (a *GitLabAdapter) Jobs(ctx context.Context, raw json.RawMessage, externalRunID string) ([]JobInfo, error) {
	cfg, err := parseGitLabConfig(raw)
	if err != nil {
		return nil, err
	}

	client, err := newGitLabClient(cfg.BaseURL, cfg.Token)
	if err != nil {
		return nil, err
	}

	pid, _ := strconv.Atoi(cfg.ProjectID)
	runID, _ := strconv.Atoi(externalRunID)

	jobs, _, err := client.Jobs.ListPipelineJobs(pid, runID, &gitlab.ListJobsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list pipeline jobs: %w", err)
	}

	result := make([]JobInfo, 0, len(jobs))
	for _, j := range jobs {
		result = append(result, JobInfo{
			ExternalJobID: strconv.Itoa(j.ID),
			Name:          j.Name,
			Stage:         j.Stage,
			Status:        mapGitLabStatus(string(j.Status)),
			LogURL:        j.WebURL,
			RunnerName:    j.Runner.Description,
			AllowFailure:  j.AllowFailure,
		})
	}
	return result, nil
}

// Logs retrieves the trace (log output) for a specific job.
func (a *GitLabAdapter) Logs(ctx context.Context, raw json.RawMessage, externalRunID string, jobID string) (string, error) {
	cfg, err := parseGitLabConfig(raw)
	if err != nil {
		return "", err
	}

	client, err := newGitLabClient(cfg.BaseURL, cfg.Token)
	if err != nil {
		return "", err
	}

	pid, _ := strconv.Atoi(cfg.ProjectID)
	jid, _ := strconv.Atoi(jobID)

	reader, _, err := client.Jobs.GetTraceFile(pid, jid, gitlab.WithContext(ctx))
	if err != nil {
		// Fallback: return empty logs if trace not available
		return "", nil
	}
	if reader == nil {
		return "", nil
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read trace: %w", err)
	}
	return string(data), nil
}

// Cancel cancels a running GitLab pipeline.
func (a *GitLabAdapter) Cancel(ctx context.Context, raw json.RawMessage, externalRunID string) error {
	cfg, err := parseGitLabConfig(raw)
	if err != nil {
		return err
	}

	client, err := newGitLabClient(cfg.BaseURL, cfg.Token)
	if err != nil {
		return err
	}

	pid, _ := strconv.Atoi(cfg.ProjectID)
	runID, _ := strconv.Atoi(externalRunID)

	_, _, err = client.Pipelines.CancelPipelineBuild(pid, runID, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("cancel pipeline: %w", err)
	}
	return nil
}

// ListRemoteRuns fetches recent pipelines from GitLab.
func (a *GitLabAdapter) ListRemoteRuns(ctx context.Context, raw json.RawMessage, perPage int) ([]RunStatus, error) {
	cfg, err := parseGitLabConfig(raw)
	if err != nil {
		return nil, err
	}
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	client, err := newGitLabClient(cfg.BaseURL, cfg.Token)
	if err != nil {
		return nil, err
	}

	pid, _ := strconv.Atoi(cfg.ProjectID)

	pipelines, _, err := client.Pipelines.ListProjectPipelines(pid, &gitlab.ListProjectPipelinesOptions{
		ListOptions: gitlab.ListOptions{PerPage: perPage},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list pipelines: %w", err)
	}

	result := make([]RunStatus, 0, len(pipelines))
	for _, p := range pipelines {
		rs := RunStatus{
			ExternalRunID: strconv.Itoa(p.ID),
			Status:        mapGitLabStatus(string(p.Status)),
			ExternalURL:   p.WebURL,
			HeadBranch:    p.Ref,
			Event:         p.Source,
		}
		if p.CreatedAt != nil {
			rs.CreatedAt = p.CreatedAt.String()
		}
		result = append(result, rs)
	}
	return result, nil
}

// specInputsToProperties parses the spec.inputs section from a GitLab CI config
// and converts each input definition to a PropertyDef.
// GitLab component pipelines use this to declare typed inputs:
//
//	spec:
//	  inputs:
//	    pipeline_target:
//	      description: "Which child pipeline to run"
//	      default: "terraform_subpipelines"
//	    terraform_action:
//	      description: "Terraform action"
//	      options: ["default", "copy_state", "destroy_recreate"]
//	    skip_manual:
//	      description: "Skip manual jobs"
//	      default: false
func specInputsToProperties(ciConfig map[string]interface{}, props map[string]PropertyDef) {
	spec, ok := ciConfig["spec"].(map[string]interface{})
	if !ok {
		return
	}
	inputs, ok := spec["inputs"].(map[string]interface{})
	if !ok {
		return
	}
	for name, raw := range inputs {
		input, ok := raw.(map[string]interface{})
		if !ok {
			// Simple string value used as default
			props[name] = PropertyDef{
				Type:        "string",
				Description: fmt.Sprintf("Pipeline input: %s", name),
				Default:     fmt.Sprintf("%v", raw),
				IsInput:     true,
			}
			continue
		}
		pd := PropertyDef{Type: "string", IsInput: true}
		if desc, ok := input["description"].(string); ok {
			pd.Description = desc
		} else {
			pd.Description = fmt.Sprintf("Pipeline input: %s", name)
		}
		if def, ok := input["default"]; ok && def != nil {
			switch v := def.(type) {
			case bool:
				pd.Type = "boolean"
				pd.Default = fmt.Sprintf("%v", v)
			case float64:
				pd.Type = "number"
				pd.Default = fmt.Sprintf("%v", v)
			default:
				pd.Default = fmt.Sprintf("%v", def)
			}
		}
		if opts, ok := input["options"].([]interface{}); ok {
			for _, o := range opts {
				pd.Enum = append(pd.Enum, fmt.Sprintf("%v", o))
			}
			if len(pd.Enum) > 0 {
				pd.Type = "enum"
			}
		}
		// Infer boolean type from default value even without explicit type
		if def, ok := input["default"].(bool); ok {
			pd.Type = "boolean"
			_ = def
		}
		props[name] = pd
	}
}

// mapGitLabStatus normalizes GitLab pipeline/job statuses to our internal model.
func mapGitLabStatus(glStatus string) string {
	s := strings.ToLower(glStatus)
	switch s {
	case "created", "waiting_for_resource", "preparing", "pending", "scheduled":
		return "pending"
	case "running":
		return "running"
	case "success":
		return "success"
	case "failed":
		return "failed"
	case "canceled", "cancelled":
		return "cancelled"
	case "skipped", "manual":
		return "cancelled"
	default:
		return s
	}
}
