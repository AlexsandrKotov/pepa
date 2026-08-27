-- ============================================================
-- Migration 031: Vault ACL Ownership
-- ============================================================
-- Adds created_by tracking to vault_acl so that only the
-- original creator (or a Super Admin) can modify or delete
-- an access rule.  The target user/team can see the rule
-- but cannot change or remove it.
-- ============================================================

-- ── 1. Add created_by to vault_acl ──────────────────────────
ALTER TABLE vault_acl ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_vault_acl_created_by ON vault_acl(created_by);

-- ── 2. Backfill created_by ──────────────────────────────────
-- For existing entries we cannot know who created them, so we
-- leave created_by NULL.  NULL entries can be managed by any
-- user with the appropriate RBAC permission (backward compat).
-- Going forward, new entries will always have created_by set.

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (31, 'Vault ACL ownership tracking')
ON CONFLICT DO NOTHING;
