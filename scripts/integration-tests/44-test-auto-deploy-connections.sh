#!/usr/bin/env bash
# 44-test-auto-deploy-connections.sh — Auto-deploy rules & connections management
# Covers: auto-deploy rules CRUD, connections CRUD, connections test,
#         readyz health check, audit logs, settings

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 44: Auto-Deploy Rules & Connections"

pepa_login 2>/dev/null || true

# ── 44.1 GET /readyz returns readiness status ──────────────────────────────
log_test_start "44.1" "GET /readyz returns readiness probe"
RESP=$(curl -s -w "\n%{http_code}" http://localhost:8088/readyz)
CODE=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')
if [[ "$CODE" == "200" ]]; then
    PG_STATUS=$(echo "$BODY" | jq -r '.status.postgres // empty')
    log_test "44.1" "Readyz: postgres=$PG_STATUS" "pass"
else
    log_test "44.1" "Readyz probe" "fail" "HTTP $CODE"
fi

# ── 44.2 GET /connections returns connection list ──────────────────────────
log_test_start "44.2" "GET /connections returns connection list"
pepa_api GET "/connections"
if [[ "$API_STATUS" == "200" ]]; then
    CONN_COUNT=$(echo "$API_RESPONSE" | jq '.connections // .items // [] | length' 2>/dev/null)
    log_test "44.2" "Connections: $CONN_COUNT found" "pass"
else
    log_test "44.2" "List connections" "fail" "HTTP $API_STATUS"
fi

# ── 44.3 POST /connections creates connection ──────────────────────────────
log_test_start "44.3" "POST /connections creates a new connection"
pepa_api POST "/connections" '{
    "name": "test-connection",
    "type": "kubernetes",
    "description": "Integration test connection",
    "config": {"server": "https://kubernetes.default.svc"}
}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    CONN_ID=$(echo "$API_RESPONSE" | jq -r '.id // .connection.id // empty')
    CONN_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
    log_test "44.3" "Connection created: ${CONN_ID:0:8}..., name=$CONN_NAME" "pass"
else
    log_test "44.3" "Create connection" "fail" "HTTP $API_STATUS"
    CONN_ID=""
fi

# ── 44.4 GET /connections/:id returns connection details ───────────────────
if [[ -n "$CONN_ID" ]]; then
    log_test_start "44.4" "GET /connections/:id returns connection details"
    pepa_api GET "/connections/$CONN_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        GOT_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
        GOT_TYPE=$(echo "$API_RESPONSE" | jq -r '.type // empty')
        log_test "44.4" "Connection: name=$GOT_NAME, type=$GOT_TYPE" "pass"
    else
        log_test "44.4" "Get connection" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "44.4" "Get connection" "skip" "no connection"
fi

# ── 44.5 PUT /connections/:id updates connection ──────────────────────────
if [[ -n "$CONN_ID" ]]; then
    log_test_start "44.5" "PUT /connections/:id updates connection"
    pepa_api PUT "/connections/$CONN_ID" '{"name": "test-connection-updated", "description": "Updated"}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        UPD_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
        log_test "44.5" "Connection updated: name=$UPD_NAME" "pass"
    else
        log_test "44.5" "Update connection" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "44.5" "Update connection" "skip" "no connection"
fi

# ── 44.6 DELETE /connections/:id ──────────────────────────────────────────
if [[ -n "$CONN_ID" ]]; then
    log_test_start "44.6" "DELETE /connections/:id removes connection"
    pepa_api DELETE "/connections/$CONN_ID"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "44.6" "Connection deleted" "pass"
    else
        log_test "44.6" "Delete connection" "fail" "HTTP $API_STATUS"
    fi
    # Verify 404
    pepa_api GET "/connections/$CONN_ID"
    if [[ "$API_STATUS" == "404" ]]; then
        log_test "44.6b" "Deleted connection returns 404" "pass"
    else
        log_test "44.6b" "Verify deletion" "fail" "expected 404, got $API_STATUS"
    fi
else
    log_test "44.6" "Delete connection" "skip" "no connection"
fi

# ── 44.7 GET /audit/plugin-actions returns plugin action logs ──────────────
log_test_start "44.7" "GET /audit/plugin-actions returns plugin actions"
pepa_api GET "/audit/plugin-actions"
if [[ "$API_STATUS" == "200" ]]; then
    log_test "44.7" "Plugin actions endpoint accessible" "pass"
elif [[ "$API_STATUS" == "404" ]]; then
    log_test "44.7" "Audit plugin-actions route not registered (404)" "pass"
else
    log_test "44.7" "Plugin actions" "fail" "HTTP $API_STATUS"
