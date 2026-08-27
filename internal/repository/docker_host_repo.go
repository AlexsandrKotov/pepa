package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/database"
)

// DockerHostRepository handles persistence for Docker hosts and Docker services.
type DockerHostRepository struct {
	pool *pgxpool.Pool
}

// NewDockerHostRepository creates a new DockerHostRepository.
func NewDockerHostRepository(db *database.DB) *DockerHostRepository {
	return &DockerHostRepository{pool: db.Pool}
}

// DockerHost represents a registered Docker engine.
type DockerHost struct {
	ID                uuid.UUID  `json:"id"`
	TenantID          uuid.UUID  `json:"tenant_id"`
	Name              string     `json:"name"`
	Description       string     `json:"description,omitempty"`
	HostType          string     `json:"host_type"`
	HostAddress       string     `json:"host_address"`
	TLSCACert         string     `json:"tls_ca_cert,omitempty"`
	TLSCert           string     `json:"tls_cert,omitempty"`
	TLSKey            string     `json:"tls_key,omitempty"`
	SSHKey            string     `json:"ssh_key,omitempty"`
	Status            string     `json:"status"`
	DockerVersion     string     `json:"docker_version,omitempty"`
	OSArch            string     `json:"os_arch,omitempty"`
	ContainersRunning int        `json:"containers_running"`
	LastCheckedAt     *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// DockerService represents a deployed compose stack.
type DockerService struct {
	ID           uuid.UUID       `json:"id"`
	TenantID     uuid.UUID       `json:"tenant_id"`
	DockerHostID uuid.UUID       `json:"docker_host_id"`
	Name         string          `json:"name"`
	ComposeYaml  string          `json:"compose_yaml"`
	EnvVars      json.RawMessage `json:"env_vars,omitempty"`
	Status       string          `json:"status"`
	Containers   json.RawMessage `json:"containers,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// ── Docker Host CRUD ─────────────────────────────────────────

// ListHosts returns all Docker hosts for a tenant.
func (r *DockerHostRepository) ListHosts(ctx context.Context, tenantID uuid.UUID) ([]DockerHost, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), host_type, host_address,
		       COALESCE(tls_ca_cert,''), COALESCE(tls_cert,''), COALESCE(tls_key,''), COALESCE(ssh_key,''),
		       status, COALESCE(docker_version,''), COALESCE(os_arch,''),
		       COALESCE(containers_running,0), last_checked_at,
		       created_at, updated_at
		FROM docker_hosts WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query docker hosts: %w", err)
	}
	defer rows.Close()

	items := make([]DockerHost, 0)
	for rows.Next() {
		var h DockerHost
		if err := rows.Scan(&h.ID, &h.TenantID, &h.Name, &h.Description,
			&h.HostType, &h.HostAddress, &h.TLSCACert, &h.TLSCert, &h.TLSKey, &h.SSHKey,
			&h.Status, &h.DockerVersion, &h.OSArch, &h.ContainersRunning,
			&h.LastCheckedAt, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan docker host: %w", err)
		}
		items = append(items, h)
	}
	return items, nil
}

// GetHost returns a single Docker host by ID, scoped to a tenant.
func (r *DockerHostRepository) GetHost(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*DockerHost, error) {
	var h DockerHost
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), host_type, host_address,
		       COALESCE(tls_ca_cert,''), COALESCE(tls_cert,''), COALESCE(tls_key,''), COALESCE(ssh_key,''),
		       status, COALESCE(docker_version,''), COALESCE(os_arch,''),
		       COALESCE(containers_running,0), last_checked_at,
		       created_at, updated_at
		FROM docker_hosts WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(&h.ID, &h.TenantID, &h.Name, &h.Description,
		&h.HostType, &h.HostAddress, &h.TLSCACert, &h.TLSCert, &h.TLSKey, &h.SSHKey,
		&h.Status, &h.DockerVersion, &h.OSArch, &h.ContainersRunning,
		&h.LastCheckedAt, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get docker host: %w", err)
	}
	return &h, nil
}

// GetHostByName returns a Docker host by its name.
func (r *DockerHostRepository) GetHostByName(ctx context.Context, name string) (*DockerHost, error) {
	var h DockerHost
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), host_type, host_address,
		       COALESCE(tls_ca_cert,''), COALESCE(tls_cert,''), COALESCE(tls_key,''), COALESCE(ssh_key,''),
		       status, COALESCE(docker_version,''), COALESCE(os_arch,''),
		       COALESCE(containers_running,0), last_checked_at,
		       created_at, updated_at
		FROM docker_hosts WHERE name = $1
	`, name).Scan(&h.ID, &h.TenantID, &h.Name, &h.Description,
		&h.HostType, &h.HostAddress, &h.TLSCACert, &h.TLSCert, &h.TLSKey, &h.SSHKey,
		&h.Status, &h.DockerVersion, &h.OSArch, &h.ContainersRunning,
		&h.LastCheckedAt, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get docker host by name: %w", err)
	}
	return &h, nil
}

