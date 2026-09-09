-- 061_docker_service_rollback.sql
-- Add deployment history tracking for Docker services to support rollback.

-- Store compose snapshots for rollback capability.
CREATE TABLE IF NOT EXISTS docker_service_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES docker_services(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL,
    compose_yaml TEXT NOT NULL,
    deployed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deployed_by TEXT,
    status VARCHAR(32) DEFAULT 'deployed'
);

CREATE INDEX IF NOT EXISTS idx_docker_svc_history_service ON docker_service_history(service_id);
CREATE INDEX IF NOT EXISTS idx_docker_svc_history_tenant ON docker_service_history(tenant_id);

-- RBAC: allow developers to rollback docker services
INSERT INTO permissions (id, role_id, resource, action, effect)
SELECT gen_random_uuid(), '20000000-0000-0000-0000-000000000002',
       'docker_services', 'rollback', 'allow'
WHERE NOT EXISTS (
    SELECT 1 FROM permissions
    WHERE role_id = '20000000-0000-0000-0000-000000000002'
      AND resource = 'docker_services' AND action = 'rollback'
);
