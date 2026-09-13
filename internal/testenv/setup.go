package testenv

import (
	"context"
	"testing"

	"github.com/pepa/pepa/internal/database"
	goredis "github.com/redis/go-redis/v9"
)

// TestEnv holds all test infrastructure containers.
type TestEnv struct {
	PG *PostgresContainer
	RD *RedisContainer
}

// NewTestEnv starts all required containers (PostgreSQL with pgvector, Redis).
// It applies migrations to the database and returns a ready-to-use test environment.
//
// Usage:
//
//	func TestSomething(t *testing.T) {
//	    env := testenv.NewTestEnv(t)
//	    defer env.Cleanup(t)
//	    // use env.DB() and env.Redis()
//	}
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()
	ctx := context.Background()

	pg, err := StartPostgres(ctx, t)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}

	rd, err := StartRedis(ctx, t)
	if err != nil {
		_ = pg.Terminate(ctx)
		t.Fatalf("start redis: %v", err)
	}

	return &TestEnv{
		PG: pg,
		RD: rd,
	}
}

// DB returns the database connection with migrations applied.
func (e *TestEnv) DB() *database.DB {
	return e.PG.DB()
}

// Redis returns the Redis client.
func (e *TestEnv) Redis() *goredis.Client {
	return e.RD.Client()
}

// Cleanup terminates all containers.
// Call this in a defer after NewTestEnv.
func (e *TestEnv) Cleanup(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	if e.RD != nil {
		if err := e.RD.Terminate(ctx); err != nil {
			t.Logf("terminate redis: %v", err)
		}
	}
	if e.PG != nil {
		if err := e.PG.Terminate(ctx); err != nil {
			t.Logf("terminate postgres: %v", err)
		}
	}
}

// ResetDB clears all data from the database while preserving the schema.
// Useful for tests that need a clean slate without restarting containers.
func (e *TestEnv) ResetDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	// Truncate all user tables in public schema.
	// This is faster than dropping and recreating the schema.
	_, err := e.DB().Exec(ctx, `
		DO $$ DECLARE
			r RECORD;
		BEGIN
			FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname = 'public') LOOP
				EXECUTE 'TRUNCATE TABLE ' || quote_ident(r.tablename) || ' CASCADE';
			END LOOP;
		END $$;
	`)
	if err != nil {
		t.Fatalf("reset database: %v", err)
	}

	// Flush Redis.
	if err := e.Redis().FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush redis: %v", err)
	}
}
