package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SSHHostGroup represents a group for organizing SSH hosts.
type SSHHostGroup struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Name        string     `json:"name" db:"name"`
	Description string     `json:"description" db:"description"`
	Color       string     `json:"color" db:"color"`
	CreatedBy   *uuid.UUID `json:"created_by,omitempty" db:"created_by"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	HostCount   int        `json:"host_count" db:"host_count"`
}

// SSHHostGroupRepository handles SSH host group persistence.
type SSHHostGroupRepository struct {
	pool *pgxpool.Pool
}

// NewSSHHostGroupRepository creates a new SSH host group repository.
func NewSSHHostGroupRepository(pool *pgxpool.Pool) *SSHHostGroupRepository {
	return &SSHHostGroupRepository{pool: pool}
}

// List returns all SSH host groups for a tenant with host counts.
func (r *SSHHostGroupRepository) List(ctx context.Context, tenantID uuid.UUID) ([]*SSHHostGroup, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT g.id, g.tenant_id, g.name, COALESCE(g.description,''), g.color,
		       g.created_by, g.created_at, g.updated_at,
		       COUNT(m.host_id)::int AS host_count
		FROM ssh_host_groups g
		LEFT JOIN ssh_host_group_members m ON m.group_id = g.id
		WHERE g.tenant_id = $1
		GROUP BY g.id
		ORDER BY g.name ASC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list ssh host groups: %w", err)
	}
	defer rows.Close()

	var groups []*SSHHostGroup
	for rows.Next() {
		g := &SSHHostGroup{}
		if err := rows.Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.Color,
			&g.CreatedBy, &g.CreatedAt, &g.UpdatedAt, &g.HostCount); err != nil {
			return nil, fmt.Errorf("scan ssh host group: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// GetByID returns a group by ID.
func (r *SSHHostGroupRepository) GetByID(ctx context.Context, id uuid.UUID) (*SSHHostGroup, error) {
	g := &SSHHostGroup{}
	err := r.pool.QueryRow(ctx, `
		SELECT g.id, g.tenant_id, g.name, COALESCE(g.description,''), g.color,
		       g.created_by, g.created_at, g.updated_at,
		       COUNT(m.host_id)::int AS host_count
		FROM ssh_host_groups g
		LEFT JOIN ssh_host_group_members m ON m.group_id = g.id
		WHERE g.id = $1
		GROUP BY g.id
	`, id).Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.Color,
		&g.CreatedBy, &g.CreatedAt, &g.UpdatedAt, &g.HostCount)
	if err != nil {
		return nil, fmt.Errorf("get ssh host group: %w", err)
	}
	return g, nil
}

// GetByIDAndTenant returns a group by ID, scoped to a specific tenant.
func (r *SSHHostGroupRepository) GetByIDAndTenant(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*SSHHostGroup, error) {
	g := &SSHHostGroup{}
	err := r.pool.QueryRow(ctx, `
		SELECT g.id, g.tenant_id, g.name, COALESCE(g.description,''), g.color,
		       g.created_by, g.created_at, g.updated_at,
		       COUNT(m.host_id)::int AS host_count
		FROM ssh_host_groups g
		LEFT JOIN ssh_host_group_members m ON m.group_id = g.id
		WHERE g.id = $1 AND g.tenant_id = $2
		GROUP BY g.id
	`, id, tenantID).Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.Color,
		&g.CreatedBy, &g.CreatedAt, &g.UpdatedAt, &g.HostCount)
	if err != nil {
		return nil, fmt.Errorf("get ssh host group by tenant: %w", err)
	}
	return g, nil
}

// Create inserts a new group.
func (r *SSHHostGroupRepository) Create(ctx context.Context, g *SSHHostGroup) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO ssh_host_groups (id, tenant_id, name, description, color, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, g.ID, g.TenantID, g.Name, g.Description, g.Color, g.CreatedBy)
	if err != nil {
		return fmt.Errorf("create ssh host group: %w", err)
	}
	return nil
}

// Update modifies an existing group.
func (r *SSHHostGroupRepository) Update(ctx context.Context, g *SSHHostGroup) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE ssh_host_groups
		SET name = $1, description = $2, color = $3, updated_at = NOW()
		WHERE id = $4
	`, g.Name, g.Description, g.Color, g.ID)
	if err != nil {
		return fmt.Errorf("update ssh host group: %w", err)
	}
	return nil
}

// Delete removes a group by ID (members are cascade-deleted by FK).
func (r *SSHHostGroupRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM ssh_host_groups WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete ssh host group: %w", err)
	}
	return nil
}

// SetHostGroups replaces all group memberships for a host.
// Validates that all groups exist and belong to the specified tenant.
func (r *SSHHostGroupRepository) SetHostGroups(ctx context.Context, hostID uuid.UUID, groupIDs []uuid.UUID, tenantID uuid.UUID) error {
	// Validate that all groups exist and belong to the tenant
	if len(groupIDs) > 0 {
		var validCount int
		err := r.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM ssh_host_groups 
			WHERE id = ANY($1) AND tenant_id = $2
		`, groupIDs, tenantID).Scan(&validCount)
		if err != nil {
			return fmt.Errorf("validate groups: %w", err)
		}
		if validCount != len(groupIDs) {
			return fmt.Errorf("one or more groups do not exist or belong to a different tenant")
		}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Remove existing memberships
	if _, err := tx.Exec(ctx, `DELETE FROM ssh_host_group_members WHERE host_id = $1`, hostID); err != nil {
		return fmt.Errorf("clear host groups: %w", err)
	}

	// Insert new memberships
	for _, gid := range groupIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ssh_host_group_members (group_id, host_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, gid, hostID); err != nil {
			return fmt.Errorf("add host group member: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// GetHostGroupIDs returns the group IDs for a given host.
func (r *SSHHostGroupRepository) GetHostGroupIDs(ctx context.Context, hostID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT group_id FROM ssh_host_group_members WHERE host_id = $1 ORDER BY group_id
	`, hostID)
	if err != nil {
		return nil, fmt.Errorf("get host group ids: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan group id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// GetGroupHostIDs returns the host IDs in a given group.
func (r *SSHHostGroupRepository) GetGroupHostIDs(ctx context.Context, groupID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT host_id FROM ssh_host_group_members WHERE group_id = $1 ORDER BY host_id
	`, groupID)
	if err != nil {
		return nil, fmt.Errorf("get group host ids: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan host id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// GetHostGroupIDsBatch returns group IDs for multiple hosts in a single query.
// Returns a map of host_id -> []group_id.
func (r *SSHHostGroupRepository) GetHostGroupIDsBatch(ctx context.Context, hostIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	if len(hostIDs) == 0 {
		return make(map[uuid.UUID][]uuid.UUID), nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT host_id, group_id 
		FROM ssh_host_group_members 
		WHERE host_id = ANY($1)
		ORDER BY host_id, group_id
	`, hostIDs)
	if err != nil {
		return nil, fmt.Errorf("get host group ids batch: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]uuid.UUID)
	for rows.Next() {
		var hostID, groupID uuid.UUID
		if err := rows.Scan(&hostID, &groupID); err != nil {
			return nil, fmt.Errorf("scan batch group ids: %w", err)
		}
		result[hostID] = append(result[hostID], groupID)
	}
	return result, nil
}
