-- ============================================================
-- 028: Seed RBAC permissions for the virtualization resource
-- ============================================================
-- Supports the Proxmox virtualization plugin. The resource name
-- 'virtualization' is generic so future VM providers (VMware etc.)
-- can reuse the same permission scope.

-- Admin: full CRUD on virtualization
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000001', 'virtualization', a, 'allow'
FROM unnest(ARRAY['create','read','update','delete']) AS a
ON CONFLICT DO NOTHING;

-- Viewer: read-only on virtualization
INSERT INTO permissions (role_id, resource, action, effect) VALUES
    ('20000000-0000-0000-0000-000000000003', 'virtualization', 'read', 'allow')
ON CONFLICT DO NOTHING;
