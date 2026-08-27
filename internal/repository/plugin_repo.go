package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/database"
)

// pluginSensitiveKeys are keys in plugin config that should be encrypted.
var pluginSensitiveKeys = []string{"token", "password", "api_key", "api_token", "secret", "access_token", "private_token"}

// PluginRepository handles plugin persistence.
type PluginRepository struct {
	pool *pgxpool.Pool
}

// NewPluginRepository creates a new plugin repository.
func NewPluginRepository(db *database.DB) *PluginRepository {
	return &PluginRepository{pool: db.Pool}
}

// Plugin represents a registered plugin.
type Plugin struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Type        string          `json:"type"`
	Status      string          `json:"status"`
	Config      json.RawMessage `json:"config,omitempty"`
	Enabled     bool            `json:"enabled"`
	TenantID    *uuid.UUID      `json:"tenant_id,omitempty"`
	InstalledAt time.Time       `json:"installed_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// List returns all registered plugins.
func (r *PluginRepository) List(ctx context.Context) ([]Plugin, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, version, type, status, config, enabled, tenant_id, installed_at, updated_at
		FROM plugins
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query plugins: %w", err)
	}
	defer rows.Close()

	var plugins []Plugin
	for rows.Next() {
		p, err := scanPlugin(rows)
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, *p)
	}

	return plugins, nil
}

// Get returns a plugin by ID.
func (r *PluginRepository) Get(ctx context.Context, id uuid.UUID) (*Plugin, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, name, version, type, status, config, enabled, tenant_id, installed_at, updated_at
		FROM plugins WHERE id = $1
	`, id)

	var p Plugin
	var configJSON []byte
	var tenantID sql.NullString

	if err := row.Scan(&p.ID, &p.Name, &p.Version, &p.Type, &p.Status,
		&configJSON, &p.Enabled, &tenantID, &p.InstalledAt, &p.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("plugin not found: %s", id)
		}
		return nil, fmt.Errorf("get plugin: %w", err)
	}

	if configJSON != nil {
		p.Config = configJSON
	}
	if tenantID.Valid {
		tid, _ := uuid.Parse(tenantID.String)
		p.TenantID = &tid
	}

	return &p, nil
}

// GetByName returns a plugin by name.
func (r *PluginRepository) GetByName(ctx context.Context, name string) (*Plugin, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, name, version, type, status, config, enabled, tenant_id, installed_at, updated_at
		FROM plugins WHERE name = $1
	`, name)

	var p Plugin
	var configJSON []byte
	var tenantID sql.NullString

	if err := row.Scan(&p.ID, &p.Name, &p.Version, &p.Type, &p.Status,
		&configJSON, &p.Enabled, &tenantID, &p.InstalledAt, &p.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("plugin not found: %s", name)
		}
		return nil, fmt.Errorf("get plugin by name: %w", err)
	}

	if configJSON != nil {
		p.Config = configJSON
	}
	if tenantID.Valid {
		tid, _ := uuid.Parse(tenantID.String)
		p.TenantID = &tid
	}

	return &p, nil
}

// Register inserts or updates a plugin. Sensitive config values are encrypted.
func (r *PluginRepository) Register(ctx context.Context, p *Plugin) error {
	now := time.Now().UTC()

	// Encrypt sensitive config values
	encConfig := encryptPluginConfig(p.Config)

	// Check if plugin already exists
	existing, _ := r.GetByName(ctx, p.Name)
	if existing != nil {
		p.ID = existing.ID
		_, err := r.pool.Exec(ctx, `
			UPDATE plugins SET version = $1, type = $2, status = $3,
			                   config = $4, enabled = $5, updated_at = $6
			WHERE id = $7
		`, p.Version, p.Type, p.Status, encConfig, p.Enabled, now, p.ID)
		if err != nil {
			return fmt.Errorf("update plugin: %w", err)
		}
		return nil
	}

	// Insert new
	p.ID = uuid.New()
	p.InstalledAt = now
	p.UpdatedAt = now

	if encConfig == nil {
		encConfig = json.RawMessage("{}")
	}
	if p.Status == "" {
		p.Status = "installed"
	}
	if p.Type == "" {
		p.Type = "builtin"
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO plugins (id, name, version, type, status, config, enabled, tenant_id, installed_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, p.ID, p.Name, p.Version, p.Type, p.Status, encConfig, p.Enabled, p.TenantID, now, now)
	if err != nil {
		return fmt.Errorf("register plugin: %w", err)
	}

	return nil
}

// GetByNameDecrypted returns a plugin by name with config decrypted.
func (r *PluginRepository) GetByNameDecrypted(ctx context.Context, name string) (*Plugin, error) {
	p, err := r.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	p.Config = decryptPluginConfig(p.Config)
	return p, nil
}

// encryptPluginConfig encrypts sensitive keys in plugin config JSON.
func encryptPluginConfig(config json.RawMessage) json.RawMessage {
	if config == nil || len(config) == 0 {
		return config
	}
	var m map[string]any
	if err := json.Unmarshal(config, &m); err != nil {
		return config // return as-is if not valid JSON
	}
	changed := false
	for _, key := range pluginSensitiveKeys {
		if val, ok := m[key]; ok {
			if strVal, ok := val.(string); ok && strVal != "" && !crypto.IsEncrypted(strVal) {
				if enc, err := crypto.Encrypt(strVal); err == nil {
					m[key] = enc
					changed = true
				}
			}
		}
	}
	if !changed {
		return config
	}
	result, err := json.Marshal(m)
	if err != nil {
		return config
	}
	return result
}

// DecryptPluginConfig decrypts sensitive keys in plugin config JSON.
// Exported for use in the API layer (e.g. listing plugins with decrypted config).
func DecryptPluginConfig(config json.RawMessage) json.RawMessage {
	return decryptPluginConfig(config)
}

// decryptPluginConfig decrypts sensitive keys in plugin config JSON.
func decryptPluginConfig(config json.RawMessage) json.RawMessage {
	if config == nil || len(config) == 0 {
		return config
	}
	var m map[string]any
	if err := json.Unmarshal(config, &m); err != nil {
		return config
	}
	changed := false
	for _, key := range pluginSensitiveKeys {
		if val, ok := m[key]; ok {
			if strVal, ok := val.(string); ok && crypto.IsEncrypted(strVal) {
				if dec, err := crypto.Decrypt(strVal); err == nil {
					m[key] = dec
					changed = true
				}
			}
		}
	}
	if !changed {
		return config
	}
	result, err := json.Marshal(m)
	if err != nil {
		return config
	}
	return result
}

// scan helper
type pluginScanner interface {
	Scan(dest ...interface{}) error
}

func scanPlugin(rows pgx.Rows) (*Plugin, error) {
	var p Plugin
	var configJSON []byte
	var tenantID sql.NullString

	if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Type, &p.Status,
		&configJSON, &p.Enabled, &tenantID, &p.InstalledAt, &p.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan plugin: %w", err)
	}

	if configJSON != nil {
		p.Config = configJSON
	}
	if tenantID.Valid {
		tid, _ := uuid.Parse(tenantID.String)
		p.TenantID = &tid
	}

	return &p, nil
}
