-- 045_plugin_activity_logging.sql
-- Tracks individual SSH commands executed through the remote console
-- and plugin/virtualization actions (VM start/stop/create/delete etc.)

-- ── SSH Command Log ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS ssh_command_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    user_id     UUID,
    host_id     UUID NOT NULL,
    host_name   TEXT NOT NULL DEFAULT '',
    username    TEXT NOT NULL DEFAULT '',
    command     TEXT NOT NULL,
    exit_code   INTEGER,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ssh_command_log_tenant ON ssh_command_log(tenant_id);
CREATE INDEX IF NOT EXISTS idx_ssh_command_log_user ON ssh_command_log(user_id);
CREATE INDEX IF NOT EXISTS idx_ssh_command_log_host ON ssh_command_log(host_id);
CREATE INDEX IF NOT EXISTS idx_ssh_command_log_created ON ssh_command_log(created_at DESC);

-- ── Plugin Action Log ───────────────────────────────────────────
-- Records every state-changing action executed through plugins
-- (VM start/stop/create/delete, container operations, etc.)
CREATE TABLE IF NOT EXISTS plugin_action_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    user_id      UUID,
    plugin_name  TEXT NOT NULL,
    action       TEXT NOT NULL,
    entity_type  TEXT NOT NULL DEFAULT '',
    entity_id    TEXT NOT NULL DEFAULT '',
    entity_name  TEXT NOT NULL DEFAULT '',
    params       JSONB,
    status       TEXT NOT NULL DEFAULT 'success',
    error_message TEXT,
    ip_address   INET,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_plugin_action_log_tenant ON plugin_action_log(tenant_id);
CREATE INDEX IF NOT EXISTS idx_plugin_action_log_user ON plugin_action_log(user_id);
CREATE INDEX IF NOT EXISTS idx_plugin_action_log_plugin ON plugin_action_log(plugin_name);
CREATE INDEX IF NOT EXISTS idx_plugin_action_log_created ON plugin_action_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_plugin_action_log_action ON plugin_action_log(plugin_name, action);
