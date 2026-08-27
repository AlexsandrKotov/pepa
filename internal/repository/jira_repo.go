package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/database"
)

// JiraRepository handles Jira issue persistence.
type JiraRepository struct {
	pool *pgxpool.Pool
}

// NewJiraRepository creates a new Jira repository.
func NewJiraRepository(db *database.DB) *JiraRepository {
	return &JiraRepository{pool: db.Pool}
}

// JiraIssue represents a synced Jira issue.
type JiraIssue struct {
	ID           uuid.UUID  `json:"id"`
	TenantID     uuid.UUID  `json:"tenant_id"`
	IssueKey     string     `json:"issue_key"`
	IssueID      string     `json:"issue_id,omitempty"`
	ProjectKey   string     `json:"project_key"`
	Summary      string     `json:"summary"`
	Description  string     `json:"description,omitempty"`
	IssueType    string     `json:"issue_type"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"`
	Assignee     string     `json:"assignee,omitempty"`
	Reporter     string     `json:"reporter,omitempty"`
	Labels       []string   `json:"labels"`
	Components   []string   `json:"components"`
	FixVersions  []string   `json:"fix_versions"`
	StoryPoints  *int       `json:"story_points,omitempty"`
	ParentKey    string     `json:"parent_key,omitempty"`
	JiraURL      string     `json:"jira_url,omitempty"`
	LinkedMRID   *int       `json:"linked_mr_id,omitempty"`
	LinkedMRURL  string     `json:"linked_mr_url,omitempty"`
	DeploymentID *uuid.UUID `json:"deployment_id,omitempty"`
	SyncedAt     time.Time  `json:"synced_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// JiraFilters holds filter parameters for advanced issue search.
type JiraFilters struct {
	ProjectKey  string     `json:"project_key"`
	IssueTypes  []string   `json:"issue_types"`
	Statuses    []string   `json:"statuses"`
	Labels      []string   `json:"labels"`
	Priorities  []string   `json:"priorities"`
	Assignee    string     `json:"assignee"`
	Search      string     `json:"search"`
	CreatedFrom *time.Time `json:"created_from"`
	CreatedTo   *time.Time `json:"created_to"`
	Page        int        `json:"page"`
	PageSize    int        `json:"page_size"`
}

// JiraStats holds aggregated statistics for the dashboard.
type JiraStats struct {
	Total      int            `json:"total"`
	ByStatus   map[string]int `json:"by_status"`
	ByType     map[string]int `json:"by_type"`
	ByPriority map[string]int `json:"by_priority"`
	OpenBugs   int            `json:"open_bugs"`
}

// JiraComment represents a cached Jira comment.
type JiraComment struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	IssueKey  string    `json:"issue_key"`
	CommentID string    `json:"comment_id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// JiraAutomationRule represents an automation rule.
type JiraAutomationRule struct {
	ID              uuid.UUID       `json:"id"`
	TenantID        uuid.UUID       `json:"tenant_id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	TriggerType     string          `json:"trigger_type"`
	JiraProjectKey  string          `json:"jira_project_key"`
	JQLFilter       string          `json:"jql_filter"`
	ActionType      string          `json:"action_type"`
	ActionConfig    json.RawMessage `json:"action_config"`
	Enabled         bool            `json:"enabled"`
	LastTriggeredAt *time.Time      `json:"last_triggered_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// List returns all Jira issues for a tenant.
func (r *JiraRepository) List(ctx context.Context, tenantID uuid.UUID) ([]JiraIssue, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, issue_key, COALESCE(issue_id,''), project_key, summary,
		       COALESCE(description,''), COALESCE(issue_type,''), COALESCE(priority,''),
		       COALESCE(status,''), COALESCE(assignee,''), COALESCE(reporter,''),
		       COALESCE(labels, '{}'), COALESCE(components, '{}'), COALESCE(fix_versions, '{}'),
		       story_points, COALESCE(parent_key,''), COALESCE(jira_url,''),
		       linked_mr_id, COALESCE(linked_mr_url,''), deployment_id,
		       synced_at, created_at, updated_at
		FROM jira_issues WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query jira issues: %w", err)
	}
	defer rows.Close()

	items := make([]JiraIssue, 0)
	for rows.Next() {
		var j JiraIssue
		if err := rows.Scan(&j.ID, &j.TenantID, &j.IssueKey, &j.IssueID, &j.ProjectKey,
			&j.Summary, &j.Description, &j.IssueType, &j.Priority, &j.Status,
			&j.Assignee, &j.Reporter, &j.Labels, &j.Components, &j.FixVersions,
			&j.StoryPoints, &j.ParentKey, &j.JiraURL, &j.LinkedMRID, &j.LinkedMRURL,
			&j.DeploymentID, &j.SyncedAt, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan jira issue: %w", err)
		}
		items = append(items, j)
	}
	return items, nil
}

// ListWithFilters returns Jira issues matching the given filters.
func (r *JiraRepository) ListWithFilters(ctx context.Context, tenantID uuid.UUID, f JiraFilters) ([]JiraIssue, int, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argIdx))
	args = append(args, tenantID)
	argIdx++

	if f.ProjectKey != "" {
		conditions = append(conditions, fmt.Sprintf("project_key = $%d", argIdx))
		args = append(args, f.ProjectKey)
		argIdx++
	}
	if len(f.IssueTypes) > 0 {
		conditions = append(conditions, fmt.Sprintf("issue_type = ANY($%d)", argIdx))
		args = append(args, f.IssueTypes)
		argIdx++
	}
	if len(f.Statuses) > 0 {
		conditions = append(conditions, fmt.Sprintf("status = ANY($%d)", argIdx))
		args = append(args, f.Statuses)
		argIdx++
	}
	if len(f.Labels) > 0 {
		conditions = append(conditions, fmt.Sprintf("labels && $%d", argIdx))
		args = append(args, f.Labels)
		argIdx++
	}
	if len(f.Priorities) > 0 {
		conditions = append(conditions, fmt.Sprintf("priority = ANY($%d)", argIdx))
		args = append(args, f.Priorities)
		argIdx++
	}
	if f.Assignee != "" {
		conditions = append(conditions, fmt.Sprintf("assignee ILIKE $%d", argIdx))
		args = append(args, "%"+f.Assignee+"%")
		argIdx++
	}
	if f.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(summary ILIKE $%d OR description ILIKE $%d OR issue_key ILIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, "%"+f.Search+"%")
		argIdx++
	}
	if f.CreatedFrom != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *f.CreatedFrom)
		argIdx++
	}
	if f.CreatedTo != nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, *f.CreatedTo)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM jira_issues WHERE %s", where)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count jira issues: %w", err)
	}

	// Pagination
	page := f.Page
	if page < 1 {
		page = 1
	}
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	query := fmt.Sprintf(`
		SELECT id, tenant_id, issue_key, COALESCE(issue_id,''), project_key, summary,
		       COALESCE(description,''), COALESCE(issue_type,''), COALESCE(priority,''),
		       COALESCE(status,''), COALESCE(assignee,''), COALESCE(reporter,''),
		       COALESCE(labels, '{}'), COALESCE(components, '{}'), COALESCE(fix_versions, '{}'),
		       story_points, COALESCE(parent_key,''), COALESCE(jira_url,''),
		       linked_mr_id, COALESCE(linked_mr_url,''), deployment_id,
		       synced_at, created_at, updated_at
		FROM jira_issues WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query jira issues with filters: %w", err)
	}
	defer rows.Close()

	items := make([]JiraIssue, 0)
	for rows.Next() {
		var j JiraIssue
		if err := rows.Scan(&j.ID, &j.TenantID, &j.IssueKey, &j.IssueID, &j.ProjectKey,
			&j.Summary, &j.Description, &j.IssueType, &j.Priority, &j.Status,
			&j.Assignee, &j.Reporter, &j.Labels, &j.Components, &j.FixVersions,
			&j.StoryPoints, &j.ParentKey, &j.JiraURL, &j.LinkedMRID, &j.LinkedMRURL,
			&j.DeploymentID, &j.SyncedAt, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan jira issue: %w", err)
		}
		items = append(items, j)
	}
	return items, total, nil
}

// Get returns a Jira issue by ID.
func (r *JiraRepository) Get(ctx context.Context, id uuid.UUID) (*JiraIssue, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, issue_key, COALESCE(issue_id,''), project_key, summary,
		       COALESCE(description,''), COALESCE(issue_type,''), COALESCE(priority,''),
		       COALESCE(status,''), COALESCE(assignee,''), COALESCE(reporter,''),
		       COALESCE(labels, '{}'), COALESCE(components, '{}'), COALESCE(fix_versions, '{}'),
		       story_points, COALESCE(parent_key,''), COALESCE(jira_url,''),
		       linked_mr_id, COALESCE(linked_mr_url,''), deployment_id,
		       synced_at, created_at, updated_at
		FROM jira_issues WHERE id = $1
	`, id)

	var j JiraIssue
	if err := row.Scan(&j.ID, &j.TenantID, &j.IssueKey, &j.IssueID, &j.ProjectKey,
		&j.Summary, &j.Description, &j.IssueType, &j.Priority, &j.Status,
		&j.Assignee, &j.Reporter, &j.Labels, &j.Components, &j.FixVersions,
		&j.StoryPoints, &j.ParentKey, &j.JiraURL, &j.LinkedMRID, &j.LinkedMRURL,
		&j.DeploymentID, &j.SyncedAt, &j.CreatedAt, &j.UpdatedAt); err != nil {
		return nil, fmt.Errorf("get jira issue: %w", err)
	}
	return &j, nil
}

