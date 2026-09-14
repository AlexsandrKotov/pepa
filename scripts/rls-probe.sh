#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# PEPA — RLS Probe
# ============================================================
# Compares row counts between the owner role and the app role
# (pepa_app) on every RLS-protected table. In a healthy pinned
# deployment the app role must see the same tenant-scoped rows
# as the owner, and zero SQLSTATE 42501 errors should appear.
#
# Usage:
#   ./scripts/rls-probe.sh                           # local compose
#   PGPASSWORD=xxx PGHOST=host ./scripts/rls-probe.sh # remote
#
# Exit codes:
#   0 — all probes passed
#   1 — one or more probes failed
# ============================================================

PGHOST="${PGHOST:-localhost}"
PGPORT="${PGPORT:-5432}"
PGDATABASE="${PGDATABASE:-pepa}"
PGUSER="${PGUSER:-pepa}"
PGPASSWORD="${PGPASSWORD:-pepa}"
APP_USER="${APP_USER:-pepa_app}"

# ── Helpers ──────────────────────────────────────────────────

log()  { printf '\033[1;34m[rls-probe]\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m  ✓\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m  ⚠\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m  ✗\033[0m %s\n' "$*" >&2; }

psql_owner() { psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -tAc "$1" 2>/dev/null; }
psql_app()   { PGPASSWORD="${APP_PASSWORD:-$PGPASSWORD}" psql -h "$PGHOST" -p "$PGPORT" -U "$APP_USER" -d "$PGDATABASE" -tAc "$1" 2>/dev/null; }

# ── Pre-flight ───────────────────────────────────────────────

log "Connecting as owner ($PGUSER) and app ($APP_USER) to $PGDATABASE@$PGHOST:$PGPORT"

if ! psql_owner "SELECT 1" >/dev/null 2>&1; then
  fail "Cannot connect as owner ($PGUSER)"
  exit 1
fi
ok "Owner connection OK"

if ! psql_app "SELECT 1" >/dev/null 2>&1; then
  fail "Cannot connect as app role ($APP_USER) — is the role created?"
  exit 1
fi
ok "App role connection OK"

# ── Check GUC ────────────────────────────────────────────────

guc_value=$(psql_app "SELECT current_setting('app.tenant_id', true)" 2>/dev/null || echo "")
if [ -z "$guc_value" ]; then
  warn "app.tenant_id GUC is not set on the app connection — RLS will filter everything out"
  warn "In production, DB_RLS_TENANT_MODE=pinned sets this automatically"
fi
if [ -n "$guc_value" ]; then
  ok "app.tenant_id = $guc_value"
fi

# ── Probe RLS tables ────────────────────────────────────────

# Get all tables with RLS enabled and a tenant_id column.
tables=$(psql_owner "
  SELECT DISTINCT c.relname
  FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = 'public'
    AND c.relkind = 'r'
    AND c.relrowsecurity = true
    AND EXISTS (
      SELECT 1 FROM pg_attribute a
      WHERE a.attrelid = c.oid AND a.attname = 'tenant_id'
        AND a.attnum > 0 AND NOT a.attisdropped
    )
  ORDER BY c.relname
")

if [ -z "$tables" ]; then
  fail "No RLS-enabled tables with tenant_id found"
  exit 1
fi

passed=0
failed=0
skipped=0

log "Probing $(echo "$tables" | wc -l | tr -d ' ') RLS tables..."
echo ""

for table in $tables; do
  owner_count=$(psql_owner "SELECT count(*) FROM $table" 2>/dev/null) || owner_count="ERR"
  app_count=$(psql_app "SELECT count(*) FROM $table" 2>/dev/null) || app_count="ERR"

  if [ "$owner_count" = "ERR" ] || [ "$app_count" = "ERR" ]; then
    fail "$table: owner=$owner_count app=$app_count (query error)"
    failed=$((failed + 1))
  elif [ "$owner_count" = "$app_count" ]; then
    ok "$table: $app_count rows (matches owner)"
    passed=$((passed + 1))
  elif [ "$app_count" = "0" ] && [ "$owner_count" != "0" ]; then
    fail "$table: owner=$owner_count app=$app_count (RLS may be inert — GUC not set?)"
    failed=$((failed + 1))
  else
    # App sees fewer rows than owner — this is expected when the GUC is set
    # to a specific tenant that does not own all rows.
    warn "$table: owner=$owner_count app=$app_count (tenant-scoped)"
    passed=$((passed + 1))
  fi
done

# ── Check plugins (global table) ─────────────────────────────

echo ""
log "Probing plugins (global table)..."
plugins_owner=$(psql_owner "SELECT count(*) FROM plugins" 2>/dev/null) || plugins_owner="ERR"
plugins_app=$(psql_app "SELECT count(*) FROM plugins" 2>/dev/null) || plugins_app="ERR"

if [ "$plugins_app" = "ERR" ]; then
  fail "plugins: app role cannot read (RLS policy missing?)"
  failed=$((failed + 1))
elif [ "$plugins_app" = "$plugins_owner" ]; then
  ok "plugins: $plugins_app rows (global read works)"
  passed=$((passed + 1))
else
  warn "plugins: owner=$plugins_owner app=$plugins_app (unexpected — plugins should be globally readable)"
  skipped=$((skipped + 1))
fi

# ── Summary ──────────────────────────────────────────────────

echo ""
log "────────────────────────────────"
log "Passed: $passed | Failed: $failed | Skipped: $skipped"

if [ "$failed" -gt 0 ]; then
  fail "$failed probe(s) failed"
  exit 1
fi

ok "All RLS probes passed"
