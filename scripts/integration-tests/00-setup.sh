#!/usr/bin/env bash
# 00-setup.sh — Create k3d clusters, install FluxCD/ArgoCD, set up Gitea repos, verify PEPA
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"
source "${SCRIPT_DIR}/lib/gitea.sh"

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
PRIMARY_CLUSTER="${PRIMARY_CLUSTER:-pepa-test-primary}"
SECONDARY_CLUSTER="${SECONDARY_CLUSTER:-pepa-test-secondary}"
PRIMARY_AGENTS="${PRIMARY_AGENTS:-2}"
SECONDARY_AGENTS="${SECONDARY_AGENTS:-1}"
TEST_NAMESPACE="${TEST_NAMESPACE:-pepa-test}"
SKIP_CLUSTER="${SKIP_CLUSTER:-false}"         # Set to "true" to reuse existing clusters
SKIP_FLUXCD="${SKIP_FLUXCD:-false}"
SKIP_ARGOCD="${SKIP_ARGOCD:-false}"
PEPA_URL="${PEPA_URL:-http://localhost:8088}"

log_phase "Phase 0: Infrastructure Setup"

# ---------------------------------------------------------------------------
# Step 1: Check prerequisites
# ---------------------------------------------------------------------------
log_step "Checking prerequisites"
check_prerequisites k3d kubectl helm

# Optional tools (warn but don't fail)
for tool in flux argocd docker jq; do
    if ! command -v "$tool" &>/dev/null; then
        log_warn "Optional tool '$tool' not found — some tests may be skipped"
    fi
done

# ---------------------------------------------------------------------------
# Step 2: Create k3d clusters
# ---------------------------------------------------------------------------
if [[ "$SKIP_CLUSTER" != "true" ]]; then
    log_step "Creating k3d clusters"

    # Primary cluster
    if k3d cluster get "$PRIMARY_CLUSTER" &>/dev/null; then
        log_info "Primary cluster '$PRIMARY_CLUSTER' already exists"
    else
        log_info "Creating primary cluster '$PRIMARY_CLUSTER' (1 server, ${PRIMARY_AGENTS} agents)..."
        k3d cluster create "$PRIMARY_CLUSTER" \
            --servers 1 \
            --agents "$PRIMARY_AGENTS" \
            --port "8080:80@loadbalancer" \
            --k3s-arg "--disable=traefik@server:0" \
            --wait
        log_info "Primary cluster created"
    fi

    # Secondary cluster
    if k3d cluster get "$SECONDARY_CLUSTER" &>/dev/null; then
        log_info "Secondary cluster '$SECONDARY_CLUSTER' already exists"
    else
        log_info "Creating secondary cluster '$SECONDARY_CLUSTER' (1 server, ${SECONDARY_AGENTS} agents)..."
        k3d cluster create "$SECONDARY_CLUSTER" \
            --servers 1 \
            --agents "$SECONDARY_AGENTS" \
            --k3s-arg "--disable=traefik@server:0" \
            --wait
        log_info "Secondary cluster created"
    fi

    # Verify clusters are reachable
    log_step "Verifying cluster connectivity"
    kubectl --context "k3d-${PRIMARY_CLUSTER}" cluster-info --request-timeout=10s
    kubectl --context "k3d-${SECONDARY_CLUSTER}" cluster-info --request-timeout=10s
else
    log_info "SKIP_CLUSTER=true — reusing existing clusters"
fi

# ---------------------------------------------------------------------------
# Step 3: Create test namespace on primary
# ---------------------------------------------------------------------------
log_step "Creating test namespace '$TEST_NAMESPACE' on primary cluster"
kubectl --context "k3d-${PRIMARY_CLUSTER}" create namespace "$TEST_NAMESPACE" --dry-run=client -o yaml | \
    kubectl --context "k3d-${PRIMARY_CLUSTER}" apply -f -

# Also create on secondary for multi-cluster tests
kubectl --context "k3d-${SECONDARY_CLUSTER}" create namespace "$TEST_NAMESPACE" --dry-run=client -o yaml | \
    kubectl --context "k3d-${SECONDARY_CLUSTER}" apply -f -

