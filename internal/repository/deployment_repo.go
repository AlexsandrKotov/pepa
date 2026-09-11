package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/database"
)

// DeploymentRepository handles deployment persistence.
type DeploymentRepository struct {
	pool *pgxpool.Pool
}

// NewDeploymentRepository creates a new deployment repository.
func NewDeploymentRepository(db *database.DB) *DeploymentRepository {
	return &DeploymentRepository{pool: db.Pool}
}

// Deployment represents a release deployment.
type Deployment struct {
	ID                uuid.UUID       `json:"id"`
	TenantID          uuid.UUID       `json:"tenant_id"`
	JiraIssueKey      string          `json:"jira_issue_key,omitempty"`
	JiraSummary       string          `json:"jira_summary,omitempty"`
	GitlabProjectID   *int            `json:"gitlab_project_id,omitempty"`
	GitlabProjectName string          `json:"gitlab_project_name,omitempty"`
	GitlabMRID        *int            `json:"gitlab_mr_id,omitempty"`
	GitlabMRURL       string          `json:"gitlab_mr_url,omitempty"`
	TargetClusterID   *uuid.UUID      `json:"target_cluster_id,omitempty"`
	TargetNamespace   string          `json:"target_namespace,omitempty"`
	ImageTag          string          `json:"image_tag,omitempty"`
	ImageRepository   string          `json:"image_repository,omitempty"`
	DeployType        string          `json:"deploy_type,omitempty"`
	Replicas          int             `json:"replicas,omitempty"`
	Strategy          string          `json:"strategy,omitempty"`
	Spec              json.RawMessage `json:"spec,omitempty"`
	Status            string          `json:"status"`
	ErrorMessage      string          `json:"error_message,omitempty"`
	Logs              string          `json:"logs,omitempty"`
	PromotedBy        string          `json:"promoted_by,omitempty"`
	PromotedAt        *time.Time      `json:"promoted_at,omitempty"`
	CreatedBy         string          `json:"created_by,omitempty"`
	TimeoutSeconds    int             `json:"timeout_seconds,omitempty"`
	TeamName          string          `json:"team_name,omitempty"`
	Stage             string          `json:"stage,omitempty"`
	EnvironmentID     *uuid.UUID      `json:"environment_id,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// List returns all deployments for a tenant.
func (r *DeploymentRepository) List(ctx context.Context, tenantID uuid.UUID) ([]Deployment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, COALESCE(jira_issue_key,''), COALESCE(jira_summary,''),
		       gitlab_project_id, COALESCE(gitlab_project_name,''),
		       gitlab_mr_id, COALESCE(gitlab_mr_url,''),
		       target_cluster_id, COALESCE(target_namespace,''),
		       COALESCE(image_tag,''), COALESCE(image_repository,''),
		       COALESCE(deploy_type,'helm'), COALESCE(replicas,1), COALESCE(strategy,'rolling'),
		       COALESCE(spec,'{}'::jsonb),
		       status, COALESCE(error_message,''), COALESCE(logs,''),
		       COALESCE(promoted_by,''), promoted_at,
		       COALESCE(created_by,''), COALESCE(timeout_seconds,300),
		       COALESCE(team_name,''), COALESCE(stage,'dev'),
		       environment_id,
		       created_at, updated_at
		FROM deployments WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query deployments: %w", err)
	}
	defer rows.Close()

	var items []Deployment
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(&d.ID, &d.TenantID, &d.JiraIssueKey, &d.JiraSummary,
			&d.GitlabProjectID, &d.GitlabProjectName, &d.GitlabMRID, &d.GitlabMRURL,
			&d.TargetClusterID, &d.TargetNamespace, &d.ImageTag, &d.ImageRepository,
			&d.DeployType, &d.Replicas, &d.Strategy, &d.Spec,
			&d.Status, &d.ErrorMessage, &d.Logs,
			&d.PromotedBy, &d.PromotedAt, &d.CreatedBy,
			&d.TimeoutSeconds,
			&d.TeamName, &d.Stage,
			&d.EnvironmentID,
			&d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan deployment: %w", err)
		}
		items = append(items, d)
	}
	return items, nil
}

// DeploymentFilter defines filtering and pagination parameters for deployment lists.
type DeploymentFilter struct {
	TenantID    uuid.UUID
	Page        int
	PerPage     int
	Search      string
	Status      string
	TeamName    string
	ServiceName string
	ClusterID   string
	Stage       string
}

// DeploymentListResponse is the paginated result of ListFiltered.
type DeploymentListResponse struct {
	Items      []Deployment `json:"items"`
	Total      int64        `json:"total"`
	Page       int          `json:"page"`
	PerPage    int          `json:"per_page"`
	TotalPages int          `json:"total_pages"`
}

// ListFiltered returns deployments with filtering and pagination.
func (r *DeploymentRepository) ListFiltered(ctx context.Context, f DeploymentFilter) (*DeploymentListResponse, error) {
	query := `
		SELECT id, tenant_id, COALESCE(jira_issue_key,''), COALESCE(jira_summary,''),
		       gitlab_project_id, COALESCE(gitlab_project_name,''),
		       gitlab_mr_id, COALESCE(gitlab_mr_url,''),
		       target_cluster_id, COALESCE(target_namespace,''),
		       COALESCE(image_tag,''), COALESCE(image_repository,''),
		       COALESCE(deploy_type,'helm'), COALESCE(replicas,1), COALESCE(strategy,'rolling'),
		       COALESCE(spec,'{}'::jsonb),
		       status, COALESCE(error_message,''), COALESCE(logs,''),
		       COALESCE(promoted_by,''), promoted_at,
		       COALESCE(created_by,''), COALESCE(timeout_seconds,300),
		       COALESCE(team_name,''), COALESCE(stage,'dev'),
		       environment_id,
		       created_at, updated_at
		FROM deployments WHERE tenant_id = $1`
	args := []interface{}{f.TenantID}
	argIdx := 2

	if f.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, f.Status)
		argIdx++
	}
	if f.TeamName != "" {
		query += fmt.Sprintf(" AND team_name = $%d", argIdx)
		args = append(args, f.TeamName)
		argIdx++
	}
	if f.ServiceName != "" {
		query += fmt.Sprintf(" AND gitlab_project_name = $%d", argIdx)
		args = append(args, f.ServiceName)
		argIdx++
	}
	if f.ClusterID != "" {
		query += fmt.Sprintf(" AND target_cluster_id::text = $%d", argIdx)
		args = append(args, f.ClusterID)
		argIdx++
	}
	if f.Stage != "" {
		query += fmt.Sprintf(" AND stage = $%d", argIdx)
		args = append(args, f.Stage)
		argIdx++
	}
	if f.Search != "" {
		query += fmt.Sprintf(" AND (gitlab_project_name ILIKE $%d OR jira_issue_key ILIKE $%d OR jira_summary ILIKE $%d)", argIdx, argIdx, argIdx)
		args = append(args, "%"+f.Search+"%")
		argIdx++
	}

	// Count
	countQuery := "SELECT COUNT(*) FROM (" + query + ") sub"
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count deployments: %w", err)
	}

	if f.Page < 1 {
		f.Page = 1
	}
	if f.PerPage < 1 {
		f.PerPage = 20
	}
	offset := (f.Page - 1) * f.PerPage

	query += " ORDER BY created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, f.PerPage, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query deployments: %w", err)
	}
	defer rows.Close()

	items := make([]Deployment, 0)
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(&d.ID, &d.TenantID, &d.JiraIssueKey, &d.JiraSummary,
			&d.GitlabProjectID, &d.GitlabProjectName, &d.GitlabMRID, &d.GitlabMRURL,
			&d.TargetClusterID, &d.TargetNamespace, &d.ImageTag, &d.ImageRepository,
			&d.DeployType, &d.Replicas, &d.Strategy, &d.Spec,
			&d.Status, &d.ErrorMessage, &d.Logs,
			&d.PromotedBy, &d.PromotedAt, &d.CreatedBy,
			&d.TimeoutSeconds,
			&d.TeamName, &d.Stage,
			&d.EnvironmentID,
			&d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan deployment: %w", err)
		}
		items = append(items, d)
	}

	totalPages := int(total) / f.PerPage
	if int(total)%f.PerPage > 0 {
		totalPages++
	}

	return &DeploymentListResponse{
		Items:      items,
		Total:      total,
		Page:       f.Page,
		PerPage:    f.PerPage,
		TotalPages: totalPages,
	}, nil
}

// Get returns a deployment by ID.
func (r *DeploymentRepository) Get(ctx context.Context, id uuid.UUID) (*Deployment, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, COALESCE(jira_issue_key,''), COALESCE(jira_summary,''),
		       gitlab_project_id, COALESCE(gitlab_project_name,''),
		       gitlab_mr_id, COALESCE(gitlab_mr_url,''),
		       target_cluster_id, COALESCE(target_namespace,''),
		       COALESCE(image_tag,''), COALESCE(image_repository,''),
		       COALESCE(deploy_type,'helm'), COALESCE(replicas,1), COALESCE(strategy,'rolling'),
		       COALESCE(spec,'{}'::jsonb),
		       status, COALESCE(error_message,''), COALESCE(logs,''),
		       COALESCE(promoted_by,''), promoted_at,
		       COALESCE(created_by,''), COALESCE(timeout_seconds,300),
		       COALESCE(team_name,''), COALESCE(stage,'dev'),
		       environment_id,
		       created_at, updated_at
		FROM deployments WHERE id = $1
	`, id)

	var d Deployment
	if err := row.Scan(&d.ID, &d.TenantID, &d.JiraIssueKey, &d.JiraSummary,
		&d.GitlabProjectID, &d.GitlabProjectName, &d.GitlabMRID, &d.GitlabMRURL,
		&d.TargetClusterID, &d.TargetNamespace, &d.ImageTag, &d.ImageRepository,
		&d.DeployType, &d.Replicas, &d.Strategy, &d.Spec,
		&d.Status, &d.ErrorMessage, &d.Logs,
		&d.PromotedBy, &d.PromotedAt, &d.CreatedBy,
		&d.TimeoutSeconds,
		&d.TeamName, &d.Stage,
		&d.EnvironmentID,
		&d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	return &d, nil
}

