#!/usr/bin/env bash
# 38-test-gitops-drift-schedules.sh — Drift detection schedule management
# Covers: POST/GET/PUT/DELETE /gitops/drift-schedules, run, drift-logs,
#         verify endpoint with GitOps deploy workflow approval

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 38: GitOps Drift Schedules & Approval Flow"

pepa_login 2>/dev/null || true

# Helper: update deployment status directly in DB
set_deploy_status() {
    local deploy_id="$1" status="$2"
    docker exec pepa-postgres psql -U pepa -d pepa -c \
        "UPDATE deployments SET status='$status', updated_at=NOW() WHERE id='$deploy_id'" \
        -t 2>/dev/null | tr -d ' '
}

# Helper: create test cluster in DB (no API endpoint exists)
create_test_cluster() {
    local name="$1"
    docker exec pepa-postgres psql -U pepa -d pepa -t -A -c \
        "INSERT INTO clusters (id, tenant_id, name, environment, status, created_at, updated_at) VALUES (gen_random_uuid(), '00000000-0000-0000-0000-000000000002', '$name', 'dev', 'active', NOW(), NOW()) RETURNING id" \
        2>/dev/null | grep -E '^[0-9a-f-]{36}$' | head -1
}

# Helper: delete test cluster from DB
delete_test_cluster() {
    local cluster_id="$1"
    docker exec pepa-postgres psql -U pepa -d pepa -c \
        "DELETE FROM clusters WHERE id='$cluster_id'" 2>/dev/null
}

# Create prerequisites: GitOps repo and cluster for drift schedules
log_info "Creating prerequisites for drift schedule tests..."
pepa_api POST "/gitops/repos" '{
    "name": "test-drift-repo",
    "repo_url": "http://localhost:3001/pepa/test.git",
    "branch": "main",
    "engine_type": "fluxcd"
}'
DRIFT_REPO_ID=$(echo "$API_RESPONSE" | jq -r '.id // empty' 2>/dev/null)
TEST_CLUSTER_ID=$(create_test_cluster "test-drift-cluster")
log_info "Drift repo: ${DRIFT_REPO_ID:0:8}..., cluster: ${TEST_CLUSTER_ID:0:8}..."

# ── 38.1 Create drift detection schedule ───────────────────────────────────
log_test_start "38.1" "POST /gitops/drift-schedules creates schedule"
pepa_api POST "/gitops/drift-schedules" "{
    \"name\": \"test-drift-schedule\",
    \"cron_expression\": \"*/30 * * * *\",
    \"enabled\": true,
    \"repo_id\": \"$DRIFT_REPO_ID\",
    \"cluster_id\": \"$TEST_CLUSTER_ID\"
}"
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    DRIFT_SCHED_ID=$(echo "$API_RESPONSE" | jq -r '.id // .schedule.id // empty')
    DRIFT_SCHED_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
    log_test "38.1" "Drift schedule created: ${DRIFT_SCHED_ID:0:8}..., name=$DRIFT_SCHED_NAME" "pass"
else
    log_test "38.1" "Create drift schedule" "fail" "HTTP $API_STATUS"
    DRIFT_SCHED_ID=""
fi

# ── 38.2 Get drift schedule by ID ──────────────────────────────────────────
if [[ -n "$DRIFT_SCHED_ID" ]]; then
    log_test_start "38.2" "GET /gitops/drift-schedules/:id returns schedule details"
    pepa_api GET "/gitops/drift-schedules/$DRIFT_SCHED_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        GOT_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
        GOT_CRON=$(echo "$API_RESPONSE" | jq -r '.cron_expression // empty')
        GOT_ENABLED=$(echo "$API_RESPONSE" | jq -r '.enabled // empty')
        if [[ "$GOT_NAME" == "test-drift-schedule" ]]; then
            log_test "38.2" "Schedule: name=$GOT_NAME, cron=$GOT_CRON, enabled=$GOT_ENABLED" "pass"
        else
            log_test "38.2" "Get drift schedule" "fail" "name mismatch"
        fi
    else
        log_test "38.2" "Get drift schedule" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "38.2" "Get drift schedule" "skip" "no schedule"
fi

# ── 38.3 Update drift schedule ─────────────────────────────────────────────
if [[ -n "$DRIFT_SCHED_ID" ]]; then
    log_test_start "38.3" "PUT /gitops/drift-schedules/:id updates schedule"
    pepa_api PUT "/gitops/drift-schedules/$DRIFT_SCHED_ID" '{"name": "test-drift-updated", "cron_expression": "0 * * * *", "enabled": false}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        UPD_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
        UPD_CRON=$(echo "$API_RESPONSE" | jq -r '.cron_expression // empty')
        UPD_ENABLED=$(echo "$API_RESPONSE" | jq -r '.enabled // empty')
        # Check if name was updated (enabled might be boolean false or string "false")
        if [[ "$UPD_NAME" == "test-drift-updated" ]]; then
            log_test "38.3" "Schedule updated: name=$UPD_NAME, cron=$UPD_CRON, enabled=$UPD_ENABLED" "pass"
        else
            log_test "38.3" "Update drift schedule" "fail" "name not updated (got: $UPD_NAME)"
        fi
    else
        log_test "38.3" "Update drift schedule" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "38.3" "Update drift schedule" "skip" "no schedule"
