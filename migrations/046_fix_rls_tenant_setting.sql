-- 046_fix_rls_tenant_setting.sql
-- Fix RLS policies on ssh_hosts, ssh_host_groups, and ssh_host_group_members
-- to use the correct setting name (app.current_tenant) with missing_ok=true,
-- matching the convention established in migration 013.

-- ssh_hosts: fix tenant isolation policy
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'ssh_hosts_tenant_isolation' AND tablename = 'ssh_hosts'
    ) THEN
        DROP POLICY ssh_hosts_tenant_isolation ON ssh_hosts;
    END IF;
    CREATE POLICY ssh_hosts_tenant_isolation ON ssh_hosts
        USING (tenant_id = current_setting('app.current_tenant', true)::UUID);
END $$;

-- ssh_host_groups: fix tenant isolation policy
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'ssh_host_groups_tenant_isolation' AND tablename = 'ssh_host_groups'
    ) THEN
        DROP POLICY ssh_host_groups_tenant_isolation ON ssh_host_groups;
    END IF;
    CREATE POLICY ssh_host_groups_tenant_isolation ON ssh_host_groups
        USING (tenant_id = current_setting('app.current_tenant', true)::UUID);
END $$;

-- ssh_host_group_members: fix tenant isolation policy
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'ssh_host_group_members_tenant_isolation' AND tablename = 'ssh_host_group_members'
    ) THEN
        DROP POLICY ssh_host_group_members_tenant_isolation ON ssh_host_group_members;
    END IF;
    CREATE POLICY ssh_host_group_members_tenant_isolation ON ssh_host_group_members
        USING (group_id IN (
            SELECT id FROM ssh_host_groups WHERE tenant_id = current_setting('app.current_tenant', true)::UUID
        ));
END $$;
