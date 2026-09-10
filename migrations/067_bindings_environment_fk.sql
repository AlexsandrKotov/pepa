-- 067_bindings_environment_fk.sql
-- Add environment_id soft reference to gitops_application_bindings, linking to environments table.
-- No DB-enforced FK constraint (multi-tenant design); integrity is maintained at the application layer.
-- The old string column `environment` is kept for backward compatibility but deprecated.

-- Add environment_id column (nullable for backward compatibility)
ALTER TABLE gitops_application_bindings
    ADD COLUMN IF NOT EXISTS environment_id UUID;

-- Backfill: if environment string matches a known environment slug, set environment_id
UPDATE gitops_application_bindings AS b
SET environment_id = e.id
FROM environments AS e
WHERE b.environment IS NOT NULL
  AND b.environment != ''
  AND b.environment_id IS NULL
  AND e.slug = b.environment
  AND e.tenant_id = b.tenant_id;

-- Add index for fast lookups by environment
CREATE INDEX IF NOT EXISTS idx_gob_environment_id
    ON gitops_application_bindings (environment_id)
    WHERE environment_id IS NOT NULL;

-- Add index for fast lookups by service
CREATE INDEX IF NOT EXISTS idx_gob_service_id
    ON gitops_application_bindings (service_id)
    WHERE service_id IS NOT NULL;
