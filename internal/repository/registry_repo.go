package repository

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/database"
)

// RegistryRepository handles persistence for container image registries.
type RegistryRepository struct {
	pool *pgxpool.Pool
}

// NewRegistryRepository creates a new RegistryRepository.
func NewRegistryRepository(db *database.DB) *RegistryRepository {
	return &RegistryRepository{pool: db.Pool}
}

// RegistryRepo represents a configured container image registry.
type RegistryRepo struct {
	ID            uuid.UUID  `json:"id"`
	TenantID      uuid.UUID  `json:"tenant_id"`
	Name          string     `json:"name"`
	Description   string     `json:"description,omitempty"`
	RegistryType  string     `json:"registry_type"` // "docker","ghcr","harbor","ecr","gcr","acr","other"
	URL           string     `json:"url"`
	Username      string     `json:"username,omitempty"`
	Password      string     `json:"password,omitempty"`
	Token         string     `json:"token,omitempty"`
	IsDefault     bool       `json:"is_default"`
	Status        string     `json:"status"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// List returns all registry repos for a tenant.
func (r *RegistryRepository) List(ctx context.Context, tenantID uuid.UUID) ([]RegistryRepo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), registry_type, url,
		       COALESCE(username,''), COALESCE(password,''), COALESCE(token,''),
		       is_default, status, last_checked_at,
		       created_at, updated_at
		FROM registry_repositories WHERE tenant_id = $1
		ORDER BY is_default DESC, created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query registry repos: %w", err)
	}
	defer rows.Close()

	var items []RegistryRepo
	for rows.Next() {
		var h RegistryRepo
		if err := rows.Scan(&h.ID, &h.TenantID, &h.Name, &h.Description,
			&h.RegistryType, &h.URL, &h.Username, &h.Password, &h.Token,
			&h.IsDefault, &h.Status, &h.LastCheckedAt,
			&h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan registry repo: %w", err)
		}
		items = append(items, h)
	}
	return items, nil
}

// Get returns a single registry repo by ID, scoped to tenantID (zero = no filter).
func (r *RegistryRepository) Get(ctx context.Context, id, tenantID uuid.UUID) (*RegistryRepo, error) {
	var h RegistryRepo
	query := `
		SELECT id, tenant_id, name, COALESCE(description,''), registry_type, url,
		       COALESCE(username,''), COALESCE(password,''), COALESCE(token,''),
		       is_default, status, last_checked_at,
		       created_at, updated_at
		FROM registry_repositories WHERE id = $1`
	args := []interface{}{id}
	if tenantID != uuid.Nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}
	err := r.pool.QueryRow(ctx, query, args...).Scan(&h.ID, &h.TenantID, &h.Name, &h.Description,
		&h.RegistryType, &h.URL, &h.Username, &h.Password, &h.Token,
		&h.IsDefault, &h.Status, &h.LastCheckedAt,
		&h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get registry repo: %w", err)
	}
	return &h, nil
}

// GetByURL returns a registry repo by URL, scoped to tenantID (zero = no filter).
func (r *RegistryRepository) GetByURL(ctx context.Context, url string, tenantID uuid.UUID) (*RegistryRepo, error) {
	var h RegistryRepo
	query := `
		SELECT id, tenant_id, name, COALESCE(description,''), registry_type, url,
		       COALESCE(username,''), COALESCE(password,''), COALESCE(token,''),
		       is_default, status, last_checked_at,
		       created_at, updated_at
		FROM registry_repositories WHERE url = $1`
	args := []interface{}{url}
	if tenantID != uuid.Nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}
	err := r.pool.QueryRow(ctx, query, args...).Scan(&h.ID, &h.TenantID, &h.Name, &h.Description,
		&h.RegistryType, &h.URL, &h.Username, &h.Password, &h.Token,
		&h.IsDefault, &h.Status, &h.LastCheckedAt,
		&h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get registry repo by url: %w", err)
	}
	return &h, nil
}

// Create inserts a new registry repo. Sensitive fields are encrypted before storage.
func (r *RegistryRepository) Create(ctx context.Context, h *RegistryRepo) error {
	encPassword, _ := crypto.Encrypt(h.Password)
	encToken, _ := crypto.Encrypt(h.Token)

	return r.pool.QueryRow(ctx, `
		INSERT INTO registry_repositories (tenant_id, name, description, registry_type, url,
			username, password, token, is_default, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, created_at, updated_at
	`, h.TenantID, h.Name, h.Description, h.RegistryType, h.URL,
		h.Username, encPassword, encToken, h.IsDefault, h.Status,
	).Scan(&h.ID, &h.CreatedAt, &h.UpdatedAt)
}

// Update updates an existing registry repo. Sensitive fields are encrypted before storage.
func (r *RegistryRepository) Update(ctx context.Context, h *RegistryRepo) error {
	encPassword, _ := crypto.Encrypt(h.Password)
	encToken, _ := crypto.Encrypt(h.Token)

	query := `
		UPDATE registry_repositories SET
			name=$2, description=$3, registry_type=$4, url=$5,
			username=$6, password=$7, token=$8,
			is_default=$9, status=$10, last_checked_at=$11, updated_at=NOW()
		WHERE id=$1`
	args := []interface{}{h.ID, h.Name, h.Description, h.RegistryType, h.URL,
		h.Username, encPassword, encToken,
		h.IsDefault, h.Status, h.LastCheckedAt}
	if h.TenantID != uuid.Nil {
		query += " AND tenant_id = $12"
		args = append(args, h.TenantID)
	}

	_, err := r.pool.Exec(ctx, query, args...)
	return err
}

// GetDecrypted returns a single registry repo with sensitive fields decrypted,
// scoped to tenantID (zero = no filter).
func (r *RegistryRepository) GetDecrypted(ctx context.Context, id, tenantID uuid.UUID) (*RegistryRepo, error) {
	h, err := r.Get(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if h.Password != "" {
		decrypted, err := crypto.Decrypt(h.Password)
		if err != nil {
			slog.Error("failed to decrypt registry password", "registry_id", id, "error", err)
			return nil, fmt.Errorf("decrypt password: %w", err)
		}
		h.Password = decrypted
	}
	if h.Token != "" {
		decrypted, err := crypto.Decrypt(h.Token)
		if err != nil {
			slog.Error("failed to decrypt registry token", "registry_id", id, "error", err)
			return nil, fmt.Errorf("decrypt token: %w", err)
		}
		h.Token = decrypted
	}
	return h, nil
}

// Delete removes a registry repo, scoped to tenantID (zero = no filter).
func (r *RegistryRepository) Delete(ctx context.Context, id, tenantID uuid.UUID) error {
	query := `DELETE FROM registry_repositories WHERE id = $1`
	args := []interface{}{id}
	if tenantID != uuid.Nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}
	_, err := r.pool.Exec(ctx, query, args...)
	return err
}
