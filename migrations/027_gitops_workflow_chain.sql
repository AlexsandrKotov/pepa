-- ============================================================
-- Migration 027: GitOps workflow chain
-- ============================================================
-- 1. Persists per-team GitOps workflow configs (previously kept
--    in API-server memory only).
-- 2. Adds team/stage attribution to deployments so the GitOps
--    Workflow board can filter by team and promote across stages.
-- ============================================================

-- Team workflow configs (stages, gitops target, CI, verification)
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

-- Team/stage attribution for the deployment pipeline board
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS team_name VARCHAR(128) DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS stage VARCHAR(64) DEFAULT 'dev';

CREATE INDEX IF NOT EXISTS idx_deployments_tenant_team_stage
    ON deployments (tenant_id, team_name, stage);

-- RLS for the new table (pattern from migrations 013/023)
ALTER TABLE team_workflow_configs ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_team_workflow_configs ON team_workflow_configs;
CREATE POLICY tenant_isolation_team_workflow_configs ON team_workflow_configs
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (27, 'GitOps workflow chain: team workflow configs + deployment team/stage')
ON CONFLICT DO NOTHING;
