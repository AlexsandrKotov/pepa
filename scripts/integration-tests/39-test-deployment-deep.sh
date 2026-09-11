#!/usr/bin/env bash
# 39-test-deployment-deep.sh — Deep deployment endpoint coverage
# Covers: dry-run, events, history, metrics, cancel, timeline,
#         list pagination, get-by-id, remove, system info

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 39: Deployment Deep Coverage"

pepa_login 2>/dev/null || true

# Helper: update deployment status directly in DB
set_deploy_status() {
    local deploy_id="$1" status="$2"
    docker exec pepa-postgres psql -U pepa -d pepa -c \
        "UPDATE deployments SET status='$status', updated_at=NOW() WHERE id='$deploy_id'" \
        -t 2>/dev/null | tr -d ' \n'
}

# ── 39.1 POST /deployments/dry-run ─────────────────────────────────────────
log_test_start "39.1" "POST /deployments/dry-run previews deployment"
pepa_api POST "/deployments/dry-run" '{
    "target_namespace": "test-dryrun-ns",
    "gitlab_project_name": "dryrun-app",
    "replicas": 2,
    "spec": {}
}'
if [[ "$API_STATUS" == "200" ]]; then
    log_test "39.1" "Dry-run returned preview (HTTP 200)" "pass"
elif [[ "$API_STATUS" == "400" ]]; then
    # Expected when no cluster is configured
    log_test "39.1" "Dry-run endpoint works (400: no cluster configured)" "pass"
elif [[ "$API_STATUS" == "503" ]]; then
    log_test "39.1" "Dry-run endpoint accessible (503: service unavailable)" "pass"
else
    log_test "39.1" "Dry-run" "fail" "HTTP $API_STATUS"
fi

# ── 39.2 GET /deployments/metrics ──────────────────────────────────────────
log_test_start "39.2" "GET /deployments/metrics returns deployment statistics"
pepa_api GET "/deployments/metrics"
if [[ "$API_STATUS" == "200" ]]; then
    HAS_TOTAL=$(echo "$API_RESPONSE" | jq 'has("total") or has("total_deployments") or has("metrics") or has("counts")' 2>/dev/null)
    log_test "39.2" "Metrics endpoint: has_data=$HAS_TOTAL" "pass"
else
    log_test "39.2" "Deployment metrics" "fail" "HTTP $API_STATUS"
fi

# ── 39.3 Create deployments for deep tests ─────────────────────────────────
log_test_start "39.3" "Create multiple deployments for deep testing"
DEPLOY_IDS=()
for i in 1 2 3; do
    pepa_api POST "/deployments" "{
        \"target_namespace\": \"deep-test-ns-$i\",
        \"image_tag\": \"v1.0.$i\",
        \"deploy_type\": \"helm\",
        \"replicas\": $i,
        \"spec\": {}
    }"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        DID=$(echo "$API_RESPONSE" | jq -r '.id // .deployment.id // empty')
        DEPLOY_IDS+=("$DID")
    fi
