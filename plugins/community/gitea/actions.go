package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// giteaAPIRequest performs an authenticated Gitea API request.
func giteaAPIRequest(ctx context.Context, baseURL, token, path string) ([]byte, error) {
	u := strings.TrimRight(baseURL, "/") + "/api/v1" + path

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: nil,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitea request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gitea API returned %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// listGroups returns organizations the authenticated user belongs to.
func (p *GiteaPlugin) listGroups(ctx context.Context, baseURL, token string) ([]byte, error) {
	data, err := giteaAPIRequest(ctx, baseURL, token, "/user/orgs?limit=50")
	if err != nil {
		return nil, fmt.Errorf("list orgs: %w", err)
	}

	var orgs []struct {
		ID        int    `json:"id"`
		Username  string `json:"username"`
		FullName  string `json:"full_name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(data, &orgs); err != nil {
		return nil, fmt.Errorf("parse orgs: %w", err)
	}

	type group struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		URL      string `json:"url"`
		Kind     string `json:"kind"`
	}
	groups := make([]group, 0, len(orgs))
	for _, o := range orgs {
		name := o.Username
		if name == "" {
			name = o.FullName
		}
		groups = append(groups, group{
			ID:       name,
			Name:     name,
			FullName: o.FullName,
			URL:      strings.TrimRight(baseURL, "/") + "/" + name,
			Kind:     "organization",
		})
	}
	return actionOutput(map[string]any{"groups": groups, "total": len(groups)})
}

// listRepos returns repositories. If group_id is provided, lists org repos; otherwise user repos.
func (p *GiteaPlugin) listRepos(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		GroupID string `json:"group_id"` // org name or ID
	}
	_ = actionInput(params, &req)

	var path string
	if req.GroupID != "" {
		path = fmt.Sprintf("/orgs/%s/repos?limit=50&sort=updated", url.PathEscape(req.GroupID))
	} else {
		path = "/user/repos?limit=50&sort=updated"
	}

	data, err := giteaAPIRequest(ctx, baseURL, token, path)
	if err != nil {
		return nil, fmt.Errorf("list repos: %w", err)
	}

	var giteaRepos []struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Description   string `json:"description"`
		HTMLURL       string `json:"html_url"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
	}
	if err := json.Unmarshal(data, &giteaRepos); err != nil {
		return nil, fmt.Errorf("parse repos: %w", err)
	}

	type repo struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Description   string `json:"description"`
		URL           string `json:"url"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
	}
	repos := make([]repo, 0, len(giteaRepos))
	for _, r := range giteaRepos {
		repos = append(repos, repo{
			ID:            fmt.Sprintf("%d", r.ID),
			Name:          r.Name,
			FullName:      r.FullName,
			Description:   r.Description,
			URL:           r.HTMLURL,
			DefaultBranch: r.DefaultBranch,
			Private:       r.Private,
		})
	}
	return actionOutput(map[string]any{"repos": repos, "total": len(repos)})
}

// getBranches returns branches for a repository.
func (p *GiteaPlugin) getBranches(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"` // "owner/repo" format
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	if req.RepoID == "" {
		return nil, fmt.Errorf("repo_id is required (owner/repo format)")
	}

	path := fmt.Sprintf("/repos/%s/branches?limit=50", req.RepoID)
	data, err := giteaAPIRequest(ctx, baseURL, token, path)
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}

	var giteaBranches []struct {
		Name      string `json:"name"`
		Protected bool   `json:"protected"`
		Commit    struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(data, &giteaBranches); err != nil {
		return nil, fmt.Errorf("parse branches: %w", err)
	}

	type branch struct {
		Name      string `json:"name"`
		SHA       string `json:"sha"`
		Protected bool   `json:"protected"`
	}
	branches := make([]branch, 0, len(giteaBranches))
	for _, b := range giteaBranches {
		branches = append(branches, branch{
			Name:      b.Name,
			SHA:       b.Commit.ID,
			Protected: b.Protected,
		})
	}
	return actionOutput(map[string]any{"branches": branches, "total": len(branches)})
}

// giteaAPIRequestPOST performs an authenticated Gitea API POST request.
func giteaAPIRequestPOST(ctx context.Context, baseURL, token, path string, body []byte) (int, []byte, error) {
	u := strings.TrimRight(baseURL, "/") + "/api/v1" + path

	req, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("gitea request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read response: %w", err)
	}
	return resp.StatusCode, respBody, nil
}

