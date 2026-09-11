#!/usr/bin/env bash
# 36-test-security-scan-advanced.sh — Security scanning targets, schedules, ignores
# Covers: POST/GET/PUT/DELETE /security/targets, scan trigger, schedules CRUD,
#         scan ignores CRUD, dashboard-v2, db-status, scan-all

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 36: Security Scanning Advanced Tests"

pepa_login 2>/dev/null || true

# ── 36.1 Create scan target ────────────────────────────────────────────────
log_test_start "36.1" "POST /security/targets creates a scan target"
pepa_api POST "/security/targets" '{
    "name": "test-scan-target",
    "scanner_type": "trivy",
    "target_type": "image",
    "target_ref": "nginx:latest",
    "scan_config": {"severity": "HIGH,CRITICAL"}
}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    TARGET_ID=$(echo "$API_RESPONSE" | jq -r '.id // .target.id // empty')
    TARGET_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
    log_test "36.1" "Scan target created: ${TARGET_ID:0:8}..., name=$TARGET_NAME" "pass"
else
    log_test "36.1" "Create scan target" "fail" "HTTP $API_STATUS"
    TARGET_ID=""
fi

# ── 36.2 Get scan target by ID ─────────────────────────────────────────────
if [[ -n "$TARGET_ID" ]]; then
    log_test_start "36.2" "GET /security/targets/:id returns target details"
    pepa_api GET "/security/targets/$TARGET_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        GOT_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
        GOT_TYPE=$(echo "$API_RESPONSE" | jq -r '.scanner_type // empty')
        if [[ "$GOT_NAME" == "test-scan-target" ]]; then
            log_test "36.2" "Target: name=$GOT_NAME, scanner=$GOT_TYPE" "pass"
        else
            log_test "36.2" "Get scan target" "fail" "name mismatch"
        fi
    else
        log_test "36.2" "Get scan target" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "36.2" "Get scan target" "skip" "no target"
fi

# ── 36.3 Update scan target ────────────────────────────────────────────────
if [[ -n "$TARGET_ID" ]]; then
    log_test_start "36.3" "PUT /security/targets/:id updates target"
    pepa_api PUT "/security/targets/$TARGET_ID" '{"name": "test-scan-target-updated", "scan_config": {"severity": "CRITICAL"}}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        UPD_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
        log_test "36.3" "Target updated: name=$UPD_NAME" "pass"
    else
        log_test "36.3" "Update scan target" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "36.3" "Update scan target" "skip" "no target"
fi

# ── 36.4 List scan targets ─────────────────────────────────────────────────
log_test_start "36.4" "GET /security/targets returns target list"
pepa_api GET "/security/targets"
if [[ "$API_STATUS" == "200" ]]; then
    TARGET_COUNT=$(echo "$API_RESPONSE" | jq 'if type == "array" then length else (.targets // .items // [] | length) end' 2>/dev/null)
    log_test "36.4" "Scan targets: $TARGET_COUNT found" "pass"
else
    log_test "36.4" "List scan targets" "fail" "HTTP $API_STATUS"
fi

# ── 36.5 Trigger scan ──────────────────────────────────────────────────────
if [[ -n "$TARGET_ID" ]]; then
    log_test_start "36.5" "POST /security/targets/:id/scan triggers scan"
    pepa_api POST "/security/targets/$TARGET_ID/scan"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "36.5" "Scan triggered (HTTP $API_STATUS)" "pass"
    else
        log_test "36.5" "Trigger scan" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "36.5" "Trigger scan" "skip" "no target"
fi

# ── 36.6 List scan runs ────────────────────────────────────────────────────
log_test_start "36.6" "GET /security/scans returns scan run list"
pepa_api GET "/security/scans"
if [[ "$API_STATUS" == "200" ]]; then
    RUN_COUNT=$(echo "$API_RESPONSE" | jq 'if type == "array" then length else (.runs // .items // [] | length) end' 2>/dev/null)
    log_test "36.6" "Scan runs: $RUN_COUNT found" "pass"
else
    log_test "36.6" "List scan runs" "fail" "HTTP $API_STATUS"
fi

