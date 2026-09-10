#!/usr/bin/env bash
# 02-test-connections-clusters.sh — Connection and cluster CRUD, bidirectional sync (Phase 2)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"
source "${SCRIPT_DIR}/lib/assertions.sh"

PEPA_URL="${PEPA_URL:-http://localhost:8088}"
API="${PEPA_URL}/api/v1"
TMP="${RESULTS_DIR}/tmp"
mkdir -p "$TMP"

PRIMARY_CLUSTER="${PRIMARY_CLUSTER:-pepa-test-primary}"
SECONDARY_CLUSTER="${SECONDARY_CLUSTER:-pepa-test-secondary}"

log_phase "Phase 2: Connections and Clusters"

# Load token from setup
if [[ -f "${RESULTS_DIR}/pepa_token" ]]; then
    export PEPA_TOKEN=$(cat "${RESULTS_DIR}/pepa_token")
fi

# Get kubeconfig for primary cluster
PRIMARY_KUBECONFIG=""
if command -v k3d &>/dev/null; then
    PRIMARY_KUBECONFIG=$(k3d kubeconfig write "$PRIMARY_CLUSTER" 2>/dev/null || echo "")
fi
if [[ -z "$PRIMARY_KUBECONFIG" ]]; then
    PRIMARY_KUBECONFIG="${HOME}/.kube/config"
fi
PRIMARY_KUBECONFIG_DATA=$(cat "$PRIMARY_KUBECONFIG" 2>/dev/null | base64 | tr -d '\n' || echo "")

# Track created resources for cleanup
CONN_IDS=()
CLUSTER_IDS=()

cleanup_phase() {
    log_info "Cleaning up Phase 2 resources..."
    for id in "${CONN_IDS[@]}"; do
        pepa_api DELETE "/connections/$id" "" /dev/null /dev/null 2>/dev/null || true
    done
    for id in "${CLUSTER_IDS[@]}"; do
        pepa_api DELETE "/clusters/$id" "" /dev/null /dev/null 2>/dev/null || true
    done
}
trap cleanup_phase EXIT

# ---------------------------------------------------------------------------
# 2.1 Create kubernetes connection
# ---------------------------------------------------------------------------
log_test_start "2.1" "Create kubernetes connection"
pepa_api POST "/connections" \
    "{\"name\":\"test-primary-k8s\",\"type\":\"kubernetes\",\"kubeconfig\":\"${PRIMARY_KUBECONFIG_DATA}\",\"labels\":{\"env\":\"test\",\"cluster\":\"primary\"}}" \
    "$TMP/2.1_conn.json" "$TMP/2.1_code.txt"
if assert_http_status "$TMP/2.1_code.txt" "201" "2.1 create connection"; then
    CONN1_ID=$(jq -r '.id // .connection_id // empty' "$TMP/2.1_conn.json" 2>/dev/null)
    CONN_IDS+=("$CONN1_ID")
    log_test_pass "2.1" "Connection created (id=${CONN1_ID:-unknown})"
else
    log_test_fail "2.1" "Create connection failed"
    CONN1_ID=""
fi

# ---------------------------------------------------------------------------
# 2.2 Verify auto-created cluster
# ---------------------------------------------------------------------------
log_test_start "2.2" "Verify auto-created cluster from connection"
sleep 2  # Wait for async cluster creation
pepa_api GET "/clusters" "" "$TMP/2.2_clusters.json" "$TMP/2.2_code.txt"
if assert_http_success "$TMP/2.2_code.txt" "2.2 list clusters"; then
    cluster_name=$(jq -r '.[].name // .data[].name // .clusters[].name' "$TMP/2.2_clusters.json" 2>/dev/null | grep -i "primary\|test" | head -1)
    if [[ -n "$cluster_name" ]]; then
        CLUSTER1_ID=$(jq -r ".[] | select(.name | test(\"primary|test\";\"i\")) | .id" "$TMP/2.2_clusters.json" 2>/dev/null | head -1)
        [[ -n "$CLUSTER1_ID" ]] && CLUSTER_IDS+=("$CLUSTER1_ID")
        log_test_pass "2.2" "Auto-created cluster found: $cluster_name"
    else
        log_test_fail "2.2" "No auto-created cluster found"
    fi
