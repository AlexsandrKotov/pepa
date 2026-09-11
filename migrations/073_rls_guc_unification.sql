-- ============================================================
-- 073: Unify the RLS tenant GUC on a single name
-- ============================================================
-- The portal grew two generations of row-level-security policies:
--   * early ones read current_setting('app.current_tenant')  (013, 023, 046...)
--   * later ones read current_setting('app.tenant_id')        (048, 049, 051...)
-- 052 only converted three policies, so both names were still live. A session
-- that sets only one of them silently gets zero rows (or, worse, an unscoped
-- query on the tables whose policy watches the other variable) and nothing
-- reports it. This migration rewrites every remaining policy to the canonical
-- name 'app.tenant_id' — no exceptions, so the Go side has exactly one knob.
--
-- Policies are rebuilt from pg_policies, which keeps their name, command type,
-- permissiveness, role list, USING and WITH CHECK expressions intact.

DO $$
DECLARE
    rec     RECORD;
    v_qual  TEXT;
    v_check TEXT;
    v_roles TEXT;
    v_moved INT := 0;
BEGIN
    FOR rec IN
        SELECT schemaname, tablename, policyname, permissive, cmd, roles, qual, with_check
        FROM pg_policies
        WHERE qual LIKE '%app.current_tenant%'
           OR with_check LIKE '%app.current_tenant%'
        ORDER BY schemaname, tablename, policyname
    LOOP
        v_qual  := replace(rec.qual, 'app.current_tenant', 'app.tenant_id');
        v_check := NULLIF(replace(COALESCE(rec.with_check, ''), 'app.current_tenant', 'app.tenant_id'), '');
        v_roles := NULLIF(array_to_string(rec.roles, ', '), '');

        EXECUTE format('DROP POLICY %I ON %I.%I', rec.policyname, rec.schemaname, rec.tablename);

        IF v_check IS NULL THEN
            EXECUTE format(
                'CREATE POLICY %I ON %I.%I AS %s FOR %s%s USING (%s)',
                rec.policyname, rec.schemaname, rec.tablename,
                lower(rec.permissive), lower(rec.cmd),
                CASE WHEN v_roles IS NULL THEN '' ELSE format(' TO %s', v_roles) END,
                v_qual
            );
        ELSE
            EXECUTE format(
                'CREATE POLICY %I ON %I.%I AS %s FOR %s%s USING (%s) WITH CHECK (%s)',
                rec.policyname, rec.schemaname, rec.tablename,
                lower(rec.permissive), lower(rec.cmd),
                CASE WHEN v_roles IS NULL THEN '' ELSE format(' TO %s', v_roles) END,
                v_qual, v_check
            );
        END IF;

        v_moved := v_moved + 1;
    END LOOP;

    RAISE NOTICE 'RLS: migrated % policies from app.current_tenant to app.tenant_id', v_moved;
END
$$;

-- Guard: the legacy GUC must not appear in any policy any more. Fail loudly
-- rather than leave a half-unified policy set behind, because a policy that
-- silently reads the wrong variable is indistinguishable from a working one.
DO $$
DECLARE
    v_leftover INT;
BEGIN
    SELECT COUNT(*) INTO v_leftover
    FROM pg_policies
    WHERE qual LIKE '%app.current_tenant%' OR with_check LIKE '%app.current_tenant%';

    IF v_leftover > 0 THEN
        RAISE EXCEPTION 'RLS unification incomplete: % policies still reference app.current_tenant', v_leftover;
    END IF;
END
$$;