# ── 36.7 Create scan schedule ──────────────────────────────────────────────
if [[ -n "$TARGET_ID" ]]; then
    log_test_start "36.7" "POST /security/schedules creates scan schedule"
    pepa_api POST "/security/schedules" "{\"target_id\": \"$TARGET_ID\", \"cron_expression\": \"0 2 * * *\"}"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        SCHED_ID=$(echo "$API_RESPONSE" | jq -r '.id // .schedule.id // empty')
        log_test "36.7" "Schedule created: ${SCHED_ID:0:8}..." "pass"
    else
        log_test "36.7" "Create schedule" "fail" "HTTP $API_STATUS"
        SCHED_ID=""
    fi
else
    log_test "36.7" "Create schedule" "skip" "no target"
    SCHED_ID=""
fi

# ── 36.8 List scan schedules ───────────────────────────────────────────────
log_test_start "36.8" "GET /security/schedules returns schedule list"
pepa_api GET "/security/schedules"
if [[ "$API_STATUS" == "200" ]]; then
    SCHED_COUNT=$(echo "$API_RESPONSE" | jq 'if type == "array" then length else (.schedules // .items // [] | length) end' 2>/dev/null)
    log_test "36.8" "Scan schedules: $SCHED_COUNT found" "pass"
else
    log_test "36.8" "List schedules" "fail" "HTTP $API_STATUS"
fi

# ── 36.9 Get scan schedule by ID ───────────────────────────────────────────
if [[ -n "${SCHED_ID:-}" ]]; then
    log_test_start "36.9" "GET /security/schedules/:id returns schedule details"
    pepa_api GET "/security/schedules/$SCHED_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        GOT_CRON=$(echo "$API_RESPONSE" | jq -r '.cron_expression // empty')
        log_test "36.9" "Schedule: cron=$GOT_CRON" "pass"
    else
        log_test "36.9" "Get schedule" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "36.9" "Get schedule" "skip" "no schedule"
fi

# ── 36.10 Update scan schedule ─────────────────────────────────────────────
if [[ -n "${SCHED_ID:-}" ]]; then
    log_test_start "36.10" "PUT /security/schedules/:id updates schedule"
    pepa_api PUT "/security/schedules/$SCHED_ID" '{"cron_expression": "0 3 * * *", "enabled": false}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        UPD_CRON=$(echo "$API_RESPONSE" | jq -r '.cron_expression // empty')
        log_test "36.10" "Schedule updated: cron=$UPD_CRON" "pass"
    else
        log_test "36.10" "Update schedule" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "36.10" "Update schedule" "skip" "no schedule"
fi

# ── 36.11 Create scan ignore (CVE) ─────────────────────────────────────────
if [[ -n "$TARGET_ID" ]]; then
    log_test_start "36.11" "POST /security/targets/:id/ignores creates CVE ignore"
    pepa_api POST "/security/targets/$TARGET_ID/ignores" '{"cve_id": "CVE-2024-TEST-001", "reason": "False positive in test environment"}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        IGNORE_ID=$(echo "$API_RESPONSE" | jq -r '.id // .ignore.id // empty')
        log_test "36.11" "Ignore created: ${IGNORE_ID:0:8}..." "pass"
    else
        log_test "36.11" "Create ignore" "fail" "HTTP $API_STATUS"
        IGNORE_ID=""
    fi
else
    log_test "36.11" "Create ignore" "skip" "no target"
    IGNORE_ID=""
fi

# ── 36.12 List scan ignores ────────────────────────────────────────────────
log_test_start "36.12" "GET /security/ignores returns ignore list"
pepa_api GET "/security/ignores"
if [[ "$API_STATUS" == "200" ]]; then
    IGNORE_COUNT=$(echo "$API_RESPONSE" | jq 'if type == "array" then length else (.ignores // .items // [] | length) end' 2>/dev/null)
    log_test "36.12" "Scan ignores: $IGNORE_COUNT found" "pass"
else
    log_test "36.12" "List ignores" "fail" "HTTP $API_STATUS"
fi

# ── 36.13 List target-specific ignores ─────────────────────────────────────
if [[ -n "$TARGET_ID" ]]; then
    log_test_start "36.13" "GET /security/targets/:id/ignores returns target ignores"
    pepa_api GET "/security/targets/$TARGET_ID/ignores"
    if [[ "$API_STATUS" == "200" ]]; then
        TGT_IGNORE=$(echo "$API_RESPONSE" | jq 'if type == "array" then length else (.ignores // .items // [] | length) end' 2>/dev/null)
        log_test "36.13" "Target ignores: $TGT_IGNORE found" "pass"
    else
        log_test "36.13" "List target ignores" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "36.13" "List target ignores" "skip" "no target"
