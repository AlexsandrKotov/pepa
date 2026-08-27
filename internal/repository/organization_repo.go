package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Organization represents a PEPA organization.
type Organization struct {
	ID        uuid.UUID
	Name      string
	Slug      string
	Plan      string
	CreatedAt time.Time
}

// Workspace represents a tenant/workspace in the system.
type Workspace struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Name           string
	Slug           string
	Settings       []byte // JSONB field
	CreatedAt      time.Time
}

// WorkspaceStats contains resource counts for a workspace.
type WorkspaceStats struct {
	Services    int
	Connections int
	Teams       int
	Users       int
	Environments int
}

// OrganizationRepository handles database operations for organizations.
type OrganizationRepository struct {
	pool *pgxpool.Pool
}

// NewOrganizationRepository creates a new OrganizationRepository.
func NewOrganizationRepository(pool *pgxpool.Pool) *OrganizationRepository {
	return &OrganizationRepository{pool: pool}
}

// GetOrganization returns an organization by ID.
func (r *OrganizationRepository) GetOrganization(ctx context.Context, id uuid.UUID) (*Organization, error) {
	var org Organization
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, slug, COALESCE(plan, 'community'), created_at
		FROM organizations WHERE id = $1
	`, id).Scan(&org.ID, &org.Name, &org.Slug, &org.Plan, &org.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get organization: %w", err)
	}
	return &org, nil
}

// GetWorkspaceCount returns the number of workspaces for an organization.
func (r *OrganizationRepository) GetWorkspaceCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tenants WHERE organization_id = $1`, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("get workspace count: %w", err)
	}
	return count, nil
}

// UpdateOrganization updates an organization's name and slug.
func (r *OrganizationRepository) UpdateOrganization(ctx context.Context, id uuid.UUID, name, slug string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE organizations SET name = $2, slug = $3, updated_at = NOW()
		WHERE id = $1
	`, id, name, slug)
	if err != nil {
		return fmt.Errorf("update organization: %w", err)
	}
	return nil
}

// ListWorkspaces returns all workspaces for an organization.
func (r *OrganizationRepository) ListWorkspaces(ctx context.Context, orgID uuid.UUID) ([]Workspace, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, organization_id, name, slug, COALESCE(settings, '{}'), created_at
		FROM tenants
		WHERE organization_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()

	var workspaces []Workspace
	for rows.Next() {
		var ws Workspace
		if err := rows.Scan(&ws.ID, &ws.OrganizationID, &ws.Name, &ws.Slug, &ws.Settings, &ws.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		workspaces = append(workspaces, ws)
	}
	return workspaces, nil
}

// ListUserWorkspaces returns workspaces where the user has role assignments.
func (r *OrganizationRepository) ListUserWorkspaces(ctx context.Context, orgID, userID uuid.UUID) ([]Workspace, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT t.id, t.organization_id, t.name, t.slug, COALESCE(t.settings, '{}'), t.created_at
		FROM tenants t
		JOIN role_assignments ra ON ra.tenant_id = t.id
		WHERE t.organization_id = $1
		  AND ra.user_id = $2
		  AND ra.is_active = true
		ORDER BY t.created_at ASC
	`, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("list user workspaces: %w", err)
	}
	defer rows.Close()

	var workspaces []Workspace
	for rows.Next() {
		var ws Workspace
		if err := rows.Scan(&ws.ID, &ws.OrganizationID, &ws.Name, &ws.Slug, &ws.Settings, &ws.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		workspaces = append(workspaces, ws)
	}
	return workspaces, nil
}

// GetWorkspace returns a workspace by ID.
func (r *OrganizationRepository) GetWorkspace(ctx context.Context, id uuid.UUID) (*Workspace, error) {
	var ws Workspace
	err := r.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, slug, COALESCE(settings, '{}'), created_at
		FROM tenants WHERE id = $1
	`, id).Scan(&ws.ID, &ws.OrganizationID, &ws.Name, &ws.Slug, &ws.Settings, &ws.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}
	return &ws, nil
}

// GetWorkspaceStats returns resource counts for a workspace.
func (r *OrganizationRepository) GetWorkspaceStats(ctx context.Context, workspaceID uuid.UUID) (*WorkspaceStats, error) {
	stats := &WorkspaceStats{}
	
	_ = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM services WHERE tenant_id = $1`, workspaceID).Scan(&stats.Services)
	_ = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM connections WHERE tenant_id = $1`, workspaceID).Scan(&stats.Connections)
	_ = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM teams WHERE tenant_id = $1`, workspaceID).Scan(&stats.Teams)
	_ = r.pool.QueryRow(ctx, `SELECT COUNT(DISTINCT user_id) FROM role_assignments WHERE tenant_id = $1 AND is_active = true`, workspaceID).Scan(&stats.Users)
	_ = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM environments WHERE tenant_id = $1`, workspaceID).Scan(&stats.Environments)
	
	return stats, nil
}

// CreateWorkspace creates a new workspace.
func (r *OrganizationRepository) CreateWorkspace(ctx context.Context, ws *Workspace) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO tenants (id, organization_id, name, slug, settings, created_at)
		VALUES ($1, $2, $3, $4, COALESCE($5, '{}'), NOW())
	`, ws.ID, ws.OrganizationID, ws.Name, ws.Slug, ws.Settings)
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	return nil
}

// CreateWorkspaceWithEnvironments creates a new workspace with default environments in a transaction.
func (r *OrganizationRepository) CreateWorkspaceWithEnvironments(ctx context.Context, ws *Workspace) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO tenants (id, organization_id, name, slug, settings, created_at)
		VALUES ($1, $2, $3, $4, COALESCE($5, '{}'), NOW())
	`, ws.ID, ws.OrganizationID, ws.Name, ws.Slug, ws.Settings)
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	// Seed default environments for the new workspace.
	_, err = tx.Exec(ctx, `
		INSERT INTO environments (tenant_id, name, slug, description, color, is_default)
		VALUES
			($1, 'Development',  'dev',        'Local development and testing',     '#3B82F6', TRUE),
			($1, 'Staging',      'staging',    'Pre-production validation',         '#F59E0B', FALSE),
			($1, 'Production',   'production', 'Live production environment',       '#EF4444', FALSE)
	`, ws.ID)
	if err != nil {
		return fmt.Errorf("seed environments: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// UpdateWorkspace updates a workspace.
func (r *OrganizationRepository) UpdateWorkspace(ctx context.Context, id uuid.UUID, name, slug string, settings []byte) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tenants SET name = $2, slug = $3, settings = COALESCE($4, settings)
		WHERE id = $1
	`, id, name, slug, settings)
	if err != nil {
		return fmt.Errorf("update workspace: %w", err)
	}
	return nil
}

// DeleteWorkspace deletes a workspace.
func (r *OrganizationRepository) DeleteWorkspace(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}
	return nil
}

// GetServiceCount returns the number of services in a workspace.
func (r *OrganizationRepository) GetServiceCount(ctx context.Context, workspaceID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM services WHERE tenant_id = $1`, workspaceID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("get service count: %w", err)
	}
	return count, nil
}
