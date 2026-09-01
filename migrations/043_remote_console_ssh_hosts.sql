-- Migration: Remote Console SSH hosts
-- Stores SSH host configurations for the remote-console plugin

CREATE TABLE IF NOT EXISTS ssh_hosts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    name VARCHAR(255) NOT NULL,
    hostname VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL DEFAULT 22,
    username VARCHAR(100) NOT NULL DEFAULT 'root',
    auth_method VARCHAR(50) NOT NULL DEFAULT 'password',
    -- auth_method: 'password', 'key', 'ldap_passthrough'
    ssh_key_enc TEXT,
    password_enc TEXT,
    tags TEXT[] DEFAULT '{}',
    description TEXT DEFAULT '',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for tenant-scoped lookups
CREATE INDEX IF NOT EXISTS idx_ssh_hosts_tenant ON ssh_hosts(tenant_id);

-- Index for tag-based filtering
CREATE INDEX IF NOT EXISTS idx_ssh_hosts_tags ON ssh_hosts USING GIN(tags);

-- Row Level Security
ALTER TABLE ssh_hosts ENABLE ROW LEVEL SECURITY;

-- Policy: users can see hosts in their tenant
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies WHERE policyname = 'ssh_hosts_tenant_isolation' AND tablename = 'ssh_hosts'
    ) THEN
        CREATE POLICY ssh_hosts_tenant_isolation ON ssh_hosts
            USING (tenant_id = current_setting('app.tenant_id')::uuid);
    END IF;
END $$;
