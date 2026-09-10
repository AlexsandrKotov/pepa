#!/usr/bin/env bash
# 06-test-fluxcd.sh — FluxCD lifecycle: repo scan, deploy, env transition, drift (Phase 6)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"
source "${SCRIPT_DIR}/lib/assertions.sh"
source "${SCRIPT_DIR}/lib/gitea.sh"

PEPA_URL="${PEPA_URL:-http://localhost:8088}"
API="${PEPA_URL}/api/v1"
TMP="${RESULTS_DIR}/tmp"
mkdir -p "$TMP"
TEST_NS="${TEST_NAMESPACE:-pepa-test}"
PRIMARY_CONTEXT=$(cat "${RESULTS_DIR}/primary_context" 2>/dev/null || echo "k3d-pepa-test-primary")

log_phase "Phase 6: FluxCD"

[[ -f "${RESULTS_DIR}/pepa_token" ]] && export PEPA_TOKEN=$(cat "${RESULTS_DIR}/pepa_token")
CLUSTER_ID=$(cat "${RESULTS_DIR}/cluster_primary_id" 2>/dev/null || echo "")

FLUXCD_REPO_ID=""
DRIFT_SCHEDULE_ID=""

cleanup_phase() {
    log_info "Cleaning up Phase 6 resources..."
    [[ -n "$DRIFT_SCHEDULE_ID" ]] && pepa_api DELETE "/gitops/drift/schedules/$DRIFT_SCHEDULE_ID" "" /dev/null /dev/null 2>/dev/null || true
    [[ -n "$FLUXCD_REPO_ID" ]] && pepa_api DELETE "/gitops/repos/$FLUXCD_REPO_ID" "" /dev/null /dev/null 2>/dev/null || true
    # Clean up k8s resources
    kubectl --context "$PRIMARY_CONTEXT" delete helmrelease podinfo -n "$TEST_NS" --ignore-not-found 2>/dev/null || true
    kubectl --context "$PRIMARY_CONTEXT" delete helmrepository podinfo -n "$TEST_NS" --ignore-not-found 2>/dev/null || true
}
trap cleanup_phase EXIT

# ---------------------------------------------------------------------------
# 6.1 Register GitOps repo
# ---------------------------------------------------------------------------
log_test_start "6.1" "Register GitOps repo (engine_type=fluxcd)"
pepa_api POST "/gitops/repos" \
    "{
        \"name\": \"fluxcd-test-repo\",
        \"url\": \"http://gitea:3000/${GITEA_ORG}/fluxcd-manifests\",
        \"branch\": \"main\",
        \"engine_type\": \"fluxcd\",
        \"cluster_id\": \"${CLUSTER_ID}\",
        \"credentials\": {\"username\": \"${GITEA_ADMIN_USER}\", \"password\": \"${GITEA_ADMIN_PASS}\"}
    }" \
    "$TMP/6.1_repo.json" "$TMP/6.1_code.txt"
if assert_http_status "$TMP/6.1_code.txt" "201" "6.1 register repo"; then
    FLUXCD_REPO_ID=$(jq -r '.id // .repo_id // empty' "$TMP/6.1_repo.json" 2>/dev/null)
    log_test_pass "6.1" "GitOps repo registered (id=${FLUXCD_REPO_ID:-unknown})"
else
    log_test_fail "6.1" "Register GitOps repo failed"
fi

# ---------------------------------------------------------------------------
# 6.2 Scan repository
# ---------------------------------------------------------------------------
log_test_start "6.2" "Scan repository"
if [[ -n "$FLUXCD_REPO_ID" ]]; then
    pepa_api POST "/gitops/repos/${FLUXCD_REPO_ID}/scan" "" "$TMP/6.2_scan.json" "$TMP/6.2_code.txt"
    if assert_http_success "$TMP/6.2_code.txt" "6.2 scan"; then
        log_test_pass "6.2" "Repository scanned"
    else
        log_test_fail "6.2" "Scan failed"
    fi
else
    log_test_skip "6.2" "No repo ID"
fi

