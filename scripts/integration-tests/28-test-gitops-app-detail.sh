#!/usr/bin/env bash
# 28-test-gitops-app-detail.sh — GitOps application detail, refresh, sync
# Covers: GET /gitops/applications, GET /gitops/applications/:conn/:ns/:name,
#         POST .../refresh, POST .../sync, .../history, .../tree, .../events

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 28: GitOps Application Detail, Refresh & Sync"

pepa_login 2>/dev/null || true

# ── 28.1 List all GitOps applications ──────────────────────────────────────
log_test_start "28.1" "GET /gitops/applications returns application list"
pepa_api GET "/gitops/applications"
if [[ "$API_STATUS" == "200" ]]; then
    APP_COUNT=$(echo "$API_RESPONSE" | jq '.applications // .items // [] | length' 2>/dev/null)
    log_test "28.1" "GitOps applications: $APP_COUNT found" "pass"
else
    log_test "28.1" "List GitOps applications" "fail" "HTTP $API_STATUS"
    APP_COUNT=0
fi

# ── 28.2 Get first application details ─────────────────────────────────────
if [[ "$APP_COUNT" -gt 0 ]] 2>/dev/null; then
    FIRST_CONN=$(echo "$API_RESPONSE" | jq -r '(.applications // .items // [])[0].connection_id // empty' 2>/dev/null)
    FIRST_NS=$(echo "$API_RESPONSE" | jq -r '(.applications // .items // [])[0].namespace // empty' 2>/dev/null)
    FIRST_NAME=$(echo "$API_RESPONSE" | jq -r '(.applications // .items // [])[0].name // empty' 2>/dev/null)
fi

if [[ -n "${FIRST_CONN:-}" && -n "${FIRST_NS:-}" && -n "${FIRST_NAME:-}" ]]; then
    log_test_start "28.2" "GET /gitops/applications/:conn/:ns/:name returns detail"
    pepa_api GET "/gitops/applications/$FIRST_CONN/$FIRST_NS/$FIRST_NAME"
    if [[ "$API_STATUS" == "200" ]]; then
        APP_HEALTH=$(echo "$API_RESPONSE" | jq -r '.health // .status.health // empty')
        APP_SYNC=$(echo "$API_RESPONSE" | jq -r '.sync_status // .status.sync_status // empty')
        APP_REV=$(echo "$API_RESPONSE" | jq -r '.revision // .status.revision // empty')
        log_test "28.2" "App detail: health=$APP_HEALTH, sync=$APP_SYNC, rev=${APP_REV:0:8}..." "pass"
    else
        log_test "28.2" "GitOps app detail" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "28.2" "GitOps app detail" "skip" "no applications available"
fi

# ── 28.3 Get application history ───────────────────────────────────────────
if [[ -n "${FIRST_CONN:-}" && -n "${FIRST_NS:-}" && -n "${FIRST_NAME:-}" ]]; then
    log_test_start "28.3" "GET /gitops/applications/:conn/:ns/:name/history"
    pepa_api GET "/gitops/applications/$FIRST_CONN/$FIRST_NS/$FIRST_NAME/history"
    if [[ "$API_STATUS" == "200" ]]; then
        log_test "28.3" "Application history endpoint accessible" "pass"
    else
        log_test "28.3" "Application history" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "28.3" "Application history" "skip" "no applications"
fi

# ── 28.4 Get application tree ──────────────────────────────────────────────
if [[ -n "${FIRST_CONN:-}" && -n "${FIRST_NS:-}" && -n "${FIRST_NAME:-}" ]]; then
    log_test_start "28.4" "GET /gitops/applications/:conn/:ns/:name/tree"
    pepa_api GET "/gitops/applications/$FIRST_CONN/$FIRST_NS/$FIRST_NAME/tree"
    if [[ "$API_STATUS" == "200" ]]; then
        log_test "28.4" "Application tree endpoint accessible" "pass"
    else
        log_test "28.4" "Application tree" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "28.4" "Application tree" "skip" "no applications"
fi

# ── 28.5 Get application events ────────────────────────────────────────────
if [[ -n "${FIRST_CONN:-}" && -n "${FIRST_NS:-}" && -n "${FIRST_NAME:-}" ]]; then
    log_test_start "28.5" "GET /gitops/applications/:conn/:ns/:name/events"
    pepa_api GET "/gitops/applications/$FIRST_CONN/$FIRST_NS/$FIRST_NAME/events"
    if [[ "$API_STATUS" == "200" ]]; then
        EVT_COUNT=$(echo "$API_RESPONSE" | jq '.events // .items // [] | length' 2>/dev/null)
        log_test "28.5" "Application events: $EVT_COUNT entries" "pass"
    else
        log_test "28.5" "Application events" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "28.5" "Application events" "skip" "no applications"
fi

# ── 28.6 Refresh application ───────────────────────────────────────────────
if [[ -n "${FIRST_CONN:-}" && -n "${FIRST_NS:-}" && -n "${FIRST_NAME:-}" ]]; then
    log_test_start "28.6" "POST /gitops/applications/:conn/:ns/:name/refresh"
    pepa_api POST "/gitops/applications/$FIRST_CONN/$FIRST_NS/$FIRST_NAME/refresh"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "28.6" "Application refresh triggered" "pass"
    else
        log_test "28.6" "Application refresh" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "28.6" "Application refresh" "skip" "no applications"
fi

# ── 28.7 Sync application ──────────────────────────────────────────────────
if [[ -n "${FIRST_CONN:-}" && -n "${FIRST_NS:-}" && -n "${FIRST_NAME:-}" ]]; then
    log_test_start "28.7" "POST /gitops/applications/:conn/:ns/:name/sync"
    pepa_api POST "/gitops/applications/$FIRST_CONN/$FIRST_NS/$FIRST_NAME/sync"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "28.7" "Application sync triggered" "pass"
    else
        log_test "28.7" "Application sync" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "28.7" "Application sync" "skip" "no applications"
fi

# ── 28.8 List GitOps repositories ──────────────────────────────────────────
log_test_start "28.8" "GET /gitops/repos returns repositories"
pepa_api GET "/gitops/repos"
if [[ "$API_STATUS" == "200" ]]; then
    REPO_COUNT=$(echo "$API_RESPONSE" | jq '.repos // .repositories // .items // [] | length' 2>/dev/null)
    log_test "28.8" "GitOps repos: $REPO_COUNT found" "pass"
else
    log_test "28.8" "List GitOps repos" "fail" "HTTP $API_STATUS"
fi

# ── 28.9 List drift schedules ──────────────────────────────────────────────
log_test_start "28.9" "GET /gitops/drift-schedules returns schedules"
pepa_api GET "/gitops/drift-schedules"
if [[ "$API_STATUS" == "200" ]]; then
    SCHED_COUNT=$(echo "$API_RESPONSE" | jq '.schedules // .items // [] | length' 2>/dev/null)
    log_test "28.9" "Drift schedules: $SCHED_COUNT found" "pass"
else
    log_test "28.9" "List drift schedules" "fail" "HTTP $API_STATUS"
fi

# ── 28.10 List drift logs ──────────────────────────────────────────────────
log_test_start "28.10" "GET /gitops/drift-logs returns logs"
pepa_api GET "/gitops/drift-logs"
if [[ "$API_STATUS" == "200" ]]; then
    log_test "28.10" "Drift logs endpoint accessible" "pass"
else
    log_test "28.10" "Drift logs" "fail" "HTTP $API_STATUS"
fi

print_summary