fi

# ── 36.14 Get dashboard v2 ─────────────────────────────────────────────────
log_test_start "36.14" "GET /security/dashboard-v2 returns dashboard data"
pepa_api GET "/security/dashboard-v2"
if [[ "$API_STATUS" == "200" ]]; then
    HAS_TARGETS=$(echo "$API_RESPONSE" | jq 'has("targets")' 2>/dev/null)
    HAS_SUMMARY=$(echo "$API_RESPONSE" | jq 'has("scan_summary")' 2>/dev/null)
    if [[ "$HAS_TARGETS" == "true" ]]; then
        log_test "36.14" "Dashboard v2: targets=$HAS_TARGETS, summary=$HAS_SUMMARY" "pass"
    else
        log_test "36.14" "Dashboard v2" "fail" "missing targets field"
    fi
else
    log_test "36.14" "Dashboard v2" "fail" "HTTP $API_STATUS"
fi

# ── 36.15 Get database status ──────────────────────────────────────────────
log_test_start "36.15" "GET /security/db-status returns Trivy DB status"
pepa_api GET "/security/db-status"
if [[ "$API_STATUS" == "200" || "$API_STATUS" == "503" ]]; then
    log_test "36.15" "DB status endpoint accessible (HTTP $API_STATUS)" "pass"
else
    log_test "36.15" "DB status" "fail" "HTTP $API_STATUS"
fi

# ── 36.16 Create target with invalid scanner_type (400) ────────────────────
log_test_start "36.16" "POST /security/targets with invalid scanner_type returns 400"
pepa_api POST "/security/targets" '{"name": "bad-scanner", "scanner_type": "invalid", "target_type": "image", "target_ref": "test:latest"}'
if [[ "$API_STATUS" == "400" ]]; then
    log_test "36.16" "Invalid scanner_type returns 400" "pass"
else
    log_test "36.16" "Invalid scanner_type" "fail" "expected 400, got $API_STATUS"
fi

# ── 36.17 Create target with invalid target_type (400) ─────────────────────
log_test_start "36.17" "POST /security/targets with invalid target_type returns 400"
pepa_api POST "/security/targets" '{"name": "bad-target", "scanner_type": "trivy", "target_type": "invalid", "target_ref": "test:latest"}'
if [[ "$API_STATUS" == "400" ]]; then
    log_test "36.17" "Invalid target_type returns 400" "pass"
else
    log_test "36.17" "Invalid target_type" "fail" "expected 400, got $API_STATUS"
fi

# ── 36.18 Delete scan ignore ───────────────────────────────────────────────
if [[ -n "${IGNORE_ID:-}" ]]; then
    log_test_start "36.18" "DELETE /security/ignores/:ignoreId"
    pepa_api DELETE "/security/ignores/$IGNORE_ID"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "36.18" "Ignore deleted" "pass"
    else
        log_test "36.18" "Delete ignore" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "36.18" "Delete ignore" "skip" "no ignore"
fi

# ── 36.19 Delete scan schedule ─────────────────────────────────────────────
if [[ -n "${SCHED_ID:-}" ]]; then
    log_test_start "36.19" "DELETE /security/schedules/:id"
    pepa_api DELETE "/security/schedules/$SCHED_ID"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "36.19" "Schedule deleted" "pass"
    else
        log_test "36.19" "Delete schedule" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "36.19" "Delete schedule" "skip" "no schedule"
fi

# ── 36.20 Delete scan target ───────────────────────────────────────────────
if [[ -n "$TARGET_ID" ]]; then
    log_test_start "36.20" "DELETE /security/targets/:id"
    pepa_api DELETE "/security/targets/$TARGET_ID"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "36.20" "Target deleted" "pass"
    else
        log_test "36.20" "Delete target" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "36.20" "Delete target" "skip" "no target"
fi

# ── 36.21 Verify target deletion (404) ─────────────────────────────────────
if [[ -n "$TARGET_ID" ]]; then
    log_test_start "36.21" "GET /security/targets/:id returns 404 after deletion"
    pepa_api GET "/security/targets/$TARGET_ID"
    if [[ "$API_STATUS" == "404" ]]; then
        log_test "36.21" "Deleted target returns 404" "pass"
    else
        log_test "36.21" "Deleted target" "fail" "expected 404, got $API_STATUS"
    fi
else
    log_test "36.21" "Verify deletion" "skip" "no target"
fi

print_summary
