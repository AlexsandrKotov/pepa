#!/usr/bin/env bash
# 21-cleanup.sh — Destroy clusters, Gitea repos, temp files (Phase 21)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"
source "${SCRIPT_DIR}/lib/gitea.sh"

PRIMARY_CLUSTER="${PRIMARY_CLUSTER:-pepa-test-primary}"
SECONDARY_CLUSTER="${SECONDARY_CLUSTER:-pepa-test-secondary}"
PEPA_URL="${PEPA_URL:-http://localhost:8088}"

log_phase "Phase 21: Cleanup"

[[ -f "${RESULTS_DIR}/pepa_token" ]] && export PEPA_TOKEN=$(cat "${RESULTS_DIR}/pepa_token")

# ---------------------------------------------------------------------------
# Step 1: Clean up PEPA resources via API
# ---------------------------------------------------------------------------
log_step "Cleaning up PEPA resources via API..."

# Delete GitOps repos
pepa_api GET "/gitops/repos" "" "${RESULTS_DIR}/tmp/cleanup_repos.json" /dev/null 2>/dev/null || true
for id in $(jq -r '.[].id // .data[].id // empty' "${RESULTS_DIR}/tmp/cleanup_repos.json" 2>/dev/null); do
    pepa_api DELETE "/gitops/repos/$id" "" /dev/null /dev/null 2>/dev/null || true
done

# Delete connections (and cascading clusters)
pepa_api GET "/connections" "" "${RESULTS_DIR}/tmp/cleanup_conns.json" /dev/null 2>/dev/null || true
for id in $(jq -r '.[] | select(.name | test("test|secondary|argocd|vault|cascade|concurrent|multi")) | .id' "${RESULTS_DIR}/tmp/cleanup_conns.json" 2>/dev/null); do
    pepa_api DELETE "/connections/$id" "" /dev/null /dev/null 2>/dev/null || true
done

# Delete docker services
pepa_api GET "/docker-services" "" "${RESULTS_DIR}/tmp/cleanup_docker.json" /dev/null 2>/dev/null || true
for id in $(jq -r '.[].id // .data[].id // empty' "${RESULTS_DIR}/tmp/cleanup_docker.json" 2>/dev/null); do
    pepa_api DELETE "/docker-services/$id" "" /dev/null /dev/null 2>/dev/null || true
done

# Delete docker hosts
pepa_api GET "/docker-hosts" "" "${RESULTS_DIR}/tmp/cleanup_hosts.json" /dev/null 2>/dev/null || true
for id in $(jq -r '.[] | select(.name | test("test")) | .id' "${RESULTS_DIR}/tmp/cleanup_hosts.json" 2>/dev/null); do
    pepa_api DELETE "/docker-hosts/$id" "" /dev/null /dev/null 2>/dev/null || true
done

# Delete SSH hosts
pepa_api GET "/ssh-hosts" "" "${RESULTS_DIR}/tmp/cleanup_ssh.json" /dev/null 2>/dev/null || true
for id in $(jq -r '.[].id // .data[].id // empty' "${RESULTS_DIR}/tmp/cleanup_ssh.json" 2>/dev/null); do
    pepa_api DELETE "/ssh-hosts/$id" "" /dev/null /dev/null 2>/dev/null || true
done

# Delete notification rules
pepa_api GET "/notifications/rules" "" "${RESULTS_DIR}/tmp/cleanup_notif.json" /dev/null 2>/dev/null || true
for id in $(jq -r '.[].id // .data[].id // empty' "${RESULTS_DIR}/tmp/cleanup_notif.json" 2>/dev/null); do
    pepa_api DELETE "/notifications/rules/$id" "" /dev/null /dev/null 2>/dev/null || true
done

# Delete security targets
pepa_api GET "/security/targets" "" "${RESULTS_DIR}/tmp/cleanup_sec.json" /dev/null 2>/dev/null || true
for id in $(jq -r '.[].id // .data[].id // empty' "${RESULTS_DIR}/tmp/cleanup_sec.json" 2>/dev/null); do
    pepa_api DELETE "/security/targets/$id" "" /dev/null /dev/null 2>/dev/null || true
done

