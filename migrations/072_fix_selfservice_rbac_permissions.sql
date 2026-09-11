-- Migration 072: Seed self_service and auto_deploy_rules RBAC permissions
-- into the main permissions table (used by the runtime RBAC engine).
--
-- Migrations 068 and 070 mistakenly seeded these resources into the
-- optional rbac_permissions table instead of the permissions table,
-- so non-admin users (and the frontend PermissionGuard) never saw them.

-- Role IDs (seeded in 001/024):
--   admin     = 20000000-0000-0000-0000-000000000001
--   developer = 20000000-0000-0000-0000-000000000002
--   viewer    = 20000000-0000-0000-0000-000000000003

-- self_service: admin full CRUD, developer create+read+update, viewer read
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000001', 'self_service', a, 'allow'
FROM unnest(ARRAY['create','read','update','delete']) AS a
ON CONFLICT DO NOTHING;

INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000002', 'self_service', a, 'allow'
FROM unnest(ARRAY['create','read','update']) AS a
ON CONFLICT DO NOTHING;

INSERT INTO permissions (role_id, resource, action, effect)
VALUES ('20000000-0000-0000-0000-000000000003', 'self_service', 'read', 'allow')
ON CONFLICT DO NOTHING;

-- auto_deploy_rules: admin full CRUD, developer read, viewer read
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000001', 'auto_deploy_rules', a, 'allow'
FROM unnest(ARRAY['create','read','update','delete']) AS a
ON CONFLICT DO NOTHING;

INSERT INTO permissions (role_id, resource, action, effect)
VALUES
    ('20000000-0000-0000-0000-000000000002', 'auto_deploy_rules', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000003', 'auto_deploy_rules', 'read', 'allow')
ON CONFLICT DO NOTHING;
