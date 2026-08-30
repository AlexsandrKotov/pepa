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

// githubCIVar represents a CI variable from GitHub Actions workflow_dispatch inputs.
type githubCIVar struct {
	Key         string   `json:"key"`
	Value       string   `json:"value"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Options     []string `json:"options,omitempty"`
	Required    bool     `json:"required"`
}

// githubAPIRequest performs an authenticated GitHub API request.
func githubAPIRequest(ctx context.Context, baseURL, token, path string) ([]byte, error) {
	u := strings.TrimRight(baseURL, "/") + path

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: nil, // use system certs
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("github API returned %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// listGroups returns the authenticated user as a personal group plus any organizations.
// GitHub personal accounts may not have organizations, so we always include the user's
// own account and gracefully handle org listing failures.
func (p *GitHubPlugin) listGroups(ctx context.Context, baseURL, token string) ([]byte, error) {
	type group struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		URL      string `json:"url"`
		Kind     string `json:"kind"`
	}

	// First, get the authenticated user's info
	userData, err := githubAPIRequest(ctx, baseURL, token, "/user")
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	var user struct {
		Login     string `json:"login"`
		ID        int    `json:"id"`
		AvatarURL string `json:"avatar_url"`
		HTMLURL   string `json:"html_url"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(userData, &user); err != nil {
		return nil, fmt.Errorf("parse user: %w", err)
	}

	groups := make([]group, 0)
	// Add the user's personal account as the first group
	displayName := user.Login
	if user.Name != "" {
		displayName = user.Name
	}
	groups = append(groups, group{
		ID:       user.Login,
		Name:     user.Login,
		FullName: displayName + " (personal)",
		URL:      user.HTMLURL,
		Kind:     "user",
	})

	// Try to list organizations; if it fails (e.g. 404 for accounts without orgs), just return personal
	orgData, err := githubAPIRequest(ctx, baseURL, token, "/user/orgs?per_page=100")
	if err == nil {
		var orgs []struct {
			Login     string `json:"login"`
			ID        int    `json:"id"`
			AvatarURL string `json:"avatar_url"`
			URL       string `json:"html_url"`
		}
		if json.Unmarshal(orgData, &orgs) == nil {
			for _, o := range orgs {
				groups = append(groups, group{
					ID:       o.Login,
					Name:     o.Login,
					FullName: o.Login,
					URL:      o.URL,
					Kind:     "organization",
				})
			}
		}
	}

	return actionOutput(map[string]any{"groups": groups, "total": len(groups)})
}

// listRepos returns repositories. If group_id matches the authenticated user, lists user repos;
// if group_id is an org, lists org repos; otherwise lists all user repos.
func (p *GitHubPlugin) listRepos(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		GroupID string `json:"group_id"`
	}
	_ = actionInput(params, &req)

	var path string
	if req.GroupID != "" {
		// Check if group_id is the authenticated user's login (personal account)
		userData, userErr := githubAPIRequest(ctx, baseURL, token, "/user")
		userLogin := ""
		if userErr == nil {
			var u struct {
				Login string `json:"login"`
			}
			if json.Unmarshal(userData, &u) == nil {
				userLogin = u.Login
			}
		}

		if req.GroupID == userLogin {
			// Personal account — use /users/{username}/repos
			path = fmt.Sprintf("/users/%s/repos?per_page=100&sort=updated&type=all", url.PathEscape(req.GroupID))
		} else {
			// Organization — use /orgs/{org}/repos
			path = fmt.Sprintf("/orgs/%s/repos?per_page=100&sort=updated", url.PathEscape(req.GroupID))
		}
	} else {
		path = "/user/repos?per_page=100&sort=updated&type=all"
	}

	data, err := githubAPIRequest(ctx, baseURL, token, path)
	if err != nil {
		return nil, fmt.Errorf("list repos: %w", err)
	}

	var ghRepos []struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Description   string `json:"description"`
		HTMLURL       string `json:"html_url"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
	}
	if err := json.Unmarshal(data, &ghRepos); err != nil {
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
	repos := make([]repo, 0, len(ghRepos))
	for _, r := range ghRepos {
		repos = append(repos, repo{
			ID:            r.FullName,
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
func (p *GitHubPlugin) getBranches(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"` // expects "owner/repo" format
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	if req.RepoID == "" {
		return nil, fmt.Errorf("repo_id is required (owner/repo format)")
	}

	path := fmt.Sprintf("/repos/%s/branches?per_page=100", url.PathEscape(req.RepoID))
	data, err := githubAPIRequest(ctx, baseURL, token, path)
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}

	var ghBranches []struct {
		Name      string `json:"name"`
		Protected bool   `json:"protected"`
		Commit    struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(data, &ghBranches); err != nil {
		return nil, fmt.Errorf("parse branches: %w", err)
	}

	type branch struct {
		Name      string `json:"name"`
		SHA       string `json:"sha"`
		Protected bool   `json:"protected"`
	}
	branches := make([]branch, 0, len(ghBranches))
	for _, b := range ghBranches {
		branches = append(branches, branch{
			Name:      b.Name,
			SHA:       b.Commit.SHA,
			Protected: b.Protected,
		})
	}
	return actionOutput(map[string]any{"branches": branches, "total": len(branches)})
}

