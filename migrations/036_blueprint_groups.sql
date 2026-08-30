-- ============================================================
-- Migration 036: Blueprint Groups + Docker Compose Support
-- ============================================================
-- Adds service grouping to blueprints for ordered group deploys,
-- and Docker Compose as a blueprint source type for deploying
-- to Docker hosts (not just Kubernetes clusters).
-- ============================================================

-- ── 1. Blueprint groups table ────────────────────────────────
CREATE TABLE IF NOT EXISTS blueprint_groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    name        VARCHAR(256) NOT NULL,
    description TEXT DEFAULT '',
    position    INTEGER DEFAULT 0,
    created_by  UUID,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_blueprint_groups_tenant ON blueprint_groups(tenant_id);
CREATE INDEX IF NOT EXISTS idx_blueprint_groups_position ON blueprint_groups(position);

-- ── 2. Add group association to service_blueprints ───────────
ALTER TABLE service_blueprints ADD COLUMN IF NOT EXISTS group_id UUID REFERENCES blueprint_groups(id) ON DELETE SET NULL;
ALTER TABLE service_blueprints ADD COLUMN IF NOT EXISTS group_position INTEGER DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_service_blueprints_group ON service_blueprints(group_id);

-- ── 3. Add Docker Compose support to blueprints ──────────────
ALTER TABLE service_blueprints ADD COLUMN IF NOT EXISTS compose_yaml TEXT DEFAULT '';
