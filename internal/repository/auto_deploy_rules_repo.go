package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AutoDeployRule maps a branch pattern to an environment for webhook-triggered deploys.
type AutoDeployRule struct {
	ID                   uuid.UUID  `json:"id"`
	TenantID             uuid.UUID  `json:"tenant_id"`
	PipelineSourceID     *uuid.UUID `json:"pipeline_source_id,omitempty"`
	ProjectID            *string    `json:"project_id,omitempty"`
	ProjectPath          *string    `json:"project_path,omitempty"`
	BranchPattern        string     `json:"branch_pattern"`
	EnvironmentID        uuid.UUID  `json:"environment_id"`
	ImageTagSource       string     `json:"image_tag_source"`        // branch_name, ci_variable, regex
	ImageTagRegex        *string    `json:"image_tag_regex,omitempty"`
	ImageName            *string    `json:"image_name,omitempty"`
	Enabled              bool       `json:"enabled"`
	RequirePipelineOK    bool       `json:"require_pipeline_success"`
	AutoCreateDeployment bool       `json:"auto_create_deployment"`
	CreatedBy            *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`

	// Joined fields (populated by ListWithDetails)
	EnvName *string `json:"env_name,omitempty"`
	EnvSlug *string `json:"env_slug,omitempty"`
	EnvColor *string `json:"env_color,omitempty"`
	SourceName *string `json:"source_name,omitempty"`
}

// AutoDeployRuleRepository handles persistence for auto_deploy_rules.
type AutoDeployRuleRepository struct {
	pool *pgxpool.Pool
}

// NewAutoDeployRuleRepository creates a new repository.
func NewAutoDeployRuleRepository(pool *pgxpool.Pool) *AutoDeployRuleRepository {
	return &AutoDeployRuleRepository{pool: pool}
}

// Create inserts a new rule.
func (r *AutoDeployRuleRepository) Create(ctx context.Context, rule *AutoDeployRule) error {
	rule.ID = uuid.New()
	now := time.Now().UTC()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	if rule.ImageTagSource == "" {
		rule.ImageTagSource = "branch_name"
	}

	return r.pool.QueryRow(ctx, `
		INSERT INTO auto_deploy_rules (
			id, tenant_id, pipeline_source_id, project_id, project_path,
			branch_pattern, environment_id, image_tag_source, image_tag_regex,
			image_name, enabled, require_pipeline_success, auto_create_deployment,
			created_by, created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16
		) RETURNING created_at, updated_at
	`, rule.ID, rule.TenantID, rule.PipelineSourceID, rule.ProjectID, rule.ProjectPath,
		rule.BranchPattern, rule.EnvironmentID, rule.ImageTagSource, rule.ImageTagRegex,
		rule.ImageName, rule.Enabled, rule.RequirePipelineOK, rule.AutoCreateDeployment,
		rule.CreatedBy, rule.CreatedAt, rule.UpdatedAt,
	).Scan(&rule.CreatedAt, &rule.UpdatedAt)
}

// Get retrieves a rule by ID.
func (r *AutoDeployRuleRepository) Get(ctx context.Context, id, tenantID uuid.UUID) (*AutoDeployRule, error) {
	rule := &AutoDeployRule{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, pipeline_source_id, project_id, project_path,
			branch_pattern, environment_id, image_tag_source, image_tag_regex,
			image_name, enabled, require_pipeline_success, auto_create_deployment,
			created_by, created_at, updated_at
		FROM auto_deploy_rules
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(
		&rule.ID, &rule.TenantID, &rule.PipelineSourceID, &rule.ProjectID, &rule.ProjectPath,
		&rule.BranchPattern, &rule.EnvironmentID, &rule.ImageTagSource, &rule.ImageTagRegex,
		&rule.ImageName, &rule.Enabled, &rule.RequirePipelineOK, &rule.AutoCreateDeployment,
		&rule.CreatedBy, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("auto-deploy rule not found: %s", id)
		}
		return nil, fmt.Errorf("get auto-deploy rule: %w", err)
	}
	return rule, nil
}

// List returns all rules for a tenant.
func (r *AutoDeployRuleRepository) List(ctx context.Context, tenantID uuid.UUID) ([]*AutoDeployRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.id, r.tenant_id, r.pipeline_source_id, r.project_id, r.project_path,
			r.branch_pattern, r.environment_id, r.image_tag_source, r.image_tag_regex,
			r.image_name, r.enabled, r.require_pipeline_success, r.auto_create_deployment,
			r.created_by, r.created_at, r.updated_at,
			e.name, e.slug, e.color,
			ps.name
		FROM auto_deploy_rules r
		LEFT JOIN environments e ON e.id = r.environment_id
		LEFT JOIN pipeline_sources ps ON ps.id = r.pipeline_source_id
		WHERE r.tenant_id = $1
		ORDER BY r.created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list auto-deploy rules: %w", err)
	}
	defer rows.Close()

	var rules []*AutoDeployRule
	for rows.Next() {
		rule := &AutoDeployRule{}
		if err := rows.Scan(
			&rule.ID, &rule.TenantID, &rule.PipelineSourceID, &rule.ProjectID, &rule.ProjectPath,
			&rule.BranchPattern, &rule.EnvironmentID, &rule.ImageTagSource, &rule.ImageTagRegex,
			&rule.ImageName, &rule.Enabled, &rule.RequirePipelineOK, &rule.AutoCreateDeployment,
			&rule.CreatedBy, &rule.CreatedAt, &rule.UpdatedAt,
			&rule.EnvName, &rule.EnvSlug, &rule.EnvColor,
			&rule.SourceName,
		); err != nil {
			return nil, fmt.Errorf("scan auto-deploy rule: %w", err)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// Update modifies an existing rule.
func (r *AutoDeployRuleRepository) Update(ctx context.Context, rule *AutoDeployRule) error {
	return r.pool.QueryRow(ctx, `
		UPDATE auto_deploy_rules SET
			pipeline_source_id = $3, project_id = $4, project_path = $5,
			branch_pattern = $6, environment_id = $7, image_tag_source = $8,
			image_tag_regex = $9, image_name = $10, enabled = $11,
			require_pipeline_success = $12, auto_create_deployment = $13,
			updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2
		RETURNING updated_at
	`, rule.ID, rule.TenantID, rule.PipelineSourceID, rule.ProjectID, rule.ProjectPath,
		rule.BranchPattern, rule.EnvironmentID, rule.ImageTagSource, rule.ImageTagRegex,
		rule.ImageName, rule.Enabled, rule.RequirePipelineOK, rule.AutoCreateDeployment,
	).Scan(&rule.UpdatedAt)
}

// Delete removes a rule.
func (r *AutoDeployRuleRepository) Delete(ctx context.Context, id, tenantID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		"DELETE FROM auto_deploy_rules WHERE id = $1 AND tenant_id = $2",
		id, tenantID)
	return err
}

// FindMatchingRules returns all enabled rules that match the given branch and project.
func (r *AutoDeployRuleRepository) FindMatchingRules(ctx context.Context, tenantID uuid.UUID, branch string, projectID string, projectPath string) ([]*AutoDeployRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.id, r.tenant_id, r.pipeline_source_id, r.project_id, r.project_path,
			r.branch_pattern, r.environment_id, r.image_tag_source, r.image_tag_regex,
			r.image_name, r.enabled, r.require_pipeline_success, r.auto_create_deployment,
			r.created_by, r.created_at, r.updated_at,
			e.name, e.slug, e.color,
			ps.name
		FROM auto_deploy_rules r
		LEFT JOIN environments e ON e.id = r.environment_id
		LEFT JOIN pipeline_sources ps ON ps.id = r.pipeline_source_id
		WHERE r.tenant_id = $1
		  AND r.enabled = true
		  AND (r.project_id = $2 OR r.project_path = $3 OR (r.project_id IS NULL AND r.project_path IS NULL))
		ORDER BY r.created_at ASC
	`, tenantID, projectID, projectPath)
	if err != nil {
		return nil, fmt.Errorf("find matching rules: %w", err)
	}
	defer rows.Close()

	var rules []*AutoDeployRule
	for rows.Next() {
		rule := &AutoDeployRule{}
		if err := rows.Scan(
			&rule.ID, &rule.TenantID, &rule.PipelineSourceID, &rule.ProjectID, &rule.ProjectPath,
			&rule.BranchPattern, &rule.EnvironmentID, &rule.ImageTagSource, &rule.ImageTagRegex,
			&rule.ImageName, &rule.Enabled, &rule.RequirePipelineOK, &rule.AutoCreateDeployment,
			&rule.CreatedBy, &rule.CreatedAt, &rule.UpdatedAt,
			&rule.EnvName, &rule.EnvSlug, &rule.EnvColor,
			&rule.SourceName,
		); err != nil {
			return nil, fmt.Errorf("scan matching rule: %w", err)
		}
		// Filter by branch pattern match (glob matching in Go)
		if matchBranch(branch, rule.BranchPattern) {
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

// matchBranch checks if a branch name matches a glob pattern.
// Supports * (any chars) and ? (single char) wildcards.
func matchBranch(branch, pattern string) bool {
	if pattern == branch {
		return true
	}
	// Use filepath.Match for glob matching (supports * and ?)
	matched, err := filepath.Match(pattern, branch)
	if err != nil {
		return false
	}
	if matched {
		return true
	}
	// Also support prefix matching with trailing /*
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		if strings.HasPrefix(branch, prefix+"/") {
			return true
		}
	}
	return false
}
