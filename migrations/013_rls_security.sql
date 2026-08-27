-- ============================================================
-- Migration 013: Row-Level Security hardening
-- ============================================================
-- Enables RLS on all tenant-scoped tables that were missing it.
-- Also adds a unique constraint on permissions to prevent
-- duplicate rows when seed data is re-applied.
-- ============================================================

-- ── Enable RLS on tenant-scoped tables ───────────────────────

ALTER TABLE services ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_services ON services;
CREATE POLICY tenant_isolation_services ON services
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE connections ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_connections ON connections;
CREATE POLICY tenant_isolation_connections ON connections
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE deployments ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_deployments ON deployments;
CREATE POLICY tenant_isolation_deployments ON deployments
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_audit_log ON audit_log;
CREATE POLICY tenant_isolation_audit_log ON audit_log
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE pipeline_sources ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_pipeline_sources ON pipeline_sources;
CREATE POLICY tenant_isolation_pipeline_sources ON pipeline_sources
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE pipeline_runs ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_pipeline_runs ON pipeline_runs;
CREATE POLICY tenant_isolation_pipeline_runs ON pipeline_runs
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE clusters ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_clusters ON clusters;
CREATE POLICY tenant_isolation_clusters ON clusters
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE workflows ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_workflows ON workflows;
CREATE POLICY tenant_isolation_workflows ON workflows
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE scorecards ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_scorecards ON scorecards;
CREATE POLICY tenant_isolation_scorecards ON scorecards
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE docker_hosts ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_docker_hosts ON docker_hosts;
CREATE POLICY tenant_isolation_docker_hosts ON docker_hosts
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE docker_services ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_docker_services ON docker_services;
CREATE POLICY tenant_isolation_docker_services ON docker_services
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE helm_repositories ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_helm_repositories ON helm_repositories;
CREATE POLICY tenant_isolation_helm_repositories ON helm_repositories
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- ── Unique constraint on permissions ─────────────────────────
-- Prevents duplicate permission rows when seed data is re-applied.

ALTER TABLE permissions ADD CONSTRAINT uq_permissions
    UNIQUE (role_id, resource, action, effect);

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (13, 'Enable RLS on all tenant-scoped tables + unique constraint on permissions')
ON CONFLICT DO NOTHING;
