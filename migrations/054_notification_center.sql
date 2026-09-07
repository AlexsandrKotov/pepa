-- ============================================================
-- 054: Notification Center — rules, delivery logs, RBAC
-- ============================================================
-- Adds the notification routing system: rules map platform events
-- to notification connections, and logs record every delivery attempt.

-- notification_rules: routing rules (event_type -> connection)
CREATE TABLE IF NOT EXISTS notification_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT true,
    event_types TEXT[] NOT NULL DEFAULT '{}',
    connection_id UUID NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    subject_template TEXT,
    body_template TEXT NOT NULL,
    format_config JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notification_rules_tenant ON notification_rules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_notification_rules_connection ON notification_rules(connection_id);

-- notification_logs: delivery history
CREATE TABLE IF NOT EXISTS notification_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    rule_id UUID REFERENCES notification_rules(id) ON DELETE SET NULL,
    connection_id UUID NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    event_payload JSONB,
    rendered_subject TEXT,
    rendered_body TEXT,
    provider TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    response_text TEXT,
    error_text TEXT,
    sent_at TIMESTAMPTZ DEFAULT NOW(),
    delivered_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notification_logs_tenant ON notification_logs(tenant_id, sent_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_logs_connection ON notification_logs(connection_id, sent_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_logs_status ON notification_logs(status);

-- ============================================================
-- RBAC permissions for the notifications resource
-- ============================================================

-- Admin: full CRUD on notifications
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000001', 'notifications', a, 'allow'
FROM unnest(ARRAY['create','read','update','delete']) AS a
ON CONFLICT DO NOTHING;

-- Developer: read-only (can see notification history)
INSERT INTO permissions (role_id, resource, action, effect)
VALUES ('20000000-0000-0000-0000-000000000002', 'notifications', 'read', 'allow')
ON CONFLICT DO NOTHING;

-- Viewer: read-only
INSERT INTO permissions (role_id, resource, action, effect)
VALUES ('20000000-0000-0000-0000-000000000003', 'notifications', 'read', 'allow')
ON CONFLICT DO NOTHING;
