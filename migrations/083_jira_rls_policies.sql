-- Migration: Add RLS policies for JIRA tables that were missing tenant isolation
-- This ensures all tenant-scoped tables have proper RLS enforcement

-- Enable RLS on JIRA tables if not already enabled
ALTER TABLE jira_assignees ENABLE ROW LEVEL SECURITY;
ALTER TABLE jira_issue_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE jira_sprints ENABLE ROW LEVEL SECURITY;
ALTER TABLE jira_worklogs ENABLE ROW LEVEL SECURITY;

-- Force RLS for table owners (except superuser)
ALTER TABLE jira_assignees FORCE ROW LEVEL SECURITY;
ALTER TABLE jira_issue_links FORCE ROW LEVEL SECURITY;
ALTER TABLE jira_sprints FORCE ROW LEVEL SECURITY;
ALTER TABLE jira_worklogs FORCE ROW LEVEL SECURITY;

-- Drop existing policies if they exist (idempotent)
DROP POLICY IF EXISTS tenant_isolation ON jira_assignees;
DROP POLICY IF EXISTS tenant_isolation ON jira_issue_links;
DROP POLICY IF EXISTS tenant_isolation ON jira_sprints;
DROP POLICY IF EXISTS tenant_isolation ON jira_worklogs;

-- Create tenant isolation policies using the unified GUC
CREATE POLICY tenant_isolation ON jira_assignees
  FOR ALL
  USING (tenant_id = current_setting('app.tenant_id')::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY tenant_isolation ON jira_issue_links
  FOR ALL
  USING (tenant_id = current_setting('app.tenant_id')::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY tenant_isolation ON jira_sprints
  FOR ALL
  USING (tenant_id = current_setting('app.tenant_id')::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY tenant_isolation ON jira_worklogs
  FOR ALL
  USING (tenant_id = current_setting('app.tenant_id')::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- Grant permissions to pepa_app role
GRANT SELECT, INSERT, UPDATE, DELETE ON jira_assignees TO pepa_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON jira_issue_links TO pepa_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON jira_sprints TO pepa_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON jira_worklogs TO pepa_app;
