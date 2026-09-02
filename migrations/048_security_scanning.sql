-- Migration: Security Scanning Infrastructure
-- Creates tables for managing scan targets, scan runs, and scan schedules
-- Supports Trivy and SonarQube integration with scheduling and reporting

-- scan_targets: what to scan
CREATE TABLE scan_targets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  name TEXT NOT NULL,
  scanner_type TEXT NOT NULL,          -- 'trivy' | 'sonarqube' | 'both'
  target_type TEXT NOT NULL,           -- 'image' | 'git_repo' | 'filesystem' | 'container' | 'service' | 'sonarqube_project'
  target_ref TEXT NOT NULL,            -- image name, repo URL, service ID, project key, etc.
  connection_id UUID,                  -- optional link to connections table
  scan_config JSONB DEFAULT '{}',      -- scanner-specific config overrides
  enabled BOOLEAN DEFAULT true,
  last_scan_at TIMESTAMPTZ,
  last_scan_status TEXT,
  last_scan_summary JSONB,
  created_by UUID,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- scan_runs: individual scan executions with persisted results
CREATE TABLE scan_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  target_id UUID REFERENCES scan_targets(id) ON DELETE CASCADE,
  scanner_type TEXT NOT NULL,
  status TEXT DEFAULT 'pending',       -- pending|running|completed|failed|cancelled
  trigger_type TEXT DEFAULT 'manual',  -- manual|schedule|pipeline|webhook
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  duration_ms INT,
  result_summary JSONB,                -- severity counts, quality gate status, etc.
  result_full JSONB,                   -- complete scan output
  error_message TEXT,
  report_url TEXT,                     -- generated report path
  created_at TIMESTAMPTZ DEFAULT now()
);

-- scan_schedules: cron-based recurring scan configs
CREATE TABLE scan_schedules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  target_id UUID NOT NULL REFERENCES scan_targets(id) ON DELETE CASCADE,
  cron_expression TEXT NOT NULL,
  enabled BOOLEAN DEFAULT true,
  last_run_at TIMESTAMPTZ,
  next_run_at TIMESTAMPTZ,
  workflow_id UUID,                    -- linked workflow execution
  created_by UUID,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- Indexes for performance
CREATE INDEX idx_scan_targets_tenant ON scan_targets(tenant_id);
CREATE INDEX idx_scan_runs_target ON scan_runs(target_id);
CREATE INDEX idx_scan_runs_tenant_status ON scan_runs(tenant_id, status);
CREATE INDEX idx_scan_schedules_next ON scan_schedules(next_run_at) WHERE enabled = true;

-- Add RLS policies
ALTER TABLE scan_targets ENABLE ROW LEVEL SECURITY;
ALTER TABLE scan_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE scan_schedules ENABLE ROW LEVEL SECURITY;

-- Policies for scan_targets
CREATE POLICY "Users can view scan targets in their tenant"
  ON scan_targets FOR SELECT
  USING (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY "Users can create scan targets in their tenant"
  ON scan_targets FOR INSERT
  WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY "Users can update scan targets in their tenant"
  ON scan_targets FOR UPDATE
  USING (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY "Users can delete scan targets in their tenant"
  ON scan_targets FOR DELETE
  USING (tenant_id = current_setting('app.tenant_id')::uuid);

-- Policies for scan_runs
CREATE POLICY "Users can view scan runs in their tenant"
  ON scan_runs FOR SELECT
  USING (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY "Users can create scan runs in their tenant"
  ON scan_runs FOR INSERT
  WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY "Users can update scan runs in their tenant"
  ON scan_runs FOR UPDATE
  USING (tenant_id = current_setting('app.tenant_id')::uuid);

-- Policies for scan_schedules
CREATE POLICY "Users can view scan schedules in their tenant"
  ON scan_schedules FOR SELECT
  USING (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY "Users can create scan schedules in their tenant"
  ON scan_schedules FOR INSERT
  WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY "Users can update scan schedules in their tenant"
  ON scan_schedules FOR UPDATE
  USING (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY "Users can delete scan schedules in their tenant"
  ON scan_schedules FOR DELETE
  USING (tenant_id = current_setting('app.tenant_id')::uuid);