# ---------------------------------------------------------------------------
# Step 4: Install FluxCD on primary
# ---------------------------------------------------------------------------
if [[ "$SKIP_FLUXCD" != "true" ]] && command -v flux &>/dev/null; then
    log_step "Installing FluxCD on primary cluster"
    export KUBECONFIG="$(k3d kubeconfig write "$PRIMARY_CLUSTER" 2>/dev/null)"
    if flux check --pre 2>/dev/null; then
        log_info "FluxCD prerequisites met"
    fi
    if flux check 2>/dev/null; then
        log_info "FluxCD already installed"
    else
        flux install 2>/dev/null || log_warn "FluxCD install failed (may need manual setup)"
    fi
    unset KUBECONFIG
else
    log_info "Skipping FluxCD install (SKIP_FLUXCD=$SKIP_FLUXCD or flux CLI not found)"
fi

# ---------------------------------------------------------------------------
# Step 5: Install ArgoCD on primary
# ---------------------------------------------------------------------------
if [[ "$SKIP_ARGOCD" != "true" ]] && command -v helm &>/dev/null; then
    log_step "Installing ArgoCD on primary cluster"
    export KUBECONFIG="$(k3d kubeconfig write "$PRIMARY_CLUSTER" 2>/dev/null)"

    if kubectl get namespace argocd &>/dev/null; then
        log_info "ArgoCD namespace already exists"
    else
        kubectl create namespace argocd
        helm repo add argo https://argoproj.github.io/argo-helm 2>/dev/null || true
        helm repo update 2>/dev/null || true
        helm install argocd argo/argo-cd \
            --namespace argocd \
            --set "server.insecure=true" \
            --set "configs.params.server.insecure=true" \
            --wait --timeout 120s 2>/dev/null || log_warn "ArgoCD helm install failed"
    fi
    unset KUBECONFIG
else
    log_info "Skipping ArgoCD install (SKIP_ARGOCD=$SKIP_ARGOCD or helm not found)"
fi

# ---------------------------------------------------------------------------
# Step 6: Initialize Gitea and create test repos
# ---------------------------------------------------------------------------
log_step "Initializing Gitea and creating test repositories"
gitea_init
gitea_ensure_org "$GITEA_ORG"

# Create test repos
gitea_create_repo "fluxcd-manifests" "FluxCD HelmRelease/Kustomization manifests"
gitea_create_repo "argocd-manifests" "ArgoCD Application manifests"
gitea_create_repo "multi-env-manifests" "Multi-environment overlay manifests"

# Push initial manifest structures
MANIFESTS_DIR="${SCRIPT_DIR}/manifests"

if [[ -d "${MANIFESTS_DIR}/fluxcd" ]]; then
    log_info "Pushing FluxCD manifests..."
    gitea_push_manifest "fluxcd-manifests" "${MANIFESTS_DIR}/fluxcd" "Initial FluxCD manifests"
fi

if [[ -d "${MANIFESTS_DIR}/argocd" ]]; then
    log_info "Pushing ArgoCD manifests..."
    gitea_push_manifest "argocd-manifests" "${MANIFESTS_DIR}/argocd" "Initial ArgoCD manifests"
fi

if [[ -d "${MANIFESTS_DIR}/multi-env" ]]; then
    log_info "Pushing multi-env manifests..."
    gitea_push_manifest "multi-env-manifests" "${MANIFESTS_DIR}/multi-env" "Initial multi-env manifests"
fi

# ---------------------------------------------------------------------------
# Step 7: Start Vault dev container (if not running)
# ---------------------------------------------------------------------------
log_step "Checking Vault availability"
if command -v docker &>/dev/null; then
    if docker ps --filter "name=pepa-vault" --format '{{.Names}}' | grep -q "pepa-vault"; then
        log_info "Vault container already running"
    else
        log_info "Starting Vault dev container..."
        docker run -d --name pepa-vault \
            -p 8200:8200 \
            -e VAULT_DEV_ROOT_TOKEN_ID=pepa-vault-token \
            -e VAULT_DEV_LISTEN_ADDRESS=0.0.0.0:8200 \
            hashicorp/vault:latest 2>/dev/null || log_warn "Could not start Vault container"
        sleep 2
        # Pre-seed some test secrets
        if docker exec pepa-vault vault kv put secret/pepa/db-password value=test-db-pass 2>/dev/null; then
            log_info "Pre-seeded Vault secrets"
        fi
    fi
