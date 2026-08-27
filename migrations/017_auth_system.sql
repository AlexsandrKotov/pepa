-- ============================================================
-- Migration 017: Authentication & Authorization System
-- ============================================================
-- Adds local login/password auth, per-user external credentials,
-- and extends RBAC for team-based role assignments.
-- ============================================================

-- ── 1. Add password auth columns to users table ───────────────
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash VARCHAR(256);
-- auth_provider already exists, but ensure default is set for new rows
-- 'local' = login/password, 'oauth' = external provider
ALTER TABLE users ALTER COLUMN auth_provider SET DEFAULT 'local';

-- ── 2. Per-user external credentials ──────────────────────────
-- Stores each user's personal access tokens for external services.
-- Tokens are encrypted using the application ENCRYPTION_KEY (AES-256-GCM).
CREATE TABLE IF NOT EXISTS user_credentials (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id     UUID NOT NULL,
    provider      VARCHAR(64) NOT NULL,       -- 'gitlab', 'github', 'gitea', 'bitbucket', 'docker_registry'
    provider_url  VARCHAR(512),               -- instance URL (e.g. https://gitlab.example.com)
    display_name  VARCHAR(128),               -- user-friendly label
    token_enc     TEXT NOT NULL,              -- encrypted PAT (enc:v2:... format)
    username      VARCHAR(256),               -- remote username (used for git commits)
    email         VARCHAR(256),               -- remote email (used for git commits)
    is_default    BOOLEAN DEFAULT FALSE,      -- default credential for this provider type
    last_verified TIMESTAMPTZ,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    updated_at    TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (user_id, provider, provider_url)
);

CREATE INDEX IF NOT EXISTS idx_user_credentials_user ON user_credentials(user_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_user_credentials_provider ON user_credentials(provider, provider_url);

-- ── 3. Extend role_assignments for team-scoped roles ──────────
-- scope_type and scope_value columns already exist in the schema.
-- This migration adds documentation indexes for clarity.
-- Usage:
--   scope_type = 'team', scope_value = <team_id> => role applies to team members
--   scope_type = 'tenant'                        => role applies tenant-wide
--   scope_type IS NULL                           => direct user assignment

-- ── 4. Create default admin user (password: 'admin') ──────────
-- The bcrypt hash below is for password 'admin' with cost 12.
-- This user should have their password changed immediately after first login.
INSERT INTO users (id, email, name, auth_provider, password_hash, is_active)
VALUES (
    '00000000-0000-0000-0000-000000000010',
    'admin@local',
    'Administrator',
    'local',
    '$2a$12$8M3wFW5FApcQnLAyiU/lV.OXiNEB5V3VTbHiWd097cFY15wwwKbKW',
    true
)
ON CONFLICT (id) DO NOTHING;

-- Assign admin role to the default admin user
INSERT INTO role_assignments (id, tenant_id, user_id, role_id, is_active)
SELECT
    '00000000-0000-0000-0000-000000000011',
    '00000000-0000-0000-0000-000000000002',
    '00000000-0000-0000-0000-000000000010',
    r.id,
    true
FROM roles r
WHERE r.tenant_id = '00000000-0000-0000-0000-000000000002'
  AND r.slug = 'admin'
  AND NOT EXISTS (
    SELECT 1 FROM role_assignments ra
    WHERE ra.user_id = '00000000-0000-0000-0000-000000000010'
      AND ra.role_id = r.id
  );
