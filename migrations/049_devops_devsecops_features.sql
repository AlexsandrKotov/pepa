-- Migration 049: DevOps & DevSecOps Features
-- Adds deployment windows, batch operations, compliance policies,
-- security findings, secret rotations, and deployment audit trail.

-- ============================================================
-- Deployment Windows & Freeze Periods
-- Controls when deployments are allowed/blocked
-- ============================================================
CREATE TABLE IF NOT EXISTS deployment_windows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    window_type VARCHAR(50) NOT NULL DEFAULT 'allowed', -- 'allowed' | 'blocked' | 'freeze'
    -- Cron expression for recurring windows (e.g., "0 9 * * 1-5" = weekdays 9am)
    cron_expression VARCHAR(100),
    -- Or specific date range for one-time windows
    start_at TIMESTAMPTZ,
    end_at TIMESTAMPTZ,
    -- Timezone for the window (e.g., "Europe/Moscow")
    timezone VARCHAR(50) DEFAULT 'UTC',
    -- Affected environments (empty = all)
    environments TEXT[] DEFAULT '{}',
    -- Affected services (empty = all)
    service_ids UUID[] DEFAULT '{}',
    -- Whether this window is enabled
    enabled BOOLEAN NOT NULL DEFAULT true,
    -- Priority for overlapping windows (higher = takes precedence)
    priority INT DEFAULT 0,
    -- Reason for freeze/blocked windows
    reason TEXT,
    -- Who can override this window
    override_roles TEXT[] DEFAULT '{}',
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_deployment_windows_tenant ON deployment_windows(tenant_id);
CREATE INDEX idx_deployment_windows_enabled ON deployment_windows(enabled);
CREATE INDEX idx_deployment_windows_type ON deployment_windows(window_type);

-- ============================================================
-- Batch Operations
-- Track mass restart/rollback/scale operations during incidents
-- ============================================================
CREATE TABLE IF NOT EXISTS batch_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    operation_type VARCHAR(50) NOT NULL, -- 'restart' | 'rollback' | 'scale' | 'custom'
    status VARCHAR(50) NOT NULL DEFAULT 'pending', -- 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
    -- Target services
    service_ids UUID[] NOT NULL DEFAULT '{}',
    -- Operation parameters
    parameters JSONB DEFAULT '{}',
    -- Execution results per service
    results JSONB DEFAULT '{}',
    -- Progress tracking
    total_count INT DEFAULT 0,
    completed_count INT DEFAULT 0,
    failed_count INT DEFAULT 0,
    -- Who initiated the operation
    initiated_by UUID REFERENCES users(id),
    -- Reason (e.g., incident response)
    reason TEXT,
    -- Incident reference
    incident_id VARCHAR(100),
    -- Timeout for the entire batch
    timeout_seconds INT DEFAULT 3600,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_batch_operations_tenant ON batch_operations(tenant_id);
CREATE INDEX idx_batch_operations_status ON batch_operations(status);
CREATE INDEX idx_batch_operations_initiated ON batch_operations(initiated_by);

-- ============================================================
-- Compliance Policies
-- Policy-as-code for deployment validation
-- ============================================================
CREATE TABLE IF NOT EXISTS compliance_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    policy_type VARCHAR(50) NOT NULL, -- 'resource_limits' | 'security_scan' | 'approval' | 'custom'
    -- Policy definition (JSON/YAML/CEL expression)
    policy_spec JSONB NOT NULL,
    -- Severity when violated: 'block' | 'warn' | 'info'
    severity VARCHAR(20) NOT NULL DEFAULT 'warn',
    -- Whether this policy blocks deployments when violated
    blocking BOOLEAN NOT NULL DEFAULT false,
    -- Affected environments (empty = all)
    environments TEXT[] DEFAULT '{}',
    -- Affected services (empty = all)
    service_ids UUID[] DEFAULT '{}',
    -- Whether this policy is enabled
    enabled BOOLEAN NOT NULL DEFAULT true,
    -- Last evaluation result
    last_evaluated_at TIMESTAMPTZ,
    last_violation_count INT DEFAULT 0,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_compliance_policies_tenant ON compliance_policies(tenant_id);
CREATE INDEX idx_compliance_policies_enabled ON compliance_policies(enabled);
CREATE INDEX idx_compliance_policies_type ON compliance_policies(policy_type);

