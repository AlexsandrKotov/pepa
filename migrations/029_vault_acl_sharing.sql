-- ============================================================
-- Migration 029: Vault ACL & Credential Sharing
-- ============================================================
-- Adds per-path access control for Vault secrets and the ability
-- to share personal credentials with specific users or teams.
-- ============================================================

-- ── 1. Vault path-based ACL ──────────────────────────────────
-- Controls which users/teams can access specific vault path prefixes.
-- Access is granted via explicit ACL entries (sharing) or by secret ownership.
-- Super Admins bypass ACL.

CREATE TABLE IF NOT EXISTS vault_acl (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    path_prefix VARCHAR(512) NOT NULL,
    user_id     UUID REFERENCES users(id) ON DELETE CASCADE,
    team_id     UUID REFERENCES teams(id) ON DELETE CASCADE,
    can_read    BOOLEAN DEFAULT TRUE,
    can_create  BOOLEAN DEFAULT FALSE,
    can_delete  BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT vault_acl_target CHECK (user_id IS NOT NULL OR team_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_vault_acl_tenant_path ON vault_acl(tenant_id, path_prefix);
CREATE INDEX IF NOT EXISTS idx_vault_acl_user ON vault_acl(user_id);
CREATE INDEX IF NOT EXISTS idx_vault_acl_team ON vault_acl(team_id);

-- ── 2. Credential sharing ────────────────────────────────────
-- Lets a user grant access to their personal credential to another
-- user or team.  shared_with_user and shared_with_team are mutually
-- exclusive (enforced by CHECK constraint).

CREATE TABLE IF NOT EXISTS credential_shares (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    credential_id       UUID NOT NULL REFERENCES user_credentials(id) ON DELETE CASCADE,
    owner_user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id           UUID NOT NULL,
    shared_with_user    UUID REFERENCES users(id) ON DELETE CASCADE,
    shared_with_team    UUID REFERENCES teams(id) ON DELETE CASCADE,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT credential_shares_target CHECK (shared_with_user IS NOT NULL OR shared_with_team IS NOT NULL),
    CONSTRAINT credential_shares_unique UNIQUE (credential_id, shared_with_user, shared_with_team)
);

CREATE INDEX IF NOT EXISTS idx_cred_shares_credential ON credential_shares(credential_id);
CREATE INDEX IF NOT EXISTS idx_cred_shares_user ON credential_shares(shared_with_user);
CREATE INDEX IF NOT EXISTS idx_cred_shares_team ON credential_shares(shared_with_team);

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (29, 'Vault ACL and credential sharing')
ON CONFLICT DO NOTHING;
