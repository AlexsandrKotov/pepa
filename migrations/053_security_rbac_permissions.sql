-- Migration 053: Add RBAC permissions for security scanning resource
-- Ensures that the security scan routes are properly gated by RBAC.

-- Admin: full CRUD on security
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000001', 'security', a, 'allow'
FROM unnest(ARRAY['create','read','update','delete']) AS a
ON CONFLICT DO NOTHING;

-- Developer: read + create on security (can trigger scans, view results)
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000002', 'security', a, 'allow'
FROM unnest(ARRAY['create','read']) AS a
ON CONFLICT DO NOTHING;

-- Viewer: read-only on security (can view scan results and dashboards)
INSERT INTO permissions (role_id, resource, action, effect)
VALUES ('20000000-0000-0000-0000-000000000003', 'security', 'read', 'allow')
ON CONFLICT DO NOTHING;
