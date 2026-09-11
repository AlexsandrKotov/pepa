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
	"github.com/pepa/pepa/pkg/utils"
)

// ConnectionType defines supported connection types.
type ConnectionType string

const (
	ConnectionGitLab       ConnectionType = "gitlab"
	ConnectionGit          ConnectionType = "git"
	ConnectionJira         ConnectionType = "jira"
	ConnectionCI           ConnectionType = "ci"
	ConnectionAI           ConnectionType = "ai"
	ConnectionStorage      ConnectionType = "storage"
	ConnectionProxmox      ConnectionType = "proxmox"
	ConnectionVMware       ConnectionType = "vmware"
	ConnectionDocker       ConnectionType = "docker"
	ConnectionSecret       ConnectionType = "secret"
	ConnectionNotification ConnectionType = "notification"
	ConnectionSonarQube    ConnectionType = "sonarqube"
	ConnectionKubernetes   ConnectionType = "kubernetes"
	ConnectionArgoCD       ConnectionType = "argocd"   // legacy — use ConnectionKubernetes for new connections
	ConnectionFluxCD       ConnectionType = "fluxcd"   // legacy — use ConnectionKubernetes for new connections
)

// Connection represents an external service connection.
type Connection struct {
	ID               uuid.UUID         `json:"id"`
	TenantID         uuid.UUID         `json:"tenant_id"`
	OwnerID          *uuid.UUID        `json:"owner_id,omitempty"`
	Type             ConnectionType    `json:"type"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Config           map[string]any    `json:"config"`
	Status           string            `json:"status"`
	LastCheckAt      *time.Time        `json:"last_check_at,omitempty"`
	Labels           map[string]string `json:"labels"`
	Notes            string            `json:"notes"`
	FallbackToAdmin  bool              `json:"fallback_to_admin"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// ConnectionRepository handles connection persistence.
type ConnectionRepository struct {
	pool *pgxpool.Pool
}

// NewConnectionRepository creates a new connection repository.
func NewConnectionRepository(db *database.DB) *ConnectionRepository {
	return &ConnectionRepository{pool: db.Pool}
}

// List returns all connections for a tenant, optionally filtered by type.
func (r *ConnectionRepository) List(ctx context.Context, tenantID uuid.UUID, connType string) ([]Connection, error) {
	var rows pgx.Rows
	var err error

	if connType != "" {
		rows, err = r.pool.Query(ctx, `
			SELECT id, tenant_id, COALESCE(owner_id, '00000000-0000-0000-0000-000000000000'),
			       type, name, COALESCE(description,''),
			       COALESCE(config,'{}'::jsonb), status, last_check_at,
			       COALESCE(labels,'{}'::jsonb), COALESCE(notes,''),
			       COALESCE(fallback_to_admin, true),
			       created_at, updated_at
			FROM connections WHERE tenant_id = $1 AND type = $2
			ORDER BY created_at DESC
		`, tenantID, connType)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT id, tenant_id, COALESCE(owner_id, '00000000-0000-0000-0000-000000000000'),
			       type, name, COALESCE(description,''),
			       COALESCE(config,'{}'::jsonb), status, last_check_at,
			       COALESCE(labels,'{}'::jsonb), COALESCE(notes,''),
			       COALESCE(fallback_to_admin, true),
			       created_at, updated_at
			FROM connections WHERE tenant_id = $1
			ORDER BY created_at DESC
		`, tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("query connections: %w", err)
	}
	defer rows.Close()

	items := make([]Connection, 0)
	for rows.Next() {
		var c Connection
		var configJSON, labelsJSON []byte
		var ownerIDRaw string
		if err := rows.Scan(&c.ID, &c.TenantID, &ownerIDRaw,
			&c.Type, &c.Name, &c.Description,
			&configJSON, &c.Status, &c.LastCheckAt,
			&labelsJSON, &c.Notes, &c.FallbackToAdmin, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan connection: %w", err)
		}
		if parsed, err := uuid.Parse(ownerIDRaw); err == nil && parsed != uuid.Nil {
			c.OwnerID = &parsed
		}
		_ = json.Unmarshal(configJSON, &c.Config)
		_ = json.Unmarshal(labelsJSON, &c.Labels)
		if c.Config == nil {
			c.Config = map[string]any{}
		}
		if c.Labels == nil {
			c.Labels = map[string]string{}
		}
		items = append(items, c)
	}
	return items, nil
}

// ConnectionFilter defines filtering and pagination parameters for connection lists.
type ConnectionFilter struct {
	TenantID uuid.UUID
	Page     int
	PerPage  int
	Search   string
	Type     string
	Status   string
}

// ConnectionListResponse is the paginated result of ListFiltered.
type ConnectionListResponse struct {
	Items      []Connection `json:"items"`
	Total      int64        `json:"total"`
	Page       int          `json:"page"`
	PerPage    int          `json:"per_page"`
	TotalPages int          `json:"total_pages"`
}

// ListFiltered returns connections with filtering, sorting, and pagination.
func (r *ConnectionRepository) ListFiltered(ctx context.Context, f ConnectionFilter) (*ConnectionListResponse, error) {
	query := `
		SELECT id, tenant_id, COALESCE(owner_id, '00000000-0000-0000-0000-000000000000'),
		       type, name, COALESCE(description,''),
		       COALESCE(config,'{}'::jsonb), status, last_check_at,
		       COALESCE(labels,'{}'::jsonb), COALESCE(notes,''),
		       COALESCE(fallback_to_admin, true),
		       created_at, updated_at
		FROM connections WHERE tenant_id = $1`
	args := []interface{}{f.TenantID}
	argIdx := 2

	if f.Type != "" {
		query += fmt.Sprintf(" AND type = $%d", argIdx)
		args = append(args, f.Type)
		argIdx++
	}
	if f.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, f.Status)
		argIdx++
	}
	if f.Search != "" {
		query += fmt.Sprintf(" AND (name ILIKE $%d OR description ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+f.Search+"%")
		argIdx++
	}

	// Count
	countQuery := "SELECT COUNT(*) FROM (" + query + ") sub"
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count connections: %w", err)
	}

	f.Page, f.PerPage = ClampPagination(f.Page, f.PerPage, 20)
	offset := (f.Page - 1) * f.PerPage

	query += " ORDER BY created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, f.PerPage, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query connections: %w", err)
	}
	defer rows.Close()

	items := make([]Connection, 0)
	for rows.Next() {
		var c Connection
		var configJSON, labelsJSON []byte
		var ownerIDRaw string
		if err := rows.Scan(&c.ID, &c.TenantID, &ownerIDRaw,
			&c.Type, &c.Name, &c.Description,
			&configJSON, &c.Status, &c.LastCheckAt,
			&labelsJSON, &c.Notes, &c.FallbackToAdmin, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan connection: %w", err)
		}
		if parsed, err := uuid.Parse(ownerIDRaw); err == nil && parsed != uuid.Nil {
			c.OwnerID = &parsed
		}
		_ = json.Unmarshal(configJSON, &c.Config)
		_ = json.Unmarshal(labelsJSON, &c.Labels)
		if c.Config == nil {
			c.Config = map[string]any{}
		}
		if c.Labels == nil {
			c.Labels = map[string]string{}
		}
		items = append(items, c)
	}

	totalPages := int(total) / f.PerPage
	if int(total)%f.PerPage > 0 {
		totalPages++
	}

	return &ConnectionListResponse{
		Items:      items,
		Total:      total,
		Page:       f.Page,
		PerPage:    f.PerPage,
		TotalPages: totalPages,
	}, nil
}

// Get returns a connection by ID, scoped to a tenant.
func (r *ConnectionRepository) Get(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*Connection, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, COALESCE(owner_id, '00000000-0000-0000-0000-000000000000'),
		       type, name, COALESCE(description,''),
		       COALESCE(config,'{}'::jsonb), status, last_check_at,
		       COALESCE(labels,'{}'::jsonb), COALESCE(notes,''),
		       COALESCE(fallback_to_admin, true),
		       created_at, updated_at
		FROM connections WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)

	var c Connection
	var configJSON, labelsJSON []byte
	var ownerIDRaw string
	if err := row.Scan(&c.ID, &c.TenantID, &ownerIDRaw,
		&c.Type, &c.Name, &c.Description,
		&configJSON, &c.Status, &c.LastCheckAt,
		&labelsJSON, &c.Notes, &c.FallbackToAdmin, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("connection not found: %s", id)
		}
		return nil, fmt.Errorf("get connection: %w", err)
	}
	if parsed, err := uuid.Parse(ownerIDRaw); err == nil && parsed != uuid.Nil {
		c.OwnerID = &parsed
	}
	_ = json.Unmarshal(configJSON, &c.Config)
	_ = json.Unmarshal(labelsJSON, &c.Labels)
	if c.Config == nil {
		c.Config = map[string]any{}
	}
	if c.Labels == nil {
		c.Labels = map[string]string{}
	}
	return &c, nil
}

// GetByClusterID returns the kubernetes connection linked to a cluster (via labels.cluster_id),
// or nil if none exists.
func (r *ConnectionRepository) GetByClusterID(ctx context.Context, clusterID uuid.UUID) (*Connection, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, COALESCE(owner_id, '00000000-0000-0000-0000-000000000000'),
		       type, name, COALESCE(description,''),
		       COALESCE(config,'{}'::jsonb), status, last_check_at,
		       COALESCE(labels,'{}'::jsonb), COALESCE(notes,''),
		       COALESCE(fallback_to_admin, true),
		       created_at, updated_at
		FROM connections
		WHERE labels->>'cluster_id' = $1
		LIMIT 1
	`, clusterID.String())

	var c Connection
	var configJSON, labelsJSON []byte
	var ownerIDRaw string
	if err := row.Scan(&c.ID, &c.TenantID, &ownerIDRaw,
		&c.Type, &c.Name, &c.Description,
		&configJSON, &c.Status, &c.LastCheckAt,
		&labelsJSON, &c.Notes, &c.FallbackToAdmin, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get connection by cluster_id: %w", err)
	}
	if parsed, err := uuid.Parse(ownerIDRaw); err == nil && parsed != uuid.Nil {
		c.OwnerID = &parsed
	}
	_ = json.Unmarshal(configJSON, &c.Config)
	_ = json.Unmarshal(labelsJSON, &c.Labels)
	if c.Config == nil {
		c.Config = map[string]any{}
	}
	if c.Labels == nil {
		c.Labels = map[string]string{}
	}
	return &c, nil
}

// Create inserts a new connection. Sensitive config fields are encrypted.
func (r *ConnectionRepository) Create(ctx context.Context, c *Connection) error {
	c.ID = uuid.New()
	now := time.Now().UTC()
	c.CreatedAt = now
	c.UpdatedAt = now

	// Encrypt sensitive config values
	encConfig, err := encryptConfig(c.Config)
	if err != nil {
		return fmt.Errorf("encrypt connection config: %w", err)
	}
	configJSON, _ := json.Marshal(encConfig)
	labelsJSON, _ := json.Marshal(c.Labels)

	_, err = r.pool.Exec(ctx, `
		INSERT INTO connections (id, tenant_id, owner_id, type, name, description, config, status, labels, notes, fallback_to_admin, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`, c.ID, c.TenantID, c.OwnerID, c.Type, c.Name, c.Description, configJSON, c.Status, labelsJSON, c.Notes, c.FallbackToAdmin, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create connection: %w", err)
	}
	return nil
}

// Update modifies an existing connection. Sensitive config fields are encrypted.
func (r *ConnectionRepository) Update(ctx context.Context, c *Connection) error {
	c.UpdatedAt = time.Now().UTC()
	// Encrypt sensitive config values
	encConfig, err := encryptConfig(c.Config)
	if err != nil {
		return fmt.Errorf("encrypt connection config: %w", err)
	}
	configJSON, _ := json.Marshal(encConfig)
	labelsJSON, _ := json.Marshal(c.Labels)

	_, err = r.pool.Exec(ctx, `
		UPDATE connections SET name=$2, description=$3, config=$4, status=$5,
		       labels=$6, notes=$7, last_check_at=$8, updated_at=$9,
		       fallback_to_admin=$10
		WHERE id=$1
	`, c.ID, c.Name, c.Description, configJSON, c.Status, labelsJSON, c.Notes, c.LastCheckAt, c.UpdatedAt, c.FallbackToAdmin)
	if err != nil {
		return fmt.Errorf("update connection: %w", err)
	}
	return nil
}

// GetDecrypted returns a connection with sensitive config values decrypted.
func (r *ConnectionRepository) GetDecrypted(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*Connection, error) {
	c, err := r.Get(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	c.Config, err = decryptConfig(c.Config)
	if err != nil {
		return nil, fmt.Errorf("decrypt connection config: %w", err)
	}
	return c, nil
}

// encryptConfig encrypts sensitive keys in a config map.
// Returns an error if encryption fails for any sensitive key.
func encryptConfig(config map[string]any) (map[string]any, error) {
	if config == nil {
		return config, nil
	}
	result := make(map[string]any)
	for k, v := range config {
		strVal, ok := v.(string)
		if ok && isSensitiveKey(k) && strVal != "" {
			encrypted, err := crypto.Encrypt(strVal)
			if err != nil {
				return nil, fmt.Errorf("encrypt config key %q: %w", k, err)
			}
			result[k] = encrypted
		} else {
			result[k] = v
		}
	}
	return result, nil
}

// decryptConfig decrypts sensitive keys in a config map.
// Returns an error if any encrypted value cannot be decrypted.
func decryptConfig(config map[string]any) (map[string]any, error) {
	if config == nil {
		return config, nil
	}
	result := make(map[string]any)
	var decryptErrors []string
	for k, v := range config {
		strVal, ok := v.(string)
		if ok && crypto.IsEncrypted(strVal) {
			if decrypted, err := crypto.Decrypt(strVal); err == nil {
				result[k] = decrypted
			} else {
				result[k] = v // fallback to encrypted on error
				decryptErrors = append(decryptErrors, k)
			}
		} else {
			result[k] = v
		}
	}
	if len(decryptErrors) > 0 {
		return result, fmt.Errorf("failed to decrypt fields: %s (encryption key may have changed)", strings.Join(decryptErrors, ", "))
	}
	return result, nil
}

func isSensitiveKey(key string) bool {
	return utils.IsSensitiveKey(key)
}

// Delete removes a connection.
func (r *ConnectionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM connections WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete connection: %w", err)
	}
	return nil
}

// UpdateStatus updates the status and last_check_at of a connection.
func (r *ConnectionRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE connections SET status=$2, last_check_at=NOW(), updated_at=NOW()
		WHERE id=$1
	`, id, status)
	if err != nil {
		return fmt.Errorf("update connection status: %w", err)
	}
	return nil
}

// CountByType returns the count of connections per type for a tenant.
func (r *ConnectionRepository) CountByType(ctx context.Context, tenantID uuid.UUID) (map[string]int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT type, COUNT(*) FROM connections WHERE tenant_id = $1 GROUP BY type
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("count connections: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var t string
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			return nil, fmt.Errorf("scan count: %w", err)
		}
		counts[t] = n
	}
	return counts, nil
}

// FindByType returns all connections of a given type across all tenants.
// Used by the plugin system to resolve connection credentials for plugins.
func (r *ConnectionRepository) FindByType(ctx context.Context, connType string) ([]Connection, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, COALESCE(owner_id, '00000000-0000-0000-0000-000000000000'),
		       type, name, COALESCE(description,''),
		       COALESCE(config,'{}'::jsonb), status, last_check_at,
		       COALESCE(labels,'{}'::jsonb), COALESCE(notes,''),
		       COALESCE(fallback_to_admin, true),
		       created_at, updated_at
		FROM connections WHERE type = $1 AND status = 'connected'
		ORDER BY updated_at DESC
	`, connType)
	if err != nil {
		return nil, fmt.Errorf("find connections by type: %w", err)
	}
	defer rows.Close()

	items := make([]Connection, 0)
	for rows.Next() {
		var c Connection
		var configJSON, labelsJSON []byte
		var ownerIDRaw string
		if err := rows.Scan(&c.ID, &c.TenantID, &ownerIDRaw,
			&c.Type, &c.Name, &c.Description,
			&configJSON, &c.Status, &c.LastCheckAt,
			&labelsJSON, &c.Notes, &c.FallbackToAdmin, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan connection: %w", err)
		}
		if parsed, err := uuid.Parse(ownerIDRaw); err == nil && parsed != uuid.Nil {
			c.OwnerID = &parsed
		}
		_ = json.Unmarshal(configJSON, &c.Config)
		_ = json.Unmarshal(labelsJSON, &c.Labels)
		if c.Config == nil {
			c.Config = map[string]any{}
		}
		if c.Labels == nil {
			c.Labels = map[string]string{}
		}
		items = append(items, c)
	}
	return items, nil
}

// FindByTypeDecrypted returns connections of a given type with decrypted config.
func (r *ConnectionRepository) FindByTypeDecrypted(ctx context.Context, connType string) ([]Connection, error) {
	conns, err := r.FindByType(ctx, connType)
	if err != nil {
		return nil, err
	}
	for i := range conns {
		conns[i].Config, err = decryptConfig(conns[i].Config)
		if err != nil {
			return nil, fmt.Errorf("decrypt connection %s config: %w", conns[i].Name, err)
		}
	}
	return conns, nil
}
