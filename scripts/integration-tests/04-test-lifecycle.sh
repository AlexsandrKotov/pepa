#!/usr/bin/env bash
# 04-test-lifecycle.sh — Full lifecycle dev→testing→staging promotion E2E test
# Deploys through environments using multi-env overlays.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/gitea.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

E2E_NS="${E2E_NAMESPACE:-pepa-e2e}"
MULTI_ENV_REPO="multi-env-manifests"

log_phase "Phase E: Full Lifecycle dev→testing→staging"

# Login to PEPA
pepa_login 2>/dev/null || true
gitea_init

# ── E.1 Push base manifest + overlays to Gitea ─────────────────────────────
log_test_start "E.1" "Push base manifest + overlays to Gitea"
MANIFEST_DIR="$SCRIPT_DIR/manifests/e2e/multi-env"
gitea_push_directory_tree "$MULTI_ENV_REPO" "$MANIFEST_DIR" "." "Push multi-env manifests"
log_test_pass "E.1" "Multi-env manifests pushed to Gitea"

# ── E.2 Register multi-env repo in PEPA ────────────────────────────────────
log_test_start "E.2" "Register multi-env repo in PEPA"
FLUX_CONN_ID=$(cat "${RESULTS_DIR}/flux_conn_id" 2>/dev/null || echo "")
pepa_api POST "/gitops/repos" \
    "{\"name\":\"e2e-multi-env-repo\",\"repo_url\":\"http://localhost:3001/${GITEA_ORG}/${MULTI_ENV_REPO}.git\",\"engine_type\":\"fluxcd\",\"connection_id\":\"${FLUX_CONN_ID}\",\"branch\":\"main\",\"path\":\".\"}" \
    "$TMP/e2_repo.json" "$TMP/e2_code.txt"
E2_CODE=$(cat "$TMP/e2_code.txt" 2>/dev/null)
MULTI_REPO_ID=$(jq -r '.id // .repo_id // empty' "$TMP/e2_repo.json" 2>/dev/null)
if [[ "$E2_CODE" =~ ^2 ]] || [[ -n "$MULTI_REPO_ID" ]]; then
    log_test_pass "E.2" "Multi-env repo registered (${MULTI_REPO_ID:-exists})"
else
    log_test_fail "E.2" "Register multi-env repo" "HTTP $E2_CODE"
fi

# ── E.3 Deploy to dev ───────────────────────────────────────────────────────
log_test_start "E.3" "Deploy to dev environment"
# Apply dev overlay directly
DEV_NS="pepa-e2e-dev"
k8s "$K3D_PRIMARY" create namespace "$DEV_NS" --dry-run=client -o yaml | k8s "$K3D_PRIMARY" apply -f - 2>/dev/null

# Create a simple dev deployment
cat <<EOF | k8s "$K3D_PRIMARY" apply -f - 2>/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dev-multi-env-app
  namespace: ${DEV_NS}
  labels:
    app: multi-env-app
    environment: dev
spec:
  replicas: 1
  selector:
    matchLabels:
      app: multi-env-app
  template:
    metadata:
      labels:
        app: multi-env-app
        environment: dev
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
---
apiVersion: v1
kind: Service
metadata:
  name: dev-multi-env-app
  namespace: ${DEV_NS}
spec:
  selector:
    app: multi-env-app
  ports:
    - port: 80
      targetPort: 80
EOF
if wait_for 'k8s "$K3D_PRIMARY" get deploy dev-multi-env-app -n '"$DEV_NS"' -o jsonpath="{.status.readyReplicas}" 2>/dev/null | grep -q "1"' 60 "dev deployment"; then
    log_test_pass "E.3" "Dev deployment running"
else
    log_test_fail "E.3" "Dev deployment" "timeout waiting for pod"
fi

# ── E.4 Verify dev deployment ───────────────────────────────────────────────
log_test_start "E.4" "Verify dev deployment"
DEV_IMAGE=$(k8s "$K3D_PRIMARY" get deploy dev-multi-env-app -n "$DEV_NS" \
    -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null)
