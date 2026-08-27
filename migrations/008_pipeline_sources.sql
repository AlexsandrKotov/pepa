-- ============================================================
-- PIPELINE SOURCES, PRESETS, RUNS & JOBS
-- ============================================================
-- Adds support for launching and tracking external pipelines
-- (GitLab CI, Ansible, Terraform) from within PEPA.

-- Pipeline Sources: connections to external pipeline definitions
CREATE TABLE IF NOT EXISTS pipeline_sources (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL,
    name              VARCHAR(256) NOT NULL,
    source_type       VARCHAR(32) NOT NULL,   -- 'gitlab_ci', 'ansible', 'terraform'
    description       TEXT DEFAULT '',
    connection_id     UUID REFERENCES connections(id) ON DELETE SET NULL,
    config            JSONB NOT NULL DEFAULT '{}',
    parameter_schema  JSONB DEFAULT '{}',
    schema_fetched_at TIMESTAMPTZ,
    status            VARCHAR(32) DEFAULT 'active',  -- active, error, disabled
    last_error        TEXT,
    created_by        UUID,
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    updated_at        TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pipelinesource_tenant ON pipeline_sources(tenant_id);
CREATE INDEX IF NOT EXISTS idx_pipelinesource_type ON pipeline_sources(source_type);

-- Pipeline Presets: saved parameter sets for quick re-use
CREATE TABLE IF NOT EXISTS pipeline_presets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    source_id    UUID NOT NULL REFERENCES pipeline_sources(id) ON DELETE CASCADE,
    name         VARCHAR(256) NOT NULL,
    description  TEXT DEFAULT '',
    parameters   JSONB NOT NULL DEFAULT '{}',
    created_by   UUID,
    use_count    INTEGER DEFAULT 0,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (source_id, name)
);

CREATE INDEX IF NOT EXISTS idx_presets_source ON pipeline_presets(source_id);

-- Pipeline Runs: history of every pipeline execution
CREATE TABLE IF NOT EXISTS pipeline_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    source_id       UUID NOT NULL REFERENCES pipeline_sources(id) ON DELETE CASCADE,
    preset_id       UUID REFERENCES pipeline_presets(id) ON DELETE SET NULL,
    external_run_id VARCHAR(256),
    external_url    TEXT,
    parameters      JSONB NOT NULL DEFAULT '{}',
    status          VARCHAR(32) DEFAULT 'pending',
    external_status VARCHAR(64),
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    duration_ms     INTEGER,
    logs            TEXT,
    logs_url        TEXT,
    job_details     JSONB,
    triggered_by    UUID,
    trigger_type    VARCHAR(32) DEFAULT 'manual',
    error_message   TEXT,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pipelinerun_source ON pipeline_runs(source_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pipelinerun_status ON pipeline_runs(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_pipelinerun_external ON pipeline_runs(external_run_id);

-- Pipeline Run Jobs: per-step/job detail within a run
CREATE TABLE IF NOT EXISTS pipeline_run_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id          UUID NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
    external_job_id VARCHAR(256),
    name            VARCHAR(256) NOT NULL,
    stage           VARCHAR(128),
    status          VARCHAR(32) DEFAULT 'pending',
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    duration_ms     INTEGER,
    log_text        TEXT,
    log_url         TEXT,
    runner_name     VARCHAR(256),
    allow_failure   BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_runjob_run ON pipeline_run_jobs(run_id);

-- Schema version
INSERT INTO schema_migrations (version, description) VALUES
    (8, 'Add pipeline_sources, pipeline_presets, pipeline_runs, pipeline_run_jobs')
ON CONFLICT DO NOTHING;
