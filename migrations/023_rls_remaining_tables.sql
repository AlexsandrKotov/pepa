-- ============================================================
-- Migration 023: RLS on remaining tenant-scoped tables
-- ============================================================
-- Closes the RLS gap for tables created after migration 013
-- that were missed: vault_secrets, gitops_repositories,
-- jira_automation_rules, jira_comments, user_credentials,
-- environment_variables.
-- ============================================================

-- vault_secrets (tenant_id added in migration 010)
ALTER TABLE vault_secrets ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_vault_secrets ON vault_secrets;
CREATE POLICY tenant_isolation_vault_secrets ON vault_secrets
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- gitops_repositories (migration 014)
ALTER TABLE gitops_repositories ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_gitops_repositories ON gitops_repositories;
CREATE POLICY tenant_isolation_gitops_repositories ON gitops_repositories
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- jira_automation_rules (migration 016)
ALTER TABLE jira_automation_rules ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_jira_automation_rules ON jira_automation_rules;
CREATE POLICY tenant_isolation_jira_automation_rules ON jira_automation_rules
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- jira_comments (migration 016)
ALTER TABLE jira_comments ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_jira_comments ON jira_comments;
CREATE POLICY tenant_isolation_jira_comments ON jira_comments
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- user_credentials (migration 017)
ALTER TABLE user_credentials ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_user_credentials ON user_credentials;
CREATE POLICY tenant_isolation_user_credentials ON user_credentials
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- environment_variables (migration 018)
ALTER TABLE environment_variables ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_environment_variables ON environment_variables;
CREATE POLICY tenant_isolation_environment_variables ON environment_variables
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (23, 'Enable RLS on remaining tenant-scoped tables')
ON CONFLICT DO NOTHING;