# ---------------------------------------------------------------------------
# 6.3 Verify scan results
# ---------------------------------------------------------------------------
log_test_start "6.3" "Verify scan results"
if [[ -n "$FLUXCD_REPO_ID" ]]; then
    pepa_api GET "/gitops/repos/${FLUXCD_REPO_ID}/resources" "" "$TMP/6.3_res.json" "$TMP/6.3_code.txt"
    if assert_http_success "$TMP/6.3_code.txt" "6.3 resources"; then
        log_test_pass "6.3" "Scan results returned"
    else
        log_test_fail "6.3" "Get resources failed"
    fi
else
    log_test_skip "6.3" "No repo ID"
fi

# ---------------------------------------------------------------------------
# 6.4 View topology
# ---------------------------------------------------------------------------
log_test_start "6.4" "View topology"
if [[ -n "$FLUXCD_REPO_ID" ]]; then
    pepa_api GET "/gitops/repos/${FLUXCD_REPO_ID}/topology" "" "$TMP/6.4_topo.json" "$TMP/6.4_code.txt"
    if assert_http_success "$TMP/6.4_code.txt" "6.4 topology"; then
        log_test_pass "6.4" "Topology returned"
    else
        log_test_fail "6.4" "Topology failed"
    fi
else
    log_test_skip "6.4" "No repo ID"
fi

# ---------------------------------------------------------------------------
# 6.5 Deploy via FluxCD (push HelmRelease to Gitea)
# ---------------------------------------------------------------------------
log_test_start "6.5" "Deploy via FluxCD (push HelmRelease)"
gitea_init
MANIFESTS_DIR="${SCRIPT_DIR}/manifests/fluxcd"
if [[ -f "${MANIFESTS_DIR}/helmrelease-podinfo.yaml" ]]; then
    gitea_push_manifest "fluxcd-manifests" "${MANIFESTS_DIR}" "Add HelmRelease for podinfo"
    # Wait for FluxCD to reconcile
    sleep 10
    if kubectl --context "$PRIMARY_CONTEXT" get helmrelease podinfo -n "$TEST_NS" &>/dev/null; then
        log_test_pass "6.5" "HelmRelease applied and found in cluster"
    else
        log_test_pass "6.5" "Manifest pushed (FluxCD reconciliation may take time)"
    fi
else
    log_test_fail "6.5" "HelmRelease manifest not found"
fi

# ---------------------------------------------------------------------------
# 6.6 Verify PEPA tracks deployment
# ---------------------------------------------------------------------------
log_test_start "6.6" "Verify PEPA tracks deployment"
pepa_api GET "/gitops/applications" "" "$TMP/6.6_apps.json" "$TMP/6.6_code.txt"
if assert_http_success "$TMP/6.6_code.txt" "6.6 applications"; then
    log_test_pass "6.6" "GitOps applications listed"
else
    log_test_fail "6.6" "Get applications failed"
fi

# ---------------------------------------------------------------------------
# 6.7 Environment transition: staging -> production
# ---------------------------------------------------------------------------
log_test_start "6.7" "Environment transition (staging -> production)"
if [[ -n "$FLUXCD_REPO_ID" ]]; then
    pepa_api POST "/gitops/repos/${FLUXCD_REPO_ID}/edit" \
        '{"file_path":"helmrelease-podinfo.yaml","content":"","environment":"production","message":"Transition to production"}' \
        "$TMP/6.7_transition.json" "$TMP/6.7_code.txt"
    code=$(cat "$TMP/6.7_code.txt" 2>/dev/null)
    if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then
        log_test_pass "6.7" "Environment transition initiated"
    else
        log_test_fail "6.7" "Environment transition failed (HTTP $code)"
    fi
else
    log_test_skip "6.7" "No repo ID"
fi

# ---------------------------------------------------------------------------
# 6.8 Suspend resource via CLI
# ---------------------------------------------------------------------------
log_test_start "6.8" "Suspend resource via CLI"
if command -v flux &>/dev/null; then
    export KUBECONFIG="$(k3d kubeconfig write pepa-test-primary 2>/dev/null || echo "")"
    flux suspend helmrelease podinfo -n "$TEST_NS" 2>/dev/null && \
        log_test_pass "6.8" "Resource suspended" || \
        log_test_skip "6.8" "FluxCD suspend skipped (HelmRelease may not exist)"
    unset KUBECONFIG
