#!/usr/bin/env bash
# 03-test-direct-deploy.sh — Direct k8s and Helm deploy via PEPA API (Phase 3)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"
source "${SCRIPT_DIR}/lib/assertions.sh"

PEPA_URL="${PEPA_URL:-http://localhost:8088}"
API="${PEPA_URL}/api/v1"
TMP="${RESULTS_DIR}/tmp"
mkdir -p "$TMP"
TEST_NS="${TEST_NAMESPACE:-pepa-test}"

log_phase "Phase 3: Direct k8s and Helm Deploy"

# Load token and IDs from previous phases
[[ -f "${RESULTS_DIR}/pepa_token" ]] && export PEPA_TOKEN=$(cat "${RESULTS_DIR}/pepa_token")
CONN_ID=$(cat "${RESULTS_DIR}/conn_primary_id" 2>/dev/null || echo "")
CLUSTER_ID=$(cat "${RESULTS_DIR}/cluster_primary_id" 2>/dev/null || echo "")
PRIMARY_CONTEXT=$(cat "${RESULTS_DIR}/primary_context" 2>/dev/null || echo "k3d-pepa-test-primary")

DEPLOY_IDS=()

cleanup_phase() {
    log_info "Cleaning up Phase 3 resources..."
    for id in "${DEPLOY_IDS[@]}"; do
        pepa_api DELETE "/deployments/$id" "" /dev/null /dev/null 2>/dev/null || true
    done
    # Clean up any remaining k8s resources
    kubectl --context "$PRIMARY_CONTEXT" delete namespace "$TEST_NS" --ignore-not-found 2>/dev/null || true
    kubectl --context "$PRIMARY_CONTEXT" create namespace "$TEST_NS" 2>/dev/null || true
}
trap cleanup_phase EXIT

# Ensure test namespace exists
kubectl --context "$PRIMARY_CONTEXT" create namespace "$TEST_NS" --dry-run=client -o yaml | \
    kubectl --context "$PRIMARY_CONTEXT" apply -f - 2>/dev/null

# ---------------------------------------------------------------------------
# 3.1 Create deployment (nginx)
# ---------------------------------------------------------------------------
log_test_start "3.1" "Create deployment (nginx)"
pepa_api POST "/deployments" \
    "{
        \"name\": \"nginx-test\",
        \"namespace\": \"${TEST_NS}\",
        \"cluster_id\": \"${CLUSTER_ID}\",
        \"connection_id\": \"${CONN_ID}\",
        \"containers\": [{
            \"name\": \"nginx\",
            \"image\": \"nginx:1.25-alpine\",
            \"ports\": [{\"containerPort\": 80}]
        }],
        \"service\": {
            \"enabled\": true,
            \"port\": 80,
            \"targetPort\": 80,
            \"type\": \"ClusterIP\"
        }
    }" \
    "$TMP/3.1_deploy.json" "$TMP/3.1_code.txt"
if assert_http_status "$TMP/3.1_code.txt" "201" "3.1 create deployment"; then
    DEPLOY1_ID=$(jq -r '.id // .deployment_id // empty' "$TMP/3.1_deploy.json" 2>/dev/null)
    DEPLOY_IDS+=("$DEPLOY1_ID")
    log_test_pass "3.1" "Deployment created (id=${DEPLOY1_ID:-unknown})"
else
    log_test_fail "3.1" "Create deployment failed"
    DEPLOY1_ID=""
fi

# ---------------------------------------------------------------------------
# 3.2 Dry-run deployment
# ---------------------------------------------------------------------------
log_test_start "3.2" "Dry-run deployment"
pepa_api POST "/deployments/dry-run" \
    "{
        \"name\": \"nginx-dryrun\",
        \"namespace\": \"${TEST_NS}\",
        \"cluster_id\": \"${CLUSTER_ID}\",
        \"containers\": [{
            \"name\": \"nginx\",
            \"image\": \"nginx:1.25-alpine\",
            \"ports\": [{\"containerPort\": 80}]
        }]
    }" \
    "$TMP/3.2_dryrun.json" "$TMP/3.2_code.txt"
if assert_http_success "$TMP/3.2_code.txt" "3.2 dry-run"; then
    log_test_pass "3.2" "Dry-run succeeded"
else
    log_test_fail "3.2" "Dry-run failed"
fi

# ---------------------------------------------------------------------------
# 3.3 Verify Deployment in cluster
# ---------------------------------------------------------------------------
log_test_start "3.3" "Verify Deployment in cluster"
if wait_for_k8s "deploy" "nginx-test" "$TEST_NS" 60; then
    log_test_pass "3.3" "nginx-test deployment found in cluster"
