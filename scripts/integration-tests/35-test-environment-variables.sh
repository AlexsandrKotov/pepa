#!/usr/bin/env bash
# 35-test-environment-variables.sh — Environment CRUD, variables, compare, contents
# Covers: POST/GET/PUT/DELETE /environments, variables CRUD, compare, contents

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 35: Environment Variables, Compare & Contents"

pepa_login 2>/dev/null || true

# ── 35.1 Create environment ────────────────────────────────────────────────
log_test_start "35.1" "POST /environments creates a new environment"
pepa_api POST "/environments" '{
    "name": "Test Environment Alpha",
    "slug": "test-env-alpha",
    "type": "staging",
    "cluster": "pepa-test-primary",
    "namespace": "test-alpha-ns",
    "description": "Integration test environment",
    "color": "#FF5733"
}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    ENV1_ID=$(echo "$API_RESPONSE" | jq -r '.id // .environment.id // empty')
    ENV1_SLUG=$(echo "$API_RESPONSE" | jq -r '.slug // empty')
    log_test "35.1" "Environment created: id=${ENV1_ID:0:8}..., slug=$ENV1_SLUG" "pass"
else
    log_test "35.1" "Create environment" "fail" "HTTP $API_STATUS"
    ENV1_ID=""
fi

# ── 35.2 Create second environment for comparison ──────────────────────────
log_test_start "35.2" "POST /environments creates second environment"
pepa_api POST "/environments" '{
    "name": "Test Environment Beta",
    "slug": "test-env-beta",
    "type": "production",
    "cluster": "pepa-test-secondary",
    "namespace": "test-beta-ns",
    "description": "Second test environment",
    "color": "#3366FF"
}'
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    ENV2_ID=$(echo "$API_RESPONSE" | jq -r '.id // .environment.id // empty')
    log_test "35.2" "Second environment created: ${ENV2_ID:0:8}..." "pass"
else
    log_test "35.2" "Create second environment" "fail" "HTTP $API_STATUS"
    ENV2_ID=""
fi

# ── 35.3 Get environment by ID ─────────────────────────────────────────────
if [[ -n "$ENV1_ID" ]]; then
    log_test_start "35.3" "GET /environments/:id returns environment details"
    pepa_api GET "/environments/$ENV1_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        GOT_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
        GOT_SLUG=$(echo "$API_RESPONSE" | jq -r '.slug // empty')
        GOT_COLOR=$(echo "$API_RESPONSE" | jq -r '.color // empty')
        if [[ "$GOT_NAME" == "Test Environment Alpha" ]]; then
            log_test "35.3" "Env: name=$GOT_NAME, slug=$GOT_SLUG, color=$GOT_COLOR" "pass"
        else
            log_test "35.3" "Get environment" "fail" "name mismatch"
        fi
    else
        log_test "35.3" "Get environment" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "35.3" "Get environment" "skip" "no env"
fi

# ── 35.4 Update environment ────────────────────────────────────────────────
if [[ -n "$ENV1_ID" ]]; then
    log_test_start "35.4" "PUT /environments/:id updates environment"
    pepa_api PUT "/environments/$ENV1_ID" '{"description": "Updated description", "color": "#00FF00"}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        UPD_DESC=$(echo "$API_RESPONSE" | jq -r '.description // empty')
        UPD_COLOR=$(echo "$API_RESPONSE" | jq -r '.color // empty')
        if [[ "$UPD_DESC" == "Updated description" && "$UPD_COLOR" == "#00FF00" ]]; then
            log_test "35.4" "Env updated: desc=$UPD_DESC, color=$UPD_COLOR" "pass"
        else
            log_test "35.4" "Update environment" "fail" "fields not updated"
        fi
    else
        log_test "35.4" "Update environment" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "35.4" "Update environment" "skip" "no env"
fi