// GetByKey returns a Jira issue by issue_key and tenant.
func (r *JiraRepository) GetByKey(ctx context.Context, tenantID uuid.UUID, issueKey string) (*JiraIssue, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, issue_key, COALESCE(issue_id,''), project_key, summary,
		       COALESCE(description,''), COALESCE(issue_type,''), COALESCE(priority,''),
		       COALESCE(status,''), COALESCE(assignee,''), COALESCE(reporter,''),
		       COALESCE(labels, '{}'), COALESCE(components, '{}'), COALESCE(fix_versions, '{}'),
		       story_points, COALESCE(parent_key,''), COALESCE(jira_url,''),
		       linked_mr_id, COALESCE(linked_mr_url,''), deployment_id,
		       synced_at, created_at, updated_at
		FROM jira_issues WHERE tenant_id = $1 AND issue_key = $2
	`, tenantID, issueKey)

	var j JiraIssue
	if err := row.Scan(&j.ID, &j.TenantID, &j.IssueKey, &j.IssueID, &j.ProjectKey,
		&j.Summary, &j.Description, &j.IssueType, &j.Priority, &j.Status,
		&j.Assignee, &j.Reporter, &j.Labels, &j.Components, &j.FixVersions,
		&j.StoryPoints, &j.ParentKey, &j.JiraURL, &j.LinkedMRID, &j.LinkedMRURL,
		&j.DeploymentID, &j.SyncedAt, &j.CreatedAt, &j.UpdatedAt); err != nil {
		return nil, fmt.Errorf("get jira issue by key: %w", err)
	}
	return &j, nil
}

// Upsert creates or updates a Jira issue.
func (r *JiraRepository) Upsert(ctx context.Context, j *JiraIssue) error {
	now := time.Now().UTC()
	j.SyncedAt = now
	if j.CreatedAt.IsZero() {
		j.CreatedAt = now
	}
	j.UpdatedAt = now
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO jira_issues (id, tenant_id, issue_key, issue_id, project_key, summary,
			description, issue_type, priority, status, assignee, reporter,
			labels, components, fix_versions, story_points, parent_key,
			jira_url, linked_mr_id, linked_mr_url, deployment_id,
			synced_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
		ON CONFLICT (tenant_id, issue_key) DO UPDATE SET
			summary = EXCLUDED.summary, status = EXCLUDED.status,
			assignee = EXCLUDED.assignee, priority = EXCLUDED.priority,
			description = EXCLUDED.description, labels = EXCLUDED.labels,
			components = EXCLUDED.components, fix_versions = EXCLUDED.fix_versions,
			story_points = EXCLUDED.story_points, parent_key = EXCLUDED.parent_key,
			linked_mr_id = EXCLUDED.linked_mr_id, linked_mr_url = EXCLUDED.linked_mr_url,
			deployment_id = EXCLUDED.deployment_id, synced_at = EXCLUDED.synced_at,
			updated_at = EXCLUDED.updated_at
	`, j.ID, j.TenantID, j.IssueKey, j.IssueID, j.ProjectKey, j.Summary,
		j.Description, j.IssueType, j.Priority, j.Status, j.Assignee, j.Reporter,
		j.Labels, j.Components, j.FixVersions, j.StoryPoints, j.ParentKey,
		j.JiraURL, j.LinkedMRID, j.LinkedMRURL, j.DeploymentID,
		j.SyncedAt, j.CreatedAt, j.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert jira issue: %w", err)
	}
	return nil
}

