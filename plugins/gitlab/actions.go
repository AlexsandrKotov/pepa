package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/xanzy/go-gitlab"
	"gopkg.in/yaml.v3"
)

// gitlabCIVar represents a CI variable from any source.
type gitlabCIVar struct {
	Key         string   `json:"key"`
	Value       string   `json:"value"`
	Description string   `json:"description"`
	Type        string   `json:"type"` // "env_var", "file"
	Options     []string `json:"options,omitempty"`
	IsInput     bool     `json:"is_input,omitempty"` // true for spec.inputs (component pipeline inputs)
}

// newClient creates a GitLab API client from connection config.
func newClient(baseURL, token string) (*gitlab.Client, error) {
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

// listGroups returns groups/subgroups the token has access to.
// If parent_id is provided, lists subgroups of that group; otherwise lists top-level groups.
func (p *GitLabPlugin) listGroups(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	client, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}

	var req struct {
		ParentID string `json:"parent_id"`
	}
	_ = actionInput(params, &req)

	type group struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		URL      string `json:"url"`
		Kind     string `json:"kind"`
	}

	if req.ParentID != "" {
		// List subgroups of a specific group
		pid, err := strconv.Atoi(req.ParentID)
		if err != nil {
			return nil, fmt.Errorf("invalid parent_id: %w", err)
		}
		subgroups, _, err := client.Groups.ListSubGroups(pid, &gitlab.ListSubGroupsOptions{
			ListOptions: gitlab.ListOptions{PerPage: 50},
		}, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list subgroups: %w", err)
		}
		groups := make([]group, 0, len(subgroups))
		for _, g := range subgroups {
			groups = append(groups, group{
				ID:       strconv.Itoa(g.ID),
				Name:     g.Name,
				FullName: g.FullPath,
				URL:      g.WebURL,
				Kind:     "group",
			})
		}
		return actionOutput(map[string]any{"groups": groups, "total": len(groups)})
	}

	// List top-level groups the user has access to
	opts := &gitlab.ListGroupsOptions{
		ListOptions:  gitlab.ListOptions{PerPage: 50},
		TopLevelOnly: gitlab.Ptr(true),
	}
	gitlabGroups, _, err := client.Groups.ListGroups(opts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	groups := make([]group, 0, len(gitlabGroups))
	for _, g := range gitlabGroups {
		groups = append(groups, group{
			ID:       strconv.Itoa(g.ID),
			Name:     g.Name,
			FullName: g.FullPath,
			URL:      g.WebURL,
			Kind:     "group",
		})
	}
	return actionOutput(map[string]any{"groups": groups, "total": len(groups)})
}

// listRepos returns all projects the token has access to.
// If group_id is provided, lists projects within that group; otherwise lists all accessible projects.
func (p *GitLabPlugin) listRepos(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	client, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}

	var req struct {
		GroupID string `json:"group_id"`
	}
	_ = actionInput(params, &req)

	var projects []*gitlab.Project
	if req.GroupID != "" {
		gid, err := strconv.Atoi(req.GroupID)
		if err != nil {
			return nil, fmt.Errorf("invalid group_id: %w", err)
		}
		groupOpts := &gitlab.ListGroupProjectsOptions{
			ListOptions: gitlab.ListOptions{PerPage: 50},
		}
		projects, _, err = client.Groups.ListGroupProjects(gid, groupOpts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list group projects: %w", err)
		}
	} else {
		opts := &gitlab.ListProjectsOptions{
			ListOptions: gitlab.ListOptions{PerPage: 50},
			Membership:  gitlab.Ptr(true),
		}
		projects, _, err = client.Projects.ListProjects(opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
		}
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
	repos := make([]repo, 0, len(projects))
	for _, pr := range projects {
		repos = append(repos, repo{
			ID:            strconv.Itoa(pr.ID),
			Name:          pr.Name,
			FullName:      pr.PathWithNamespace,
			Description:   pr.Description,
			URL:           pr.WebURL,
			DefaultBranch: pr.DefaultBranch,
			Private:       pr.Visibility == gitlab.PrivateVisibility,
		})
	}
	return actionOutput(map[string]any{"repos": repos, "total": len(repos)})
}

// getBranches returns branches for a project.
func (p *GitLabPlugin) getBranches(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	pid, err := strconv.Atoi(req.RepoID)
	if err != nil {
		return nil, fmt.Errorf("invalid repo_id: %w", err)
	}

	client, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}

	branches, _, err := client.Branches.ListBranches(pid, &gitlab.ListBranchesOptions{}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}

	type branch struct {
		Name      string `json:"name"`
		SHA       string `json:"sha"`
		Protected bool   `json:"protected"`
	}
	result := make([]branch, 0, len(branches))
	for _, b := range branches {
		sha := ""
		if b.Commit != nil {
			sha = b.Commit.ID
		}
		result = append(result, branch{Name: b.Name, SHA: sha, Protected: b.Protected})
	}
	return actionOutput(map[string]any{"branches": result, "total": len(result)})
}

