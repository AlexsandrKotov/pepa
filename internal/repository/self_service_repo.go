package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SelfServiceDeployment represents a developer self-service deployment request.
type SelfServiceDeployment struct {
	ID            uuid.UUID  `json:"id"`
	TenantID      uuid.UUID  `json:"tenant_id"`
	UserID        uuid.UUID  `json:"user_id"`
	GitRepoURL    string     `json:"git_repo_url"`
	GitBranch     string     `json:"git_branch"`
	GitCommitSHA  *string    `json:"git_commit_sha"`
	EnvironmentID *uuid.UUID `json:"environment_id"`
	ServiceID     *uuid.UUID `json:"service_id"`
	BlueprintType string     `json:"blueprint_type"`
	BlueprintConfig map[string]interface{} `json:"blueprint_config"`
	DeploymentID  *uuid.UUID `json:"deployment_id"`
	Status        string     `json:"status"`
	Progress      int        `json:"progress"`
	Logs          string     `json:"logs"`
	ErrorMessage  *string    `json:"error_message"`
	IsPreview     bool       `json:"is_preview"`
	PRNumber      *int       `json:"pr_number"`
	PreviewURL    *string    `json:"preview_url"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	DeployedAt    *time.Time `json:"deployed_at"`
}

// SelfServiceDeploymentRepository handles self-service deployment persistence.
type SelfServiceDeploymentRepository struct {
	pool *pgxpool.Pool
}

// NewSelfServiceDeploymentRepository creates a new repository.
func NewSelfServiceDeploymentRepository(pool *pgxpool.Pool) *SelfServiceDeploymentRepository {
	return &SelfServiceDeploymentRepository{pool: pool}
}

// Create creates a new self-service deployment request.
func (r *SelfServiceDeploymentRepository) Create(ctx context.Context, d *SelfServiceDeployment) error {
	now := time.Now().UTC()
	d.CreatedAt = now
	d.UpdatedAt = now
	if d.Status == "" {
		d.Status = "pending"
	}
	if d.BlueprintType == "" {
		d.BlueprintType = "auto"
	}

	configJSON, err := json.Marshal(d.BlueprintConfig)
	if err != nil {
		return fmt.Errorf("marshal blueprint config: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		INSERT INTO self_service_deployments (
			tenant_id, user_id, git_repo_url, git_branch, git_commit_sha,
			environment_id, service_id, blueprint_type, blueprint_config,
			deployment_id, status, progress, logs, error_message,
			is_preview, pr_number, preview_url, created_at, updated_at, deployed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		RETURNING id
	`, d.TenantID, d.UserID, d.GitRepoURL, d.GitBranch, d.GitCommitSHA,
		d.EnvironmentID, d.ServiceID, d.BlueprintType, configJSON,
		d.DeploymentID, d.Status, d.Progress, d.Logs, d.ErrorMessage,
		d.IsPreview, d.PRNumber, d.PreviewURL, d.CreatedAt, d.UpdatedAt, d.DeployedAt,
	).Scan(&d.ID)
	if err != nil {
		return fmt.Errorf("create self-service deployment: %w", err)
	}
	return nil
}

// Get returns a self-service deployment by ID.
func (r *SelfServiceDeploymentRepository) Get(ctx context.Context, tenantID, id uuid.UUID) (*SelfServiceDeployment, error) {
	var d SelfServiceDeployment
	var configJSON []byte

	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, user_id, git_repo_url, git_branch, git_commit_sha,
		       environment_id, service_id, blueprint_type, blueprint_config,
		       deployment_id, status, progress, logs, error_message,
		       is_preview, pr_number, preview_url, created_at, updated_at, deployed_at
		FROM self_service_deployments
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(
		&d.ID, &d.TenantID, &d.UserID, &d.GitRepoURL, &d.GitBranch, &d.GitCommitSHA,
		&d.EnvironmentID, &d.ServiceID, &d.BlueprintType, &configJSON,
		&d.DeploymentID, &d.Status, &d.Progress, &d.Logs, &d.ErrorMessage,
		&d.IsPreview, &d.PRNumber, &d.PreviewURL, &d.CreatedAt, &d.UpdatedAt, &d.DeployedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("self-service deployment not found")
		}
		return nil, fmt.Errorf("get self-service deployment: %w", err)
	}

	if configJSON != nil {
		if err := json.Unmarshal(configJSON, &d.BlueprintConfig); err != nil {
			d.BlueprintConfig = make(map[string]interface{})
		}
	}
	return &d, nil
}