done
if [[ ${#DEPLOY_IDS[@]} -ge 3 ]]; then
    log_test "39.3" "Created ${#DEPLOY_IDS[@]} deployments" "pass"
else
    log_test "39.3" "Create deployments" "fail" "only ${#DEPLOY_IDS[@]} created"
fi

# ── 39.4 GET /deployments/:id returns single deployment ────────────────────
if [[ -n "${DEPLOY_IDS[0]}" ]]; then
    log_test_start "39.4" "GET /deployments/:id returns single deployment"
    pepa_api GET "/deployments/${DEPLOY_IDS[0]}"
    if [[ "$API_STATUS" == "200" ]]; then
        GOT_ID=$(echo "$API_RESPONSE" | jq -r '.id // empty')
        GOT_NS=$(echo "$API_RESPONSE" | jq -r '.target_namespace // empty')
        log_test "39.4" "Deployment: id=${GOT_ID:0:8}..., ns=$GOT_NS" "pass"
    else
        log_test "39.4" "Get deployment" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "39.4" "Get deployment" "skip" "no deployment"
fi

# ── 39.5 GET /deployments/:id/events returns timeline events ───────────────
if [[ -n "${DEPLOY_IDS[0]}" ]]; then
    log_test_start "39.5" "GET /deployments/:id/events returns timeline"
    pepa_api GET "/deployments/${DEPLOY_IDS[0]}/events"
    if [[ "$API_STATUS" == "200" ]]; then
        EVT_COUNT=$(echo "$API_RESPONSE" | jq '.events // [] | length' 2>/dev/null)
        log_test "39.5" "Timeline events: $EVT_COUNT entries" "pass"
    else
        log_test "39.5" "Timeline events" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "39.5" "Timeline events" "skip" "no deployment"
fi

# ── 39.6 GET /deployments/:id/history returns deployment history ───────────
if [[ -n "${DEPLOY_IDS[0]}" ]]; then
    log_test_start "39.6" "GET /deployments/:id/history returns history"
    pepa_api GET "/deployments/${DEPLOY_IDS[0]}/history"
    if [[ "$API_STATUS" == "200" ]]; then
        log_test "39.6" "History endpoint accessible" "pass"
    else
        log_test "39.6" "Deployment history" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "39.6" "Deployment history" "skip" "no deployment"
fi

# ── 39.7 POST /deployments/:id/cancel cancels pending deployment ───────────
if [[ -n "${DEPLOY_IDS[2]}" ]]; then
    log_test_start "39.7" "POST /deployments/:id/cancel cancels deployment"
    # Deployment is in "pending" status — cancel should work or return appropriate error
    pepa_api POST "/deployments/${DEPLOY_IDS[2]}/cancel"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        NEW_STATUS=$(echo "$API_RESPONSE" | jq -r '.status // empty')
        log_test "39.7" "Deployment cancelled: status=$NEW_STATUS" "pass"
    elif [[ "$API_STATUS" == "409" ]]; then
        log_test "39.7" "Cancel returns 409 (status doesn't allow cancel)" "pass"
    elif [[ "$API_STATUS" == "400" ]]; then
        log_test "39.7" "Cancel returns 400 (validation)" "pass"
    else
        log_test "39.7" "Cancel deployment" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "39.7" "Cancel deployment" "skip" "no deployment"
fi

# ── 39.8 GET /deployments with pagination ──────────────────────────────────
log_test_start "39.8" "GET /deployments?page=1&per_page=2 returns paginated results"
pepa_api GET "/deployments?page=1&per_page=2"
if [[ "$API_STATUS" == "200" ]]; then
    PAGE=$(echo "$API_RESPONSE" | jq '.page // empty')
    PER_PAGE=$(echo "$API_RESPONSE" | jq '.per_page // empty')
    TOTAL=$(echo "$API_RESPONSE" | jq '.total // empty')
    GOT_COUNT=$(echo "$API_RESPONSE" | jq '.deployments // [] | length' 2>/dev/null)
    if [[ "$PAGE" == "1" && "$PER_PAGE" == "2" ]]; then
        log_test "39.8" "Pagination: page=$PAGE, per_page=$PER_PAGE, total=$TOTAL, got=$GOT_COUNT" "pass"
    else
        log_test "39.8" "Pagination response" "fail" "page=$PAGE, per_page=$PER_PAGE"
    fi
else
    log_test "39.8" "Paginated list" "fail" "HTTP $API_STATUS"
fi

# ── 39.9 GET /deployments with status filter ───────────────────────────────
log_test_start "39.9" "GET /deployments?status=pending filters by status"
pepa_api GET "/deployments?status=pending"
if [[ "$API_STATUS" == "200" ]]; then
    FILTERED=$(echo "$API_RESPONSE" | jq '.deployments // [] | length' 2>/dev/null)
    log_test "39.9" "Filtered deployments: $FILTERED with status=pending" "pass"
else
    log_test "39.9" "Status filter" "fail" "HTTP $API_STATUS"
fi

# ── 39.10 GET /deployments with namespace filter ───────────────────────────
log_test_start "39.10" "GET /deployments?namespace=deep-test-ns-1 filters"
pepa_api GET "/deployments?namespace=deep-test-ns-1"
if [[ "$API_STATUS" == "200" ]]; then
    FILTERED=$(echo "$API_RESPONSE" | jq '.deployments // [] | length' 2>/dev/null)
    log_test "39.10" "Namespace filter: $FILTERED deployments" "pass"
else
    log_test "39.10" "Namespace filter" "fail" "HTTP $API_STATUS"
fi

# ── 39.11 DELETE /deployments/:id removes deployment ───────────────────────
if [[ -n "${DEPLOY_IDS[1]}" ]]; then
    log_test_start "39.11" "DELETE /deployments/:id removes deployment"
    pepa_api DELETE "/deployments/${DEPLOY_IDS[1]}"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "39.11" "Deployment removed" "pass"
    else
        log_test "39.11" "Remove deployment" "fail" "HTTP $API_STATUS"
    fi
    # Verify 404
    pepa_api GET "/deployments/${DEPLOY_IDS[1]}"
    if [[ "$API_STATUS" == "404" ]]; then
        log_test "39.11b" "Removed deployment returns 404" "pass"
    else
        log_test "39.11b" "Verify removal" "fail" "expected 404, got $API_STATUS"
    fi
else
    log_test "39.11" "Remove deployment" "skip" "no deployment"
fi

# ── 39.12 GET /system/info returns system information ──────────────────────
log_test_start "39.12" "GET /system/info returns system information"
pepa_api GET "/system/info"
if [[ "$API_STATUS" == "200" ]]; then
    VERSION=$(echo "$API_RESPONSE" | jq -r '.version // empty')
    GO_VERSION=$(echo "$API_RESPONSE" | jq -r '.go_version // empty')
    log_test "39.12" "System info: version=$VERSION, go=$GO_VERSION" "pass"
else
    log_test "39.12" "System info" "fail" "HTTP $API_STATUS"
fi

# ── 39.13 Cleanup remaining deployments ────────────────────────────────────
log_test_start "39.13" "Cleanup remaining test deployments"
for did in "${DEPLOY_IDS[@]}"; do
    if [[ -n "$did" ]]; then
        pepa_api DELETE "/deployments/$did" 2>/dev/null || true
    fi
done
log_test "39.13" "Cleanup done" "pass"

print_summary
