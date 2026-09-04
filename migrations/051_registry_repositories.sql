-- 051_registry_repositories.sql
-- Container image registry repositories (Docker Hub, GHCR, Harbor, ECR, GCR, ACR, etc.)
-- Modeled after helm_repositories for consistency.

CREATE TABLE IF NOT EXISTS registry_repositories (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    name            VARCHAR(128) NOT NULL,
    description     TEXT DEFAULT '',
    registry_type   VARCHAR(16) NOT NULL DEFAULT 'docker', -- 'docker','ghcr','harbor','ecr','gcr','acr','other'
    url             VARCHAR(512) NOT NULL,
    username        VARCHAR(256),
    password        VARCHAR(512),
    token           VARCHAR(1024),
    is_default      BOOLEAN DEFAULT FALSE,
    status          VARCHAR(32) DEFAULT 'active',
    last_checked_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_registry_repo_tenant ON registry_repositories(tenant_id);

-- RLS
ALTER TABLE registry_repositories ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_registry_repositories ON registry_repositories;
CREATE POLICY tenant_isolation_registry_repositories ON registry_repositories
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- Schema version tracking
INSERT INTO schema_migrations (version, description) VALUES
    (51, 'Add registry_repositories for container image registries')
ON CONFLICT DO NOTHING;
