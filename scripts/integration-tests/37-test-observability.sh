#!/usr/bin/env bash
# 37-test-observability.sh — Observability endpoints
# Covers: GET /observability/overview, /metrics, /logs, /traces, /dashboards,
#         /alerts, /correlate, /settings, PUT /settings, debug/send-test

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 37: Observability Overview, Metrics & Settings"

pepa_login 2>/dev/null || true

# ── 37.1 Get observability overview ────────────────────────────────────────
log_test_start "37.1" "GET /observability/overview returns system health"
pepa_api GET "/observability/overview"
if [[ "$API_STATUS" == "200" ]]; then
    HAS_STATUS=$(echo "$API_RESPONSE" | jq 'has("status")' 2>/dev/null)
    HAS_SYSTEM=$(echo "$API_RESPONSE" | jq 'has("system")' 2>/dev/null)
    HAS_SERVICES=$(echo "$API_RESPONSE" | jq 'has("services")' 2>/dev/null)
    HAS_ACTIVITY=$(echo "$API_RESPONSE" | jq 'has("activity")' 2>/dev/null)
    STATUS=$(echo "$API_RESPONSE" | jq -r '.status // empty')
    if [[ "$HAS_STATUS" == "true" && "$HAS_SYSTEM" == "true" ]]; then
        log_test "37.1" "Overview: status=$STATUS, system=$HAS_SYSTEM, services=$HAS_SERVICES, activity=$HAS_ACTIVITY" "pass"
    else
        log_test "37.1" "Overview" "fail" "missing required fields"
    fi
else
    log_test "37.1" "Observability overview" "fail" "HTTP $API_STATUS"
fi

# ── 37.2 Verify overview system fields ─────────────────────────────────────
log_test_start "37.2" "Overview system has version, goroutines, memory"
pepa_api GET "/observability/overview"
if [[ "$API_STATUS" == "200" ]]; then
    VERSION=$(echo "$API_RESPONSE" | jq -r '.system.version // empty')
    GO_VERSION=$(echo "$API_RESPONSE" | jq -r '.system.go_version // empty')
    GOROUTINES=$(echo "$API_RESPONSE" | jq -r '.system.goroutines // empty')
    MEM_MB=$(echo "$API_RESPONSE" | jq -r '.system.memory_alloc_mb // empty')
    if [[ -n "$GO_VERSION" && -n "$GOROUTINES" ]]; then
        log_test "37.2" "System: go=$GO_VERSION, goroutines=$GOROUTINES, mem=${MEM_MB}MB" "pass"
    else
        log_test "37.2" "Overview system fields" "fail" "missing go_version or goroutines"
    fi
else
    log_test "37.2" "Overview system fields" "fail" "HTTP $API_STATUS"
fi

# ── 37.3 Get observability metrics ─────────────────────────────────────────
log_test_start "37.3" "GET /observability/metrics returns Prometheus metrics"
pepa_api GET "/observability/metrics"
if [[ "$API_STATUS" == "200" ]]; then
    METRIC_COUNT=$(echo "$API_RESPONSE" | jq 'if type == "array" then length else (.metrics // .items // [] | length) end' 2>/dev/null)
    HAS_METRICS=$(echo "$API_RESPONSE" | jq 'has("metrics") or has("data") or (type == "array")' 2>/dev/null)
    log_test "37.3" "Metrics: $METRIC_COUNT entries, has_data=$HAS_METRICS" "pass"
else
    log_test "37.3" "Observability metrics" "fail" "HTTP $API_STATUS"
fi

# ── 37.4 Get observability logs ────────────────────────────────────────────
log_test_start "37.4" "GET /observability/logs returns recent logs"
pepa_api GET "/observability/logs"
if [[ "$API_STATUS" == "200" ]]; then
    LOG_COUNT=$(echo "$API_RESPONSE" | jq 'if type == "array" then length else (.logs // .items // [] | length) end' 2>/dev/null)
    log_test "37.4" "Observability logs: $LOG_COUNT entries" "pass"
else
    log_test "37.4" "Observability logs" "fail" "HTTP $API_STATUS"
fi

# ── 37.5 Get observability traces ──────────────────────────────────────────
log_test_start "37.5" "GET /observability/traces returns trace data"
pepa_api GET "/observability/traces"
if [[ "$API_STATUS" == "200" ]]; then
    TRACE_COUNT=$(echo "$API_RESPONSE" | jq 'if type == "array" then length else (.traces // .items // [] | length) end' 2>/dev/null)
    log_test "37.5" "Observability traces: $TRACE_COUNT entries" "pass"
else
    log_test "37.5" "Observability traces" "fail" "HTTP $API_STATUS"
fi

# ── 37.6 Get observability dashboards ──────────────────────────────────────
log_test_start "37.6" "GET /observability/dashboards returns dashboard configs"
pepa_api GET "/observability/dashboards"
if [[ "$API_STATUS" == "200" ]]; then
    DASH_COUNT=$(echo "$API_RESPONSE" | jq 'if type == "array" then length else (.dashboards // .items // [] | length) end' 2>/dev/null)
    log_test "37.6" "Dashboards: $DASH_COUNT found" "pass"
else
    log_test "37.6" "Observability dashboards" "fail" "HTTP $API_STATUS"
fi

