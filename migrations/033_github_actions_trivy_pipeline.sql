-- ============================================================
-- GITHUB ACTIONS & TRIVY PIPELINE TYPES
-- ============================================================
-- Extends pipeline_sources to support GitHub Actions workflow
-- dispatching and Trivy vulnerability scanning as first-class
-- pipeline source types.

-- Add check constraint to document allowed source types
-- (the column is VARCHAR(32) so new types can still be added
--  without a migration, but this makes the intent explicit)
COMMENT ON COLUMN pipeline_sources.source_type IS
    'Pipeline engine type: gitlab_ci, gitlab, ansible, terraform, github_actions, trivy';

-- Index for fast lookup of GitHub Actions pipeline sources
CREATE INDEX IF NOT EXISTS idx_pipelinesource_github_actions
    ON pipeline_sources(source_type)
    WHERE source_type = 'github_actions';

-- Index for fast lookup of Trivy scanner pipeline sources
CREATE INDEX IF NOT EXISTS idx_pipelinesource_trivy
    ON pipeline_sources(source_type)
    WHERE source_type = 'trivy';
