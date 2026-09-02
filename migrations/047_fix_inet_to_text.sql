-- 047_fix_inet_to_text.sql
-- Fix: PostgreSQL INET columns cannot be scanned into Go *string.
-- Change ip_address columns from INET to TEXT to match Go struct types.

ALTER TABLE plugin_action_log
    ALTER COLUMN ip_address TYPE TEXT USING ip_address::TEXT;

ALTER TABLE audit_log
    ALTER COLUMN ip_address TYPE TEXT USING COALESCE(ip_address::TEXT, '');
