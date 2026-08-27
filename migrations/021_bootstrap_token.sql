-- ============================================================
-- Migration 021: Bootstrap Token for First-Run Setup
-- ============================================================
-- Adds a one-time bootstrap token table used for initial super-admin
-- activation. The token is generated on first start, printed to the
-- console, and expires after 1 hour.
-- ============================================================

-- ── 1. Bootstrap tokens table ─────────────────────────────────
CREATE TABLE IF NOT EXISTS bootstrap_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash  VARCHAR(256) NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- ── 2. Add must_change_password flag to users ─────────────────
ALTER TABLE users ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN DEFAULT FALSE;