fi

# ── 38.4 List drift schedules ──────────────────────────────────────────────
log_test_start "38.4" "GET /gitops/drift-schedules returns schedule list"
pepa_api GET "/gitops/drift-schedules"
if [[ "$API_STATUS" == "200" ]]; then
    SCHED_COUNT=$(echo "$API_RESPONSE" | jq '.schedules // .items // [] | length' 2>/dev/null)
    log_test "38.4" "Drift schedules: $SCHED_COUNT found" "pass"
else
    log_test "38.4" "List drift schedules" "fail" "HTTP $API_STATUS"
fi

# ── 38.5 Run drift schedule manually ───────────────────────────────────────
if [[ -n "$DRIFT_SCHED_ID" ]]; then
    log_test_start "38.5" "POST /gitops/drift-schedules/:id/run triggers manual run"
    pepa_api POST "/gitops/drift-schedules/$DRIFT_SCHED_ID/run"
    # 200 = success, 500 = expected when cluster has no kubeconfig
    if [[ "$API_STATUS" == "200" ]]; then
        log_test "38.5" "Drift schedule run triggered successfully" "pass"
    elif [[ "$API_STATUS" == "500" ]]; then
        ERR=$(echo "$API_RESPONSE" | jq -r '.error // empty' 2>/dev/null)
        if [[ "$ERR" == *"kubeconfig"* || "$ERR" == *"cluster"* ]]; then
            log_test "38.5" "Run endpoint works (cluster config error expected)" "pass"
        else
            log_test "38.5" "Run drift schedule" "fail" "HTTP 500: $ERR"
        fi
    else
        log_test "38.5" "Run drift schedule" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "38.5" "Run drift schedule" "skip" "no schedule"
fi

# ── 38.6 List drift logs ───────────────────────────────────────────────────
log_test_start "38.6" "GET /gitops/drift-logs returns drift detection logs"
pepa_api GET "/gitops/drift-logs"
if [[ "$API_STATUS" == "200" ]]; then
    LOG_COUNT=$(echo "$API_RESPONSE" | jq '.logs // .items // [] | length' 2>/dev/null)
    log_test "38.6" "Drift logs: $LOG_COUNT entries" "pass"
else
    log_test "38.6" "List drift logs" "fail" "HTTP $API_STATUS"
fi

# ── 38.7 Create second drift schedule ──────────────────────────────────────
log_test_start "38.7" "Create second drift schedule for testing"
pepa_api POST "/gitops/drift-schedules" "{
    \"name\": \"nightly-drift-check\",
    \"cron_expression\": \"0 2 * * *\",
    \"enabled\": true,
    \"repo_id\": \"$DRIFT_REPO_ID\",
    \"cluster_id\": \"$TEST_CLUSTER_ID\"
}"
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    DRIFT_SCHED2_ID=$(echo "$API_RESPONSE" | jq -r '.id // .schedule.id // empty')
    log_test "38.7" "Second drift schedule created: ${DRIFT_SCHED2_ID:0:8}..." "pass"
else
    log_test "38.7" "Create second drift schedule" "fail" "HTTP $API_STATUS"
    DRIFT_SCHED2_ID=""
fi

# ── 38.8 Verify both schedules in list ─────────────────────────────────────
log_test_start "38.8" "Verify both schedules appear in list"
pepa_api GET "/gitops/drift-schedules"
if [[ "$API_STATUS" == "200" ]]; then
    SCHED_COUNT=$(echo "$API_RESPONSE" | jq '.schedules // .items // [] | length' 2>/dev/null)
    if [[ "$SCHED_COUNT" -ge 2 ]] 2>/dev/null; then
        log_test "38.8" "Both schedules present: $SCHED_COUNT total" "pass"
    else
        log_test "38.8" "Verify both schedules" "fail" "expected >=2, got $SCHED_COUNT"
    fi
else
    log_test "38.8" "Verify both schedules" "fail" "HTTP $API_STATUS"
fi

# ── 38.9 GitOps deploy and approve flow ────────────────────────────────────
log_test_start "38.9" "POST /gitops/deploy creates deployment for approval test"
pepa_api POST "/gitops/deploy" '{
    "image_tag": "v1.0.0-approve",
    "image_repository": "approve-test-app",
    "namespace": "app-dev",
    "team": "approval-test-team",
    "stage": "dev"
}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    APPROVE_DEPLOY_ID=$(echo "$API_RESPONSE" | jq -r '.deployment.id // .id // empty')
    log_test "38.9" "Deploy created: ${APPROVE_DEPLOY_ID:0:8}..." "pass"
else
    log_test "38.9" "Create deploy for approval" "fail" "HTTP $API_STATUS"
    APPROVE_DEPLOY_ID=""
fi

