package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/database"
)

// SettingsRepository handles platform-wide settings persistence.
type SettingsRepository struct {
	pool *pgxpool.Pool
}

// NewSettingsRepository creates a new settings repository.
func NewSettingsRepository(db *database.DB) *SettingsRepository {
	return &SettingsRepository{pool: db.Pool}
}

// Get returns the value for a single settings key.
func (r *SettingsRepository) Get(ctx context.Context, key string) (json.RawMessage, error) {
	var value []byte
	err := r.pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, key).Scan(&value)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("setting not found: %s", key)
		}
		return nil, fmt.Errorf("get setting: %w", err)
	}
	return json.RawMessage(value), nil
}

// GetAll returns all settings as a map.
func (r *SettingsRepository) GetAll(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := r.pool.Query(ctx, `SELECT key, value FROM settings ORDER BY key ASC`)
	if err != nil {
		return nil, fmt.Errorf("query settings: %w", err)
	}
	defer rows.Close()

	result := make(map[string]json.RawMessage)
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan setting: %w", err)
		}
		result[key] = json.RawMessage(value)
	}
	return result, nil
}

// Set creates or updates a settings key.
func (r *SettingsRepository) Set(ctx context.Context, key string, value json.RawMessage) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO settings (key, value, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE
		SET value = $2, updated_at = $3
	`, key, []byte(value), now)
	if err != nil {
		return fmt.Errorf("set setting: %w", err)
	}
	return nil
}

// Delete removes a settings key.
func (r *SettingsRepository) Delete(ctx context.Context, key string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM settings WHERE key = $1`, key)
	if err != nil {
		return fmt.Errorf("delete setting: %w", err)
	}
	return nil
}
