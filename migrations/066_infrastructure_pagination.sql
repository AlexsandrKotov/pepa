-- 066_infrastructure_pagination.sql
-- Composite indexes to support paginated, filtered queries on infrastructure tables.

-- Clusters: filter by environment+status, sort by created_at
CREATE INDEX IF NOT EXISTS idx_clusters_tenant_env_status ON clusters(tenant_id, environment, status);
CREATE INDEX IF NOT EXISTS idx_clusters_tenant_created_desc ON clusters(tenant_id, created_at DESC);

-- Connections: filter by status, sort by created_at
CREATE INDEX IF NOT EXISTS idx_connections_tenant_status ON connections(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_connections_tenant_created_desc ON connections(tenant_id, created_at DESC);

-- Deployments: filter by status, cluster; sort by created_at
CREATE INDEX IF NOT EXISTS idx_deployments_tenant_status ON deployments(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_deployments_tenant_cluster ON deployments(tenant_id, target_cluster_id);
CREATE INDEX IF NOT EXISTS idx_deployments_tenant_created_desc ON deployments(tenant_id, created_at DESC);

-- Docker hosts: filter by status
CREATE INDEX IF NOT EXISTS idx_docker_hosts_tenant_status ON docker_hosts(tenant_id, status);

-- Docker services: filter by host
CREATE INDEX IF NOT EXISTS idx_docker_services_tenant_host ON docker_services(tenant_id, docker_host_id);
