-- ============================================================
-- Migration 026: Token version for session revocation
-- ============================================================
-- Adds a per-user token_version counter. It is embedded in every
-- issued JWT and bumped whenever the user's credentials or access
-- change (password change, deactivation, role change). Tokens with
-- a stale version are rejected on refresh, effectively revoking all
-- outstanding sessions.
-- ============================================================

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS token_version INTEGER NOT NULL DEFAULT 0;
