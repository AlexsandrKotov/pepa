package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultQueryTimeout is applied to every Query/Exec when the caller's
// context does not already carry a deadline.
const DefaultQueryTimeout = 30 * time.Second

// DB wraps a pgx connection pool with helper methods.
type DB struct {
	Pool *pgxpool.Pool
}

// New creates a new database connection pool.
func New(connString string) (*DB, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	config.MaxConns = 20
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 10 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Close closes the connection pool.
func (d *DB) Close() {
	if d.Pool != nil {
		d.Pool.Close()
	}
}

// Query executes a query with the default query timeout if the context does
// not already carry a deadline.
func (d *DB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	ctx, cancel := withDefaultTimeout(ctx)
	defer cancel()
	return d.Pool.Query(ctx, sql, args...)
}

// QueryRow executes a single-row query with the default query timeout.
func (d *DB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	ctx, cancel := withDefaultTimeout(ctx)
	defer cancel()
	return d.Pool.QueryRow(ctx, sql, args...)
}

// Exec executes a statement with the default query timeout.
func (d *DB) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	ctx, cancel := withDefaultTimeout(ctx)
	defer cancel()
	return d.Pool.Exec(ctx, sql, args...)
}

// withDefaultTimeout returns a context with DefaultQueryTimeout if the
// caller's context does not already have a deadline.
func withDefaultTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, DefaultQueryTimeout)
}

// SetTenant sets the current tenant for RLS policies.
func (d *DB) SetTenant(ctx context.Context, tenantID string) error {
	_, err := d.Pool.Exec(ctx, "SELECT set_config('app.current_tenant', $1, true)", tenantID)
	return err
}
