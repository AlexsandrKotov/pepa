-- Add timeout_seconds column to deployments table
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS timeout_seconds INTEGER DEFAULT 300;
