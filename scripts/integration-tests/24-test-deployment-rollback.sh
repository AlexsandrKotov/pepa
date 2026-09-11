#!/usr/bin/env bash
# 24-test-deployment-rollback.sh — Rollback, cancel, retry operations
# Uses direct DB status update since API always creates with status=pending
# Covers: POST /deployments/:id/rollback, POST /deployments/:id/cancel, POST /deployments/:id/retry

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 24: Deployment Rollback, Cancel & Retry"

pepa_login 2>/dev/null || true

# Helper: update deployment status directly in DB
set_deploy_status() {
    local deploy_id="$1" status="$2"
    docker exec pepa-postgres psql -U pepa -d pepa -c \
        "UPDATE deployments SET status='$status', updated_at=NOW() WHERE id='$deploy_id'" \
        -t 2>/dev/null | tr -d ' '
}

# ── 24.1 Create deployment for rollback test ───────────────────────────────
log_test_start "24.1" "Create deployment for rollback test"
pepa_api POST "/deployments" '{
    "target_namespace": "app-dev",
    "image_tag": "v1.0.0-rb",
    "image_repository": "rollback-app",
    "deploy_type": "helm",
    "replicas": 1,
    "spec": {}
}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    RB_ID=$(echo "$API_RESPONSE" | jq -r '.id // .deployment.id // empty')
    set_deploy_status "$RB_ID" "deployed"
    log_test "24.1" "Deployment created and set to deployed: ${RB_ID:0:8}..." "pass"
else
    log_test "24.1" "Create rollback test deployment" "fail" "HTTP $API_STATUS"
    RB_ID=""
fi

# ── 24.2 Rollback the deployment ───────────────────────────────────────────
if [[ -n "$RB_ID" ]]; then
    log_test_start "24.2" "POST /deployments/:id/rollback"
    pepa_api POST "/deployments/$RB_ID/rollback"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        RB_STATUS=$(echo "$API_RESPONSE" | jq -r '.status // .deployment.status // empty')
        if [[ "$RB_STATUS" == "rolled_back" ]]; then
            log_test "24.2" "Rollback successful: status=$RB_STATUS" "pass"
        else
            log_test "24.2" "Rollback status" "fail" "expected 'rolled_back', got '$RB_STATUS'"
        fi
    else
        log_test "24.2" "Rollback" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "24.2" "Rollback" "skip" "no deployment"
fi

# ── 24.3 Verify rollback status persists ───────────────────────────────────
if [[ -n "$RB_ID" ]]; then
    log_test_start "24.3" "GET /deployments/:id confirms rolled_back status"
    pepa_api GET "/deployments/$RB_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        GOT_STATUS=$(echo "$API_RESPONSE" | jq -r '.status // empty')
        if [[ "$GOT_STATUS" == "rolled_back" ]]; then
            log_test "24.3" "Status persisted: $GOT_STATUS" "pass"
        else
            log_test "24.3" "Rollback status persist" "fail" "expected 'rolled_back', got '$GOT_STATUS'"
        fi
    else
        log_test "24.3" "Get rolled-back deployment" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "24.3" "Verify rollback" "skip" "no deployment"
fi

# ── 24.4 Create deployment for cancel test ─────────────────────────────────
log_test_start "24.4" "Create deployment for cancel test"
pepa_api POST "/deployments" '{
    "target_namespace": "app-testing",
    "image_tag": "v1.0.0-cancel",
    "image_repository": "cancel-app",
    "deploy_type": "helm",
    "replicas": 1,
    "spec": {}
}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    CANCEL_ID=$(echo "$API_RESPONSE" | jq -r '.id // .deployment.id // empty')
    log_test "24.4" "Cancel test deployment created: ${CANCEL_ID:0:8}..." "pass"
else
    log_test "24.4" "Create cancel test deployment" "fail" "HTTP $API_STATUS"
    CANCEL_ID=""
fi

