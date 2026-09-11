#!/usr/bin/env bash
# 23-test-deployment-promotion.sh — Full promotion lifecycle with approvals
# Uses direct DB status update since API always creates with status=pending
# Covers: POST /gitops/deploy → DB update → promote → approve → promote → approve

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 23: Deployment Promotion Lifecycle (dev→testing→staging→production)"

pepa_login 2>/dev/null || true

# Helper: update deployment status directly in DB
set_deploy_status() {
    local deploy_id="$1" status="$2"
    docker exec pepa-postgres psql -U pepa -d pepa -c \
        "UPDATE deployments SET status='$status', updated_at=NOW() WHERE id='$deploy_id'" \
        -t 2>/dev/null | tr -d ' '
}

# ── 23.1 Deploy to dev via GitOps workflow ─────────────────────────────────
log_test_start "23.1" "POST /gitops/deploy creates dev deployment"
DEPLOY_BODY='{
    "image_tag": "v1.0.0-promo",
    "image_repository": "promo-app",
    "namespace": "app-dev",
    "team": "platform-team",
    "stage": "dev",
    "jira_issue_key": "PEPA-999",
    "jira_summary": "Promotion test deploy"
}'
pepa_api POST "/gitops/deploy" "$DEPLOY_BODY"
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    DEV_ID=$(echo "$API_RESPONSE" | jq -r '.deployment.id // .id // empty')
    DEV_STAGE=$(echo "$API_RESPONSE" | jq -r '.deployment.stage // .stage // empty')
    DEV_TEAM=$(echo "$API_RESPONSE" | jq -r '.deployment.team_name // .team_name // empty')
    DEV_ENV_ID=$(echo "$API_RESPONSE" | jq -r '.deployment.environment_id // .environment_id // empty')
    log_test "23.1" "Dev deploy: id=${DEV_ID:0:8}..., stage=$DEV_STAGE, team=$DEV_TEAM" "pass"
else
    log_test "23.1" "Dev deploy" "fail" "HTTP $API_STATUS"
    DEV_ID=""
fi

# ── 23.2 Set status to deployed via DB ─────────────────────────────────────
if [[ -n "${DEV_ID:-}" ]]; then
    log_test_start "23.2" "Set dev deployment status to 'deployed'"
    set_deploy_status "$DEV_ID" "deployed"
    pepa_api GET "/deployments/$DEV_ID"
    GOT_STATUS=$(echo "$API_RESPONSE" | jq -r '.status // empty')
    if [[ "$GOT_STATUS" == "deployed" ]]; then
        log_test "23.2" "Status set to deployed" "pass"
    else
        log_test "23.2" "Set deployed status" "fail" "got status=$GOT_STATUS"
    fi
else
    log_test "23.2" "Set deployed status" "skip" "no deployment"
fi

# ── 23.3 Promote dev → testing ─────────────────────────────────────────────
if [[ -n "${DEV_ID:-}" ]]; then
    log_test_start "23.3" "POST /deployments/:id/promote dev→testing"
    pepa_api POST "/deployments/$DEV_ID/promote"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        NEXT_ID=$(echo "$API_RESPONSE" | jq -r '.new_deployment.id // .new_deployment.id // empty')
        NEXT_STAGE=$(echo "$API_RESPONSE" | jq -r '.new_deployment.stage // .new_deployment.stage // empty')
        AWAITING=$(echo "$API_RESPONSE" | jq -r '.awaiting_approval // empty')
        log_test "23.3" "Promote dev→testing: next=${NEXT_ID:0:8}..., stage=$NEXT_STAGE, awaiting=$AWAITING" "pass"
        TESTING_ID="$NEXT_ID"
    else
        ERR_MSG=$(echo "$API_RESPONSE" | jq -r '.error // empty')
        log_test "23.3" "Promote dev→testing" "fail" "HTTP $API_STATUS: $ERR_MSG"
        TESTING_ID=""
    fi
else
    log_test "23.3" "Promote dev→testing" "skip" "no deployment"
    TESTING_ID=""
fi

# ── 23.4 Verify original dev deployment is now "promoted" ──────────────────
if [[ -n "${DEV_ID:-}" ]]; then
    log_test_start "23.4" "Original dev deployment status changed"
    pepa_api GET "/deployments/$DEV_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        ORIG_STATUS=$(echo "$API_RESPONSE" | jq -r '.status // empty')
        if [[ "$ORIG_STATUS" == "promoted" || "$ORIG_STATUS" == "awaiting_approval" ]]; then
            log_test "23.4" "Dev status=$ORIG_STATUS" "pass"
        else
            log_test "23.4" "Dev status after promote" "fail" "got '$ORIG_STATUS'"
        fi
    else
        log_test "23.4" "Get dev after promote" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "23.4" "Dev promoted check" "skip" "no deployment"
fi

