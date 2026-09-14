//go:build integration

package testenv

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/database"
)

// appRolePassword is the password we set for pepa_app in the test database.
// The migration creates the role with a random placeholder; we override it so
// the test can open a second pool as the non-owner role.
const appRolePassword = "pepa_app_test_password"

// setupAppRolePool resets the pepa_app password (set by migration 079 to a
// random value) and opens a connection pool as that role. The pool has the
// tenant GUC pinned to the given tenantID, mirroring the production
// "pinned" mode.
func setupAppRolePool(t *testing.T, ctx context.Context, env *TestEnv, tenantID string) *pgxpool.Pool {
	t.Helper()

	// Reset password to a known value — migration 079 uses a random placeholder.
	// ALTER ROLE is DDL and does not support $1 parameters, so use fmt.Sprintf.
	_, err := env.DB().Exec(ctx,
		fmt.Sprintf(`ALTER ROLE pepa_app PASSWORD '%s'`, appRolePassword))
	if err != nil {
		t.Fatalf("set pepa_app password: %v", err)
	}

	// Build a connection string that connects as pepa_app.
	ownerConnStr := env.PG.ConnStr()
	appConnStr := replaceRoleInConnStr(t, ownerConnStr, "pepa_app", appRolePassword)

	config, err := pgxpool.ParseConfig(appConnStr)
	if err != nil {
		t.Fatalf("parse app connection string: %v", err)
	}
	config.MaxConns = 5
	config.MinConns = 1

	// Pin the tenant GUC on every new connection — same mechanism as production.
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SELECT set_config($1, $2, false)", database.TenantGUC, tenantID)
		return err
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect as pepa_app: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping as pepa_app: %v", err)
	}

	return pool
}

// replaceRoleInConnStr swaps the user/password in a pgx connection string.
func replaceRoleInConnStr(t *testing.T, connStr, user, password string) string {
	t.Helper()
	c, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		t.Fatalf("parse conn string: %v", err)
	}
	// Build a new connection string with the updated credentials.
	// pgx's ConnString() doesn't always reflect ConnConfig mutations,
	// so we reconstruct it manually to ensure the new user/password are used.
	host := c.ConnConfig.Host
	port := c.ConnConfig.Port
	dbname := c.ConnConfig.Database
	return fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
		host, port, dbname, user, password)
}

// TestRLS_PinnedTenantSeesOwnData verifies that in pinned mode the app role
// sees rows belonging to the pinned tenant and nothing else. This is the core
// guarantee that the RLS tenant pin actually works.
func TestRLS_PinnedTenantSeesOwnData(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup(t)

	ctx := context.Background()

	tenantA := "00000000-0000-0000-0000-000000000002" // DefaultTenantID
	tenantB := "11111111-1111-1111-1111-111111111111"

	// Seed data for both tenants as the owner (superuser, bypasses RLS).
	_, err := env.DB().Exec(ctx,
		`INSERT INTO roles (id, name, slug, description, tenant_id) VALUES
		 (gen_random_uuid(), 'test-admin-a', 'test-admin-a', 'Tenant A admin', $1),
		 (gen_random_uuid(), 'test-admin-b', 'test-admin-b', 'Tenant B admin', $2)`,
		tenantA, tenantB)
	if err != nil {
		t.Fatalf("seed roles: %v", err)
	}

	// Open a pool as pepa_app pinned to tenant A.
	appPool := setupAppRolePool(t, ctx, env, tenantA)

	// Query roles through the app pool — should only see tenant A's row.
	var count int
	if err := appPool.QueryRow(ctx,
		`SELECT count(*) FROM roles WHERE name LIKE 'test-admin-%'`,
	).Scan(&count); err != nil {
		t.Fatalf("count roles as pepa_app: %v", err)
	}
	if count != 1 {
		t.Errorf("pinned to tenant A: want 1 role, got %d", count)
	}

	// Verify it is the correct row.
	var name string
	if err := appPool.QueryRow(ctx,
		`SELECT name FROM roles WHERE name LIKE 'test-admin-%'`,
	).Scan(&name); err != nil {
		t.Fatalf("read role name: %v", err)
	}
	if name != "test-admin-a" {
		t.Errorf("want test-admin-a, got %s", name)
	}
}