-- Policy evaluation results
CREATE TABLE IF NOT EXISTS compliance_evaluations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    policy_id UUID NOT NULL REFERENCES compliance_policies(id) ON DELETE CASCADE,
    service_id UUID REFERENCES services(id) ON DELETE CASCADE,
    deployment_id UUID REFERENCES service_deployments(id) ON DELETE CASCADE,
    -- 'pass' | 'fail' | 'warn' | 'skip'
    result VARCHAR(20) NOT NULL,
    -- Details of violations
    violations JSONB DEFAULT '[]',
    -- Context at evaluation time
    context JSONB DEFAULT '{}',
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_compliance_evaluations_tenant ON compliance_evaluations(tenant_id);
CREATE INDEX idx_compliance_evaluations_policy ON compliance_evaluations(policy_id);
CREATE INDEX idx_compliance_evaluations_service ON compliance_evaluations(service_id);

-- ============================================================
-- Security Findings
-- Aggregated security findings from scans
-- ============================================================
CREATE TABLE IF NOT EXISTS security_findings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    scan_run_id UUID REFERENCES scan_runs(id) ON DELETE CASCADE,
    -- Finding type: 'vulnerability' | 'misconfiguration' | 'secret' | 'license'
    finding_type VARCHAR(50) NOT NULL,
    -- Severity: 'critical' | 'high' | 'medium' | 'low' | 'info'
    severity VARCHAR(20) NOT NULL,
    -- Finding details
    title VARCHAR(500) NOT NULL,
    description TEXT,
    -- CVE/identifier
    identifier VARCHAR(100),
    -- Affected resource (image, file, service)
    resource_type VARCHAR(50),
    resource_name VARCHAR(500),
    -- Fix information
    fix_available BOOLEAN DEFAULT false,
    fix_version VARCHAR(100),
    fix_instructions TEXT,
    -- Status: 'open' | 'acknowledged' | 'fixed' | 'false_positive' | 'risk_accepted'
    status VARCHAR(30) NOT NULL DEFAULT 'open',
    -- Who acknowledged/accepted risk
    resolved_by UUID REFERENCES users(id),
    resolved_at TIMESTAMPTZ,
    resolution_notes TEXT,
    -- First and last seen
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_security_findings_tenant ON security_findings(tenant_id);
CREATE INDEX idx_security_findings_severity ON security_findings(severity);
CREATE INDEX idx_security_findings_status ON security_findings(status);
CREATE INDEX idx_security_findings_type ON security_findings(finding_type);
CREATE INDEX idx_security_findings_scan ON security_findings(scan_run_id);

-- ============================================================
-- Secret Rotations
-- Track secret rotation schedules and history
-- ============================================================
CREATE TABLE IF NOT EXISTS secret_rotations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    -- Vault path or secret reference
    secret_path VARCHAR(500) NOT NULL,
    -- Rotation configuration
    rotation_type VARCHAR(50) NOT NULL, -- 'scheduled' | 'on_demand' | 'on_expiry'
    -- Cron expression for scheduled rotations
    cron_expression VARCHAR(100),
    -- Rotation interval (for scheduled)
    rotation_interval_days INT,
    -- Last rotation info
    last_rotated_at TIMESTAMPTZ,
    last_rotated_by UUID REFERENCES users(id),
    -- Next scheduled rotation
    next_rotation_at TIMESTAMPTZ,
    -- Expiry tracking
    expires_at TIMESTAMPTZ,
    -- Status
    status VARCHAR(50) NOT NULL DEFAULT 'active', -- 'active' | 'paused' | 'expired' | 'failed'
    -- Rotation history
    rotation_count INT DEFAULT 0,
    -- Affected services
    service_ids UUID[] DEFAULT '{}',
    -- Whether this rotation is enabled
    enabled BOOLEAN NOT NULL DEFAULT true,
    -- Error info
    last_error TEXT,
    last_error_at TIMESTAMPTZ,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_secret_rotations_tenant ON secret_rotations(tenant_id);
CREATE INDEX idx_secret_rotations_enabled ON secret_rotations(enabled);
CREATE INDEX idx_secret_rotations_next ON secret_rotations(next_rotation_at);
CREATE INDEX idx_secret_rotations_expires ON secret_rotations(expires_at);

