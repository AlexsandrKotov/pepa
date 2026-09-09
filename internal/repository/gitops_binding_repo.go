package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GitOpsBinding represents a mapping between a service/entity and a GitOps application.
type GitOpsBinding struct {
	ID              uuid.UUID  `json:"id"`
	TenantID        uuid.UUID  `json:"tenant_id"`
	ServiceID       *uuid.UUID `json:"service_id,omitempty"`
	EntityID        *uuid.UUID `json:"entity_id,omitempty"`
	Name            string     `json:"name"`
	RepoID          *uuid.UUID `json:"repo_id,omitempty"`
	ClusterID       *uuid.UUID `json:"cluster_id,omitempty"`
	ArgoConnectionID *uuid.UUID `json:"argo_connection_id,omitempty"`
	EngineType      string     `json:"engine_type"` // 'argocd' or 'fluxcd'
	AppName         string     `json:"app_name"`
	AppNamespace    string     `json:"app_namespace"`
	AppProject      *string    `json:"app_project,omitempty"`
	Environment     *string    `json:"environment,omitempty"`
	ManifestPath    *string    `json:"manifest_path,omitempty"`
	UpdateStrategy  string     `json:"update_strategy"` // 'kustomize_image', 'helm_values', 'appset_param', 'raw_yaml'
	UpdatePath      *string    `json:"update_path,omitempty"`
	VerifyURL       *string    `json:"verify_url,omitempty"`
	AutoBound       bool       `json:"auto_bound"`
	CreatedBy       *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// GitOpsBindingRepository handles database operations for gitops_application_bindings.
type GitOpsBindingRepository struct {
	pool *pgxpool.Pool
}

// NewGitOpsBindingRepository creates a new binding repository.
func NewGitOpsBindingRepository(pool *pgxpool.Pool) *GitOpsBindingRepository {
	return &GitOpsBindingRepository{pool: pool}
}

// Create inserts a new binding.
func (r *GitOpsBindingRepository) Create(ctx context.Context, b *GitOpsBinding) error {
	query := `
		INSERT INTO gitops_application_bindings (
			tenant_id, service_id, entity_id, name, repo_id, cluster_id,
			argo_connection_id, engine_type, app_name, app_namespace, app_project,
			environment, manifest_path, update_strategy, update_path, verify_url,
			auto_bound, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
		) RETURNING id, created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		b.TenantID, b.ServiceID, b.EntityID, b.Name, b.RepoID, b.ClusterID,
		b.ArgoConnectionID, b.EngineType, b.AppName, b.AppNamespace, b.AppProject,
		b.Environment, b.ManifestPath, b.UpdateStrategy, b.UpdatePath, b.VerifyURL,
		b.AutoBound, b.CreatedBy,
	).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
}

// Get retrieves a binding by ID and tenant.
func (r *GitOpsBindingRepository) Get(ctx context.Context, id, tenantID uuid.UUID) (*GitOpsBinding, error) {
	b := &GitOpsBinding{}
	query := `
		SELECT id, tenant_id, service_id, entity_id, name, repo_id, cluster_id,
			argo_connection_id, engine_type, app_name, app_namespace, app_project,
			environment, manifest_path, update_strategy, update_path, verify_url,
			auto_bound, created_by, created_at, updated_at
		FROM gitops_application_bindings
		WHERE id = $1 AND tenant_id = $2`

	err := r.pool.QueryRow(ctx, query, id, tenantID).Scan(
		&b.ID, &b.TenantID, &b.ServiceID, &b.EntityID, &b.Name, &b.RepoID, &b.ClusterID,
		&b.ArgoConnectionID, &b.EngineType, &b.AppName, &b.AppNamespace, &b.AppProject,
		&b.Environment, &b.ManifestPath, &b.UpdateStrategy, &b.UpdatePath, &b.VerifyURL,
		&b.AutoBound, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get binding: %w", err)
	}
	return b, nil
}

// List returns all bindings for a tenant.
func (r *GitOpsBindingRepository) List(ctx context.Context, tenantID uuid.UUID) ([]*GitOpsBinding, error) {
	query := `
		SELECT id, tenant_id, service_id, entity_id, name, repo_id, cluster_id,
			argo_connection_id, engine_type, app_name, app_namespace, app_project,
			environment, manifest_path, update_strategy, update_path, verify_url,
			auto_bound, created_by, created_at, updated_at
		FROM gitops_application_bindings
		WHERE tenant_id = $1
		ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	defer rows.Close()

	var bindings []*GitOpsBinding
	for rows.Next() {
		b := &GitOpsBinding{}
		if err := rows.Scan(
			&b.ID, &b.TenantID, &b.ServiceID, &b.EntityID, &b.Name, &b.RepoID, &b.ClusterID,
			&b.ArgoConnectionID, &b.EngineType, &b.AppName, &b.AppNamespace, &b.AppProject,
			&b.Environment, &b.ManifestPath, &b.UpdateStrategy, &b.UpdatePath, &b.VerifyURL,
			&b.AutoBound, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan binding: %w", err)
		}
		bindings = append(bindings, b)
	}
	return bindings, nil
}

// Update modifies an existing binding.
func (r *GitOpsBindingRepository) Update(ctx context.Context, b *GitOpsBinding) error {
	query := `
		UPDATE gitops_application_bindings SET
			service_id = $3, entity_id = $4, name = $5, repo_id = $6, cluster_id = $7,
			argo_connection_id = $8, engine_type = $9, app_name = $10, app_namespace = $11,
			app_project = $12, environment = $13, manifest_path = $14, update_strategy = $15,
			update_path = $16, verify_url = $17, auto_bound = $18, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2
		RETURNING updated_at`

	return r.pool.QueryRow(ctx, query,
		b.ID, b.TenantID, b.ServiceID, b.EntityID, b.Name, b.RepoID, b.ClusterID,
		b.ArgoConnectionID, b.EngineType, b.AppName, b.AppNamespace, b.AppProject,
		b.Environment, b.ManifestPath, b.UpdateStrategy, b.UpdatePath, b.VerifyURL,
		b.AutoBound,
	).Scan(&b.UpdatedAt)
}

// Delete removes a binding.
func (r *GitOpsBindingRepository) Delete(ctx context.Context, id, tenantID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		"DELETE FROM gitops_application_bindings WHERE id = $1 AND tenant_id = $2",
		id, tenantID)
	return err
}

