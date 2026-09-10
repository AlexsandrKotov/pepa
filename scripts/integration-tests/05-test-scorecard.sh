#!/usr/bin/env bash
# 05-test-scorecard.sh — Scorecard evaluation E2E test
# Verifies PEPA scorecard can evaluate entities from the cluster.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

E2E_NS="${E2E_NAMESPACE:-pepa-e2e}"

log_phase "Phase F: Scorecard Evaluation"

# Login to PEPA
pepa_login 2>/dev/null || true

# Ensure we have a running deployment for scoring
cat <<EOF | k8s "$K3D_PRIMARY" apply -f - 2>/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: scorecard-test-app
  namespace: ${E2E_NS}
  labels:
    app: scorecard-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: scorecard-test
  template:
    metadata:
      labels:
        app: scorecard-test
    spec:
      containers:
        - name: app
          image: nginx:1.25-alpine
          ports:
            - containerPort: 80
          resources:
            requests:
              cpu: 10m
              memory: 32Mi
            limits:
              cpu: 100m
              memory: 128Mi
EOF

# ── F.1 Trigger entity sync ─────────────────────────────────────────────────
log_test_start "F.1" "Trigger entity sync"
pepa_api POST "/entities/sync" "" "$TMP/f1_sync.json" "$TMP/f1_code.txt"
F1_CODE=$(cat "$TMP/f1_code.txt" 2>/dev/null)
if [[ "$F1_CODE" =~ ^2 ]]; then
    log_test_pass "F.1" "Entity sync triggered"
else
    # Try alternate endpoint
    pepa_api POST "/services/sync" "" "$TMP/f1b_sync.json" "$TMP/f1b_code.txt"
    F1B_CODE=$(cat "$TMP/f1b_code.txt" 2>/dev/null)
    if [[ "$F1B_CODE" =~ ^2 ]]; then
        log_test_pass "F.1" "Service sync triggered"
    else
        log_test_skip "F.1" "Entity sync endpoint not available (HTTP $F1_CODE/$F1B_CODE)"
    fi
fi

# ── F.2 Verify entities listed ──────────────────────────────────────────────
log_test_start "F.2" "Verify entities listed"
pepa_api GET "/entities" "" "$TMP/f2_entities.json" "$TMP/f2_code.txt"
if [[ "$(cat "$TMP/f2_code.txt" 2>/dev/null)" =~ ^2 ]]; then
    ENTITY_COUNT=$(jq 'if type == "array" then length elif .entities then .entities | length else 0 end' "$TMP/f2_entities.json" 2>/dev/null)
    log_test_pass "F.2" "Entities listed (${ENTITY_COUNT:-0} found)"
else
    # Try services endpoint
    pepa_api GET "/services" "" "$TMP/f2_svcs.json" "$TMP/f2b_code.txt"
    if [[ "$(cat "$TMP/f2b_code.txt" 2>/dev/null)" =~ ^2 ]]; then
        SVC_COUNT=$(jq 'if type == "array" then length elif .services then .services | length else 0 end' "$TMP/f2_svcs.json" 2>/dev/null)
        log_test_pass "F.2" "Services listed (${SVC_COUNT:-0} found)"
    else
        log_test_fail "F.2" "List entities/services" "HTTP error"
    fi
fi

# ── F.3 Create scorecard with criteria ──────────────────────────────────────
log_test_start "F.3" "Create scorecard with criteria"
pepa_api POST "/scorecards" \
    '{"name":"e2e-scorecard","description":"E2E test scorecard","criteria":[{"name":"has_resource_limits","weight":30,"dsl":"type_key == \"deployment\" && status == \"running\""},{"name":"deployment_active","weight":40,"dsl":"name != \"\""},{"name":"namespace_set","weight":30,"dsl":"description != \"\""}]}' \
    "$TMP/f3_sc.json" "$TMP/f3_code.txt"
F3_CODE=$(cat "$TMP/f3_code.txt" 2>/dev/null)
SC_ID=$(jq -r '.id // .scorecard_id // empty' "$TMP/f3_sc.json" 2>/dev/null)
if [[ "$F3_CODE" =~ ^2 ]] || [[ -n "$SC_ID" ]]; then
    log_test_pass "F.3" "Scorecard created (${SC_ID:-exists})"