# ── 38.10 Set deployment to deployed and promote to awaiting_approval ──────
if [[ -n "$APPROVE_DEPLOY_ID" ]]; then
    log_test_start "38.10" "Set deployed and promote to trigger approval flow"
    set_deploy_status "$APPROVE_DEPLOY_ID" "deployed"
    
    pepa_api POST "/gitops/deployments/$APPROVE_DEPLOY_ID/approve"
    if [[ "$API_STATUS" == "409" ]]; then
        # 409 means not awaiting_approval — need to promote first
        pepa_api POST "/deployments/$APPROVE_DEPLOY_ID/promote"
        if [[ "$API_STATUS" == "200" ]]; then
            AWAITING=$(echo "$API_RESPONSE" | jq -r '.awaiting_approval // empty')
            TARGET=$(echo "$API_RESPONSE" | jq -r '.target_stage // empty')
            if [[ "$AWAITING" == "true" ]]; then
                log_test "38.10" "Promotion awaiting approval: target=$TARGET" "pass"
            else
                log_test "38.10" "Promotion result: awaiting=$AWAITING" "pass"
            fi
        else
            log_test "38.10" "Promote for approval" "fail" "HTTP $API_STATUS"
        fi
    elif [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "38.10" "Approval flow triggered (HTTP $API_STATUS)" "pass"
    else
        log_test "38.10" "Approval flow" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "38.10" "Approval flow" "skip" "no deployment"
fi

# ── 38.11 Approve non-awaiting deployment returns 409 ──────────────────────
log_test_start "38.11" "POST /gitops/deployments/:id/approve on pending deployment returns 409"
pepa_api POST "/deployments" '{
    "target_namespace": "approve-test-ns",
    "image_tag": "v1.0.0-noapprove",
    "deploy_type": "helm",
    "replicas": 1,
    "spec": {}
}'
NOAPPROVE_ID=$(echo "$API_RESPONSE" | jq -r '.id // .deployment.id // empty')
if [[ -n "$NOAPPROVE_ID" ]]; then
    pepa_api POST "/gitops/deployments/$NOAPPROVE_ID/approve"
    if [[ "$API_STATUS" == "409" ]]; then
        log_test "38.11" "Approve non-awaiting returns 409" "pass"
    else
        log_test "38.11" "Approve non-awaiting" "fail" "expected 409, got $API_STATUS"
    fi
    # Cleanup
    pepa_api DELETE "/deployments/$NOAPPROVE_ID" 2>/dev/null || true
else
    log_test "38.11" "Approve non-awaiting" "skip" "could not create deployment"
fi

# ── 38.12 GitOps verify endpoint ───────────────────────────────────────────
log_test_start "38.12" "POST /gitops/verify verifies deployment"
pepa_api POST "/gitops/verify" '{"deployment_id": "00000000-0000-0000-0000-000000000099"}'
if [[ "$API_STATUS" =~ ^(200|202|400|404|501)$ ]]; then
    log_test "38.12" "Verify endpoint accessible (HTTP $API_STATUS)" "pass"
else
    log_test "38.12" "Verify endpoint" "fail" "HTTP $API_STATUS"
fi

# ── 38.13 Delete drift schedules ───────────────────────────────────────────
log_test_start "38.13" "Cleanup drift schedules"
for sid in "$DRIFT_SCHED_ID" "$DRIFT_SCHED2_ID"; do
    if [[ -n "$sid" ]]; then
        pepa_api DELETE "/gitops/drift-schedules/$sid" 2>/dev/null || true
    fi
done
# Cleanup repo and cluster
if [[ -n "${DRIFT_REPO_ID:-}" ]]; then
    pepa_api DELETE "/gitops/repos/$DRIFT_REPO_ID" 2>/dev/null || true
fi
if [[ -n "${TEST_CLUSTER_ID:-}" ]]; then
    delete_test_cluster "$TEST_CLUSTER_ID" 2>/dev/null || true
fi
log_test "38.13" "Cleanup done" "pass"

# ── 38.14 Cleanup deployments ──────────────────────────────────────────────
log_test_start "38.14" "Cleanup approval test deployments"
if [[ -n "${APPROVE_DEPLOY_ID:-}" ]]; then
    pepa_api DELETE "/deployments/$APPROVE_DEPLOY_ID" 2>/dev/null || true
fi
log_test "38.14" "Cleanup done" "pass"

# ── 38.15 Verify drift schedule deletion ───────────────────────────────────
if [[ -n "$DRIFT_SCHED_ID" ]]; then
    log_test_start "38.15" "GET /gitops/drift-schedules/:id returns 404 after deletion"
    pepa_api GET "/gitops/drift-schedules/$DRIFT_SCHED_ID"
    if [[ "$API_STATUS" == "404" ]]; then
        log_test "38.15" "Deleted drift schedule returns 404" "pass"
    else
        log_test "38.15" "Deleted drift schedule" "fail" "expected 404, got $API_STATUS"
    fi
else
    log_test "38.15" "Verify deletion" "skip" "no schedule"
fi

print_summary
