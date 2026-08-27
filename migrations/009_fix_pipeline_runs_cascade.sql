-- Fix: pipeline_runs.source_id FK should cascade on delete
-- so that deleting a pipeline_source also cleans up its runs (and transitively run_jobs).

ALTER TABLE pipeline_runs
    DROP CONSTRAINT IF EXISTS pipeline_runs_source_id_fkey;

ALTER TABLE pipeline_runs
    ADD CONSTRAINT pipeline_runs_source_id_fkey
    FOREIGN KEY (source_id) REFERENCES pipeline_sources(id) ON DELETE CASCADE;

INSERT INTO schema_migrations (version, description) VALUES
    (9, 'Fix pipeline_runs FK to cascade on delete from pipeline_sources')
ON CONFLICT DO NOTHING;
