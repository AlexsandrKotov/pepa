-- ============================================================
-- Migration 038: Blueprint Groups Many-to-Many
-- ============================================================
-- Changes blueprint-group relationship from one-to-many (group_id
-- column on service_blueprints) to many-to-many via a junction
-- table, so the same blueprint can belong to multiple groups.
-- ============================================================

-- ── 1. Create junction table ─────────────────────────────────
CREATE TABLE IF NOT EXISTS blueprint_group_members (
    group_id     UUID NOT NULL REFERENCES blueprint_groups(id) ON DELETE CASCADE,
    blueprint_id UUID NOT NULL REFERENCES service_blueprints(id) ON DELETE CASCADE,
    position     INTEGER DEFAULT 0,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (group_id, blueprint_id)
);

CREATE INDEX IF NOT EXISTS idx_blueprint_group_members_group ON blueprint_group_members(group_id);
CREATE INDEX IF NOT EXISTS idx_blueprint_group_members_blueprint ON blueprint_group_members(blueprint_id);

-- ── 2. Migrate existing group_id data ────────────────────────
INSERT INTO blueprint_group_members (group_id, blueprint_id, position)
SELECT group_id, id, COALESCE(group_position, 0)
FROM service_blueprints
WHERE group_id IS NOT NULL
ON CONFLICT (group_id, blueprint_id) DO NOTHING;
