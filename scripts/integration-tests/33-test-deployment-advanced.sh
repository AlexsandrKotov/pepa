#!/usr/bin/env bash
# 33-test-deployment-advanced.sh — Advanced deployment tests
# Covers: deployment diff with compare_with, invalid UUID handling,
#         state transition validation, pipeline with project filter,
#         deployment logs for failed deployments

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 33: Deployment Advanced Tests"

pepa_login 2>/dev/null || true

# Helper: update deployment status directly in DB
set_deploy_status() {
    local deploy_id="$1" status="$2"
    docker exec pepa-postgres psql -U pepa -d pepa -c \
        "UPDATE deployments SET status='$status', updated_at=NOW() WHERE id='$deploy_id'" \
        -t 2>/dev/null | tr -d ' '
}

# ── 33.1 Create two deployments for diff comparison ────────────────────────
log_test_start "33.1" "Create two deployments for diff test"
pepa_api POST "/deployments" '{
    "target_namespace": "diff-test-ns",
    "image_tag": "v1.0.0",
    "image_repository": "diff-app",
    "deploy_type": "helm",
    "replicas": 1,
    "spec": {}
}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    DIFF_ID1=$(echo "$API_RESPONSE" | jq -r '.id // .deployment.id // empty')
else
    DIFF_ID1=""
fi

pepa_api POST "/deployments" '{
    "target_namespace": "diff-test-ns-2",
    "image_tag": "v2.0.0",
    "image_repository": "diff-app-2",
    "deploy_type": "kubectl",
    "replicas": 3,
    "spec": {}
}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    DIFF_ID2=$(echo "$API_RESPONSE" | jq -r '.id // .deployment.id // empty')
else
    DIFF_ID2=""
fi

if [[ -n "$DIFF_ID1" && -n "$DIFF_ID2" ]]; then
    log_test "33.1" "Created 2 deployments for diff" "pass"
else
    log_test "33.1" "Create deployments for diff" "fail" "HTTP $API_STATUS"
fi

# ── 33.2 Get deployment diff with compare_with ─────────────────────────────
if [[ -n "$DIFF_ID1" && -n "$DIFF_ID2" ]]; then
    log_test_start "33.2" "GET /deployments/:id/diff?compare_with=:id2 returns diff"
    pepa_api GET "/deployments/$DIFF_ID1/diff?compare_with=$DIFF_ID2"
    if [[ "$API_STATUS" == "200" ]]; then
        DIFF_COUNT=$(echo "$API_RESPONSE" | jq '.total_changes // .diffs | length' 2>/dev/null)
        HAS_DIFFS=$(echo "$API_RESPONSE" | jq 'has("diffs")' 2>/dev/null)
        if [[ "$HAS_DIFFS" == "true" && "$DIFF_COUNT" -ge 1 ]] 2>/dev/null; then
            log_test "33.2" "Diff: $DIFF_COUNT changes detected" "pass"
        else
            log_test "33.2" "Diff response" "fail" "no diffs found"
        fi
    else
        log_test "33.2" "Deployment diff" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "33.2" "Deployment diff" "skip" "no deployments"
fi

# ── 33.3 Diff without compare_with returns 400 ─────────────────────────────
if [[ -n "$DIFF_ID1" ]]; then
    log_test_start "33.3" "GET /deployments/:id/diff without compare_with returns 400"
    pepa_api GET "/deployments/$DIFF_ID1/diff"
    if [[ "$API_STATUS" == "400" ]]; then
        log_test "33.3" "Missing compare_with returns 400" "pass"
    else
        log_test "33.3" "Missing compare_with" "fail" "expected 400, got $API_STATUS"
    fi
else
    log_test "33.3" "Missing compare_with" "skip" "no deployment"
fi

# ── 33.4 Invalid UUID format returns 400 ───────────────────────────────────
log_test_start "33.4" "GET /deployments/invalid-uuid returns 400"
pepa_api GET "/deployments/not-a-valid-uuid"
if [[ "$API_STATUS" == "400" ]]; then
    log_test "33.4" "Invalid UUID returns 400" "pass"
else
    log_test "33.4" "Invalid UUID" "fail" "expected 400, got $API_STATUS"
fi

# ── 33.5 Promote non-deployed deployment returns 409 ───────────────────────
if [[ -n "$DIFF_ID1" ]]; then
    log_test_start "33.5" "POST /deployments/:id/promote with status=pending returns 409"
    pepa_api POST "/deployments/$DIFF_ID1/promote"
    if [[ "$API_STATUS" == "409" ]]; then
        log_test "33.5" "Promote non-deployed returns 409" "pass"
    else
        log_test "33.5" "Promote non-deployed" "fail" "expected 409, got $API_STATUS"
    fi
else
    log_test "33.5" "Promote non-deployed" "skip" "no deployment"
fi

