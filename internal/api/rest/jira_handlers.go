package rest

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
)

func registerJiraRoutes(r *gin.RouterGroup, deps Dependencies) {
	jira := r.Group("/jira")
	{
		// Issues
		jira.GET("/issues", listJiraIssues(deps))
		jira.POST("/issues/search", searchJiraIssues(deps))
		jira.GET("/issues/:id", getJiraIssue(deps))
		jira.POST("/issues", createJiraIssue(deps))
		jira.PUT("/issues/:id", updateJiraIssue(deps))
		jira.DELETE("/issues/:id", deleteJiraIssue(deps))
		jira.POST("/issues/:id/link", linkJiraDeployment(deps))

		// My issues
		jira.GET("/my-issues", listMyJiraIssues(deps))

		// Comments
		jira.GET("/issues/:id/comments", listJiraComments(deps))
		jira.POST("/issues/:id/comments", addJiraComment(deps))

		// Transitions
		jira.GET("/issues/:id/transitions", listJiraTransitions(deps))
		jira.POST("/issues/:id/transition", transitionJiraIssue(deps))

		// Worklogs
		jira.GET("/issues/:id/worklogs", listJiraWorklogs(deps))
		jira.POST("/issues/:id/worklogs", addJiraWorklog(deps))

		// Issue Links
		jira.GET("/issues/:id/links", listJiraIssueLinks(deps))

		// Metadata
		jira.GET("/labels", listJiraLabels(deps))
		jira.GET("/stats", getJiraStats(deps))
		jira.GET("/projects", listJiraProjects(deps))
		jira.GET("/assignees", listJiraAssignees(deps))
		jira.GET("/sprints", listJiraSprints(deps))
		jira.GET("/components", listJiraComponents(deps))

		// Sync
		jira.POST("/sync", syncJiraIssues(deps))

		// Automation rules
		jira.GET("/automation/rules", listAutomationRules(deps))
		jira.POST("/automation/rules", createAutomationRule(deps))
		jira.PUT("/automation/rules/:id", updateAutomationRule(deps))
		jira.DELETE("/automation/rules/:id", deleteAutomationRule(deps))

		// Webhook receiver
		jira.POST("/webhook", jiraWebhook(deps))
	}
}

// ── Issue Handlers ────────────────────────────────────────────

func listJiraIssues(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusOK, gin.H{"issues": []interface{}{}, "total": 0})
			return
		}
		tenantID := auth.GetTenantID(c)

		// Parse query params as filters
		filters := repository.JiraFilters{
			ProjectKey: c.Query("project_key"),
			Assignee:   c.Query("assignee"),
			Search:     c.Query("search"),
		}
		if v := c.QueryArray("issue_type"); len(v) > 0 {
			filters.IssueTypes = v
		}
		if v := c.QueryArray("status"); len(v) > 0 {
			filters.Statuses = v
		}
		if v := c.QueryArray("label"); len(v) > 0 {
			filters.Labels = v
		}
		if v := c.QueryArray("priority"); len(v) > 0 {
			filters.Priorities = v
		}

		items, total, err := deps.Repos.Jira.ListWithFilters(c.Request.Context(), tenantID, filters)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if items == nil {
			items = []repository.JiraIssue{}
		}
		c.JSON(http.StatusOK, gin.H{"issues": items, "total": total})
	}
}

func searchJiraIssues(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusOK, gin.H{"issues": []interface{}{}, "total": 0})
			return
		}
		tenantID := auth.GetTenantID(c)

		var filters repository.JiraFilters
		if err := c.ShouldBindJSON(&filters); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		items, total, err := deps.Repos.Jira.ListWithFilters(c.Request.Context(), tenantID, filters)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if items == nil {
			items = []repository.JiraIssue{}
		}
		c.JSON(http.StatusOK, gin.H{"issues": items, "total": total})
	}
}

func getJiraIssue(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jira repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issue ID"})
			return
		}
		issue, err := deps.Repos.Jira.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, issue)
	}
}

