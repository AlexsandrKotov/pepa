-- ============================================================
-- Migration 032: Invalidate default admin password
-- ============================================================
-- The default admin user (created in migration 017) had password 'admin'.
-- This migration invalidates that password by setting password_hash to empty,
-- forcing the admin to set a new password via bootstrap activation.
-- Only affects the admin user if the password_hash is still the default.
-- ============================================================

UPDATE users SET password_hash = ''
WHERE id = '00000000-0000-0000-0000-000000000010'
  AND password_hash = '$2a$12$8M3wFW5FApcQnLAyiU/lV.OXiNEB5V3VTbHiWd097cFY15wwwKbKW';
