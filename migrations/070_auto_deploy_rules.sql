-- Migration 070: Auto-Deploy Rules
-- Stores branch-pattern → environment mappings for webhook-triggered auto-deploys.
-- When a GitLab push webhook matches a rule's branch pattern, PEPA automatically
-- writes the new image tag to the manifest repo for all bindings in that environment.

CREATE TABLE IF NOT EXISTS auto_deploy_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Source: which GitLab project triggers this rule
    pipeline_source_id UUID REFERENCES pipeline_sources(id) ON DELETE CASCADE,
    project_id TEXT,            -- GitLab project ID (alternative to pipeline_source_id)
    project_path TEXT,          -- GitLab project path e.g. "group/repo"

    -- Branch matching: glob pattern (e.g. "main", "release/*", "testing/*")
    branch_pattern TEXT NOT NULL,

    -- Target: which environment to deploy to
    environment_id UUID NOT NULL REFERENCES environments(id) ON DELETE CASCADE,

    -- Image tag extraction
    image_tag_source TEXT NOT NULL DEFAULT 'branch_name', -- branch_name, ci_variable, regex
    image_tag_regex TEXT,       -- regex to extract tag from branch name (when image_tag_source = 'regex')
    image_name TEXT,            -- container image name to update in manifests

    -- Behavior
    enabled BOOLEAN NOT NULL DEFAULT true,
    require_pipeline_success BOOLEAN NOT NULL DEFAULT false, -- wait for CI pipeline to succeed
    auto_create_deployment BOOLEAN NOT NULL DEFAULT true,    -- create deployment record

    -- Metadata
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for fast lookups
CREATE INDEX IF NOT EXISTS idx_auto_deploy_rules_tenant ON auto_deploy_rules (tenant_id);
CREATE INDEX IF NOT EXISTS idx_auto_deploy_rules_source ON auto_deploy_rules (pipeline_source_id) WHERE pipeline_source_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_auto_deploy_rules_project ON auto_deploy_rules (project_id) WHERE project_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_auto_deploy_rules_env ON auto_deploy_rules (environment_id);
CREATE INDEX IF NOT EXISTS idx_auto_deploy_rules_enabled ON auto_deploy_rules (enabled) WHERE enabled = true;

-- Ensure no duplicate rules for same source + pattern + environment.
-- PostgreSQL does not allow expressions inside table UNIQUE constraints, so
-- the dedupe (including COALESCE so project_path-based rules, where
-- pipeline_source_id IS NULL, collapse to one group) is a unique expression index.
CREATE UNIQUE INDEX IF NOT EXISTS uq_auto_deploy_rule
    ON auto_deploy_rules (
        tenant_id,
        COALESCE(pipeline_source_id, '00000000-0000-0000-0000-000000000000'::uuid),
        branch_pattern,
        environment_id
    );

-- RLS policies
ALTER TABLE auto_deploy_rules ENABLE ROW LEVEL SECURITY;

CREATE POLICY "auto_deploy_rules_tenant_isolation" ON auto_deploy_rules
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid);

-- RBAC permissions (only if the optional rbac_permissions table exists;
-- the runtime RBAC engine and the admin JWT bypass use the permissions table)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'rbac_permissions') THEN
        INSERT INTO rbac_permissions (role, resource, action) VALUES
            ('admin', 'auto_deploy_rules', 'read'),
            ('admin', 'auto_deploy_rules', 'create'),
            ('admin', 'auto_deploy_rules', 'update'),
            ('admin', 'auto_deploy_rules', 'delete'),
            ('platform_admin', 'auto_deploy_rules', 'read'),
            ('platform_admin', 'auto_deploy_rules', 'create'),
            ('platform_admin', 'auto_deploy_rules', 'update'),
            ('platform_admin', 'auto_deploy_rules', 'delete')
        ON CONFLICT (role, resource, action) DO NOTHING;
    END IF;
END $$;