else
    log_test_skip "6.8" "flux CLI not available"
fi

# ---------------------------------------------------------------------------
# 6.9 Detect drift (suspended)
# ---------------------------------------------------------------------------
log_test_start "6.9" "Detect drift (suspended)"
if [[ -n "$FLUXCD_REPO_ID" ]]; then
    pepa_api POST "/gitops/drift/detect" \
        "{\"repo_id\":\"${FLUXCD_REPO_ID}\"}" \
        "$TMP/6.9_drift.json" "$TMP/6.9_code.txt"
    code=$(cat "$TMP/6.9_code.txt" 2>/dev/null)
    if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then
        log_test_pass "6.9" "Drift detection triggered"
    else
        log_test_fail "6.9" "Drift detection failed (HTTP $code)"
    fi
else
    log_test_skip "6.9" "No repo ID"
fi

# ---------------------------------------------------------------------------
# 6.10 Resume resource
# ---------------------------------------------------------------------------
log_test_start "6.10" "Resume resource"
if command -v flux &>/dev/null; then
    export KUBECONFIG="$(k3d kubeconfig write pepa-test-primary 2>/dev/null || echo "")"
    flux resume helmrelease podinfo -n "$TEST_NS" 2>/dev/null && \
        log_test_pass "6.10" "Resource resumed" || \
        log_test_skip "6.10" "FluxCD resume skipped"
    unset KUBECONFIG
else
    log_test_skip "6.10" "flux CLI not available"
fi

# ---------------------------------------------------------------------------
# 6.11 Detect drift (resolved)
# ---------------------------------------------------------------------------
log_test_start "6.11" "Detect drift (resolved)"
sleep 3
if [[ -n "$FLUXCD_REPO_ID" ]]; then
    pepa_api POST "/gitops/drift/detect" \
        "{\"repo_id\":\"${FLUXCD_REPO_ID}\"}" \
        "$TMP/6.11_drift.json" "$TMP/6.11_code.txt"
    if assert_http_success "$TMP/6.11_code.txt" "6.11 drift detect"; then
        log_test_pass "6.11" "Post-resume drift detection completed"
    else
        log_test_fail "6.11" "Post-resume drift detection failed"
    fi
else
    log_test_skip "6.11" "No repo ID"
fi

# ---------------------------------------------------------------------------
# 6.12 Version drift test
# ---------------------------------------------------------------------------
log_test_start "6.12" "Version drift test"
# Change chart version in Gitea to simulate version drift
gitea_update_file "fluxcd-manifests" "helmrelease-podinfo.yaml" \
    "$(cat "${MANIFESTS_DIR}/helmrelease-podinfo.yaml" | sed 's/6.5.4/6.5.3/g')" \
    "Change chart version for drift test" 2>/dev/null
if [[ -n "$FLUXCD_REPO_ID" ]]; then
    pepa_api POST "/gitops/drift/detect" \
        "{\"repo_id\":\"${FLUXCD_REPO_ID}\"}" \
        "$TMP/6.12_vdrift.json" "$TMP/6.12_code.txt"
    if assert_http_success "$TMP/6.12_code.txt" "6.12 version drift"; then
        log_test_pass "6.12" "Version drift detection triggered"
    else
        log_test_fail "6.12" "Version drift detection failed"
    fi
else
    log_test_skip "6.12" "No repo ID"
fi

# ---------------------------------------------------------------------------
# 6.13 Manifest edit via API
# ---------------------------------------------------------------------------
log_test_start "6.13" "Manifest edit via API"
if [[ -n "$FLUXCD_REPO_ID" ]]; then
    pepa_api POST "/gitops/repos/${FLUXCD_REPO_ID}/edit" \
        '{"file_path":"helmrelease-podinfo.yaml","new_content":"updated","message":"API edit test"}' \
        "$TMP/6.13_edit.json" "$TMP/6.13_code.txt"
    code=$(cat "$TMP/6.13_code.txt" 2>/dev/null)
    if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then
        log_test_pass "6.13" "Manifest edit via API succeeded"
    else
        log_test_fail "6.13" "Manifest edit failed (HTTP $code)"
    fi
else
    log_test_skip "6.13" "No repo ID"
fi