// List returns self-service deployments for a tenant with optional filtering.
func (r *SelfServiceDeploymentRepository) List(ctx context.Context, tenantID uuid.UUID, userID *uuid.UUID, status string, limit int) ([]SelfServiceDeployment, error) {
	query := `
		SELECT id, tenant_id, user_id, git_repo_url, git_branch, git_commit_sha,
		       environment_id, service_id, blueprint_type, blueprint_config,
		       deployment_id, status, progress, logs, error_message,
		       is_preview, pr_number, preview_url, created_at, updated_at, deployed_at
		FROM self_service_deployments
		WHERE tenant_id = $1
	`
	args := []interface{}{tenantID}
	argIdx := 2

	if userID != nil {
		query += fmt.Sprintf(" AND user_id = $%d", argIdx)
		args = append(args, *userID)
		argIdx++
	}
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	} else {
		query += " LIMIT 100"
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list self-service deployments: %w", err)
	}
	defer rows.Close()

	var results []SelfServiceDeployment
	for rows.Next() {
		var d SelfServiceDeployment
		var configJSON []byte
		if err := rows.Scan(
			&d.ID, &d.TenantID, &d.UserID, &d.GitRepoURL, &d.GitBranch, &d.GitCommitSHA,
			&d.EnvironmentID, &d.ServiceID, &d.BlueprintType, &configJSON,
			&d.DeploymentID, &d.Status, &d.Progress, &d.Logs, &d.ErrorMessage,
			&d.IsPreview, &d.PRNumber, &d.PreviewURL, &d.CreatedAt, &d.UpdatedAt, &d.DeployedAt,
		); err != nil {
			return nil, fmt.Errorf("scan self-service deployment: %w", err)
		}
		if configJSON != nil {
			if err := json.Unmarshal(configJSON, &d.BlueprintConfig); err != nil {
				d.BlueprintConfig = make(map[string]interface{})
			}
		}
		results = append(results, d)
	}
	return results, nil
}

// Update updates a self-service deployment.
func (r *SelfServiceDeploymentRepository) Update(ctx context.Context, d *SelfServiceDeployment) error {
	d.UpdatedAt = time.Now().UTC()

	configJSON, err := json.Marshal(d.BlueprintConfig)
	if err != nil {
		return fmt.Errorf("marshal blueprint config: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		UPDATE self_service_deployments SET
			git_repo_url = $3, git_branch = $4, git_commit_sha = $5,
			environment_id = $6, service_id = $7, blueprint_type = $8, blueprint_config = $9,
			deployment_id = $10, status = $11, progress = $12, logs = $13, error_message = $14,
			is_preview = $15, pr_number = $16, preview_url = $17, updated_at = $18, deployed_at = $19
		WHERE tenant_id = $1 AND id = $2
	`, d.TenantID, d.ID, d.GitRepoURL, d.GitBranch, d.GitCommitSHA,
		d.EnvironmentID, d.ServiceID, d.BlueprintType, configJSON,
		d.DeploymentID, d.Status, d.Progress, d.Logs, d.ErrorMessage,
		d.IsPreview, d.PRNumber, d.PreviewURL, d.UpdatedAt, d.DeployedAt,
	)
	if err != nil {
		return fmt.Errorf("update self-service deployment: %w", err)
	}
	return nil
}

// UpdateStatus updates only the status, progress, and logs of a self-service deployment.
func (r *SelfServiceDeploymentRepository) UpdateStatus(ctx context.Context, tenantID, id uuid.UUID, status string, progress int, logs string, errMsg *string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE self_service_deployments SET
			status = $3, progress = $4, logs = $5, error_message = $6, updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id, status, progress, logs, errMsg)
	if err != nil {
		return fmt.Errorf("update self-service deployment status: %w", err)
	}
	return nil
}

// Delete deletes a self-service deployment.
func (r *SelfServiceDeploymentRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM self_service_deployments WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	if err != nil {
		return fmt.Errorf("delete self-service deployment: %w", err)
	}
	return nil
}
