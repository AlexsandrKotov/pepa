-- 057_connection_auth_enhancements.sql
-- Add owner tracking and admin-fallback control to connections.
-- This enables per-user credential isolation: admin configures WHERE (connections),
-- users configure WHO (their own credentials in user_credentials).

-- Track who created each connection (audit trail).
ALTER TABLE connections ADD COLUMN IF NOT EXISTS owner_id UUID REFERENCES users(id) ON DELETE SET NULL;

-- When TRUE (default), users without personal credentials fall back to the admin
-- connection credential. When FALSE, users MUST add their own credential or be blocked.
ALTER TABLE connections ADD COLUMN IF NOT EXISTS fallback_to_admin BOOLEAN DEFAULT TRUE;

-- Index for owner lookups.
CREATE INDEX IF NOT EXISTS idx_connections_owner ON connections(owner_id);

-- Grant developer role read access to connections (they need to USE them).
-- Admin retains full CRUD. This is enforced in the RBAC middleware.
INSERT INTO permissions (id, role_id, resource, action, effect)
SELECT
    gen_random_uuid(),
    '20000000-0000-0000-0000-000000000002', -- developer role
    'connections',
    'read',
    'allow'
WHERE NOT EXISTS (
    SELECT 1 FROM permissions
    WHERE role_id = '20000000-0000-0000-0000-000000000002'
      AND resource = 'connections' AND action = 'read'
);