// createBranch creates a new branch in a project.
func (p *GitLabPlugin) createBranch(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"`
		Name   string `json:"name"`
		Ref    string `json:"ref"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	pid, err := strconv.Atoi(req.RepoID)
	if err != nil {
		return nil, fmt.Errorf("invalid repo_id: %w", err)
	}

	client, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}

	branch, _, err := client.Branches.CreateBranch(pid, &gitlab.CreateBranchOptions{
		Branch: gitlab.Ptr(req.Name),
		Ref:    gitlab.Ptr(req.Ref),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create branch: %w", err)
	}

	sha := ""
	if branch.Commit != nil {
		sha = branch.Commit.ID
	}
	return actionOutput(map[string]any{
		"name":      branch.Name,
		"sha":       sha,
		"protected": branch.Protected,
	})
}

// createMR creates a merge request.
func (p *GitLabPlugin) createMR(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID       string `json:"repo_id"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	pid, err := strconv.Atoi(req.RepoID)
	if err != nil {
		return nil, fmt.Errorf("invalid repo_id: %w", err)
	}

	client, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}

	mr, _, err := client.MergeRequests.CreateMergeRequest(pid, &gitlab.CreateMergeRequestOptions{
		Title:        gitlab.Ptr(req.Title),
		Description:  gitlab.Ptr(req.Description),
		SourceBranch: gitlab.Ptr(req.SourceBranch),
		TargetBranch: gitlab.Ptr(req.TargetBranch),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create merge request: %w", err)
	}

	return actionOutput(map[string]any{
		"id":            mr.IID,
		"title":         mr.Title,
		"source_branch": mr.SourceBranch,
		"target_branch": mr.TargetBranch,
		"state":         mr.State,
		"url":           mr.WebURL,
	})
}

// getMR gets a single merge request by IID.
func (p *GitLabPlugin) getMR(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"`
		MRIID  int    `json:"mr_iid"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	pid, err := strconv.Atoi(req.RepoID)
	if err != nil {
		return nil, fmt.Errorf("invalid repo_id: %w", err)
	}

	client, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}

	mr, _, err := client.MergeRequests.GetMergeRequest(pid, req.MRIID, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get merge request: %w", err)
	}

	author := ""
	if mr.Author != nil {
		author = mr.Author.Username
	}
	return actionOutput(map[string]any{
		"id":            mr.IID,
		"title":         mr.Title,
		"description":   mr.Description,
		"source_branch": mr.SourceBranch,
		"target_branch": mr.TargetBranch,
		"state":         mr.State,
		"author":        author,
		"url":           mr.WebURL,
	})
}

// listPipelines returns recent pipelines for a project.
func (p *GitLabPlugin) listPipelines(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	pid, err := strconv.Atoi(req.RepoID)
	if err != nil {
		return nil, fmt.Errorf("invalid repo_id: %w", err)
	}

	client, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}

	pipelines, _, err := client.Pipelines.ListProjectPipelines(pid, &gitlab.ListProjectPipelinesOptions{
		ListOptions: gitlab.ListOptions{PerPage: 20},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list pipelines: %w", err)
	}

	type pipeline struct {
		ID     int    `json:"id"`
		SHA    string `json:"sha"`
		Ref    string `json:"ref"`
		Status string `json:"status"`
		Source string `json:"source"`
		URL    string `json:"url"`
	}
	result := make([]pipeline, 0, len(pipelines))
	for _, p := range pipelines {
		result = append(result, pipeline{
			ID: p.ID, SHA: p.SHA, Ref: p.Ref, Status: string(p.Status), Source: p.Source, URL: p.WebURL,
		})
	}
	return actionOutput(map[string]any{"pipelines": result, "total": len(result)})
}

// triggerPipeline triggers a new pipeline on a given ref with optional CI variables.
// Supports passing spec.inputs via the "inputs" field (GitLab 17.10+).
func (p *GitLabPlugin) triggerPipeline(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID    string            `json:"repo_id"`
		Ref       string            `json:"ref"`
		Variables map[string]string `json:"variables,omitempty"`
		Inputs    map[string]string `json:"inputs,omitempty"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}

	ref := req.Ref
	if ref == "" {
		ref = "main"
	}

	// When inputs are present, use a direct HTTP request to support the "inputs"
	// field in the API body (go-gitlab v0.115 CreatePipelineOptions lacks it).
	if len(req.Inputs) > 0 {
		return triggerPipelineWithInputs(ctx, baseURL, token, req.RepoID, ref, req.Variables, req.Inputs)
	}

	pid, err := strconv.Atoi(req.RepoID)
	if err != nil {
		return nil, fmt.Errorf("invalid repo_id: %w", err)
	}

	client, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}

	// Build CI variables from request
	var variables []*gitlab.PipelineVariableOptions
	for k, v := range req.Variables {
		variables = append(variables, &gitlab.PipelineVariableOptions{
			Key:   gitlab.Ptr(k),
			Value: gitlab.Ptr(v),
		})
	}

	pipeline, _, err := client.Pipelines.CreatePipeline(pid, &gitlab.CreatePipelineOptions{
		Ref:       gitlab.Ptr(ref),
		Variables: &variables,
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("trigger pipeline: %w", err)
	}

	return actionOutput(map[string]any{
		"id":     pipeline.ID,
		"sha":    pipeline.SHA,
		"ref":    pipeline.Ref,
		"status": string(pipeline.Status),
		"url":    pipeline.WebURL,
	})
}

