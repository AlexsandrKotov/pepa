-- ============================================================
-- Migration 022: Service Blueprints (Pipeline Blueprints)
-- ============================================================
-- Moves pipeline blueprints from browser localStorage to database.
-- Blueprints are reusable service definitions with pre-configured
-- values.yaml, images, and resource configs for the Pipeline Builder.
-- ============================================================

-- ── 1. Service blueprints table ───────────────────────────────
CREATE TABLE IF NOT EXISTS service_blueprints (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(256) NOT NULL,
    description     TEXT DEFAULT '',
    source_type     VARCHAR(32) NOT NULL DEFAULT 'container',
    helm_repo_id    UUID,
    image           TEXT DEFAULT '',
    chart_url       TEXT DEFAULT '',
    chart_name      VARCHAR(256) DEFAULT '',
    chart_version   VARCHAR(64) DEFAULT '',
    chart_path      TEXT DEFAULT '',
    namespace       VARCHAR(256) DEFAULT 'default',
    values_yaml     TEXT DEFAULT '',
    cpu             VARCHAR(32) DEFAULT '100m',
    memory          VARCHAR(32) DEFAULT '128Mi',
    replicas        INTEGER DEFAULT 1,
    ports           INTEGER[] DEFAULT '{}',
    category        VARCHAR(64) DEFAULT 'general',
    created_by      UUID,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Index for category filtering
CREATE INDEX IF NOT EXISTS idx_service_blueprints_category ON service_blueprints(category);
CREATE INDEX IF NOT EXISTS idx_service_blueprints_source_type ON service_blueprints(source_type);
