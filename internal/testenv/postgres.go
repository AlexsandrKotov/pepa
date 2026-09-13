// Package testenv provides test infrastructure for integration tests.
//
// All containers are started on-demand and cleaned up when Cleanup() is called.
// Tests must use the //go:build integration build tag.
package testenv

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/database"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// pgImage is the PostgreSQL image with pgvector extension.
	pgImage = "pgvector/pgvector:pg16"
	// pgUser is the default superuser for test databases.
	pgUser = "pepa"
	// pgPassword is the password for the test superuser.
	pgPassword = "pepa-test"
	// pgDatabase is the name of the test database.
	pgDatabase = "pepa_test"
)

// PostgresContainer wraps a testcontainers PostgreSQL instance with pgvector.
type PostgresContainer struct {
	container *postgres.PostgresContainer
	connStr   string
	db        *database.DB
}

// StartPostgres starts a PostgreSQL container with pgvector extension.
// It applies all migrations from the embedded migrations package.
func StartPostgres(ctx context.Context, t *testing.T) (*PostgresContainer, error) {
	t.Helper()

	pgContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage(pgImage),
		postgres.WithDatabase(pgDatabase),
		postgres.WithUsername(pgUser),
		postgres.WithPassword(pgPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("get connection string: %w", err)
	}

	// Apply migrations using the embedded migrations package.
	db, err := database.New(connStr)
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	if err := db.RunMigrations(ctx); err != nil {
		db.Close()
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// Verify pgvector extension is available.
	var extVersion string
	err = db.Pool.QueryRow(ctx,
		"SELECT extversion FROM pg_extension WHERE extname = 'vector'",
	).Scan(&extVersion)
	if err != nil {
		db.Close()
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("pgvector extension not available: %w", err)
	}

	t.Logf("PostgreSQL started with pgvector %s", extVersion)

	return &PostgresContainer{
		container: pgContainer,
		connStr:   connStr,
		db:        db,
	}, nil
}

// DB returns the database connection with migrations applied.
func (p *PostgresContainer) DB() *database.DB {
	return p.db
}

// ConnStr returns the connection string for direct pool creation.
func (p *PostgresContainer) ConnStr() string {
	return p.connStr
}

// NewPool creates a new connection pool to the test database.
// Useful for tests that need isolated connections.
func (p *PostgresContainer) NewPool(ctx context.Context) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(p.connStr)
	if err != nil {
		return nil, err
	}
	config.MaxConns = 10
	config.MinConns = 1
	return pgxpool.NewWithConfig(ctx, config)
}

// Terminate stops and removes the PostgreSQL container.
func (p *PostgresContainer) Terminate(ctx context.Context) error {
	if p.db != nil {
		p.db.Close()
	}
	if p.container != nil {
		return p.container.Terminate(ctx)
	}
	return nil
}