# ── 35.5 Create environment variable ───────────────────────────────────────
if [[ -n "$ENV1_ID" ]]; then
    log_test_start "35.5" "POST /environments/:id/variables creates variable"
    pepa_api POST "/environments/$ENV1_ID/variables" '{"key": "DATABASE_URL", "value": "postgres://localhost:5432/testdb", "is_secret": false}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        VAR_KEY=$(echo "$API_RESPONSE" | jq -r '.variable.key // .key // empty')
        log_test "35.5" "Variable created: key=$VAR_KEY" "pass"
    else
        log_test "35.5" "Create variable" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "35.5" "Create variable" "skip" "no env"
fi

# ── 35.6 Create secret variable ────────────────────────────────────────────
if [[ -n "$ENV1_ID" ]]; then
    log_test_start "35.6" "POST /environments/:id/variables creates secret"
    pepa_api POST "/environments/$ENV1_ID/variables" '{"key": "API_SECRET", "value": "super-secret-key-123", "is_secret": true}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "35.6" "Secret variable created" "pass"
    else
        log_test "35.6" "Create secret variable" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "35.6" "Create secret variable" "skip" "no env"
fi

# ── 35.7 List environment variables ────────────────────────────────────────
if [[ -n "$ENV1_ID" ]]; then
    log_test_start "35.7" "GET /environments/:id/variables lists variables"
    pepa_api GET "/environments/$ENV1_ID/variables"
    if [[ "$API_STATUS" == "200" ]]; then
        VAR_COUNT=$(echo "$API_RESPONSE" | jq '.variables // .items // [] | length' 2>/dev/null)
        if [[ "$VAR_COUNT" -ge 2 ]] 2>/dev/null; then
            log_test "35.7" "Variables: $VAR_COUNT found" "pass"
        else
            log_test "35.7" "List variables" "fail" "expected >=2, got $VAR_COUNT"
        fi
    else
        log_test "35.7" "List variables" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "35.7" "List variables" "skip" "no env"
fi

# ── 35.8 Add variable to second environment for comparison ─────────────────
if [[ -n "$ENV2_ID" ]]; then
    log_test_start "35.8" "Add variables to second environment"
    pepa_api POST "/environments/$ENV2_ID/variables" '{"key": "DATABASE_URL", "value": "postgres://prod:5432/proddb", "is_secret": false}'
    pepa_api POST "/environments/$ENV2_ID/variables" '{"key": "CACHE_TTL", "value": "3600", "is_secret": false}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "35.8" "Variables added to second env" "pass"
    else
        log_test "35.8" "Add variables to second env" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "35.8" "Add variables to second env" "skip" "no env2"
fi

# ── 35.9 Compare environments ──────────────────────────────────────────────
if [[ -n "$ENV1_ID" && -n "$ENV2_ID" ]]; then
    log_test_start "35.9" "GET /environments/compare?env1=slug1&env2=slug2 compares"
    pepa_api GET "/environments/compare?env1=test-env-alpha&env2=test-env-beta"
    if [[ "$API_STATUS" == "200" ]]; then
        COMPARE_TOTAL=$(echo "$API_RESPONSE" | jq '.total // .comparison | length' 2>/dev/null)
        DIFFS=$(echo "$API_RESPONSE" | jq '.differences // 0')
        HAS_COMPARISON=$(echo "$API_RESPONSE" | jq 'has("comparison")' 2>/dev/null)
        if [[ "$HAS_COMPARISON" == "true" ]]; then
            log_test "35.9" "Compare: $COMPARE_TOTAL vars, $DIFFS differences" "pass"
        else
            log_test "35.9" "Compare environments" "fail" "no comparison field"
        fi
    else
        log_test "35.9" "Compare environments" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "35.9" "Compare environments" "skip" "missing envs"
fi

# ── 35.10 Compare without required params returns 400 ──────────────────────
log_test_start "35.10" "GET /environments/compare without params returns 400"
pepa_api GET "/environments/compare"
if [[ "$API_STATUS" == "400" ]]; then
    log_test "35.10" "Missing compare params returns 400" "pass"
else
    log_test "35.10" "Missing compare params" "fail" "expected 400, got $API_STATUS"
fi

