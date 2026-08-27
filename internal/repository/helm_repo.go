package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/database"
)

// HelmRepository handles persistence for Helm chart repositories.
type HelmRepository struct {
	pool *pgxpool.Pool
}

// NewHelmRepository creates a new HelmRepository.
func NewHelmRepository(db *database.DB) *HelmRepository {
	return &HelmRepository{pool: db.Pool}
}

// HelmRepo represents a configured Helm chart repository.
type HelmRepo struct {
	ID            uuid.UUID  `json:"id"`
	TenantID      uuid.UUID  `json:"tenant_id"`
	Name          string     `json:"name"`
	Description   string     `json:"description,omitempty"`
	RepoType      string     `json:"repo_type"` // "git", "http", "oci"
	URL           string     `json:"url"`
	Username      string     `json:"username,omitempty"`
	Password      string     `json:"password,omitempty"`
	Token         string     `json:"token,omitempty"`
	SSHKey        string     `json:"ssh_key,omitempty"`
	CACert        string     `json:"ca_cert,omitempty"`
	IsDefault     bool       `json:"is_default"`
	Status        string     `json:"status"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// List returns all Helm repos for a tenant.
func (r *HelmRepository) List(ctx context.Context, tenantID uuid.UUID) ([]HelmRepo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), repo_type, url,
		       COALESCE(username,''), COALESCE(password,''), COALESCE(token,''),
		       COALESCE(ssh_key,''), COALESCE(ca_cert,''),
		       is_default, status, last_checked_at,
		       created_at, updated_at
		FROM helm_repositories WHERE tenant_id = $1
		ORDER BY is_default DESC, created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query helm repos: %w", err)
	}
	defer rows.Close()

	var items []HelmRepo
	for rows.Next() {
		var h HelmRepo
		if err := rows.Scan(&h.ID, &h.TenantID, &h.Name, &h.Description,
			&h.RepoType, &h.URL, &h.Username, &h.Password, &h.Token,
			&h.SSHKey, &h.CACert, &h.IsDefault, &h.Status, &h.LastCheckedAt,
			&h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan helm repo: %w", err)
		}
		items = append(items, h)
	}
	return items, nil
}

// Get returns a single Helm repo by ID, scoped to tenantID (zero = no filter).
func (r *HelmRepository) Get(ctx context.Context, id, tenantID uuid.UUID) (*HelmRepo, error) {
	var h HelmRepo
	query := `
		SELECT id, tenant_id, name, COALESCE(description,''), repo_type, url,
		       COALESCE(username,''), COALESCE(password,''), COALESCE(token,''),
		       COALESCE(ssh_key,''), COALESCE(ca_cert,''),
		       is_default, status, last_checked_at,
		       created_at, updated_at
		FROM helm_repositories WHERE id = $1`
	args := []interface{}{id}
	if tenantID != uuid.Nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}
	err := r.pool.QueryRow(ctx, query, args...).Scan(&h.ID, &h.TenantID, &h.Name, &h.Description,
		&h.RepoType, &h.URL, &h.Username, &h.Password, &h.Token,
		&h.SSHKey, &h.CACert, &h.IsDefault, &h.Status, &h.LastCheckedAt,
		&h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get helm repo: %w", err)
	}
	return &h, nil
}

// GetByURL returns a Helm repo by URL (for looking up credentials during deploy),
// scoped to tenantID (zero = no filter).
func (r *HelmRepository) GetByURL(ctx context.Context, url string, tenantID uuid.UUID) (*HelmRepo, error) {
	var h HelmRepo
	query := `
		SELECT id, tenant_id, name, COALESCE(description,''), repo_type, url,
		       COALESCE(username,''), COALESCE(password,''), COALESCE(token,''),
		       COALESCE(ssh_key,''), COALESCE(ca_cert,''),
		       is_default, status, last_checked_at,
		       created_at, updated_at
		FROM helm_repositories WHERE url = $1`
	args := []interface{}{url}
	if tenantID != uuid.Nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}
	err := r.pool.QueryRow(ctx, query, args...).Scan(&h.ID, &h.TenantID, &h.Name, &h.Description,
		&h.RepoType, &h.URL, &h.Username, &h.Password, &h.Token,
		&h.SSHKey, &h.CACert, &h.IsDefault, &h.Status, &h.LastCheckedAt,
		&h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get helm repo by url: %w", err)
	}
	return &h, nil
}

// Create inserts a new Helm repo. Sensitive fields are encrypted before storage.
func (r *HelmRepository) Create(ctx context.Context, h *HelmRepo) error {
	// Encrypt sensitive fields
	encPassword, _ := crypto.Encrypt(h.Password)
	encToken, _ := crypto.Encrypt(h.Token)
	encSSHKey, _ := crypto.Encrypt(h.SSHKey)

	return r.pool.QueryRow(ctx, `
		INSERT INTO helm_repositories (tenant_id, name, description, repo_type, url,
			username, password, token, ssh_key, ca_cert, is_default, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id, created_at, updated_at
	`, h.TenantID, h.Name, h.Description, h.RepoType, h.URL,
		h.Username, encPassword, encToken, encSSHKey, h.CACert, h.IsDefault, h.Status,
	).Scan(&h.ID, &h.CreatedAt, &h.UpdatedAt)
}

// Update updates an existing Helm repo. Sensitive fields are encrypted before storage.
func (r *HelmRepository) Update(ctx context.Context, h *HelmRepo) error {
	// Encrypt sensitive fields
	encPassword, _ := crypto.Encrypt(h.Password)
	encToken, _ := crypto.Encrypt(h.Token)
	encSSHKey, _ := crypto.Encrypt(h.SSHKey)

	query := `
		UPDATE helm_repositories SET
			name=$2, description=$3, repo_type=$4, url=$5,
			username=$6, password=$7, token=$8, ssh_key=$9, ca_cert=$10,
			is_default=$11, status=$12, last_checked_at=$13, updated_at=NOW()
		WHERE id=$1`
	args := []interface{}{h.ID, h.Name, h.Description, h.RepoType, h.URL,
		h.Username, encPassword, encToken, encSSHKey, h.CACert,
		h.IsDefault, h.Status, h.LastCheckedAt}
	if h.TenantID != uuid.Nil {
		query += " AND tenant_id = $14"
		args = append(args, h.TenantID)
	}

	_, err := r.pool.Exec(ctx, query, args...)
	return err
}

// GetDecrypted returns a single Helm repo with sensitive fields decrypted,
// scoped to tenantID (zero = no filter). Use this when you need the actual
// credentials (e.g., for fetching helm index).
func (r *HelmRepository) GetDecrypted(ctx context.Context, id, tenantID uuid.UUID) (*HelmRepo, error) {
	h, err := r.Get(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	// Decrypt sensitive fields
	if h.Password != "" {
		h.Password, _ = crypto.Decrypt(h.Password)
	}
	if h.Token != "" {
		h.Token, _ = crypto.Decrypt(h.Token)
	}
	if h.SSHKey != "" {
		h.SSHKey, _ = crypto.Decrypt(h.SSHKey)
	}
	return h, nil
}

// Delete removes a Helm repo, scoped to tenantID (zero = no filter).
func (r *HelmRepository) Delete(ctx context.Context, id, tenantID uuid.UUID) error {
	query := `DELETE FROM helm_repositories WHERE id = $1`
	args := []interface{}{id}
	if tenantID != uuid.Nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}
	_, err := r.pool.Exec(ctx, query, args...)
	return err
}
