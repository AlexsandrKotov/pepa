-- Migration 068: Environment Overview & Developer Self-Service
-- Adds support for unified environment dashboard and developer self-service deployments

-- Environment overview: no new tables needed, we use existing data
-- But we add a view for convenience

-- Self-service deployment requests tracking
CREATE TABLE IF NOT EXISTS self_service_deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- Source
    git_repo_url TEXT NOT NULL,
    git_branch TEXT NOT NULL DEFAULT 'main',
    git_commit_sha TEXT,
    
    -- Target
    environment_id UUID REFERENCES environments(id) ON DELETE SET NULL,
    service_id UUID REFERENCES services(id) ON DELETE SET NULL,
    
    -- Blueprint
    blueprint_type TEXT NOT NULL DEFAULT 'auto', -- auto, helm, docker_compose, raw_k8s
    blueprint_config JSONB DEFAULT '{}',
    
    -- Deployment
    deployment_id UUID REFERENCES deployments(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending', -- pending, detecting, building, deploying, deployed, failed, cancelled
    progress INTEGER DEFAULT 0, -- 0-100
    logs TEXT DEFAULT '',
    error_message TEXT,
    
    -- Preview environment (for PR deployments)
    is_preview BOOLEAN DEFAULT FALSE,
    pr_number INTEGER,
    preview_url TEXT,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deployed_at TIMESTAMPTZ,
    
    CONSTRAINT self_service_deployments_status_check CHECK (status IN ('pending', 'detecting', 'building', 'deploying', 'deployed', 'failed', 'cancelled'))
);

-- Indexes for self-service deployments
CREATE INDEX IF NOT EXISTS idx_self_service_deployments_tenant ON self_service_deployments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_self_service_deployments_user ON self_service_deployments(user_id);
CREATE INDEX IF NOT EXISTS idx_self_service_deployments_status ON self_service_deployments(status);
CREATE INDEX IF NOT EXISTS idx_self_service_deployments_env ON self_service_deployments(environment_id);
CREATE INDEX IF NOT EXISTS idx_self_service_deployments_service ON self_service_deployments(service_id);

-- RLS policies
ALTER TABLE self_service_deployments ENABLE ROW LEVEL SECURITY;

-- All authenticated users in the tenant can see self-service deployments
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies 
        WHERE policyname = 'self_service_deployments_tenant_isolation' 
        AND tablename = 'self_service_deployments'
    ) THEN
        CREATE POLICY "self_service_deployments_tenant_isolation" 
        ON self_service_deployments 
        FOR ALL 
        USING (tenant_id = current_setting('app.tenant_id', true)::uuid);
    END IF;
END $$;

-- RBAC permissions for self-service (only if rbac_permissions table exists)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'rbac_permissions') THEN
        INSERT INTO rbac_permissions (role, resource, action) VALUES
            ('admin', 'self_service', 'create'),
            ('admin', 'self_service', 'read'),
            ('admin', 'self_service', 'update'),
            ('admin', 'self_service', 'delete'),
            ('developer', 'self_service', 'create'),
            ('developer', 'self_service', 'read'),
            ('developer', 'self_service', 'update'),
            ('viewer', 'self_service', 'read')
        ON CONFLICT (role, resource, action) DO NOTHING;
    END IF;
END $$;
