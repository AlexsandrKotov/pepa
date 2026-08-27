package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CredentialShare represents a sharing entry for a user credential.
type CredentialShare struct {
	ID             uuid.UUID
	CredentialID   uuid.UUID
	OwnerUserID    uuid.UUID
	TenantID       uuid.UUID
	SharedWithUser *uuid.UUID
	SharedWithTeam *uuid.UUID
	CreatedAt      time.Time
}

// CredentialShareWithDetails includes user/team information.
type CredentialShareWithDetails struct {
	CredentialShare
	SharedWithUserName  string
	SharedWithUserEmail string
	SharedWithTeamName  string
}

// SharedCredential represents a credential shared with a user.
type SharedCredential struct {
	ID          uuid.UUID
	Provider    string
	ProviderURL string
	DisplayName string
	TokenEnc    string
	Username    string
	Email       string
	OwnerName   string
	OwnerEmail  string
	CreatedAt   time.Time
}

// CredentialShareRepository handles database operations for credential shares.
type CredentialShareRepository struct {
	pool *pgxpool.Pool
}

// NewCredentialShareRepository creates a new CredentialShareRepository.
func NewCredentialShareRepository(pool *pgxpool.Pool) *CredentialShareRepository {
	return &CredentialShareRepository{pool: pool}
}

// Create adds a new credential share.
func (r *CredentialShareRepository) Create(ctx context.Context, share *CredentialShare) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO credential_shares (id, credential_id, owner_user_id, tenant_id, shared_with_user, shared_with_team)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING
	`, share.ID, share.CredentialID, share.OwnerUserID, share.TenantID, share.SharedWithUser, share.SharedWithTeam)
	if err != nil {
		return fmt.Errorf("create credential share: %w", err)
	}
	return nil
}

// ListByCredential returns all shares for a credential owned by a specific user.
func (r *CredentialShareRepository) ListByCredential(ctx context.Context, credID, ownerUserID uuid.UUID) ([]CredentialShareWithDetails, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT cs.id, cs.shared_with_user, cs.shared_with_team,
		       COALESCE(u.name,''), COALESCE(u.email,''),
		       COALESCE(t.name,''),
		       cs.created_at
		FROM credential_shares cs
		LEFT JOIN users u ON u.id = cs.shared_with_user
		LEFT JOIN teams t ON t.id = cs.shared_with_team
		WHERE cs.credential_id = $1 AND cs.owner_user_id = $2
		ORDER BY cs.created_at DESC
	`, credID, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("list credential shares: %w", err)
	}
	defer rows.Close()

	var shares []CredentialShareWithDetails
	for rows.Next() {
		var s CredentialShareWithDetails
		if err := rows.Scan(&s.ID, &s.SharedWithUser, &s.SharedWithTeam,
			&s.SharedWithUserName, &s.SharedWithUserEmail, &s.SharedWithTeamName, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan credential share: %w", err)
		}
		s.CredentialID = credID
		s.OwnerUserID = ownerUserID
		shares = append(shares, s)
	}

	return shares, nil
}

// Delete removes a credential share.
func (r *CredentialShareRepository) Delete(ctx context.Context, shareID, credID, ownerUserID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM credential_shares WHERE id = $1 AND credential_id = $2 AND owner_user_id = $3
	`, shareID, credID, ownerUserID)
	if err != nil {
		return fmt.Errorf("delete credential share: %w", err)
	}
	return nil
}

// ListSharedWithMe returns credentials shared with a specific user (directly or via team).
func (r *CredentialShareRepository) ListSharedWithMe(ctx context.Context, userID, tenantID uuid.UUID) ([]SharedCredential, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT uc.id, uc.provider, COALESCE(uc.provider_url,''),
		       COALESCE(uc.display_name,''), uc.token_enc,
		       COALESCE(uc.username,''), COALESCE(uc.email,''),
		       u.name, u.email, cs.created_at
		FROM credential_shares cs
		JOIN user_credentials uc ON uc.id = cs.credential_id
		JOIN users u ON u.id = cs.owner_user_id
		WHERE cs.tenant_id = $2
		  AND (
		    cs.shared_with_user = $1
		    OR cs.shared_with_team IN (
		      SELECT team_id FROM team_memberships WHERE user_id = $1
		    )
		  )
		ORDER BY cs.created_at DESC
	`, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list shared credentials: %w", err)
	}
	defer rows.Close()

	var creds []SharedCredential
	for rows.Next() {
		var sc SharedCredential
		if err := rows.Scan(&sc.ID, &sc.Provider, &sc.ProviderURL,
			&sc.DisplayName, &sc.TokenEnc, &sc.Username, &sc.Email,
			&sc.OwnerName, &sc.OwnerEmail, &sc.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan shared credential: %w", err)
		}
		creds = append(creds, sc)
	}

	return creds, nil
}

// GetSharedToken retrieves a shared credential's encrypted token.
// Returns the token for the most recently shared credential matching the criteria.
func (r *CredentialShareRepository) GetSharedToken(ctx context.Context, userID, tenantID uuid.UUID, provider, providerURL string) (tokenEnc string, username string, email string, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT uc.token_enc, COALESCE(uc.username,''), COALESCE(uc.email,'')
		FROM credential_shares cs
		JOIN user_credentials uc ON uc.id = cs.credential_id
		WHERE cs.tenant_id = $1
		  AND uc.provider = $2 AND uc.provider_url = $3
		  AND (
		    cs.shared_with_user = $4
		    OR cs.shared_with_team IN (
		      SELECT team_id FROM team_memberships WHERE user_id = $4
		    )
		  )
		ORDER BY cs.created_at DESC
		LIMIT 1
	`, tenantID, provider, providerURL, userID).Scan(&tokenEnc, &username, &email)
	if err != nil {
		return "", "", "", fmt.Errorf("get shared credential token: %w", err)
	}
	return tokenEnc, username, email, nil
}
