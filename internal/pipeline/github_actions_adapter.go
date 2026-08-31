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
	"time"

	"gopkg.in/yaml.v3"
)

// GitHubActionsConfig is the expected shape of PipelineSource.Config for github_actions sources.
type GitHubActionsConfig struct {
	Owner    string `json:"owner"`    // repository owner (e.g. "alexsandrkotov")
	Repo     string `json:"repo"`     // repository name (e.g. "pepa")
	Token    string `json:"token"`    // GitHub PAT or App token
	Workflow string `json:"workflow"` // workflow file name or ID (e.g. "ci.yml")
	Ref      string `json:"ref"`      // default git ref (branch/tag/sha)
}

func parseGitHubConfig(raw json.RawMessage) (*GitHubActionsConfig, error) {
	var cfg GitHubActionsConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid github_actions config: %w", err)
	}

	// Sanitize: users may paste a full GitHub URL into owner or repo.
	// Extract owner/repo from URLs like https://github.com/owner/repo[/...]
	cfg.Owner, cfg.Repo = ghNormalizeOwnerRepo(cfg.Owner, cfg.Repo)

	if cfg.Owner == "" || cfg.Repo == "" {
		return nil, fmt.Errorf("github_actions config: owner and repo are required")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("github_actions config: token is required")
	}
	if cfg.Ref == "" {
		cfg.Ref = "main"
	}
	return &cfg, nil
}

// ghNormalizeOwnerRepo extracts owner/repo when the user pastes a full GitHub URL
// into either field. Handles:
//   - owner="AlexsandrKotov", repo="pepa" → unchanged
//   - owner="", repo="https://github.com/pepa/pepa" → owner="AlexsandrKotov", repo="pepa"
//   - owner="https://github.com/pepa/pepa", repo="" → owner="AlexsandrKotov", repo="pepa"
//   - repo="https://github.com/pepa/pepa/actions" → owner="AlexsandrKotov", repo="pepa"
func ghNormalizeOwnerRepo(owner, repo string) (string, string) {
	// If repo looks like a URL, parse it
	if strings.Contains(repo, "://") || strings.HasPrefix(repo, "github.com") {
		repo = strings.TrimPrefix(repo, "https://")
		repo = strings.TrimPrefix(repo, "http://")
		repo = strings.TrimPrefix(repo, "github.com/")
		parts := strings.SplitN(repo, "/", 3)
		if len(parts) >= 2 {
			return parts[0], strings.TrimSuffix(parts[1], ".git")
		}
	}
	// If owner looks like a URL, parse it
	if strings.Contains(owner, "://") || strings.HasPrefix(owner, "github.com") {
		owner = strings.TrimPrefix(owner, "https://")
		owner = strings.TrimPrefix(owner, "http://")
		owner = strings.TrimPrefix(owner, "github.com/")
		parts := strings.SplitN(owner, "/", 3)
		if len(parts) >= 2 {
			return parts[0], strings.TrimSuffix(parts[1], ".git")
		}
	}
	// Strip .git suffix from repo if present
	repo = strings.TrimSuffix(repo, ".git")
	return owner, repo
}

// GitHubActionsAdapter implements Provider for GitHub Actions pipelines.
type GitHubActionsAdapter struct{}

// NewGitHubActionsAdapter creates a new GitHub Actions adapter.
func NewGitHubActionsAdapter() *GitHubActionsAdapter {
	return &GitHubActionsAdapter{}
}

func (a *GitHubActionsAdapter) Name() string { return "github_actions" }

// ResolveSchema fetches the workflow file and extracts workflow_dispatch inputs.
func (a *GitHubActionsAdapter) ResolveSchema(ctx context.Context, raw json.RawMessage) (*ParameterSchema, error) {
	cfg, err := parseGitHubConfig(raw)
	if err != nil {
		return nil, err
	}

	props := make(map[string]PropertyDef)

	// Fetch the workflow YAML content to extract workflow_dispatch inputs
	if cfg.Workflow == "" {
		// No specific workflow — provide basic schema and let sync list all runs
		props["ref"] = PropertyDef{Type: "string", Description: "Git ref to run against", Default: cfg.Ref}
		return &ParameterSchema{
			Type:       "object",
			Properties: props,
			Required:   []string{"ref"},
		}, nil
	}
	contentURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", cfg.Owner, cfg.Repo, ".github/workflows/"+cfg.Workflow)
	if strings.Contains(cfg.Workflow, "/") {
		contentURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", cfg.Owner, cfg.Repo, cfg.Workflow)
	}
	contentBody, contentErr := ghAPIGet(ctx, contentURL+"?ref="+cfg.Ref, cfg.Token)
	if contentErr == nil {
		var contentResp struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}
		if json.Unmarshal(contentBody, &contentResp) == nil && contentResp.Content != "" {
			// Decode base64 content and parse YAML for workflow_dispatch inputs
			decoded := ghDecodeBase64(contentResp.Content)
			props = ghParseWorkflowInputs(decoded, props)
		}
	} else {
		// Fallback: provide basic params if we can't fetch the workflow
		props["ref"] = PropertyDef{Type: "string", Description: "Git ref to run against", Default: cfg.Ref}
		return &ParameterSchema{
			Type:       "object",
			Properties: props,
			Required:   []string{"ref"},
		}, nil
	}

	// Always include ref
	if _, ok := props["ref"]; !ok {
		props["ref"] = PropertyDef{Type: "string", Description: "Git ref to run against", Default: cfg.Ref}
	}

	return &ParameterSchema{
		Type:       "object",
		Properties: props,
	}, nil
}

