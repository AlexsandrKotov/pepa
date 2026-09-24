-- 082_connection_access_control.sql
-- Per-connection ACL (Access Control List) for fine-grained connection visibility and usage control.
-- Modeled after vault_acl (migration 029) for consistency with existing patterns.

-- ============================================================
-- CONNECTION ACL TABLE
-- ============================================================
-- Grants per-connection access to individual users or teams.
-- When a connection has restricted=true, only users/teams with ACL entries
-- (plus admin and owner) can see and use the connection.

CREATE TABLE IF NOT EXISTS connection_acl (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    connection_id   UUID NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    user_id         UUID REFERENCES users(id) ON DELETE CASCADE,
    team_id         UUID REFERENCES teams(id) ON DELETE CASCADE,
    can_read        BOOLEAN NOT NULL DEFAULT true,   -- can see connection in list
    can_use         BOOLEAN NOT NULL DEFAULT true,   -- can test/browse/execute
    created_by      UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Exactly one of user_id or team_id must be set (same pattern as vault_acl).
    CONSTRAINT chk_connection_acl_target CHECK (
        (user_id IS NOT NULL AND team_id IS NULL) OR
        (user_id IS NULL AND team_id IS NOT NULL)
    ),
    -- No duplicate grants for the same user or team on the same connection.
    CONSTRAINT uq_connection_acl_user UNIQUE (connection_id, user_id),
    CONSTRAINT uq_connection_acl_team UNIQUE (connection_id, team_id)
);

CREATE INDEX IF NOT EXISTS idx_connection_acl_connection ON connection_acl(connection_id);
CREATE INDEX IF NOT EXISTS idx_connection_acl_tenant ON connection_acl(tenant_id);
CREATE INDEX IF NOT EXISTS idx_connection_acl_user ON connection_acl(user_id);
CREATE INDEX IF NOT EXISTS idx_connection_acl_team ON connection_acl(team_id);

-- Row Level Security (consistent with all tenant-scoped tables).
ALTER TABLE connection_acl ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_connection_acl ON connection_acl
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- ============================================================
-- RESTRICTED COLUMN ON CONNECTIONS
-- ============================================================
-- When restricted=true, only users/teams in connection_acl (+ admin/owner) can access.
-- When restricted=false (default), all users with connections:read can access (backward compatible).

ALTER TABLE connections ADD COLUMN IF NOT EXISTS restricted BOOLEAN NOT NULL DEFAULT false;
COMMENT ON COLUMN connections.restricted IS 'If true, only users/teams in connection_acl can access this connection';

-- ============================================================
-- DUPLICATE DETECTION FUNCTION
-- ============================================================
-- Checks if a connection with the same type and URL already exists in the tenant.
-- Used by the API to warn users before creating duplicate connections.

CREATE OR REPLACE FUNCTION check_connection_duplicate(
    p_tenant_id UUID,
    p_type VARCHAR,
    p_url VARCHAR
) RETURNS TABLE(existing_id UUID, existing_name VARCHAR) AS $$
BEGIN
    RETURN QUERY
    SELECT c.id, c.name
    FROM connections c
    WHERE c.tenant_id = p_tenant_id
      AND c.type = p_type
      AND (
          c.config->>'url' = p_url
          OR c.config->>'endpoint' = p_url
          OR c.config->>'repo_url' = p_url
          OR c.config->>'server_url' = p_url
      )
    LIMIT 1;
END;
$$ LANGUAGE plpgsql STABLE;
