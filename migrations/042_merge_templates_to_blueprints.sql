-- ============================================================
-- Migration 042: Merge Service Templates into Blueprints
-- ============================================================
-- Unifies service_templates and service_blueprints into a single
-- "Blueprint" concept. System templates become system blueprints
-- (read-only, provided by PEPA). User blueprints remain editable.
-- ============================================================

-- ── 1. Extend service_blueprints with template metadata ───────
ALTER TABLE service_blueprints
    ADD COLUMN IF NOT EXISTS slug             VARCHAR(128),
    ADD COLUMN IF NOT EXISTS tenant_id        UUID,
    ADD COLUMN IF NOT EXISTS icon             VARCHAR(256),
    ADD COLUMN IF NOT EXISTS language         VARCHAR(64),
    ADD COLUMN IF NOT EXISTS framework        VARCHAR(64),
    ADD COLUMN IF NOT EXISTS tags             VARCHAR(64)[] DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS is_enabled       BOOLEAN DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS is_system        BOOLEAN DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS dockerfile_tmpl  TEXT DEFAULT '',
    ADD COLUMN IF NOT EXISTS helm_chart       JSONB DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS cicd_tmpl        TEXT DEFAULT '',
    ADD COLUMN IF NOT EXISTS default_values   JSONB DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS resource_defaults JSONB DEFAULT '{"cpu":"100m","memory":"128Mi","replicas":1}';

-- Indexes for filtering
CREATE INDEX IF NOT EXISTS idx_blueprints_slug ON service_blueprints(slug) WHERE slug IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_blueprints_tenant ON service_blueprints(tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_blueprints_system ON service_blueprints(is_system);

-- ── 2. Seed system blueprints from existing templates ─────────
INSERT INTO service_blueprints (
    slug, tenant_id, name, description, category, icon, language, framework,
    tags, is_enabled, is_system, dockerfile_tmpl, helm_chart, cicd_tmpl,
    default_values, resource_defaults,
    source_type, image, chart_url, chart_name, chart_version,
    namespace, cpu, memory, replicas
)
SELECT
    t.slug,
    t.tenant_id,
    t.name,
    t.description,
    t.category,
    t.icon,
    t.language,
    t.framework,
    t.tags,
    t.is_enabled,
    TRUE AS is_system,
    COALESCE(t.dockerfile_tmpl, ''),
    t.helm_chart,
    COALESCE(t.cicd_tmpl, ''),
    t.default_values,
    t.resource_defaults,
    -- Determine source_type from helm_chart JSON
    CASE
        WHEN t.helm_chart IS NOT NULL AND t.helm_chart::text != '{}' AND t.helm_chart->>'repo_url' IS NOT NULL THEN 'helm_http'
        WHEN t.helm_chart IS NOT NULL AND t.helm_chart::text != '{}' AND t.helm_chart->>'image' IS NOT NULL THEN 'container'
        ELSE 'container'
    END,
    COALESCE(t.helm_chart->>'image', ''),
    COALESCE(t.helm_chart->>'repo_url', ''),
    COALESCE(t.helm_chart->>'chart_name', ''),
    COALESCE(t.helm_chart->>'chart_version', ''),
    'default',
    COALESCE(t.resource_defaults->>'cpu', '100m'),
    COALESCE(t.resource_defaults->>'memory', '128Mi'),
    COALESCE((t.resource_defaults->>'replicas')::int, 1)
FROM service_templates t
WHERE t.is_system = TRUE
  AND NOT EXISTS (
      SELECT 1 FROM service_blueprints sb WHERE sb.slug = t.slug
  );
