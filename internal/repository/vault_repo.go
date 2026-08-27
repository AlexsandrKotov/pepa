package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/database"
)

// VaultSecret represents a stored secret.
type VaultSecret struct {
	Path     string            `json:"path"`
	Data     map[string]string `json:"data"`
	Metadata VaultMetadata     `json:"metadata"`
	TenantID uuid.UUID         `json:"tenant_id,omitempty"`
}

// VaultMetadata holds secret metadata.
type VaultMetadata struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy string    `json:"created_by,omitempty"`
}

// VaultRepository handles secrets persistence.
type VaultRepository struct {
	pool *pgxpool.Pool
}

// NewVaultRepository creates a new vault repository.
func NewVaultRepository(db *database.DB) *VaultRepository {
	return &VaultRepository{pool: db.Pool}
}

// List returns secrets matching the given prefix, scoped to the tenant.
func (r *VaultRepository) List(ctx context.Context, tenantID uuid.UUID, prefix string) ([]VaultSecret, error) {
	var rows pgx.Rows
	var err error

	if prefix == "" {
		rows, err = r.pool.Query(ctx, `SELECT path, encrypted_data, version, created_at, updated_at, COALESCE(created_by,'') FROM vault_secrets WHERE tenant_id = $1 ORDER BY path ASC`, tenantID)
	} else {
		rows, err = r.pool.Query(ctx, `SELECT path, encrypted_data, version, created_at, updated_at, COALESCE(created_by,'') FROM vault_secrets WHERE tenant_id = $1 AND path LIKE $2 ORDER BY path ASC`, tenantID, prefix+"%")
	}
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	defer rows.Close()

	var secrets []VaultSecret
	for rows.Next() {
		var s VaultSecret
		var encryptedData []byte
		if err := rows.Scan(&s.Path, &encryptedData, &s.Metadata.Version, &s.Metadata.CreatedAt, &s.Metadata.UpdatedAt, &s.Metadata.CreatedBy); err != nil {
			return nil, fmt.Errorf("scan secret: %w", err)
		}
		// Decrypt data using per-path key
		decrypted, err := crypto.DecryptPath(string(encryptedData), s.Path)
		if err != nil {
			return nil, fmt.Errorf("decrypt secret %s: %w", s.Path, err)
		}
		s.Data = make(map[string]string)
		if err := json.Unmarshal([]byte(decrypted), &s.Data); err != nil {
			s.Data = map[string]string{"_raw": decrypted}
		}
		s.TenantID = tenantID
		secrets = append(secrets, s)
	}
	return secrets, nil
}

