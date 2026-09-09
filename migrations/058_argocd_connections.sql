-- Migration 058: ArgoCD/FluxCD connection types and GitOps application bindings
-- Establishes proper connection types for GitOps engines and creates the binding table

-- Add argocd_connection_id column to gitops_repositories
ALTER TABLE gitops_repositories ADD COLUMN IF NOT EXISTS argocd_connection_id UUID;

-- Create gitops_application_bindings table
CREATE TABLE IF NOT EXISTS gitops_application_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    service_id UUID,
    entity_id UUID,
    name TEXT NOT NULL,
    repo_id UUID REFERENCES gitops_repositories(id) ON DELETE SET NULL,
    cluster_id UUID,
    argo_connection_id UUID,
    engine_type TEXT NOT NULL DEFAULT 'argocd', -- 'argocd' or 'fluxcd'
    app_name TEXT NOT NULL,
    app_namespace TEXT NOT NULL DEFAULT 'default',
    app_project TEXT,
    environment TEXT,
    manifest_path TEXT,
    update_strategy TEXT NOT NULL DEFAULT 'kustomize_image', -- 'kustomize_image', 'helm_values', 'appset_param', 'raw_yaml'
    update_path TEXT,
    verify_url TEXT,
    auto_bound BOOLEAN DEFAULT FALSE,
    created_by UUID,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(tenant_id, argo_connection_id, app_namespace, app_name)
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_gitops_bindings_tenant ON gitops_application_bindings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_gitops_bindings_service ON gitops_application_bindings(service_id);
CREATE INDEX IF NOT EXISTS idx_gitops_bindings_repo ON gitops_application_bindings(repo_id);
CREATE INDEX IF NOT EXISTS idx_gitops_bindings_cluster ON gitops_application_bindings(cluster_id);
CREATE INDEX IF NOT EXISTS idx_gitops_bindings_connection ON gitops_application_bindings(argo_connection_id);
CREATE INDEX IF NOT EXISTS idx_gitops_bindings_env ON gitops_application_bindings(environment);

-- RLS policy (matching 023_rls_remaining_tables.sql pattern)
ALTER TABLE gitops_application_bindings ENABLE ROW LEVEL SECURITY;

-- Drop existing policy if it exists (for idempotency)
DROP POLICY IF EXISTS gitops_bindings_tenant_isolation ON gitops_application_bindings;

-- Create RLS policy based on tenant_setting pattern
DO $$
BEGIN
    -- Check if tenant_setting column exists in a related table to determine the pattern
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'gitops_repositories' AND column_name = 'tenant_id') THEN
        EXECUTE format('CREATE POLICY gitops_bindings_tenant_isolation ON gitops_application_bindings
            FOR ALL
            USING (tenant_id = current_setting(''app.tenant_id'', true)::uuid)');
    ELSE
        -- Fallback: allow all (will be restricted by application logic)
        EXECUTE 'CREATE POLICY gitops_bindings_tenant_isolation ON gitops_application_bindings
            FOR ALL
            USING (true)';
    END IF;
END $$;

-- Add gitops fields to deployments table
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS argo_app_ref TEXT;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS argo_connection_id UUID;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS git_commit_sha TEXT;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS deployed_revision TEXT;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS verification JSONB;

-- Index for faster lookups
CREATE INDEX IF NOT EXISTS idx_deployments_argo_ref ON deployments(argo_app_ref) WHERE argo_app_ref IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_deployments_git_commit ON deployments(git_commit_sha) WHERE git_commit_sha IS NOT NULL;

-- Comment for documentation
COMMENT ON TABLE gitops_application_bindings IS 'Maps services/entities to GitOps applications (ArgoCD Applications or FluxCD Kustomizations/HelmReleases)';
COMMENT ON COLUMN gitops_application_bindings.engine_type IS 'GitOps engine: argocd or fluxcd';
COMMENT ON COLUMN gitops_application_bindings.update_strategy IS 'How to update image tags: kustomize_image, helm_values, appset_param, or raw_yaml';
