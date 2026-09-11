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

-- Backfill: resolve environment_id from stage slug matching environments.slug.
-- Guarded: databases initialized from init-db.sql record versions 1-29 as applied
-- while its deployments definition predates 027, so the stage column may be absent
-- here; the backfill is completed by 071 once that column is restored.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'deployments' AND column_name = 'stage'
    ) THEN
        UPDATE deployments AS d
        SET environment_id = e.id
        FROM environments AS e
        WHERE d.stage IS NOT NULL
          AND d.stage != ''
          AND d.environment_id IS NULL
          AND e.slug = d.stage
          AND e.tenant_id = d.tenant_id;
    END IF;
END $$;