// Trigger dispatches a workflow_run event via the GitHub API.
func (a *GitHubActionsAdapter) Trigger(ctx context.Context, raw json.RawMessage, params map[string]any) (*TriggerResult, error) {
	cfg, err := parseGitHubConfig(raw)
	if err != nil {
		return nil, err
	}

	ref := cfg.Ref
	if r, ok := params["ref"]; ok && r != "" {
		ref = fmt.Sprintf("%v", r)
	}

	// Build inputs from params (exclude ref)
	inputs := make(map[string]interface{})
	for k, v := range params {
		if k == "ref" {
			continue
		}
		inputs[k] = v
	}

	payload := map[string]interface{}{
		"ref":    ref,
		"inputs": inputs,
	}

	if cfg.Workflow == "" {
		return nil, fmt.Errorf("github_actions trigger: workflow file is required (e.g. ci.yml)")
	}
	dispatchURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/workflows/%s/dispatches", cfg.Owner, cfg.Repo, cfg.Workflow)
	if err := ghAPIPost(ctx, dispatchURL, cfg.Token, payload); err != nil {
		return nil, fmt.Errorf("dispatch workflow: %w", err)
	}

	// GitHub returns 204 No Content, so we need to poll for the run
	// Wait briefly then find the most recent run with retry
	var runsResp struct {
		WorkflowRuns []struct {
			ID      int    `json:"id"`
			HTMLURL string `json:"html_url"`
			Status  string `json:"status"`
		} `json:"workflow_runs"`
	}

	runsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs?per_page=1&event=workflow_dispatch", cfg.Owner, cfg.Repo)
	for attempt := 0; attempt < 3; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(1+attempt) * time.Second):
		}

		runsBody, err := ghAPIGet(ctx, runsURL, cfg.Token)
		if err != nil {
			continue
		}
		if json.Unmarshal(runsBody, &runsResp) == nil && len(runsResp.WorkflowRuns) > 0 {
			break
		}
	}

	if len(runsResp.WorkflowRuns) > 0 {
		run := runsResp.WorkflowRuns[0]
		return &TriggerResult{
			ExternalRunID: strconv.Itoa(run.ID),
			ExternalURL:   run.HTMLURL,
			Status:        mapGitHubStatus(run.Status),
		}, nil
	}

	return &TriggerResult{
		ExternalRunID: "pending",
		Status:        "pending",
	}, nil
}

// Status returns the current status of a GitHub Actions run.
func (a *GitHubActionsAdapter) Status(ctx context.Context, raw json.RawMessage, externalRunID string) (*RunStatus, error) {
	cfg, err := parseGitHubConfig(raw)
	if err != nil {
		return nil, err
	}

	runURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs/%s", cfg.Owner, cfg.Repo, externalRunID)
	body, err := ghAPIGet(ctx, runURL, cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("get run status: %w", err)
	}

	var run struct {
		ID              int    `json:"id"`
		HTMLURL         string `json:"html_url"`
		Status          string `json:"status"`
		Conclusion      string `json:"conclusion"`
		RunStartedAt    string `json:"run_started_at"`
	}
	if err := json.Unmarshal(body, &run); err != nil {
		return nil, fmt.Errorf("parse run response: %w", err)
	}

	var durationMs *int
	if run.RunStartedAt != "" {
		startTime, parseErr := time.Parse(time.RFC3339, run.RunStartedAt)
		if parseErr == nil {
			d := int(time.Since(startTime).Milliseconds())
			durationMs = &d
		}
	}

	status := mapGitHubStatus(run.Status)
	if run.Conclusion != "" {
		status = mapGitHubConclusion(run.Conclusion)
	}

	return &RunStatus{
		ExternalRunID: externalRunID,
		Status:        status,
		ExternalURL:   run.HTMLURL,
		DurationMs:    durationMs,
	}, nil
}