// Update updates mutable fields of a Jira issue.
func (r *JiraRepository) Update(ctx context.Context, id uuid.UUID, summary, description, assignee, priority, status string, labels []string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE jira_issues SET summary=$2, description=$3, assignee=$4, priority=$5,
		       status=$6, labels=$7, updated_at=NOW()
		WHERE id=$1
	`, id, summary, description, assignee, priority, status, labels)
	if err != nil {
		return fmt.Errorf("update jira issue: %w", err)
	}
	return nil
}

// Delete removes a Jira issue from the local DB.
func (r *JiraRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM jira_issues WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete jira issue: %w", err)
	}
	return nil
}

// LinkDeployment links a Jira issue to a deployment.
func (r *JiraRepository) LinkDeployment(ctx context.Context, issueID, deploymentID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE jira_issues SET deployment_id=$2, updated_at=NOW() WHERE id=$1
	`, issueID, deploymentID)
	if err != nil {
		return fmt.Errorf("link deployment: %w", err)
	}
	return nil
}

// GetLabels returns all unique labels across synced issues for a tenant.
func (r *JiraRepository) GetLabels(ctx context.Context, tenantID uuid.UUID) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT unnest(labels) AS label
		FROM jira_issues WHERE tenant_id = $1
		ORDER BY label
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get labels: %w", err)
	}
	defer rows.Close()

	labels := make([]string, 0)
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, fmt.Errorf("scan label: %w", err)
		}
		labels = append(labels, l)
	}
	return labels, nil
}

