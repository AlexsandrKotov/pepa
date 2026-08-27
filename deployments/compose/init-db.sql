-- ============================================================
-- PEPA — Database Initialization
-- ============================================================
-- This script runs automatically on first PostgreSQL startup.
-- It creates the core schema, extensions, and seed data.
-- ============================================================

-- ── Extensions ───────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "vector";       -- PGvector for AI embeddings
CREATE EXTENSION IF NOT EXISTS "pg_trgm";      -- Trigram for text search

-- ============================================================
-- ORGANIZATIONS & TENANTS
-- ============================================================

CREATE TABLE IF NOT EXISTS organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(256) NOT NULL,
    slug        VARCHAR(128) NOT NULL UNIQUE,
    settings    JSONB DEFAULT '{}',
    plan        VARCHAR(32) DEFAULT 'community',
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tenants (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    name            VARCHAR(256) NOT NULL,
    slug            VARCHAR(128) NOT NULL,
    settings        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, slug)
);

-- Default organization and tenant
INSERT INTO organizations (id, name, slug) VALUES
    ('00000000-0000-0000-0000-000000000001', 'Default Organization', 'default')
ON CONFLICT DO NOTHING;

INSERT INTO tenants (id, organization_id, name, slug) VALUES
    ('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 'Default Tenant', 'default')
ON CONFLICT DO NOTHING;

-- ============================================================
-- USERS & AUTHENTICATION
-- ============================================================

CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           VARCHAR(256) NOT NULL UNIQUE,
    name            VARCHAR(256) NOT NULL,
    avatar_url      VARCHAR(512),
    auth_provider   VARCHAR(32) DEFAULT 'local',
    external_id     VARCHAR(256),
    password_hash   VARCHAR(256),
    is_active       BOOLEAN DEFAULT TRUE,
    must_change_password BOOLEAN DEFAULT FALSE,
    token_version   INTEGER NOT NULL DEFAULT 0,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(256) NOT NULL,
    key_hash    VARCHAR(512) NOT NULL,
    key_prefix  VARCHAR(16) NOT NULL,
    tenant_id   UUID NOT NULL,
    created_by  UUID REFERENCES users(id),
    expires_at  TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    is_active   BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================
-- TEAMS
-- ============================================================

CREATE TABLE IF NOT EXISTS teams (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    name            VARCHAR(256) NOT NULL,
    slug            VARCHAR(128) NOT NULL,
    description     TEXT,
    parent_team_id  UUID REFERENCES teams(id),
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

CREATE TABLE IF NOT EXISTS team_memberships (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id     UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        VARCHAR(64) NOT NULL DEFAULT 'member',
    joined_at   TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (team_id, user_id)
);

-- ============================================================
-- RBAC — ROLES & PERMISSIONS
-- ============================================================

CREATE TABLE IF NOT EXISTS roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    name        VARCHAR(128) NOT NULL,
    slug        VARCHAR(128) NOT NULL,
    description TEXT,
    is_system   BOOLEAN DEFAULT FALSE,
    scope       VARCHAR(32) DEFAULT 'tenant',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

CREATE TABLE IF NOT EXISTS permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id     UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    resource    VARCHAR(128) NOT NULL,
    action      VARCHAR(64) NOT NULL,
    effect      VARCHAR(16) DEFAULT 'allow',
    conditions  JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (role_id, resource, action, effect)
);

CREATE TABLE IF NOT EXISTS role_assignments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    user_id     UUID REFERENCES users(id),
    team_id     UUID REFERENCES teams(id),
    role_id     UUID NOT NULL REFERENCES roles(id),
    scope_type  VARCHAR(32),
    scope_value VARCHAR(256),
    granted_by  UUID REFERENCES users(id),
    expires_at  TIMESTAMPTZ,
    is_active   BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_role_assign_user ON role_assignments(user_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_role_assign_team ON role_assignments(team_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_permissions_role ON permissions(role_id);

-- ============================================================
-- ENTITY TYPES REGISTRY
-- ============================================================

CREATE TABLE IF NOT EXISTS entity_types (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type_key        VARCHAR(128) NOT NULL UNIQUE,
    display_name    VARCHAR(256) NOT NULL,
    description     TEXT,
    plugin_name     VARCHAR(128),
    icon            VARCHAR(256),
    category        VARCHAR(64),
    metadata_schema JSONB NOT NULL DEFAULT '{}',
    ui_config       JSONB DEFAULT '{}',
    is_system       BOOLEAN DEFAULT FALSE,
    is_enabled      BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Core entity types
INSERT INTO entity_types (type_key, display_name, category, is_system) VALUES
    ('service',       'Service',          'compute',      TRUE),
    ('team',          'Team',             'organization', TRUE),
    ('environment',   'Environment',      'deployment',   TRUE),
    ('api_endpoint',  'API Endpoint',     'interface',    TRUE)
ON CONFLICT (type_key) DO NOTHING;

-- ============================================================
-- ENTITIES (Universal Entity Store)
-- ============================================================

CREATE TABLE IF NOT EXISTS entities (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type_id         UUID NOT NULL REFERENCES entity_types(id),
    type_key        VARCHAR(128) NOT NULL,
    name            VARCHAR(512) NOT NULL,
    description     TEXT,
    external_id     VARCHAR(512),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    metadata        JSONB NOT NULL DEFAULT '{}',
    status          VARCHAR(64) DEFAULT 'active',
    status_detail   TEXT,
    plugin_name     VARCHAR(128),
    sync_status     VARCHAR(32) DEFAULT 'synced',
    last_synced_at  TIMESTAMPTZ,
    embedding       vector(1536),
    created_by      UUID REFERENCES users(id),
    updated_by      UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (type_key, external_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_entities_type ON entities(type_key, tenant_id);
CREATE INDEX IF NOT EXISTS idx_entities_external ON entities(type_key, external_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_entities_metadata ON entities USING GIN(metadata);
CREATE INDEX IF NOT EXISTS idx_entities_status ON entities(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_entities_search ON entities USING GIN(to_tsvector('english', name || ' ' || COALESCE(description, '')));

-- Row-Level Security
ALTER TABLE entities ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON entities;
CREATE POLICY tenant_isolation ON entities
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- ============================================================
-- RELATIONSHIP TYPES & ENTITY RELATIONSHIPS
-- ============================================================

CREATE TABLE IF NOT EXISTS relationship_types (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type_key        VARCHAR(128) NOT NULL UNIQUE,
    display_name    VARCHAR(256) NOT NULL,
    description     TEXT,
    source_types    VARCHAR(128)[] NOT NULL,
    target_types    VARCHAR(128)[] NOT NULL,
    cardinality     VARCHAR(16) DEFAULT 'many_to_many',
    display_color   VARCHAR(7),
    display_label   VARCHAR(64),
    is_directional  BOOLEAN DEFAULT TRUE,
    plugin_name     VARCHAR(128),
    is_system       BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Core relationship types
INSERT INTO relationship_types (type_key, display_name, source_types, target_types, cardinality, display_color, is_directional, is_system) VALUES
    ('owns',           'Owned By',       ARRAY['*'],       ARRAY['team'],    'many_to_one',  '#3B82F6', TRUE,  TRUE),
    ('depends_on',     'Depends On',     ARRAY['*'],       ARRAY['*'],       'many_to_many', '#EF4444', TRUE,  TRUE),
    ('deployed_to',    'Deployed To',    ARRAY['service'], ARRAY['environment'], 'many_to_many', '#10B981', TRUE, TRUE),
    ('part_of',        'Part Of',        ARRAY['*'],       ARRAY['*'],       'many_to_one',  '#6366F1', TRUE,  TRUE)
ON CONFLICT (type_key) DO NOTHING;

CREATE TABLE IF NOT EXISTS entity_relationships (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_type_id UUID NOT NULL REFERENCES relationship_types(id),
    type_key            VARCHAR(128) NOT NULL,
    source_id           UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    target_id           UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    metadata            JSONB DEFAULT '{}',
    tenant_id           UUID NOT NULL,
    plugin_name         VARCHAR(128),
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (source_id, target_id, relationship_type_id),
    CHECK (source_id != target_id)
);

CREATE INDEX IF NOT EXISTS idx_rel_source ON entity_relationships(source_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_rel_target ON entity_relationships(target_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_rel_type ON entity_relationships(type_key, tenant_id);

ALTER TABLE entity_relationships ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_rel ON entity_relationships;
CREATE POLICY tenant_isolation_rel ON entity_relationships
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- ============================================================
-- PLUGINS
-- ============================================================

CREATE TABLE IF NOT EXISTS plugins (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(128) NOT NULL UNIQUE,
    version     VARCHAR(32) NOT NULL,
    type        VARCHAR(64) NOT NULL,
    status      VARCHAR(32) DEFAULT 'installed',
    config      JSONB DEFAULT '{}',
    enabled     BOOLEAN DEFAULT FALSE,
    tenant_id   UUID,
    installed_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS plugin_state (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plugin_name VARCHAR(128) NOT NULL,
    tenant_id   UUID NOT NULL,
    state_key   VARCHAR(256) NOT NULL,
    state_value JSONB NOT NULL,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (plugin_name, tenant_id, state_key)
);

-- ============================================================
-- WORKFLOWS
-- ============================================================

CREATE TABLE IF NOT EXISTS workflows (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(256) NOT NULL,
    tenant_id   UUID NOT NULL,
    spec        JSONB NOT NULL,
    version     INTEGER DEFAULT 1,
    source      VARCHAR(32) DEFAULT 'yaml',
    git_path    VARCHAR(512),
    is_enabled  BOOLEAN DEFAULT TRUE,
    is_locked   BOOLEAN DEFAULT FALSE,
    created_by  UUID,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (name, tenant_id)
);

CREATE TABLE IF NOT EXISTS workflow_executions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id     UUID NOT NULL REFERENCES workflows(id),
    tenant_id       UUID NOT NULL,
    trigger_type    VARCHAR(32),
    trigger_payload JSONB,
    triggered_by    UUID,
    status          VARCHAR(32) DEFAULT 'pending',
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    duration_ms     INTEGER,
    context         JSONB DEFAULT '{}',
    result          JSONB,
    error           TEXT,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS step_executions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id    UUID NOT NULL REFERENCES workflow_executions(id) ON DELETE CASCADE,
    step_name       VARCHAR(256) NOT NULL,
    step_type       VARCHAR(32),
    plugin_name     VARCHAR(128),
    action_name     VARCHAR(128),
    params          JSONB,
    status          VARCHAR(32) DEFAULT 'pending',
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    duration_ms     INTEGER,
    output          JSONB,
    error           TEXT,
    retry_count     INTEGER DEFAULT 0,
    approvers       JSONB,
    approvals       JSONB,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_exec_workflow ON workflow_executions(workflow_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_exec_status ON workflow_executions(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_step_exec ON step_executions(execution_id, step_name);

-- ============================================================
-- DASHBOARDS
-- ============================================================

CREATE TABLE IF NOT EXISTS dashboards (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    name        VARCHAR(256) NOT NULL,
    slug        VARCHAR(128) NOT NULL,
    description TEXT,
    owner_id    UUID NOT NULL REFERENCES users(id),
    is_public   BOOLEAN DEFAULT FALSE,
    shared_with JSONB DEFAULT '[]',
    layout      JSONB NOT NULL DEFAULT '{}',
    settings    JSONB DEFAULT '{}',
    template_id UUID,
    is_system   BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

CREATE TABLE IF NOT EXISTS dashboard_widgets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dashboard_id UUID NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
    widget_type  VARCHAR(64) NOT NULL,
    title        VARCHAR(256),
    config       JSONB NOT NULL DEFAULT '{}',
    position     JSONB DEFAULT '{}',
    data_source  JSONB NOT NULL,
    sort_order   INTEGER DEFAULT 0,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================
-- AI / RAG
-- ============================================================

CREATE TABLE IF NOT EXISTS rag_documents (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    source      VARCHAR(64) NOT NULL,
    source_type VARCHAR(128),
    source_id   VARCHAR(512),
    source_url  VARCHAR(1024),
    content     TEXT NOT NULL,
    metadata    JSONB DEFAULT '{}',
    ingested_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS rag_chunks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES rag_documents(id) ON DELETE CASCADE,
    tenant_id   UUID NOT NULL,
    content     TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,
    embedding   vector(1536),
    metadata    JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (document_id, chunk_index)
);

CREATE INDEX IF NOT EXISTS idx_rag_chunks_doc ON rag_chunks(document_id);
CREATE INDEX IF NOT EXISTS idx_rag_chunks_tenant ON rag_chunks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_rag_chunks_metadata ON rag_chunks USING GIN(metadata);
CREATE INDEX IF NOT EXISTS idx_rag_docs_source ON rag_documents(tenant_id, source, source_type);

ALTER TABLE rag_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE rag_chunks ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_rag_docs ON rag_documents;
CREATE POLICY tenant_rag_docs ON rag_documents
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

DROP POLICY IF EXISTS tenant_rag_chunks ON rag_chunks;
CREATE POLICY tenant_rag_chunks ON rag_chunks
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

CREATE TABLE IF NOT EXISTS ai_conversations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    user_id     UUID NOT NULL,
    title       VARCHAR(256),
    status      VARCHAR(32) DEFAULT 'active',
    metadata    JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ai_messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role            VARCHAR(32) NOT NULL,
    content         TEXT NOT NULL,
    tool_calls      JSONB,
    tool_call_id    VARCHAR(256),
    tokens_input    INTEGER,
    tokens_output   INTEGER,
    model_used      VARCHAR(128),
    provider_used   VARCHAR(64),
    cost_usd        DECIMAL(10, 6),
    citations       JSONB,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_conv_user ON ai_conversations(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_msg_conv ON ai_messages(conversation_id, created_at);

-- ============================================================
-- SCORECARDS
-- ============================================================

CREATE TABLE IF NOT EXISTS scorecards (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    name        VARCHAR(256) NOT NULL,
    description TEXT,
    enabled     BOOLEAN DEFAULT TRUE,
    config      JSONB DEFAULT '{}',
    created_by  UUID,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (name, tenant_id)
);

CREATE TABLE IF NOT EXISTS scorecard_rules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scorecard_id UUID NOT NULL REFERENCES scorecards(id) ON DELETE CASCADE,
    name         VARCHAR(256) NOT NULL,
    description  TEXT,
    expression   TEXT NOT NULL,
    weight       INTEGER DEFAULT 5,
    pass_message VARCHAR(512),
    fail_message VARCHAR(512),
    severity     VARCHAR(16) DEFAULT 'warning',
    metadata     JSONB DEFAULT '{}',
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS scorecard_results (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scorecard_id UUID NOT NULL REFERENCES scorecards(id) ON DELETE CASCADE,
    entity_id    UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    tenant_id    UUID NOT NULL,
    score        INTEGER DEFAULT 0,
    max_score    INTEGER DEFAULT 0,
    pass_count   INTEGER DEFAULT 0,
    fail_count   INTEGER DEFAULT 0,
    total_rules  INTEGER DEFAULT 0,
    level        VARCHAR(16) DEFAULT 'none',
    details      JSONB DEFAULT '[]',
    evaluated_at TIMESTAMPTZ DEFAULT NOW(),
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scorecard_tenant ON scorecards(tenant_id);
CREATE INDEX IF NOT EXISTS idx_scorecard_rules_sc ON scorecard_rules(scorecard_id);
CREATE INDEX IF NOT EXISTS idx_scorecard_result_entity ON scorecard_results(entity_id, scorecard_id);
CREATE INDEX IF NOT EXISTS idx_scorecard_result_tenant ON scorecard_results(tenant_id, evaluated_at DESC);

-- ============================================================
-- CONNECTIONS (unified service registry)
-- ============================================================
-- Stores configuration for all external services PEPA integrates with.
-- Deployment-agnostic: K8s clusters, GitLab, Jira, CI, AI providers, storage.
-- NOTE: Must be created before clusters/pipeline_sources which reference it.
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
    timeout_seconds     INTEGER DEFAULT 300,
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

-- ============================================================
-- INITIALIZATION COMPLETE
-- ============================================================
-- ============================================================
-- SERVICE TEMPLATES (Self-Service Portal)
-- ============================================================

CREATE TABLE IF NOT EXISTS service_templates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    name            VARCHAR(128) NOT NULL,
    slug            VARCHAR(128) NOT NULL,
    description     TEXT DEFAULT '',
    category        VARCHAR(64) DEFAULT 'general',
    icon            VARCHAR(256),
    language        VARCHAR(64),
    framework       VARCHAR(64),
    dockerfile_tmpl TEXT,
    helm_chart      JSONB DEFAULT '{}',
    cicd_tmpl       TEXT,
    default_values  JSONB DEFAULT '{}',
    resource_defaults JSONB DEFAULT '{"cpu":"100m","memory":"128Mi","replicas":1}',
    tags            VARCHAR(64)[] DEFAULT '{}',
    is_enabled      BOOLEAN DEFAULT TRUE,
    is_system       BOOLEAN DEFAULT FALSE,
    created_by      UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_svc_tmpl_tenant ON service_templates(tenant_id);
CREATE INDEX IF NOT EXISTS idx_svc_tmpl_category ON service_templates(category);

-- Seed default templates
INSERT INTO service_templates (tenant_id, name, slug, description, category, icon, language, framework, tags, is_system, resource_defaults, default_values) VALUES
    -- Backend
    ('00000000-0000-0000-0000-000000000002', 'Node.js API', 'nodejs-api', 'Node.js Express REST API with health checks and structured logging', 'backend', 'nodejs', 'javascript', 'express', ARRAY['nodejs','api','rest','express'], TRUE, '{"cpu":"200m","memory":"256Mi","replicas":2}', '{"server.port":"3000","node_env":"production"}'),
    ('00000000-0000-0000-0000-000000000002', 'Go API', 'go-api', 'Go Gin REST API with graceful shutdown and Prometheus metrics', 'backend', 'go', 'go', 'gin', ARRAY['go','api','rest','gin','prometheus'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":2}', '{"server.port":"8080","log_level":"info"}'),
    ('00000000-0000-0000-0000-000000000002', 'Python API', 'python-api', 'Python FastAPI service with async support and auto-generated docs', 'backend', 'python', 'python', 'fastapi', ARRAY['python','api','fastapi','async'], TRUE, '{"cpu":"200m","memory":"256Mi","replicas":2}', '{"server.port":"8000","workers":"4"}'),
    ('00000000-0000-0000-0000-000000000002', 'Java Spring Boot', 'java-spring', 'Java Spring Boot REST API with Actuator health endpoints', 'backend', 'java', 'java', 'spring-boot', ARRAY['java','spring','api','rest','maven'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":2}', '{"server.port":"8080","spring.profiles.active":"prod"}'),
    ('00000000-0000-0000-0000-000000000002', '.NET API', 'dotnet-api', 'ASP.NET Core Web API with Swagger and health checks', 'backend', 'dotnet', 'csharp', 'aspnet', ARRAY['dotnet','csharp','api','rest'], TRUE, '{"cpu":"300m","memory":"384Mi","replicas":2}', '{"ASPNETCORE_ENVIRONMENT":"Production","ASPNETCORE_URLS":"http://+:8080"}'),
    ('00000000-0000-0000-0000-000000000002', 'Ruby on Rails', 'ruby-rails', 'Ruby on Rails API mode with PostgreSQL adapter', 'backend', 'ruby', 'ruby', 'rails', ARRAY['ruby','rails','api','postgresql'], TRUE, '{"cpu":"300m","memory":"384Mi","replicas":2}', '{"rails_env":"production","rails_log_level":"info"}'),
    ('00000000-0000-0000-0000-000000000002', 'PHP Laravel', 'php-laravel', 'PHP Laravel API with Octane for high performance', 'backend', 'php', 'php', 'laravel', ARRAY['php','laravel','api','octane'], TRUE, '{"cpu":"300m","memory":"384Mi","replicas":2}', '{"app_env":"production","app_debug":"false"}'),
    ('00000000-0000-0000-0000-000000000002', 'Rust API', 'rust-api', 'Rust Axum API with Tokio runtime, low memory footprint', 'backend', 'rust', 'rust', 'axum', ARRAY['rust','api','axum','tokio'], TRUE, '{"cpu":"50m","memory":"64Mi","replicas":2}', '{"RUST_LOG":"info","server.port":"8080"}'),
    -- Frontend
    ('00000000-0000-0000-0000-000000000002', 'React SPA', 'react-spa', 'React single-page app with Vite build and Nginx serving', 'frontend', 'react', 'javascript', 'react', ARRAY['react','spa','vite','frontend'], TRUE, '{"cpu":"50m","memory":"64Mi","replicas":1}', '{"nginx.worker_connections":"1024"}'),
    ('00000000-0000-0000-0000-000000000002', 'Next.js App', 'nextjs-app', 'Next.js SSR/SSG application with API routes', 'frontend', 'nextjs', 'javascript', 'nextjs', ARRAY['nextjs','react','ssr','frontend'], TRUE, '{"cpu":"200m","memory":"256Mi","replicas":2}', '{"node_env":"production","port":"3000"}'),
    ('00000000-0000-0000-0000-000000000002', 'Vue.js App', 'vue-app', 'Vue 3 SPA with Vite and Vue Router', 'frontend', 'vue', 'javascript', 'vue', ARRAY['vue','spa','vite','frontend'], TRUE, '{"cpu":"50m","memory":"64Mi","replicas":1}', '{"node_env":"production"}'),
    ('00000000-0000-0000-0000-000000000002', 'Angular App', 'angular-app', 'Angular application with Angular CLI build', 'frontend', 'angular', 'typescript', 'angular', ARRAY['angular','spa','typescript','frontend'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{"node_env":"production"}'),
    ('00000000-0000-0000-0000-000000000002', 'Static Site', 'static-site', 'Static website (HTML/CSS/JS) served by Nginx', 'frontend', 'nginx', 'html', 'none', ARRAY['static','website','frontend','nginx'], TRUE, '{"cpu":"50m","memory":"64Mi","replicas":1}', '{"nginx.worker_connections":"1024"}'),
    -- Data & Databases
    ('00000000-0000-0000-0000-000000000002', 'PostgreSQL', 'postgresql', 'PostgreSQL relational database with persistent storage', 'data', 'postgresql', 'any', 'postgresql', ARRAY['database','sql','postgresql','storage'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":1}', '{"postgres.max_connections":"100","postgres.shared_buffers":"128MB"}'),
    ('00000000-0000-0000-0000-000000000002', 'Redis', 'redis', 'Redis in-memory cache and message broker', 'data', 'redis', 'any', 'redis', ARRAY['cache','redis','in-memory','broker'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{"redis.maxmemory":"128mb","redis.maxmemory_policy":"allkeys-lru"}'),
    ('00000000-0000-0000-0000-000000000002', 'MongoDB', 'mongodb', 'MongoDB document database with replica set support', 'data', 'mongodb', 'any', 'mongodb', ARRAY['database','nosql','mongodb','document'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":1}', '{"mongodb.wiredTiger.cacheSize":"256m"}'),
    ('00000000-0000-0000-0000-000000000002', 'MySQL', 'mysql', 'MySQL relational database with InnoDB engine', 'data', 'mysql', 'any', 'mysql', ARRAY['database','sql','mysql','innodb'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":1}', '{"mysql.max_connections":"200","mysql.innodb_buffer_pool_size":"256M"}'),
    ('00000000-0000-0000-0000-000000000002', 'Elasticsearch', 'elasticsearch', 'Elasticsearch search and analytics engine', 'data', 'elasticsearch', 'java', 'elasticsearch', ARRAY['search','elasticsearch','analytics','logging'], TRUE, '{"cpu":"1000m","memory":"1Gi","replicas":1}', '{"cluster.name":"elasticsearch","xpack.security.enabled":"false"}'),
    -- Infrastructure
    ('00000000-0000-0000-0000-000000000002', 'Nginx Proxy', 'nginx-proxy', 'Nginx reverse proxy / load balancer with custom config', 'infrastructure', 'nginx', 'any', 'nginx', ARRAY['nginx','proxy','loadbalancer','reverse-proxy'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":2}', '{"nginx.worker_connections":"4096","nginx.worker_processes":"auto"}'),
    ('00000000-0000-0000-0000-000000000002', 'Traefik', 'traefik', 'Traefik edge router with auto-discovery and Let''s Encrypt', 'infrastructure', 'traefik', 'go', 'traefik', ARRAY['traefik','proxy','ingress','letsencrypt'], TRUE, '{"cpu":"200m","memory":"256Mi","replicas":1}', '{"traefik.log.level":"INFO","traefik.entrypoints.web.port":"80"}'),
    ('00000000-0000-0000-0000-000000000002', 'Prometheus', 'prometheus', 'Prometheus monitoring with alerting and service discovery', 'infrastructure', 'prometheus', 'go', 'prometheus', ARRAY['monitoring','prometheus','metrics','alerting'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":1}', '{"prometheus.retention":"15d","prometheus.scrape_interval":"15s"}'),
    ('00000000-0000-0000-0000-000000000002', 'Grafana', 'grafana', 'Grafana dashboards for visualization and alerting', 'infrastructure', 'grafana', 'go', 'grafana', ARRAY['monitoring','grafana','dashboards','visualization'], TRUE, '{"cpu":"200m","memory":"256Mi","replicas":1}', '{"grafana.server.root_url":"http://grafana.local"}'),
    -- Messaging & Streaming
    ('00000000-0000-0000-0000-000000000002', 'RabbitMQ', 'rabbitmq', 'RabbitMQ message broker with management UI', 'messaging', 'rabbitmq', 'erlang', 'rabbitmq', ARRAY['messaging','rabbitmq','amqp','queue'], TRUE, '{"cpu":"300m","memory":"384Mi","replicas":1}', '{"rabbitmq.default_vhost":"/","rabbitmq.erlang_cookie":"secret"}'),
    ('00000000-0000-0000-0000-000000000002', 'Kafka', 'kafka', 'Apache Kafka distributed event streaming platform', 'messaging', 'kafka', 'java', 'kafka', ARRAY['messaging','kafka','streaming','events'], TRUE, '{"cpu":"500m","memory":"1Gi","replicas":3}', '{"kafka.log.retention.hours":"168","kafka.num.partitions":"3"}'),
    ('00000000-0000-0000-0000-000000000002', 'NATS', 'nats', 'NATS lightweight cloud-native messaging system', 'messaging', 'nats', 'go', 'nats', ARRAY['messaging','nats','cloud-native','events'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{"nats.port":"4222","nats.max_payload":"1MB"}'),
    -- ML & Data Science
    ('00000000-0000-0000-0000-000000000002', 'Jupyter Notebook', 'jupyter', 'Jupyter Notebook server for data science and ML experiments', 'ml', 'jupyter', 'python', 'jupyter', ARRAY['jupyter','notebook','ml','datascience'], TRUE, '{"cpu":"500m","memory":"1Gi","replicas":1}', '{"jupyter.token":"change-me","jupyter.port":"8888"}'),
    ('00000000-0000-0000-0000-000000000002', 'MLflow', 'mlflow', 'MLflow experiment tracking and model registry', 'ml', 'mlflow', 'python', 'mlflow', ARRAY['mlflow','ml','tracking','model-registry'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":1}', '{"mlflow.backend_store":"postgresql","mlflow.default_artifact_root":"s3://mlflow"}'),
    -- CI/CD & DevOps
    ('00000000-0000-0000-0000-000000000002', 'GitLab Runner', 'gitlab-runner', 'GitLab CI runner for executing CI/CD pipelines', 'devops', 'gitlab', 'go', 'gitlab', ARRAY['ci','cd','gitlab','runner','pipeline'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":1}', '{"runner.concurrent":"10","runner.executor":"kubernetes"}'),
    ('00000000-0000-0000-0000-000000000002', 'SonarQube', 'sonarqube', 'SonarQube code quality and security analysis', 'devops', 'sonarqube', 'java', 'sonarqube', ARRAY['code-quality','sonarqube','security','analysis'], TRUE, '{"cpu":"1000m","memory":"2Gi","replicas":1}', '{"sonar.web.port":"9000","sonar.search.javaOpts":"-Xmx512m"}'),
    -- Import / Custom
    ('00000000-0000-0000-0000-000000000002', 'Helm Chart Import', 'helm-import', 'Import and deploy an existing Helm chart from any source', 'import', 'helm', 'any', 'helm', ARRAY['helm','import','chart'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{}'),
    ('00000000-0000-0000-0000-000000000002', 'Docker Compose Import', 'docker-compose-import', 'Import a docker-compose.yml and deploy to a Docker host', 'import', 'docker', 'any', 'docker-compose', ARRAY['docker','compose','import'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{}'),
    ('00000000-0000-0000-0000-000000000002', 'Custom Container', 'custom-container', 'Deploy any Docker container image with custom settings', 'import', 'docker', 'any', 'none', ARRAY['custom','container','docker','image'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{}'),
    ('00000000-0000-0000-0000-000000000002', 'Blank Service', 'blank', 'Start from scratch — define everything yourself', 'import', 'storage', 'any', 'none', ARRAY['blank','custom','empty'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{}')
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Add public Helm charts and Docker images to templates (latest stable releases — Aug 2026)
UPDATE service_templates SET helm_chart = '{"image":"bitnami/node:24","docs_url":"https://hub.docker.com/r/bitnami/node"}' WHERE slug = 'nodejs-api';
UPDATE service_templates SET helm_chart = '{"image":"golang:1.26-alpine","docs_url":"https://hub.docker.com/_/golang"}' WHERE slug = 'go-api';
UPDATE service_templates SET helm_chart = '{"image":"python:3.14-slim","docs_url":"https://hub.docker.com/_/python"}' WHERE slug = 'python-api';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"spring-boot","chart_version":"4.0.0","image":"bitnami/spring-boot:latest","docs_url":"https://artifacthub.io/packages/helm/bitnami/spring-boot"}' WHERE slug = 'java-spring';
UPDATE service_templates SET helm_chart = '{"image":"mcr.microsoft.com/dotnet/aspnet:10.0","docs_url":"https://hub.docker.com/_/microsoft-dotnet-aspnet"}' WHERE slug = 'dotnet-api';
UPDATE service_templates SET helm_chart = '{"image":"ruby:4.0-slim","docs_url":"https://hub.docker.com/_/ruby"}' WHERE slug = 'ruby-rails';
UPDATE service_templates SET helm_chart = '{"image":"php:8.5-fpm","docs_url":"https://hub.docker.com/_/php"}' WHERE slug = 'php-laravel';
UPDATE service_templates SET helm_chart = '{"image":"rust:1.96-slim","docs_url":"https://hub.docker.com/_/rust"}' WHERE slug = 'rust-api';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"nginx","chart_version":"19.0.0","image":"bitnami/nginx:1.30","docs_url":"https://artifacthub.io/packages/helm/bitnami/nginx"}' WHERE slug IN ('react-spa','vue-app','angular-app','static-site','nginx-proxy');
UPDATE service_templates SET helm_chart = '{"image":"node:24-alpine","docs_url":"https://hub.docker.com/_/node"}' WHERE slug = 'nextjs-app';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"postgresql","chart_version":"14.0.0","image":"bitnami/postgresql:18","docs_url":"https://artifacthub.io/packages/helm/bitnami/postgresql"}' WHERE slug = 'postgresql';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"redis","chart_version":"20.0.0","image":"bitnami/redis:8.4","docs_url":"https://artifacthub.io/packages/helm/bitnami/redis"}' WHERE slug = 'redis';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"mongodb","chart_version":"16.0.0","image":"bitnami/mongodb:8.3","docs_url":"https://artifacthub.io/packages/helm/bitnami/mongodb"}' WHERE slug = 'mongodb';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"mysql","chart_version":"12.0.0","image":"bitnami/mysql:8.4","docs_url":"https://artifacthub.io/packages/helm/bitnami/mysql"}' WHERE slug = 'mysql';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://helm.elastic.co","chart_name":"elasticsearch","chart_version":"9.5.1","image":"docker.elastic.co/elasticsearch/elasticsearch:9.5.1","docs_url":"https://artifacthub.io/packages/helm/elastic/elasticsearch"}' WHERE slug = 'elasticsearch';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://helm.traefik.io/traefik","chart_name":"traefik","chart_version":"33.1.0","image":"traefik:v3.7","docs_url":"https://artifacthub.io/packages/helm/traefik/traefik"}' WHERE slug = 'traefik';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://prometheus-community.github.io/helm-charts","chart_name":"prometheus","chart_version":"27.0.0","image":"prom/prometheus:v3.13.1","docs_url":"https://artifacthub.io/packages/helm/prometheus-community/prometheus"}' WHERE slug = 'prometheus';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://grafana.github.io/helm-charts","chart_name":"grafana","chart_version":"9.0.0","image":"grafana/grafana:13.1.3","docs_url":"https://artifacthub.io/packages/helm/grafana/grafana"}' WHERE slug = 'grafana';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"rabbitmq","chart_version":"15.0.0","image":"bitnami/rabbitmq:4.3","docs_url":"https://artifacthub.io/packages/helm/bitnami/rabbitmq"}' WHERE slug = 'rabbitmq';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"kafka","chart_version":"31.0.0","image":"bitnami/kafka:4.3","docs_url":"https://artifacthub.io/packages/helm/bitnami/kafka"}' WHERE slug = 'kafka';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://nats-io.github.io/k8s/helm/charts","chart_name":"nats","chart_version":"1.4.0","image":"nats:2.14","docs_url":"https://artifacthub.io/packages/helm/nats/nats"}' WHERE slug = 'nats';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://jupyterhub.github.io/helm-chart","chart_name":"jupyterhub","chart_version":"4.0.0","image":"jupyterhub/k8s-singleuser-sample:4.0.0","docs_url":"https://z2jh.jupyter.org"}' WHERE slug = 'jupyter';
UPDATE service_templates SET helm_chart = '{"image":"ghcr.io/mlflow/mlflow:v3.15.1","docs_url":"https://mlflow.org/docs/latest/index.html"}' WHERE slug = 'mlflow';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.gitlab.io","chart_name":"gitlab-runner","chart_version":"0.91.0","image":"gitlab/gitlab-runner:alpine","docs_url":"https://artifacthub.io/packages/helm/gitlab/gitlab-runner"}' WHERE slug = 'gitlab-runner';
UPDATE service_templates SET helm_chart = '{"image":"sonarqube:2026-lts-community","docs_url":"https://hub.docker.com/_/sonarqube"}' WHERE slug = 'sonarqube';

-- ============================================================
-- SERVICES (created from templates)
-- ============================================================

CREATE TABLE IF NOT EXISTS services (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    template_id     UUID REFERENCES service_templates(id),
    name            VARCHAR(128) NOT NULL,
    slug            VARCHAR(128) NOT NULL,
    description     TEXT DEFAULT '',
    owner_team_id   UUID REFERENCES teams(id),
    language        VARCHAR(64),
    framework       VARCHAR(64),
    gitlab_project_url VARCHAR(512),
    helm_chart_url  VARCHAR(512),
    image_repository VARCHAR(256),
    namespace       VARCHAR(63) DEFAULT 'default',
    status          VARCHAR(32) DEFAULT 'creating',
    resource_config JSONB DEFAULT '{}',
    environment_variables JSONB DEFAULT '{}',
    vault_secrets   JSONB DEFAULT '{}',
    deployment_strategy VARCHAR(32) DEFAULT 'rolling',
    target_clusters UUID[] DEFAULT '{}',
    metadata        JSONB DEFAULT '{}',
    created_by      UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_services_tenant ON services(tenant_id);
CREATE INDEX IF NOT EXISTS idx_services_status ON services(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_services_template ON services(template_id);

-- ============================================================
-- SERVICE DEPLOYMENT STAGES (tracking dev→testing→staging→prod)
-- ============================================================

CREATE TABLE IF NOT EXISTS service_deployments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    service_id      UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    environment     VARCHAR(32) NOT NULL,
    cluster_id      UUID REFERENCES clusters(id),
    namespace       VARCHAR(63),
    branch          VARCHAR(128),
    image_tag       VARCHAR(128),
    helm_release    VARCHAR(128),
    deploy_type     VARCHAR(32) DEFAULT 'automatic',
    status          VARCHAR(32) DEFAULT 'pending',
    verification_status VARCHAR(32) DEFAULT 'pending',
    verification_details JSONB DEFAULT '{}',
    flux_synced     BOOLEAN DEFAULT FALSE,
    pods_ready      INTEGER DEFAULT 0,
    pods_total      INTEGER DEFAULT 0,
    mr_url          VARCHAR(512),
    pipeline_url    VARCHAR(512),
    deployed_at     TIMESTAMPTZ,
    verified_at     TIMESTAMPTZ,
    promoted_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_svc_deploy_tenant ON service_deployments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_svc_deploy_service ON service_deployments(service_id);
CREATE INDEX IF NOT EXISTS idx_svc_deploy_env ON service_deployments(environment);

-- ============================================================
-- SEED: DEFAULT SCORECARD — Production Readiness
-- ============================================================

INSERT INTO scorecards (id, tenant_id, name, description, enabled, config)
VALUES (
    '10000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000002',
    'Production Readiness',
    'Evaluates whether a service meets production readiness criteria based on CNCF best practices.',
    TRUE,
    '{"levels": ["bronze", "silver", "gold", "platinum"], "thresholds": {"bronze": 25, "silver": 50, "gold": 75, "platinum": 90}}'
) ON CONFLICT (name, tenant_id) DO NOTHING;

INSERT INTO scorecard_rules (scorecard_id, name, description, expression, weight, pass_message, fail_message, severity) VALUES
    ('10000000-0000-0000-0000-000000000001', 'Has Health Endpoint', 'Service exposes a /healthz or /health endpoint', 'metadata.health_endpoint == true', 8, 'Health endpoint configured', 'Missing health endpoint — required for liveness probes', 'error'),
    ('10000000-0000-0000-0000-000000000001', 'Has Readiness Endpoint', 'Service exposes a /readyz or /ready endpoint', 'metadata.readiness_endpoint == true', 8, 'Readiness endpoint configured', 'Missing readiness endpoint — required for traffic routing', 'error'),
    ('10000000-0000-0000-0000-000000000001', 'Has Owner Team', 'Service has an owning team assigned', 'owner_team_id != null', 5, 'Owner team assigned', 'No owner team — every service needs a responsible team', 'warning'),
    ('10000000-0000-0000-0000-000000000001', 'Has Description', 'Service has a meaningful description', 'description != ""', 3, 'Description provided', 'Missing service description', 'info'),
    ('10000000-0000-0000-0000-000000000001', 'Resource Limits Set', 'CPU and memory limits are defined', 'resource_config.cpu != null && resource_config.memory != null', 7, 'Resource limits configured', 'Missing resource limits — can cause noisy-neighbor issues', 'warning'),
    ('10000000-0000-0000-0000-000000000001', 'Replicas >= 2', 'Service runs at least 2 replicas for HA', 'resource_config.replicas >= 2', 6, 'Multiple replicas configured', 'Single replica — not resilient to failures', 'warning'),
    ('10000000-0000-0000-0000-000000000001', 'Has GitLab Project', 'Service is linked to a GitLab project', 'gitlab_project_url != null && gitlab_project_url != ""', 4, 'GitLab project linked', 'No GitLab project URL set', 'info'),
    ('10000000-0000-0000-0000-000000000001', 'Has Helm Chart', 'Service has an associated Helm chart', 'helm_chart_url != null && helm_chart_url != ""', 5, 'Helm chart configured', 'No Helm chart — GitOps deployment requires a chart', 'warning'),
    ('10000000-0000-0000-0000-000000000001', 'Deployment Strategy Set', 'A deployment strategy is defined', 'deployment_strategy != null && deployment_strategy != ""', 4, 'Deployment strategy defined', 'No deployment strategy — consider rolling or canary', 'info'),
    ('10000000-0000-0000-0000-000000000001', 'Environment Variables Documented', 'Service has environment variables configured', 'environment_variables != null', 2, 'Environment variables present', 'No environment variables defined', 'info')
ON CONFLICT DO NOTHING;

-- ============================================================
-- SEED: DEFAULT RBAC ROLES
-- ============================================================

INSERT INTO roles (id, tenant_id, name, slug, description, is_system, scope) VALUES
    ('20000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000002', 'Platform Admin', 'admin', 'Full access to all platform resources', TRUE, 'tenant'),
    ('20000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000002', 'Developer', 'developer', 'Can manage services, workflows, and deployments', TRUE, 'tenant'),
    ('20000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000002', 'Viewer', 'viewer', 'Read-only access to all resources', TRUE, 'tenant')
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Admin permissions: full access
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000001', r, a, 'allow'
FROM unnest(ARRAY['entities','services','deployments','workflows','clusters','connections','scorecards','plugins','roles','audit','settings','policies','vault','pipelines','gitops','docker','helm','environments','discovery','import','ai','jira','credentials']) AS r
CROSS JOIN unnest(ARRAY['create','read','update','delete']) AS a
ON CONFLICT DO NOTHING;

-- Developer permissions
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000002', r, a, 'allow'
FROM unnest(ARRAY['entities','services','deployments','workflows','clusters','connections','scorecards','pipelines','gitops','docker','helm','discovery','ai']) AS r
CROSS JOIN unnest(ARRAY['create','read','update']) AS a
ON CONFLICT DO NOTHING;

INSERT INTO permissions (role_id, resource, action, effect) VALUES
    ('20000000-0000-0000-0000-000000000002', 'audit', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'policies', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'vault', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'vault', 'create', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'vault', 'delete', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'environments', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'credentials', 'create', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'credentials', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'credentials', 'delete', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'import', 'create', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'import', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'import', 'update', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'jira', 'create', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'jira', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'jira', 'update', 'allow')
ON CONFLICT DO NOTHING;

-- Viewer permissions: read-only
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000003', r, 'read', 'allow'
FROM unnest(ARRAY['entities','services','deployments','workflows','clusters','connections','scorecards','plugins','audit','policies','pipelines','gitops','docker','helm','environments','discovery','ai','credentials']) AS r
ON CONFLICT DO NOTHING;

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
    type        VARCHAR(32) DEFAULT '',
    cluster     VARCHAR(128) DEFAULT '',
    namespace   VARCHAR(63) DEFAULT '',
    status      VARCHAR(32) DEFAULT 'active',
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
    owner_id       UUID REFERENCES users(id) ON DELETE SET NULL,
    created_by     VARCHAR(255),
    created_at     TIMESTAMPTZ DEFAULT NOW(),
    updated_at     TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_vault_secrets_tenant ON vault_secrets (tenant_id);
CREATE INDEX IF NOT EXISTS idx_vault_secrets_owner ON vault_secrets(owner_id);

-- Policies are managed in-memory by the policy handler.
-- The handler seeds default policies on startup.

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
-- PIPELINE SOURCES, PRESETS, RUNS & JOBS
-- ============================================================

CREATE TABLE IF NOT EXISTS pipeline_sources (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL,
    name              VARCHAR(256) NOT NULL,
    source_type       VARCHAR(32) NOT NULL,
    description       TEXT DEFAULT '',
    connection_id     UUID REFERENCES connections(id) ON DELETE SET NULL,
    config            JSONB NOT NULL DEFAULT '{}',
    parameter_schema  JSONB DEFAULT '{}',
    schema_fetched_at TIMESTAMPTZ,
    status            VARCHAR(32) DEFAULT 'active',
    last_error        TEXT,
    created_by        UUID,
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    updated_at        TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pipelinesource_tenant ON pipeline_sources(tenant_id);
CREATE INDEX IF NOT EXISTS idx_pipelinesource_type ON pipeline_sources(source_type);

CREATE TABLE IF NOT EXISTS pipeline_presets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    source_id    UUID NOT NULL REFERENCES pipeline_sources(id) ON DELETE CASCADE,
    name         VARCHAR(256) NOT NULL,
    description  TEXT DEFAULT '',
    parameters   JSONB NOT NULL DEFAULT '{}',
    created_by   UUID,
    use_count    INTEGER DEFAULT 0,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (source_id, name)
);

CREATE INDEX IF NOT EXISTS idx_presets_source ON pipeline_presets(source_id);

CREATE TABLE IF NOT EXISTS pipeline_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    source_id       UUID NOT NULL REFERENCES pipeline_sources(id) ON DELETE CASCADE,
    preset_id       UUID REFERENCES pipeline_presets(id) ON DELETE SET NULL,
    external_run_id VARCHAR(256),
    external_url    TEXT,
    parameters      JSONB NOT NULL DEFAULT '{}',
    status          VARCHAR(32) DEFAULT 'pending',
    external_status VARCHAR(64),
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    duration_ms     INTEGER,
    logs            TEXT,
    logs_url        TEXT,
    job_details     JSONB,
    triggered_by    UUID,
    trigger_type    VARCHAR(32) DEFAULT 'manual',
    error_message   TEXT,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pipelinerun_source ON pipeline_runs(source_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pipelinerun_status ON pipeline_runs(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_pipelinerun_external ON pipeline_runs(external_run_id);

CREATE TABLE IF NOT EXISTS pipeline_run_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id          UUID NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
    external_job_id VARCHAR(256),
    name            VARCHAR(256) NOT NULL,
    stage           VARCHAR(128),
    status          VARCHAR(32) DEFAULT 'pending',
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    duration_ms     INTEGER,
    log_text        TEXT,
    log_url         TEXT,
    runner_name     VARCHAR(256),
    allow_failure   BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_runjob_run ON pipeline_run_jobs(run_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_runjob_run_external ON pipeline_run_jobs(run_id, external_job_id)
    WHERE external_job_id IS NOT NULL AND external_job_id != '';

-- ============================================================
-- GITOPS REPOSITORIES (migration 014)
-- ============================================================

CREATE TABLE IF NOT EXISTS gitops_repositories (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    name          TEXT NOT NULL,
    connection_id UUID REFERENCES connections(id) ON DELETE SET NULL,
    repo_url      TEXT NOT NULL,
    branch        TEXT NOT NULL DEFAULT 'main',
    path          TEXT NOT NULL DEFAULT '.',
    engine_type   TEXT NOT NULL DEFAULT 'auto',
    scan_status   TEXT NOT NULL DEFAULT 'pending',
    scan_error    TEXT,
    last_scanned_at TIMESTAMPTZ,
    config        JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gitops_repos_tenant ON gitops_repositories(tenant_id);

-- ============================================================
-- JIRA AUTOMATION (migration 016)
-- ============================================================

CREATE TABLE IF NOT EXISTS jira_automation_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    trigger_type TEXT NOT NULL,
    jira_project_key TEXT DEFAULT '',
    jql_filter TEXT DEFAULT '',
    action_type TEXT NOT NULL,
    action_config JSONB NOT NULL DEFAULT '{}',
    enabled BOOLEAN DEFAULT true,
    last_triggered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_jira_automation_rules_tenant ON jira_automation_rules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_jira_automation_rules_trigger ON jira_automation_rules(trigger_type);

CREATE TABLE IF NOT EXISTS jira_comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    issue_key TEXT NOT NULL,
    comment_id TEXT DEFAULT '',
    author TEXT DEFAULT '',
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(tenant_id, issue_key, comment_id)
);

CREATE INDEX IF NOT EXISTS idx_jira_comments_issue ON jira_comments(tenant_id, issue_key);

-- ============================================================
-- USER CREDENTIALS (migration 017)
-- ============================================================

CREATE TABLE IF NOT EXISTS user_credentials (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id     UUID NOT NULL,
    provider      VARCHAR(64) NOT NULL,
    provider_url  VARCHAR(512),
    display_name  VARCHAR(128),
    token_enc     TEXT NOT NULL,
    username      VARCHAR(256),
    email         VARCHAR(256),
    is_default    BOOLEAN DEFAULT FALSE,
    last_verified TIMESTAMPTZ,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    updated_at    TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (user_id, provider, provider_url)
);

CREATE INDEX IF NOT EXISTS idx_user_credentials_user ON user_credentials(user_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_user_credentials_provider ON user_credentials(provider, provider_url);

-- ============================================================
-- ENVIRONMENT VARIABLES (migration 018)
-- ============================================================

CREATE TABLE IF NOT EXISTS environment_variables (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID NOT NULL,
    env_id     UUID NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    key        VARCHAR(256) NOT NULL,
    value      TEXT DEFAULT '',
    is_secret  BOOLEAN DEFAULT FALSE,
    source     VARCHAR(64) DEFAULT 'manual',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (env_id, key)
);

CREATE INDEX IF NOT EXISTS idx_env_vars_env_id ON environment_variables(env_id);
CREATE INDEX IF NOT EXISTS idx_env_vars_tenant ON environment_variables(tenant_id);

-- ============================================================
-- PERFORMANCE INDEXES (migration 019)
-- ============================================================

CREATE INDEX IF NOT EXISTS idx_clusters_tenant_active ON clusters(tenant_id, is_active);
CREATE INDEX IF NOT EXISTS idx_connections_tenant_type ON connections(tenant_id, type);
CREATE INDEX IF NOT EXISTS idx_services_tenant_status ON services(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_deployments_created ON deployments(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_tenant_created ON audit_log(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_deployments_tenant_status ON deployments(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_tenant_created ON pipeline_runs(tenant_id, created_at DESC);

-- ============================================================
-- ROW-LEVEL SECURITY (RLS) for tenant-scoped tables
-- ============================================================

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

ALTER TABLE gitops_repositories ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_gitops_repositories ON gitops_repositories;
CREATE POLICY tenant_isolation_gitops_repositories ON gitops_repositories
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE jira_automation_rules ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_jira_automation_rules ON jira_automation_rules;
CREATE POLICY tenant_isolation_jira_automation_rules ON jira_automation_rules
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE jira_comments ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_jira_comments ON jira_comments;
CREATE POLICY tenant_isolation_jira_comments ON jira_comments
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE user_credentials ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_user_credentials ON user_credentials;
CREATE POLICY tenant_isolation_user_credentials ON user_credentials
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

ALTER TABLE environment_variables ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_environment_variables ON environment_variables;
CREATE POLICY tenant_isolation_environment_variables ON environment_variables
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  TIMESTAMPTZ DEFAULT NOW(),
    description TEXT
);

-- Bootstrap tokens for first-run setup
CREATE TABLE IF NOT EXISTS bootstrap_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash  VARCHAR(256) NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Service blueprints for pipeline builder
CREATE TABLE IF NOT EXISTS service_blueprints (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(256) NOT NULL,
    description     TEXT DEFAULT '',
    source_type     VARCHAR(32) NOT NULL DEFAULT 'container',
    helm_repo_id    UUID,
    image           TEXT DEFAULT '',
    chart_url       TEXT DEFAULT '',
    chart_name      VARCHAR(256) DEFAULT '',
    chart_version   VARCHAR(64) DEFAULT '',
    chart_path      TEXT DEFAULT '',
    namespace       VARCHAR(256) DEFAULT 'default',
    values_yaml     TEXT DEFAULT '',
    cpu             VARCHAR(32) DEFAULT '100m',
    memory          VARCHAR(32) DEFAULT '128Mi',
    replicas        INTEGER DEFAULT 1,
    ports           INTEGER[] DEFAULT '{}',
    category        VARCHAR(64) DEFAULT 'general',
    created_by      UUID,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_service_blueprints_category ON service_blueprints(category);
CREATE INDEX IF NOT EXISTS idx_service_blueprints_source_type ON service_blueprints(source_type);

-- ============================================================
-- VAULT ACL (per-path access control)
-- ============================================================

CREATE TABLE IF NOT EXISTS vault_acl (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    path_prefix VARCHAR(512) NOT NULL,
    user_id     UUID REFERENCES users(id) ON DELETE CASCADE,
    team_id     UUID REFERENCES teams(id) ON DELETE CASCADE,
    can_read    BOOLEAN DEFAULT TRUE,
    can_create  BOOLEAN DEFAULT FALSE,
    can_delete  BOOLEAN DEFAULT FALSE,
    created_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT vault_acl_target CHECK (user_id IS NOT NULL OR team_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_vault_acl_tenant_path ON vault_acl(tenant_id, path_prefix);
CREATE INDEX IF NOT EXISTS idx_vault_acl_user ON vault_acl(user_id);
CREATE INDEX IF NOT EXISTS idx_vault_acl_team ON vault_acl(team_id);
CREATE INDEX IF NOT EXISTS idx_vault_acl_created_by ON vault_acl(created_by);

-- ============================================================
-- CREDENTIAL SHARING
-- ============================================================

CREATE TABLE IF NOT EXISTS credential_shares (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    credential_id       UUID NOT NULL REFERENCES user_credentials(id) ON DELETE CASCADE,
    owner_user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id           UUID NOT NULL,
    shared_with_user    UUID REFERENCES users(id) ON DELETE CASCADE,
    shared_with_team    UUID REFERENCES teams(id) ON DELETE CASCADE,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT credential_shares_target CHECK (shared_with_user IS NOT NULL OR shared_with_team IS NOT NULL),
    CONSTRAINT credential_shares_unique UNIQUE (credential_id, shared_with_user, shared_with_team)
);

CREATE INDEX IF NOT EXISTS idx_cred_shares_credential ON credential_shares(credential_id);
CREATE INDEX IF NOT EXISTS idx_cred_shares_user ON credential_shares(shared_with_user);
CREATE INDEX IF NOT EXISTS idx_cred_shares_team ON credential_shares(shared_with_team);

-- Default admin user (password must be set via bootstrap activation)
INSERT INTO users (id, email, name, auth_provider, password_hash, is_active, must_change_password)
VALUES (
    '00000000-0000-0000-0000-000000000010',
    'admin@local',
    'Administrator',
    'local',
    '',
    true,
    true
)
ON CONFLICT (id) DO NOTHING;

-- Assign admin role to default admin user
INSERT INTO role_assignments (id, tenant_id, user_id, role_id, is_active)
SELECT
    '00000000-0000-0000-0000-000000000011',
    '00000000-0000-0000-0000-000000000002',
    '00000000-0000-0000-0000-000000000010',
    r.id,
    true
FROM roles r
WHERE r.tenant_id = '00000000-0000-0000-0000-000000000002'
  AND r.slug = 'admin'
  AND NOT EXISTS (
    SELECT 1 FROM role_assignments ra
    WHERE ra.user_id = '00000000-0000-0000-0000-000000000010'
      AND ra.role_id = r.id
  );

INSERT INTO schema_migrations (version, description) VALUES
    (1, 'Initial schema — entities, relationships, RBAC, workflows, RAG, audit'),
    (2, 'Add plugins, plugin_state, workflows, workflow_executions, step_executions'),
    (3, 'Add dashboards, dashboard_widgets, rag_documents, rag_chunks, ai_conversations, ai_messages'),
    (4, 'Add scorecards, scorecard_rules, scorecard_results'),
    (5, 'Add connections, clusters, flux_resources, deployments, jira_issues, incident_knowledge_base, audit_log'),
    (6, 'Add service_templates, services, service_deployments + seed scorecard and RBAC roles'),
    (7, 'Add settings, environments, vault_secrets, docker_hosts, docker_services, helm_repositories'),
    (8, 'Add pipeline_sources, pipeline_presets, pipeline_runs, pipeline_run_jobs'),
    (9, 'Fix pipeline_runs FK to cascade on delete from pipeline_sources'),
    (10, 'Vault security hardening — tenant isolation and audit tracking'),
    (11, 'Add unique partial index on pipeline_run_jobs for upsert support'),
    (12, 'Add error_message and logs to deployments for debugging'),
    (13, 'Enable RLS on all tenant-scoped tables + unique constraint on permissions'),
    (14, 'Gitops repositories rework'),
    (15, 'Deployment timeout support'),
    (16, 'Jira automation'),
    (17, 'Auth system — local users, JWT, RBAC enforcement'),
    (18, 'Enhanced environments'),
    (19, 'Performance indexes'),
    (20, 'Fix vault RBAC'),
    (21, 'Bootstrap token for first-run setup'),
    (22, 'Service blueprints for pipeline builder'),
    (23, 'Enable RLS on remaining tenant-scoped tables'),
    (24, 'RBAC permissions'),
    (25, 'Fix bootstrap must_change_password'),
    (26, 'Users token version'),
    (27, 'GitOps workflow chain'),
    (28, 'Proxmox virtualization'),
    (29, 'Vault ACL and credential sharing')
ON CONFLICT DO NOTHING;
