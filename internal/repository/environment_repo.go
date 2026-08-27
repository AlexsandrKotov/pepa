package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/database"
)

// Environment represents a deployment environment.
type Environment struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Type        string    `json:"type"`
	Cluster     string    `json:"cluster,omitempty"`
	Namespace   string    `json:"namespace,omitempty"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	Color       string    `json:"color"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// EnvironmentVariable represents a key-value variable scoped to an environment.
type EnvironmentVariable struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	EnvID     uuid.UUID `json:"env_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	IsSecret  bool      `json:"is_secret"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EnvironmentRepository handles environment persistence.
type EnvironmentRepository struct {
	pool *pgxpool.Pool
}

// NewEnvironmentRepository creates a new environment repository.
func NewEnvironmentRepository(db *database.DB) *EnvironmentRepository {
	return &EnvironmentRepository{pool: db.Pool}
}

// List returns all environments for a tenant.
func (r *EnvironmentRepository) List(ctx context.Context, tenantID uuid.UUID) ([]Environment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, slug, COALESCE(type,''), COALESCE(cluster,''), COALESCE(namespace,''), COALESCE(status,'active'),
		       description, color, is_default, created_at, updated_at
		FROM environments
		WHERE tenant_id = $1
		ORDER BY is_default DESC, name ASC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	defer rows.Close()

	var envs []Environment
	for rows.Next() {
		var e Environment
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Name, &e.Slug, &e.Type, &e.Cluster, &e.Namespace, &e.Status,
			&e.Description, &e.Color, &e.IsDefault, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan environment: %w", err)
		}
		envs = append(envs, e)
	}
	return envs, nil
}

// Get returns a single environment by ID.
func (r *EnvironmentRepository) Get(ctx context.Context, tenantID, id uuid.UUID) (*Environment, error) {
	var e Environment
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, slug, COALESCE(type,''), COALESCE(cluster,''), COALESCE(namespace,''), COALESCE(status,'active'),
		       description, color, is_default, created_at, updated_at
		FROM environments
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(&e.ID, &e.TenantID, &e.Name, &e.Slug, &e.Type, &e.Cluster, &e.Namespace, &e.Status,
		&e.Description, &e.Color, &e.IsDefault, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("environment not found")
		}
		return nil, fmt.Errorf("get environment: %w", err)
	}
	return &e, nil
}

// GetBySlug returns a single environment by slug.
func (r *EnvironmentRepository) GetBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (*Environment, error) {
	var e Environment
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, slug, COALESCE(type,''), COALESCE(cluster,''), COALESCE(namespace,''), COALESCE(status,'active'),
		       description, color, is_default, created_at, updated_at
		FROM environments
		WHERE tenant_id = $1 AND slug = $2
	`, tenantID, slug).Scan(&e.ID, &e.TenantID, &e.Name, &e.Slug, &e.Type, &e.Cluster, &e.Namespace, &e.Status,
		&e.Description, &e.Color, &e.IsDefault, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("environment not found")
		}
		return nil, fmt.Errorf("get environment: %w", err)
	}
	return &e, nil
}

// Create creates a new environment.
func (r *EnvironmentRepository) Create(ctx context.Context, e *Environment) error {
	now := time.Now().UTC()
	e.CreatedAt = now
	e.UpdatedAt = now
	if e.Status == "" {
		e.Status = "active"
	}

	err := r.pool.QueryRow(ctx, `
		INSERT INTO environments (tenant_id, name, slug, type, cluster, namespace, status, description, color, is_default, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`, e.TenantID, e.Name, e.Slug, e.Type, e.Cluster, e.Namespace, e.Status, e.Description, e.Color, e.IsDefault, e.CreatedAt, e.UpdatedAt).Scan(&e.ID)
	if err != nil {
		return fmt.Errorf("create environment: %w", err)
	}
	return nil
}

// Update updates an existing environment.
func (r *EnvironmentRepository) Update(ctx context.Context, e *Environment) error {
	e.UpdatedAt = time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE environments
		SET name = $3, slug = $4, type = $5, cluster = $6, namespace = $7, status = $8,
		    description = $9, color = $10, is_default = $11, updated_at = $12
		WHERE tenant_id = $1 AND id = $2
	`, e.TenantID, e.ID, e.Name, e.Slug, e.Type, e.Cluster, e.Namespace, e.Status,
		e.Description, e.Color, e.IsDefault, e.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update environment: %w", err)
	}
	return nil
}

// Delete removes an environment.
func (r *EnvironmentRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM environments WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return fmt.Errorf("delete environment: %w", err)
	}
	return nil
}

// ── Environment Variables ─────────────────────────────────────────────────────

// EnvironmentVariableRepository handles environment variable persistence.
type EnvironmentVariableRepository struct {
	pool *pgxpool.Pool
}

// NewEnvironmentVariableRepository creates a new environment variable repository.
func NewEnvironmentVariableRepository(db *database.DB) *EnvironmentVariableRepository {
	return &EnvironmentVariableRepository{pool: db.Pool}
}

// ListByEnvID returns all variables for a given environment.
func (r *EnvironmentVariableRepository) ListByEnvID(ctx context.Context, tenantID, envID uuid.UUID) ([]EnvironmentVariable, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, env_id, key, value, is_secret, COALESCE(source,'manual'), created_at, updated_at
		FROM environment_variables
		WHERE env_id = $1 AND tenant_id = $2
		ORDER BY key ASC
	`, envID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list env variables: %w", err)
	}
	defer rows.Close()

	var vars []EnvironmentVariable
	for rows.Next() {
		var v EnvironmentVariable
		if err := rows.Scan(&v.ID, &v.TenantID, &v.EnvID, &v.Key, &v.Value, &v.IsSecret, &v.Source, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan env variable: %w", err)
		}
		vars = append(vars, v)
	}
	return vars, nil
}

// Set creates or updates a variable (upsert by env_id + key).
func (r *EnvironmentVariableRepository) Set(ctx context.Context, v *EnvironmentVariable) error {
	now := time.Now().UTC()
	v.CreatedAt = now
	v.UpdatedAt = now

	err := r.pool.QueryRow(ctx, `
		INSERT INTO environment_variables (tenant_id, env_id, key, value, is_secret, source, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (env_id, key) DO UPDATE
		SET value = EXCLUDED.value, is_secret = EXCLUDED.is_secret, source = EXCLUDED.source, updated_at = EXCLUDED.updated_at
		RETURNING id, created_at, updated_at
	`, v.TenantID, v.EnvID, v.Key, v.Value, v.IsSecret, v.Source, v.CreatedAt, v.UpdatedAt).Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return fmt.Errorf("set env variable: %w", err)
	}
	return nil
}

// DeleteByKey removes a variable by environment ID and key.
func (r *EnvironmentVariableRepository) DeleteByKey(ctx context.Context, tenantID, envID uuid.UUID, key string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM environment_variables
		WHERE env_id = $1 AND tenant_id = $2 AND key = $3
	`, envID, tenantID, key)
	if err != nil {
		return fmt.Errorf("delete env variable: %w", err)
	}
	return nil
}

// CountByEnvID returns the number of variables for an environment.
func (r *EnvironmentVariableRepository) CountByEnvID(ctx context.Context, envID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM environment_variables WHERE env_id = $1
	`, envID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count env variables: %w", err)
	}
	return count, nil
}
