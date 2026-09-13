-- Transactional outbox pattern for reliable event publishing.
-- Events are written to this table within the same transaction as the business operation,
-- then a relay process reads and publishes them to the event bus asynchronously.
-- This guarantees at-least-once delivery without distributed transactions.

CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type TEXT NOT NULL,
    aggregate_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    tenant_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    published BOOLEAN NOT NULL DEFAULT FALSE,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 5,
    last_error TEXT
);

-- Index for relay polling: find unpublished events ordered by creation time
CREATE INDEX IF NOT EXISTS idx_outbox_events_unpublished
    ON outbox_events (created_at ASC)
    WHERE published = false AND retry_count < max_retries;

-- Index for tenant-scoped queries
CREATE INDEX IF NOT EXISTS idx_outbox_events_tenant
    ON outbox_events (tenant_id, created_at DESC);

-- Index for aggregate lookups (e.g., all events for a specific deployment)
CREATE INDEX IF NOT EXISTS idx_outbox_events_aggregate
    ON outbox_events (aggregate_type, aggregate_id, created_at DESC);

-- RLS policy: tenant isolation
ALTER TABLE outbox_events ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS outbox_events_tenant_isolation ON outbox_events;
CREATE POLICY outbox_events_tenant_isolation ON outbox_events
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid);