// TestRLS_GlobalPluginsTable verifies that the plugins table is readable
// globally (the registry is cross-tenant by contract) and that inserting with
// tenant_id = NULL succeeds.
func TestRLS_GlobalPluginsTable(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup(t)

	ctx := context.Background()

	tenantA := "00000000-0000-0000-0000-000000000002"
	appPool := setupAppRolePool(t, ctx, env, tenantA)

	// Insert a global plugin row (tenant_id = NULL).
	_, err := appPool.Exec(ctx,
		`INSERT INTO plugins (name, version, type, status, enabled)
		 VALUES ($1, $2, $3, $4, $5)`,
		"test-plugin", "0.1.0", "test", "installed", true)
	if err != nil {
		t.Fatalf("insert global plugin as pepa_app: %v", err)
	}

	// Read it back — the global read policy (USING true) must allow it.
	var name string
	if err := appPool.QueryRow(ctx,
		`SELECT name FROM plugins WHERE name = $1`, "test-plugin",
	).Scan(&name); err != nil {
		t.Fatalf("read global plugin as pepa_app: %v", err)
	}
	if name != "test-plugin" {
		t.Errorf("want test-plugin, got %s", name)
	}

	// Cleanup.
	_, _ = appPool.Exec(ctx, `DELETE FROM plugins WHERE name = $1`, "test-plugin")
}

// TestRLS_PoliciesExist verifies that RLS is enabled on the core tenant-scoped
// tables and that at least one policy exists per table. This catches accidental
// migration drops or schema regressions.
func TestRLS_PoliciesExist(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup(t)

	ctx := context.Background()

	// Core tables that must have RLS enabled with at least one policy.
	coreTables := []string{
		"services", "connections", "deployments", "environments",
		"roles", "workflows", "scorecards", "plugins",
	}

	for _, table := range coreTables {
		var enabled bool
		err := env.DB().QueryRow(ctx,
			`SELECT relrowsecurity FROM pg_class
			 JOIN pg_namespace ON pg_namespace.oid = pg_class.relnamespace
			 WHERE nspname = 'public' AND relname = $1`, table,
		).Scan(&enabled)
		if err != nil {
			t.Errorf("table %s: %v", table, err)
			continue
		}
		if !enabled {
			t.Errorf("table %s: RLS not enabled", table)
		}

		var policies int
		err = env.DB().QueryRow(ctx,
			`SELECT count(*) FROM pg_policies
			 WHERE schemaname = 'public' AND tablename = $1`, table,
		).Scan(&policies)
		if err != nil {
			t.Errorf("table %s policies: %v", table, err)
			continue
		}
		if policies == 0 {
			t.Errorf("table %s: no RLS policies", table)
		}
	}
}

// TestRLS_NoLegacyGUC verifies that no RLS policy references the retired
// app.current_tenant GUC. The application only sets app.tenant_id; a policy
// reading the old name silently matches nothing.
func TestRLS_NoLegacyGUC(t *testing.T) {
	env := NewTestEnv(t)
	defer env.Cleanup(t)

	ctx := context.Background()

	var legacy int
	err := env.DB().QueryRow(ctx,
		`SELECT count(*) FROM pg_policies
		 WHERE schemaname = 'public'
		   AND (qual LIKE '%%app.current_tenant%%'
		        OR with_check LIKE '%%app.current_tenant%%')`,
	).Scan(&legacy)
	if err != nil {
		t.Fatalf("check legacy GUC: %v", err)
	}
	if legacy > 0 {
		t.Errorf("%d policies still reference the retired app.current_tenant GUC", legacy)
	}
}