else
    log_test_fail "3.3" "nginx-test deployment not found after 60s"
fi

# ---------------------------------------------------------------------------
# 3.4 Verify Service in cluster
# ---------------------------------------------------------------------------
log_test_start "3.4" "Verify Service in cluster"
if kubectl --context "$PRIMARY_CONTEXT" get svc nginx-test -n "$TEST_NS" &>/dev/null; then
    log_test_pass "3.4" "nginx-test service found"
else
    log_test_fail "3.4" "nginx-test service not found"
fi

# ---------------------------------------------------------------------------
# 3.5 Get deployment details
# ---------------------------------------------------------------------------
log_test_start "3.5" "Get deployment details"
if [[ -n "$DEPLOY1_ID" ]]; then
    pepa_api GET "/deployments/${DEPLOY1_ID}" "" "$TMP/3.5_detail.json" "$TMP/3.5_code.txt"
    if assert_http_success "$TMP/3.5_code.txt" "3.5 deployment details"; then
        log_test_pass "3.5" "Deployment details returned"
    else
        log_test_fail "3.5" "Get deployment details failed"
    fi
else
    log_test_skip "3.5" "No deployment ID"
fi

# ---------------------------------------------------------------------------
# 3.6 List deployments
# ---------------------------------------------------------------------------
log_test_start "3.6" "List deployments"
pepa_api GET "/deployments" "" "$TMP/3.6_list.json" "$TMP/3.6_code.txt"
if assert_http_success "$TMP/3.6_code.txt" "3.6 list deployments"; then
    log_test_pass "3.6" "Deployments listed"
else
    log_test_fail "3.6" "List deployments failed"
fi

# ---------------------------------------------------------------------------
# 3.7 Deployment metrics
# ---------------------------------------------------------------------------
log_test_start "3.7" "Deployment metrics"
pepa_api GET "/deployments/metrics" "" "$TMP/3.7_metrics.json" "$TMP/3.7_code.txt"
if assert_http_success "$TMP/3.7_code.txt" "3.7 metrics"; then
    log_test_pass "3.7" "Deployment metrics returned"
else
    log_test_fail "3.7" "Deployment metrics failed"
fi

# ---------------------------------------------------------------------------
# 3.8 Deployment pipeline
# ---------------------------------------------------------------------------
log_test_start "3.8" "Deployment pipeline"
pepa_api GET "/deployments/pipeline" "" "$TMP/3.8_pipeline.json" "$TMP/3.8_code.txt"
if assert_http_success "$TMP/3.8_code.txt" "3.8 pipeline"; then
    log_test_pass "3.8" "Deployment pipeline returned"
else
    log_test_fail "3.8" "Deployment pipeline failed"
fi

# ---------------------------------------------------------------------------
# 3.9 Deployment history
# ---------------------------------------------------------------------------
log_test_start "3.9" "Deployment history"
if [[ -n "$DEPLOY1_ID" ]]; then
    pepa_api GET "/deployments/${DEPLOY1_ID}/history" "" "$TMP/3.9_history.json" "$TMP/3.9_code.txt"
    if assert_http_success "$TMP/3.9_code.txt" "3.9 history"; then
        log_test_pass "3.9" "Deployment history returned"
    else
        log_test_fail "3.9" "Deployment history failed"
    fi
else
    log_test_skip "3.9" "No deployment ID"
fi

# ---------------------------------------------------------------------------
# 3.10 Deployment logs
# ---------------------------------------------------------------------------
log_test_start "3.10" "Deployment logs"
if [[ -n "$DEPLOY1_ID" ]]; then
    pepa_api GET "/deployments/${DEPLOY1_ID}/logs" "" "$TMP/3.10_logs.json" "$TMP/3.10_code.txt"
    if assert_http_success "$TMP/3.10_code.txt" "3.10 logs"; then
        log_test_pass "3.10" "Deployment logs returned"
    else
        log_test_fail "3.10" "Deployment logs failed"
    fi
else
    log_test_skip "3.10" "No deployment ID"
fi

# ---------------------------------------------------------------------------
# 3.11 Deployment diff
# ---------------------------------------------------------------------------
log_test_start "3.11" "Deployment diff"
if [[ -n "$DEPLOY1_ID" ]]; then
    pepa_api GET "/deployments/${DEPLOY1_ID}/diff" "" "$TMP/3.11_diff.json" "$TMP/3.11_code.txt"
    if assert_http_success "$TMP/3.11_code.txt" "3.11 diff"; then
        log_test_pass "3.11" "Deployment diff returned"
    else
        log_test_fail "3.11" "Deployment diff failed"
    fi
