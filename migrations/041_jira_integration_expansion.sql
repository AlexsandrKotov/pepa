-- 041_jira_integration_expansion.sql
-- Expands Jira integration with assignees, sprints, worklogs, and issue links.

-- Cached Jira assignees/users
CREATE TABLE IF NOT EXISTS jira_assignees (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    jira_account TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    email       TEXT NOT NULL DEFAULT '',
    avatar_url  TEXT NOT NULL DEFAULT '',
    active      BOOLEAN NOT NULL DEFAULT true,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, jira_account)
);

CREATE INDEX IF NOT EXISTS idx_jira_assignees_tenant ON jira_assignees (tenant_id);

-- Cached Jira sprints
CREATE TABLE IF NOT EXISTS jira_sprints (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    jira_id     INTEGER NOT NULL,
    board_id    INTEGER NOT NULL DEFAULT 0,
    name        TEXT NOT NULL DEFAULT '',
    state       TEXT NOT NULL DEFAULT 'active',
    start_date  TIMESTAMPTZ,
    end_date    TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, jira_id)
);

CREATE INDEX IF NOT EXISTS idx_jira_sprints_tenant ON jira_sprints (tenant_id);
CREATE INDEX IF NOT EXISTS idx_jira_sprints_state ON jira_sprints (tenant_id, state);

-- Jira worklog entries (time tracking)
CREATE TABLE IF NOT EXISTS jira_worklogs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID NOT NULL,
    issue_key        TEXT NOT NULL,
    jira_worklog_id  TEXT NOT NULL DEFAULT '',
    author           TEXT NOT NULL DEFAULT '',
    time_spent       TEXT NOT NULL DEFAULT '',
    time_spent_secs  INTEGER NOT NULL DEFAULT 0,
    comment          TEXT NOT NULL DEFAULT '',
    started_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, issue_key, jira_worklog_id)
);

CREATE INDEX IF NOT EXISTS idx_jira_worklogs_tenant_issue ON jira_worklogs (tenant_id, issue_key);

-- Jira issue links (relationships between issues)
CREATE TABLE IF NOT EXISTS jira_issue_links (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    inward_key    TEXT NOT NULL,
    outward_key   TEXT NOT NULL,
    link_type     TEXT NOT NULL DEFAULT 'Relates',
    inward_label  TEXT NOT NULL DEFAULT '',
    outward_label TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, inward_key, outward_key, link_type)
);

CREATE INDEX IF NOT EXISTS idx_jira_issue_links_tenant ON jira_issue_links (tenant_id);
CREATE INDEX IF NOT EXISTS idx_jira_issue_links_inward ON jira_issue_links (tenant_id, inward_key);
CREATE INDEX IF NOT EXISTS idx_jira_issue_links_outward ON jira_issue_links (tenant_id, outward_key);

-- Add RLS policies
ALTER TABLE jira_assignees ENABLE ROW LEVEL SECURITY;
ALTER TABLE jira_sprints ENABLE ROW LEVEL SECURITY;
ALTER TABLE jira_worklogs ENABLE ROW LEVEL SECURITY;
ALTER TABLE jira_issue_links ENABLE ROW LEVEL SECURITY;
