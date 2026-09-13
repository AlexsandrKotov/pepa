//go:build integration

package testenv

import (
	"context"
	"testing"
)

func TestTestEnv_StartsAndCleans(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup(t)

	ctx := context.Background()

	// Verify PostgreSQL is accessible and migrations applied.
	var version string
	err := env.DB().Pool.QueryRow(ctx, "SELECT version()").Scan(&version)
	if err != nil {
		t.Fatalf("query postgres: %v", err)
	}
	t.Logf("PostgreSQL: %s", version)

	// Verify pgvector extension.
	var extVersion string
	err = env.DB().Pool.QueryRow(ctx,
		"SELECT extversion FROM pg_extension WHERE extname = 'vector'",
	).Scan(&extVersion)
	if err != nil {
		t.Fatalf("pgvector not available: %v", err)
	}
	t.Logf("pgvector version: %s", extVersion)

	// Verify Redis is accessible.
	pong, err := env.Redis().Ping(ctx).Result()
	if err != nil {
		t.Fatalf("ping redis: %v", err)
	}
	if pong != "PONG" {
		t.Fatalf("expected PONG, got %s", pong)
	}
	t.Logf("Redis: %s", pong)

	// Verify migrations created tables.
	var tableCount int
	err = env.DB().Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'",
	).Scan(&tableCount)
	if err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tableCount < 10 {
		t.Fatalf("expected at least 10 tables after migrations, got %d", tableCount)
	}
	t.Logf("Tables created: %d", tableCount)
}

func TestTestEnv_ResetDB(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup(t)

	ctx := context.Background()

	// Insert some data (settings.value is jsonb, so wrap in JSON).
	_, err := env.DB().Exec(ctx,
		`INSERT INTO settings (key, value) VALUES ('test_key', '"test_value"')`,
	)
	if err != nil {
		t.Fatalf("insert data: %v", err)
	}

	// Verify data exists.
	var count int
	err = env.DB().Pool.QueryRow(ctx, "SELECT COUNT(*) FROM settings WHERE key = 'test_key'").Scan(&count)
	if err != nil {
		t.Fatalf("query data: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}

	// Reset database.
	env.ResetDB(t)

	// Verify data is gone.
	err = env.DB().Pool.QueryRow(ctx, "SELECT COUNT(*) FROM settings").Scan(&count)
	if err != nil {
		t.Fatalf("query after reset: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 rows after reset, got %d", count)
	}

	// Verify Redis is flushed.
	keys, err := env.Redis().Keys(ctx, "*").Result()
	if err != nil {
		t.Fatalf("redis keys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys after reset, got %d", len(keys))
	}
}