# ── 37.7 Get observability alerts ──────────────────────────────────────────
log_test_start "37.7" "GET /observability/alerts returns alert list"
pepa_api GET "/observability/alerts"
if [[ "$API_STATUS" == "200" ]]; then
    ALERT_COUNT=$(echo "$API_RESPONSE" | jq 'if type == "array" then length else (.alerts // .items // [] | length) end' 2>/dev/null)
    log_test "37.7" "Alerts: $ALERT_COUNT found" "pass"
else
    log_test "37.7" "Observability alerts" "fail" "HTTP $API_STATUS"
fi

# ── 37.8 Get observability settings ────────────────────────────────────────
log_test_start "37.8" "GET /observability/settings returns settings"
pepa_api GET "/observability/settings"
if [[ "$API_STATUS" == "200" ]]; then
    HAS_SETTINGS=$(echo "$API_RESPONSE" | jq 'keys | length' 2>/dev/null)
    log_test "37.8" "Settings: $HAS_SETTINGS top-level keys" "pass"
else
    log_test "37.8" "Observability settings" "fail" "HTTP $API_STATUS"
fi

# ── 37.9 Update observability settings ─────────────────────────────────────
log_test_start "37.9" "PUT /observability/settings updates settings"
pepa_api PUT "/observability/settings" '{"log_level": "debug", "retention_days": 7}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    log_test "37.9" "Settings updated (HTTP $API_STATUS)" "pass"
else
    log_test "37.9" "Update settings" "fail" "HTTP $API_STATUS"
fi

# ── 37.10 Correlate by trace_id ────────────────────────────────────────────
log_test_start "37.10" "GET /observability/correlate?trace_id=test correlates data"
pepa_api GET "/observability/correlate?trace_id=test-trace-123"
if [[ "$API_STATUS" == "200" ]]; then
    log_test "37.10" "Correlate endpoint accessible" "pass"
else
    log_test "37.10" "Correlate" "fail" "HTTP $API_STATUS"
fi

# ── 37.11 Test syslog connectivity ─────────────────────────────────────────
log_test_start "37.11" "POST /observability/settings/test-syslog tests connectivity"
pepa_api POST "/observability/settings/test-syslog" '{"host": "localhost", "port": 514, "protocol": "udp"}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ || "$API_STATUS" == "400" ]]; then
    log_test "37.11" "Syslog test endpoint accessible (HTTP $API_STATUS)" "pass"
else
    log_test "37.11" "Test syslog" "fail" "HTTP $API_STATUS"
fi

# ── 37.12 Test OTLP endpoint ───────────────────────────────────────────────
log_test_start "37.12" "POST /observability/settings/test-otlp tests OTLP"
pepa_api POST "/observability/settings/test-otlp" '{"endpoint": "http://localhost:4317"}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ || "$API_STATUS" == "400" ]]; then
    log_test "37.12" "OTLP test endpoint accessible (HTTP $API_STATUS)" "pass"
else
    log_test "37.12" "Test OTLP" "fail" "HTTP $API_STATUS"
fi

# ── 37.13 Send test telemetry ──────────────────────────────────────────────
log_test_start "37.13" "POST /observability/debug/send-test sends test data"
pepa_api POST "/observability/debug/send-test"
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    log_test "37.13" "Test telemetry sent (HTTP $API_STATUS)" "pass"
else
    log_test "37.13" "Send test telemetry" "fail" "HTTP $API_STATUS"
fi

# ── 37.14 Overview activity metrics ────────────────────────────────────────
log_test_start "37.14" "Overview activity has deployment and pipeline counts"
pepa_api GET "/observability/overview"
if [[ "$API_STATUS" == "200" ]]; then
    DEPLOY_24H=$(echo "$API_RESPONSE" | jq -r '.activity.deployments_24h // empty')
    PIPELINE_24H=$(echo "$API_RESPONSE" | jq -r '.activity.pipeline_runs_24h // empty')
    if [[ -n "$DEPLOY_24H" && -n "$PIPELINE_24H" ]]; then
        log_test "37.14" "Activity: deployments_24h=$DEPLOY_24H, pipeline_runs_24h=$PIPELINE_24H" "pass"
    else
        log_test "37.14" "Activity metrics" "fail" "missing deployment or pipeline counts"
    fi
else
    log_test "37.14" "Activity metrics" "fail" "HTTP $API_STATUS"
fi

# ── 37.15 Overview services metrics ────────────────────────────────────────
log_test_start "37.15" "Overview services has plugin and connection counts"
pepa_api GET "/observability/overview"
if [[ "$API_STATUS" == "200" ]]; then
    PLUGINS=$(echo "$API_RESPONSE" | jq -r '.services.plugins // empty')
    CONNECTIONS=$(echo "$API_RESPONSE" | jq -r '.services.active_connections // empty')
    WORKFLOWS=$(echo "$API_RESPONSE" | jq -r '.services.active_workflows // empty')
    if [[ -n "$PLUGINS" ]]; then
        log_test "37.15" "Services: plugins=$PLUGINS, connections=$CONNECTIONS, workflows=$WORKFLOWS" "pass"
    else
        log_test "37.15" "Services metrics" "fail" "missing plugin count"
    fi
else
    log_test "37.15" "Services metrics" "fail" "HTTP $API_STATUS"
fi

print_summary
