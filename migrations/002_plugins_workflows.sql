-- ============================================================
-- Migration 002: Plugins & Workflows
-- ============================================================
-- Plugin registry, plugin state, workflow engine,
-- workflow executions, and step executions.
-- ============================================================

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

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (2, 'Add plugins, plugin_state, workflows, workflow_executions, step_executions')
ON CONFLICT DO NOTHING;