// triggerPipelineWithInputs triggers a GitLab pipeline passing spec.inputs
// via the "inputs" field in the API request body (GitLab 17.10+).
func triggerPipelineWithInputs(ctx context.Context, baseURL, token, repoID, ref string, variables map[string]string, inputs map[string]string) ([]byte, error) {
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/pipeline", baseURL, url.PathEscape(repoID))

	body := map[string]interface{}{"ref": ref}

	if len(variables) > 0 {
		vars := make([]map[string]string, 0, len(variables))
		for k, v := range variables {
			vars = append(vars, map[string]string{"key": k, "value": v})
		}
		body["variables"] = vars
	}

	// Convert inputs to map[string]interface{} for JSON marshalling.
	inputsAny := make(map[string]interface{}, len(inputs))
	for k, v := range inputs {
		inputsAny[k] = v
	}
	body["inputs"] = inputsAny

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal pipeline request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create pipeline request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("PRIVATE-TOKEN", token)

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("trigger pipeline: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read pipeline response: %w", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, fmt.Errorf("trigger pipeline (%d): %s", httpResp.StatusCode, string(respBody))
	}

	var result struct {
		ID     int    `json:"id"`
		SHA    string `json:"sha"`
		Ref    string `json:"ref"`
		Status string `json:"status"`
		WebURL string `json:"web_url"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse pipeline response: %w", err)
	}

	return actionOutput(map[string]any{
		"id":     result.ID,
		"sha":    result.SHA,
		"ref":    result.Ref,
		"status": result.Status,
		"url":    result.WebURL,
	})
}

// parseCIConfig fetches .gitlab-ci.yml and project-level CI variables.
// Returns variable names, descriptions, default values, types, and options.
// Handles both simple (KEY: value) and expanded (KEY: {value, description, options}) variable formats
// as documented at https://docs.gitlab.com/ee/ci/variables/
func (p *GitLabPlugin) parseCIConfig(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"`
		Ref    string `json:"ref"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	pid, err := strconv.Atoi(req.RepoID)
	if err != nil {
		return nil, fmt.Errorf("invalid repo_id: %w", err)
	}

	client, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}

	ref := req.Ref
	if ref == "" {
		ref = "main"
	}

	// Collect variables from both .gitlab-ci.yml and project-level CI variables
	varMap := make(map[string]*gitlabCIVar)

	// 1. Parse .gitlab-ci.yml top-level variables
	// GitLab supports two variable formats:
	//   Simple:   MY_VAR: "value"
	//   Expanded: MY_VAR:
	//               value: "value"
	//               description: "What this does"
	//               options: ["opt1", "opt2"]
	file, _, fileErr := client.RepositoryFiles.GetFile(pid, ".gitlab-ci.yml", &gitlab.GetFileOptions{
		Ref: gitlab.Ptr(ref),
	}, gitlab.WithContext(ctx))
	if fileErr == nil && file.Content != "" {
		decoded, decErr := base64.StdEncoding.DecodeString(file.Content)
		if decErr == nil {
			var ciConfig map[string]interface{}
			if yaml.Unmarshal(decoded, &ciConfig) == nil {
				// Debug: log top-level keys
				keys := make([]string, 0, len(ciConfig))
				for k := range ciConfig {
					keys = append(keys, k)
				}
				log.Printf("[parseCIConfig] top-level keys: %v", keys)
				if vars, ok := ciConfig["variables"].(map[string]interface{}); ok {
					for k, v := range vars {
						ciVarEntry := parseGitLabVariable(k, v)
						varMap[k] = ciVarEntry
					}
				}
				// Also extract workflow:rules:variables for conditional pipeline variables
				extractWorkflowVariables(ciConfig, varMap)
				// Parse spec.inputs (component pipeline inputs for Terraform, Ansible, etc.)
				extractSpecInputs(ciConfig, varMap)
			} else {
				log.Printf("[parseCIConfig] YAML unmarshal failed")
			}
		} else {
			log.Printf("[parseCIConfig] base64 decode failed: %v", decErr)
		}
	} else {
		log.Printf("[parseCIConfig] file fetch error: %v", fileErr)
	}

	// 2. Fetch project-level CI variables to enrich existing ones with descriptions/types.
	// Only enrich variables already found in .gitlab-ci.yml; do not add new ones
	// to avoid showing irrelevant project-level variables.
	projVars, _, projErr := client.ProjectVariables.ListVariables(pid, &gitlab.ListProjectVariablesOptions{}, gitlab.WithContext(ctx))
	if projErr == nil {
		for _, v := range projVars {
			if existing, ok := varMap[v.Key]; ok {
				// Project variable enriches .gitlab-ci.yml variable with description/type
				if v.Description != "" {
					existing.Description = v.Description
				}
				existing.Type = string(v.VariableType)
				if v.Value != "" {
					existing.Value = v.Value
				}
			}
		}
	}

	// Build result list
	variables := make([]gitlabCIVar, 0, len(varMap))
	for _, v := range varMap {
		variables = append(variables, *v)
	}

	hasCIFile := fileErr == nil

	return actionOutput(map[string]any{
		"variables":   variables,
		"has_ci_file": hasCIFile,
	})
}

// parseGitLabVariable parses a single GitLab CI variable.
// Handles both simple string values and expanded object format:
//
//	Simple:   MY_VAR: "value"
//	Expanded: MY_VAR: {value: "...", description: "...", options: [...], type: "..."}
func parseGitLabVariable(key string, raw interface{}) *gitlabCIVar {
	switch v := raw.(type) {
	case map[string]interface{}:
		// Expanded format: {value: "...", description: "...", options: [...]}
		entry := &gitlabCIVar{Key: key, Type: "env_var"}
		if val, ok := v["value"]; ok && val != nil {
			entry.Value = fmt.Sprintf("%v", val)
		}
		if desc, ok := v["description"]; ok && desc != nil {
			entry.Description = fmt.Sprintf("%v", desc)
		}
		if opts, ok := v["options"]; ok {
			if optList, ok := opts.([]interface{}); ok {
				for _, o := range optList {
					entry.Options = append(entry.Options, fmt.Sprintf("%v", o))
				}
			}
		}
		if t, ok := v["type"]; ok && t != nil {
			entry.Type = fmt.Sprintf("%v", t)
		}
		if entry.Description == "" {
			entry.Description = "From .gitlab-ci.yml"
		}
		return entry
	default:
		// Simple format: MY_VAR: "value"
		val := ""
		if raw != nil {
			val = fmt.Sprintf("%v", raw)
		}
		return &gitlabCIVar{
			Key:         key,
			Value:       val,
			Description: "From .gitlab-ci.yml",
			Type:        "env_var",
		}
	}
}

// extractWorkflowVariables extracts variables from workflow:rules:variables section.
// These are conditional variables that can be overridden when triggering a pipeline.
func extractWorkflowVariables(ciConfig map[string]interface{}, varMap map[string]*gitlabCIVar) {
	workflow, ok := ciConfig["workflow"].(map[string]interface{})
	if !ok {
		return
	}
	rules, ok := workflow["rules"].([]interface{})
	if !ok {
		return
	}
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}
		vars, ok := ruleMap["variables"].(map[string]interface{})
		if !ok {
			continue
		}
		for k, v := range vars {
			if _, exists := varMap[k]; !exists {
				varMap[k] = &gitlabCIVar{
					Key:         k,
					Value:       fmt.Sprintf("%v", v),
					Description: "From workflow:rules:variables",
					Type:        "env_var",
				}
			}
		}
	}
}

// extractSpecInputs parses the spec.inputs section from a GitLab CI config
// and adds them to the variable map. Component pipelines (Terraform, Ansible, etc.)
// declare typed inputs like:
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
func extractSpecInputs(ciConfig map[string]interface{}, varMap map[string]*gitlabCIVar) {
	spec, ok := ciConfig["spec"].(map[string]interface{})
	if !ok {
		log.Printf("[extractSpecInputs] no 'spec' key found")
		return
	}
	log.Printf("[extractSpecInputs] found 'spec' key, keys: %v", mapKeys(spec))
	inputs, ok := spec["inputs"].(map[string]interface{})
	if !ok {
		log.Printf("[extractSpecInputs] no 'spec.inputs' key found")
		return
	}
	log.Printf("[extractSpecInputs] found %d inputs", len(inputs))
	for name, raw := range inputs {
		// Skip if already defined as a regular variable
		if _, exists := varMap[name]; exists {
			continue
		}
		input, ok := raw.(map[string]interface{})
		if !ok {
			// Simple value used as default
			varMap[name] = &gitlabCIVar{
				Key:         name,
				Value:       fmt.Sprintf("%v", raw),
				Description: fmt.Sprintf("Pipeline input: %s", name),
				Type:        "env_var",
				IsInput:     true,
			}
			continue
		}
		entry := &gitlabCIVar{
			Key:     name,
			Type:    "env_var",
			IsInput: true,
		}
		if desc, ok := input["description"].(string); ok {
			entry.Description = desc
		} else {
			entry.Description = fmt.Sprintf("Pipeline input: %s", name)
		}
		if def, ok := input["default"]; ok && def != nil {
			switch v := def.(type) {
			case bool:
				entry.Value = fmt.Sprintf("%v", v)
			case float64:
				entry.Value = fmt.Sprintf("%v", v)
			default:
				entry.Value = fmt.Sprintf("%v", def)
			}
		}
		if opts, ok := input["options"].([]interface{}); ok {
			for _, o := range opts {
				entry.Options = append(entry.Options, fmt.Sprintf("%v", o))
			}
		}
		varMap[name] = entry
	}
}

// getPipelineJobs returns jobs for a specific pipeline.
func (p *GitLabPlugin) getPipelineJobs(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID     string `json:"repo_id"`
		PipelineID int    `json:"pipeline_id"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	pid, err := strconv.Atoi(req.RepoID)
	if err != nil {
		return nil, fmt.Errorf("invalid repo_id: %w", err)
	}

	client, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}

	jobs, _, err := client.Jobs.ListPipelineJobs(pid, req.PipelineID, &gitlab.ListJobsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list pipeline jobs: %w", err)
	}

	type jobInfo struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		Stage        string `json:"stage"`
		Status       string `json:"status"`
		Ref          string `json:"ref"`
		AllowFailure bool   `json:"allow_failure"`
		Duration     float64 `json:"duration"`
		Runner       string `json:"runner"`
		WebURL       string `json:"web_url"`
		StartedAt    string `json:"started_at,omitempty"`
		FinishedAt   string `json:"finished_at,omitempty"`
	}

	result := make([]jobInfo, 0, len(jobs))
	for _, j := range jobs {
		runner := j.Runner.Description
		startedAt := ""
		if j.StartedAt != nil {
			startedAt = j.StartedAt.String()
		}
		finishedAt := ""
		if j.FinishedAt != nil {
			finishedAt = j.FinishedAt.String()
		}
		result = append(result, jobInfo{
			ID:           j.ID,
			Name:         j.Name,
			Stage:        j.Stage,
			Status:       string(j.Status),
			Ref:          j.Ref,
			AllowFailure: j.AllowFailure,
			Duration:     j.Duration,
			Runner:       runner,
			WebURL:       j.WebURL,
			StartedAt:    startedAt,
			FinishedAt:   finishedAt,
		})
	}

	return actionOutput(map[string]any{"jobs": result, "total": len(result)})
}

// getJobLog retrieves the trace/log output for a specific job.
func (p *GitLabPlugin) getJobLog(ctx context.Context, baseURL, token string, params []byte) ([]byte, error) {
	var req struct {
		RepoID string `json:"repo_id"`
		JobID  int    `json:"job_id"`
	}
	if err := actionInput(params, &req); err != nil {
		return nil, err
	}
	pid, err := strconv.Atoi(req.RepoID)
	if err != nil {
		return nil, fmt.Errorf("invalid repo_id: %w", err)
	}

	client, err := newClient(baseURL, token)
	if err != nil {
		return nil, err
	}

	reader, _, err := client.Jobs.GetTraceFile(pid, req.JobID, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get trace file: %w", err)
	}
	if reader == nil {
		return actionOutput(map[string]any{"log": "", "job_id": req.JobID})
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read trace: %w", err)
	}

	return actionOutput(map[string]any{"log": string(data), "job_id": req.JobID})
}

// mapKeys returns the keys of a map as a sorted slice.
func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
