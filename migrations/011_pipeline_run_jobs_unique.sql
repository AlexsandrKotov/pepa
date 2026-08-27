-- Add unique index on pipeline_run_jobs for upsert support
-- Only index non-empty external_job_id values
CREATE UNIQUE INDEX IF NOT EXISTS idx_runjob_run_external
    ON pipeline_run_jobs (run_id, external_job_id)
    WHERE external_job_id IS NOT NULL AND external_job_id != '';

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (11, 'Add unique partial index on pipeline_run_jobs for upsert support')
ON CONFLICT DO NOTHING;
