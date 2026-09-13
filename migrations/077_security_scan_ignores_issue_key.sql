-- Migration: SonarQube finding ignores
-- scan_ignores.cve_id belongs to Trivy. SonarQube findings have no CVE, they are
-- suppressed by their issue key, or by "rule:<rule>" to silence a whole rule.
-- The two identifiers live in separate columns so each scanner keeps its own key
-- space and the export of an ignore list stays unambiguous.

ALTER TABLE scan_ignores ADD COLUMN IF NOT EXISTS issue_key TEXT;

COMMENT ON COLUMN scan_ignores.cve_id IS 'Trivy CVE identifier; empty for SonarQube ignores';
COMMENT ON COLUMN scan_ignores.issue_key IS 'SonarQube issue key, or "rule:<rule>" to suppress a whole rule; NULL for Trivy ignores';

-- The original UNIQUE(target_id, cve_id) would reject a second SonarQube ignore on
-- the same target: those rows keep cve_id empty (NOT NULL, but scanner-inapplicable)
-- and would all collide on it. Replace it with two partial uniques, one per
-- identifier kind. The constraint is looked up by type instead of by name so it
-- works on every environment.
DO $$
DECLARE
  con_name TEXT;
BEGIN
  SELECT c.conname INTO con_name
    FROM pg_constraint c
   WHERE c.conrelid = 'scan_ignores'::regclass
     AND c.contype = 'u'
   LIMIT 1;
  IF con_name IS NOT NULL THEN
    EXECUTE format('ALTER TABLE scan_ignores DROP CONSTRAINT %I', con_name);
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_scan_ignores_target_cve
    ON scan_ignores (target_id, cve_id)
    WHERE cve_id IS NOT NULL AND cve_id <> '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_scan_ignores_target_issue_key
    ON scan_ignores (target_id, issue_key)
    WHERE issue_key IS NOT NULL AND issue_key <> '';
