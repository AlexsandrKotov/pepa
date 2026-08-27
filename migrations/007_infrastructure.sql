-- ============================================================
-- Migration 007: Infrastructure & Settings
-- ============================================================
-- Settings, environments, vault secrets, Docker hosts,
-- Docker services, Helm repositories, and schema_migrations
-- table creation.
-- ============================================================

-- ============================================================
-- SETTINGS (platform-wide configuration)
-- ============================================================

CREATE TABLE IF NOT EXISTS settings (
    key         VARCHAR(128) PRIMARY KEY,
    value       JSONB NOT NULL DEFAULT '{}',
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_by  UUID
);

-- ============================================================
-- ENVIRONMENTS
-- ============================================================

CREATE TABLE IF NOT EXISTS environments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    name        VARCHAR(64) NOT NULL,
    slug        VARCHAR(64) NOT NULL,
    description TEXT DEFAULT '',
    color       VARCHAR(7) DEFAULT '#6B7280',
    is_default  BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

-- Seed default environments
INSERT INTO environments (tenant_id, name, slug, description, color, is_default) VALUES
    ('00000000-0000-0000-0000-000000000002', 'Development', 'dev', 'Local development and testing', '#3B82F6', TRUE),
    ('00000000-0000-0000-0000-000000000002', 'Staging', 'staging', 'Pre-production validation', '#F59E0B', FALSE),
    ('00000000-0000-0000-0000-000000000002', 'Production', 'production', 'Live production environment', '#EF4444', FALSE)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- VAULT SECRETS (encrypted KV store)
-- ============================================================

CREATE TABLE IF NOT EXISTS vault_secrets (
    path           VARCHAR(512) PRIMARY KEY,
    encrypted_data TEXT NOT NULL,
    version        INT DEFAULT 1,
    tenant_id      UUID,
    created_by     VARCHAR(255),
    created_at     TIMESTAMPTZ DEFAULT NOW(),
    updated_at     TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_vault_secrets_tenant ON vault_secrets (tenant_id);

-- ============================================================
-- DOCKER HOSTS (remote/local Docker engines)
-- ============================================================

CREATE TABLE IF NOT EXISTS docker_hosts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL,
    name                VARCHAR(128) NOT NULL,
    description         TEXT DEFAULT '',
    host_type           VARCHAR(16) NOT NULL DEFAULT 'local', -- 'local','tcp','ssh'
    host_address        VARCHAR(512) NOT NULL DEFAULT 'unix:///var/run/docker.sock',
    tls_ca_cert         TEXT,
    tls_cert            TEXT,
    tls_key             TEXT,
    ssh_key             TEXT,
    status              VARCHAR(32) DEFAULT 'disconnected', -- 'connected','disconnected','error'
    docker_version      VARCHAR(64),
    os_arch             VARCHAR(64),
    containers_running  INTEGER DEFAULT 0,
    last_checked_at     TIMESTAMPTZ,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_docker_hosts_tenant ON docker_hosts(tenant_id);

-- ============================================================
-- DOCKER SERVICES (compose stacks deployed to Docker hosts)
-- ============================================================

CREATE TABLE IF NOT EXISTS docker_services (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    docker_host_id  UUID NOT NULL REFERENCES docker_hosts(id) ON DELETE CASCADE,
    name            VARCHAR(128) NOT NULL,
    compose_yaml    TEXT NOT NULL,
    env_vars        JSONB DEFAULT '{}',
    status          VARCHAR(32) DEFAULT 'deploying', -- 'running','stopped','error','deploying'
    containers      JSONB DEFAULT '[]',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_docker_svc_tenant ON docker_services(tenant_id);
CREATE INDEX IF NOT EXISTS idx_docker_svc_host ON docker_services(docker_host_id);

-- ============================================================
-- HELM REPOSITORIES (chart registries with credentials)
-- ============================================================

CREATE TABLE IF NOT EXISTS helm_repositories (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    name            VARCHAR(128) NOT NULL,
    description     TEXT DEFAULT '',
    repo_type       VARCHAR(16) NOT NULL DEFAULT 'http', -- 'git','http','oci'
    url             VARCHAR(512) NOT NULL,
    username        VARCHAR(256),
    password        VARCHAR(512),
    token           VARCHAR(1024),
    ssh_key         TEXT,
    ca_cert         TEXT,
    is_default      BOOLEAN DEFAULT FALSE,
    status          VARCHAR(32) DEFAULT 'active',
    last_checked_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_helm_repo_tenant ON helm_repositories(tenant_id);

-- ============================================================
-- SCHEMA VERSION TRACKING
-- ============================================================
-- This table is created here so that migrations 001-007 can
-- INSERT their version records. The Go migration runner also
-- ensures this table exists before running any migrations.

CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  TIMESTAMPTZ DEFAULT NOW(),
    description TEXT
);

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (7, 'Add settings, environments, vault_secrets, docker_hosts, docker_services, helm_repositories')
ON CONFLICT DO NOTHING;