# Delete service blueprints
pepa_api GET "/service-blueprints" "" "${RESULTS_DIR}/tmp/cleanup_bp.json" /dev/null 2>/dev/null || true
for id in $(jq -r '.[].id // .data[].id // empty' "${RESULTS_DIR}/tmp/cleanup_bp.json" 2>/dev/null); do
    pepa_api DELETE "/service-blueprints/$id" "" /dev/null /dev/null 2>/dev/null || true
done

# Delete registry repos
pepa_api GET "/registry-repositories" "" "${RESULTS_DIR}/tmp/cleanup_reg.json" /dev/null 2>/dev/null || true
for id in $(jq -r '.[].id // .data[].id // empty' "${RESULTS_DIR}/tmp/cleanup_reg.json" 2>/dev/null); do
    pepa_api DELETE "/registry-repositories/$id" "" /dev/null /dev/null 2>/dev/null || true
done

# Delete test users
pepa_api GET "/auth/users" "" "${RESULTS_DIR}/tmp/cleanup_users.json" /dev/null 2>/dev/null || true
for id in $(jq -r '.[] | select(.username | test("rbac|testdev")) | .id' "${RESULTS_DIR}/tmp/cleanup_users.json" 2>/dev/null); do
    pepa_api DELETE "/auth/users/$id" "" /dev/null /dev/null 2>/dev/null || true
done

log_info "PEPA resources cleaned up"

# ---------------------------------------------------------------------------
# Step 2: Clean up Gitea repos
# ---------------------------------------------------------------------------
log_step "Cleaning up Gitea repos..."
gitea_init 2>/dev/null || true
gitea_cleanup_repos "fluxcd-manifests" "argocd-manifests" "multi-env-manifests" \
    "layout-monorepo" "layout-base-overlay" "layout-flat" "layout-team" \
    "ansible-playbooks" "terraform-configs" 2>/dev/null || true

# ---------------------------------------------------------------------------
# Step 3: Clean up k8s resources
# ---------------------------------------------------------------------------
log_step "Cleaning up k8s resources..."
PRIMARY_CONTEXT=$(cat "${RESULTS_DIR}/primary_context" 2>/dev/null || echo "k3d-pepa-test-primary")
kubectl --context "$PRIMARY_CONTEXT" delete namespace pepa-test --ignore-not-found 2>/dev/null || true
kubectl --context "$PRIMARY_CONTEXT" delete namespace pepa-prod --ignore-not-found 2>/dev/null || true

# ---------------------------------------------------------------------------
# Step 4: Stop Vault container
# ---------------------------------------------------------------------------
log_step "Stopping Vault container..."
if command -v docker &>/dev/null; then
    docker rm -f pepa-vault 2>/dev/null || true
fi

# ---------------------------------------------------------------------------
# Step 5: Destroy k3d clusters (optional, controlled by DESTROY_CLUSTERS env)
# ---------------------------------------------------------------------------
if [[ "${DESTROY_CLUSTERS:-false}" == "true" ]]; then
    log_step "Destroying k3d clusters..."
    k3d cluster delete "$PRIMARY_CLUSTER" 2>/dev/null || true
    k3d cluster delete "$SECONDARY_CLUSTER" 2>/dev/null || true
    log_info "Clusters destroyed"
else
    log_info "Clusters preserved (set DESTROY_CLUSTERS=true to destroy)"
fi

# ---------------------------------------------------------------------------
# Step 6: Clean up temp files
# ---------------------------------------------------------------------------
log_step "Cleaning up temporary files..."
rm -rf "${RESULTS_DIR}/tmp" 2>/dev/null || true
rm -f "${RESULTS_DIR}/pepa_token" 2>/dev/null || true
rm -f "${RESULTS_DIR}/conn_primary_id" 2>/dev/null || true
rm -f "${RESULTS_DIR}/cluster_primary_id" 2>/dev/null || true
rm -f "${RESULTS_DIR}/primary_context" 2>/dev/null || true
rm -f "${RESULTS_DIR}/secondary_context" 2>/dev/null || true
rm -f "${RESULTS_DIR}/fluxcd_repo_id" 2>/dev/null || true
rm -f "${RESULTS_DIR}/argocd_repo_id" 2>/dev/null || true

log_phase "Cleanup Complete"
log_info "All test resources have been cleaned up"
log_info "Results preserved in: ${RESULTS_DIR}/"
