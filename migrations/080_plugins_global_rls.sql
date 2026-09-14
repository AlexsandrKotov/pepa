-- ============================================================
-- 080: plugins is a global registry — let RLS say so
-- ============================================================
-- The plugin table is deliberately cross-tenant: `name` carries a global UNIQUE
-- constraint and the repository reads it without any tenant filter
-- (internal/repository/plugin_repo.go List / GetByName), because a built-in
-- provider binary is the same for every tenant. Registering one writes
-- tenant_id = NULL.
--
-- 078 did not know that. It attached its blanket
--     USING (tenant_id::text = current_setting('app.tenant_id', true))
-- policy to every table with a tenant_id column, plugins included. Under the
-- app role that policy is unsatisfiable for the registry: NULL = '<uuid>' is
-- NULL, so the Marketplace could list nothing and every install failed with
-- SQLSTATE 42501 — even with the tenant GUC correctly pinned.
--
-- So the table keeps row-level security, but with policies that model what it
-- actually is: readable by any session that got past RBAC, writable either as a
-- global entry (tenant_id IS NULL) or as a tenant-scoped row of the current
-- tenant. ENABLE / FORCE ROW LEVEL SECURITY are left exactly as 078 set them.
--
-- CREATE POLICY accepts exactly one command, so the write side is three
-- policies rather than a single "FOR INSERT, UPDATE, DELETE".
-- DROP POLICY IF EXISTS before every CREATE keeps this file re-runnable.

-- 1. Remove the blanket tenant-isolation policy from 078.
DROP POLICY IF EXISTS plugins_tenant_isolation ON plugins;

-- 2. Reads are global: the registry has to be visible to every tenant.
DROP POLICY IF EXISTS plugins_read ON plugins;
CREATE POLICY plugins_read ON plugins FOR SELECT
    USING (true);

-- 3. Writes accept a global row (tenant_id IS NULL) or a row of the tenant the
--    session is pinned to, so a tenant can never rewrite the registry on behalf
--    of another one.
DROP POLICY IF EXISTS plugins_insert ON plugins;
CREATE POLICY plugins_insert ON plugins FOR INSERT
    WITH CHECK (
        tenant_id IS NULL
        OR tenant_id::text = current_setting('app.tenant_id', true)
    );

DROP POLICY IF EXISTS plugins_update ON plugins;
CREATE POLICY plugins_update ON plugins FOR UPDATE
    USING (true)
    WITH CHECK (
        tenant_id IS NULL
        OR tenant_id::text = current_setting('app.tenant_id', true)
    );

-- Uninstall must reach global rows, so the delete policy cannot demand a tenant.
DROP POLICY IF EXISTS plugins_delete ON plugins;
CREATE POLICY plugins_delete ON plugins FOR DELETE
    USING (true);

-- Guard: a leftover policy that only matches a non-NULL tenant would silently
-- put the Marketplace out of business again, so make sure it is gone.
DO $$
DECLARE
    v_leftover INT;
BEGIN
    SELECT COUNT(*) INTO v_leftover
    FROM pg_policies
    WHERE schemaname = 'public'
      AND tablename = 'plugins'
      AND policyname = 'plugins_tenant_isolation';

    IF v_leftover > 0 THEN
        RAISE EXCEPTION 'plugins still carries the blanket plugins_tenant_isolation policy';
    END IF;
END
$$;
