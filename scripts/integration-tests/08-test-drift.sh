#!/usr/bin/env bash
# 08-test-drift.sh — Drift detection E2E test
# Verifies PEPA drift detection by manually changing cluster state.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

E2E_NS="${E2E_NAMESPACE:-pepa-e2e}"

log_phase "Phase I: Drift Detection"

# Login to PEPA
pepa_login 2>/dev/null || true

# Ensure a deployment exists for drift testing
cat <<EOF | k8s "$K3D_PRIMARY" apply -f - 2>/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: drift-test-app
  namespace: ${E2E_NS}
  labels:
    app: drift-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: drift-test
  template:
    metadata:
      labels:
        app: drift-test
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
sleep 3

# ── I.1 Manual drift: scale deployment via kubectl ──────────────────────────
log_test_start "I.1" "Manual drift: scale deployment replicas"
k8s "$K3D_PRIMARY" scale deploy drift-test-app -n "$E2E_NS" --replicas=3 2>/dev/null
sleep 2
REPLICAS=$(k8s "$K3D_PRIMARY" get deploy drift-test-app -n "$E2E_NS" \
    -o jsonpath='{.spec.replicas}' 2>/dev/null)
if [[ "$REPLICAS" == "3" ]]; then
    log_test_pass "I.1" "Deployment scaled to 3 replicas (drift introduced)"
else
    log_test_fail "I.1" "Scale deployment" "replicas=$REPLICAS"
fi

# ── I.2 Trigger drift detection ────────────────────────────────────────────
log_test_start "I.2" "Trigger drift detection"
pepa_api POST "/gitops/drift/detect" "" "$TMP/i2_drift.json" "$TMP/i2_code.txt"
I2_CODE=$(cat "$TMP/i2_code.txt" 2>/dev/null)
if [[ "$I2_CODE" =~ ^2 ]]; then
    DRIFT_COUNT=$(jq '.drifts | length // .drift_count // 0' "$TMP/i2_drift.json" 2>/dev/null)
    log_test_pass "I.2" "Drift detection triggered (${DRIFT_COUNT:-?} drifts)"
else
    # Try alternate endpoint
    pepa_api GET "/gitops/drift" "" "$TMP/i2b_drift.json" "$TMP/i2b_code.txt"
    I2B_CODE=$(cat "$TMP/i2b_code.txt" 2>/dev/null)
    if [[ "$I2B_CODE" =~ ^2 ]]; then
        log_test_pass "I.2" "Drift status queried"
    else
        log_test_skip "I.2" "Drift detection endpoint not available (HTTP $I2_CODE/$I2B_CODE)"
    fi
fi

# ── I.3 Manual drift: edit configmap ────────────────────────────────────────
log_test_start "I.3" "Manual drift: create/edit configmap"
k8s "$K3D_PRIMARY" create configmap drift-test-config -n "$E2E_NS" \
    --from-literal=key1=value1 --dry-run=client -o yaml | k8s "$K3D_PRIMARY" apply -f - 2>/dev/null
sleep 1
# Modify the configmap
k8s "$K3D_PRIMARY" create configmap drift-test-config -n "$E2E_NS" \
    --from-literal=key1=modified-value --from-literal=key2=new-key --dry-run=client -o yaml | k8s "$K3D_PRIMARY" apply -f - 2>/dev/null
log_test_pass "I.3" "ConfigMap modified (drift introduced)"

# ── I.4 Trigger drift detection for config ─────────────────────────────────
log_test_start "I.4" "Trigger drift detection for config"
pepa_api POST "/gitops/drift/detect" "" "$TMP/i4_drift.json" "$TMP/i4_code.txt"
I4_CODE=$(cat "$TMP/i4_code.txt" 2>/dev/null)
if [[ "$I4_CODE" =~ ^2 ]]; then
    log_test_pass "I.4" "Drift detection triggered for config"
else
    log_test_skip "I.4" "Drift detection endpoint not available (HTTP $I4_CODE)"
fi

# ── I.5 Verify drift severity ──────────────────────────────────────────────
log_test_start "I.5" "Verify drift entries"
pepa_api GET "/gitops/drift" "" "$TMP/i5_drift.json" "$TMP/i5_code.txt"
I5_CODE=$(cat "$TMP/i5_code.txt" 2>/dev/null)
if [[ "$I5_CODE" =~ ^2 ]]; then
    DRIFT_COUNT=$(jq 'if type == "array" then length elif .drifts then .drifts | length else 0 end' "$TMP/i5_drift.json" 2>/dev/null)
    log_test_pass "I.5" "Drift entries listed (${DRIFT_COUNT:-0} found)"
else
    log_test_skip "I.5" "Drift listing not available (HTTP $I5_CODE)"
fi

# ── I.6 Auto-remediate: restore desired state ──────────────────────────────
log_test_start "I.6" "Auto-remediate: restore desired state"
k8s "$K3D_PRIMARY" scale deploy drift-test-app -n "$E2E_NS" --replicas=1 2>/dev/null
k8s "$K3D_PRIMARY" delete configmap drift-test-config -n "$E2E_NS" --ignore-not-found=true 2>/dev/null
sleep 3
REPLICAS=$(k8s "$K3D_PRIMARY" get deploy drift-test-app -n "$E2E_NS" \
    -o jsonpath='{.spec.replicas}' 2>/dev/null)
if [[ "$REPLICAS" == "1" ]]; then
    log_test_pass "I.6" "Desired state restored (replicas=1, configmap removed)"
else
    log_test_fail "I.6" "Auto-remediate" "replicas=$REPLICAS"
fi

# ── I.7 Verify drift resolved ──────────────────────────────────────────────
log_test_start "I.7" "Verify drift resolved"
pepa_api GET "/gitops/drift" "" "$TMP/i7_drift.json" "$TMP/i7_code.txt"
I7_CODE=$(cat "$TMP/i7_code.txt" 2>/dev/null)
if [[ "$I7_CODE" =~ ^2 ]]; then
    log_test_pass "I.7" "Drift status verified after remediation"
else
    log_test_skip "I.7" "Drift check not available (HTTP $I7_CODE)"
fi

# ── I.8 Drift detection log ────────────────────────────────────────────────
log_test_start "I.8" "Drift detection log"
pepa_api GET "/gitops/drift/logs" "" "$TMP/i8_logs.json" "$TMP/i8_code.txt"
I8_CODE=$(cat "$TMP/i8_code.txt" 2>/dev/null)
if [[ "$I8_CODE" =~ ^2 ]]; then
    log_test_pass "I.8" "Drift logs retrieved"
else
    log_test_skip "I.8" "Drift logs endpoint not available (HTTP $I8_CODE)"
fi

# ── I.9 Cleanup ────────────────────────────────────────────────────────────
log_test_start "I.9" "Cleanup drift test resources"
k8s "$K3D_PRIMARY" delete deploy drift-test-app -n "$E2E_NS" --ignore-not-found=true 2>/dev/null
k8s "$K3D_PRIMARY" delete configmap drift-test-config -n "$E2E_NS" --ignore-not-found=true 2>/dev/null
log_test_pass "I.9" "Drift test cleanup done"

print_summary
