package database

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/pepa/pepa/migrations"
)

// RunMigrations applies all pending SQL migrations in version order.
// It creates the schema_migrations table if it does not exist, then
// applies each migration file that has not yet been recorded.
func (d *DB) RunMigrations(ctx context.Context) error {
	// Ensure schema_migrations table exists
	if err := d.ensureMigrationsTable(ctx); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	// Get already-applied versions
	applied, err := d.getAppliedVersions(ctx)
	if err != nil {
		return fmt.Errorf("get applied versions: %w", err)
	}

	// Read embedded migration files
	entries, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		return fmt.Errorf("list migration files: %w", err)
	}
	sort.Strings(entries)

	// Apply pending migrations
	for _, entry := range entries {
		version := parseMigrationVersion(entry)
		if version == 0 {
			continue // not a numbered migration
		}
		if applied[version] {
			continue // already applied
		}

		content, err := fs.ReadFile(migrations.Files, entry)
		if err != nil {
			return fmt.Errorf("read %s: %w", entry, err)
		}

		slog.Info("applying migration", "file", entry)
		start := time.Now()

		// Wrap each migration in a transaction so partial failures roll back.
		tx, txErr := d.Pool.Begin(ctx)
		if txErr != nil {
			return fmt.Errorf("begin tx for %s: %w", entry, txErr)
		}
		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", entry, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version, description) VALUES ($1, $2) ON CONFLICT DO NOTHING",
			version, entry,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", entry, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", entry, err)
		}

		slog.Info("migration applied", "file", entry, "duration", time.Since(start).Round(time.Millisecond))
	}

	return nil
}

// ensureMigrationsTable creates the schema_migrations table if it does not exist.
func (d *DB) ensureMigrationsTable(ctx context.Context) error {
	_, err := d.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     INTEGER PRIMARY KEY,
			applied_at  TIMESTAMPTZ DEFAULT NOW(),
			description TEXT
		)
	`)
	return err
}

// getAppliedVersions returns a set of already-applied migration versions.
func (d *DB) getAppliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := d.Pool.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// parseMigrationVersion extracts the leading version number from a migration
// filename like "001_initial_schema.sql" → 1. Returns 0 if no match.
func parseMigrationVersion(filename string) int {
	// Extract prefix before first underscore
	base := filename
	if idx := strings.Index(base, "_"); idx > 0 {
		base = base[:idx]
	}
	// Parse as integer
	var v int
	for _, c := range base {
		if c < '0' || c > '9' {
			return 0
		}
		v = v*10 + int(c-'0')
	}
	return v
}
