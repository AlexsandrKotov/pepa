#!/usr/bin/env bash
# 29-test-gitops-bindings.sh — GitOps bindings CRUD and discovery
# Covers: GET /gitops/bindings, POST /gitops/bindings, GET /gitops/bindings/:id,
#         PUT /gitops/bindings/:id, DELETE /gitops/bindings/:id,
#         POST /gitops/bindings/discover, GET .../by-environment/:envId, .../by-service/:svcId

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 29: GitOps Bindings CRUD & Discovery"

pepa_login 2>/dev/null || true

# ── 29.1 List all bindings ─────────────────────────────────────────────────
log_test_start "29.1" "GET /gitops/bindings returns binding list"
pepa_api GET "/gitops/bindings"
if [[ "$API_STATUS" == "200" ]]; then
    BIND_COUNT=$(echo "$API_RESPONSE" | jq '.bindings // .items // [] | length' 2>/dev/null)
    log_test "29.1" "GitOps bindings: $BIND_COUNT found" "pass"
else
    log_test "29.1" "List bindings" "fail" "HTTP $API_STATUS"
    BIND_COUNT=0
fi

# ── 29.2 Get first connection for binding creation ─────────────────────────
log_test_start "29.2" "Get connection for binding test"
pepa_api GET "/connections?connection_type=kubernetes"
if [[ "$API_STATUS" == "200" ]]; then
    CONN_ID=$(echo "$API_RESPONSE" | jq -r '(.connections // .items // [])[0].id // empty' 2>/dev/null)
    if [[ -n "$CONN_ID" ]]; then
        log_test "29.2" "Kubernetes connection: ${CONN_ID:0:8}..." "pass"
    else
        log_test "29.2" "Get connection" "fail" "no kubernetes connections"
    fi
else
    log_test "29.2" "Get connection" "fail" "HTTP $API_STATUS"
    CONN_ID=""
fi

# ── 29.3 Create a binding ──────────────────────────────────────────────────
if [[ -n "${CONN_ID:-}" ]]; then
    log_test_start "29.3" "POST /gitops/bindings creates a new binding"
    BIND_BODY="{
        \"name\": \"test-binding-integration\",
        \"argo_connection_id\": \"$CONN_ID\",
        \"app_namespace\": \"pepa-e2e\",
        \"app_name\": \"test-binding-app\",
        \"engine_type\": \"argocd\"
    }"
    pepa_api POST "/gitops/bindings" "$BIND_BODY"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        BIND_ID=$(echo "$API_RESPONSE" | jq -r '.id // .binding.id // empty')
        log_test "29.3" "Binding created: ${BIND_ID:0:8}..." "pass"
    else
        log_test "29.3" "Create binding" "fail" "HTTP $API_STATUS"
        BIND_ID=""
    fi
else
    log_test "29.3" "Create binding" "skip" "no connection"
    BIND_ID=""
fi

# ── 29.4 Get binding by ID ─────────────────────────────────────────────────
if [[ -n "${BIND_ID:-}" ]]; then
    log_test_start "29.4" "GET /gitops/bindings/:id returns binding details"
    pepa_api GET "/gitops/bindings/$BIND_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        B_NS=$(echo "$API_RESPONSE" | jq -r '.app_namespace // empty')
        B_APP=$(echo "$API_RESPONSE" | jq -r '.app_name // empty')
        log_test "29.4" "Binding: ns=$B_NS, app=$B_APP" "pass"
    else
        log_test "29.4" "Get binding" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "29.4" "Get binding" "skip" "no binding"
fi

# ── 29.5 Update binding ────────────────────────────────────────────────────
if [[ -n "${BIND_ID:-}" ]]; then
    log_test_start "29.5" "PUT /gitops/bindings/:id updates binding"
    pepa_api PUT "/gitops/bindings/$BIND_ID" '{"app_namespace": "pepa-e2e-updated"}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "29.5" "Binding updated" "pass"
    else
        log_test "29.5" "Update binding" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "29.5" "Update binding" "skip" "no binding"
fi

# ── 29.6 Query bindings by environment ─────────────────────────────────────
log_test_start "29.6" "GET /gitops/bindings/by-environment/:envId"
# Get dev environment ID
pepa_api GET "/environments"
DEV_ENV=$(echo "$API_RESPONSE" | jq -r '(.environments // .items // [])[] | select(.slug=="dev") | .id' 2>/dev/null | head -1)
if [[ -n "${DEV_ENV:-}" ]]; then
    pepa_api GET "/gitops/bindings/by-environment/$DEV_ENV"
    if [[ "$API_STATUS" == "200" ]]; then
        log_test "29.6" "Bindings by environment endpoint accessible" "pass"
    else
        log_test "29.6" "Bindings by environment" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "29.6" "Bindings by environment" "skip" "no dev environment"
fi

# ── 29.7 Query bindings by service ─────────────────────────────────────────
log_test_start "29.7" "GET /gitops/bindings/by-service/:serviceId"
pepa_api GET "/gitops/bindings/by-service/00000000-0000-0000-0000-000000000099"
if [[ "$API_STATUS" == "200" ]]; then
    log_test "29.7" "Bindings by service endpoint accessible" "pass"
else
    log_test "29.7" "Bindings by service" "fail" "HTTP $API_STATUS"
fi

# ── 29.8 Discover bindings ─────────────────────────────────────────────────
if [[ -n "${CONN_ID:-}" ]]; then
    log_test_start "29.8" "POST /gitops/bindings/discover triggers auto-discovery"
    pepa_api POST "/gitops/bindings/discover" "{\"argo_connection_id\": \"$CONN_ID\"}"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        DISC_COUNT=$(echo "$API_RESPONSE" | jq '.discovered // .items // [] | length' 2>/dev/null)
        log_test "29.8" "Discovery: $DISC_COUNT apps found" "pass"
    else
        log_test "29.8" "Discover bindings" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "29.8" "Discover bindings" "skip" "no connection"
fi

# ── 29.9 Delete binding ────────────────────────────────────────────────────
if [[ -n "${BIND_ID:-}" ]]; then
    log_test_start "29.9" "DELETE /gitops/bindings/:id"
    pepa_api DELETE "/gitops/bindings/$BIND_ID"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "29.9" "Binding deleted" "pass"
    else
        log_test "29.9" "Delete binding" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "29.9" "Delete binding" "skip" "no binding"
fi

# ── 29.10 Verify deletion (404) ────────────────────────────────────────────
if [[ -n "${BIND_ID:-}" ]]; then
    log_test_start "29.10" "GET /gitops/bindings/:id returns 404 after deletion"
    pepa_api GET "/gitops/bindings/$BIND_ID"
    if [[ "$API_STATUS" == "404" ]]; then
        log_test "29.10" "Deleted binding returns 404" "pass"
    else
        log_test "29.10" "Deleted binding" "fail" "expected 404, got $API_STATUS"
    fi
else
    log_test "29.10" "Verify deletion" "skip" "no binding"
fi

print_summary