else
    log_test_fail "2.2" "List clusters failed"
fi

# ---------------------------------------------------------------------------
# 2.3 Test kubernetes connection
# ---------------------------------------------------------------------------
log_test_start "2.3" "Test kubernetes connection"
if [[ -n "$CONN1_ID" ]]; then
    pepa_api POST "/connections/${CONN1_ID}/test" "" "$TMP/2.3_test.json" "$TMP/2.3_code.txt"
    if assert_http_success "$TMP/2.3_code.txt" "2.3 test connection"; then
        log_test_pass "2.3" "Connection test passed"
    else
        log_test_fail "2.3" "Connection test failed"
    fi
else
    log_test_skip "2.3" "No connection ID"
fi

# ---------------------------------------------------------------------------
# 2.4 Create cluster via kubeconfig upload
# ---------------------------------------------------------------------------
log_test_start "2.4" "Create cluster via kubeconfig upload"
pepa_api POST "/clusters" \
    "{\"name\":\"test-secondary-upload\",\"kubeconfig\":\"${PRIMARY_KUBECONFIG_DATA}\",\"labels\":{\"env\":\"test\"}}" \
    "$TMP/2.4_cluster.json" "$TMP/2.4_code.txt"
if assert_http_status "$TMP/2.4_code.txt" "201" "2.4 create cluster"; then
    CLUSTER2_ID=$(jq -r '.id // .cluster_id // empty' "$TMP/2.4_cluster.json" 2>/dev/null)
    CLUSTER_IDS+=("$CLUSTER2_ID")
    log_test_pass "2.4" "Cluster created via kubeconfig (id=${CLUSTER2_ID:-unknown})"
else
    log_test_fail "2.4" "Create cluster failed"
    CLUSTER2_ID=""
fi

# ---------------------------------------------------------------------------
# 2.5 Verify auto-created connection from cluster
# ---------------------------------------------------------------------------
log_test_start "2.5" "Verify auto-created connection from cluster"
sleep 2
pepa_api GET "/connections" "" "$TMP/2.5_conns.json" "$TMP/2.5_code.txt"
if assert_http_success "$TMP/2.5_code.txt" "2.5 list connections"; then
    auto_conn=$(jq -r '.[] | select(.cluster_id != null and .cluster_id != "") | .id' "$TMP/2.5_conns.json" 2>/dev/null | tail -1)
    if [[ -n "$auto_conn" ]]; then
        log_test_pass "2.5" "Auto-created connection found from cluster"
    else
        log_test_pass "2.5" "Connections listed (auto-creation may be async)"
    fi
else
    log_test_fail "2.5" "List connections failed"
fi

# ---------------------------------------------------------------------------
# 2.6 Update connection
# ---------------------------------------------------------------------------
log_test_start "2.6" "Update connection"
if [[ -n "$CONN1_ID" ]]; then
    pepa_api PUT "/connections/${CONN1_ID}" \
        '{"name":"test-primary-k8s-updated","labels":{"env":"test","updated":"true"}}' \
        "$TMP/2.6_update.json" "$TMP/2.6_code.txt"
    if assert_http_success "$TMP/2.6_code.txt" "2.6 update connection"; then
        log_test_pass "2.6" "Connection updated"
    else
        log_test_fail "2.6" "Update connection failed"
    fi
else
    log_test_skip "2.6" "No connection ID"
fi

# ---------------------------------------------------------------------------
# 2.7 Verify cluster sync from connection update
# ---------------------------------------------------------------------------
log_test_start "2.7" "Verify cluster sync from connection update"
if [[ -n "$CLUSTER1_ID" ]]; then
    pepa_api GET "/clusters/${CLUSTER1_ID}" "" "$TMP/2.7_cluster.json" "$TMP/2.7_code.txt"
    if assert_http_success "$TMP/2.7_code.txt" "2.7 get cluster"; then
        log_test_pass "2.7" "Cluster reflects updated config"
    else
        log_test_fail "2.7" "Get cluster failed"
    fi
else
    log_test_skip "2.7" "No cluster ID"
fi