else
    log_test_skip "3.11" "No deployment ID"
fi

# ---------------------------------------------------------------------------
# 3.12 Deployment resources
# ---------------------------------------------------------------------------
log_test_start "3.12" "Deployment resources"
if [[ -n "$DEPLOY1_ID" ]]; then
    pepa_api GET "/deployments/${DEPLOY1_ID}/resources" "" "$TMP/3.12_res.json" "$TMP/3.12_code.txt"
    if assert_http_success "$TMP/3.12_code.txt" "3.12 resources"; then
        log_test_pass "3.12" "Deployment resources returned"
    else
        log_test_fail "3.12" "Deployment resources failed"
    fi
else
    log_test_skip "3.12" "No deployment ID"
fi

# ---------------------------------------------------------------------------
# 3.13 Deployment events
# ---------------------------------------------------------------------------
log_test_start "3.13" "Deployment events"
if [[ -n "$DEPLOY1_ID" ]]; then
    pepa_api GET "/deployments/${DEPLOY1_ID}/events" "" "$TMP/3.13_events.json" "$TMP/3.13_code.txt"
    if assert_http_success "$TMP/3.13_code.txt" "3.13 events"; then
        log_test_pass "3.13" "Deployment events returned"
    else
        log_test_fail "3.13" "Deployment events failed"
    fi
else
    log_test_skip "3.13" "No deployment ID"
fi

# ---------------------------------------------------------------------------
# 3.14 Deploy Helm chart (podinfo)
# ---------------------------------------------------------------------------
log_test_start "3.14" "Deploy Helm chart (podinfo)"
pepa_api POST "/deployments" \
    "{
        \"name\": \"podinfo-test\",
        \"namespace\": \"${TEST_NS}\",
        \"cluster_id\": \"${CLUSTER_ID}\",
        \"connection_id\": \"${CONN_ID}\",
        \"chart\": {
            \"name\": \"podinfo\",
            \"repo\": \"oci://ghcr.io/stefanprodan/charts\",
            \"version\": \"6.5.4\"
        },
        \"values\": {
            \"replicaCount\": 1,
            \"resources\": {
                \"requests\": {\"cpu\": \"10m\", \"memory\": \"32Mi\"},
                \"limits\": {\"cpu\": \"100m\", \"memory\": \"64Mi\"}
            }
        }
    }" \
    "$TMP/3.14_helm.json" "$TMP/3.14_code.txt"
if assert_http_status "$TMP/3.14_code.txt" "201" "3.14 Helm deploy"; then
    DEPLOY2_ID=$(jq -r '.id // .deployment_id // empty' "$TMP/3.14_helm.json" 2>/dev/null)
    [[ -n "$DEPLOY2_ID" ]] && DEPLOY_IDS+=("$DEPLOY2_ID")
    log_test_pass "3.14" "Helm chart deployed (id=${DEPLOY2_ID:-unknown})"
else
    log_test_fail "3.14" "Helm chart deploy failed"
fi

# ---------------------------------------------------------------------------
# 3.15 Verify Helm release
# ---------------------------------------------------------------------------
log_test_start "3.15" "Verify Helm release"
if helm list -n "$TEST_NS" --kube-context "$PRIMARY_CONTEXT" 2>/dev/null | grep -q "podinfo"; then
    log_test_pass "3.15" "podinfo Helm release found"
else
    # May not be helm-managed, check for pods instead
    if kubectl --context "$PRIMARY_CONTEXT" get pods -n "$TEST_NS" -l app.kubernetes.io/name=podinfo 2>/dev/null | grep -q "Running"; then
        log_test_pass "3.15" "podinfo pods running (Helm release verified via k8s)"
    else
        log_test_fail "3.15" "podinfo not found"
    fi
fi

# ---------------------------------------------------------------------------
# 3.16 Promote deployment
# ---------------------------------------------------------------------------
log_test_start "3.16" "Promote deployment"
if [[ -n "$DEPLOY1_ID" ]]; then
    pepa_api POST "/deployments/${DEPLOY1_ID}/promote" \
        '{"environment":"production"}' \
        "$TMP/3.16_promote.json" "$TMP/3.16_code.txt"
    if assert_http_success "$TMP/3.16_code.txt" "3.16 promote"; then
        log_test_pass "3.16" "Deployment promoted"
    else
        log_test_fail "3.16" "Promote failed"
    fi
else
    log_test_skip "3.16" "No deployment ID"
fi

