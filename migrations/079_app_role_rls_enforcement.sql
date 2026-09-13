-- ============================================================
-- Migration 079: Application role for active RLS enforcement
-- ============================================================
-- Creates a non-superuser role (pepa_app) that the application
-- uses for runtime queries. Unlike the table-owner role (pepa),
-- pepa_app is subject to row-level-security policies, providing
-- defence-in-depth tenant isolation on top of the repository
-- WHERE tenant_id filters.
--
-- For fresh deployments, init-db.sql creates pepa_app before
-- migrations run. For existing deployments, this migration
-- creates it if missing. The Docker entrypoint rotates the
-- password to the configured APP_POSTGRES_PASSWORD before the
-- application starts.
-- ============================================================

-- 1. Ensure the application role exists.
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pepa_app') THEN
        -- Use a placeholder password; the entrypoint rotates it
        -- before the app connects. The placeholder is intentionally
        -- unusable so that even if someone discovers it, the window
        -- of exposure is limited to the time between migration and
        -- entrypoint rotation.
        EXECUTE format(
            'CREATE ROLE pepa_app LOGIN PASSWORD %L',
            'placeholder_change_me_' || md5(random()::text)
        );
    END IF;
END $$;

-- 2. Grant DML on all existing tables.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO pepa_app;

-- 3. Grant DML on all FUTURE tables (created by subsequent migrations).
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pepa_app;

-- 4. Grant usage on sequences (for serial/identity columns).
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO pepa_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE ON SEQUENCES TO pepa_app;

-- 5. Revoke schema creation — pepa_app must not alter the schema.
REVOKE CREATE ON SCHEMA public FROM pepa_app;

-- 6. Pin search path so pepa_app cannot be tricked by schema search
--    order manipulation (search_path injection).
ALTER ROLE pepa_app SET search_path = public;

-- 7. Ensure RLS is active: pepa_app must NOT have BYPASSRLS.
ALTER ROLE pepa_app NOBYPASSRLS;
