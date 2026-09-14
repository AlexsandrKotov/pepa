-- ============================================================
-- Migration 081: Add missing team_workflow_configs table
-- ============================================================
-- This table was defined in migration 027 but was missing from
-- init-db.sql, so fresh installs never created it. This migration
-- ensures existing and new databases have the table.
-- ============================================================

CREATE TABLE IF NOT EXISTS team_workflow_configs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    team_name       VARCHAR(128) NOT NULL,
    stages          JSONB NOT NULL DEFAULT '[]',
    gitops          JSONB NOT NULL DEFAULT '{}',
    ci              JSONB NOT NULL DEFAULT '{}',
    verification    JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT team_workflow_configs_tenant_team_unique UNIQUE (tenant_id, team_name)
);

CREATE INDEX IF NOT EXISTS idx_team_workflow_configs_tenant
    ON team_workflow_configs (tenant_id);

-- RLS (idempotent — safe if already enabled by an earlier migration)
ALTER TABLE team_workflow_configs ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_team_workflow_configs ON team_workflow_configs;
CREATE POLICY tenant_isolation_team_workflow_configs ON team_workflow_configs
    USING (tenant_id = current_setting('app.tenant_id', true)::UUID);

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (81, 'Add missing team_workflow_configs table')
ON CONFLICT DO NOTHING;
