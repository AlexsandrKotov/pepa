-- 018: Enhance environments with type, cluster, namespace, status
-- and add environment_variables table for per-env KV management.

-- Add missing columns to environments
ALTER TABLE environments ADD COLUMN IF NOT EXISTS type VARCHAR(32) DEFAULT '';
ALTER TABLE environments ADD COLUMN IF NOT EXISTS cluster VARCHAR(128) DEFAULT '';
ALTER TABLE environments ADD COLUMN IF NOT EXISTS namespace VARCHAR(63) DEFAULT '';
ALTER TABLE environments ADD COLUMN IF NOT EXISTS status VARCHAR(32) DEFAULT 'active';

-- Environment variables table (per-environment KV store)
CREATE TABLE IF NOT EXISTS environment_variables (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID NOT NULL,
    env_id     UUID NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    key        VARCHAR(256) NOT NULL,
    value      TEXT DEFAULT '',
    is_secret  BOOLEAN DEFAULT FALSE,
    source     VARCHAR(64) DEFAULT 'manual',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (env_id, key)
);

CREATE INDEX IF NOT EXISTS idx_env_vars_env_id ON environment_variables(env_id);
CREATE INDEX IF NOT EXISTS idx_env_vars_tenant ON environment_variables(tenant_id);