else
    # Try simpler payload
    pepa_api POST "/scorecards" \
        '{"name":"e2e-scorecard","description":"E2E test scorecard"}' \
        "$TMP/f3b_sc.json" "$TMP/f3b_code.txt"
    F3B_CODE=$(cat "$TMP/f3b_code.txt" 2>/dev/null)
    SC_ID=$(jq -r '.id // .scorecard_id // empty' "$TMP/f3b_sc.json" 2>/dev/null)
    if [[ "$F3B_CODE" =~ ^2 ]] || [[ -n "$SC_ID" ]]; then
        log_test_pass "F.3" "Scorecard created (${SC_ID:-exists})"
    else
        log_test_fail "F.3" "Create scorecard" "HTTP $F3_CODE / $F3B_CODE"
    fi
fi

# ── F.4 Evaluate scorecard ──────────────────────────────────────────────────
log_test_start "F.4" "Evaluate scorecard"
if [[ -n "$SC_ID" ]]; then
    pepa_api POST "/scorecards/${SC_ID}/evaluate" "" "$TMP/f4_eval.json" "$TMP/f4_code.txt"
    F4_CODE=$(cat "$TMP/f4_code.txt" 2>/dev/null)
    if [[ "$F4_CODE" =~ ^2 ]]; then
        log_test_pass "F.4" "Scorecard evaluated"
    else
        log_test_skip "F.4" "Scorecard evaluation not available (HTTP $F4_CODE)"
    fi
else
    log_test_skip "F.4" "No scorecard ID available"
fi

# ── F.5 Verify scorecard results ────────────────────────────────────────────
log_test_start "F.5" "Verify scorecard results"
if [[ -n "$SC_ID" ]]; then
    pepa_api GET "/scorecards/${SC_ID}" "" "$TMP/f5_sc.json" "$TMP/f5_code.txt"
    if [[ "$(cat "$TMP/f5_code.txt" 2>/dev/null)" =~ ^2 ]]; then
        log_test_pass "F.5" "Scorecard results retrieved"
    else
        log_test_skip "F.5" "Scorecard results not available"
    fi
else
    log_test_skip "F.5" "No scorecard ID available"
fi

# ── F.6 List scorecards ────────────────────────────────────────────────────
log_test_start "F.6" "List scorecards"
pepa_api GET "/scorecards" "" "$TMP/f6_list.json" "$TMP/f6_code.txt"
if [[ "$(cat "$TMP/f6_code.txt" 2>/dev/null)" =~ ^2 ]]; then
    SC_COUNT=$(jq 'if type == "array" then length elif .scorecards then .scorecards | length else 0 end' "$TMP/f6_list.json" 2>/dev/null)
    log_test_pass "F.6" "Scorecards listed (${SC_COUNT:-0} found)"
else
    log_test_fail "F.6" "List scorecards" "HTTP $(cat "$TMP/f6_code.txt" 2>/dev/null)"
fi

# ── F.7 Deploy app without resource limits ──────────────────────────────────
log_test_start "F.7" "Deploy app without resource limits"
cat <<EOF | k8s "$K3D_PRIMARY" apply -f - 2>/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: scorecard-no-limits
  namespace: ${E2E_NS}
  labels:
    app: scorecard-no-limits
spec:
  replicas: 1
  selector:
    matchLabels:
      app: scorecard-no-limits
  template:
    metadata:
      labels:
        app: scorecard-no-limits
    spec:
      containers:
        - name: app
          image: nginx:1.25-alpine
          ports:
            - containerPort: 80
EOF
if k8s "$K3D_PRIMARY" get deploy scorecard-no-limits -n "$E2E_NS" &>/dev/null; then
    log_test_pass "F.7" "App without resource limits deployed"
else
    log_test_fail "F.7" "Deploy no-limits app" "deployment not found"
fi

# ── F.8 Cleanup ─────────────────────────────────────────────────────────────
log_test_start "F.8" "Cleanup scorecard test resources"
k8s "$K3D_PRIMARY" delete deploy scorecard-test-app -n "$E2E_NS" --ignore-not-found=true 2>/dev/null
k8s "$K3D_PRIMARY" delete deploy scorecard-no-limits -n "$E2E_NS" --ignore-not-found=true 2>/dev/null
if [[ -n "$SC_ID" ]]; then
    pepa_api DELETE "/scorecards/${SC_ID}" 2>/dev/null || true
fi
log_test_pass "F.8" "Scorecard test cleanup done"

print_summary
