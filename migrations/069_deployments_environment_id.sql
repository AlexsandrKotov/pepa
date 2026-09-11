-- 069: Add environment_id to deployments table
-- Links GitOps workflow deployments to environments for proper visibility
-- on the environment detail pages.

-- Add environment_id column (nullable for backward compatibility)
ALTER TABLE deployments
    ADD COLUMN IF NOT EXISTS environment_id UUID;

-- Add index for fast lookups by environment
CREATE INDEX IF NOT EXISTS idx_deployments_environment_id
    ON deployments (environment_id)
    WHERE environment_id IS NOT NULL;

-- Backfill: resolve environment_id from stage slug matching environments.slug
UPDATE deployments AS d
SET environment_id = e.id
FROM environments AS e
WHERE d.stage IS NOT NULL
  AND d.stage != ''
  AND d.environment_id IS NULL
  AND e.slug = d.stage
  AND e.tenant_id = d.tenant_id;