// FindByService returns bindings for a specific service.
func (r *GitOpsBindingRepository) FindByService(ctx context.Context, serviceID, tenantID uuid.UUID) ([]*GitOpsBinding, error) {
	query := `
		SELECT id, tenant_id, service_id, entity_id, name, repo_id, cluster_id,
			argo_connection_id, engine_type, app_name, app_namespace, app_project,
			environment, manifest_path, update_strategy, update_path, verify_url,
			auto_bound, created_by, created_at, updated_at
		FROM gitops_application_bindings
		WHERE service_id = $1 AND tenant_id = $2
		ORDER BY environment NULLS LAST`

	rows, err := r.pool.Query(ctx, query, serviceID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find by service: %w", err)
	}
	defer rows.Close()

	var bindings []*GitOpsBinding
	for rows.Next() {
		b := &GitOpsBinding{}
		if err := rows.Scan(
			&b.ID, &b.TenantID, &b.ServiceID, &b.EntityID, &b.Name, &b.RepoID, &b.ClusterID,
			&b.ArgoConnectionID, &b.EngineType, &b.AppName, &b.AppNamespace, &b.AppProject,
			&b.Environment, &b.ManifestPath, &b.UpdateStrategy, &b.UpdatePath, &b.VerifyURL,
			&b.AutoBound, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan binding: %w", err)
		}
		bindings = append(bindings, b)
	}
	return bindings, nil
}

// FindByApp returns a binding by GitOps application reference.
func (r *GitOpsBindingRepository) FindByApp(ctx context.Context, connID uuid.UUID, namespace, appName, tenantID string) (*GitOpsBinding, error) {
	b := &GitOpsBinding{}
	query := `
		SELECT id, tenant_id, service_id, entity_id, name, repo_id, cluster_id,
			argo_connection_id, engine_type, app_name, app_namespace, app_project,
			environment, manifest_path, update_strategy, update_path, verify_url,
			auto_bound, created_by, created_at, updated_at
		FROM gitops_application_bindings
		WHERE argo_connection_id = $1 AND app_namespace = $2 AND app_name = $3 AND tenant_id = $4`

	err := r.pool.QueryRow(ctx, query, connID, namespace, appName, tenantID).Scan(
		&b.ID, &b.TenantID, &b.ServiceID, &b.EntityID, &b.Name, &b.RepoID, &b.ClusterID,
		&b.ArgoConnectionID, &b.EngineType, &b.AppName, &b.AppNamespace, &b.AppProject,
		&b.Environment, &b.ManifestPath, &b.UpdateStrategy, &b.UpdatePath, &b.VerifyURL,
		&b.AutoBound, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("find by app: %w", err)
	}
	return b, nil
}

// FindByDeploymentName finds a binding by app name (for deployment resolution).
func (r *GitOpsBindingRepository) FindByDeploymentName(ctx context.Context, appName, tenantID string) (*GitOpsBinding, error) {
	b := &GitOpsBinding{}
	query := `
		SELECT id, tenant_id, service_id, entity_id, name, repo_id, cluster_id,
			argo_connection_id, engine_type, app_name, app_namespace, app_project,
			environment, manifest_path, update_strategy, update_path, verify_url,
			auto_bound, created_by, created_at, updated_at
		FROM gitops_application_bindings
		WHERE app_name = $1 AND tenant_id = $2
		LIMIT 1`

	err := r.pool.QueryRow(ctx, query, appName, tenantID).Scan(
		&b.ID, &b.TenantID, &b.ServiceID, &b.EntityID, &b.Name, &b.RepoID, &b.ClusterID,
		&b.ArgoConnectionID, &b.EngineType, &b.AppName, &b.AppNamespace, &b.AppProject,
		&b.Environment, &b.ManifestPath, &b.UpdateStrategy, &b.UpdatePath, &b.VerifyURL,
		&b.AutoBound, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("find by deployment name: %w", err)
	}
	return b, nil
}