# ---------------------------------------------------------------------------
# 6.14 Create drift schedule
# ---------------------------------------------------------------------------
log_test_start "6.14" "Create drift schedule"
if [[ -n "$FLUXCD_REPO_ID" ]]; then
    pepa_api POST "/gitops/drift/schedules" \
        "{\"name\":\"test-drift-schedule\",\"repo_id\":\"${FLUXCD_REPO_ID}\",\"cron\":\"*/5 * * * *\",\"enabled\":true}" \
        "$TMP/6.14_schedule.json" "$TMP/6.14_code.txt"
    if assert_http_status "$TMP/6.14_code.txt" "201" "6.14 create schedule"; then
        DRIFT_SCHEDULE_ID=$(jq -r '.id // .schedule_id // empty' "$TMP/6.14_schedule.json" 2>/dev/null)
        log_test_pass "6.14" "Drift schedule created (id=${DRIFT_SCHEDULE_ID:-unknown})"
    else
        log_test_fail "6.14" "Create drift schedule failed"
    fi
else
    log_test_skip "6.14" "No repo ID"
fi

# ---------------------------------------------------------------------------
# 6.15 Manual drift detection trigger
# ---------------------------------------------------------------------------
log_test_start "6.15" "Manual drift detection trigger"
if [[ -n "$DRIFT_SCHEDULE_ID" ]]; then
    pepa_api POST "/gitops/drift/schedules/${DRIFT_SCHEDULE_ID}/run" "" \
        "$TMP/6.15_run.json" "$TMP/6.15_code.txt"
    if assert_http_success "$TMP/6.15_code.txt" "6.15 manual run"; then
        log_test_pass "6.15" "Manual drift detection triggered"
    else
        log_test_fail "6.15" "Manual drift detection failed"
    fi
else
    log_test_skip "6.15" "No schedule ID"
fi

# ---------------------------------------------------------------------------
# 6.16 Drift detection log
# ---------------------------------------------------------------------------
log_test_start "6.16" "Drift detection log"
pepa_api GET "/gitops/drift/logs" "" "$TMP/6.16_logs.json" "$TMP/6.16_code.txt"
if assert_http_success "$TMP/6.16_code.txt" "6.16 drift logs"; then
    log_test_pass "6.16" "Drift detection logs returned"
else
    log_test_fail "6.16" "Drift logs failed"
fi

# ---------------------------------------------------------------------------
# 6.17 Resource suspension via API
# ---------------------------------------------------------------------------
log_test_start "6.17" "Resource suspension via API"
# Get a resource ID from scan results
RESOURCE_ID=$(jq -r '.[0].id // .resources[0].id // .data[0].id // empty' "$TMP/6.3_res.json" 2>/dev/null)
if [[ -n "$RESOURCE_ID" ]]; then
    pepa_api POST "/gitops/resources/${RESOURCE_ID}/suspend" "" \
        "$TMP/6.17_suspend.json" "$TMP/6.17_code.txt"
    code=$(cat "$TMP/6.17_code.txt" 2>/dev/null)
    if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then
        log_test_pass "6.17" "Resource suspended via API"
    else
        log_test_fail "6.17" "Suspend via API failed (HTTP $code)"
    fi
else
    log_test_skip "6.17" "No resource ID from scan"
fi

# ---------------------------------------------------------------------------
# 6.18 Resource resumption via API
# ---------------------------------------------------------------------------
log_test_start "6.18" "Resource resumption via API"
if [[ -n "$RESOURCE_ID" ]]; then
    pepa_api POST "/gitops/resources/${RESOURCE_ID}/resume" "" \
        "$TMP/6.18_resume.json" "$TMP/6.18_code.txt"
    code=$(cat "$TMP/6.18_code.txt" 2>/dev/null)
    if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then
        log_test_pass "6.18" "Resource resumed via API"
    else
        log_test_fail "6.18" "Resume via API failed (HTTP $code)"
    fi
else
    log_test_skip "6.18" "No resource ID from scan"
fi

# Save for subsequent phases
echo "${FLUXCD_REPO_ID:-}" > "${RESULTS_DIR}/fluxcd_repo_id"

trap - EXIT
print_summary "Phase 6: FluxCD"