func createJiraIssue(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jira repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)

		var req struct {
			IssueKey    string   `json:"issue_key" binding:"required"`
			ProjectKey  string   `json:"project_key" binding:"required"`
			Summary     string   `json:"summary" binding:"required"`
			Description string   `json:"description"`
			IssueType   string   `json:"issue_type" binding:"required"`
			Priority    string   `json:"priority"`
			Assignee    string   `json:"assignee"`
			Reporter    string   `json:"reporter"`
			Labels      []string `json:"labels"`
			JiraURL     string   `json:"jira_url"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		issue := &repository.JiraIssue{
			TenantID:    tenantID,
			IssueKey:    req.IssueKey,
			ProjectKey:  req.ProjectKey,
			Summary:     req.Summary,
			Description: req.Description,
			IssueType:   req.IssueType,
			Priority:    req.Priority,
			Assignee:    req.Assignee,
			Reporter:    req.Reporter,
			Labels:      req.Labels,
			JiraURL:     req.JiraURL,
			Status:      "Open",
		}
		if issue.Labels == nil {
			issue.Labels = []string{}
		}

		if err := deps.Repos.Jira.Upsert(c.Request.Context(), issue); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "create", "jira_issue", issue.ID.String(), nil, gin.H{"issue_key": issue.IssueKey, "project": issue.ProjectKey})
		c.JSON(http.StatusCreated, issue)
	}
}

func updateJiraIssue(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jira repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issue ID"})
			return
		}

		var req struct {
			Summary     string   `json:"summary"`
			Description string   `json:"description"`
			Assignee    string   `json:"assignee"`
			Priority    string   `json:"priority"`
			Status      string   `json:"status"`
			Labels      []string `json:"labels"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := deps.Repos.Jira.Update(c.Request.Context(), id, req.Summary, req.Description, req.Assignee, req.Priority, req.Status, req.Labels); err != nil {
			respondInternalError(c, err)
			return
		}

		issue, _ := deps.Repos.Jira.Get(c.Request.Context(), id)
		logAudit(deps, c, "update", "jira_issue", id.String(), nil, nil)
		c.JSON(http.StatusOK, issue)
	}
}

func deleteJiraIssue(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jira repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issue ID"})
			return
		}
		if err := deps.Repos.Jira.Delete(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "delete", "jira_issue", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "issue deleted"})
	}
}

func linkJiraDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jira repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issue ID"})
			return
		}
		var req struct {
			DeploymentID uuid.UUID `json:"deployment_id" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := deps.Repos.Jira.LinkDeployment(c.Request.Context(), id, req.DeploymentID); err != nil {
			respondInternalError(c, err)
			return
		}
		issue, _ := deps.Repos.Jira.Get(c.Request.Context(), id)
		c.JSON(http.StatusOK, issue)
	}
}

// ── Comment Handlers ──────────────────────────────────────────

func listJiraComments(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusOK, gin.H{"comments": []interface{}{}})
			return
		}
		tenantID := auth.GetTenantID(c)
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issue ID"})
			return
		}
		issue, err := deps.Repos.Jira.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "issue not found"})
			return
		}
		comments, err := deps.Repos.Jira.ListComments(c.Request.Context(), tenantID, issue.IssueKey)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if comments == nil {
			comments = []repository.JiraComment{}
		}
		c.JSON(http.StatusOK, gin.H{"comments": comments})
	}
}

func addJiraComment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jira repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issue ID"})
			return
		}
		issue, err := deps.Repos.Jira.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "issue not found"})
			return
		}

		var req struct {
			Body string `json:"body" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		comment := &repository.JiraComment{
			TenantID:  tenantID,
			IssueKey:  issue.IssueKey,
			CommentID: uuid.New().String()[:8],
			Author:    "PEPA",
			Body:      req.Body,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := deps.Repos.Jira.UpsertComment(c.Request.Context(), comment); err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusCreated, comment)
	}
}

