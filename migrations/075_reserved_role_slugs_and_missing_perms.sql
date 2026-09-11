-- ============================================================
-- 075: Seed the missing RBAC resources and guard privileged role slugs
-- ============================================================
-- Two separate defects are fixed here.
--
-- 1) rbacResourceMap routes /registry-repositories at resource "registry" and
--    /observability at resource "observability", but no permission rows were
--    ever seeded for those resources. Every non-admin therefore got a hard 403
--    (fail-closed on a missing grant), which made the registry browser and the
--    observability page unreachable for developers and viewers.
--
-- 2) The RBAC middleware grants a blanket bypass to any principal whose JWT
--    carries the role "admin"/"super_admin"/"platform_admin". Because those
--    strings are only slugs in the roles table, a role created with the slug
--    "admin" (or an existing role renamed to it) turned its holder into a
--    super-user. The trigger below makes the reserved slugs belong to the
--    seeded system roles only, and makes those roles undeletable.

-- ------------------------------------------------------------
-- 1) Missing permission seeds
-- ------------------------------------------------------------

-- Admin: full CRUD on both resources.
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000001', r, a, 'allow'
FROM unnest(ARRAY['registry','observability']) AS r
CROSS JOIN unnest(ARRAY['create','read','update','delete']) AS a
ON CONFLICT DO NOTHING;

-- Developer: read registries, read observability, no writes.
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000002', r, 'read', 'allow'
FROM unnest(ARRAY['registry','observability']) AS r
ON CONFLICT DO NOTHING;

-- Viewer: read-only.
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000003', r, 'read', 'allow'
FROM unnest(ARRAY['registry','observability']) AS r
ON CONFLICT DO NOTHING;

-- ------------------------------------------------------------
-- 2) Reserved role slugs
-- ------------------------------------------------------------

-- The administrator role seeded by 006_service_portal.sql is the only owner of
-- the privileged slugs; 'developer'/'viewer' stay editable on purpose.
CREATE OR REPLACE FUNCTION guard_reserved_role_slugs()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    reserved CONSTANT TEXT[] := ARRAY['admin','super_admin','platform admin','platform_admin'];
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.is_system THEN
            RAISE EXCEPTION 'role % (%) is a system role and cannot be deleted', OLD.name, OLD.slug
                USING ERRCODE = '42501';  -- insufficient_privilege
        END IF;
        RETURN OLD;
    END IF;

    -- A reserved slug may only exist on a system role. is_system is never settable
    -- through the role API, so this is the line between "built-in administrator
    -- role of a workspace" and "role somebody granted themselves".
    IF TG_OP = 'INSERT' THEN
        IF lower(NEW.slug) = ANY (reserved) AND NEW.is_system IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION 'slug % is reserved for the built-in administrator role', NEW.slug
                USING ERRCODE = '42501';  -- insufficient_privilege
        END IF;
        RETURN NEW;
    END IF;

    -- UPDATE: renaming into a reserved slug is the escalation path; renaming a
    -- system role out of its slug removes the platform's own admin.
    IF lower(NEW.slug) <> lower(OLD.slug) THEN
        IF lower(NEW.slug) = ANY (reserved) AND NEW.is_system IS DISTINCT FROM TRUE THEN
            RAISE EXCEPTION 'slug % is reserved for the built-in administrator role', NEW.slug
                USING ERRCODE = '42501';  -- insufficient_privilege
        END IF;
        IF OLD.is_system THEN
            RAISE EXCEPTION 'slug of system role % cannot be changed', OLD.name
                USING ERRCODE = '42501';  -- insufficient_privilege
        END IF;
    END IF;

    IF OLD.is_system AND NEW.is_system IS DISTINCT FROM TRUE THEN
        RAISE EXCEPTION 'role % is a system role and cannot be marked non-system', OLD.name
            USING ERRCODE = '42501';  -- insufficient_privilege
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_guard_reserved_role_slugs ON roles;
CREATE TRIGGER trg_guard_reserved_role_slugs
    BEFORE UPDATE OR DELETE ON roles
    FOR EACH ROW EXECUTE FUNCTION guard_reserved_role_slugs();

-- Any pre-existing non-system role that already carries a reserved slug is
-- renamed now, otherwise the bypass would keep working after the trigger is
-- installed (the trigger only checks rows that are written from now on).
UPDATE roles r
SET    slug  = 'imported_' || r.slug,
       name  = r.name || ' (imported)'
FROM  (
    SELECT id FROM roles
    WHERE  lower(slug) IN ('admin','super_admin','platform admin','platform_admin')
      AND  NOT is_system
) dup
WHERE  r.id = dup.id;
