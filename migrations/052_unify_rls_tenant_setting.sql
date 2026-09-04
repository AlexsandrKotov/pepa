-- Migration 052: Unify RLS policy tenant isolation pattern
-- Fixes inconsistency where some tables use current_setting('app.tenant_id')::uuid
-- (which errors if the setting is missing) while others use the safer
-- current_setting('app.tenant_id', true) pattern (returns empty string if unset).
-- All policies are unified to the safer text-comparison pattern.

-- ── scan_targets ──────────────────────────────────────────────
DROP POLICY IF EXISTS "Users can view scan targets in their tenant" ON scan_targets;
DROP POLICY IF EXISTS "Users can create scan targets in their tenant" ON scan_targets;
DROP POLICY IF EXISTS "Users can update scan targets in their tenant" ON scan_targets;
DROP POLICY IF EXISTS "Users can delete scan targets in their tenant" ON scan_targets;

CREATE POLICY "scan_targets_tenant_isolation" ON scan_targets
    FOR ALL
    USING (tenant_id::text = current_setting('app.tenant_id', true));

-- ── scan_runs ─────────────────────────────────────────────────
DROP POLICY IF EXISTS "Users can view scan runs in their tenant" ON scan_runs;
DROP POLICY IF EXISTS "Users can create scan runs in their tenant" ON scan_runs;
DROP POLICY IF EXISTS "Users can update scan runs in their tenant" ON scan_runs;

CREATE POLICY "scan_runs_tenant_isolation" ON scan_runs
    FOR ALL
    USING (tenant_id::text = current_setting('app.tenant_id', true));

-- ── scan_schedules ────────────────────────────────────────────
DROP POLICY IF EXISTS "Users can view scan schedules in their tenant" ON scan_schedules;
DROP POLICY IF EXISTS "Users can create scan schedules in their tenant" ON scan_schedules;
DROP POLICY IF EXISTS "Users can update scan schedules in their tenant" ON scan_schedules;
DROP POLICY IF EXISTS "Users can delete scan schedules in their tenant" ON scan_schedules;

CREATE POLICY "scan_schedules_tenant_isolation" ON scan_schedules
    FOR ALL
    USING (tenant_id::text = current_setting('app.tenant_id', true));
