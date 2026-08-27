-- ============================================================
-- Migration 004: Scorecards
-- ============================================================
-- Scorecards, scorecard rules, and scorecard results for
-- evaluating entity quality and readiness.
-- ============================================================

CREATE TABLE IF NOT EXISTS scorecards (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    name        VARCHAR(256) NOT NULL,
    description TEXT,
    enabled     BOOLEAN DEFAULT TRUE,
    config      JSONB DEFAULT '{}',
    created_by  UUID,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (name, tenant_id)
);

CREATE TABLE IF NOT EXISTS scorecard_rules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scorecard_id UUID NOT NULL REFERENCES scorecards(id) ON DELETE CASCADE,
    name         VARCHAR(256) NOT NULL,
    description  TEXT,
    expression   TEXT NOT NULL,
    weight       INTEGER DEFAULT 5,
    pass_message VARCHAR(512),
    fail_message VARCHAR(512),
    severity     VARCHAR(16) DEFAULT 'warning',
    metadata     JSONB DEFAULT '{}',
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS scorecard_results (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scorecard_id UUID NOT NULL REFERENCES scorecards(id) ON DELETE CASCADE,
    entity_id    UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    tenant_id    UUID NOT NULL,
    score        INTEGER DEFAULT 0,
    max_score    INTEGER DEFAULT 0,
    pass_count   INTEGER DEFAULT 0,
    fail_count   INTEGER DEFAULT 0,
    total_rules  INTEGER DEFAULT 0,
    level        VARCHAR(16) DEFAULT 'none',
    details      JSONB DEFAULT '[]',
    evaluated_at TIMESTAMPTZ DEFAULT NOW(),
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scorecard_tenant ON scorecards(tenant_id);
CREATE INDEX IF NOT EXISTS idx_scorecard_rules_sc ON scorecard_rules(scorecard_id);
CREATE INDEX IF NOT EXISTS idx_scorecard_result_entity ON scorecard_results(entity_id, scorecard_id);
CREATE INDEX IF NOT EXISTS idx_scorecard_result_tenant ON scorecard_results(tenant_id, evaluated_at DESC);

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (4, 'Add scorecards, scorecard_rules, scorecard_results')
ON CONFLICT DO NOTHING;
