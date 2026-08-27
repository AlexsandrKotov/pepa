package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserCredential represents a user's external service credential.
type UserCredential struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	TenantID     uuid.UUID
	Provider     string
	ProviderURL  string
	DisplayName  string
	TokenEnc     string
	Username     string
	Email        string
	IsDefault    bool
	LastVerified *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserCredentialRepository handles database operations for user credentials.
type UserCredentialRepository struct {
	pool *pgxpool.Pool
}

// NewUserCredentialRepository creates a new UserCredentialRepository.
func NewUserCredentialRepository(pool *pgxpool.Pool) *UserCredentialRepository {
	return &UserCredentialRepository{pool: pool}
}

// ListByUser returns all credentials for a user in a tenant.
func (r *UserCredentialRepository) ListByUser(ctx context.Context, userID, tenantID uuid.UUID) ([]UserCredential, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, tenant_id, provider, COALESCE(provider_url,''),
		       COALESCE(display_name,''), token_enc, COALESCE(username,''),
		       COALESCE(email,''), is_default, last_verified, created_at, updated_at
		FROM user_credentials
		WHERE user_id = $1 AND tenant_id = $2
		ORDER BY created_at DESC
	`, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	defer rows.Close()

	var creds []UserCredential
	for rows.Next() {
		var cred UserCredential
		if err := rows.Scan(&cred.ID, &cred.UserID, &cred.TenantID, &cred.Provider,
			&cred.ProviderURL, &cred.DisplayName, &cred.TokenEnc, &cred.Username,
			&cred.Email, &cred.IsDefault, &cred.LastVerified, &cred.CreatedAt, &cred.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan credential: %w", err)
		}
		creds = append(creds, cred)
	}

	return creds, nil
}

// GetByID returns a credential by ID.
func (r *UserCredentialRepository) GetByID(ctx context.Context, id uuid.UUID) (*UserCredential, error) {
	var cred UserCredential
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, tenant_id, provider, COALESCE(provider_url,''),
		       COALESCE(display_name,''), token_enc, COALESCE(username,''),
		       COALESCE(email,''), is_default, last_verified, created_at, updated_at
		FROM user_credentials
		WHERE id = $1
	`, id).Scan(&cred.ID, &cred.UserID, &cred.TenantID, &cred.Provider,
		&cred.ProviderURL, &cred.DisplayName, &cred.TokenEnc, &cred.Username,
		&cred.Email, &cred.IsDefault, &cred.LastVerified, &cred.CreatedAt, &cred.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get credential: %w", err)
	}
	return &cred, nil
}

// GetByProvider returns a credential by user, provider, and provider URL.
func (r *UserCredentialRepository) GetByProvider(ctx context.Context, userID uuid.UUID, provider, providerURL string) (*UserCredential, error) {
	var cred UserCredential
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, tenant_id, provider, COALESCE(provider_url,''),
		       COALESCE(display_name,''), token_enc, COALESCE(username,''),
		       COALESCE(email,''), is_default, last_verified, created_at, updated_at
		FROM user_credentials
		WHERE user_id = $1 AND provider = $2 AND provider_url = $3
		ORDER BY is_default DESC, created_at DESC
		LIMIT 1
	`, userID, provider, providerURL).Scan(&cred.ID, &cred.UserID, &cred.TenantID, &cred.Provider,
		&cred.ProviderURL, &cred.DisplayName, &cred.TokenEnc, &cred.Username,
		&cred.Email, &cred.IsDefault, &cred.LastVerified, &cred.CreatedAt, &cred.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get credential by provider: %w", err)
	}
	return &cred, nil
}

// Create inserts a new credential or updates an existing one (upsert).
func (r *UserCredentialRepository) Create(ctx context.Context, cred *UserCredential) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_credentials (id, user_id, tenant_id, provider, provider_url, display_name, token_enc, username, email, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, provider, provider_url) DO UPDATE
		SET token_enc = $7, username = $8, email = $9, is_default = $10, display_name = $6, updated_at = NOW()
	`, cred.ID, cred.UserID, cred.TenantID, cred.Provider, cred.ProviderURL,
		cred.DisplayName, cred.TokenEnc, cred.Username, cred.Email, cred.IsDefault)
	if err != nil {
		return fmt.Errorf("create credential: %w", err)
	}
	return nil
}