// giteaWorkflowFile describes a workflow definition file inside a repository.
type giteaWorkflowFile struct {
	Name string // file name, e.g. "ci.yml"
	Path string // repo-relative path, e.g. ".gitea/workflows/ci.yml"
}

// giteaWorkflowDirs are the directories Gitea scans for Actions workflows.
// Gitea ("Действия" / Actions in the UI) picks up files from both locations.
var giteaWorkflowDirs = []string{".gitea/workflows", ".github/workflows"}

// listWorkflowFiles returns Gitea Actions workflow definition files for a ref.
func listWorkflowFiles(ctx context.Context, baseURL, token, repoID, ref string) []giteaWorkflowFile {
	out := make([]giteaWorkflowFile, 0)
	for _, dir := range giteaWorkflowDirs {
		listPath := fmt.Sprintf("/repos/%s/contents/%s?ref=%s", url.PathEscape(repoID), dir, url.PathEscape(ref))
		data, err := giteaAPIRequest(ctx, baseURL, token, listPath)
		if err != nil {
			continue
		}
		var files []struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &files) != nil {
			continue
		}
		for _, f := range files {
			if f.Type != "file" {
				continue
			}
			if !strings.HasSuffix(f.Name, ".yml") && !strings.HasSuffix(f.Name, ".yaml") {
				continue
			}
			out = append(out, giteaWorkflowFile{Name: f.Name, Path: f.Path})
		}
	}
	return out
}

// fetchWorkflowFile fetches and base64-decodes a workflow file at a ref.
func fetchWorkflowFile(ctx context.Context, baseURL, token, repoID, path, ref string) ([]byte, error) {
	contentPath := fmt.Sprintf("/repos/%s/contents/%s?ref=%s", url.PathEscape(repoID), url.PathEscape(path), url.PathEscape(ref))
	data, err := giteaAPIRequest(ctx, baseURL, token, contentPath)
	if err != nil {
		return nil, err
	}
	var fileContent struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(data, &fileContent); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(strings.ReplaceAll(fileContent.Content, "\n", ""))
}

// defaultBranch returns the repository default branch (best effort).
func defaultBranch(ctx context.Context, baseURL, token, repoID string) string {
	data, err := giteaAPIRequest(ctx, baseURL, token, fmt.Sprintf("/repos/%s", repoID))
	if err == nil {
		var repo struct {
			DefaultBranch string `json:"default_branch"`
		}
		if json.Unmarshal(data, &repo) == nil && repo.DefaultBranch != "" {
			return repo.DefaultBranch
		}
	}
	return "main"
}

