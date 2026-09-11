#!/usr/bin/env bash
# 26-test-environment-deployments.sh — Environment detail + compare tests
# Covers: GET /environments, GET /environments/:id/contents, GET /environments/compare

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 26: Environment Deployments & Compare"

pepa_login 2>/dev/null || true

# ── 26.1 List all environments ─────────────────────────────────────────────
log_test_start "26.1" "GET /environments returns environment list"
pepa_api GET "/environments"
if [[ "$API_STATUS" == "200" ]]; then
    ENV_COUNT=$(echo "$API_RESPONSE" | jq '.environments // .items // [] | length' 2>/dev/null)
    log_test "26.1" "Environments: $ENV_COUNT found" "pass"
    # Save environment IDs for later tests
    DEV_ENV_ID=$(echo "$API_RESPONSE" | jq -r '(.environments // .items // [])[] | select(.slug=="dev") | .id' 2>/dev/null | head -1)
    PROD_ENV_ID=$(echo "$API_RESPONSE" | jq -r '(.environments // .items // [])[] | select(.slug=="production") | .id' 2>/dev/null | head -1)
else
    log_test "26.1" "List environments" "fail" "HTTP $API_STATUS"
fi

# ── 26.2 Create deployments linked to environments ─────────────────────────
log_test_start "26.2" "Create deployments in dev and production stages"
ENV_DEPLOY_IDS=()
for stage in dev production; do
    pepa_api POST "/gitops/deploy" "{
        \"image_tag\": \"v1.0.0-env-$stage\",
        \"image_repository\": \"env-app\",
        \"namespace\": \"app-$stage\",
        \"team\": \"platform-team\",
        \"stage\": \"$stage\"
    }"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        EID=$(echo "$API_RESPONSE" | jq -r '.deployment.id // .id // empty')
        ENV_DEPLOY_IDS+=("$EID")
    fi
done
log_test "26.2" "Created ${#ENV_DEPLOY_IDS[@]} environment-linked deployments" "pass"

# ── 26.3 Get dev environment contents ──────────────────────────────────────
if [[ -n "${DEV_ENV_ID:-}" ]]; then
    log_test_start "26.3" "GET /environments/:id/contents shows dev deployments"
    pepa_api GET "/environments/$DEV_ENV_ID/contents"
    if [[ "$API_STATUS" == "200" ]]; then
        DEP_COUNT=$(echo "$API_RESPONSE" | jq '.deployments // [] | length' 2>/dev/null)
        SVC_COUNT=$(echo "$API_RESPONSE" | jq '.service_deployments // [] | length' 2>/dev/null)
        log_test "26.3" "Dev contents: $DEP_COUNT GitOps deployments, $SVC_COUNT Docker services" "pass"
    else
        log_test "26.3" "Dev environment contents" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "26.3" "Dev environment contents" "skip" "no dev environment"
fi

# ── 26.4 Get production environment contents ───────────────────────────────
if [[ -n "${PROD_ENV_ID:-}" ]]; then
    log_test_start "26.4" "GET /environments/:id/contents shows production deployments"
    pepa_api GET "/environments/$PROD_ENV_ID/contents"
    if [[ "$API_STATUS" == "200" ]]; then
        PROD_DEP_COUNT=$(echo "$API_RESPONSE" | jq '.deployments // [] | length' 2>/dev/null)
        if [[ "$PROD_DEP_COUNT" -ge 1 ]] 2>/dev/null; then
            log_test "26.4" "Production has $PROD_DEP_COUNT deployment(s)" "pass"
        else
            log_test "26.4" "Production deployments" "fail" "expected >=1, got $PROD_DEP_COUNT"
        fi
    else
        log_test "26.4" "Production environment contents" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "26.4" "Production environment contents" "skip" "no production environment"
fi

# ── 26.5 Verify deployment has environment_id set ──────────────────────────
if [[ ${#ENV_DEPLOY_IDS[@]} -gt 0 && -n "${ENV_DEPLOY_IDS[0]}" ]]; then
    log_test_start "26.5" "Deployment has environment_id linked"
    pepa_api GET "/deployments/${ENV_DEPLOY_IDS[0]}"
    if [[ "$API_STATUS" == "200" ]]; then
        ENV_ID=$(echo "$API_RESPONSE" | jq -r '.environment_id // empty')
        if [[ -n "$ENV_ID" && "$ENV_ID" != "null" ]]; then
            log_test "26.5" "Deployment environment_id=${ENV_ID:0:8}..." "pass"
        else
            log_test "26.5" "Environment ID linkage" "fail" "environment_id is null/empty"
        fi
    else
        log_test "26.5" "Get deployment env_id" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "26.5" "Environment ID linkage" "skip" "no deployment"
fi

# ── 26.6 Compare two environments ──────────────────────────────────────────
if [[ -n "${DEV_ENV_ID:-}" && -n "${PROD_ENV_ID:-}" ]]; then
    log_test_start "26.6" "GET /environments/compare?env1=dev&env2=production"
    pepa_api GET "/environments/compare?env1=dev&env2=production"
    if [[ "$API_STATUS" == "200" ]]; then
        log_test "26.6" "Environment compare endpoint accessible" "pass"
    else
        log_test "26.6" "Environment compare" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "26.6" "Environment compare" "skip" "missing environments"
fi

# ── 26.7 Compare with missing params (400) ─────────────────────────────────
log_test_start "26.7" "GET /environments/compare without params returns 400"
pepa_api GET "/environments/compare"
if [[ "$API_STATUS" == "400" ]]; then
    log_test "26.7" "Missing params returns 400" "pass"
else
    log_test "26.7" "Missing compare params" "fail" "expected 400, got $API_STATUS"
fi

# ── 26.8 Get non-existent environment (404) ────────────────────────────────
log_test_start "26.8" "GET /environments/00000000-...-0099/contents returns 404"
pepa_api GET "/environments/00000000-0000-0000-0000-000000000099/contents"
if [[ "$API_STATUS" == "404" ]]; then
    log_test "26.8" "Non-existent environment returns 404" "pass"
else
    log_test "26.8" "Non-existent environment" "fail" "expected 404, got $API_STATUS"
fi

# ── 26.9 Cleanup ───────────────────────────────────────────────────────────
log_test_start "26.9" "Cleanup environment test deployments"
for eid in "${ENV_DEPLOY_IDS[@]}"; do
    if [[ -n "$eid" ]]; then
        pepa_api DELETE "/deployments/$eid" 2>/dev/null || true
    fi
done
log_test "26.9" "Cleanup done" "pass"

print_summary
