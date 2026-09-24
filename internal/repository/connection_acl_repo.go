package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConnectionACL represents an access control entry for a connection.
// Modeled after VaultACL for consistency with existing patterns.
type ConnectionACL struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	ConnectionID uuid.UUID
	UserID       *uuid.UUID // nil if team grant
	TeamID       *uuid.UUID // nil if user grant
	CanRead      bool
	CanUse       bool
	CreatedBy    uuid.UUID
	CreatedAt    time.Time

	// Denormalized for display (populated by List with JOINs).
	UserName  string
	UserEmail string
	TeamName  string
}

// ConnectionACLRepository handles database operations for connection ACL.
type ConnectionACLRepository struct {
	pool *pgxpool.Pool
}

// NewConnectionACLRepository creates a new ConnectionACLRepository.
func NewConnectionACLRepository(pool *pgxpool.Pool) *ConnectionACLRepository {
	return &ConnectionACLRepository{pool: pool}
}

// List returns all ACL entries for a connection, with user/team names for display.
func (r *ConnectionACLRepository) List(ctx context.Context, tenantID, connectionID uuid.UUID) ([]ConnectionACL, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ca.id, ca.tenant_id, ca.connection_id, ca.user_id, ca.team_id,
		       ca.can_read, ca.can_use, ca.created_by, ca.created_at,
		       COALESCE(u.name, ''), COALESCE(u.email, ''),
		       COALESCE(t.name, '')
		FROM connection_acl ca
		LEFT JOIN users u ON u.id = ca.user_id
		LEFT JOIN teams t ON t.id = ca.team_id
		WHERE ca.tenant_id = $1 AND ca.connection_id = $2
		ORDER BY ca.created_at ASC
	`, tenantID, connectionID)
	if err != nil {
		return nil, fmt.Errorf("list connection ACL entries: %w", err)
	}
	defer rows.Close()

	var entries []ConnectionACL
	for rows.Next() {
		var e ConnectionACL
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.ConnectionID,
			&e.UserID, &e.TeamID,
			&e.CanRead, &e.CanUse, &e.CreatedBy, &e.CreatedAt,
			&e.UserName, &e.UserEmail, &e.TeamName,
		); err != nil {
			return nil, fmt.Errorf("scan connection ACL entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// Create adds a new ACL entry.
func (r *ConnectionACLRepository) Create(ctx context.Context, acl *ConnectionACL) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO connection_acl (id, tenant_id, connection_id, user_id, team_id, can_read, can_use, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at
	`, acl.ID, acl.TenantID, acl.ConnectionID, acl.UserID, acl.TeamID, acl.CanRead, acl.CanUse, acl.CreatedBy).Scan(&acl.CreatedAt)
	if err != nil {
		return fmt.Errorf("create connection ACL entry: %w", err)
	}
	return nil
}

// Delete removes an ACL entry, scoped to tenant and connection.
func (r *ConnectionACLRepository) Delete(ctx context.Context, tenantID, connectionID, entryID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM connection_acl
		WHERE id = $1 AND tenant_id = $2 AND connection_id = $3
	`, entryID, tenantID, connectionID)
	if err != nil {
		return fmt.Errorf("delete connection ACL entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("ACL entry not found")
	}
	return nil
}

// CheckAccess checks if a user has access to a connection via ACL.
// It checks both direct user grants and team membership grants.
// action should be "read" or "use".
func (r *ConnectionACLRepository) CheckAccess(ctx context.Context, tenantID, connectionID, userID uuid.UUID, action string) (bool, error) {
	// Determine which column to check based on action.
	permCol := "can_read"
	if action == "use" {
		permCol = "can_use"
	}

	// Check direct user grant.
	var hasAccess bool
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(bool_or(`+permCol+`), false)
		FROM connection_acl
		WHERE tenant_id = $1 AND connection_id = $2 AND user_id = $3
	`, tenantID, connectionID, userID).Scan(&hasAccess)
	if err != nil {
		return false, fmt.Errorf("check user ACL access: %w", err)
	}
	if hasAccess {
		return true, nil
	}

	// Check team grant via team_memberships.
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(bool_or(ca.`+permCol+`), false)
		FROM connection_acl ca
		JOIN team_memberships tm ON tm.team_id = ca.team_id
		WHERE ca.tenant_id = $1 AND ca.connection_id = $2 AND tm.user_id = $3
	`, tenantID, connectionID, userID).Scan(&hasAccess)
	if err != nil {
		return false, fmt.Errorf("check team ACL access: %w", err)
	}
	return hasAccess, nil
}

// ListUserAccessibleConnectionIDs returns connection IDs accessible to a user via ACL.
// This includes both direct grants and team-based grants.
func (r *ConnectionACLRepository) ListUserAccessibleConnectionIDs(ctx context.Context, tenantID, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT connection_id FROM (
			-- Direct user grants.
			SELECT connection_id FROM connection_acl
			WHERE tenant_id = $1 AND user_id = $2 AND can_read = true
			UNION
			-- Team-based grants.
			SELECT ca.connection_id FROM connection_acl ca
			JOIN team_memberships tm ON tm.team_id = ca.team_id
			WHERE ca.tenant_id = $1 AND tm.user_id = $2 AND ca.can_read = true
		) accessible
	`, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("list user accessible connections: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan connection ID: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// HasAnyACL returns true if the connection has at least one ACL entry.
func (r *ConnectionACLRepository) HasAnyACL(ctx context.Context, tenantID, connectionID uuid.UUID) (bool, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM connection_acl
		WHERE tenant_id = $1 AND connection_id = $2
	`, tenantID, connectionID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count connection ACL entries: %w", err)
	}
	return count > 0, nil
}

// CountGrantedUsers returns the number of direct user grants for a connection.
func (r *ConnectionACLRepository) CountGrantedUsers(ctx context.Context, tenantID, connectionID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM connection_acl
		WHERE tenant_id = $1 AND connection_id = $2 AND user_id IS NOT NULL
	`, tenantID, connectionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count granted users: %w", err)
	}
	return count, nil
}

// CountGrantedTeams returns the number of team grants for a connection.
func (r *ConnectionACLRepository) CountGrantedTeams(ctx context.Context, tenantID, connectionID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM connection_acl
		WHERE tenant_id = $1 AND connection_id = $2 AND team_id IS NOT NULL
	`, tenantID, connectionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count granted teams: %w", err)
	}
	return count, nil
}