// ListPaths returns distinct paths with their children info, scoped to the tenant.
func (r *VaultRepository) ListPaths(ctx context.Context, tenantID uuid.UUID, prefix string) ([]map[string]interface{}, error) {
	rows, err := r.pool.Query(ctx, `SELECT path, COALESCE(owner_id::text, '') FROM vault_secrets WHERE tenant_id = $1 ORDER BY path ASC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list paths: %w", err)
	}
	defer rows.Close()

	allPaths := make(map[string]map[string]interface{})
	for rows.Next() {
		var path, ownerID string
		if err := rows.Scan(&path, &ownerID); err != nil {
			return nil, fmt.Errorf("scan path: %w", err)
		}
		// Build tree relative to prefix
		if prefix != "" && !strings.HasPrefix(path, prefix) {
			continue
		}
		rel := strings.TrimPrefix(path, prefix)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			continue
		}
		parts := strings.SplitN(rel, "/", 2)
		key := parts[0]
		fullKey := key
		if prefix != "" {
			fullKey = strings.TrimSuffix(prefix, "/") + "/" + key
		}
		if existing, exists := allPaths[fullKey]; !exists {
			hasChildren := len(parts) > 1
			entry := map[string]interface{}{
				"path":         fullKey,
				"type":         "kv",
				"has_children": hasChildren,
			}
			if ownerID != "" {
				entry["owner_id"] = ownerID
			}
			allPaths[fullKey] = entry
		} else {
			if len(parts) > 1 {
				existing["has_children"] = true
			}
		}
	}

	items := make([]map[string]interface{}, 0, len(allPaths))
	for _, v := range allPaths {
		items = append(items, v)
	}
	return items, nil
}

// Get returns a single secret by path, scoped to the tenant.
func (r *VaultRepository) Get(ctx context.Context, tenantID uuid.UUID, path string) (*VaultSecret, error) {
	var s VaultSecret
	var encryptedData []byte
	err := r.pool.QueryRow(ctx, `
		SELECT path, encrypted_data, version, created_at, updated_at, COALESCE(created_by,'')
		FROM vault_secrets WHERE path = $1 AND tenant_id = $2
	`, path, tenantID).Scan(&s.Path, &encryptedData, &s.Metadata.Version, &s.Metadata.CreatedAt, &s.Metadata.UpdatedAt, &s.Metadata.CreatedBy)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("secret not found: %s", path)
		}
		return nil, fmt.Errorf("get secret: %w", err)
	}

	decrypted, err := crypto.DecryptPath(string(encryptedData), s.Path)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	s.Data = make(map[string]string)
	if err := json.Unmarshal([]byte(decrypted), &s.Data); err != nil {
		s.Data = map[string]string{"_raw": decrypted}
	}
	s.TenantID = tenantID
	return &s, nil
}

// GetAnyTenant returns a secret by path without tenant scoping.
//
// SECURITY: This method bypasses tenant isolation and should ONLY be used by
// system-level operations (e.g., pipeline execution, internal service lookups)
// where the caller has already been authorized at the application level.
// Do NOT expose this method directly to user-facing API handlers without
// adding explicit tenant authorization checks.
func (r *VaultRepository) GetAnyTenant(ctx context.Context, path string) (*VaultSecret, error) {
	var s VaultSecret
	var encryptedData []byte
	err := r.pool.QueryRow(ctx, `
		SELECT path, encrypted_data, version, created_at, updated_at, COALESCE(created_by,'')
		FROM vault_secrets WHERE path = $1
	`, path).Scan(&s.Path, &encryptedData, &s.Metadata.Version, &s.Metadata.CreatedAt, &s.Metadata.UpdatedAt, &s.Metadata.CreatedBy)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("secret not found: %s", path)
		}
		return nil, fmt.Errorf("get secret: %w", err)
	}

	decrypted, err := crypto.DecryptPath(string(encryptedData), s.Path)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	s.Data = make(map[string]string)
	if err := json.Unmarshal([]byte(decrypted), &s.Data); err != nil {
		s.Data = map[string]string{"_raw": decrypted}
	}
	return &s, nil
}

// Set creates or updates a secret, scoped to the tenant.
func (r *VaultRepository) Set(ctx context.Context, tenantID uuid.UUID, path string, data map[string]string, createdBy string, ownerID *uuid.UUID) (*VaultSecret, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}

	// Encrypt with per-path key
	encrypted, err := crypto.EncryptPath(string(jsonData), path)
	if err != nil {
		return nil, fmt.Errorf("encrypt data: %w", err)
	}

	now := time.Now().UTC()
	var version int

	// Check if exists to increment version
	err = r.pool.QueryRow(ctx, `SELECT version FROM vault_secrets WHERE path = $1 AND tenant_id = $2`, path, tenantID).Scan(&version)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("check existing: %w", err)
	}

	if err == pgx.ErrNoRows {
		version = 1
		_, err = r.pool.Exec(ctx, `
			INSERT INTO vault_secrets (path, encrypted_data, version, tenant_id, owner_id, created_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, path, encrypted, version, tenantID, ownerID, createdBy, now, now)
	} else {
		version++
		_, err = r.pool.Exec(ctx, `
			UPDATE vault_secrets
			SET encrypted_data = $2, version = $3, updated_at = $4
			WHERE path = $1 AND tenant_id = $5
		`, path, encrypted, version, now, tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("set secret: %w", err)
	}

	return &VaultSecret{
		Path:     path,
		Data:     data,
		TenantID: tenantID,
		Metadata: VaultMetadata{
			Version:   version,
			CreatedAt: now,
			UpdatedAt: now,
			CreatedBy: createdBy,
		},
	}, nil
}

// Delete removes a secret, scoped to the tenant.
func (r *VaultRepository) Delete(ctx context.Context, tenantID uuid.UUID, path string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM vault_secrets WHERE path = $1 AND tenant_id = $2`, path, tenantID)
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	return nil
}

// Engines returns the available secret engine paths.
func (r *VaultRepository) Engines(ctx context.Context) ([]map[string]string, error) {
	// Built-in KV engine
	engines := []map[string]string{
		{
			"path":        "secret",
			"type":        "kv",
			"description": "Key/Value secret engine",
			"version":     "2",
		},
	}
	return engines, nil
}

// ── Key Rotation ───────────────────────────────────────────────

// EncryptedSecret holds a secret in its raw encrypted form (for rotation).
type EncryptedSecret struct {
	Path          string
	EncryptedData string
	Version       int
	TenantID      uuid.UUID
}

// GetAllEncrypted returns all secrets in their encrypted form without decrypting.
// Used for key rotation: decrypt each with old key, re-encrypt with new key.
func (r *VaultRepository) GetAllEncrypted(ctx context.Context, tenantID uuid.UUID) ([]EncryptedSecret, error) {
	rows, err := r.pool.Query(ctx, `SELECT path, encrypted_data, version, COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000') FROM vault_secrets WHERE tenant_id = $1 ORDER BY path ASC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get all encrypted: %w", err)
	}
	defer rows.Close()

	var result []EncryptedSecret
	for rows.Next() {
		var s EncryptedSecret
		if err := rows.Scan(&s.Path, &s.EncryptedData, &s.Version, &s.TenantID); err != nil {
			return nil, fmt.Errorf("scan encrypted: %w", err)
		}
		result = append(result, s)
	}
	return result, nil
}

