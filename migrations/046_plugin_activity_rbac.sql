-- ============================================================
-- 046: Seed RBAC permissions for the plugin_activity resource
-- ============================================================
-- Plugin Activity was previously gated behind the 'audit' permission.
-- This migration creates a dedicated 'plugin_activity' resource so
-- that viewing plugin action logs can be granted independently.

-- Admin: full CRUD on plugin_activity
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000001', 'plugin_activity', a, 'allow'
FROM unnest(ARRAY['create','read','update','delete']) AS a
ON CONFLICT DO NOTHING;

-- Viewer: read-only on plugin_activity
INSERT INTO permissions (role_id, resource, action, effect) VALUES
    ('20000000-0000-0000-0000-000000000003', 'plugin_activity', 'read', 'allow')
ON CONFLICT DO NOTHING;