# ── 24.5 Cancel the deployment ─────────────────────────────────────────────
if [[ -n "$CANCEL_ID" ]]; then
    log_test_start "24.5" "POST /deployments/:id/cancel"
    pepa_api POST "/deployments/$CANCEL_ID/cancel"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        C_STATUS=$(echo "$API_RESPONSE" | jq -r '.status // .deployment.status // empty')
        log_test "24.5" "Cancel: status=$C_STATUS" "pass"
    else
        log_test "24.5" "Cancel" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "24.5" "Cancel" "skip" "no deployment"
fi

# ── 24.6 Verify cancel status ──────────────────────────────────────────────
if [[ -n "$CANCEL_ID" ]]; then
    log_test_start "24.6" "GET /deployments/:id confirms cancelled status"
    pepa_api GET "/deployments/$CANCEL_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        GOT_STATUS=$(echo "$API_RESPONSE" | jq -r '.status // empty')
        if [[ "$GOT_STATUS" == "cancelled" || "$GOT_STATUS" == "canceled" ]]; then
            log_test "24.6" "Cancel status persisted: $GOT_STATUS" "pass"
        else
            log_test "24.6" "Cancel status persist" "fail" "expected 'cancelled', got '$GOT_STATUS'"
        fi
    else
        log_test "24.6" "Get cancelled deployment" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "24.6" "Verify cancel" "skip" "no deployment"
fi

# ── 24.7 Create deployment for retry test ──────────────────────────────────
log_test_start "24.7" "Create deployment for retry test"
pepa_api POST "/deployments" '{
    "target_namespace": "app-dev",
    "image_tag": "v1.0.0-retry",
    "image_repository": "retry-app",
    "deploy_type": "helm",
    "replicas": 1,
    "spec": {}
}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    RETRY_ID=$(echo "$API_RESPONSE" | jq -r '.id // .deployment.id // empty')
    set_deploy_status "$RETRY_ID" "failed"
    log_test "24.7" "Retry test deployment created and set to failed: ${RETRY_ID:0:8}..." "pass"
else
    log_test "24.7" "Create retry test deployment" "fail" "HTTP $API_STATUS"
    RETRY_ID=""
fi

# ── 24.8 Retry the deployment ──────────────────────────────────────────────
if [[ -n "$RETRY_ID" ]]; then
    log_test_start "24.8" "POST /deployments/:id/retry"
    pepa_api POST "/deployments/$RETRY_ID/retry"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        R_STATUS=$(echo "$API_RESPONSE" | jq -r '.status // .deployment.status // empty')
        R_NEW_ID=$(echo "$API_RESPONSE" | jq -r '.id // .new_deployment.id // empty')
        log_test "24.8" "Retry: new_id=${R_NEW_ID:0:8}..., status=$R_STATUS" "pass"
    else
        log_test "24.8" "Retry" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "24.8" "Retry" "skip" "no deployment"
fi

# ── 24.9 Rollback non-existent deployment (404) ───────────────────────────
log_test_start "24.9" "POST /deployments/00000000-...-0099/rollback returns 404"
pepa_api POST "/deployments/00000000-0000-0000-0000-000000000099/rollback"
if [[ "$API_STATUS" == "404" ]]; then
    log_test "24.9" "Rollback non-existent returns 404" "pass"
else
    log_test "24.9" "Rollback non-existent" "fail" "expected 404, got $API_STATUS"
fi

# ── 24.10 Cancel non-existent deployment (404) ────────────────────────────
log_test_start "24.10" "POST /deployments/00000000-...-0099/cancel returns 4xx"
pepa_api POST "/deployments/00000000-0000-0000-0000-000000000099/cancel"
if [[ "$API_STATUS" == "404" || "$API_STATUS" == "409" ]]; then
    log_test "24.10" "Cancel non-existent returns $API_STATUS" "pass"
else
    log_test "24.10" "Cancel non-existent" "fail" "expected 404, got $API_STATUS"
fi

# ── 24.11 Cleanup ──────────────────────────────────────────────────────────
log_test_start "24.11" "Cleanup rollback/cancel/retry test deployments"
for cid in "$RB_ID" "$CANCEL_ID" "$RETRY_ID"; do
    if [[ -n "$cid" ]]; then
        pepa_api DELETE "/deployments/$cid" 2>/dev/null || true
    fi
done
log_test "24.11" "Cleanup done" "pass"

print_summary
