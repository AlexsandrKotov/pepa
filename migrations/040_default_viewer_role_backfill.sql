-- Migration: Backfill default viewer role for existing users without role assignments.
--
-- Prior to this migration, users created via OAuth/LDAP had no explicit
-- role_assignments and relied on hardcoded fallback roles in the JWT.
-- Now that fallback has been removed, so existing users need an explicit
-- viewer role in the default workspace to retain read-only access.

-- Assign the viewer role in the default workspace to every active user
-- that does not yet have any role assignment in that workspace.
-- Excludes the super admin user (who has implicit full access).
INSERT INTO role_assignments (id, tenant_id, user_id, role_id, is_active, created_at)
SELECT
    gen_random_uuid(),
    t.id,
    u.id,
    r.id,
    true,
    NOW()
FROM users u
CROSS JOIN tenants t
CROSS JOIN roles r
WHERE t.id = '00000000-0000-0000-0000-000000000002'          -- default workspace
  AND r.tenant_id = t.id AND r.slug = 'viewer'                -- viewer role
  AND u.is_active = true
  AND u.id != '00000000-0000-0000-0000-000000000010'          -- exclude super admin
  AND NOT EXISTS (
      SELECT 1 FROM role_assignments ra
      WHERE ra.user_id = u.id AND ra.tenant_id = t.id
  );

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (40, 'Backfill default viewer role for existing users without role assignments')
ON CONFLICT DO NOTHING;
