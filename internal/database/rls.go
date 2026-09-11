package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// TenantGUC is the single session variable every row-level-security policy in
// PEPA reads. Migration 073 retired the legacy 'app.current_tenant' spelling so
// there is exactly one knob to set; the startup check below fails loudly if a
// policy ever goes back to the old name.
const TenantGUC = "app.tenant_id"

// SetTenantInTx sets the RLS tenant for the lifetime of tx.
//
// The GUC is set with is_local = true, so it disappears when the transaction
// ends and can never leak into the next caller that happens to reuse this
// pooled connection. It must therefore be called inside a transaction: on a
// bare pool connection a local setting is rolled back immediately and has no
// effect at all.
func SetTenantInTx(ctx context.Context, tx pgx.Tx, tenantID string) error {
	if tx == nil {
		return fmt.Errorf("set tenant: nil transaction")
	}
	_, err := tx.Exec(ctx, "SELECT set_config($1, $2, true)", TenantGUC, tenantID)
	if err != nil {
		return fmt.Errorf("set tenant: %w", err)
	}
	return nil
}

// RLSReport describes how much of the schema row-level security actually covers
// and whether the connected database role is even subject to it.
type RLSReport struct {
	// RoleName is the current database user; owners/superusers bypass RLS.
	RoleName string
	// RoleBypassesRLS is true when the role is superuser, has BYPASSRLS, or owns
	// the tables — in that case policies exist but never apply to this app.
	RoleBypassesRLS bool
	// Policies is the total number of RLS policies in the schema.
	Policies int
	// LegacyGUCPolicies counts policies still reading app.current_tenant. Those
	// silently never match, because the application sets app.tenant_id only.
	LegacyGUCPolicies int
	// EnabledTables / ForcedTables track ALTER TABLE ... ENABLE / FORCE ROW LEVEL
	// SECURITY, which is what makes policies apply to the table owner too.
	EnabledTables int
	ForcedTables  int
	// TablesWithTenantColumnWithoutPolicy is the count of tenant-aware tables that
	// have no RLS policy at all — the isolation there depends purely on the query.
	TablesWithTenantColumnWithoutPolicy int
}

// InspectRLS reports the effective state of row-level security. It is meant for
// a startup self-check: RLS in PEPA is defence-in-depth on top of the explicit
// tenant_id filters in the repositories, and it is easy to believe it is
// protecting queries that it is not.
func (d *DB) InspectRLS(ctx context.Context) (*RLSReport, error) {
	r := &RLSReport{}
	if d == nil || d.Pool == nil {
		return r, fmt.Errorf("inspect rls: database not initialised")
	}

	err := d.Pool.QueryRow(ctx, `
		SELECT current_user,
		       (SELECT COUNT(*) FROM pg_policies WHERE schemaname = 'public'),
		       (SELECT COUNT(*) FROM pg_policies
		         WHERE schemaname = 'public'
		           AND (qual LIKE '%app.current_tenant%' OR with_check LIKE '%app.current_tenant%')),
		       (SELECT COUNT(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		         WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relrowsecurity),
		       (SELECT COUNT(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		         WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relforcerowsecurity),
		       (SELECT COUNT(*) FROM pg_attribute a
		         JOIN pg_class c ON c.oid = a.attrelid
		         JOIN pg_namespace n ON n.oid = c.relnamespace
		         WHERE n.nspname = 'public' AND c.relkind = 'r' AND a.attname = 'tenant_id'
		           AND a.attnum > 0 AND NOT a.attisdropped
		           AND NOT EXISTS (
		               SELECT 1 FROM pg_policies p
		               WHERE p.schemaname = n.nspname AND p.tablename = c.relname
		           )),
		       (SELECT bool_or(rolsuper OR rolbypassrls) FROM pg_roles WHERE rolname = current_user)
	`).Scan(
		&r.RoleName,
		&r.Policies,
		&r.LegacyGUCPolicies,
		&r.EnabledTables,
		&r.ForcedTables,
		&r.TablesWithTenantColumnWithoutPolicy,
		&r.RoleBypassesRLS,
	)
	if err != nil {
		return r, fmt.Errorf("inspect rls: %w", err)
	}

	// A role that owns the tables bypasses every non-FORCED policy even without
	// BYPASSRLS, so compare ownership as well.
	var ownedNonForced int
	if err := d.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_roles ro ON ro.oid = c.relowner
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relrowsecurity
		  AND NOT c.relforcerowsecurity
		  AND ro.rolname = current_user
	`).Scan(&ownedNonForced); err == nil && ownedNonForced > 0 {
		r.RoleBypassesRLS = true
	}

	return r, nil
}