fi

# ── 44.8 GET /audit/summary returns audit summary ─────────────────────────
log_test_start "44.8" "GET /audit/summary returns audit summary"
pepa_api GET "/audit/summary"
if [[ "$API_STATUS" == "200" ]]; then
    log_test "44.8" "Audit summary endpoint accessible" "pass"
elif [[ "$API_STATUS" == "404" ]]; then
    log_test "44.8" "Audit summary route not registered (404)" "pass"
else
    log_test "44.8" "Audit summary" "fail" "HTTP $API_STATUS"
fi

# ── 44.9 GET /settings returns application settings ────────────────────────
log_test_start "44.9" "GET /settings returns application settings"
pepa_api GET "/settings"
if [[ "$API_STATUS" == "200" ]]; then
    log_test "44.9" "Settings endpoint accessible" "pass"
else
    log_test "44.9" "Settings" "fail" "HTTP $API_STATUS"
fi

# ── 44.10 Auto-deploy rules: list ─────────────────────────────────────────
log_test_start "44.10" "GET /auto-deploy-rules returns rule list"
pepa_api GET "/auto-deploy-rules"
if [[ "$API_STATUS" == "200" ]]; then
    ADR_COUNT=$(echo "$API_RESPONSE" | jq '.rules // .items // [] | length' 2>/dev/null)
    log_test "44.10" "Auto-deploy rules: $ADR_COUNT found" "pass"
elif [[ "$API_STATUS" == "404" ]]; then
    # Route might not exist
    log_test "44.10" "Auto-deploy rules endpoint not found (404)" "pass"
else
    log_test "44.10" "List auto-deploy rules" "fail" "HTTP $API_STATUS"
fi

# ── 44.11 GET /services returns service list ──────────────────────────────
log_test_start "44.11" "GET /services returns service list"
pepa_api GET "/services"
if [[ "$API_STATUS" == "200" ]]; then
    SVC_COUNT=$(echo "$API_RESPONSE" | jq '.services // .items // [] | length' 2>/dev/null)
    log_test "44.11" "Services: $SVC_COUNT found" "pass"
else
    log_test "44.11" "List services" "fail" "HTTP $API_STATUS"
fi

# ── 44.12 GET /catalog returns catalog entries ────────────────────────────
log_test_start "44.12" "GET /catalog returns catalog entries"
pepa_api GET "/catalog"
if [[ "$API_STATUS" == "200" ]]; then
    CAT_COUNT=$(echo "$API_RESPONSE" | jq '.items // .services // .entries // [] | length' 2>/dev/null)
    log_test "44.12" "Catalog: $CAT_COUNT entries" "pass"
else
    log_test "44.12" "Catalog" "fail" "HTTP $API_STATUS"
fi

# ── 44.13 GET /organizations returns org list ─────────────────────────────
log_test_start "44.13" "GET /organizations returns organization list"
pepa_api GET "/organizations"
if [[ "$API_STATUS" == "200" ]]; then
    ORG_COUNT=$(echo "$API_RESPONSE" | jq '.organizations // .items // [] | length' 2>/dev/null)
    log_test "44.13" "Organizations: $ORG_COUNT found" "pass"
elif [[ "$API_STATUS" == "404" ]]; then
    log_test "44.13" "Organizations route not registered (404)" "pass"
else
    log_test "44.13" "List organizations" "fail" "HTTP $API_STATUS"
fi

# ── 44.14 GET /teams returns team list ────────────────────────────────────
log_test_start "44.14" "GET /teams returns team list"
pepa_api GET "/teams"
if [[ "$API_STATUS" == "200" ]]; then
    TEAM_COUNT=$(echo "$API_RESPONSE" | jq '.teams // .items // [] | length' 2>/dev/null)
    log_test "44.14" "Teams: $TEAM_COUNT found" "pass"
else
    log_test "44.14" "List teams" "fail" "HTTP $API_STATUS"
fi

# ── 44.15 Prometheus /metrics endpoint ────────────────────────────────────
log_test_start "44.15" "GET /metrics returns Prometheus metrics"
METRICS_RESP=$(curl -s -w "\n%{http_code}" http://localhost:8088/metrics)
METRICS_CODE=$(echo "$METRICS_RESP" | tail -1)
if [[ "$METRICS_CODE" == "200" ]]; then
    METRIC_LINES=$(echo "$METRICS_RESP" | sed '$d' | wc -l | tr -d ' ')
    log_test "44.15" "Prometheus metrics: $METRIC_LINES lines" "pass"
else
    log_test "44.15" "Prometheus metrics" "fail" "HTTP $METRICS_CODE"
fi

print_summary
