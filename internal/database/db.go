package database

import (
	"context"
	"fmt"
	"strings"
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

	// pin records how TenantGUC is populated for connections from this pool.
	pin TenantPin
}

// TenantPin selects how the app.tenant_id GUC — the single knob every RLS
// policy in the schema reads — is populated for a connection pool.
//
// The GUC cannot be set per call on a pooled connection: set_config(..., true)
// is transaction-local, so outside an explicit transaction it is discarded
// immediately, and the next caller draws a different connection. Pinning at
// connection setup is therefore the only mechanism that works with the current
// "repositories hold a *pgxpool.Pool" design.
type TenantPin struct {
	// Mode is "off", "pinned" or "per_request" (see config.RLSTenantMode*).
	Mode string
	// TenantID is the tenant pinned onto every connection when Mode is "pinned".
	TenantID string
}

// New creates a new database connection pool with RLS tenant pinning disabled.
func New(connString string) (*DB, error) {
	return NewWithPin(connString, TenantPin{Mode: "off"})
}

// NewWithPin creates a database connection pool whose connections carry the
// given RLS tenant pin. In "pinned" mode every connection is established with
// app.tenant_id already set to pin.TenantID, so row-level security applies to
// the non-owner runtime role instead of silently filtering everything out.
func NewWithPin(connString string, pin TenantPin) (*DB, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	config.MaxConns = 50
	config.MinConns = 5
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 10 * time.Second

	switch pin.Mode {
	case PinModePinned:
		tenantID := strings.TrimSpace(pin.TenantID)
		if tenantID == "" {
			return nil, fmt.Errorf("tenant pin: %s mode requires a tenant id", PinModePinned)
		}
		// Session-scoped (is_local = false): the value must survive the implicit
		// transaction that set_config runs in, because it has to be in effect for
		// every later query that reuses this pooled connection.
		config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			if _, err := conn.Exec(ctx, "SELECT set_config($1, $2, false)", TenantGUC, tenantID); err != nil {
				return fmt.Errorf("set %s on new connection: %w", TenantGUC, err)
			}
			return nil
		}
	case PinModeOff, "":
		// No GUC: policies that read it can never match, which is only correct
		// when the connecting role owns the tables or has BYPASSRLS.
	case PinModePerRequest:
		return nil, fmt.Errorf("tenant pin: %s mode needs request-scoped connection pinning, which the repositories do not support yet", PinModePerRequest)
	default:
		return nil, fmt.Errorf("tenant pin: unknown mode %q", pin.Mode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Verify connection. This also exercises AfterConnect, because the pool
	// pre-spawns MinConns connections during Ping.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &DB{Pool: pool, pin: pin}, nil
}

// TenantPin modes, mirroring config.RLSTenantMode*. Kept as literals in this
// package so internal/database does not depend on internal/config.
const (
	PinModeOff        = "off"
	PinModePinned     = "pinned"
	PinModePerRequest = "per_request"
)

// Pin returns the tenant-pinning configuration this pool was created with.
func (d *DB) Pin() TenantPin { return d.pin }

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

// BeginTx starts a new database transaction. The caller is responsible for
// committing or rolling back the transaction. Use with defer:
//
//	tx, err := db.BeginTx(ctx)
//	if err != nil { return err }
//	defer tx.Rollback(ctx) // no-op after Commit
func (d *DB) BeginTx(ctx context.Context) (pgx.Tx, error) {
	ctx, cancel := withDefaultTimeout(ctx)
	defer cancel()
	return d.Pool.Begin(ctx)
}