// GetStats returns aggregated statistics for the dashboard.
func (r *JiraRepository) GetStats(ctx context.Context, tenantID uuid.UUID) (*JiraStats, error) {
	stats := &JiraStats{
		ByStatus:   make(map[string]int),
		ByType:     make(map[string]int),
		ByPriority: make(map[string]int),
	}

	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(status,'Unknown'), COALESCE(issue_type,'Unknown'),
		       COALESCE(priority,'None'), COUNT(*)
		FROM jira_issues WHERE tenant_id = $1
		GROUP BY status, issue_type, priority
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status, issueType, priority string
		var count int
		if err := rows.Scan(&status, &issueType, &priority, &count); err != nil {
			return nil, fmt.Errorf("scan stats: %w", err)
		}
		stats.ByStatus[status] += count
		stats.ByType[issueType] += count
		stats.ByPriority[priority] += count
		stats.Total += count
		if issueType == "Bug" && status != "Done" && status != "Closed" {
			stats.OpenBugs += count
		}
	}
	return stats, nil
}

// ── Comments ──────────────────────────────────────────────────

// ListComments returns cached comments for an issue.
func (r *JiraRepository) ListComments(ctx context.Context, tenantID uuid.UUID, issueKey string) ([]JiraComment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, issue_key, COALESCE(comment_id,''), COALESCE(author,''),
		       body, created_at, updated_at
		FROM jira_comments WHERE tenant_id = $1 AND issue_key = $2
		ORDER BY created_at ASC
	`, tenantID, issueKey)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	comments := make([]JiraComment, 0)
	for rows.Next() {
		var c JiraComment
		if err := rows.Scan(&c.ID, &c.TenantID, &c.IssueKey, &c.CommentID, &c.Author,
			&c.Body, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, nil
}

// UpsertComment caches a comment.
func (r *JiraRepository) UpsertComment(ctx context.Context, c *JiraComment) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO jira_comments (id, tenant_id, issue_key, comment_id, author, body, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (tenant_id, issue_key, comment_id) DO UPDATE SET
			body = EXCLUDED.body, author = EXCLUDED.author, updated_at = EXCLUDED.updated_at
	`, c.ID, c.TenantID, c.IssueKey, c.CommentID, c.Author, c.Body, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert comment: %w", err)
	}
	return nil
}

// ── Automation Rules ──────────────────────────────────────────

