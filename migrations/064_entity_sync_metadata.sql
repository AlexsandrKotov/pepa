-- Migration 064: Entity sync metadata indexes
-- Adds indexes to support efficient entity sync operations

-- Index for looking up entities by external_id (used during sync)
CREATE INDEX IF NOT EXISTS idx_entities_external_id_lookup
    ON entities(external_id, type_key, tenant_id)
    WHERE deleted_at IS NULL;

-- Index for sync status filtering
CREATE INDEX IF NOT EXISTS idx_entities_sync_status
    ON entities(tenant_id, sync_status)
    WHERE deleted_at IS NULL;

-- Index for last_synced_at ordering
CREATE INDEX IF NOT EXISTS idx_entities_last_synced
    ON entities(tenant_id, last_synced_at DESC)
    WHERE deleted_at IS NULL;
