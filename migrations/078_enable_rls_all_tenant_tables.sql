-- ============================================================
-- 078: Enable RLS on all tenant-scoped tables that lack it
-- ============================================================
-- This migration finds every table with a `tenant_id` column that
-- does not yet have row-level security enabled, turns it on, and
-- creates a default tenant-isolation policy.
--
-- Tables that already have RLS enabled are skipped.
-- The policies use the canonical GUC 'app.tenant_id' unified by migration 073.

DO $$
DECLARE
    rec RECORD;
    v_policy_name TEXT;
    v_created INT := 0;
    v_enabled INT := 0;
BEGIN
    -- Find all tables with a tenant_id column that don't have RLS enabled
    FOR rec IN
        SELECT t.table_schema, t.table_name
        FROM information_schema.tables t
        JOIN information_schema.columns c
            ON c.table_schema = t.table_schema
            AND c.table_name = t.table_name
            AND c.column_name = 'tenant_id'
        WHERE t.table_schema = 'public'
          AND t.table_type = 'BASE TABLE'
          AND NOT EXISTS (
              SELECT 1 FROM pg_tables pg
              WHERE pg.schemaname = t.table_schema
                AND pg.tablename = t.table_name
                AND pg.rowsecurity = true
          )
        ORDER BY t.table_name
    LOOP
        -- Enable RLS
        EXECUTE format('ALTER TABLE %I.%I ENABLE ROW LEVEL SECURITY',
            rec.table_schema, rec.table_name);
        EXECUTE format('ALTER TABLE %I.%I FORCE ROW LEVEL SECURITY',
            rec.table_schema, rec.table_name);
        v_enabled := v_enabled + 1;

        -- Create default tenant isolation policy if none exists
        v_policy_name := rec.table_name || '_tenant_isolation';
        IF NOT EXISTS (
            SELECT 1 FROM pg_policies
            WHERE schemaname = rec.table_schema
              AND tablename = rec.table_name
        ) THEN
            EXECUTE format(
                'CREATE POLICY %I ON %I.%I USING (tenant_id::text = current_setting(%L, true))',
                v_policy_name, rec.table_schema, rec.table_name, 'app.tenant_id'
            );
            v_created := v_created + 1;
        END IF;
    END LOOP;

    RAISE NOTICE 'RLS: enabled on % tables, created % policies', v_enabled, v_created;
END
$$;
