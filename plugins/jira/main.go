// PEPA Jira Plugin — implements TaskTrackerProvider.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	jira "github.com/andygrunwald/go-jira"
	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// JiraPlugin implements provider.Provider for Jira.
type JiraPlugin struct {
	client *jira.Client
	config map[string]string
}

func NewJiraPlugin(config map[string]string) (*JiraPlugin, error) {
	baseURL := config["base_url"]
	username := config["username"]
	token := config["api_token"]
	if baseURL == "" || token == "" {
		return nil, fmt.Errorf("jira plugin requires base_url and api_token")
	}

	tp := jira.BasicAuthTransport{
		Username: username,
		Password: token,
	}

	// Ensure URL ends without slash
	baseURL = strings.TrimRight(baseURL, "/")

	client, err := jira.NewClient(tp.Client(), baseURL)
	if err != nil {
		return nil, fmt.Errorf("create jira client: %w", err)
	}

	return &JiraPlugin{client: client, config: config}, nil
}

func (p *JiraPlugin) Name() string    { return "jira" }
func (p *JiraPlugin) Version() string { return "2.0.0" }
func (p *JiraPlugin) Description() string {
	return "Jira integration — issues, projects, transitions, comments, automation"
}
func (p *JiraPlugin) PluginType() string { return "task_tracker" }

func (p *JiraPlugin) Actions() []string {
	return []string{
		"list_projects",
		"list_issues",
		"search_issues",
		"get_issue",
		"create_issue",
		"update_issue",
		"delete_issue",
		"transition_issue",
		"list_transitions",
		"add_comment",
		"list_comments",
		"list_labels",
		"get_project_meta",
		"link_mr",
		"notify_deployment",
		"list_assignees",
		"list_sprints",
		"add_worklog",
		"list_worklogs",
		"link_issues",
		"create_subtask",
		"sync_issues",
		"list_components",
		"list_versions",
	}
}