// ── Transition Handlers ───────────────────────────────────────

func listJiraTransitions(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Return common Jira transitions based on current status
		// In a full implementation, this would call the Jira plugin
		c.JSON(http.StatusOK, gin.H{
			"transitions": []map[string]string{
				{"id": "11", "name": "To Do", "to": "To Do"},
				{"id": "21", "name": "In Progress", "to": "In Progress"},
				{"id": "31", "name": "In Review", "to": "In Review"},
				{"id": "41", "name": "Done", "to": "Done"},
			},
		})
	}
}

func transitionJiraIssue(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jira repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issue ID"})
			return
		}

		var req struct {
			Status string `json:"status" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Update status locally
		if err := deps.Repos.Jira.Update(c.Request.Context(), id, "", "", "", "", req.Status, nil); err != nil {
			respondInternalError(c, err)
			return
		}
		issue, _ := deps.Repos.Jira.Get(c.Request.Context(), id)
		c.JSON(http.StatusOK, gin.H{"issue": issue, "message": "transitioned to " + req.Status})
	}
}

// ── Metadata Handlers ─────────────────────────────────────────

func listJiraLabels(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusOK, gin.H{"labels": []string{}})
			return
		}
		tenantID := auth.GetTenantID(c)
		labels, err := deps.Repos.Jira.GetLabels(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if labels == nil {
			labels = []string{}
		}
		c.JSON(http.StatusOK, gin.H{"labels": labels})
	}
}

func getJiraStats(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusOK, &repository.JiraStats{
				ByStatus:   map[string]int{},
				ByType:     map[string]int{},
				ByPriority: map[string]int{},
			})
			return
		}
		tenantID := auth.GetTenantID(c)
		stats, err := deps.Repos.Jira.GetStats(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, stats)
	}
}

func listJiraProjects(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusOK, gin.H{"projects": []interface{}{}})
			return
		}
		tenantID := auth.GetTenantID(c)
		// Get unique project keys from synced issues
		items, err := deps.Repos.Jira.List(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		projectMap := make(map[string]bool)
		var projects []map[string]string
		for _, item := range items {
			if !projectMap[item.ProjectKey] {
				projectMap[item.ProjectKey] = true
				projects = append(projects, map[string]string{
					"key": item.ProjectKey,
				})
			}
		}
		if projects == nil {
			projects = []map[string]string{}
		}
		c.JSON(http.StatusOK, gin.H{"projects": projects})
	}
}

func syncJiraIssues(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Placeholder — actual sync would call the Jira plugin
		c.JSON(http.StatusOK, gin.H{"message": "sync initiated", "synced": 0})
	}
}

// ── Automation Rule Handlers ──────────────────────────────────

func listAutomationRules(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusOK, gin.H{"rules": []interface{}{}})
			return
		}
		tenantID := auth.GetTenantID(c)
		rules, err := deps.Repos.Jira.ListAutomationRules(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if rules == nil {
			rules = []repository.JiraAutomationRule{}
		}
		c.JSON(http.StatusOK, gin.H{"rules": rules})
	}
}

func createAutomationRule(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jira repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)

		var req struct {
			Name           string          `json:"name" binding:"required"`
			Description    string          `json:"description"`
			TriggerType    string          `json:"trigger_type" binding:"required"`
			JiraProjectKey string          `json:"jira_project_key"`
			JQLFilter      string          `json:"jql_filter"`
			ActionType     string          `json:"action_type" binding:"required"`
			ActionConfig   json.RawMessage `json:"action_config"`
			Enabled        bool            `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.ActionConfig == nil {
			req.ActionConfig = json.RawMessage(`{}`)
		}

		rule := &repository.JiraAutomationRule{
			TenantID:       tenantID,
			Name:           req.Name,
			Description:    req.Description,
			TriggerType:    req.TriggerType,
			JiraProjectKey: req.JiraProjectKey,
			JQLFilter:      req.JQLFilter,
			ActionType:     req.ActionType,
			ActionConfig:   req.ActionConfig,
			Enabled:        req.Enabled,
		}

		if err := deps.Repos.Jira.CreateAutomationRule(c.Request.Context(), rule); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "create", "jira_automation_rule", rule.ID.String(), nil, gin.H{"name": req.Name, "trigger": req.TriggerType})
		c.JSON(http.StatusCreated, rule)
	}
}