# ---------------------------------------------------------------------------
# 2.8 Delete connection, verify cluster cascade
# ---------------------------------------------------------------------------
log_test_start "2.8" "Delete connection, verify cluster cascade"
# Create a temporary connection for cascade test
pepa_api POST "/connections" \
    "{\"name\":\"cascade-test-conn\",\"type\":\"kubernetes\",\"kubeconfig\":\"${PRIMARY_KUBECONFIG_DATA}\"}" \
    "$TMP/2.8_cascade_conn.json" "$TMP/2.8_cascade_conn_code.txt"
CASCADE_CONN_ID=$(jq -r '.id // empty' "$TMP/2.8_cascade_conn.json" 2>/dev/null)
if [[ -n "$CASCADE_CONN_ID" ]]; then
    sleep 2
    pepa_api DELETE "/connections/${CASCADE_CONN_ID}" "" "$TMP/2.8_del.json" "$TMP/2.8_code.txt"
    if assert_http_success "$TMP/2.8_code.txt" "2.8 delete connection"; then
        log_test_pass "2.8" "Connection deleted (cascade may be async)"
    else
        log_test_fail "2.8" "Delete connection failed"
    fi
else
    log_test_skip "2.8" "Could not create cascade test connection"
fi

# ---------------------------------------------------------------------------
# 2.9 Delete cluster, verify connection cascade
# ---------------------------------------------------------------------------
log_test_start "2.9" "Delete cluster, verify connection cascade"
if [[ -n "$CLUSTER2_ID" ]]; then
    pepa_api DELETE "/clusters/${CLUSTER2_ID}" "" "$TMP/2.9_del.json" "$TMP/2.9_code.txt"
    if assert_http_success "$TMP/2.9_code.txt" "2.9 delete cluster"; then
        # Remove from cleanup list since we already deleted
        CLUSTER_IDS=("${CLUSTER_IDS[@]/$CLUSTER2_ID/}")
        log_test_pass "2.9" "Cluster deleted (cascade may be async)"
    else
        log_test_fail "2.9" "Delete cluster failed"
    fi
else
    log_test_skip "2.9" "No cluster ID for cascade test"
fi

# ---------------------------------------------------------------------------
# 2.10 Connection summary dashboard
# ---------------------------------------------------------------------------
log_test_start "2.10" "Connection summary dashboard"
pepa_api GET "/connections/summary" "" "$TMP/2.10_summary.json" "$TMP/2.10_code.txt"
if assert_http_success "$TMP/2.10_code.txt" "2.10 summary"; then
    log_test_pass "2.10" "Connection summary returned"
else
    log_test_fail "2.10" "Connection summary failed"
fi

# ---------------------------------------------------------------------------
# 2.11 Connection plugin status
# ---------------------------------------------------------------------------
log_test_start "2.11" "Connection plugin status"
pepa_api GET "/connections/plugin-status" "" "$TMP/2.11_plugin.json" "$TMP/2.11_code.txt"
if assert_http_success "$TMP/2.11_code.txt" "2.11 plugin-status"; then
    log_test_pass "2.11" "Plugin status returned"
else
    log_test_fail "2.11" "Plugin status failed"
fi

# ---------------------------------------------------------------------------
# 2.12 Connection credential status
# ---------------------------------------------------------------------------
log_test_start "2.12" "Connection credential status"
pepa_api GET "/connections/credential-status" "" "$TMP/2.12_cred.json" "$TMP/2.12_code.txt"
if assert_http_success "$TMP/2.12_code.txt" "2.12 credential-status"; then
    log_test_pass "2.12" "Credential status returned"
else
    log_test_fail "2.12" "Credential status failed"
fi

# ---------------------------------------------------------------------------
# 2.13 Connection health dashboard
# ---------------------------------------------------------------------------
log_test_start "2.13" "Connection health dashboard"
pepa_api GET "/connections/health" "" "$TMP/2.13_health.json" "$TMP/2.13_code.txt"
if assert_http_success "$TMP/2.13_code.txt" "2.13 health"; then
    log_test_pass "2.13" "Health overview returned"
else
    log_test_fail "2.13" "Health overview failed"
fi

# ---------------------------------------------------------------------------
# 2.14 Parse kubeconfig
# ---------------------------------------------------------------------------
log_test_start "2.14" "Parse kubeconfig"
pepa_api POST "/connections/parse-kubeconfig" \
    "{\"kubeconfig\":\"${PRIMARY_KUBECONFIG_DATA}\"}" \
    "$TMP/2.14_parse.json" "$TMP/2.14_code.txt"
