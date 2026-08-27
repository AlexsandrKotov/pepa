-- Migration 010: Vault security hardening
-- Adds tenant isolation and audit tracking to vault_secrets.
-- NOTE: These columns (tenant_id, created_by) already exist in init-db.sql for fresh installs.
-- This migration is for backward compatibility with databases created before those columns were added.
-- The ADD COLUMN IF NOT EXISTS and UPDATE are safe no-ops on fresh databases.

-- Add tenant_id for multi-tenant isolation (nullable for backward compat)
ALTER TABLE vault_secrets ADD COLUMN IF NOT EXISTS tenant_id UUID;

-- Add created_by to track who created each secret
ALTER TABLE vault_secrets ADD COLUMN IF NOT EXISTS created_by VARCHAR(255);

-- Index for tenant-scoped queries
CREATE INDEX IF NOT EXISTS idx_vault_secrets_tenant ON vault_secrets (tenant_id);

-- Backfill existing rows with the default tenant
UPDATE vault_secrets SET tenant_id = '00000000-0000-0000-0000-000000000002' WHERE tenant_id IS NULL;

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (10, 'Vault security hardening — tenant isolation and audit tracking')
ON CONFLICT DO NOTHING;