// listPipelines returns Gitea Actions workflow runs for a repository.
// Gitea calls this feature "Actions". If the runs API is unavailable or the
// repository has no runs yet, we fall back to listing workflow definition
// files (.gitea/workflows, .github/workflows) so pipelines stay visible.
func (p *GiteaPlugin) listPipelines(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"` // "owner/repo" format
		Ref    string `json:"ref"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	if req.RepoID == "" {
		return nil, fmt.Errorf("repo_id is required (owner/repo format)")
	}

	type pipeline struct {
		ID     int    `json:"id"`
		SHA    string `json:"sha"`
		Ref    string `json:"ref"`
		Status string `json:"status"`
		Source string `json:"source"`
		URL    string `json:"url"`
	}

	// 1. Try the Actions runs API (Gitea >= 1.19 with Actions enabled).
	path := fmt.Sprintf("/repos/%s/actions/runs?limit=20", req.RepoID)
	if data, err := giteaAPIRequest(ctx, baseURL, token, path); err == nil {
		var result struct {
			WorkflowRuns []struct {
				ID         int    `json:"id"`
				HeadSHA    string `json:"head_sha"`
				HeadBranch string `json:"head_branch"`
				Status     string `json:"status"`
				Name       string `json:"name"`
				HTMLURL    string `json:"html_url"`
			} `json:"workflow_runs"`
		}
		if json.Unmarshal(data, &result) == nil && len(result.WorkflowRuns) > 0 {
			pipelines := make([]pipeline, 0, len(result.WorkflowRuns))
			for _, r := range result.WorkflowRuns {
				status := r.Status
				if status == "done" {
					status = "success"
				}
				pipelines = append(pipelines, pipeline{
					ID:     r.ID,
					SHA:    r.HeadSHA,
					Ref:    r.HeadBranch,
					Status: status,
					Source: r.Name,
					URL:    r.HTMLURL,
				})
			}
			return actionOutput(map[string]any{"pipelines": pipelines, "total": len(pipelines)})
		}
	}

	// 2. Fallback: expose workflow definition files as pipelines so the user
	//    sees the "Actions" of the repository even without run history.
	ref := req.Ref
	if ref == "" {
		ref = defaultBranch(ctx, baseURL, token, req.RepoID)
	}
	files := listWorkflowFiles(ctx, baseURL, token, req.RepoID, ref)
	pipelines := make([]pipeline, 0, len(files))
	for i, f := range files {
		pipelines = append(pipelines, pipeline{
			ID:     i + 1,
			SHA:    "",
			Ref:    ref,
			Status: "active",
			Source: f.Name,
			URL:    strings.TrimRight(baseURL, "/") + "/" + req.RepoID + "/actions",
		})
	}
	return actionOutput(map[string]any{"pipelines": pipelines, "total": len(pipelines)})
}

// triggerPipeline dispatches a Gitea Actions workflow via workflow_dispatch
// (POST /repos/{owner}/{repo}/actions/workflows/{workflow}/dispatches, Gitea >= 1.21).
// Variables are passed as dispatch inputs.
func (p *GiteaPlugin) triggerPipeline(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID    string            `json:"repo_id"` // "owner/repo"
		Ref       string            `json:"ref"`
		Workflow  string            `json:"workflow"` // optional: workflow file name to dispatch
		Variables map[string]string `json:"variables"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	if req.RepoID == "" {
		return nil, fmt.Errorf("repo_id is required (owner/repo format)")
	}
	ref := req.Ref
	if ref == "" {
		ref = defaultBranch(ctx, baseURL, token, req.RepoID)
	}

	// Discover workflow files and prefer ones with a workflow_dispatch trigger.
	files := listWorkflowFiles(ctx, baseURL, token, req.RepoID, ref)
	if len(files) == 0 {
		return nil, fmt.Errorf("no workflow files found in %s (checked .gitea/workflows and .github/workflows at ref %s)", req.RepoID, ref)
	}

	type candidate struct {
		file     giteaWorkflowFile
		dispatch bool
		name     string
	}
	candidates := make([]candidate, 0, len(files))
	for _, f := range files {
		decoded, err := fetchWorkflowFile(ctx, baseURL, token, req.RepoID, f.Path, ref)
		if err != nil {
			candidates = append(candidates, candidate{file: f})
			continue
		}
		var workflow map[string]interface{}
		if yaml.Unmarshal(decoded, &workflow) != nil {
			candidates = append(candidates, candidate{file: f})
			continue
		}
		c := candidate{file: f}
		if name, ok := workflow["name"].(string); ok {
			c.name = name
		}
		if onSection, ok := workflow["on"].(map[string]interface{}); ok {
			if _, ok := onSection["workflow_dispatch"]; ok {
				c.dispatch = true
			}
		}
		candidates = append(candidates, c)
	}

	// Pick the target workflow: explicit choice > first dispatchable > first file.
	var target *candidate
	if req.Workflow != "" {
		for i := range candidates {
			if candidates[i].file.Name == req.Workflow {
				target = &candidates[i]
				break
			}
		}
		if target == nil {
			return nil, fmt.Errorf("workflow %q not found in repository", req.Workflow)
		}
	} else {
		for i := range candidates {
			if candidates[i].dispatch {
				target = &candidates[i]
				break
			}
		}
		if target == nil {
			return nil, fmt.Errorf("no workflow with a workflow_dispatch trigger found — add 'on: workflow_dispatch' to the workflow to enable manual runs with parameters")
		}
	}

	body, err := json.Marshal(map[string]any{
		"ref":    ref,
		"inputs": req.Variables,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal dispatch body: %w", err)
	}
	dispatchPath := fmt.Sprintf("/repos/%s/actions/workflows/%s/dispatches?ref=%s",
		req.RepoID, url.PathEscape(target.file.Name), url.QueryEscape(ref))
	status, respBody, err := giteaAPIRequestPOST(ctx, baseURL, token, dispatchPath, body)
	if err != nil {
		return nil, fmt.Errorf("dispatch workflow: %w", err)
	}
	if status >= 400 {
		if status == 404 {
			return nil, fmt.Errorf("dispatch failed: workflow_dispatch API not available (requires Gitea >= 1.21 with Actions enabled): %s", string(respBody))
		}
		return nil, fmt.Errorf("dispatch failed (%d): %s", status, string(respBody))
	}

	// Best effort: find the freshly created run to return its ID/URL.
	result := map[string]any{
		"id":     0,
		"ref":    ref,
		"status": "queued",
		"url":    strings.TrimRight(baseURL, "/") + "/" + req.RepoID + "/actions",
	}
	time.Sleep(2 * time.Second)
	if data, err := giteaAPIRequest(ctx, baseURL, token, fmt.Sprintf("/repos/%s/actions/runs?limit=5", req.RepoID)); err == nil {
		var runs struct {
			WorkflowRuns []struct {
				ID         int    `json:"id"`
				HeadBranch string `json:"head_branch"`
				HTMLURL    string `json:"html_url"`
			} `json:"workflow_runs"`
		}
		if json.Unmarshal(data, &runs) == nil {
			for _, r := range runs.WorkflowRuns {
				if r.HeadBranch == ref {
					result["id"] = r.ID
					result["url"] = r.HTMLURL
					break
				}
			}
		}
	}
	return actionOutput(result)
}

// getPipelineJobs returns jobs for a specific Gitea Actions run.
func (p *GiteaPlugin) getPipelineJobs(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID     string `json:"repo_id"`
		PipelineID int    `json:"pipeline_id"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	if req.RepoID == "" || req.PipelineID == 0 {
		return nil, fmt.Errorf("repo_id and pipeline_id are required")
	}

	path := fmt.Sprintf("/repos/%s/actions/runs/%d/jobs?limit=50", req.RepoID, req.PipelineID)
	data, err := giteaAPIRequest(ctx, baseURL, token, path)
	if err != nil {
		return nil, fmt.Errorf("list run jobs: %w", err)
	}

	var result struct {
		Jobs []struct {
			ID        int     `json:"id"`
			Name      string  `json:"name"`
			Status    string  `json:"status"`
			HTMLURL   string  `json:"html_url"`
			RunID     int64   `json:"run_id"`
			StartedAt float64 `json:"started_at"`
			StoppedAt float64 `json:"stopped_at"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse jobs: %w", err)
	}

	type jobInfo struct {
		ID           int     `json:"id"`
		Name         string  `json:"name"`
		Stage        string  `json:"stage"`
		Status       string  `json:"status"`
		Ref          string  `json:"ref"`
		AllowFailure bool    `json:"allow_failure"`
		Duration     float64 `json:"duration"`
		Runner       string  `json:"runner"`
		WebURL       string  `json:"web_url"`
		StartedAt    string  `json:"started_at,omitempty"`
		FinishedAt   string  `json:"finished_at,omitempty"`
	}

	jobs := make([]jobInfo, 0, len(result.Jobs))
	for _, j := range result.Jobs {
		status := j.Status
		if status == "done" {
			status = "success"
		}
		info := jobInfo{
			ID:     j.ID,
			Name:   j.Name,
			Status: status,
			WebURL: j.HTMLURL,
		}
		if j.StartedAt > 0 {
			started := time.Unix(int64(j.StartedAt), 0).UTC()
			info.StartedAt = started.Format(time.RFC3339)
			if j.StoppedAt > j.StartedAt {
				stopped := time.Unix(int64(j.StoppedAt), 0).UTC()
				info.FinishedAt = stopped.Format(time.RFC3339)
				info.Duration = stopped.Sub(started).Seconds()
			}
		}
		jobs = append(jobs, info)
	}
	return actionOutput(map[string]any{"jobs": jobs, "total": len(jobs)})
}

