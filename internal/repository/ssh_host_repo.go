package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SSHHost represents an SSH host configuration.
type SSHHost struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TenantID    uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Name        string    `json:"name" db:"name"`
	Hostname    string    `json:"hostname" db:"hostname"`
	Port        int       `json:"port" db:"port"`
	Username    string    `json:"username" db:"username"`
	AuthMethod  string    `json:"auth_method" db:"auth_method"`
	SSHEncryptedKey   string `json:"-" db:"ssh_key_enc"`
	PasswordEnc string    `json:"-" db:"password_enc"`
	Tags        []string  `json:"tags" db:"tags"`
	Description string    `json:"description" db:"description"`
	CreatedBy   *uuid.UUID `json:"created_by,omitempty" db:"created_by"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// SSHHostRepository handles SSH host persistence.
type SSHHostRepository struct {
	pool *pgxpool.Pool
}

// NewSSHHostRepository creates a new SSH host repository.
func NewSSHHostRepository(pool *pgxpool.Pool) *SSHHostRepository {
	return &SSHHostRepository{pool: pool}
}

// List returns all SSH hosts for a tenant.
func (r *SSHHostRepository) List(ctx context.Context, tenantID uuid.UUID) ([]*SSHHost, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, hostname, port, username, auth_method,
		       COALESCE(ssh_key_enc,''), COALESCE(password_enc,''),
		       COALESCE(tags,'{}'::text[]), COALESCE(description,''),
		       created_by, created_at, updated_at
		FROM ssh_hosts
		WHERE tenant_id = $1
		ORDER BY name ASC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list ssh hosts: %w", err)
	}
	defer rows.Close()

	var hosts []*SSHHost
	for rows.Next() {
		h := &SSHHost{}
		if err := rows.Scan(&h.ID, &h.TenantID, &h.Name, &h.Hostname, &h.Port,
			&h.Username, &h.AuthMethod, &h.SSHEncryptedKey, &h.PasswordEnc,
			&h.Tags, &h.Description, &h.CreatedBy, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan ssh host: %w", err)
		}
		hosts = append(hosts, h)
	}
	return hosts, nil
}

// GetByID returns an SSH host by ID.
func (r *SSHHostRepository) GetByID(ctx context.Context, id uuid.UUID) (*SSHHost, error) {
	h := &SSHHost{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, hostname, port, username, auth_method,
		       COALESCE(ssh_key_enc,''), COALESCE(password_enc,''),
		       COALESCE(tags,'{}'::text[]), COALESCE(description,''),
		       created_by, created_at, updated_at
		FROM ssh_hosts
		WHERE id = $1
	`, id).Scan(&h.ID, &h.TenantID, &h.Name, &h.Hostname, &h.Port,
		&h.Username, &h.AuthMethod, &h.SSHEncryptedKey, &h.PasswordEnc,
		&h.Tags, &h.Description, &h.CreatedBy, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get ssh host: %w", err)
	}
	return h, nil
}

// Create inserts a new SSH host.
func (r *SSHHostRepository) Create(ctx context.Context, h *SSHHost) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO ssh_hosts (id, tenant_id, name, hostname, port, username, auth_method,
		                       ssh_key_enc, password_enc, tags, description, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, h.ID, h.TenantID, h.Name, h.Hostname, h.Port, h.Username, h.AuthMethod,
		h.SSHEncryptedKey, h.PasswordEnc, h.Tags, h.Description, h.CreatedBy)
	if err != nil {
		return fmt.Errorf("create ssh host: %w", err)
	}
	return nil
}

// Update modifies an existing SSH host.
func (r *SSHHostRepository) Update(ctx context.Context, h *SSHHost) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE ssh_hosts
		SET name = $1, hostname = $2, port = $3, username = $4, auth_method = $5,
		    ssh_key_enc = $6, password_enc = $7, tags = $8, description = $9,
		    updated_at = NOW()
		WHERE id = $10
	`, h.Name, h.Hostname, h.Port, h.Username, h.AuthMethod,
		h.SSHEncryptedKey, h.PasswordEnc, h.Tags, h.Description, h.ID)
	if err != nil {
		return fmt.Errorf("update ssh host: %w", err)
	}
	return nil
}

// Delete removes an SSH host by ID.
func (r *SSHHostRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM ssh_hosts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete ssh host: %w", err)
	}
	return nil
}