# ---------------------------------------------------------------------------
# 3.17 Rollback deployment
# ---------------------------------------------------------------------------
log_test_start "3.17" "Rollback deployment"
if [[ -n "$DEPLOY1_ID" ]]; then
    pepa_api POST "/deployments/${DEPLOY1_ID}/rollback" "" \
        "$TMP/3.17_rollback.json" "$TMP/3.17_code.txt"
    if assert_http_success "$TMP/3.17_code.txt" "3.17 rollback"; then
        log_test_pass "3.17" "Deployment rolled back"
    else
        log_test_fail "3.17" "Rollback failed"
    fi
else
    log_test_skip "3.17" "No deployment ID"
fi

# ---------------------------------------------------------------------------
# 3.18 Retry deployment
# ---------------------------------------------------------------------------
log_test_start "3.18" "Retry deployment"
if [[ -n "$DEPLOY1_ID" ]]; then
    pepa_api POST "/deployments/${DEPLOY1_ID}/retry" "" \
        "$TMP/3.18_retry.json" "$TMP/3.18_code.txt"
    if assert_http_success "$TMP/3.18_code.txt" "3.18 retry"; then
        log_test_pass "3.18" "Deployment retried"
    else
        log_test_fail "3.18" "Retry failed"
    fi
else
    log_test_skip "3.18" "No deployment ID"
fi

# ---------------------------------------------------------------------------
# 3.19 Cancel deployment
# ---------------------------------------------------------------------------
log_test_start "3.19" "Cancel deployment"
# Create a new deployment to cancel
pepa_api POST "/deployments" \
    "{
        \"name\": \"nginx-cancel-test\",
        \"namespace\": \"${TEST_NS}\",
        \"cluster_id\": \"${CLUSTER_ID}\",
        \"containers\": [{\"name\": \"nginx\", \"image\": \"nginx:latest\", \"ports\": [{\"containerPort\": 80}]}]
    }" \
    "$TMP/3.19_cancel_create.json" "$TMP/3.19_cancel_create_code.txt"
CANCEL_ID=$(jq -r '.id // empty' "$TMP/3.19_cancel_create.json" 2>/dev/null)
if [[ -n "$CANCEL_ID" ]]; then
    DEPLOY_IDS+=("$CANCEL_ID")
    pepa_api POST "/deployments/${CANCEL_ID}/cancel" "" \
        "$TMP/3.19_cancel.json" "$TMP/3.19_code.txt"
    if assert_http_success "$TMP/3.19_code.txt" "3.19 cancel"; then
        log_test_pass "3.19" "Deployment cancelled"
    else
        log_test_fail "3.19" "Cancel failed"
    fi
else
    log_test_skip "3.19" "Could not create deployment to cancel"
fi

# ---------------------------------------------------------------------------
# 3.20 Delete deployment
# ---------------------------------------------------------------------------
log_test_start "3.20" "Delete deployment"
if [[ -n "$DEPLOY1_ID" ]]; then
    pepa_api DELETE "/deployments/${DEPLOY1_ID}" "" "$TMP/3.20_del.json" "$TMP/3.20_code.txt"
    if assert_http_success "$TMP/3.20_code.txt" "3.20 delete"; then
        DEPLOY_IDS=("${DEPLOY_IDS[@]/$DEPLOY1_ID/}")
        log_test_pass "3.20" "Deployment deleted"
    else
        log_test_fail "3.20" "Delete failed"
    fi
else
    log_test_skip "3.20" "No deployment ID"
fi

# ---------------------------------------------------------------------------
# 3.21 Verify cleanup
# ---------------------------------------------------------------------------
log_test_start "3.21" "Verify cleanup"
sleep 5
remaining=$(kubectl --context "$PRIMARY_CONTEXT" get deploy nginx-test -n "$TEST_NS" 2>/dev/null | grep -c "nginx-test" || echo "0")
if [[ "$remaining" == "0" ]]; then
    log_test_pass "3.21" "No test deployments remain"
else
    log_test_fail "3.21" "nginx-test still exists in cluster"
fi

# ---------------------------------------------------------------------------
# 3.22 Deploy without containers rejected
# ---------------------------------------------------------------------------
log_test_start "3.22" "Deploy without containers rejected"
pepa_api POST "/deployments" \
    '{"name":"empty-deploy","namespace":"'"${TEST_NS}"'","containers":[]}' \
    "$TMP/3.22_empty.json" "$TMP/3.22_code.txt"
code=$(cat "$TMP/3.22_code.txt" 2>/dev/null)
if [[ "$code" == "400" || "$code" == "422" ]]; then
    log_test_pass "3.22" "Empty containers correctly rejected (HTTP $code)"
else
    log_test_fail "3.22" "Expected 400/422, got HTTP $code"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
trap - EXIT
print_summary "Phase 3: Direct k8s and Helm Deploy"
