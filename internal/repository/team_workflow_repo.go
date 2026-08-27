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

// TeamWorkflowRepository handles persistence of per-team GitOps workflow configs.
type TeamWorkflowRepository struct {
	pool *pgxpool.Pool
}

// NewTeamWorkflowRepository creates a new team workflow repository.
func NewTeamWorkflowRepository(db *database.DB) *TeamWorkflowRepository {
	return &TeamWorkflowRepository{pool: db.Pool}
}

// TeamWorkflowConfig represents a team-specific workflow configuration.
type TeamWorkflowConfig struct {
	ID           uuid.UUID       `json:"-"`
	TenantID     uuid.UUID       `json:"-"`
	TeamName     string          `json:"team_name"`
	Stages       []WorkflowStage `json:"stages"`
	GitOps       GitOpsConfig    `json:"gitops"`
	CI           CIConfig        `json:"ci"`
	Verification VerifyConfig    `json:"verification"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// WorkflowStage is a single promotion stage in a team's pipeline.
type WorkflowStage struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Color       string `json:"color"`
	AutoPromote bool   `json:"auto_promote"`
	Approval    bool   `json:"requires_approval"`
}

// GitOpsConfig points at the manifest repository for the team.
type GitOpsConfig struct {
	Provider string `json:"provider"`
	RepoURL  string `json:"repo_url"`
	Branch   string `json:"branch"`
	Path     string `json:"path"`
}

// CIConfig describes the CI pipeline used to build images.
type CIConfig struct {
	Provider string `json:"provider"`
	Pipeline string `json:"pipeline"`
}

// VerifyConfig lists the verification checks run after a deploy.
type VerifyConfig struct {
	Checks []string `json:"checks"`
}

// List returns all team workflow configs for a tenant.
func (r *TeamWorkflowRepository) List(ctx context.Context, tenantID uuid.UUID) ([]TeamWorkflowConfig, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT team_name, stages, gitops, ci, verification, created_at, updated_at
		FROM team_workflow_configs
		WHERE tenant_id = $1
		ORDER BY team_name
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query team workflow configs: %w", err)
	}
	defer rows.Close()

	var items []TeamWorkflowConfig
	for rows.Next() {
		cfg, err := scanTeamWorkflowConfig(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, *cfg)
	}
	return items, nil
}

// Get returns a team workflow config by team name.
func (r *TeamWorkflowRepository) Get(ctx context.Context, tenantID uuid.UUID, team string) (*TeamWorkflowConfig, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT team_name, stages, gitops, ci, verification, created_at, updated_at
		FROM team_workflow_configs
		WHERE tenant_id = $1 AND team_name = $2
	`, tenantID, team)

	cfg, err := scanTeamWorkflowConfig(row.Scan)
	if err != nil {
		return nil, fmt.Errorf("get team workflow config: %w", err)
	}
	return cfg, nil
}

// Upsert creates or updates a team workflow config.
func (r *TeamWorkflowRepository) Upsert(ctx context.Context, tenantID uuid.UUID, cfg *TeamWorkflowConfig) error {
	stagesJSON, err := json.Marshal(cfg.Stages)
	if err != nil {
		return fmt.Errorf("marshal stages: %w", err)
	}
	gitopsJSON, err := json.Marshal(cfg.GitOps)
	if err != nil {
		return fmt.Errorf("marshal gitops: %w", err)
	}
	ciJSON, err := json.Marshal(cfg.CI)
	if err != nil {
		return fmt.Errorf("marshal ci: %w", err)
	}
	verifyJSON, err := json.Marshal(cfg.Verification)
	if err != nil {
		return fmt.Errorf("marshal verification: %w", err)
	}

	now := time.Now().UTC()
	err = r.pool.QueryRow(ctx, `
		INSERT INTO team_workflow_configs (id, tenant_id, team_name, stages, gitops, ci, verification, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $7)
		ON CONFLICT (tenant_id, team_name) DO UPDATE SET
			stages = EXCLUDED.stages,
			gitops = EXCLUDED.gitops,
			ci = EXCLUDED.ci,
			verification = EXCLUDED.verification,
			updated_at = EXCLUDED.updated_at
		RETURNING created_at, updated_at
	`, tenantID, cfg.TeamName, stagesJSON, gitopsJSON, ciJSON, verifyJSON, now).Scan(&cfg.CreatedAt, &cfg.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert team workflow config: %w", err)
	}
	return nil
}

// Delete removes a team workflow config.
func (r *TeamWorkflowRepository) Delete(ctx context.Context, tenantID uuid.UUID, team string) error {
	if _, err := r.pool.Exec(ctx, `
		DELETE FROM team_workflow_configs WHERE tenant_id = $1 AND team_name = $2
	`, tenantID, team); err != nil {
		return fmt.Errorf("delete team workflow config: %w", err)
	}
	return nil
}

// scanTeamWorkflowConfig scans a (team_name, stages, gitops, ci, verification, created_at, updated_at) row.
func scanTeamWorkflowConfig(scan func(dest ...any) error) (*TeamWorkflowConfig, error) {
	var (
		cfg        TeamWorkflowConfig
		stagesJSON []byte
		gitopsJSON []byte
		ciJSON     []byte
		verifyJSON []byte
	)
	if err := scan(&cfg.TeamName, &stagesJSON, &gitopsJSON, &ciJSON, &verifyJSON, &cfg.CreatedAt, &cfg.UpdatedAt); err != nil {
		return nil, err
	}
	if len(stagesJSON) > 0 {
		_ = json.Unmarshal(stagesJSON, &cfg.Stages)
	}
	if cfg.Stages == nil {
		cfg.Stages = []WorkflowStage{}
	}
	if len(gitopsJSON) > 0 {
		_ = json.Unmarshal(gitopsJSON, &cfg.GitOps)
	}
	if len(ciJSON) > 0 {
		_ = json.Unmarshal(ciJSON, &cfg.CI)
	}
	if len(verifyJSON) > 0 {
		_ = json.Unmarshal(verifyJSON, &cfg.Verification)
	}
	if cfg.Verification.Checks == nil {
		cfg.Verification.Checks = []string{}
	}
	return &cfg, nil
}