// listPipelines returns GitHub Actions workflow runs for a repository.
func (p *GitHubPlugin) listPipelines(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"` // expects "owner/repo" format
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	if req.RepoID == "" {
		return nil, fmt.Errorf("repo_id is required (owner/repo format)")
	}

	path := fmt.Sprintf("/repos/%s/actions/runs?per_page=20", url.PathEscape(req.RepoID))
	data, err := githubAPIRequest(ctx, baseURL, token, path)
	if err != nil {
		return nil, fmt.Errorf("list pipeline runs: %w", err)
	}

	var result struct {
		WorkflowRuns []struct {
			ID         int    `json:"id"`
			HeadSHA    string `json:"head_sha"`
			HeadBranch string `json:"head_branch"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			Name       string `json:"name"`
			HTMLURL    string `json:"html_url"`
			CreatedAt  string `json:"created_at"`
		} `json:"workflow_runs"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse runs: %w", err)
	}

	type pipeline struct {
		ID     int    `json:"id"`
		SHA    string `json:"sha"`
		Ref    string `json:"ref"`
		Status string `json:"status"`
		Source string `json:"source"`
		URL    string `json:"url"`
	}
	pipelines := make([]pipeline, 0, len(result.WorkflowRuns))
	for _, r := range result.WorkflowRuns {
		status := r.Status
		if status == "completed" {
			// GitHub uses conclusion field for completed runs (success, failure, cancelled, etc.)
			status = r.Conclusion
			if status == "" {
				status = "completed"
			}
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

// triggerPipeline creates a workflow dispatch event (triggers a GitHub Actions workflow).
func (p *GitHubPlugin) triggerPipeline(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"` // "owner/repo"
		Ref    string `json:"ref"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	if req.RepoID == "" || req.Ref == "" {
		return nil, fmt.Errorf("repo_id and ref are required")
	}

	// First, get the workflow ID (use the first active workflow)
	workflowsPath := fmt.Sprintf("/repos/%s/actions/workflows", req.RepoID)
	data, err := githubAPIRequest(ctx, baseURL, token, workflowsPath)
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}

	var wfResult struct {
		Workflows []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"workflows"`
	}
	if err := json.Unmarshal(data, &wfResult); err != nil {
		return nil, fmt.Errorf("parse workflows: %w", err)
	}
	if len(wfResult.Workflows) == 0 {
		return nil, fmt.Errorf("no workflows found in %s", req.RepoID)
	}

	// Trigger the first workflow
	workflowID := wfResult.Workflows[0].ID
	dispatchPath := fmt.Sprintf("/repos/%s/actions/workflows/%d/dispatches", url.PathEscape(req.RepoID), workflowID)

	u := strings.TrimRight(baseURL, "/") + dispatchPath
	dispatchBody, err := json.Marshal(map[string]string{"ref": req.Ref})
	if err != nil {
		return nil, fmt.Errorf("marshal dispatch body: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(dispatchBody))
	if err != nil {
		return nil, fmt.Errorf("create dispatch request: %w", err)
	}
	httpReq.Header.Set("Authorization", "token "+token)
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("dispatch workflow: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dispatch failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return actionOutput(map[string]any{
		"workflow_id": workflowID,
		"ref":         req.Ref,
		"status":      "queued",
	})
}

// parseCIConfig fetches GitHub Actions workflow files and extracts workflow_dispatch inputs.
// See https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#onworkflow_dispatchinputs
func (p *GitHubPlugin) parseCIConfig(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
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
		ref = "main"
	}

	type ciVar = githubCIVar
	varMap := make(map[string]*ciVar)
	hasCIFile := false

	// List workflow files from .github/workflows/
	listPath := fmt.Sprintf("/repos/%s/contents/.github/workflows?ref=%s", url.PathEscape(req.RepoID), url.PathEscape(ref))
	data, err := githubAPIRequest(ctx, baseURL, token, listPath)
	if err == nil {
		var files []struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &files) == nil {
			for _, f := range files {
				if f.Type != "file" {
					continue
				}
				if !strings.HasSuffix(f.Name, ".yml") && !strings.HasSuffix(f.Name, ".yaml") {
					continue
				}
				hasCIFile = true

				// Fetch the workflow file content
				contentPath := fmt.Sprintf("/repos/%s/contents/%s?ref=%s", url.PathEscape(req.RepoID), url.PathEscape(f.Path), url.PathEscape(ref))
				contentData, contentErr := githubAPIRequest(ctx, baseURL, token, contentPath)
				if contentErr != nil {
					continue
				}

				var fileContent struct {
					Content  string `json:"content"`
					Encoding string `json:"encoding"`
				}
				if json.Unmarshal(contentData, &fileContent) != nil {
					continue
				}

				decoded, decErr := base64.StdEncoding.DecodeString(strings.ReplaceAll(fileContent.Content, "\n", ""))
				if decErr != nil {
					continue
				}

				// Parse the workflow YAML
				var workflow map[string]interface{}
				if yaml.Unmarshal(decoded, &workflow) != nil {
					continue
				}

				// Extract on: workflow_dispatch: inputs
				extractGitHubDispatchInputs(workflow, varMap)
			}
		}
	}

	// Build result list
	variables := make([]ciVar, 0, len(varMap))
	for _, v := range varMap {
		variables = append(variables, *v)
	}

	return actionOutput(map[string]any{
		"variables":   variables,
		"has_ci_file": hasCIFile,
	})
}

// extractGitHubDispatchInputs extracts workflow_dispatch inputs from a GitHub Actions workflow.
// Format:
//
//	on:
//	  workflow_dispatch:
//	    inputs:
//	      my_input:
//	        description: '...'
//	        required: true
//	        default: 'value'
//	        type: choice
//	        options: [a, b, c]
func extractGitHubDispatchInputs(workflow map[string]interface{}, varMap map[string]*githubCIVar) {
	onSection, ok := workflow["on"].(map[string]interface{})
	if !ok {
		// "on" can also be a string or list in simple cases, but we need the map form
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
		entry := &githubCIVar{Key: name, Type: "string"}
		inputMap, ok := raw.(map[string]interface{})
		if !ok {
			// Simple format: just a value
			if raw != nil {
				entry.Value = fmt.Sprintf("%v", raw)
			}
			entry.Description = fmt.Sprintf("From GitHub Actions workflow")
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
			switch inputType {
			case "choice":
				entry.Type = "env_var"
				if opts, ok := inputMap["options"].([]interface{}); ok {
					for _, o := range opts {
						entry.Options = append(entry.Options, fmt.Sprintf("%v", o))
					}
				}
			case "boolean":
				entry.Type = "env_var"
				if entry.Value == "" {
					entry.Value = "false"
				}
			default:
				entry.Type = "env_var"
			}
		} else {
			entry.Type = "env_var"
		}
		if entry.Description == "" {
			entry.Description = "From GitHub Actions workflow"
		}
		varMap[name] = entry
	}
}
