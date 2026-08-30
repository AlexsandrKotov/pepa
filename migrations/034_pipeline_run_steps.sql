-- ============================================================
-- PIPELINE RUN JOB STEPS
-- ============================================================
-- Adds a JSONB column to pipeline_run_jobs for storing the
-- ordered list of steps within each job (GitHub Actions returns
-- a steps[] array inside every job object).
-- Also adds a unique partial index on pipeline_runs for upsert
-- by external_run_id per source.

ALTER TABLE pipeline_run_jobs
    ADD COLUMN IF NOT EXISTS steps JSONB DEFAULT '[]'::jsonb;

-- Unique partial index for upsert by (source_id, external_run_id)
CREATE UNIQUE INDEX IF NOT EXISTS idx_pipelinerun_source_external
    ON pipeline_runs (source_id, external_run_id)
    WHERE external_run_id IS NOT NULL AND external_run_id != '';
