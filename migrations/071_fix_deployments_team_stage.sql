-- 071: Restore deployments team/stage attribution (self-heal for init-db drift)
-- Databases created from init-db.sql record schema_migrations versions 1-29 as
-- applied, but its deployments definition predates migration 027 — so team_name
-- and stage never materialize and 027 is skipped forever. This migration
-- idempotently restores the columns and completes the environment_id backfill
-- that guarded migration 069 skipped when the stage column was absent.

ALTER TABLE deployments ADD COLUMN IF NOT EXISTS team_name VARCHAR(128) DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS stage VARCHAR(64) DEFAULT 'dev';

CREATE INDEX IF NOT EXISTS idx_deployments_tenant_team_stage
    ON deployments (tenant_id, team_name, stage);

-- Complete 069's environment_id backfill from stage slug matching
UPDATE deployments AS d
SET environment_id = e.id
FROM environments AS e
WHERE d.stage IS NOT NULL
  AND d.stage != ''
  AND d.environment_id IS NULL
  AND e.slug = d.stage
  AND e.tenant_id = d.tenant_id;