// ListAutomationRules returns all automation rules for a tenant.
func (r *JiraRepository) ListAutomationRules(ctx context.Context, tenantID uuid.UUID) ([]JiraAutomationRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), trigger_type,
		       COALESCE(jira_project_key,''), COALESCE(jql_filter,''),
		       action_type, action_config, enabled, last_triggered_at,
		       created_at, updated_at
		FROM jira_automation_rules WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list automation rules: %w", err)
	}
	defer rows.Close()

	rules := make([]JiraAutomationRule, 0)
	for rows.Next() {
		var rule JiraAutomationRule
		if err := rows.Scan(&rule.ID, &rule.TenantID, &rule.Name, &rule.Description,
			&rule.TriggerType, &rule.JiraProjectKey, &rule.JQLFilter,
			&rule.ActionType, &rule.ActionConfig, &rule.Enabled, &rule.LastTriggeredAt,
			&rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan automation rule: %w", err)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// GetAutomationRule returns a single automation rule.
func (r *JiraRepository) GetAutomationRule(ctx context.Context, id uuid.UUID) (*JiraAutomationRule, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), trigger_type,
		       COALESCE(jira_project_key,''), COALESCE(jql_filter,''),
		       action_type, action_config, enabled, last_triggered_at,
		       created_at, updated_at
		FROM jira_automation_rules WHERE id = $1
	`, id)

	var rule JiraAutomationRule
	if err := row.Scan(&rule.ID, &rule.TenantID, &rule.Name, &rule.Description,
		&rule.TriggerType, &rule.JiraProjectKey, &rule.JQLFilter,
		&rule.ActionType, &rule.ActionConfig, &rule.Enabled, &rule.LastTriggeredAt,
		&rule.CreatedAt, &rule.UpdatedAt); err != nil {
		return nil, fmt.Errorf("get automation rule: %w", err)
	}
	return &rule, nil
}

// CreateAutomationRule inserts a new automation rule.
func (r *JiraRepository) CreateAutomationRule(ctx context.Context, rule *JiraAutomationRule) error {
	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}
	now := time.Now().UTC()
	rule.CreatedAt = now
	rule.UpdatedAt = now

	_, err := r.pool.Exec(ctx, `
		INSERT INTO jira_automation_rules (id, tenant_id, name, description, trigger_type,
			jira_project_key, jql_filter, action_type, action_config, enabled, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, rule.ID, rule.TenantID, rule.Name, rule.Description, rule.TriggerType,
		rule.JiraProjectKey, rule.JQLFilter, rule.ActionType, rule.ActionConfig, rule.Enabled,
		rule.CreatedAt, rule.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create automation rule: %w", err)
	}
	return nil
}

// UpdateAutomationRule updates an existing automation rule.
func (r *JiraRepository) UpdateAutomationRule(ctx context.Context, id uuid.UUID, name, description, triggerType, jiraProjectKey, jqlFilter, actionType string, actionConfig json.RawMessage, enabled bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE jira_automation_rules SET name=$2, description=$3, trigger_type=$4,
			jira_project_key=$5, jql_filter=$6, action_type=$7, action_config=$8,
			enabled=$9, updated_at=NOW()
		WHERE id=$1
	`, id, name, description, triggerType, jiraProjectKey, jqlFilter, actionType, actionConfig, enabled)
	if err != nil {
		return fmt.Errorf("update automation rule: %w", err)
	}
	return nil
}

// DeleteAutomationRule removes an automation rule.
func (r *JiraRepository) DeleteAutomationRule(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM jira_automation_rules WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete automation rule: %w", err)
	}
	return nil
}

// GetAutomationRulesByTrigger returns enabled rules matching a trigger type.
func (r *JiraRepository) GetAutomationRulesByTrigger(ctx context.Context, tenantID uuid.UUID, triggerType string) ([]JiraAutomationRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), trigger_type,
		       COALESCE(jira_project_key,''), COALESCE(jql_filter,''),
		       action_type, action_config, enabled, last_triggered_at,
		       created_at, updated_at
		FROM jira_automation_rules
		WHERE tenant_id = $1 AND trigger_type = $2 AND enabled = true
		ORDER BY created_at DESC
	`, tenantID, triggerType)
	if err != nil {
		return nil, fmt.Errorf("get automation rules by trigger: %w", err)
	}
	defer rows.Close()

	rules := make([]JiraAutomationRule, 0)
	for rows.Next() {
		var rule JiraAutomationRule
		if err := rows.Scan(&rule.ID, &rule.TenantID, &rule.Name, &rule.Description,
			&rule.TriggerType, &rule.JiraProjectKey, &rule.JQLFilter,
			&rule.ActionType, &rule.ActionConfig, &rule.Enabled, &rule.LastTriggeredAt,
			&rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan automation rule: %w", err)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// MarkAutomationRuleTriggered updates the last_triggered_at timestamp.
func (r *JiraRepository) MarkAutomationRuleTriggered(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE jira_automation_rules SET last_triggered_at=NOW(), updated_at=NOW() WHERE id=$1
	`, id)
	if err != nil {
		return fmt.Errorf("mark automation rule triggered: %w", err)
	}
	return nil
}
