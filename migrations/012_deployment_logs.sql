-- Add error_message and logs columns to deployments table
-- This enables tracking of deployment failures and success details

ALTER TABLE deployments ADD COLUMN IF NOT EXISTS error_message TEXT;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS logs TEXT;

-- Update schema version
INSERT INTO schema_migrations (version, description) VALUES
    (12, 'Add error_message and logs to deployments for debugging')
ON CONFLICT DO NOTHING;