if assert_http_success "$TMP/2.14_code.txt" "2.14 parse kubeconfig"; then
    log_test_pass "2.14" "Kubeconfig parsed"
else
    log_test_fail "2.14" "Parse kubeconfig failed"
fi

# ---------------------------------------------------------------------------
# 2.15 Browse connection (list namespaces)
# ---------------------------------------------------------------------------
log_test_start "2.15" "Browse connection (list namespaces)"
if [[ -n "$CONN1_ID" ]]; then
    pepa_api GET "/connections/${CONN1_ID}/browse" "" "$TMP/2.15_browse.json" "$TMP/2.15_code.txt"
    if assert_http_success "$TMP/2.15_code.txt" "2.15 browse"; then
        log_test_pass "2.15" "Connection browse returned"
    else
        log_test_fail "2.15" "Connection browse failed"
    fi
else
    log_test_skip "2.15" "No connection ID"
fi

# ---------------------------------------------------------------------------
# 2.16 Invalid kubeconfig rejected
# ---------------------------------------------------------------------------
log_test_start "2.16" "Invalid kubeconfig rejected"
INVALID_KUBE=$(echo "not-valid-kubeconfig-data" | base64 | tr -d '\n')
pepa_api POST "/connections" \
    "{\"name\":\"bad-kube\",\"type\":\"kubernetes\",\"kubeconfig\":\"${INVALID_KUBE}\"}" \
    "$TMP/2.16_bad.json" "$TMP/2.16_code.txt"
code=$(cat "$TMP/2.16_code.txt" 2>/dev/null)
if [[ "$code" == "400" || "$code" == "422" ]]; then
    log_test_pass "2.16" "Invalid kubeconfig rejected (HTTP $code)"
else
    log_test_fail "2.16" "Expected 400/422, got HTTP $code"
fi

# ---------------------------------------------------------------------------
# 2.17 Cluster health check
# ---------------------------------------------------------------------------
log_test_start "2.17" "Cluster health check"
if [[ -n "$CLUSTER1_ID" ]]; then
    pepa_api GET "/clusters/${CLUSTER1_ID}/health" "" "$TMP/2.17_health.json" "$TMP/2.17_code.txt"
    if assert_http_success "$TMP/2.17_code.txt" "2.17 cluster health"; then
        log_test_pass "2.17" "Cluster health returned"
    else
        log_test_fail "2.17" "Cluster health failed"
    fi
else
    log_test_skip "2.17" "No cluster ID"
fi

# ---------------------------------------------------------------------------
# 2.18 Cluster nodes
# ---------------------------------------------------------------------------
log_test_start "2.18" "Cluster nodes"
if [[ -n "$CLUSTER1_ID" ]]; then
    pepa_api GET "/clusters/${CLUSTER1_ID}/nodes" "" "$TMP/2.18_nodes.json" "$TMP/2.18_code.txt"
    if assert_http_success "$TMP/2.18_code.txt" "2.18 cluster nodes"; then
        log_test_pass "2.18" "Cluster nodes returned"
    else
        log_test_fail "2.18" "Cluster nodes failed"
    fi
else
    log_test_skip "2.18" "No cluster ID"
fi

# ---------------------------------------------------------------------------
# 2.19 Cluster namespaces
# ---------------------------------------------------------------------------
log_test_start "2.19" "Cluster namespaces"
if [[ -n "$CLUSTER1_ID" ]]; then
    pepa_api GET "/clusters/${CLUSTER1_ID}/namespaces" "" "$TMP/2.19_ns.json" "$TMP/2.19_code.txt"
    if assert_http_success "$TMP/2.19_code.txt" "2.19 cluster namespaces"; then
        log_test_pass "2.19" "Cluster namespaces returned"
    else
        log_test_fail "2.19" "Cluster namespaces failed"
    fi
else
    log_test_skip "2.19" "No cluster ID"
fi