// getJobLog returns the log output for a Gitea Actions job.
func (p *GiteaPlugin) getJobLog(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"`
		JobID  int    `json:"job_id"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	if req.RepoID == "" || req.JobID == 0 {
		return nil, fmt.Errorf("repo_id and job_id are required")
	}

	path := fmt.Sprintf("/repos/%s/actions/jobs/%d/logs", req.RepoID, req.JobID)
	data, err := giteaAPIRequest(ctx, baseURL, token, path)
	if err != nil {
		return nil, fmt.Errorf("fetch job log: %w", err)
	}
	return actionOutput(map[string]any{"log": string(data), "job_id": req.JobID})
}

// giteaCIVar represents a CI variable from Gitea Actions workflow_dispatch inputs.
type giteaCIVar struct {
	Key         string   `json:"key"`
	Value       string   `json:"value"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Options     []string `json:"options,omitempty"`
	Required    bool     `json:"required"`
}

// parseCIConfig fetches Gitea Actions workflow files and extracts workflow_dispatch inputs.
// Gitea Actions uses the same format as GitHub Actions.
// Workflow files live in .gitea/workflows/ and .github/workflows/ (both are scanned).
func (p *GiteaPlugin) parseCIConfig(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"` // "owner/repo"
		Ref    string `json:"ref"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	if req.RepoID == "" {
		return nil, fmt.Errorf("repo_id is required (owner/repo format)")
	}

	ref := req.Ref
	if ref == "" {
		ref = defaultBranch(ctx, baseURL, token, req.RepoID)
	}

	varMap := make(map[string]*giteaCIVar)
	workflows := make([]string, 0)
	hasCIFile := false

	for _, f := range listWorkflowFiles(ctx, baseURL, token, req.RepoID, ref) {
		hasCIFile = true
		workflows = append(workflows, f.Name)

		decoded, decErr := fetchWorkflowFile(ctx, baseURL, token, req.RepoID, f.Path, ref)
		if decErr != nil {
			continue
		}

		var workflow map[string]interface{}
		if yaml.Unmarshal(decoded, &workflow) != nil {
			continue
		}

		extractGiteaDispatchInputs(workflow, varMap)
	}

	variables := make([]giteaCIVar, 0, len(varMap))
	for _, v := range varMap {
		variables = append(variables, *v)
	}

	return actionOutput(map[string]any{
		"variables":   variables,
		"workflows":   workflows,
		"has_ci_file": hasCIFile,
	})
}

// extractGiteaDispatchInputs extracts workflow_dispatch inputs from a Gitea Actions workflow.
func extractGiteaDispatchInputs(workflow map[string]interface{}, varMap map[string]*giteaCIVar) {
	onSection, ok := workflow["on"].(map[string]interface{})
	if !ok {
		return
	}
	dispatch, ok := onSection["workflow_dispatch"].(map[string]interface{})
	if !ok {
		return
	}
	inputs, ok := dispatch["inputs"].(map[string]interface{})
	if !ok {
		return
	}
	for name, raw := range inputs {
		if _, exists := varMap[name]; exists {
			continue
		}
		entry := &giteaCIVar{Key: name, Type: "env_var"}
		inputMap, ok := raw.(map[string]interface{})
		if !ok {
			if raw != nil {
				entry.Value = fmt.Sprintf("%v", raw)
			}
			entry.Description = "From Gitea Actions workflow"
			varMap[name] = entry
			continue
		}
		if desc, ok := inputMap["description"]; ok && desc != nil {
			entry.Description = fmt.Sprintf("%v", desc)
		}
		if def, ok := inputMap["default"]; ok && def != nil {
			entry.Value = fmt.Sprintf("%v", def)
		}
		if req, ok := inputMap["required"]; ok {
			if b, ok := req.(bool); ok {
				entry.Required = b
			}
		}
		if t, ok := inputMap["type"]; ok && t != nil {
			inputType := fmt.Sprintf("%v", t)
			if inputType == "choice" {
				if opts, ok := inputMap["options"].([]interface{}); ok {
					for _, o := range opts {
						entry.Options = append(entry.Options, fmt.Sprintf("%v", o))
					}
				}
			}
			if inputType == "boolean" && entry.Value == "" {
				entry.Value = "false"
			}
		}
		if entry.Description == "" {
			entry.Description = "From Gitea Actions workflow"
		}
		varMap[name] = entry
	}
}