// Jobs returns the jobs for a GitHub Actions run.
func (a *GitHubActionsAdapter) Jobs(ctx context.Context, raw json.RawMessage, externalRunID string) ([]JobInfo, error) {
	cfg, err := parseGitHubConfig(raw)
	if err != nil {
		return nil, err
	}

	jobsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs/%s/jobs?per_page=100", cfg.Owner, cfg.Repo, externalRunID)
	body, err := ghAPIGet(ctx, jobsURL, cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("list run jobs: %w", err)
	}

	var jobsResp struct {
		Jobs []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			Status      string `json:"status"`
			Conclusion  string `json:"conclusion"`
			HTMLURL     string `json:"html_url"`
			RunnerName  string `json:"runner_name"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &jobsResp); err != nil {
		return nil, fmt.Errorf("parse jobs response: %w", err)
	}

	result := make([]JobInfo, 0, len(jobsResp.Jobs))
	for _, j := range jobsResp.Jobs {
		status := mapGitHubStatus(j.Status)
		if j.Conclusion != "" {
			status = mapGitHubConclusion(j.Conclusion)
		}
		result = append(result, JobInfo{
			ExternalJobID: strconv.Itoa(j.ID),
			Name:          j.Name,
			Status:        status,
			LogURL:        j.HTMLURL,
			RunnerName:    j.RunnerName,
		})
	}
	return result, nil
}

// Logs retrieves the logs for a GitHub Actions run (download URL).
func (a *GitHubActionsAdapter) Logs(ctx context.Context, raw json.RawMessage, externalRunID string, jobID string) (string, error) {
	cfg, err := parseGitHubConfig(raw)
	if err != nil {
		return "", err
	}

	// If a specific job is requested, try to get its logs
	if jobID != "" {
		logURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/jobs/%s/logs", cfg.Owner, cfg.Repo, jobID)
		body, err := ghAPIGet(ctx, logURL, cfg.Token)
		if err == nil {
			return string(body), nil
		}
	}

	// Fallback: get the full run logs
	logsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs/%s/logs", cfg.Owner, cfg.Repo, externalRunID)
	body, err := ghAPIGet(ctx, logsURL, cfg.Token)
	if err != nil {
		// Logs may not be available yet (e.g., still running)
		return "", nil
	}
	return string(body), nil
}

// Cancel cancels a running GitHub Actions run.
func (a *GitHubActionsAdapter) Cancel(ctx context.Context, raw json.RawMessage, externalRunID string) error {
	cfg, err := parseGitHubConfig(raw)
	if err != nil {
		return err
	}

	cancelURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs/%s/cancel", cfg.Owner, cfg.Repo, externalRunID)
	if err := ghAPIPost(ctx, cancelURL, cfg.Token, nil); err != nil {
		return fmt.Errorf("cancel run: %w", err)
	}
	return nil
}

// GetWorkflowGraph fetches and parses the workflow YAML to return a visual job graph.
func (a *GitHubActionsAdapter) GetWorkflowGraph(ctx context.Context, raw json.RawMessage) (*WorkflowGraph, error) {
	cfg, err := parseGitHubConfig(raw)
	if err != nil {
		return nil, err
	}

	if cfg.Workflow == "" {
		return nil, fmt.Errorf("github_actions: workflow file is required to build graph")
	}

	// Fetch the workflow YAML content via Contents API
	contentURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", cfg.Owner, cfg.Repo, ".github/workflows/"+cfg.Workflow)
	if strings.Contains(cfg.Workflow, "/") {
		contentURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", cfg.Owner, cfg.Repo, cfg.Workflow)
	}
	contentBody, err := ghAPIGet(ctx, contentURL+"?ref="+cfg.Ref, cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("fetch workflow file: %w", err)
	}

	var contentResp struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(contentBody, &contentResp); err != nil {
		return nil, fmt.Errorf("parse workflow response: %w", err)
	}
	if contentResp.Content == "" {
		return nil, fmt.Errorf("workflow file is empty")
	}

	decoded := ghDecodeBase64(contentResp.Content)
	graph, err := ParseGitHubWorkflowGraph(decoded)
	if err != nil {
		return nil, fmt.Errorf("parse workflow graph: %w", err)
	}

	return graph, nil
}

// ListRemoteRuns fetches recent workflow runs from GitHub Actions.
func (a *GitHubActionsAdapter) ListRemoteRuns(ctx context.Context, raw json.RawMessage, perPage int) ([]RunStatus, error) {
	cfg, err := parseGitHubConfig(raw)
	if err != nil {
		return nil, err
	}
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	// If a specific workflow is configured, scope to that workflow.
	// Fall back to listing all runs if the workflow-specific URL fails (e.g. wrong filename).
	var body []byte
	if cfg.Workflow != "" {
		workflowURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/workflows/%s/runs?per_page=%d",
			cfg.Owner, cfg.Repo, cfg.Workflow, perPage)
		body, err = ghAPIGet(ctx, workflowURL, cfg.Token)
		if err != nil {
			// Workflow file not found or other error — fall back to all runs
			fallbackURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs?per_page=%d",
				cfg.Owner, cfg.Repo, perPage)
			body, err = ghAPIGet(ctx, fallbackURL, cfg.Token)
			if err != nil {
				return nil, fmt.Errorf("list workflow runs: %w", err)
			}
		}
	} else {
		var runsURL string
		runsURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs?per_page=%d",
			cfg.Owner, cfg.Repo, perPage)
		body, err = ghAPIGet(ctx, runsURL, cfg.Token)
		if err != nil {
			return nil, fmt.Errorf("list workflow runs: %w", err)
		}
	}

	var resp struct {
		WorkflowRuns []struct {
			ID         int    `json:"id"`
			HTMLURL    string `json:"html_url"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HeadBranch string `json:"head_branch"`
			Event      string `json:"event"`
			CreatedAt  string `json:"created_at"`
			RunStartedAt string `json:"run_started_at"`
			UpdatedAt  string `json:"updated_at"`
		} `json:"workflow_runs"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse runs response: %w", err)
	}

	results := make([]RunStatus, 0, len(resp.WorkflowRuns))
	for _, r := range resp.WorkflowRuns {
		status := mapGitHubStatus(r.Status)
		if r.Conclusion != "" {
			status = mapGitHubConclusion(r.Conclusion)
		}

		var durationMs *int
		if r.RunStartedAt != "" {
			startTime, parseErr := time.Parse(time.RFC3339, r.RunStartedAt)
			if parseErr == nil {
				var endTime time.Time
				if r.UpdatedAt != "" {
					endTime, _ = time.Parse(time.RFC3339, r.UpdatedAt)
				}
				if endTime.IsZero() {
					endTime = time.Now()
				}
				d := int(endTime.Sub(startTime).Milliseconds())
				if d < 0 {
					d = 0
				}
				durationMs = &d
			}
		}

		rs := RunStatus{
			ExternalRunID: strconv.Itoa(r.ID),
			Status:        status,
			ExternalURL:   r.HTMLURL,
			DurationMs:    durationMs,
			HeadBranch:    r.HeadBranch,
			Event:         r.Event,
			CreatedAt:     r.CreatedAt,
		}

		// Fetch jobs for each run
		jobs, jobErr := a.jobsForRun(ctx, cfg, r.ID)
		if jobErr == nil {
			rs.Jobs = jobs
		}

		results = append(results, rs)
	}
	return results, nil
}

// jobsForRun fetches jobs and their steps for a single workflow run.
func (a *GitHubActionsAdapter) jobsForRun(ctx context.Context, cfg *GitHubActionsConfig, runID int) ([]JobInfo, error) {
	jobsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs/%d/jobs?per_page=100",
		cfg.Owner, cfg.Repo, runID)
	body, err := ghAPIGet(ctx, jobsURL, cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("list run jobs: %w", err)
	}

	var jobsResp struct {
		Jobs []struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HTMLURL    string `json:"html_url"`
			RunnerName string `json:"runner_name"`
			StartedAt  string `json:"started_at"`
			CompletedAt string `json:"completed_at"`
			Steps []struct {
				Name        string `json:"name"`
				Status      string `json:"status"`
				Conclusion  string `json:"conclusion"`
				Number      int    `json:"number"`
				StartedAt   string `json:"started_at"`
				CompletedAt string `json:"completed_at"`
			} `json:"steps"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &jobsResp); err != nil {
		return nil, fmt.Errorf("parse jobs response: %w", err)
	}

	result := make([]JobInfo, 0, len(jobsResp.Jobs))
	for _, j := range jobsResp.Jobs {
		status := mapGitHubStatus(j.Status)
		if j.Conclusion != "" {
			status = mapGitHubConclusion(j.Conclusion)
		}

		steps := make([]StepInfo, 0, len(j.Steps))
		for _, s := range j.Steps {
			stepStatus := mapGitHubStatus(s.Status)
			if s.Conclusion != "" {
				stepStatus = mapGitHubConclusion(s.Conclusion)
			}
			steps = append(steps, StepInfo{
				Name:        s.Name,
				Status:      stepStatus,
				Number:      s.Number,
				StartedAt:   s.StartedAt,
				CompletedAt: s.CompletedAt,
			})
		}

		result = append(result, JobInfo{
			ExternalJobID: strconv.Itoa(j.ID),
			Name:          j.Name,
			Status:        status,
			LogURL:        j.HTMLURL,
			RunnerName:    j.RunnerName,
			Steps:         steps,
		})
	}
	return result, nil
}

