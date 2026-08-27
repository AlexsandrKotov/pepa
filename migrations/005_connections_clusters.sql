-- ============================================================
-- Migration 005: Connections, Clusters & Deployments
-- ============================================================
-- Unified connections registry, Kubernetes clusters, Flux
-- resources, deployments, Jira issues, incident knowledge
-- base, and audit log.
-- ============================================================

-- ============================================================
-- CONNECTIONS (unified service registry)
-- ============================================================
-- Stores configuration for all external services PEPA integrates with.
-- Deployment-agnostic: K8s clusters, GitLab, Jira, CI, AI providers, storage.
CREATE TABLE IF NOT EXISTS connections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    type            VARCHAR(32) NOT NULL,          -- 'kubernetes','gitlab','jira','ci','ai','storage'
    name            VARCHAR(128) NOT NULL,
    description     TEXT DEFAULT '',
    config          JSONB DEFAULT '{}',            -- credentials, tokens, kubeconfigs
    status          VARCHAR(32) DEFAULT 'disconnected', -- 'connected','disconnected','error','checking'
    last_check_at   TIMESTAMPTZ,
    labels          JSONB DEFAULT '{}',
    notes           TEXT DEFAULT '',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, type, name)
);

CREATE INDEX IF NOT EXISTS idx_connections_tenant ON connections(tenant_id);
CREATE INDEX IF NOT EXISTS idx_connections_type ON connections(type);

-- ============================================================
-- KUBERNETES CLUSTERS
-- ============================================================

CREATE TABLE IF NOT EXISTS clusters (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL,
    name                VARCHAR(128) NOT NULL,
    description         TEXT DEFAULT '',
    environment         VARCHAR(32) NOT NULL DEFAULT 'dev',
    api_server_url      VARCHAR(512),
    kubeconfig_encrypted TEXT,
    flux_installed      BOOLEAN DEFAULT FALSE,
    status              VARCHAR(32) DEFAULT 'disconnected',
    node_count          INTEGER DEFAULT 0,
    kubernetes_version  VARCHAR(32),
    labels              JSONB DEFAULT '{}',
    notes               TEXT DEFAULT '',
    is_active           BOOLEAN DEFAULT TRUE,
    connection_id       UUID REFERENCES connections(id) ON DELETE SET NULL,
    last_heartbeat_at   TIMESTAMPTZ,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_clusters_tenant ON clusters(tenant_id);

-- ============================================================
-- FLUXCD RESOURCES (Kustomizations, HelmReleases)
-- ============================================================

CREATE TABLE IF NOT EXISTS flux_resources (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL,
    cluster_id          UUID NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    namespace           VARCHAR(63) NOT NULL,
    name                VARCHAR(253) NOT NULL,
    kind                VARCHAR(32) NOT NULL,
    status              VARCHAR(32) NOT NULL DEFAULT 'Unknown',
    message             TEXT,
    revision            VARCHAR(256),
    last_reconciled_at  TIMESTAMPTZ,
    suspended           BOOLEAN DEFAULT FALSE,
    metadata            JSONB DEFAULT '{}',
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (cluster_id, namespace, name, kind)
);

CREATE INDEX IF NOT EXISTS idx_flux_cluster ON flux_resources(cluster_id);
CREATE INDEX IF NOT EXISTS idx_flux_tenant ON flux_resources(tenant_id);

-- ============================================================
-- DEPLOYMENTS (Jira -> GitLab -> K8s pipeline)
-- ============================================================

CREATE TABLE IF NOT EXISTS deployments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL,
    jira_issue_key      VARCHAR(32),
    jira_summary        VARCHAR(512),
    gitlab_project_id   INTEGER,
    gitlab_project_name VARCHAR(256),
    gitlab_mr_id        INTEGER,
    gitlab_mr_url       VARCHAR(512),
    target_cluster_id   UUID REFERENCES clusters(id),
    target_namespace    VARCHAR(63),
    image_tag           VARCHAR(128),
    image_repository    VARCHAR(256),
    deploy_type         VARCHAR(32) DEFAULT 'helm',
    replicas            INTEGER DEFAULT 1,
    strategy            VARCHAR(32) DEFAULT 'rolling',
    spec                JSONB DEFAULT '{}',
    status              VARCHAR(32) NOT NULL DEFAULT 'pending',
    error_message       TEXT,
    logs                TEXT,
    promoted_by         VARCHAR(64),
    promoted_at         TIMESTAMPTZ,
    created_by          VARCHAR(64),
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_deploy_tenant ON deployments(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_deploy_jira ON deployments(jira_issue_key);
CREATE INDEX IF NOT EXISTS idx_deploy_status ON deployments(status);

-- ============================================================
-- JIRA ISSUES (synced from Jira API)
-- ============================================================

CREATE TABLE IF NOT EXISTS jira_issues (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    issue_key       VARCHAR(32) NOT NULL,
    issue_id        VARCHAR(64),
    project_key     VARCHAR(32) NOT NULL,
    summary         VARCHAR(512) NOT NULL,
    description     TEXT,
    issue_type      VARCHAR(64),
    priority        VARCHAR(32),
    status          VARCHAR(64),
    assignee        VARCHAR(128),
    reporter        VARCHAR(128),
    labels          VARCHAR(128)[],
    components      VARCHAR(128)[],
    fix_versions    VARCHAR(64)[],
    story_points    INTEGER,
    parent_key      VARCHAR(32),
    jira_url        VARCHAR(512),
    linked_mr_id    INTEGER,
    linked_mr_url   VARCHAR(512),
    deployment_id   UUID REFERENCES deployments(id),
    metadata        JSONB DEFAULT '{}',
    synced_at       TIMESTAMPTZ DEFAULT NOW(),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, issue_key)
);

CREATE INDEX IF NOT EXISTS idx_jira_tenant ON jira_issues(tenant_id);
CREATE INDEX IF NOT EXISTS idx_jira_project ON jira_issues(project_key);
CREATE INDEX IF NOT EXISTS idx_jira_status ON jira_issues(status);

-- ============================================================
-- INCIDENT KNOWLEDGE BASE (RAG)
-- ============================================================

CREATE TABLE IF NOT EXISTS incident_knowledge_base (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL,
    error_pattern     TEXT NOT NULL,
    root_cause        TEXT NOT NULL,
    remediation_steps TEXT NOT NULL,
    severity          VARCHAR(16) DEFAULT 'warning',
    embedding         vector(1536),
    source            VARCHAR(64),
    created_at        TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_incident_tenant ON incident_knowledge_base(tenant_id);

-- ============================================================
-- AUDIT LOG
-- ============================================================

CREATE TABLE IF NOT EXISTS audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    user_id     UUID,
    api_key_id  UUID,
    plugin_name VARCHAR(128),
    action      VARCHAR(32) NOT NULL,
    entity_type VARCHAR(128),
    entity_id   UUID,
    old_values  JSONB,
    new_values  JSONB,
    diff        JSONB,
    ip_address  INET,
    user_agent  TEXT,
    request_id  UUID,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_tenant ON audit_log(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_log(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_log(user_id, created_at DESC);

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (5, 'Add connections, clusters, flux_resources, deployments, jira_issues, incident_knowledge_base, audit_log')
ON CONFLICT DO NOTHING;