// UpdateEncrypted replaces the encrypted data for a secret path.
// Used during key rotation to store re-encrypted data.
func (r *VaultRepository) UpdateEncrypted(ctx context.Context, path, encryptedData string) error {
	_, err := r.pool.Exec(ctx, `UPDATE vault_secrets SET encrypted_data = $1 WHERE path = $2`, encryptedData, path)
	if err != nil {
		return fmt.Errorf("update encrypted: %w", err)
	}
	return nil
}

// RotateAll re-encrypts all secrets with the current encryption key.
// It reads each secret (auto-detecting v1/v2 format), decrypts it, then
// re-encrypts with the current per-path Argon2id key.
// Returns the number of secrets rotated and any errors encountered.
func (r *VaultRepository) RotateAll(ctx context.Context, tenantID uuid.UUID) (rotated int, errors []string, err error) {
	secrets, err := r.GetAllEncrypted(ctx, tenantID)
	if err != nil {
		return 0, nil, err
	}

	for _, s := range secrets {
		// Decrypt using whatever format the data is in
		plaintext, decErr := crypto.DecryptPath(s.EncryptedData, s.Path)
		if decErr != nil {
			errors = append(errors, fmt.Sprintf("decrypt %s: %v", s.Path, decErr))
			continue
		}

		// Re-encrypt with current per-path key (always v2 format)
		newEncrypted, encErr := crypto.EncryptPath(plaintext, s.Path)
		if encErr != nil {
			errors = append(errors, fmt.Sprintf("encrypt %s: %v", s.Path, encErr))
			continue
		}

		if updateErr := r.UpdateEncrypted(ctx, s.Path, newEncrypted); updateErr != nil {
			errors = append(errors, fmt.Sprintf("update %s: %v", s.Path, updateErr))
			continue
		}
		rotated++
	}
	return rotated, errors, nil
}

// VaultStatus holds security information about the local vault.
type VaultStatus struct {
	TotalSecrets    int                    `json:"total_secrets"`
	V1Secrets       int                    `json:"v1_secrets"`
	V2Secrets       int                    `json:"v2_secrets"`
	EncryptionType  string                 `json:"encryption_type"`
	KeyDerivation   string                 `json:"key_derivation"`
	PerPathKeys     bool                   `json:"per_path_keys"`
	NeedsRotation   bool                   `json:"needs_rotation"`
	TenantIsolation bool                   `json:"tenant_isolation"`
	CreatedByTrack  bool                   `json:"created_by_tracking"`
	Argon2Params    map[string]interface{} `json:"argon2_params"`
}

// Status returns security information about the vault.
func (r *VaultRepository) Status(ctx context.Context, tenantID uuid.UUID) (*VaultStatus, error) {
	secrets, err := r.GetAllEncrypted(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	status := &VaultStatus{
		TotalSecrets:    len(secrets),
		EncryptionType:  "aes-256-gcm",
		KeyDerivation:   "argon2id",
		PerPathKeys:     true,
		TenantIsolation: true,
		CreatedByTrack:  true,
		Argon2Params:    crypto.Argon2Params(),
	}

	for _, s := range secrets {
		if crypto.IsV2Encrypted(s.EncryptedData) {
			status.V2Secrets++
		} else if crypto.IsEncrypted(s.EncryptedData) {
			status.V1Secrets++
		}
	}

	// If any secrets still use v1 format, rotation is recommended
	status.NeedsRotation = status.V1Secrets > 0

	return status, nil
}
