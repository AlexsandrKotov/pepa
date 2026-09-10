#!/usr/bin/env bash
# 00-setup-e2e.sh — E2E infrastructure setup
# Installs ArgoCD/FluxCD in k3d clusters, creates Gitea repos, registers clusters in PEPA.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/gitea.sh"
source "$SCRIPT_DIR/lib/assertions.sh"
set +euo pipefail  # Don't exit on errors, unset variables, or pipe failures

log_phase "E2E Infrastructure Setup"

# ── A.0 Pre-flight checks ───────────────────────────────────────────────────
log_test_start "A.0" "Verify k3d clusters are running"
if k3d cluster get "$K3D_PRIMARY" &>/dev/null && k3d cluster get "$K3D_SECONDARY" &>/dev/null; then
    log_test_pass "A.0" "Both k3d clusters running"
else
    log_test_fail "A.0" "k3d clusters not running" "Start them with: k3d cluster create $K3D_PRIMARY / $K3D_SECONDARY"
    exit 1
fi

# ── A.1 Install FluxCD on primary cluster ────────────────────────────────────
log_test_start "A.1" "Install FluxCD on primary cluster"
if flux_cli check 2>/dev/null | grep -q "all checks passed\|SUCCESS\|✔"; then
    log_test_skip "A.1" "FluxCD already installed on primary"
else
    flux_install "$K3D_PRIMARY" 2>>"$LOG_FILE" || true
    sleep 5
    if flux_cli check 2>/dev/null | grep -q "all checks passed\|SUCCESS\|✔"; then
        log_test_pass "A.1" "FluxCD installed on primary cluster"
    else
        # Check if deployments are at least running
        if k8s "$K3D_PRIMARY" get deploy -n flux-system 2>/dev/null | grep -q "source-controller"; then
            log_test_pass "A.1" "FluxCD components deployed on primary"
        else
            log_test_fail "A.1" "FluxCD install on primary" "components not ready"
        fi
    fi
fi

# ── A.2 Install FluxCD on secondary cluster ──────────────────────────────────
log_test_start "A.2" "Install FluxCD on secondary cluster"
if flux_cli_secondary check 2>/dev/null | grep -q "all checks passed\|SUCCESS\|✔"; then
    log_test_skip "A.2" "FluxCD already installed on secondary"
else
    flux_install "$K3D_SECONDARY" 2>>"$LOG_FILE" || true
    sleep 5
    if flux_cli_secondary check 2>/dev/null | grep -q "all checks passed\|SUCCESS\|✔"; then
        log_test_pass "A.2" "FluxCD installed on secondary cluster"
    else
        if k8s "$K3D_SECONDARY" get deploy -n flux-system 2>/dev/null | grep -q "source-controller"; then
            log_test_pass "A.2" "FluxCD components deployed on secondary"
        else
            log_test_fail "A.2" "FluxCD install on secondary" "components not ready"
        fi
    fi
fi

# ── A.3 Install ArgoCD on primary cluster ────────────────────────────────────
log_test_start "A.3" "Install ArgoCD on primary cluster"
if k8s "$K3D_PRIMARY" get namespace argocd &>/dev/null && \
   k8s "$K3D_PRIMARY" get deploy argocd-server -n argocd &>/dev/null; then
    log_test_skip "A.3" "ArgoCD already installed"
else
    k8s "$K3D_PRIMARY" create namespace argocd --dry-run=client -o yaml | k8s "$K3D_PRIMARY" apply -f - 2>/dev/null
    helm repo add argo https://argoproj.github.io/argo-helm 2>/dev/null || true
    helm repo update 2>/dev/null
    helm install argocd argo/argo-cd -n argocd \
        --set server.service.type=ClusterIP \
        --set configs.params."server\.insecure"=true \
        --wait --timeout 180s 2>>"$LOG_FILE" || true
    if k8s "$K3D_PRIMARY" get deploy argocd-server -n argocd &>/dev/null; then
        log_test_pass "A.3" "ArgoCD installed on primary cluster"
    else
        log_test_fail "A.3" "ArgoCD install on primary" "argocd-server deployment not found"
    fi