// Create inserts a new deployment.
func (r *DeploymentRepository) Create(ctx context.Context, d *Deployment) error {
	d.ID = uuid.New()
	now := time.Now().UTC()
	d.CreatedAt = now
	d.UpdatedAt = now

	_, err := r.pool.Exec(ctx, `
		INSERT INTO deployments (id, tenant_id, jira_issue_key, jira_summary,
			gitlab_project_id, gitlab_project_name, gitlab_mr_id, gitlab_mr_url,
			target_cluster_id, target_namespace, image_tag, image_repository,
			deploy_type, replicas, strategy, spec,
			status, error_message, logs, created_by, timeout_seconds, team_name, stage, environment_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)
	`, d.ID, d.TenantID, d.JiraIssueKey, d.JiraSummary,
		d.GitlabProjectID, d.GitlabProjectName, d.GitlabMRID, d.GitlabMRURL,
		d.TargetClusterID, d.TargetNamespace, d.ImageTag, d.ImageRepository,
		d.DeployType, d.Replicas, d.Strategy, d.Spec,
		d.Status, d.ErrorMessage, d.Logs, d.CreatedBy, d.TimeoutSeconds, d.TeamName, d.Stage, d.EnvironmentID, d.CreatedAt, d.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}
	return nil
}

// Promote marks a deployment as promoted to the next environment.
func (r *DeploymentRepository) Promote(ctx context.Context, id uuid.UUID, promotedBy string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE deployments SET status='promoted', promoted_by=$2, promoted_at=$3, updated_at=$3
		WHERE id=$1
	`, id, promotedBy, now)
	if err != nil {
		return fmt.Errorf("promote deployment: %w", err)
	}
	return nil
}

// Rollback reverts a deployment status to rolled_back and creates a new rollback entry.
func (r *DeploymentRepository) Rollback(ctx context.Context, id uuid.UUID, rolledBackBy string) error {
	now := time.Now().UTC()
	tag, err := r.pool.Exec(ctx, `
		UPDATE deployments SET status='rolled_back', promoted_by=$2, updated_at=$3
		WHERE id=$1 AND status IN ('deployed','promoted')
	`, id, rolledBackBy, now)
	if err != nil {
		return fmt.Errorf("rollback deployment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deployment cannot be rolled back: must be in 'deployed' or 'promoted' status")
	}
	return nil
}

// Cancel marks a deployment as cancelled.
func (r *DeploymentRepository) Cancel(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	tag, err := r.pool.Exec(ctx, `
		UPDATE deployments SET status='cancelled', updated_at=$2
		WHERE id=$1 AND status IN ('pending','syncing')
	`, id, now)
	if err != nil {
		return fmt.Errorf("cancel deployment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deployment cannot be cancelled: must be in 'pending' or 'syncing' status")
	}
	return nil
}

// Update updates a deployment's mutable fields.
func (r *DeploymentRepository) Update(ctx context.Context, d *Deployment) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE deployments SET status=$2, target_namespace=$3, image_tag=$4,
		       image_repository=$5, promoted_by=$6, updated_at=$7,
		       replicas=$8, strategy=$9, spec=$10, error_message=$11, logs=$12
		WHERE id=$1
	`, d.ID, d.Status, d.TargetNamespace, d.ImageTag, d.ImageRepository, d.PromotedBy, d.UpdatedAt,
		d.Replicas, d.Strategy, d.Spec, d.ErrorMessage, d.Logs)
	if err != nil {
		return fmt.Errorf("update deployment: %w", err)
	}
	return nil
}