func (p *JiraPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	// Allow per-request config override
	if config != nil && config["api_token"] != "" {
		plugin, err := NewJiraPlugin(config)
		if err != nil {
			return nil, err
		}
		return plugin.Execute(ctx, action, params, nil)
	}

	// Lazy init client if not yet created
	if p.client == nil && p.config != nil {
		plugin, err := NewJiraPlugin(p.config)
		if err != nil {
			return nil, err
		}
		p.client = plugin.client
	}

	switch action {
	case "list_projects":
		return p.listProjects(ctx)
	case "list_issues":
		return p.listIssues(ctx, params)
	case "search_issues":
		return p.searchIssues(ctx, params)
	case "get_issue":
		return p.getIssue(ctx, params)
	case "create_issue":
		return p.createIssue(ctx, params)
	case "update_issue":
		return p.updateIssue(ctx, params)
	case "delete_issue":
		return p.deleteIssue(ctx, params)
	case "transition_issue":
		return p.transitionIssue(ctx, params)
	case "list_transitions":
		return p.listTransitions(ctx, params)
	case "add_comment":
		return p.addComment(ctx, params)
	case "list_comments":
		return p.listComments(ctx, params)
	case "list_labels":
		return p.listLabels(ctx, params)
	case "get_project_meta":
		return p.getProjectMeta(ctx, params)
	case "link_mr":
		return p.linkMR(ctx, params)
	case "notify_deployment":
		return p.notifyDeployment(ctx, params)
	case "list_assignees":
		return p.listAssignees(ctx, params)
	case "list_sprints":
		return p.listSprints(ctx, params)
	case "add_worklog":
		return p.addWorklog(ctx, params)
	case "list_worklogs":
		return p.listWorklogs(ctx, params)
	case "link_issues":
		return p.linkIssues(ctx, params)
	case "create_subtask":
		return p.createSubtask(ctx, params)
	case "sync_issues":
		return p.syncIssues(ctx, params)
	case "list_components":
		return p.listComponents(ctx, params)
	case "list_versions":
		return p.listVersions(ctx, params)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *JiraPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	if p.client == nil {
		return &provider.HealthStatus{Status: "unhealthy", Message: "jira client not configured"}, nil
	}
	start := time.Now()
	_, resp, err := p.client.Project.GetList()
	latency := time.Since(start)
	if err != nil {
		return &provider.HealthStatus{Status: "unhealthy", Message: fmt.Sprintf("jira unreachable: %v", err), LatencyMs: latency.Milliseconds()}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return &provider.HealthStatus{Status: "degraded", Message: fmt.Sprintf("jira status %d", resp.StatusCode), LatencyMs: latency.Milliseconds()}, nil
	}
	return &provider.HealthStatus{Status: "healthy", Message: "connected to jira", LatencyMs: latency.Milliseconds()}, nil
}

// ── Helper: enrich issue with all fields ──────────────────────

func (p *JiraPlugin) enrichIssue(iss *jira.Issue) provider.Issue {
	result := provider.Issue{
		ID:       iss.ID,
		Key:      iss.Key,
		Summary:  iss.Fields.Summary,
		Status:   iss.Fields.Status.Name,
		Priority: iss.Fields.Priority.Name,
		Type:     iss.Fields.Type.Name,
	}

	if iss.Fields.Description != "" {
		result.Description = iss.Fields.Description
	}
	if iss.Fields.Assignee != nil {
		result.Assignee = iss.Fields.Assignee.DisplayName
	}
	if iss.Fields.Reporter != nil {
		result.Reporter = iss.Fields.Reporter.DisplayName
	}
	if len(iss.Fields.Labels) > 0 {
		result.Labels = iss.Fields.Labels
	}
	// Extract extra fields
	result.Fields = make(map[string]string)
	if len(iss.Fields.Components) > 0 {
		components := make([]string, len(iss.Fields.Components))
		for i, c := range iss.Fields.Components {
			components[i] = c.Name
		}
		if data, _ := json.Marshal(components); data != nil {
			result.Fields["components"] = string(data)
		}
	}
	if len(iss.Fields.FixVersions) > 0 {
		versions := make([]string, len(iss.Fields.FixVersions))
		for i, v := range iss.Fields.FixVersions {
			versions[i] = v.Name
		}
		if data, _ := json.Marshal(versions); data != nil {
			result.Fields["fix_versions"] = string(data)
		}
	}
	if iss.Fields.Resolution != nil {
		result.Fields["resolution"] = iss.Fields.Resolution.Name
	}

	// Build URL from self link
	if iss.Self != "" {
		result.URL = iss.Self
		result.URL = strings.Replace(result.URL, "/rest/api/2/issue/", "/browse/", 1)
		result.URL = strings.Split(result.URL, "/rest/")[0]
	}

	return result
}

func (p *JiraPlugin) listProjects(ctx context.Context) ([]byte, error) {
	projects, _, err := p.client.Project.GetList()
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	result := make([]provider.Project, 0, len(*projects))
	for _, proj := range *projects {
		result = append(result, provider.Project{
			ID:   proj.ID,
			Key:  proj.Key,
			Name: proj.Name,
		})
	}
	return sdk.ActionOutput(result)
}

func (p *JiraPlugin) listIssues(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		ProjectKey string `json:"project_key"`
		JQL        string `json:"jql"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	jql := input.JQL
	if jql == "" && input.ProjectKey != "" {
		jql = fmt.Sprintf("project = %s ORDER BY updated DESC", input.ProjectKey)
	}
	if jql == "" {
		jql = "ORDER BY updated DESC"
	}
	maxResults := input.MaxResults
	if maxResults == 0 {
		maxResults = 50
	}

	issues, _, err := p.client.Issue.Search(jql, &jira.SearchOptions{
		MaxResults: maxResults,
		Fields:     []string{"summary", "status", "priority", "issuetype", "assignee", "reporter", "labels", "created", "updated", "components", "fixVersions", "resolution", "resolutiondate", "description"},
	})
	if err != nil {
		return nil, fmt.Errorf("search issues: %w", err)
	}

	result := make([]provider.Issue, 0, len(issues))
	for i := range issues {
		result = append(result, p.enrichIssue(&issues[i]))
	}
	return sdk.ActionOutput(result)
}

func (p *JiraPlugin) searchIssues(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		JQL        string   `json:"jql"`
		MaxResults int      `json:"max_results"`
		StartAt    int      `json:"start_at"`
		Fields     []string `json:"fields"`
		Expand     string   `json:"expand"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	if input.JQL == "" {
		input.JQL = "ORDER BY updated DESC"
	}
	if input.MaxResults == 0 {
		input.MaxResults = 50
	}

	fields := input.Fields
	if len(fields) == 0 {
		fields = []string{"summary", "status", "priority", "issuetype", "assignee", "reporter",
			"labels", "created", "updated", "components", "fixVersions", "resolution",
			"resolutiondate", "description", "parent", "subtasks", "comment"}
	}

	opts := &jira.SearchOptions{
		MaxResults: input.MaxResults,
		StartAt:    input.StartAt,
		Fields:     fields,
	}
	if input.Expand != "" {
		opts.Expand = input.Expand
	}

	issues, resp, err := p.client.Issue.Search(input.JQL, opts)
	if err != nil {
		return nil, fmt.Errorf("search issues: %w", err)
	}

	result := make([]provider.Issue, 0, len(issues))
	for i := range issues {
		result = append(result, p.enrichIssue(&issues[i]))
	}

	output := map[string]interface{}{
		"issues": result,
		"total":  resp.Total,
	}
	return sdk.ActionOutput(output)
}

func (p *JiraPlugin) getIssue(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		IssueKey string `json:"issue_key"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	issue, _, err := p.client.Issue.Get(input.IssueKey, nil)
	if err != nil {
		return nil, fmt.Errorf("get issue: %w", err)
	}

	return sdk.ActionOutput(p.enrichIssue(issue))
}

func (p *JiraPlugin) createIssue(ctx context.Context, params []byte) ([]byte, error) {
	var input provider.CreateIssueRequest
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	issue := &jira.Issue{
		Fields: &jira.IssueFields{
			Project:     jira.Project{Key: input.ProjectKey},
			Summary:     input.Summary,
			Description: input.Description,
			Type:        jira.IssueType{Name: input.Type},
		},
	}
	if input.Priority != "" {
		issue.Fields.Priority = &jira.Priority{Name: input.Priority}
	}
	if input.Assignee != "" {
		issue.Fields.Assignee = &jira.User{Name: input.Assignee}
	}
	if len(input.Labels) > 0 {
		issue.Fields.Labels = input.Labels
	}

	created, _, err := p.client.Issue.Create(issue)
	if err != nil {
		return nil, fmt.Errorf("create issue: %w", err)
	}

	return sdk.ActionOutput(p.enrichIssue(created))
}

func (p *JiraPlugin) updateIssue(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		IssueKey    string            `json:"issue_key"`
		Summary     string            `json:"summary"`
		Description string            `json:"description"`
		Assignee    string            `json:"assignee"`
		Priority    string            `json:"priority"`
		Labels      []string          `json:"labels"`
		Fields      map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	issue := &jira.Issue{
		Key: input.IssueKey,
		Fields: &jira.IssueFields{},
	}

	if input.Summary != "" {
		issue.Fields.Summary = input.Summary
	}
	if input.Description != "" {
		issue.Fields.Description = input.Description
	}
	if input.Assignee != "" {
		issue.Fields.Assignee = &jira.User{Name: input.Assignee}
	}
	if input.Priority != "" {
		issue.Fields.Priority = &jira.Priority{Name: input.Priority}
	}
	if len(input.Labels) > 0 {
		issue.Fields.Labels = input.Labels
	}

	_, _, err := p.client.Issue.Update(issue)
	if err != nil {
		return nil, fmt.Errorf("update issue: %w", err)
	}

	return sdk.ActionOutput(map[string]string{"status": "updated", "issue": input.IssueKey})
}

func (p *JiraPlugin) deleteIssue(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		IssueKey         string `json:"issue_key"`
		DeleteSubtasks   bool   `json:"delete_subtasks"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	resp, err := p.client.Issue.Delete(input.IssueKey)
	if err != nil {
		return nil, fmt.Errorf("delete issue: %w", err)
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("delete issue returned status %d", resp.StatusCode)
	}

	return sdk.ActionOutput(map[string]string{"status": "deleted", "issue": input.IssueKey})
}

func (p *JiraPlugin) transitionIssue(ctx context.Context, params []byte) ([]byte, error) {
	var input provider.TransitionRequest
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	transitionID := input.TransitionID
	if transitionID == "" && input.TransitionName != "" {
		transitions, _, err := p.client.Issue.GetTransitions(input.IssueKey)
		if err != nil {
			return nil, fmt.Errorf("get transitions: %w", err)
		}
		for _, t := range transitions {
			if strings.EqualFold(t.Name, input.TransitionName) {
				transitionID = t.ID
				break
			}
		}
		if transitionID == "" {
			return nil, fmt.Errorf("transition %q not found", input.TransitionName)
		}
	}

	_, err := p.client.Issue.DoTransition(input.IssueKey, transitionID)
	if err != nil {
		return nil, fmt.Errorf("transition issue: %w", err)
	}

	// Add comment if provided
	if input.Comment != "" {
		comment := &jira.Comment{Body: input.Comment}
		_, _, _ = p.client.Issue.AddComment(input.IssueKey, comment)
	}

	return sdk.ActionOutput(map[string]string{"status": "transitioned", "issue": input.IssueKey})
}

func (p *JiraPlugin) listTransitions(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		IssueKey string `json:"issue_key"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	transitions, _, err := p.client.Issue.GetTransitions(input.IssueKey)
	if err != nil {
		return nil, fmt.Errorf("get transitions: %w", err)
	}

	result := make([]provider.Transition, 0, len(transitions))
	for _, t := range transitions {
		result = append(result, provider.Transition{
			ID:   t.ID,
			Name: t.Name,
			To:   t.To.Name,
		})
	}
	return sdk.ActionOutput(result)
}

func (p *JiraPlugin) addComment(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		IssueKey string `json:"issue_key"`
		Body     string `json:"body"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	comment := &jira.Comment{Body: input.Body}
	created, _, err := p.client.Issue.AddComment(input.IssueKey, comment)
	if err != nil {
		return nil, fmt.Errorf("add comment: %w", err)
	}

	return sdk.ActionOutput(map[string]interface{}{
		"status":     "comment_added",
		"comment_id": created.ID,
	})
}

func (p *JiraPlugin) listComments(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		IssueKey string `json:"issue_key"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	// Fetch issue with comments expanded
	issue, _, err := p.client.Issue.Get(input.IssueKey, &jira.GetQueryOptions{
		Expand: "renderedFields",
	})
	if err != nil {
		return nil, fmt.Errorf("get issue for comments: %w", err)
	}

	// Extract comments from issue fields
	result := make([]map[string]interface{}, 0)
	if issue.Fields.Comments != nil {
		for _, c := range issue.Fields.Comments.Comments {
			result = append(result, map[string]interface{}{
				"id":      c.ID,
				"body":    c.Body,
				"author":  c.Author.DisplayName,
				"created": c.Created,
				"updated": c.Updated,
			})
		}
	}
	return sdk.ActionOutput(result)
}

func (p *JiraPlugin) listLabels(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		ProjectKey string `json:"project_key"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	// Use JQL to find all unique labels
	jql := "project IS NOT EMPTY"
	if input.ProjectKey != "" {
		jql = fmt.Sprintf("project = %s", input.ProjectKey)
	}

	// Jira doesn't have a direct labels endpoint, so we search and aggregate
	issues, _, err := p.client.Issue.Search(jql, &jira.SearchOptions{
		MaxResults: 1000,
		Fields:     []string{"labels"},
	})
	if err != nil {
		return nil, fmt.Errorf("search for labels: %w", err)
	}

	labelSet := make(map[string]bool)
	for _, iss := range issues {
		for _, l := range iss.Fields.Labels {
			labelSet[l] = true
		}
	}

	labels := make([]string, 0, len(labelSet))
	for l := range labelSet {
		labels = append(labels, l)
	}
	return sdk.ActionOutput(labels)
}

func (p *JiraPlugin) getProjectMeta(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		ProjectKey string `json:"project_key"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	if input.ProjectKey == "" {
		return nil, fmt.Errorf("project_key is required")
	}

	// Get project info
	project, _, err := p.client.Project.Get(input.ProjectKey)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	// Extract issue types
	issueTypes := make([]map[string]interface{}, 0, len(project.IssueTypes))
	for _, it := range project.IssueTypes {
		issueTypes = append(issueTypes, map[string]interface{}{
			"id":          it.ID,
			"name":        it.Name,
			"description": it.Description,
			"subtask":     it.Subtask,
		})
	}

	result := map[string]interface{}{
		"key":         project.Key,
		"name":        project.Name,
		"issue_types": issueTypes,
	}
	return sdk.ActionOutput(result)
}

func (p *JiraPlugin) linkMR(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		IssueKey string `json:"issue_key"`
		MRTitle  string `json:"mr_title"`
		MRURL    string `json:"mr_url"`
		MRSource string `json:"mr_source"` // e.g., "GitLab", "GitHub"
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	body := fmt.Sprintf("[%s] Merge Request: [%s](%s)", input.MRSource, input.MRTitle, input.MRURL)
	comment := &jira.Comment{Body: body}
	_, _, err := p.client.Issue.AddComment(input.IssueKey, comment)
	if err != nil {
		return nil, fmt.Errorf("link MR comment: %w", err)
	}

	return sdk.ActionOutput(map[string]string{"status": "mr_linked", "issue": input.IssueKey})
}

func (p *JiraPlugin) notifyDeployment(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		IssueKey    string `json:"issue_key"`
		Event       string `json:"event"` // deployment_started, deployment_succeeded, deployment_failed
		ServiceName string `json:"service_name"`
		Environment string `json:"environment"`
		Cluster     string `json:"cluster"`
		ImageTag    string `json:"image_tag"`
		User        string `json:"user"`
		Duration    string `json:"duration"`
		Error       string `json:"error"`
		LogURL      string `json:"log_url"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	var body string
	switch input.Event {
	case "deployment_started":
		body = fmt.Sprintf(`**[PEPA] Deployment Started**
- Service: %s
- Environment: %s
- Image: %s
- Cluster: %s
- Triggered by: %s`, input.ServiceName, input.Environment, input.ImageTag, input.Cluster, input.User)

	case "deployment_succeeded":
		body = fmt.Sprintf(`**[PEPA] Deployment Succeeded**
- Service: %s is now live
- Environment: %s
- Cluster: %s
- Duration: %s`, input.ServiceName, input.Environment, input.Cluster, input.Duration)

	case "deployment_failed":
		body = fmt.Sprintf(`**[PEPA] Deployment FAILED**
- Service: %s
- Environment: %s
- Error: %s
- Logs: %s`, input.ServiceName, input.Environment, input.Error, input.LogURL)

	default:
		body = fmt.Sprintf("[PEPA] Deployment event: %s for %s", input.Event, input.ServiceName)
	}

	comment := &jira.Comment{Body: body}
	_, _, err := p.client.Issue.AddComment(input.IssueKey, comment)
	if err != nil {
		return nil, fmt.Errorf("notify deployment: %w", err)
	}

	return sdk.ActionOutput(map[string]string{"status": "notified", "issue": input.IssueKey, "event": input.Event})
}

func (p *JiraPlugin) listAssignees(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		ProjectKey string `json:"project_key"`
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	if input.MaxResults == 0 {
		input.MaxResults = 50
	}

	query := input.Query
	if query == "" {
		query = "."
	}

	// Use Jira REST API to search users
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/rest/api/2/user/search?username=%s&maxResults=%d", p.client.GetBaseURL(), query, input.MaxResults), nil)
	if err != nil {
		return nil, err
	}

	var users []struct {
		Name         string            `json:"name"`
		DisplayName  string            `json:"displayName"`
		EmailAddress string            `json:"emailAddress"`
		Active       bool              `json:"active"`
		AvatarUrls   map[string]string `json:"avatarUrls"`
	}
	_, err = p.client.Do(req, &users)
	if err != nil {
		return nil, fmt.Errorf("list assignees: %w", err)
	}

	result := make([]map[string]string, 0, len(users))
	for _, u := range users {
		avatarURL := ""
		if u.AvatarUrls != nil {
			avatarURL = u.AvatarUrls["48x48"]
		}
		result = append(result, map[string]string{
			"name":       u.Name,
			"display":    u.DisplayName,
			"email":      u.EmailAddress,
			"avatar_url": avatarURL,
			"active":     fmt.Sprintf("%v", u.Active),
		})
	}
	return sdk.ActionOutput(result)
}

func (p *JiraPlugin) listSprints(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		ProjectKey string `json:"project_key"`
		BoardID    int    `json:"board_id"`
		State      string `json:"state"` // active, future, closed
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	// Use Jira Agile API to get sprints
	state := input.State
	if state == "" {
		state = "active,future"
	}

	// If board_id is provided, use it directly
	if input.BoardID > 0 {
		sprints, err := p.listBoardSprints(ctx, input.BoardID, state)
		if err != nil {
			return nil, err
		}
		return sdk.ActionOutput(sprints)
	}

	// Otherwise, try to find boards for the project
	boards, err := p.findProjectBoards(ctx, input.ProjectKey)
	if err != nil {
		return nil, err
	}

	allSprints := make([]map[string]interface{}, 0)
	for _, board := range boards {
		sprints, err := p.listBoardSprints(ctx, board["id"].(int), state)
		if err != nil {
			continue
		}
		allSprints = append(allSprints, sprints...)
	}
	return sdk.ActionOutput(allSprints)
}

func (p *JiraPlugin) findProjectBoards(ctx context.Context, projectKey string) ([]map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/rest/agile/1.0/board?projectKeyOrId=%s", p.client.GetBaseURL(), projectKey), nil)
	if err != nil {
		return nil, err
	}
	var data struct {
		Values []map[string]interface{} `json:"values"`
	}
	_, err = p.client.Do(req, &data)
	if err != nil {
		return nil, fmt.Errorf("find boards: %w", err)
	}
	result := make([]map[string]interface{}, 0, len(data.Values))
	for _, b := range data.Values {
		id := 0
		if v, ok := b["id"].(float64); ok {
			id = int(v)
		}
		result = append(result, map[string]interface{}{
			"id":   id,
			"name": b["name"],
			"type": b["type"],
		})
	}
	return result, nil
}

func (p *JiraPlugin) listBoardSprints(ctx context.Context, boardID int, state string) ([]map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/rest/agile/1.0/board/%d/sprint?state=%s&maxResults=50", p.client.GetBaseURL(), boardID, state), nil)
	if err != nil {
		return nil, err
	}
	var data struct {
		Values []map[string]interface{} `json:"values"`
	}
	_, err = p.client.Do(req, &data)
	if err != nil {
		return nil, fmt.Errorf("list sprints: %w", err)
	}
	result := make([]map[string]interface{}, 0, len(data.Values))
	for _, s := range data.Values {
		sprint := map[string]interface{}{
			"id":         s["id"],
			"name":       s["name"],
			"state":      s["state"],
			"start_date": s["startDate"],
			"end_date":   s["endDate"],
			"board_id":   boardID,
		}
		result = append(result, sprint)
	}
	return result, nil
}

func (p *JiraPlugin) addWorklog(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		IssueKey       string `json:"issue_key"`
		TimeSpent      string `json:"time_spent"`       // e.g. "2h 30m"
		TimeSpentSecs  int    `json:"time_spent_secs"`  // alternative: seconds
		Comment        string `json:"comment"`
		Started        string `json:"started"` // ISO date
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	if input.IssueKey == "" {
		return nil, fmt.Errorf("issue_key is required")
	}
	if input.TimeSpent == "" && input.TimeSpentSecs <= 0 {
		return nil, fmt.Errorf("either time_spent or time_spent_secs must be provided")
	}

	worklog := &jira.WorklogRecord{
		Comment: input.Comment,
	}
	if input.TimeSpent != "" {
		worklog.TimeSpent = input.TimeSpent
	} else if input.TimeSpentSecs > 0 {
		worklog.TimeSpentSeconds = input.TimeSpentSecs
	}

	record, _, err := p.client.Issue.AddWorklogRecord(input.IssueKey, worklog)
	if err != nil {
		return nil, fmt.Errorf("add worklog: %w", err)
	}

	return sdk.ActionOutput(map[string]interface{}{
		"status":   "worklog_added",
		"id":       record.ID,
		"time":     record.TimeSpent,
	})
}

func (p *JiraPlugin) listWorklogs(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		IssueKey string `json:"issue_key"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	if input.IssueKey == "" {
		return nil, fmt.Errorf("issue_key is required")
	}

	// Use REST API directly for worklogs
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/rest/api/2/issue/%s/worklog", p.client.GetBaseURL(), input.IssueKey), nil)
	if err != nil {
		return nil, err
	}
	var data struct {
		Worklogs []struct {
			ID               string          `json:"id"`
			TimeSpent        string          `json:"timeSpent"`
			TimeSpentSeconds int             `json:"timeSpentSeconds"`
			Comment          string          `json:"comment"`
			Author           *jira.User      `json:"author"`
			Started          string          `json:"started"`
		} `json:"worklogs"`
	}
	_, err = p.client.Do(req, &data)
	if err != nil {
		return nil, fmt.Errorf("list worklogs: %w", err)
	}

	result := make([]map[string]interface{}, 0, len(data.Worklogs))
	for _, w := range data.Worklogs {
		entry := map[string]interface{}{
			"id":               w.ID,
			"time_spent":       w.TimeSpent,
			"time_spent_secs":  w.TimeSpentSeconds,
			"comment":          w.Comment,
			"started":          w.Started,
		}
		if w.Author != nil {
			entry["author"] = w.Author.DisplayName
		}
		result = append(result, entry)
	}
	return sdk.ActionOutput(result)
}

func (p *JiraPlugin) linkIssues(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		InwardKey  string `json:"inward_key"`   // the issue that has the link
		OutwardKey string `json:"outward_key"`  // the linked issue
		LinkType   string `json:"link_type"`    // "Blocks", "Clones", "Duplicate", "Relates"
		Comment    string `json:"comment"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	if input.InwardKey == "" || input.OutwardKey == "" {
		return nil, fmt.Errorf("inward_key and outward_key are required")
	}
	if input.LinkType == "" {
		input.LinkType = "Relates"
	}

	// Use REST API to create issue link
	body := map[string]interface{}{
		"inwardIssue":  map[string]string{"key": input.InwardKey},
		"outwardIssue": map[string]string{"key": input.OutwardKey},
		"type":         map[string]string{"name": input.LinkType},
	}
	if input.Comment != "" {
		body["comment"] = map[string]string{"body": input.Comment}
	}

	reqBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/rest/api/2/issueLink", p.client.GetBaseURL()), strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	_, err = p.client.Do(req, nil)
	if err != nil {
		return nil, fmt.Errorf("link issues: %w", err)
	}

	return sdk.ActionOutput(map[string]string{
		"status":      "linked",
		"inward_key":  input.InwardKey,
		"outward_key": input.OutwardKey,
		"link_type":   input.LinkType,
	})
}

func (p *JiraPlugin) createSubtask(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		ParentKey   string `json:"parent_key"`
		Summary     string `json:"summary"`
		Description string `json:"description"`
		Assignee    string `json:"assignee"`
		Priority    string `json:"priority"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}

	// Get parent issue to determine project
	parent, _, err := p.client.Issue.Get(input.ParentKey, nil)
	if err != nil {
		return nil, fmt.Errorf("get parent issue: %w", err)
	}

	issue := &jira.Issue{
		Fields: &jira.IssueFields{
			Project:     jira.Project{Key: parent.Fields.Project.Key},
			Summary:     input.Summary,
			Description: input.Description,
			Type:        jira.IssueType{Name: "Sub-task"},
		},
	}
	// Set parent via raw JSON since go-jira uses a different Parent type
	issue.Fields.Unknowns = map[string]interface{}{
		"parent": map[string]string{"key": input.ParentKey},
	}
	if input.Priority != "" {
		issue.Fields.Priority = &jira.Priority{Name: input.Priority}
	}
	if input.Assignee != "" {
		issue.Fields.Assignee = &jira.User{Name: input.Assignee}
	}

	created, _, err := p.client.Issue.Create(issue)
	if err != nil {
		return nil, fmt.Errorf("create subtask: %w", err)
	}

	return sdk.ActionOutput(p.enrichIssue(created))
}

func (p *JiraPlugin) syncIssues(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		ProjectKey string `json:"project_key"`
		JQL        string `json:"jql"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	if input.MaxResults == 0 {
		input.MaxResults = 200
	}

	jql := input.JQL
	if jql == "" && input.ProjectKey != "" {
		jql = fmt.Sprintf("project = %s ORDER BY updated DESC", input.ProjectKey)
	}
	if jql == "" {
		jql = "ORDER BY updated DESC"
	}

	issues, _, err := p.client.Issue.Search(jql, &jira.SearchOptions{
		MaxResults: input.MaxResults,
		Fields:     []string{"summary", "status", "priority", "issuetype", "assignee", "reporter", "labels", "created", "updated", "components", "fixVersions", "resolution", "description", "parent", "subtasks", "comment"},
	})
	if err != nil {
		return nil, fmt.Errorf("sync search: %w", err)
	}

	result := make([]provider.Issue, 0, len(issues))
	for i := range issues {
		result = append(result, p.enrichIssue(&issues[i]))
	}

	return sdk.ActionOutput(map[string]interface{}{
		"issues": result,
		"total":  len(result),
		"synced": len(result),
	})
}

func (p *JiraPlugin) listComponents(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		ProjectKey string `json:"project_key"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	if input.ProjectKey == "" {
		return nil, fmt.Errorf("project_key is required")
	}

	project, _, err := p.client.Project.Get(input.ProjectKey)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	result := make([]map[string]interface{}, 0, len(project.Components))
	for _, c := range project.Components {
		result = append(result, map[string]interface{}{
			"id":          c.ID,
			"name":        c.Name,
			"description": c.Description,
			"lead":        c.Lead.DisplayName,
		})
	}
	return sdk.ActionOutput(result)
}

func (p *JiraPlugin) listVersions(ctx context.Context, params []byte) ([]byte, error) {
	var input struct {
		ProjectKey string `json:"project_key"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	if input.ProjectKey == "" {
		return nil, fmt.Errorf("project_key is required")
	}

	// Use REST API directly for versions
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/rest/api/2/project/%s/versions", p.client.GetBaseURL(), input.ProjectKey), nil)
	if err != nil {
		return nil, err
	}
	var versions []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Released    bool   `json:"released"`
		Archived    bool   `json:"archived"`
		ReleaseDate string `json:"releaseDate"`
	}
	_, err = p.client.Do(req, &versions)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}

	result := make([]map[string]interface{}, 0, len(versions))
	for _, v := range versions {
		result = append(result, map[string]interface{}{
			"id":           v.ID,
			"name":         v.Name,
			"description":  v.Description,
			"released":     v.Released,
			"archived":     v.Archived,
			"release_date": v.ReleaseDate,
		})
	}
	return sdk.ActionOutput(result)
}

func main() {
	log.Println("[jira-plugin] starting Jira plugin v2.0.0")
	plugin := &JiraPlugin{}
	sdk.Serve(plugin)
}