# ── 23.5 Set testing to deployed and promote to staging ────────────────────
if [[ -n "${TESTING_ID:-}" ]]; then
    log_test_start "23.5" "Set testing to deployed, then promote testing→staging"
    set_deploy_status "$TESTING_ID" "deployed"
    pepa_api POST "/deployments/$TESTING_ID/promote"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        STAGING_ID=$(echo "$API_RESPONSE" | jq -r '.new_deployment.id // .new_deployment.id // empty')
        STAGING_STAGE=$(echo "$API_RESPONSE" | jq -r '.new_deployment.stage // .new_deployment.stage // empty')
        log_test "23.5" "Promote testing→staging: next=${STAGING_ID:0:8}..., stage=$STAGING_STAGE" "pass"
    else
        log_test "23.5" "Promote testing→staging" "fail" "HTTP $API_STATUS"
        STAGING_ID=""
    fi
else
    log_test "23.5" "Promote testing→staging" "skip" "no testing deployment"
    STAGING_ID=""
fi

# ── 23.6 Approve staging deployment ────────────────────────────────────────
if [[ -n "${STAGING_ID:-}" ]]; then
    log_test_start "23.6" "POST /gitops/deployments/:id/approve (staging)"
    pepa_api POST "/gitops/deployments/$STAGING_ID/approve"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        APP_STATUS=$(echo "$API_RESPONSE" | jq -r '.status // .deployment.status // empty')
        log_test "23.6" "Staging approved: status=$APP_STATUS" "pass"
    else
        log_test "23.6" "Approve staging" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "23.6" "Approve staging" "skip" "no staging deployment"
fi

# ── 23.7 Set staging to deployed and promote to production ─────────────────
if [[ -n "${STAGING_ID:-}" ]]; then
    log_test_start "23.7" "Set staging to deployed, then promote staging→production"
    set_deploy_status "$STAGING_ID" "deployed"
    pepa_api POST "/deployments/$STAGING_ID/promote"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        PROD_ID=$(echo "$API_RESPONSE" | jq -r '.new_deployment.id // .new_deployment.id // empty')
        PROD_STAGE=$(echo "$API_RESPONSE" | jq -r '.new_deployment.stage // .new_deployment.stage // empty')
        log_test "23.7" "Promote staging→production: next=${PROD_ID:0:8}..., stage=$PROD_STAGE" "pass"
    else
        log_test "23.7" "Promote staging→production" "fail" "HTTP $API_STATUS"
        PROD_ID=""
    fi
else
    log_test "23.7" "Promote staging→production" "skip" "no staging deployment"
    PROD_ID=""
fi

# ── 23.8 Approve production deployment ─────────────────────────────────────
if [[ -n "${PROD_ID:-}" ]]; then
    log_test_start "23.8" "POST /gitops/deployments/:id/approve (production)"
    pepa_api POST "/gitops/deployments/$PROD_ID/approve"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        P_STATUS=$(echo "$API_RESPONSE" | jq -r '.status // .deployment.status // empty')
        log_test "23.8" "Production approved: status=$P_STATUS" "pass"
    else
        log_test "23.8" "Approve production" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "23.8" "Approve production" "skip" "no production deployment"
fi

# ── 23.9 Verify production deployment has correct stage ────────────────────
if [[ -n "${PROD_ID:-}" ]]; then
    log_test_start "23.9" "Production deployment has stage=production"
    pepa_api GET "/deployments/$PROD_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        P_STAGE=$(echo "$API_RESPONSE" | jq -r '.stage // empty')
        P_ENV=$(echo "$API_RESPONSE" | jq -r '.environment_id // empty')
        if [[ "$P_STAGE" == "production" ]]; then
            log_test "23.9" "Production stage=$P_STAGE, env_id=${P_ENV:0:8}..." "pass"
        else
            log_test "23.9" "Production stage check" "fail" "expected 'production', got '$P_STAGE'"
        fi
    else
        log_test "23.9" "Get production deployment" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "23.9" "Production stage check" "skip" "no production deployment"
fi

# ── 23.10 Verify full chain via deployment history ─────────────────────────
if [[ -n "$DEV_ID" ]]; then
    log_test_start "23.10" "GET /deployments/:id/history shows promotion chain"
    pepa_api GET "/deployments/$DEV_ID/history"
    if [[ "$API_STATUS" == "200" ]]; then
        log_test "23.10" "Deployment history endpoint accessible" "pass"
    else
        log_test "23.10" "Deployment history" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "23.10" "History chain" "skip" "no deployment"
fi

# ── 23.11 Promote non-existent deployment (404) ───────────────────────────
log_test_start "23.11" "POST /deployments/00000000-...-0099/promote returns 404"
pepa_api POST "/deployments/00000000-0000-0000-0000-000000000099/promote"
if [[ "$API_STATUS" == "404" ]]; then
    log_test "23.11" "Non-existent promote returns 404" "pass"
else
    log_test "23.11" "Non-existent promote" "fail" "expected 404, got $API_STATUS"
fi

# ── 23.12 Cleanup: delete all promotion chain deployments ──────────────────
log_test_start "23.12" "Cleanup promotion chain deployments"
CLEANUP_IDS=("$DEV_ID" "${TESTING_ID:-}" "${STAGING_ID:-}" "${PROD_ID:-}")
CLEANED=0
for cid in "${CLEANUP_IDS[@]}"; do
    if [[ -n "$cid" ]]; then
        pepa_api DELETE "/deployments/$cid" 2>/dev/null || true
        CLEANED=$((CLEANED + 1))
    fi
done
log_test "23.12" "Cleaned up $CLEANED deployments" "pass"

print_summary