// CreateHost inserts a new Docker host. Sensitive fields are encrypted.
func (r *DockerHostRepository) CreateHost(ctx context.Context, h *DockerHost) error {
	// Encrypt sensitive fields
	encTLSCACert, _ := crypto.Encrypt(h.TLSCACert)
	encTLSCert, _ := crypto.Encrypt(h.TLSCert)
	encTLSKey, _ := crypto.Encrypt(h.TLSKey)
	encSSHKey, _ := crypto.Encrypt(h.SSHKey)

	return r.pool.QueryRow(ctx, `
		INSERT INTO docker_hosts (tenant_id, name, description, host_type, host_address,
			tls_ca_cert, tls_cert, tls_key, ssh_key, status, docker_version, os_arch, containers_running)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, created_at, updated_at
	`, h.TenantID, h.Name, h.Description, h.HostType, h.HostAddress,
		encTLSCACert, encTLSCert, encTLSKey, encSSHKey, h.Status, h.DockerVersion, h.OSArch, h.ContainersRunning,
	).Scan(&h.ID, &h.CreatedAt, &h.UpdatedAt)
}

// UpdateHost updates an existing Docker host. Sensitive fields are encrypted.
func (r *DockerHostRepository) UpdateHost(ctx context.Context, h *DockerHost) error {
	// Encrypt sensitive fields
	encTLSCACert, _ := crypto.Encrypt(h.TLSCACert)
	encTLSCert, _ := crypto.Encrypt(h.TLSCert)
	encTLSKey, _ := crypto.Encrypt(h.TLSKey)
	encSSHKey, _ := crypto.Encrypt(h.SSHKey)

	_, err := r.pool.Exec(ctx, `
		UPDATE docker_hosts SET
			name=$2, description=$3, host_type=$4, host_address=$5,
			tls_ca_cert=$6, tls_cert=$7, tls_key=$8, ssh_key=$9,
			status=$10, docker_version=$11, os_arch=$12, containers_running=$13,
			last_checked_at=$14, updated_at=NOW()
		WHERE id=$1
	`, h.ID, h.Name, h.Description, h.HostType, h.HostAddress,
		encTLSCACert, encTLSCert, encTLSKey, encSSHKey,
		h.Status, h.DockerVersion, h.OSArch, h.ContainersRunning, h.LastCheckedAt)
	return err
}

// GetHostDecrypted returns a Docker host with sensitive fields decrypted.
func (r *DockerHostRepository) GetHostDecrypted(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*DockerHost, error) {
	h, err := r.GetHost(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	// Decrypt sensitive fields
	if h.TLSCACert != "" {
		h.TLSCACert, _ = crypto.Decrypt(h.TLSCACert)
	}
	if h.TLSCert != "" {
		h.TLSCert, _ = crypto.Decrypt(h.TLSCert)
	}
	if h.TLSKey != "" {
		h.TLSKey, _ = crypto.Decrypt(h.TLSKey)
	}
	if h.SSHKey != "" {
		h.SSHKey, _ = crypto.Decrypt(h.SSHKey)
	}
	return h, nil
}

// DeleteHost removes a Docker host.
func (r *DockerHostRepository) DeleteHost(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM docker_hosts WHERE id = $1`, id)
	return err
}

// ── Docker Service CRUD ──────────────────────────────────────

// ListServices returns all Docker services for a tenant.
func (r *DockerHostRepository) ListServices(ctx context.Context, tenantID uuid.UUID) ([]DockerService, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, docker_host_id, name, compose_yaml,
		       COALESCE(env_vars,'{}'::jsonb), status,
		       COALESCE(containers,'[]'::jsonb),
		       created_at, updated_at
		FROM docker_services WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query docker services: %w", err)
	}
	defer rows.Close()

	items := make([]DockerService, 0)
	for rows.Next() {
		var s DockerService
		if err := rows.Scan(&s.ID, &s.TenantID, &s.DockerHostID, &s.Name,
			&s.ComposeYaml, &s.EnvVars, &s.Status, &s.Containers,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan docker service: %w", err)
		}
		items = append(items, s)
	}
	return items, nil
}

// GetService returns a single Docker service by ID, scoped to a tenant.
func (r *DockerHostRepository) GetService(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*DockerService, error) {
	var s DockerService
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, docker_host_id, name, compose_yaml,
		       COALESCE(env_vars,'{}'::jsonb), status,
		       COALESCE(containers,'[]'::jsonb),
		       created_at, updated_at
		FROM docker_services WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(&s.ID, &s.TenantID, &s.DockerHostID, &s.Name,
		&s.ComposeYaml, &s.EnvVars, &s.Status, &s.Containers,
		&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get docker service: %w", err)
	}
	return &s, nil
}

// CreateService inserts a new Docker service.
func (r *DockerHostRepository) CreateService(ctx context.Context, s *DockerService) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO docker_services (tenant_id, docker_host_id, name, compose_yaml, env_vars, status, containers)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, created_at, updated_at
	`, s.TenantID, s.DockerHostID, s.Name, s.ComposeYaml, s.EnvVars, s.Status, s.Containers,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

// UpdateService updates an existing Docker service.
func (r *DockerHostRepository) UpdateService(ctx context.Context, s *DockerService) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE docker_services SET
			status=$2, containers=$3, updated_at=NOW()
		WHERE id=$1
	`, s.ID, s.Status, s.Containers)
	return err
}

// DeleteService removes a Docker service.
func (r *DockerHostRepository) DeleteService(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM docker_services WHERE id = $1`, id)
	return err
}