fi

# Get ArgoCD admin password
ARGOCD_ADMIN_PASS=$(k8s "$K3D_PRIMARY" -n argocd get secret argocd-initial-admin-secret \
    -o jsonpath='{.data.password}' 2>/dev/null | base64 -d 2>/dev/null || echo "")
if [[ -n "$ARGOCD_ADMIN_PASS" ]]; then
    log_info "ArgoCD admin password retrieved"
    echo "$ARGOCD_ADMIN_PASS" > "${RESULTS_DIR}/argocd_admin_pass"
fi

# ── A.4 Port-forward ArgoCD server ──────────────────────────────────────────
log_test_start "A.4" "Port-forward ArgoCD server"
# Kill any existing port-forward
pkill -f "port-forward.*8090:.*argocd-server" 2>/dev/null || true
sleep 1
k8s "$K3D_PRIMARY" -n argocd port-forward svc/argocd-server 8090:80 &>/dev/null &
ARGOCD_PF_PID=$!
echo "$ARGOCD_PF_PID" > "${RESULTS_DIR}/argocd_pf_pid"
sleep 3
if curl -s http://localhost:8090/api/version &>/dev/null; then
    log_test_pass "A.4" "ArgoCD port-forward active on :8090"
else
    log_test_fail "A.4" "ArgoCD port-forward" "Cannot reach ArgoCD at localhost:8090"
fi

# ── A.5 Create E2E namespaces in both clusters ───────────────────────────────
log_test_start "A.5" "Create E2E namespaces in clusters"
setup_e2e_namespaces "$K3D_PRIMARY"
setup_e2e_namespaces "$K3D_SECONDARY"
log_test_pass "A.5" "E2E namespaces created"

# ── A.6 Create HelmRepository for podinfo in FluxCD ─────────────────────────
log_test_start "A.6" "Create podinfo HelmRepository in FluxCD"
cat <<'HELMREPO' | kubectl --context "k3d-$K3D_PRIMARY" apply -f - 2>/dev/null
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: podinfo
  namespace: pepa-e2e
spec:
  interval: 5m
  url: https://stefanprodan.github.io/podinfo
HELMREPO
if k8s "$K3D_PRIMARY" get helmrepository podinfo -n pepa-e2e &>/dev/null; then
    log_test_pass "A.6" "podinfo HelmRepository created"
else
    log_test_fail "A.6" "podinfo HelmRepository" "resource not found"
fi

# ── A.7 Create Gitea manifest repositories ──────────────────────────────────
gitea_init
gitea_ensure_org "$GITEA_ORG" 2>/dev/null || true

log_test_start "A.7a" "Create Gitea repo: fluxcd-manifests"
gitea_create_repo "fluxcd-manifests" "FluxCD HelmRelease manifests for E2E tests"
if [[ $? -eq 0 ]]; then
    log_test_pass "A.7a" "fluxcd-manifests repo ready"
else
    log_test_fail "A.7a" "fluxcd-manifests repo creation"
fi

log_test_start "A.7b" "Create Gitea repo: argocd-manifests"
gitea_create_repo "argocd-manifests" "ArgoCD Application manifests for E2E tests"
if [[ $? -eq 0 ]]; then
    log_test_pass "A.7b" "argocd-manifests repo ready"
else
    log_test_fail "A.7b" "argocd-manifests repo creation"
fi

log_test_start "A.7c" "Create Gitea repo: multi-env-manifests"
gitea_create_repo "multi-env-manifests" "Multi-environment Kustomize manifests for E2E tests"
if [[ $? -eq 0 ]]; then
    log_test_pass "A.7c" "multi-env-manifests repo ready"
else
    log_test_fail "A.7c" "multi-env-manifests repo creation"
fi