// Update updates an existing credential.
func (r *UserCredentialRepository) Update(ctx context.Context, cred *UserCredential) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE user_credentials
		SET display_name = COALESCE($2, display_name),
		    token_enc = $3,
		    username = COALESCE($4, username),
		    email = COALESCE($5, email),
		    is_default = $6,
		    updated_at = NOW()
		WHERE id = $1
	`, cred.ID, cred.DisplayName, cred.TokenEnc, cred.Username, cred.Email, cred.IsDefault)
	if err != nil {
		return fmt.Errorf("update credential: %w", err)
	}
	return nil
}

// UpdateWithoutToken updates a credential without changing the token.
func (r *UserCredentialRepository) UpdateWithoutToken(ctx context.Context, cred *UserCredential) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE user_credentials
		SET display_name = COALESCE($2, display_name),
		    username = COALESCE($3, username),
		    email = COALESCE($4, email),
		    is_default = $5,
		    updated_at = NOW()
		WHERE id = $1
	`, cred.ID, cred.DisplayName, cred.Username, cred.Email, cred.IsDefault)
	if err != nil {
		return fmt.Errorf("update credential: %w", err)
	}
	return nil
}

// Delete removes a credential by ID.
func (r *UserCredentialRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM user_credentials WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	return nil
}

// VerifyOwnership checks if a credential belongs to a user.
func (r *UserCredentialRepository) VerifyOwnership(ctx context.Context, credID, userID uuid.UUID) (bool, error) {
	var ownerID uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT user_id FROM user_credentials WHERE id = $1`, credID).Scan(&ownerID)
	if err != nil {
		return false, fmt.Errorf("verify ownership: %w", err)
	}
	return ownerID == userID, nil
}

// VerifyOwnershipWithTenant checks if a credential belongs to a user in a specific tenant.
func (r *UserCredentialRepository) VerifyOwnershipWithTenant(ctx context.Context, credID, userID, tenantID uuid.UUID) (bool, error) {
	var ownerID uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT user_id FROM user_credentials WHERE id = $1 AND tenant_id = $2`, credID, tenantID).Scan(&ownerID)
	if err != nil {
		return false, fmt.Errorf("verify ownership: %w", err)
	}
	return ownerID == userID, nil
}

// GetProvider returns the provider for a credential.
func (r *UserCredentialRepository) GetProvider(ctx context.Context, credID uuid.UUID) (string, error) {
	var provider string
	err := r.pool.QueryRow(ctx, `SELECT provider FROM user_credentials WHERE id = $1`, credID).Scan(&provider)
	if err != nil {
		return "", fmt.Errorf("get provider: %w", err)
	}
	return provider, nil
}

// UnsetDefaults removes the default flag from all credentials of a user for a specific provider.
func (r *UserCredentialRepository) UnsetDefaults(ctx context.Context, userID uuid.UUID, provider string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE user_credentials SET is_default = false
		WHERE user_id = $1 AND provider = $2
	`, userID, provider)
	if err != nil {
		return fmt.Errorf("unset defaults: %w", err)
	}
	return nil
}

// UpdateVerification updates the last_verified timestamp and optionally user info.
func (r *UserCredentialRepository) UpdateVerification(ctx context.Context, credID uuid.UUID, username, email string) error {
	if username != "" || email != "" {
		_, err := r.pool.Exec(ctx, `
			UPDATE user_credentials
			SET last_verified = NOW(),
			    username = CASE WHEN $2 != '' THEN $2 ELSE username END,
			    email = CASE WHEN $3 != '' THEN $3 ELSE email END
			WHERE id = $1
		`, credID, username, email)
		if err != nil {
			return fmt.Errorf("update verification: %w", err)
		}
	} else {
		_, err := r.pool.Exec(ctx, `
			UPDATE user_credentials SET last_verified = NOW() WHERE id = $1
		`, credID)
		if err != nil {
			return fmt.Errorf("update verification: %w", err)
		}
	}
	return nil
}
