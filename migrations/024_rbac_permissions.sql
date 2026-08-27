-- ============================================================
-- 024: Expand RBAC permission seeds to cover all resources
-- ============================================================
-- Previously only a subset of resources had seeded permissions.
-- This migration adds the missing resources so that the RBAC
-- system can enforce fine-grained access control on all pages.

-- Admin: add missing resources with full CRUD
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000001', r, a, 'allow'
FROM unnest(ARRAY['pipelines','gitops','docker','helm','environments','discovery','import','ai','jira','credentials']) AS r
CROSS JOIN unnest(ARRAY['create','read','update','delete']) AS a
ON CONFLICT DO NOTHING;

-- Developer: add missing resources
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000002', r, a, 'allow'
FROM unnest(ARRAY['pipelines','gitops','docker','helm','discovery','ai']) AS r
CROSS JOIN unnest(ARRAY['create','read','update']) AS a
ON CONFLICT DO NOTHING;

-- Developer: read-only on environments
INSERT INTO permissions (role_id, resource, action, effect) VALUES
    ('20000000-0000-0000-0000-000000000002', 'environments', 'read', 'allow')
ON CONFLICT DO NOTHING;

-- Developer: CRUD on credentials (personal), read on import
INSERT INTO permissions (role_id, resource, action, effect) VALUES
    ('20000000-0000-0000-0000-000000000002', 'credentials', 'create', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'credentials', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'credentials', 'delete', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'import', 'create', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'import', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'import', 'update', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'jira', 'create', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'jira', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'jira', 'update', 'allow')
ON CONFLICT DO NOTHING;

-- Viewer: add missing resources with read-only
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000003', r, 'read', 'allow'
FROM unnest(ARRAY['pipelines','gitops','docker','helm','environments','discovery','ai','credentials']) AS r
ON CONFLICT DO NOTHING;
