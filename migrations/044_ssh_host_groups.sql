-- Migration: SSH Host Groups
-- Adds grouping support for SSH hosts in the remote-console plugin

-- Groups table
CREATE TABLE IF NOT EXISTS ssh_host_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    name VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    color VARCHAR(7) DEFAULT '#7aa2f7',  -- hex color for UI badge
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ssh_host_groups_tenant ON ssh_host_groups(tenant_id);

-- Many-to-many: hosts <-> groups
CREATE TABLE IF NOT EXISTS ssh_host_group_members (
    group_id UUID NOT NULL REFERENCES ssh_host_groups(id) ON DELETE CASCADE,
    host_id  UUID NOT NULL REFERENCES ssh_hosts(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, host_id)
);

CREATE INDEX IF NOT EXISTS idx_ssh_host_group_members_host ON ssh_host_group_members(host_id);
CREATE INDEX IF NOT EXISTS idx_ssh_host_group_members_group ON ssh_host_group_members(group_id);

-- Prevent duplicate group names per tenant
CREATE UNIQUE INDEX IF NOT EXISTS idx_ssh_host_groups_tenant_name 
    ON ssh_host_groups(tenant_id, LOWER(name));

-- RLS
ALTER TABLE ssh_host_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE ssh_host_group_members ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'ssh_host_groups_tenant_isolation' AND tablename = 'ssh_host_groups'
    ) THEN
        CREATE POLICY ssh_host_groups_tenant_isolation ON ssh_host_groups
            USING (tenant_id = current_setting('app.tenant_id')::uuid);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'ssh_host_group_members_tenant_isolation' AND tablename = 'ssh_host_group_members'
    ) THEN
        -- Members are scoped to the host's tenant
        CREATE POLICY ssh_host_group_members_tenant_isolation ON ssh_host_group_members
            USING (group_id IN (
                SELECT id FROM ssh_host_groups WHERE tenant_id = current_setting('app.tenant_id')::uuid
            ));
    END IF;
END $$;