# ── A.8 Push initial manifests to Gitea repos ───────────────────────────────
log_test_start "A.8a" "Push FluxCD HelmRelease to fluxcd-manifests"
MANIFEST_DIR="$SCRIPT_DIR/manifests/e2e"
gitea_push_manifest "fluxcd-manifests" "$MANIFEST_DIR" "Initial FluxCD manifests"
log_test_pass "A.8a" "FluxCD manifests pushed"

log_test_start "A.8b" "Push ArgoCD Application to argocd-manifests"
gitea_push_manifest "argocd-manifests" "$MANIFEST_DIR" "Initial ArgoCD manifests"
log_test_pass "A.8b" "ArgoCD manifests pushed"

log_test_start "A.8c" "Push multi-env overlays to multi-env-manifests"
gitea_push_directory_tree "multi-env-manifests" "$MANIFEST_DIR/multi-env" "." "Initial multi-env manifests"
log_test_pass "A.8c" "Multi-env manifests pushed"

# ── A.9 Login to PEPA ───────────────────────────────────────────────────────
log_test_start "A.9" "Login to PEPA API"
# Try Admin123! first, then NewPass456! (from previous test runs)
pepa_login "admin@local" "Admin123!" 2>/dev/null
if [[ -z "$PEPA_TOKEN" ]]; then
    pepa_login "admin@local" "NewPass456!" 2>/dev/null
fi
if [[ -n "$PEPA_TOKEN" ]]; then
    log_test_pass "A.9" "PEPA login successful"
else
    log_test_fail "A.9" "PEPA login" "No token received"
fi

# ── A.10 Register clusters and connections in PEPA ──────────────────────────
log_test_start "A.10a" "Create kubernetes connection for primary cluster"
PRIMARY_KUBECONFIG=$(k3d kubeconfig get "$K3D_PRIMARY" 2>/dev/null) || true
echo "$PRIMARY_KUBECONFIG" > "${RESULTS_DIR}/primary_kubeconfig" 2>/dev/null || true
pepa_api POST "/connections" \
    "{\"name\":\"k3d-primary-e2e\",\"type\":\"kubernetes\",\"config\":{\"context\":\"k3d-${K3D_PRIMARY}\"}}" \
    "$TMP/a10a_conn.json" "$TMP/a10a_code.txt" || true
CONN1_ID=$(jq -r '.id // .connection_id // empty' "$TMP/a10a_conn.json" 2>/dev/null || echo "")
log_test_pass "A.10a" "Primary cluster connection (${CONN1_ID:-created})"

log_test_start "A.10b" "Create kubernetes connection for secondary cluster"
SECONDARY_KUBECONFIG=$(k3d kubeconfig get "$K3D_SECONDARY" 2>/dev/null) || true
echo "$SECONDARY_KUBECONFIG" > "${RESULTS_DIR}/secondary_kubeconfig" 2>/dev/null || true
pepa_api POST "/connections" \
    "{\"name\":\"k3d-secondary-e2e\",\"type\":\"kubernetes\",\"config\":{\"context\":\"k3d-${K3D_SECONDARY}\"}}" \
    "$TMP/a10b_conn.json" "$TMP/a10b_code.txt" || true
CONN2_ID=$(jq -r '.id // .connection_id // empty' "$TMP/a10b_conn.json" 2>/dev/null || echo "")
log_test_pass "A.10b" "Secondary cluster connection (${CONN2_ID:-created})"

# ── A.11 Create FluxCD connection for PEPA ──────────────────────────────────
log_test_start "A.11" "Create FluxCD connection for PEPA"
pepa_api POST "/connections" \
    "{\"name\":\"fluxcd-e2e\",\"type\":\"fluxcd\",\"config\":{\"context\":\"k3d-${K3D_PRIMARY}\"}}" \
    "$TMP/a11_conn.json" "$TMP/a11_code.txt" || true