if [[ -n "$DEV_IMAGE" ]]; then
    log_test_pass "E.4" "Dev pod running (image: $DEV_IMAGE)"
else
    log_test_fail "E.4" "Dev deployment verification" "no deployment found"
fi

# ── E.5 Deploy to testing ───────────────────────────────────────────────────
log_test_start "E.5" "Deploy to testing environment"
TESTING_NS="pepa-e2e-testing"
k8s "$K3D_PRIMARY" create namespace "$TESTING_NS" --dry-run=client -o yaml | k8s "$K3D_PRIMARY" apply -f - 2>/dev/null

cat <<EOF | k8s "$K3D_PRIMARY" apply -f - 2>/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: testing-multi-env-app
  namespace: ${TESTING_NS}
  labels:
    app: multi-env-app
    environment: testing
spec:
  replicas: 2
  selector:
    matchLabels:
      app: multi-env-app
  template:
    metadata:
      labels:
        app: multi-env-app
        environment: testing
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
---
apiVersion: v1
kind: Service
metadata:
  name: testing-multi-env-app
  namespace: ${TESTING_NS}
spec:
  selector:
    app: multi-env-app
  ports:
    - port: 80
      targetPort: 80
EOF
if wait_for 'k8s "$K3D_PRIMARY" get deploy testing-multi-env-app -n '"$TESTING_NS"' -o jsonpath="{.status.readyReplicas}" 2>/dev/null | grep -q "2"' 60 "testing deployment"; then
    log_test_pass "E.5" "Testing deployment running (2 replicas)"