# ── 35.11 Get environment contents ─────────────────────────────────────────
if [[ -n "$ENV1_ID" ]]; then
    log_test_start "35.11" "GET /environments/:id/contents returns aggregated data"
    pepa_api GET "/environments/$ENV1_ID/contents"
    if [[ "$API_STATUS" == "200" ]]; then
        HAS_ENV=$(echo "$API_RESPONSE" | jq 'has("environment")' 2>/dev/null)
        HAS_CLUSTERS=$(echo "$API_RESPONSE" | jq 'has("clusters")' 2>/dev/null)
        HAS_DEPLOYS=$(echo "$API_RESPONSE" | jq 'has("deployments")' 2>/dev/null)
        HAS_SUMMARY=$(echo "$API_RESPONSE" | jq 'has("summary")' 2>/dev/null)
        if [[ "$HAS_ENV" == "true" && "$HAS_SUMMARY" == "true" ]]; then
            log_test "35.11" "Contents: env=$HAS_ENV, clusters=$HAS_CLUSTERS, deploys=$HAS_DEPLOYS, summary=$HAS_SUMMARY" "pass"
        else
            log_test "35.11" "Environment contents" "fail" "missing fields"
        fi
    else
        log_test "35.11" "Environment contents" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "35.11" "Environment contents" "skip" "no env"
fi

# ── 35.12 Delete environment variable ──────────────────────────────────────
if [[ -n "$ENV1_ID" ]]; then
    log_test_start "35.12" "DELETE /environments/:id/variables/:key deletes variable"
    pepa_api DELETE "/environments/$ENV1_ID/variables/DATABASE_URL"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "35.12" "Variable deleted" "pass"
    else
        log_test "35.12" "Delete variable" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "35.12" "Delete variable" "skip" "no env"
fi

# ── 35.13 Verify variable deletion ─────────────────────────────────────────
if [[ -n "$ENV1_ID" ]]; then
    log_test_start "35.13" "GET /environments/:id/variables confirms deletion"
    pepa_api GET "/environments/$ENV1_ID/variables"
    if [[ "$API_STATUS" == "200" ]]; then
        VAR_COUNT=$(echo "$API_RESPONSE" | jq '.variables // .items // [] | length' 2>/dev/null)
        HAS_DB_URL=$(echo "$API_RESPONSE" | jq '[.variables // .items // [] | .[] | select(.key=="DATABASE_URL")] | length' 2>/dev/null)
        if [[ "$HAS_DB_URL" == "0" ]]; then
            log_test "35.13" "Variable deleted: $VAR_COUNT remaining" "pass"
        else
            log_test "35.13" "Verify variable deletion" "fail" "DATABASE_URL still present"
        fi
    else
        log_test "35.13" "Verify variable deletion" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "35.13" "Verify variable deletion" "skip" "no env"
fi

# ── 35.14 Create environment with invalid color (400) ──────────────────────
log_test_start "35.14" "POST /environments with invalid color returns 400"
pepa_api POST "/environments" '{"name": "Bad Color", "slug": "bad-color", "color": "not-a-color"}'
if [[ "$API_STATUS" == "400" ]]; then
    log_test "35.14" "Invalid color returns 400" "pass"
else
    log_test "35.14" "Invalid color" "fail" "expected 400, got $API_STATUS"
fi

# ── 35.15 Delete environments ──────────────────────────────────────────────
log_test_start "35.15" "Cleanup test environments"
for eid in "$ENV1_ID" "$ENV2_ID"; do
    if [[ -n "$eid" ]]; then
        pepa_api DELETE "/environments/$eid" 2>/dev/null || true
    fi
done
log_test "35.15" "Cleanup done" "pass"

# ── 35.16 Verify environment deletion ──────────────────────────────────────
if [[ -n "$ENV1_ID" ]]; then
    log_test_start "35.16" "GET /environments/:id returns 404 after deletion"
    pepa_api GET "/environments/$ENV1_ID"
    if [[ "$API_STATUS" == "404" ]]; then
        log_test "35.16" "Deleted environment returns 404" "pass"
    else
        log_test "35.16" "Deleted environment" "fail" "expected 404, got $API_STATUS"
    fi
else
    log_test "35.16" "Verify deletion" "skip" "no env"
fi

print_summary
