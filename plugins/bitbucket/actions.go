package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// bitbucketAPIRequest performs an authenticated Bitbucket API request.
func bitbucketAPIRequest(ctx context.Context, baseURL, token, path string) ([]byte, error) {
	u := strings.TrimRight(baseURL, "/") + path

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: nil,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bitbucket request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("bitbucket API returned %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// listGroups returns workspaces the authenticated user has access to.
func (p *BitbucketPlugin) listGroups(ctx context.Context, baseURL, token string) ([]byte, error) {
	data, err := bitbucketAPIRequest(ctx, baseURL, token, "/workspaces?pagelen=50")
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}

	var result struct {
		Values []struct {
			UUID  string `json:"uuid"`
			Slug  string `json:"slug"`
			Name  string `json:"name"`
			Links struct {
				HTML struct {
					Href string `json:"href"`
				} `json:"html"`
			} `json:"links"`
		} `json:"values"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse workspaces: %w", err)
	}

	type group struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		URL      string `json:"url"`
		Kind     string `json:"kind"`
	}
	groups := make([]group, 0, len(result.Values))
	for _, w := range result.Values {
		groups = append(groups, group{
			ID:       w.UUID,
			Name:     w.Name,
			FullName: w.Slug,
			URL:      w.Links.HTML.Href,
			Kind:     "workspace",
		})
	}
	return actionOutput(map[string]any{"groups": groups, "total": len(groups)})
}

// listRepos returns repositories. If group_id (workspace slug) is provided, lists workspace repos.
func (p *BitbucketPlugin) listRepos(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		GroupID string `json:"group_id"` // workspace slug
	}
	_ = actionInput(params, &req)

	var path string
	if req.GroupID != "" {
		path = fmt.Sprintf("/repositories/%s?pagelen=50", url.PathEscape(req.GroupID))
	} else {
		// List repos for all workspaces (user's repos)
		path = "/repositories?pagelen=50&role=member"
	}

	data, err := bitbucketAPIRequest(ctx, baseURL, token, path)
	if err != nil {
		return nil, fmt.Errorf("list repos: %w", err)
	}

	var result struct {
		Values []struct {
			UUID        string `json:"uuid"`
			Name        string `json:"name"`
			FullName    string `json:"full_name"`
			Description string `json:"description"`
			IsPrivate   bool   `json:"is_private"`
			Links       struct {
				HTML struct {
					Href string `json:"href"`
				} `json:"html"`
			} `json:"links"`
			MainBranch *struct {
				Name string `json:"name"`
			} `json:"mainbranch"`
		} `json:"values"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
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
	repos := make([]repo, 0, len(result.Values))
	for _, r := range result.Values {
		defaultBranch := "main"
		if r.MainBranch != nil && r.MainBranch.Name != "" {
			defaultBranch = r.MainBranch.Name
		}
		repos = append(repos, repo{
			ID:            r.UUID,
			Name:          r.Name,
			FullName:      r.FullName,
			Description:   r.Description,
			URL:           r.Links.HTML.Href,
			DefaultBranch: defaultBranch,
			Private:       r.IsPrivate,
		})
	}
	return actionOutput(map[string]any{"repos": repos, "total": len(repos)})
}

// getBranches returns branches for a repository.
func (p *BitbucketPlugin) getBranches(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"` // "workspace/repo" format
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	if req.RepoID == "" {
		return nil, fmt.Errorf("repo_id is required (workspace/repo format)")
	}

	path := fmt.Sprintf("/repositories/%s/refs/branches?pagelen=50", req.RepoID)
	data, err := bitbucketAPIRequest(ctx, baseURL, token, path)
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}

	var result struct {
		Values []struct {
			Name   string `json:"name"`
			Target struct {
				Hash string `json:"hash"`
			} `json:"target"`
		} `json:"values"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse branches: %w", err)
	}

	type branch struct {
		Name      string `json:"name"`
		SHA       string `json:"sha"`
		Protected bool   `json:"protected"`
	}
	branches := make([]branch, 0, len(result.Values))
	for _, b := range result.Values {
		branches = append(branches, branch{
			Name: b.Name,
			SHA:  b.Target.Hash,
		})
	}
	return actionOutput(map[string]any{"branches": branches, "total": len(branches)})
}

// listPipelines returns Bitbucket Pipelines for a repository.
func (p *BitbucketPlugin) listPipelines(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"` // "workspace/repo" format
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	if req.RepoID == "" {
		return nil, fmt.Errorf("repo_id is required (workspace/repo format)")
	}

	path := fmt.Sprintf("/repositories/%s/pipelines/?pagelen=20&sort=-created_on", req.RepoID)
	data, err := bitbucketAPIRequest(ctx, baseURL, token, path)
	if err != nil {
		return nil, fmt.Errorf("list pipelines: %w", err)
	}

	var result struct {
		Values []struct {
			UUID    string `json:"uuid"`
			BuildID int    `json:"build_number"`
			State   struct {
				Name   string `json:"name"` // PENDING, RUNNING, SUCCESSFUL, FAILED, STOPPED, EXPIRED, ERROR
				Result struct {
					Name string `json:"name"`
				} `json:"result"`
			} `json:"state"`
			Target struct {
				RefName string `json:"ref_name"`
				Commit  struct {
					Hash string `json:"hash"`
				} `json:"commit"`
			} `json:"target"`
		} `json:"values"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse pipelines: %w", err)
	}

	type pipeline struct {
		ID     int    `json:"id"`
		SHA    string `json:"sha"`
		Ref    string `json:"ref"`
		Status string `json:"status"`
		Source string `json:"source"`
	}
	pipelines := make([]pipeline, 0, len(result.Values))
	for _, p := range result.Values {
		status := strings.ToLower(p.State.Name)
		if p.State.Result.Name != "" {
			status = strings.ToLower(p.State.Result.Name)
		}
		pipelines = append(pipelines, pipeline{
			ID:     p.BuildID,
			SHA:    p.Target.Commit.Hash,
			Ref:    p.Target.RefName,
			Status: status,
		})
	}
	return actionOutput(map[string]any{"pipelines": pipelines, "total": len(pipelines)})
}

// bitbucketCIVar represents a CI variable from bitbucket-pipelines.yml.
type bitbucketCIVar struct {
	Key         string   `json:"key"`
	Value       string   `json:"value"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Options     []string `json:"options,omitempty"`
}

// parseCIConfig fetches bitbucket-pipelines.yml and extracts top-level variables.
// See https://support.atlassian.com/bitbucket-cloud/docs/variables-in-pipelines/
func (p *BitbucketPlugin) parseCIConfig(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"` // "workspace/repo"
		Ref    string `json:"ref"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	if req.RepoID == "" {
		return nil, fmt.Errorf("repo_id is required (workspace/repo format)")
	}

	ref := req.Ref
	if ref == "" {
		ref = "main"
	}

	varMap := make(map[string]*bitbucketCIVar)
	hasCIFile := false

	// Fetch bitbucket-pipelines.yml via the src endpoint
	srcPath := fmt.Sprintf("/repositories/%s/src/%s/bitbucket-pipelines.yml", url.PathEscape(req.RepoID), url.PathEscape(ref))
	data, err := bitbucketAPIRequest(ctx, baseURL, token, srcPath)
	if err == nil {
		hasCIFile = true
		var ciConfig map[string]interface{}
		if yaml.Unmarshal(data, &ciConfig) == nil {
			// Extract top-level variables
			// Bitbucket format:
			//   variables:
			//     - name: MY_VAR
			//       default: "value"
			// Or the simpler map format:
			//   variables:
			//     MY_VAR: "value"
			if vars, ok := ciConfig["variables"]; ok {
				switch v := vars.(type) {
				case []interface{}:
					// List format: [{name: "KEY", default: "value"}, ...]
					for _, item := range v {
						if m, ok := item.(map[string]interface{}); ok {
							name, _ := m["name"].(string)
							if name == "" {
								continue
							}
							entry := &bitbucketCIVar{Key: name, Type: "env_var", Description: "From bitbucket-pipelines.yml"}
							if def, ok := m["default"]; ok && def != nil {
								entry.Value = fmt.Sprintf("%v", def)
							}
							varMap[name] = entry
						}
					}
				case map[string]interface{}:
					// Map format: {KEY: "value", ...}
					for k, val := range v {
						entry := &bitbucketCIVar{Key: k, Type: "env_var", Description: "From bitbucket-pipelines.yml"}
						if val != nil {
							entry.Value = fmt.Sprintf("%v", val)
						}
						varMap[k] = entry
					}
				}
			}
		}
	}

	variables := make([]bitbucketCIVar, 0, len(varMap))
	for _, v := range varMap {
		variables = append(variables, *v)
	}

	return actionOutput(map[string]any{
		"variables":   variables,
		"has_ci_file": hasCIFile,
	})
}
