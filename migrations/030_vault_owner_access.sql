-- ============================================================
-- Migration 030: Vault Owner-Based Access Control
-- ============================================================
-- Switches Vault from backward-compatible (all visible when no ACL)
-- to strict owner-based access:
--   • Each secret has an owner_id (the user who created it).
--   • Owners always have full access to their own secrets.
--   • Other users need explicit ACL grants (path-based sharing).
--   • Default-deny: secrets without an owner or ACL grant are hidden.
-- ============================================================

-- ── 1. Add owner_id to vault_secrets ─────────────────────────
ALTER TABLE vault_secrets ADD COLUMN IF NOT EXISTS owner_id UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_vault_secrets_owner ON vault_secrets(owner_id);

-- ── 2. Backfill owner_id from created_by email ───────────────
UPDATE vault_secrets vs
SET owner_id = u.id
FROM users u
WHERE vs.created_by = u.email
  AND vs.owner_id IS NULL;

-- ── 3. Auto-create ACL entries for existing secrets ──────────
-- Grant each owner read+create+delete on their own secret paths
-- so they keep access under the new strict model.
INSERT INTO vault_acl (tenant_id, path_prefix, user_id, can_read, can_create, can_delete)
SELECT DISTINCT vs.tenant_id, vs.path, vs.owner_id, TRUE, TRUE, TRUE
FROM vault_secrets vs
WHERE vs.owner_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM vault_acl va
    WHERE va.tenant_id = vs.tenant_id
      AND va.path_prefix = vs.path
      AND va.user_id = vs.owner_id
  )
ON CONFLICT DO NOTHING;

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (30, 'Vault owner-based access control')
ON CONFLICT DO NOTHING;