# ── 33.6 Rollback non-deployed deployment returns 409 ──────────────────────
if [[ -n "$DIFF_ID1" ]]; then
    log_test_start "33.6" "POST /deployments/:id/rollback with status=pending returns 409"
    pepa_api POST "/deployments/$DIFF_ID1/rollback"
    if [[ "$API_STATUS" == "409" ]]; then
        log_test "33.6" "Rollback non-deployed returns 409" "pass"
    else
        log_test "33.6" "Rollback non-deployed" "fail" "expected 409, got $API_STATUS"
    fi
else
    log_test "33.6" "Rollback non-deployed" "skip" "no deployment"
fi

# ── 33.7 Retry non-failed deployment returns 409 ───────────────────────────
if [[ -n "$DIFF_ID1" ]]; then
    log_test_start "33.7" "POST /deployments/:id/retry with status=pending returns 409"
    pepa_api POST "/deployments/$DIFF_ID1/retry"
    if [[ "$API_STATUS" == "409" ]]; then
        log_test "33.7" "Retry non-failed returns 409" "pass"
    else
        log_test "33.7" "Retry non-failed" "fail" "expected 409, got $API_STATUS"
    fi
else
    log_test "33.7" "Retry non-failed" "skip" "no deployment"
fi

# ── 33.8 Create failed deployment and verify logs ──────────────────────────
log_test_start "33.8" "Create failed deployment and check logs"
pepa_api POST "/deployments" '{
    "target_namespace": "failed-test-ns",
    "image_tag": "v1.0.0-fail",
    "image_repository": "failed-app",
    "deploy_type": "helm",
    "replicas": 1,
    "spec": {}
}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    FAIL_ID=$(echo "$API_RESPONSE" | jq -r '.id // .deployment.id // empty')
    # Set status to failed and add error message
    docker exec pepa-postgres psql -U pepa -d pepa -c \
        "UPDATE deployments SET status='failed', error_message='Test failure: deployment timeout', updated_at=NOW() WHERE id='$FAIL_ID'" \
        -t 2>/dev/null | tr -d ' '
    
    pepa_api GET "/deployments/$FAIL_ID/logs"
    if [[ "$API_STATUS" == "200" ]]; then
        HAS_ERROR=$(echo "$API_RESPONSE" | jq 'has("error_message")' 2>/dev/null)
        ERROR_MSG=$(echo "$API_RESPONSE" | jq -r '.error_message // empty')
        if [[ "$HAS_ERROR" == "true" && -n "$ERROR_MSG" ]]; then
            log_test "33.8" "Failed deployment logs: error_message present" "pass"
        else
            log_test "33.8" "Failed deployment logs" "fail" "no error_message"
        fi
    else
        log_test "33.8" "Failed deployment logs" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "33.8" "Create failed deployment" "fail" "HTTP $API_STATUS"
    FAIL_ID=""
fi

# ── 33.9 Pipeline with project filter ──────────────────────────────────────
log_test_start "33.9" "GET /deployments/pipeline?project=diff-app filters by project"
pepa_api GET "/deployments/pipeline?project=diff-app"
if [[ "$API_STATUS" == "200" ]]; then
    PROJECTS=$(echo "$API_RESPONSE" | jq '.projects // [] | length' 2>/dev/null)
    log_test "33.9" "Pipeline with project filter: $PROJECTS projects" "pass"
else
    log_test "33.9" "Pipeline with project filter" "fail" "HTTP $API_STATUS"
fi

# ── 33.10 Deployment resources without target_cluster_id ───────────────────
if [[ -n "$DIFF_ID1" ]]; then
    log_test_start "33.10" "GET /deployments/:id/resources without cluster returns empty"
    pepa_api GET "/deployments/$DIFF_ID1/resources"
    if [[ "$API_STATUS" == "200" ]]; then
        RES_COUNT=$(echo "$API_RESPONSE" | jq '.resources // [] | length' 2>/dev/null)
        HAS_MSG=$(echo "$API_RESPONSE" | jq 'has("message")' 2>/dev/null)
        if [[ "$HAS_MSG" == "true" || "$RES_COUNT" == "0" ]]; then
            log_test "33.10" "Resources without cluster: empty or message" "pass"
        else
            log_test "33.10" "Resources without cluster" "fail" "unexpected response"
        fi
    else
        log_test "33.10" "Resources without cluster" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "33.10" "Resources without cluster" "skip" "no deployment"
fi

# ── 33.11 Cleanup ──────────────────────────────────────────────────────────
log_test_start "33.11" "Cleanup advanced test deployments"
for did in "$DIFF_ID1" "$DIFF_ID2" "$FAIL_ID"; do
    if [[ -n "$did" ]]; then
        pepa_api DELETE "/deployments/$did" 2>/dev/null || true
    fi
done
log_test "33.11" "Cleanup done" "pass"

print_summary
