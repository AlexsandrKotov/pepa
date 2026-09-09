-- Migration 059: Add RBAC permissions for GitOps fine-grained actions
-- Adds sync, rollback, approve, and write_git permissions for the gitops resource

-- Admin: full CRUD + sync/rollback/approve/write_git on gitops
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000001', 'gitops', a, 'allow'
FROM unnest(ARRAY['create','read','update','delete','sync','rollback','approve','write_git']) AS a
ON CONFLICT DO NOTHING;

-- Developer: read + create + update + sync + rollback + write_git on gitops
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000002', 'gitops', a, 'allow'
FROM unnest(ARRAY['create','read','update','sync','rollback','write_git']) AS a
ON CONFLICT DO NOTHING;

-- Viewer: read-only on gitops (can view applications, diffs, history, logs)
INSERT INTO permissions (role_id, resource, action, effect)
VALUES ('20000000-0000-0000-0000-000000000003', 'gitops', 'read', 'allow')
ON CONFLICT DO NOTHING;

-- Comment for documentation
COMMENT ON TABLE permissions IS 'RBAC permissions table - stores role-resource-action mappings';