else
    log_warn "Docker not available — skipping Vault setup"
fi

# ---------------------------------------------------------------------------
# Step 8: Verify PEPA API is healthy
# ---------------------------------------------------------------------------
log_step "Verifying PEPA API health"
MAX_RETRIES=30
RETRY_INTERVAL=2
for i in $(seq 1 $MAX_RETRIES); do
    status=$(curl -s -o /dev/null -w "%{http_code}" "${PEPA_URL}/healthz" 2>/dev/null || echo "000")
    if [[ "$status" == "200" ]]; then
        log_info "PEPA API is healthy (attempt $i)"
        break
    fi
    if [[ $i -eq $MAX_RETRIES ]]; then
        log_error "PEPA API not healthy after $MAX_RETRIES attempts (last status: $status)"
        log_info "Is PEPA running? Try: cd pepa && docker compose up -d"
        exit 1
    fi
    log_info "Waiting for PEPA API... (attempt $i/$MAX_RETRIES, status=$status)"
    sleep $RETRY_INTERVAL
done

# ---------------------------------------------------------------------------
# Step 9: Bootstrap PEPA (get admin token)
# ---------------------------------------------------------------------------
log_step "Bootstrapping PEPA"

# Check bootstrap status
bootstrap_resp=$(curl -s "${PEPA_URL}/api/v1/auth/bootstrap/status" 2>/dev/null)
bootstrap_needed=$(echo "$bootstrap_resp" | jq -r '.needs_bootstrap // .needsBootstrap // false' 2>/dev/null)

if [[ "$bootstrap_needed" == "true" ]]; then
    # Get bootstrap token
    bootstrap_token=$(echo "$bootstrap_resp" | jq -r '.token // .bootstrap_token // empty' 2>/dev/null)
    if [[ -n "$bootstrap_token" ]]; then
        log_info "Activating bootstrap..."
        activate_resp=$(curl -s -w "\n%{http_code}" -X POST "${PEPA_URL}/api/v1/auth/bootstrap/activate" \
            -H "Content-Type: application/json" \
            -d "{\"token\":\"${bootstrap_token}\",\"username\":\"admin\",\"password\":\"admin123\",\"email\":\"admin@pepa.local\"}")
        activate_code=$(echo "$activate_resp" | tail -1)
        if [[ "$activate_code" == "200" ]]; then
            log_info "Bootstrap activated successfully"
        else
            log_warn "Bootstrap activation returned $activate_code"
        fi
    else
        log_warn "Bootstrap needed but no token found in response"
    fi
else
    log_info "Bootstrap already completed or not needed"
fi

# Login to get JWT token
log_step "Logging in to PEPA"
pepa_login "admin" "admin123"

if [[ -n "${PEPA_TOKEN:-}" ]]; then
    log_info "PEPA login successful — token acquired"
    echo "$PEPA_TOKEN" > "${RESULTS_DIR}/pepa_token"
else
    log_warn "PEPA login failed — subsequent API tests may fail"
fi

# ---------------------------------------------------------------------------
# Step 10: Export kubeconfig for test scripts
# ---------------------------------------------------------------------------
log_step "Exporting kubeconfigs"
k3d kubeconfig merge "$PRIMARY_CLUSTER" "$SECONDARY_CLUSTER" \
    --kubeconfig-merge-default 2>/dev/null || \
    log_warn "Could not merge kubeconfigs — using default context"

# Save cluster contexts for test scripts
echo "k3d-${PRIMARY_CLUSTER}" > "${RESULTS_DIR}/primary_context"
echo "k3d-${SECONDARY_CLUSTER}" > "${RESULTS_DIR}/secondary_context"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
log_phase "Setup Complete"
log_info "Primary cluster:   k3d-${PRIMARY_CLUSTER}"
log_info "Secondary cluster: k3d-${SECONDARY_CLUSTER}"
log_info "Test namespace:    ${TEST_NAMESPACE}"
log_info "PEPA API:          ${PEPA_URL}"
log_info "Gitea:             ${GITEA_URL}"
log_info "Results dir:       ${RESULTS_DIR}"
log_info ""
log_info "Run tests with: ./run-all.sh"
log_info "Or individual:  ./01-test-bootstrap-auth.sh"
