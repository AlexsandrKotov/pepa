-- ============================================================
-- Migration 025: Fix bootstrap flow
-- ============================================================
-- The default admin user must have must_change_password = TRUE
-- so that the bootstrap token entry form is shown on first run.
-- Without this, bootstrapStatusHandler sees must_change_password = false
-- and incorrectly reports that bootstrap is already complete.
-- ============================================================

UPDATE users SET must_change_password = TRUE
WHERE id = '00000000-0000-0000-0000-000000000010'
  AND must_change_password = FALSE;
