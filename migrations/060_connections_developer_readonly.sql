-- 060: Restrict developer role on connections to read-only.
-- Admins manage connections (WHERE); developers supply personal
-- credentials (WHO) via user_credentials. Developers should not
-- create or modify admin-configured connections.

-- Revoke create and update on connections from the developer role.
DELETE FROM permissions
WHERE role_id = '20000000-0000-0000-0000-000000000002'  -- developer
  AND resource = 'connections'
  AND action IN ('create', 'update');

-- Ensure developer has delete on credentials (personal creds management).
INSERT INTO permissions (id, role_id, resource, action, effect)
SELECT gen_random_uuid(),
       '20000000-0000-0000-0000-000000000002',
       'credentials', 'delete', 'allow'
WHERE NOT EXISTS (
    SELECT 1 FROM permissions
    WHERE role_id = '20000000-0000-0000-0000-000000000002'
      AND resource = 'credentials' AND action = 'delete'
);

INSERT INTO schema_migrations (version, description)
VALUES (60, 'Restrict developer connections to read-only; grant credentials delete')
ON CONFLICT DO NOTHING;