// History returns deployments for the same project/namespace as the given deployment.
func (r *DeploymentRepository) History(ctx context.Context, tenantID uuid.UUID, projectName string, namespace string, limit int) ([]Deployment, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, COALESCE(jira_issue_key,''), COALESCE(jira_summary,''),
		       gitlab_project_id, COALESCE(gitlab_project_name,''),
		       gitlab_mr_id, COALESCE(gitlab_mr_url,''),
		       target_cluster_id, COALESCE(target_namespace,''),
		       COALESCE(image_tag,''), COALESCE(image_repository,''),
		       COALESCE(deploy_type,'helm'), COALESCE(replicas,1), COALESCE(strategy,'rolling'),
		       COALESCE(spec,'{}'::jsonb),
		       status, COALESCE(error_message,''), COALESCE(logs,''),
		       COALESCE(promoted_by,''), promoted_at,
		       COALESCE(created_by,''), COALESCE(timeout_seconds,300),
		       COALESCE(team_name,''), COALESCE(stage,'dev'),
		       environment_id,
		       created_at, updated_at
		FROM deployments
		WHERE tenant_id = $1 AND gitlab_project_name = $2 AND target_namespace = $3
		ORDER BY created_at DESC
		LIMIT $4
	`, tenantID, projectName, namespace, limit)
	if err != nil {
		return nil, fmt.Errorf("query deployment history: %w", err)
	}
	defer rows.Close()

	var items []Deployment
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(&d.ID, &d.TenantID, &d.JiraIssueKey, &d.JiraSummary,
			&d.GitlabProjectID, &d.GitlabProjectName, &d.GitlabMRID, &d.GitlabMRURL,
			&d.TargetClusterID, &d.TargetNamespace, &d.ImageTag, &d.ImageRepository,
			&d.DeployType, &d.Replicas, &d.Strategy, &d.Spec,
			&d.Status, &d.ErrorMessage, &d.Logs,
			&d.PromotedBy, &d.PromotedAt, &d.CreatedBy,
			&d.TimeoutSeconds,
			&d.TeamName, &d.Stage,
			&d.EnvironmentID,
			&d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan deployment: %w", err)
		}
		items = append(items, d)
	}
	return items, nil
}

// Delete removes a deployment by ID.
func (r *DeploymentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM deployments WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete deployment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deployment not found")
	}
	return nil
}
