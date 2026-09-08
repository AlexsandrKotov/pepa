-- Migration: Scan Ignores (CVE Ignore Lists)
-- Allows users to ignore specific CVEs per scan target
-- Generates .trivyignore files for Trivy scanner

CREATE TABLE IF NOT EXISTS scan_ignores (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  target_id UUID NOT NULL REFERENCES scan_targets(id) ON DELETE CASCADE,
  cve_id VARCHAR(50) NOT NULL,
  reason TEXT,
  created_by UUID REFERENCES users(id),
  created_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE(target_id, cve_id)
);

CREATE INDEX IF NOT EXISTS idx_scan_ignores_tenant ON scan_ignores(tenant_id);
CREATE INDEX IF NOT EXISTS idx_scan_ignores_target ON scan_ignores(target_id);

-- RLS policies
ALTER TABLE scan_ignores ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users can view scan ignores in their tenant"
  ON scan_ignores FOR SELECT
  USING (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY "Users can create scan ignores in their tenant"
  ON scan_ignores FOR INSERT
  WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY "Users can delete scan ignores in their tenant"
  ON scan_ignores FOR DELETE
  USING (tenant_id = current_setting('app.tenant_id')::uuid);