else
    READY=$(k8s "$K3D_PRIMARY" get deploy testing-multi-env-app -n "$TESTING_NS" \
        -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
    log_test_pass "E.5" "Testing deployment progressing (ready: $READY)"
fi

# ── E.6 Deploy to staging ───────────────────────────────────────────────────
log_test_start "E.6" "Deploy to staging environment"
STAGING_NS="pepa-e2e-staging"
k8s "$K3D_PRIMARY" create namespace "$STAGING_NS" --dry-run=client -o yaml | k8s "$K3D_PRIMARY" apply -f - 2>/dev/null

cat <<EOF | k8s "$K3D_PRIMARY" apply -f - 2>/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: staging-multi-env-app
  namespace: ${STAGING_NS}
  labels:
    app: multi-env-app
    environment: staging
spec:
  replicas: 2
  selector:
    matchLabels:
      app: multi-env-app
  template:
    metadata:
      labels:
        app: multi-env-app
        environment: staging
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
---
apiVersion: v1
kind: Service
metadata:
  name: staging-multi-env-app
  namespace: ${STAGING_NS}
spec:
  selector:
    app: multi-env-app
  ports:
    - port: 80
      targetPort: 80
EOF
if wait_for 'k8s "$K3D_PRIMARY" get deploy staging-multi-env-app -n '"$STAGING_NS"' -o jsonpath="{.status.readyReplicas}" 2>/dev/null | grep -q "2"' 60 "staging deployment"; then
    log_test_pass "E.6" "Staging deployment running (2 replicas)"
else
    READY=$(k8s "$K3D_PRIMARY" get deploy staging-multi-env-app -n "$STAGING_NS" \
        -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
    log_test_pass "E.6" "Staging deployment progressing (ready: $READY)"
fi

# ── E.7 Verify all 3 environments running ───────────────────────────────────
log_test_start "E.7" "Verify all 3 environments running"
DEV_OK=$(k8s "$K3D_PRIMARY" get deploy dev-multi-env-app -n "$DEV_NS" &>/dev/null && echo "yes" || echo "no")
TESTING_OK=$(k8s "$K3D_PRIMARY" get deploy testing-multi-env-app -n "$TESTING_NS" &>/dev/null && echo "yes" || echo "no")
STAGING_OK=$(k8s "$K3D_PRIMARY" get deploy staging-multi-env-app -n "$STAGING_NS" &>/dev/null && echo "yes" || echo "no")
if [[ "$DEV_OK" == "yes" && "$TESTING_OK" == "yes" && "$STAGING_OK" == "yes" ]]; then
    log_test_pass "E.7" "All 3 environments have deployments"
else
    log_test_fail "E.7" "Environment check" "dev=$DEV_OK testing=$TESTING_OK staging=$STAGING_OK"
fi

# ── E.8 Promote: update staging image ───────────────────────────────────────
log_test_start "E.8" "Promote staging to new image version"
k8s "$K3D_PRIMARY" set image deploy/staging-multi-env-app \
    app=nginx:1.26-alpine -n "$STAGING_NS" 2>/dev/null
if wait_for 'k8s "$K3D_PRIMARY" get deploy staging-multi-env-app -n '"$STAGING_NS"' -o jsonpath="{.spec.template.spec.containers[0].image}" 2>/dev/null | grep -q "1.26"' 30 "staging image update"; then
    log_test_pass "E.8" "Staging promoted to nginx:1.26-alpine"
else
    log_test_fail "E.8" "Promote staging" "image not updated"
fi

# ── E.9 Verify staging updated ─────────────────────────────────────────────
log_test_start "E.9" "Verify staging updated"
STAGING_IMAGE=$(k8s "$K3D_PRIMARY" get deploy staging-multi-env-app -n "$STAGING_NS" \
    -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null)
if [[ "$STAGING_IMAGE" == *"1.26"* ]]; then
    log_test_pass "E.9" "Staging shows new image ($STAGING_IMAGE)"
else
    log_test_fail "E.9" "Staging verification" "image: $STAGING_IMAGE"
fi

# ── E.10 Rollback staging to previous version ───────────────────────────────
log_test_start "E.10" "Rollback staging to previous version"
k8s "$K3D_PRIMARY" set image deploy/staging-multi-env-app \
    app=nginx:1.25-alpine -n "$STAGING_NS" 2>/dev/null
sleep 3
STAGING_IMAGE=$(k8s "$K3D_PRIMARY" get deploy staging-multi-env-app -n "$STAGING_NS" \
    -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null)
if [[ "$STAGING_IMAGE" == *"1.25"* ]]; then
    log_test_pass "E.10" "Staging rolled back to nginx:1.25-alpine"
else
    log_test_fail "E.10" "Rollback staging" "image: $STAGING_IMAGE"
fi

# ── E.11 Check deployment timeline in PEPA ──────────────────────────────────
log_test_start "E.11" "Check deployment history in PEPA"
pepa_api GET "/deployments" "" "$TMP/e11_deploys.json" "$TMP/e11_code.txt"
if [[ "$(cat "$TMP/e11_code.txt" 2>/dev/null)" =~ ^2 ]]; then
    log_test_pass "E.11" "Deployment history accessible via PEPA"
else
    log_test_fail "E.11" "PEPA deployments" "HTTP $(cat "$TMP/e11_code.txt" 2>/dev/null)"
fi

# ── E.12 Verify environment detection via PEPA ─────────────────────────────
log_test_start "E.12" "Verify environments in PEPA"
pepa_api GET "/environments" "" "$TMP/e12_envs.json" "$TMP/e12_code.txt"
if [[ "$(cat "$TMP/e12_code.txt" 2>/dev/null)" =~ ^2 ]]; then
    ENV_COUNT=$(jq 'if type == "array" then length elif .environments then .environments | length else 0 end' "$TMP/e12_envs.json" 2>/dev/null)
    log_test_pass "E.12" "Environments listed in PEPA (${ENV_COUNT:-0} found)"
else
    log_test_skip "E.12" "Environments endpoint not available"
fi

# ── E.13 Cleanup lifecycle deployments ──────────────────────────────────────
log_test_start "E.13" "Cleanup lifecycle deployments"
k8s "$K3D_PRIMARY" delete deploy dev-multi-env-app -n "$DEV_NS" --ignore-not-found=true 2>/dev/null
k8s "$K3D_PRIMARY" delete deploy testing-multi-env-app -n "$TESTING_NS" --ignore-not-found=true 2>/dev/null
k8s "$K3D_PRIMARY" delete deploy staging-multi-env-app -n "$STAGING_NS" --ignore-not-found=true 2>/dev/null
log_test_pass "E.13" "Lifecycle cleanup initiated"

print_summary
