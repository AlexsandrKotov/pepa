-- Jira automation rules and comments cache

CREATE TABLE IF NOT EXISTS jira_automation_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    trigger_type TEXT NOT NULL, -- deployment_created, deployment_succeeded, deployment_failed, pipeline_completed, service_created, manual
    jira_project_key TEXT DEFAULT '',
    jql_filter TEXT DEFAULT '',
    action_type TEXT NOT NULL, -- add_comment, transition, update_field, notify
    action_config JSONB NOT NULL DEFAULT '{}',
    enabled BOOLEAN DEFAULT true,
    last_triggered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_jira_automation_rules_tenant ON jira_automation_rules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_jira_automation_rules_trigger ON jira_automation_rules(trigger_type);

-- Comments cache table for faster retrieval without hitting Jira API every time
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
