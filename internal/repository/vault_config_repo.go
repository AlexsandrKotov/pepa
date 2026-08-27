package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// VaultACL represents a vault access control entry.
type VaultACL struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	PathPrefix string
	UserID    *uuid.UUID
	TeamID    *uuid.UUID
	CanRead   bool
	CanCreate bool
	CanDelete bool
}

// VaultConfigRepository handles database operations for vault configuration and ACL.
type VaultConfigRepository struct {
	pool *pgxpool.Pool
}

// NewVaultConfigRepository creates a new VaultConfigRepository.
func NewVaultConfigRepository(pool *pgxpool.Pool) *VaultConfigRepository {
	return &VaultConfigRepository{pool: pool}
}

// CountSecretOwner counts secrets owned by a user at specified paths.
func (r *VaultConfigRepository) CountSecretOwner(ctx context.Context, tenantID, userID uuid.UUID, paths []string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM vault_secrets
		WHERE tenant_id = $1 AND owner_id = $2 AND path = ANY($3)
	`, tenantID, userID, paths).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count secret owner: %w", err)
	}
	return count, nil
}

// CountACLEntries counts ACL entries for specified path prefixes.
func (r *VaultConfigRepository) CountACLEntries(ctx context.Context, tenantID uuid.UUID, prefixes []string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM vault_acl WHERE tenant_id = $1 AND path_prefix = ANY($2)
	`, tenantID, prefixes).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count ACL entries: %w", err)
	}
	return count, nil
}

// ListACLEntries returns all ACL entries for a tenant.
func (r *VaultConfigRepository) ListACLEntries(ctx context.Context, tenantID uuid.UUID) ([]VaultACL, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, path_prefix, user_id, team_id, can_read, can_create, can_delete
		FROM vault_acl
		WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list ACL entries: %w", err)
	}
	defer rows.Close()

	var acls []VaultACL
	for rows.Next() {
		var acl VaultACL
		if err := rows.Scan(&acl.ID, &acl.TenantID, &acl.PathPrefix, &acl.UserID, &acl.TeamID, &acl.CanRead, &acl.CanCreate, &acl.CanDelete); err != nil {
			return nil, fmt.Errorf("scan ACL entry: %w", err)
		}
		acls = append(acls, acl)
	}
	return acls, nil
}

// ListACLEntriesForPaths returns ACL entries matching specified path prefixes.
func (r *VaultConfigRepository) ListACLEntriesForPaths(ctx context.Context, tenantID uuid.UUID, prefixes []string) ([]VaultACL, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, path_prefix, user_id, team_id, can_read, can_create, can_delete
		FROM vault_acl
		WHERE tenant_id = $1 AND path_prefix = ANY($2)
	`, tenantID, prefixes)
	if err != nil {
		return nil, fmt.Errorf("list ACL entries for paths: %w", err)
	}
	defer rows.Close()

	var acls []VaultACL
	for rows.Next() {
		var acl VaultACL
		if err := rows.Scan(&acl.ID, &acl.TenantID, &acl.PathPrefix, &acl.UserID, &acl.TeamID, &acl.CanRead, &acl.CanCreate, &acl.CanDelete); err != nil {
			return nil, fmt.Errorf("scan ACL entry: %w", err)
		}
		acls = append(acls, acl)
	}
	return acls, nil
}

// ListUserTeamMemberships returns team IDs for a user.
func (r *VaultConfigRepository) ListUserTeamMemberships(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT team_id FROM team_memberships WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list team memberships: %w", err)
	}
	defer rows.Close()

	var teamIDs []uuid.UUID
	for rows.Next() {
		var teamID uuid.UUID
		if err := rows.Scan(&teamID); err != nil {
			return nil, fmt.Errorf("scan team ID: %w", err)
		}
		teamIDs = append(teamIDs, teamID)
	}
	return teamIDs, nil
}

// CheckTeamACLAccess checks if a user has access via team membership.
func (r *VaultConfigRepository) CheckTeamACLAccess(ctx context.Context, tenantID, userID uuid.UUID, prefixes []string, action string) (bool, error) {
	query := `
		SELECT va.can_read, va.can_create, va.can_delete
		FROM vault_acl va
		JOIN team_memberships tm ON tm.team_id = va.team_id
		WHERE va.tenant_id = $1
		  AND va.path_prefix = ANY($2)
		  AND tm.user_id = $3
	`
	rows, err := r.pool.Query(ctx, query, tenantID, prefixes, userID)
	if err != nil {
		return false, fmt.Errorf("check team ACL access: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var canRead, canCreate, canDelete bool
		if err := rows.Scan(&canRead, &canCreate, &canDelete); err != nil {
			continue
		}
		if action == "read" && canRead {
			return true, nil
		}
		if action == "create" && canCreate {
			return true, nil
		}
		if action == "delete" && canDelete {
			return true, nil
		}
	}
	return false, nil
}
