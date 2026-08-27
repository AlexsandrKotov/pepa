-- Migration 020: Fix vault RBAC permissions
-- The vault handlers were checking for "vault:write" which didn't exist in the
-- permissions seed data. Handlers now use "vault:create" and "vault:delete".
-- This migration grants Developer role the vault:create and vault:delete permissions
-- so developers can manage secrets.

-- Grant Developer role vault:create and vault:delete permissions
INSERT INTO permissions (role_id, resource, action, effect) VALUES
    ('20000000-0000-0000-0000-000000000002', 'vault', 'create', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'vault', 'delete', 'allow')
ON CONFLICT DO NOTHING;

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (20, 'Fix vault RBAC permissions for Developer role')
ON CONFLICT DO NOTHING;
