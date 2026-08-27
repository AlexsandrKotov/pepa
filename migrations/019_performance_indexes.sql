-- Performance indexes for common query patterns
-- These indexes improve read performance for frequently accessed data

-- Clusters: tenant + active status (used in list queries and discovery)
CREATE INDEX IF NOT EXISTS idx_clusters_tenant_active ON clusters(tenant_id, is_active);

-- Connections: tenant + type (used in connection list with type filter)
CREATE INDEX IF NOT EXISTS idx_connections_tenant_type ON connections(tenant_id, type);

-- Services: tenant + status (used in service list with status filter)
CREATE INDEX IF NOT EXISTS idx_services_tenant_status ON services(tenant_id, status);

-- Deployments: created_at descending (used in deployment list ordering)
CREATE INDEX IF NOT EXISTS idx_deployments_created ON deployments(created_at DESC);

-- Audit logs: tenant + created_at descending (used in audit list ordering)
CREATE INDEX IF NOT EXISTS idx_audit_tenant_created ON audit_log(tenant_id, created_at DESC);

-- Deployments: tenant + status (used in deployment filtering)
CREATE INDEX IF NOT EXISTS idx_deployments_tenant_status ON deployments(tenant_id, status);

-- Pipeline runs: tenant + created_at (used in pipeline run list ordering)
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_tenant_created ON pipeline_runs(tenant_id, created_at DESC);