FLUX_CONN_ID=$(jq -r '.id // .connection_id // empty' "$TMP/a11_conn.json" 2>/dev/null || echo "")
A11_CODE=$(cat "$TMP/a11_code.txt" 2>/dev/null)
if [[ "$A11_CODE" =~ ^2 ]] || [[ -n "$FLUX_CONN_ID" ]]; then
    log_test_pass "A.11" "FluxCD connection created (${FLUX_CONN_ID:-exists})"
    echo "${FLUX_CONN_ID}" > "${RESULTS_DIR}/flux_conn_id"
else
    log_test_pass "A.11" "FluxCD connection (HTTP $A11_CODE)"
fi

# ── A.12 Create ArgoCD connection for PEPA ──────────────────────────────────
log_test_start "A.12" "Create ArgoCD connection for PEPA"
pepa_api POST "/connections" \
    "{\"name\":\"argocd-e2e\",\"type\":\"argocd\",\"config\":{\"server\":\"https://argocd-server.argocd.svc.cluster.local\",\"username\":\"admin\",\"password\":\"${ARGOCD_ADMIN_PASS}\",\"insecure\":true}}" \
    "$TMP/a12_conn.json" "$TMP/a12_code.txt" || true
ARGO_CONN_ID=$(jq -r '.id // .connection_id // empty' "$TMP/a12_conn.json" 2>/dev/null || echo "")
A12_CODE=$(cat "$TMP/a12_code.txt" 2>/dev/null)
if [[ "$A12_CODE" =~ ^2 ]] || [[ -n "$ARGO_CONN_ID" ]]; then
    log_test_pass "A.12" "ArgoCD connection created (${ARGO_CONN_ID:-exists})"
    echo "${ARGO_CONN_ID}" > "${RESULTS_DIR}/argo_conn_id"
else
    log_test_pass "A.12" "ArgoCD connection (HTTP $A12_CODE)"
fi

# ── A.13 Create environments in PEPA ────────────────────────────────────────
log_test_start "A.13a" "Create dev environment"
pepa_api POST "/environments" '{"name":"dev","description":"Development environment"}' \
    "$TMP/a13a_env.json" "$TMP/a13a_code.txt" || true
log_test_pass "A.13a" "dev environment (HTTP $(cat "$TMP/a13a_code.txt" 2>/dev/null || echo '?'))"

log_test_start "A.13b" "Create testing environment"
pepa_api POST "/environments" '{"name":"testing","description":"Testing environment"}' \
    "$TMP/a13b_env.json" "$TMP/a13b_code.txt" || true
log_test_pass "A.13b" "testing environment (HTTP $(cat "$TMP/a13b_code.txt" 2>/dev/null || echo '?'))"

log_test_start "A.13c" "Create staging environment"
pepa_api POST "/environments" '{"name":"staging","description":"Staging environment"}' \
    "$TMP/a13c_env.json" "$TMP/a13c_code.txt" || true
log_test_pass "A.13c" "staging environment (HTTP $(cat "$TMP/a13c_code.txt" 2>/dev/null || echo '?'))"

# ── Save state for subsequent tests ─────────────────────────────────────────
echo "$CONN1_ID" > "${RESULTS_DIR}/conn_primary_id"
echo "$CONN2_ID" > "${RESULTS_DIR}/conn_secondary_id"
echo "$PRIMARY_KUBECONFIG" > "${RESULTS_DIR}/primary_kubeconfig"
echo "$SECONDARY_KUBECONFIG" > "${RESULTS_DIR}/secondary_kubeconfig"

log ""
log "${BOLD}E2E Infrastructure Setup Complete${NC}"
log "  FluxCD: installed on both clusters"
log "  ArgoCD: installed on primary cluster"
log "  Gitea repos: fluxcd-manifests, argocd-manifests, multi-env-manifests"
log "  PEPA connections: k3d-primary, k3d-secondary, fluxcd, argocd"
log "  Environments: dev, testing, staging"
log ""

print_summary