func updateAutomationRule(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jira repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule ID"})
			return
		}

		var req struct {
			Name           string          `json:"name"`
			Description    string          `json:"description"`
			TriggerType    string          `json:"trigger_type"`
			JiraProjectKey string          `json:"jira_project_key"`
			JQLFilter      string          `json:"jql_filter"`
			ActionType     string          `json:"action_type"`
			ActionConfig   json.RawMessage `json:"action_config"`
			Enabled        bool            `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := deps.Repos.Jira.UpdateAutomationRule(c.Request.Context(), id, req.Name, req.Description, req.TriggerType, req.JiraProjectKey, req.JQLFilter, req.ActionType, req.ActionConfig, req.Enabled); err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "jira_automation_rule", id.String(), nil, gin.H{"name": req.Name})

		rule, _ := deps.Repos.Jira.GetAutomationRule(c.Request.Context(), id)
		c.JSON(http.StatusOK, rule)
	}
}

func deleteAutomationRule(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jira repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule ID"})
			return
		}
		if err := deps.Repos.Jira.DeleteAutomationRule(c.Request.Context(), id); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "delete", "jira_automation_rule", id.String(), nil, nil)
		c.JSON(http.StatusOK, gin.H{"message": "rule deleted"})
	}
}

// ── Webhook Handler ───────────────────────────────────────────

// ── My Issues Handler ─────────────────────────────────────────

func listMyJiraIssues(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusOK, gin.H{"issues": []interface{}{}, "total": 0})
			return
		}
		tenantID := auth.GetTenantID(c)
		assignee := c.Query("assignee")
		if assignee == "" {
			// Try to get current user's email as fallback
			assignee = auth.GetEmail(c)
		}
		if assignee == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "assignee is required"})
			return
		}

		statuses := c.QueryArray("status")
		if len(statuses) == 0 {
			statuses = []string{"Open", "To Do", "In Progress", "In Review"}
		}

		items, total, err := deps.Repos.Jira.GetMyIssues(c.Request.Context(), tenantID, assignee, statuses)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if items == nil {
			items = []repository.JiraIssue{}
		}
		c.JSON(http.StatusOK, gin.H{"issues": items, "total": total})
	}
}

// ── Worklog Handlers ──────────────────────────────────────────

func listJiraWorklogs(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusOK, gin.H{"worklogs": []interface{}{}, "total_seconds": 0})
			return
		}
		tenantID := auth.GetTenantID(c)
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issue ID"})
			return
		}
		issue, err := deps.Repos.Jira.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "issue not found"})
			return
		}
		worklogs, err := deps.Repos.Jira.ListWorklogs(c.Request.Context(), tenantID, issue.IssueKey)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		totalSecs, _ := deps.Repos.Jira.GetTotalTimeSpent(c.Request.Context(), tenantID, issue.IssueKey)
		if worklogs == nil {
			worklogs = []repository.JiraWorklog{}
		}
		c.JSON(http.StatusOK, gin.H{"worklogs": worklogs, "total_seconds": totalSecs})
	}
}

func addJiraWorklog(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jira repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issue ID"})
			return
		}
		issue, err := deps.Repos.Jira.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "issue not found"})
			return
		}

		var req struct {
			TimeSpent     string `json:"time_spent"`
			TimeSpentSecs int    `json:"time_spent_secs"`
			Comment       string `json:"comment"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		author := auth.GetEmail(c)
		if author == "" {
			author = "PEPA"
		}

		worklog := &repository.JiraWorklog{
			TenantID:      tenantID,
			IssueKey:      issue.IssueKey,
			JiraWorklogID: uuid.New().String()[:8],
			Author:        author,
			TimeSpent:     req.TimeSpent,
			TimeSpentSecs: req.TimeSpentSecs,
			Comment:       req.Comment,
			StartedAt:     time.Now().UTC(),
		}
		if err := deps.Repos.Jira.UpsertWorklog(c.Request.Context(), worklog); err != nil {
			respondInternalError(c, err)
			return
		}
		logAudit(deps, c, "create", "jira_worklog", worklog.ID.String(), nil, gin.H{"issue_key": issue.IssueKey})
		c.JSON(http.StatusCreated, worklog)
	}
}

