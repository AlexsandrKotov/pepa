#!/usr/bin/env bash
# 09-cleanup-e2e.sh — E2E cleanup script
# Removes all test resources from clusters, Gitea, and PEPA.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/gitea.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase J: E2E Cleanup"

# Login to PEPA
pepa_login 2>/dev/null || true
gitea_init

# ── J.1 Delete all test namespaces in both clusters ────────────────────────
log_test_start "J.1" "Delete test namespaces in primary cluster"
cleanup_e2e_namespaces "$K3D_PRIMARY"
log_test_pass "J.1" "Primary cluster namespaces cleanup"

log_test_start "J.1b" "Delete test namespaces in secondary cluster"
cleanup_e2e_namespaces "$K3D_SECONDARY"
log_test_pass "J.1b" "Secondary cluster namespaces cleanup"

# Also clean up the argocd namespace
k8s "$K3D_PRIMARY" delete namespace argocd --ignore-not-found=true --wait=false 2>/dev/null || true

# ── J.2 Delete Gitea test repos ────────────────────────────────────────────
log_test_start "J.2" "Delete Gitea test repos"
for repo in fluxcd-manifests argocd-manifests multi-env-manifests; do
    gitea_delete_repo "$repo" 2>/dev/null || true
done
log_test_pass "J.2" "Gitea test repos deleted"

# ── J.3 Delete PEPA GitOps repos ───────────────────────────────────────────
log_test_start "J.3" "Delete PEPA GitOps repos"
pepa_api GET "/gitops/repos" "" "$TMP/j3_repos.json" "$TMP/j3_code.txt" 2>/dev/null
if [[ "$(cat "$TMP/j3_code.txt" 2>/dev/null)" =~ ^2 ]]; then
    REPO_IDS=$(jq -r '.[].id // .repos[].id // empty' "$TMP/j3_repos.json" 2>/dev/null)
    for rid in $REPO_IDS; do
        pepa_api DELETE "/gitops/repos/${rid}" 2>/dev/null || true
    done
fi
log_test_pass "J.3" "PEPA GitOps repos cleanup"

# ── J.4 Delete PEPA scorecards ─────────────────────────────────────────────
log_test_start "J.4" "Delete PEPA scorecards"
pepa_api GET "/scorecards" "" "$TMP/j4_sc.json" "$TMP/j4_code.txt" 2>/dev/null
if [[ "$(cat "$TMP/j4_code.txt" 2>/dev/null)" =~ ^2 ]]; then
    SC_IDS=$(jq -r '.[].id // .scorecards[].id // empty' "$TMP/j4_sc.json" 2>/dev/null)
    for sid in $SC_IDS; do
        pepa_api DELETE "/scorecards/${sid}" 2>/dev/null || true
    done
fi
log_test_pass "J.4" "PEPA scorecards cleanup"

# ── J.5 Kill port-forwards ─────────────────────────────────────────────────
log_test_start "J.5" "Kill port-forwards"
pkill -f "port-forward.*8090.*argocd" 2>/dev/null || true
log_test_pass "J.5" "Port-forwards killed"

# ── J.6 Optionally destroy k3d clusters ────────────────────────────────────
log_test_start "J.6" "Optionally destroy k3d clusters"
if [[ "${E2E_DESTROY_CLUSTERS:-false}" == "true" ]]; then
    k3d cluster delete "$K3D_PRIMARY" 2>/dev/null || true
    k3d cluster delete "$K3D_SECONDARY" 2>/dev/null || true
    log_test_pass "J.6" "k3d clusters destroyed"
else
    log_test_skip "J.6" "Cluster destruction skipped (set E2E_DESTROY_CLUSTERS=true to enable)"
fi

log ""
log "${BOLD}E2E Cleanup Complete${NC}"
log ""

print_summary