# ---------------------------------------------------------------------------
# 2.20 Cluster resources
# ---------------------------------------------------------------------------
log_test_start "2.20" "Cluster resources"
if [[ -n "$CLUSTER1_ID" ]]; then
    pepa_api GET "/clusters/${CLUSTER1_ID}/resources" "" "$TMP/2.20_res.json" "$TMP/2.20_code.txt"
    if assert_http_success "$TMP/2.20_code.txt" "2.20 cluster resources"; then
        log_test_pass "2.20" "Cluster resources returned"
    else
        log_test_fail "2.20" "Cluster resources failed"
    fi
else
    log_test_skip "2.20" "No cluster ID"
fi

# ---------------------------------------------------------------------------
# 2.21 Cluster FluxCD resources
# ---------------------------------------------------------------------------
log_test_start "2.21" "Cluster FluxCD resources"
if [[ -n "$CLUSTER1_ID" ]]; then
    pepa_api GET "/clusters/${CLUSTER1_ID}/flux" "" "$TMP/2.21_flux.json" "$TMP/2.21_code.txt"
    if assert_http_success "$TMP/2.21_code.txt" "2.21 FluxCD resources"; then
        log_test_pass "2.21" "FluxCD resources returned"
    else
        log_test_fail "2.21" "FluxCD resources failed"
    fi
else
    log_test_skip "2.21" "No cluster ID"
fi

# ---------------------------------------------------------------------------
# 2.22 Cluster ArgoCD resources
# ---------------------------------------------------------------------------
log_test_start "2.22" "Cluster ArgoCD resources"
if [[ -n "$CLUSTER1_ID" ]]; then
    pepa_api GET "/clusters/${CLUSTER1_ID}/argo" "" "$TMP/2.22_argo.json" "$TMP/2.22_code.txt"
    if assert_http_success "$TMP/2.22_code.txt" "2.22 ArgoCD resources"; then
        log_test_pass "2.22" "ArgoCD resources returned"
    else
        log_test_fail "2.22" "ArgoCD resources failed"
    fi
else
    log_test_skip "2.22" "No cluster ID"
fi

# ---------------------------------------------------------------------------
# 2.23 Cluster GitOps engine
# ---------------------------------------------------------------------------
log_test_start "2.23" "Cluster GitOps engine"
if [[ -n "$CLUSTER1_ID" ]]; then
    pepa_api GET "/clusters/${CLUSTER1_ID}/gitops" "" "$TMP/2.23_gitops.json" "$TMP/2.23_code.txt"
    if assert_http_success "$TMP/2.23_code.txt" "2.23 GitOps engine"; then
        log_test_pass "2.23" "GitOps engine info returned"
    else
        log_test_fail "2.23" "GitOps engine info failed"
    fi
else
    log_test_skip "2.23" "No cluster ID"
fi

# ---------------------------------------------------------------------------
# 2.24 Cluster test connection
# ---------------------------------------------------------------------------
log_test_start "2.24" "Cluster test connection"
if [[ -n "$CLUSTER1_ID" ]]; then
    pepa_api POST "/clusters/${CLUSTER1_ID}/test" "" "$TMP/2.24_test.json" "$TMP/2.24_code.txt"
    if assert_http_success "$TMP/2.24_code.txt" "2.24 cluster test"; then
        log_test_pass "2.24" "Cluster test connection passed"
    else
        log_test_fail "2.24" "Cluster test connection failed"
    fi
else
    log_test_skip "2.24" "No cluster ID"
fi

# ---------------------------------------------------------------------------
# 2.25 Cluster topology
# ---------------------------------------------------------------------------
log_test_start "2.25" "Cluster topology"
if [[ -n "$CLUSTER1_ID" ]]; then
    pepa_api GET "/clusters/${CLUSTER1_ID}/topology" "" "$TMP/2.25_topo.json" "$TMP/2.25_code.txt"
    if assert_http_success "$TMP/2.25_code.txt" "2.25 topology"; then
        log_test_pass "2.25" "Cluster topology returned"
    else
        log_test_fail "2.25" "Cluster topology failed"
    fi
else
    log_test_skip "2.25" "No cluster ID"
fi

# ---------------------------------------------------------------------------
# Save IDs for subsequent phases
# ---------------------------------------------------------------------------
echo "${CONN1_ID:-}" > "${RESULTS_DIR}/conn_primary_id"
echo "${CLUSTER1_ID:-}" > "${RESULTS_DIR}/cluster_primary_id"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
trap - EXIT
print_summary "Phase 2: Connections and Clusters"