// ── GitHub API helpers ──────────────────────────────────────────

func ghAPIGet(ctx context.Context, rawURL, token string) ([]byte, error) {
	// SSRF protection: only allow api.github.com
	if err := ghValidateURL(rawURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github API %s: %d %s", rawURL, resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

func ghAPIPost(ctx context.Context, rawURL, token string, payload interface{}) error {
	// SSRF protection: only allow api.github.com
	if err := ghValidateURL(rawURL); err != nil {
		return err
	}

	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github API %s: %d %s", rawURL, resp.StatusCode, string(body))
	}
	return nil
}

// ghValidateURL ensures the URL points to api.github.com only (SSRF protection).
func ghValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("only HTTPS is allowed, got: %s", u.Scheme)
	}
	if u.Host != "api.github.com" {
		return fmt.Errorf("only api.github.com is allowed, got: %s", u.Host)
	}
	return nil
}

// ghDecodeBase64 decodes base64 content from GitHub Contents API.
func ghDecodeBase64(content string) string {
	// GitHub returns base64 with newlines — strip them
	cleaned := strings.ReplaceAll(content, "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return ""
	}
	return string(decoded)
}

// ghParseWorkflowInputs parses a GitHub Actions workflow YAML to extract workflow_dispatch inputs.
func ghParseWorkflowInputs(yamlContent string, props map[string]PropertyDef) map[string]PropertyDef {
	if yamlContent == "" || props == nil {
		return props
	}

	var workflow map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &workflow); err != nil {
		return props
	}

	// Navigate to on.workflow_dispatch.inputs
	on, ok := workflow["on"].(map[string]interface{})
	if !ok {
		return props
	}
	wd, ok := on["workflow_dispatch"].(map[string]interface{})
	if !ok {
		return props
	}
	inputs, ok := wd["inputs"].(map[string]interface{})
	if !ok {
		return props
	}

	for name, raw := range inputs {
		input, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		pd := PropertyDef{Type: "string"}
		if desc, ok := input["description"].(string); ok {
			pd.Description = desc
		} else {
			pd.Description = fmt.Sprintf("Workflow input: %s", name)
		}
		if def, ok := input["default"]; ok && def != nil {
			pd.Default = fmt.Sprintf("%v", def)
		}
		if req, ok := input["required"].(bool); ok && req {
			// Mark as required (handled at schema level)
		}
		if opts, ok := input["options"].([]interface{}); ok {
			for _, o := range opts {
				pd.Enum = append(pd.Enum, fmt.Sprintf("%v", o))
			}
			if len(pd.Enum) > 0 {
				pd.Type = "enum"
			}
		}
		if t, ok := input["type"].(string); ok {
			switch t {
			case "boolean":
				pd.Type = "boolean"
			case "number":
				pd.Type = "number"
			case "choice":
				if len(pd.Enum) > 0 {
					pd.Type = "enum"
				}
			}
		}
		props[name] = pd
	}

	return props
}

// mapGitHubStatus normalizes GitHub Actions status to our internal model.
func mapGitHubStatus(ghStatus string) string {
	switch strings.ToLower(ghStatus) {
	case "queued", "waiting", "pending":
		return "pending"
	case "in_progress":
		return "running"
	case "completed":
		return "success" // will be overridden by conclusion
	default:
		return strings.ToLower(ghStatus)
	}
}

// mapGitHubConclusion normalizes GitHub Actions conclusion to our internal model.
func mapGitHubConclusion(conclusion string) string {
	switch strings.ToLower(conclusion) {
	case "success":
		return "success"
	case "failure":
		return "failed"
	case "cancelled", "skipped":
		return "cancelled"
	case "timed_out":
		return "timeout"
	case "action_required":
		return "pending"
	default:
		return strings.ToLower(conclusion)
	}
}