-- Rotation execution history
CREATE TABLE IF NOT EXISTS secret_rotation_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rotation_id UUID NOT NULL REFERENCES secret_rotations(id) ON DELETE CASCADE,
    -- 'success' | 'failed' | 'skipped'
    status VARCHAR(20) NOT NULL,
    -- Details
    details JSONB DEFAULT '{}',
    error_message TEXT,
    -- Who triggered (manual or system)
    triggered_by UUID REFERENCES users(id),
    trigger_type VARCHAR(50) DEFAULT 'scheduled', -- 'scheduled' | 'manual' | 'on_expiry'
    executed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_secret_rotation_logs_rotation ON secret_rotation_logs(rotation_id);

-- ============================================================
-- Deployment Audit Trail
-- Detailed audit for deployments (separate from general audit)
-- ============================================================
CREATE TABLE IF NOT EXISTS deployment_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    -- Deployment reference
    deployment_id UUID REFERENCES service_deployments(id) ON DELETE SET NULL,
    service_id UUID REFERENCES services(id) ON DELETE SET NULL,
    -- Action details
    action VARCHAR(100) NOT NULL, -- 'deploy' | 'rollback' | 'promote' | 'scale' | 'restart' | 'cancel' | 'verify'
    -- Actor
    actor_type VARCHAR(50) NOT NULL, -- 'user' | 'system' | 'workflow' | 'api_key'
    actor_id UUID,
    actor_name VARCHAR(255),
    -- Context
    environment VARCHAR(100),
    cluster_id UUID,
    namespace VARCHAR(255),
    image_tag VARCHAR(255),
    -- Before/after state
    previous_state JSONB,
    new_state JSONB,
    -- Compliance check results
    compliance_results JSONB DEFAULT '{}',
    -- Security gate results
    security_gate_results JSONB DEFAULT '{}',
    -- Risk assessment
    risk_score INT,
    risk_factors JSONB DEFAULT '{}',
    -- Outcome
    status VARCHAR(50) NOT NULL DEFAULT 'success', -- 'success' | 'failed' | 'blocked' | 'rolled_back'
    error_message TEXT,
    -- Additional metadata
    metadata JSONB DEFAULT '{}',
    ip_address VARCHAR(50),
    user_agent TEXT,
    request_id VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_deployment_audit_tenant ON deployment_audit_logs(tenant_id);
CREATE INDEX idx_deployment_audit_deployment ON deployment_audit_logs(deployment_id);
CREATE INDEX idx_deployment_audit_service ON deployment_audit_logs(service_id);
CREATE INDEX idx_deployment_audit_action ON deployment_audit_logs(action);
CREATE INDEX idx_deployment_audit_created ON deployment_audit_logs(created_at);

-- ============================================================
-- RLS Policies
-- ============================================================
ALTER TABLE deployment_windows ENABLE ROW LEVEL SECURITY;
ALTER TABLE batch_operations ENABLE ROW LEVEL SECURITY;
ALTER TABLE compliance_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE compliance_evaluations ENABLE ROW LEVEL SECURITY;
ALTER TABLE security_findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE secret_rotations ENABLE ROW LEVEL SECURITY;
ALTER TABLE secret_rotation_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE deployment_audit_logs ENABLE ROW LEVEL SECURITY;

-- Tenant isolation policies
CREATE POLICY "deployment_windows_tenant_isolation" ON deployment_windows
    USING (tenant_id::text = current_setting('app.tenant_id', true));

CREATE POLICY "batch_operations_tenant_isolation" ON batch_operations
    USING (tenant_id::text = current_setting('app.tenant_id', true));

CREATE POLICY "compliance_policies_tenant_isolation" ON compliance_policies
    USING (tenant_id::text = current_setting('app.tenant_id', true));

CREATE POLICY "compliance_evaluations_tenant_isolation" ON compliance_evaluations
    USING (tenant_id::text = current_setting('app.tenant_id', true));

CREATE POLICY "security_findings_tenant_isolation" ON security_findings
    USING (tenant_id::text = current_setting('app.tenant_id', true));

CREATE POLICY "secret_rotations_tenant_isolation" ON secret_rotations
    USING (tenant_id::text = current_setting('app.tenant_id', true));

CREATE POLICY "secret_rotation_logs_tenant_isolation" ON secret_rotation_logs
    USING (tenant_id::text = current_setting('app.tenant_id', true));

CREATE POLICY "deployment_audit_logs_tenant_isolation" ON deployment_audit_logs
    USING (tenant_id::text = current_setting('app.tenant_id', true));