// ── Issue Links Handler ───────────────────────────────────────

func listJiraIssueLinks(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusOK, gin.H{"links": []interface{}{}})
			return
		}
		tenantID := auth.GetTenantID(c)
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid issue ID"})
			return
		}
		issue, err := deps.Repos.Jira.Get(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "issue not found"})
			return
		}
		links, err := deps.Repos.Jira.ListIssueLinks(c.Request.Context(), tenantID, issue.IssueKey)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if links == nil {
			links = []repository.JiraIssueLink{}
		}
		c.JSON(http.StatusOK, gin.H{"links": links})
	}
}

// ── Assignees Handler ─────────────────────────────────────────

func listJiraAssignees(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusOK, gin.H{"assignees": []interface{}{}})
			return
		}
		tenantID := auth.GetTenantID(c)

		// First try cached assignees
		assignees, err := deps.Repos.Jira.ListAssignees(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		// If no cached assignees, fall back to distinct from issues
		if len(assignees) == 0 {
			names, err := deps.Repos.Jira.GetDistinctAssignees(c.Request.Context(), tenantID)
			if err != nil {
				respondInternalError(c, err)
				return
			}
			result := make([]map[string]string, 0, len(names))
			for _, n := range names {
				result = append(result, map[string]string{"display_name": n, "jira_account": n})
			}
			c.JSON(http.StatusOK, gin.H{"assignees": result})
			return
		}

		c.JSON(http.StatusOK, gin.H{"assignees": assignees})
	}
}

// ── Sprints Handler ───────────────────────────────────────────

func listJiraSprints(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusOK, gin.H{"sprints": []interface{}{}})
			return
		}
		tenantID := auth.GetTenantID(c)
		state := c.Query("state")
		sprints, err := deps.Repos.Jira.ListSprints(c.Request.Context(), tenantID, state)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if sprints == nil {
			sprints = []repository.JiraSprint{}
		}
		c.JSON(http.StatusOK, gin.H{"sprints": sprints})
	}
}

// ── Components Handler ────────────────────────────────────────

func listJiraComponents(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.Jira == nil {
			c.JSON(http.StatusOK, gin.H{"components": []interface{}{}})
			return
		}
		tenantID := auth.GetTenantID(c)
		// Get distinct components from synced issues
		items, err := deps.Repos.Jira.List(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		compSet := make(map[string]bool)
		var components []string
		for _, item := range items {
			for _, c := range item.Components {
				if !compSet[c] {
					compSet[c] = true
					components = append(components, c)
				}
			}
		}
		if components == nil {
			components = []string{}
		}
		c.JSON(http.StatusOK, gin.H{"components": components})
	}
}

func jiraWebhook(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Jira sends webhook events for issue updates
		var payload struct {
			WebhookEvent string `json:"webhookEvent"`
			IssueEvent   string `json:"issue_event_type_name"`
			Issue        struct {
				Key  string `json:"key"`
				ID   string `json:"id"`
				Self string `json:"self"`
			} `json:"issue"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook payload"})
			return
		}

		// Log the webhook event — full processing would update local DB
		c.JSON(http.StatusOK, gin.H{
			"message": "webhook received",
			"event":   payload.WebhookEvent,
			"issue":   payload.Issue.Key,
		})
	}
}
