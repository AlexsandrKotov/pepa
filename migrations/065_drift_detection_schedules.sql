-- ============================================================
-- 065: Drift Detection Schedules & Alerting
-- ============================================================
-- Adds scheduled drift detection with cron-based execution
-- and alerting integration via the notification system.

-- drift_schedules: cron-based drift detection configurations
CREATE TABLE IF NOT EXISTS drift_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    repo_id UUID NOT NULL REFERENCES gitops_repositories(id) ON DELETE CASCADE,
    cluster_id UUID NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    scope_path TEXT,
    name TEXT NOT NULL,
    description TEXT,
    cron_expression TEXT NOT NULL DEFAULT '0 */6 * * *',
    enabled BOOLEAN NOT NULL DEFAULT true,
    alert_on_drift BOOLEAN NOT NULL DEFAULT true,
    alert_severity_threshold TEXT DEFAULT 'warning',
    last_run_at TIMESTAMPTZ,
    last_run_status TEXT,
    last_drift_count INTEGER DEFAULT 0,
    next_run_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_drift_schedules_tenant ON drift_schedules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_drift_schedules_repo ON drift_schedules(repo_id);
CREATE INDEX IF NOT EXISTS idx_drift_schedules_enabled_next ON drift_schedules(enabled, next_run_at) WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_drift_schedules_next_run ON drift_schedules(next_run_at) WHERE enabled = true;

-- drift_detection_logs: history of drift detection runs
CREATE TABLE IF NOT EXISTS drift_detection_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    schedule_id UUID REFERENCES drift_schedules(id) ON DELETE CASCADE,
    repo_id UUID NOT NULL REFERENCES gitops_repositories(id) ON DELETE CASCADE,
    cluster_id UUID NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    scope_path TEXT,
    triggered_by TEXT NOT NULL DEFAULT 'schedule',
    status TEXT NOT NULL DEFAULT 'running',
    drift_count INTEGER DEFAULT 0,
    critical_count INTEGER DEFAULT 0,
    warning_count INTEGER DEFAULT 0,
    info_count INTEGER DEFAULT 0,
    drift_details JSONB,
    started_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_drift_logs_tenant ON drift_detection_logs(tenant_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_drift_logs_schedule ON drift_detection_logs(schedule_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_drift_logs_repo ON drift_detection_logs(repo_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_drift_logs_status ON drift_detection_logs(status);

-- ============================================================
-- RBAC permissions for drift schedules
-- ============================================================

-- Only insert permissions if the table exists
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'permissions') THEN
        -- Admin: full CRUD on drift schedules
        INSERT INTO permissions (role_id, resource, action, effect)
        SELECT '20000000-0000-0000-0000-000000000001', 'drift_schedules', a, 'allow'
        FROM unnest(ARRAY['create','read','update','delete']) AS a
        ON CONFLICT DO NOTHING;

        -- Developer: read and execute (can trigger manual runs)
        INSERT INTO permissions (role_id, resource, action, effect)
        SELECT '20000000-0000-0000-0000-000000000002', 'drift_schedules', a, 'allow'
        FROM unnest(ARRAY['read','execute']) AS a
        ON CONFLICT DO NOTHING;

        -- Viewer: read-only
        INSERT INTO permissions (role_id, resource, action, effect)
        VALUES ('20000000-0000-0000-0000-000000000003', 'drift_schedules', 'read', 'allow')
        ON CONFLICT DO NOTHING;
    END IF;
END $$;
