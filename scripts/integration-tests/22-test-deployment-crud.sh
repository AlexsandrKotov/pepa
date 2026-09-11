#!/usr/bin/env bash
# 22-test-deployment-crud.sh — Deployment CRUD, filter, pagination tests
# Covers: POST /deployments, GET /deployments, GET /deployments/:id, DELETE /deployments/:id
# Also: POST /deployments/dry-run, query filters (status, team, stage, search, cluster_id)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 22: Deployment CRUD, Filter & Pagination"

pepa_login 2>/dev/null || true

# ── 22.1 List deployments (empty or existing) ──────────────────────────────
log_test_start "22.1" "GET /deployments returns paginated list"
pepa_api GET "/deployments"
if [[ "$API_STATUS" == "200" ]]; then
    TOTAL=$(echo "$API_RESPONSE" | jq -r '.total // 0')
    PAGE=$(echo "$API_RESPONSE" | jq -r '.page // 1')
    PER_PAGE=$(echo "$API_RESPONSE" | jq -r '.per_page // 20')
    log_test "22.1" "GET /deployments returns 200 (total=$TOTAL, page=$PAGE, per_page=$PER_PAGE)" "pass"
else
    log_test "22.1" "GET /deployments" "fail" "HTTP $API_STATUS"
fi

# ── 22.2 Create deployment via POST /deployments ───────────────────────────
log_test_start "22.2" "POST /deployments creates a new deployment"
DEPLOY_BODY='{
    "target_namespace": "test-crud-ns",
    "image_tag": "v1.0.0-crud",
    "image_repository": "test-app",
    "deploy_type": "helm",
    "replicas": 2,
    "strategy": "rolling",
    "created_by": "integration-test",
    "spec": {"chart": "nginx", "values": {"replicaCount": 2}}
}'
pepa_api POST "/deployments" "$DEPLOY_BODY"
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    DEPLOY_ID=$(echo "$API_RESPONSE" | jq -r '.id // .deployment.id // empty')
    DEPLOY_STATUS=$(echo "$API_RESPONSE" | jq -r '.status // .deployment.status // empty')
    log_test "22.2" "POST /deployments creates deployment (id=${DEPLOY_ID:0:8}..., status=$DEPLOY_STATUS)" "pass"
else
    log_test "22.2" "POST /deployments" "fail" "HTTP $API_STATUS"
    DEPLOY_ID=""
fi

# ── 22.3 Get deployment by ID ──────────────────────────────────────────────
if [[ -n "$DEPLOY_ID" ]]; then
    log_test_start "22.3" "GET /deployments/:id returns the created deployment"
    pepa_api GET "/deployments/$DEPLOY_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        GOT_ID=$(echo "$API_RESPONSE" | jq -r '.id // empty')
        GOT_NS=$(echo "$API_RESPONSE" | jq -r '.target_namespace // empty')
        GOT_TAG=$(echo "$API_RESPONSE" | jq -r '.image_tag // empty')
        if [[ "$GOT_ID" == "$DEPLOY_ID" ]]; then
            log_test "22.3" "GET /deployments/:id returns correct deployment (ns=$GOT_NS, tag=$GOT_TAG)" "pass"
        else
            log_test "22.3" "GET /deployments/:id" "fail" "id mismatch: expected=$DEPLOY_ID got=$GOT_ID"
        fi
    else
        log_test "22.3" "GET /deployments/:id" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "22.3" "GET /deployments/:id" "skip" "no deployment created"
fi

# ── 22.4 Create second deployment with different attributes ────────────────
log_test_start "22.4" "POST /deployments creates second deployment for filtering"
DEPLOY2_BODY='{
    "target_namespace": "test-crud-ns-2",
    "image_tag": "v2.0.0-crud",
    "image_repository": "test-app-2",
    "deploy_type": "kubectl",
    "replicas": 1,
    "strategy": "recreate",
    "created_by": "integration-test-2",
    "status": "running",
    "spec": {}
}'
pepa_api POST "/deployments" "$DEPLOY2_BODY"
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    DEPLOY2_ID=$(echo "$API_RESPONSE" | jq -r '.id // .deployment.id // empty')
    log_test "22.4" "Second deployment created (id=${DEPLOY2_ID:0:8}...)" "pass"
else
    log_test "22.4" "POST /deployments (second)" "fail" "HTTP $API_STATUS"
    DEPLOY2_ID=""
fi

# ── 22.5 Filter deployments by status ─────────────────────────────────────
log_test_start "22.5" "GET /deployments?status=pending filters by status"
pepa_api GET "/deployments?status=pending"
if [[ "$API_STATUS" == "200" ]]; then
    FILTERED_TOTAL=$(echo "$API_RESPONSE" | jq -r '.total // 0')
    ALL_PENDING=true
    for s in $(echo "$API_RESPONSE" | jq -r '.deployments[]?.status // empty' 2>/dev/null); do
        if [[ "$s" != "pending" ]]; then ALL_PENDING=false; break; fi
    done
    if $ALL_PENDING; then
        log_test "22.5" "Filter by status=pending: total=$FILTERED_TOTAL, all pending" "pass"
    else
        log_test "22.5" "Filter by status=pending" "fail" "non-pending found in result"
    fi
else
    log_test "22.5" "Filter by status" "fail" "HTTP $API_STATUS"
fi

# ── 22.6 Filter by search term (searches jira_issue_key, jira_summary, gitlab_project_name) ─
log_test_start "22.6" "GET /deployments?search=integration-test filters by created_by/project"
# Create a deployment with a searchable jira key
pepa_api POST "/deployments" '{
    "target_namespace": "test-search-ns",
    "image_tag": "v1.0.0-search",
    "deploy_type": "helm",
    "replicas": 1,
    "jira_issue_key": "PEPA-SEARCH-TEST",
    "spec": {}
}'
SEARCH_DEPLOY_ID=$(echo "$API_RESPONSE" | jq -r '.id // .deployment.id // empty')
pepa_api GET "/deployments?search=PEPA-SEARCH-TEST"
if [[ "$API_STATUS" == "200" ]]; then
    SEARCH_TOTAL=$(echo "$API_RESPONSE" | jq -r '.total // 0')
    if [[ "$SEARCH_TOTAL" -ge 1 ]] 2>/dev/null; then
        log_test "22.6" "Search by jira_issue_key: found $SEARCH_TOTAL result(s)" "pass"
    else
        log_test "22.6" "Search by jira_issue_key" "fail" "expected >=1 result, got $SEARCH_TOTAL"
    fi
else
    log_test "22.6" "Search filter" "fail" "HTTP $API_STATUS"
fi
# Cleanup search test deployment
if [[ -n "$SEARCH_DEPLOY_ID" ]]; then
    pepa_api DELETE "/deployments/$SEARCH_DEPLOY_ID" 2>/dev/null || true
fi

# ── 22.7 Pagination: per_page=1 ────────────────────────────────────────────
log_test_start "22.7" "GET /deployments?per_page=1 returns paginated result"
pepa_api GET "/deployments?per_page=1"
if [[ "$API_STATUS" == "200" ]]; then
    PP_TOTAL=$(echo "$API_RESPONSE" | jq -r '.total // 0')
    PP_PAGES=$(echo "$API_RESPONSE" | jq -r '.total_pages // 0')
    PP_ITEMS=$(echo "$API_RESPONSE" | jq '.deployments | length')
    if [[ "$PP_ITEMS" -le 1 ]] 2>/dev/null; then
        log_test "22.7" "Pagination per_page=1: items=$PP_ITEMS, total=$PP_TOTAL, pages=$PP_PAGES" "pass"
    else
        log_test "22.7" "Pagination per_page=1" "fail" "expected <=1 item, got $PP_ITEMS"
    fi
else
    log_test "22.7" "Pagination" "fail" "HTTP $API_STATUS"
fi

# ── 22.8 Pagination: page=2 ────────────────────────────────────────────────
log_test_start "22.8" "GET /deployments?per_page=1&page=2 returns second page"
pepa_api GET "/deployments?per_page=1&page=2"
if [[ "$API_STATUS" == "200" ]]; then
    P2_PAGE=$(echo "$API_RESPONSE" | jq -r '.page // 0')
    if [[ "$P2_PAGE" == "2" ]]; then
        log_test "22.8" "Page 2 returned page=$P2_PAGE" "pass"
    else
        log_test "22.8" "Page 2" "fail" "expected page=2, got $P2_PAGE"
    fi
else
    log_test "22.8" "Page 2" "fail" "HTTP $API_STATUS"
fi

# ── 22.9 Dry-run deployment ────────────────────────────────────────────────
log_test_start "22.9" "POST /deployments/dry-run validates without creating"
DRY_BODY='{
    "target_namespace": "dry-run-ns",
    "image_tag": "v1.0.0-dry",
    "deploy_type": "helm",
    "replicas": 1,
    "spec": {}
}'
pepa_api POST "/deployments/dry-run" "$DRY_BODY"
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    log_test "22.9" "Dry-run deployment accepted (HTTP $API_STATUS)" "pass"
else
    log_test "22.9" "Dry-run deployment" "fail" "HTTP $API_STATUS"
fi

# ── 22.10 Get deployment history ───────────────────────────────────────────
if [[ -n "$DEPLOY_ID" ]]; then
    log_test_start "22.10" "GET /deployments/:id/history returns history"
    pepa_api GET "/deployments/$DEPLOY_ID/history"
    if [[ "$API_STATUS" == "200" ]]; then
        log_test "22.10" "Deployment history endpoint accessible" "pass"
    else
        log_test "22.10" "Deployment history" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "22.10" "GET /deployments/:id/history" "skip" "no deployment"
fi

# ── 22.11 Get deployment events ────────────────────────────────────────────
if [[ -n "$DEPLOY_ID" ]]; then
    log_test_start "22.11" "GET /deployments/:id/events returns timeline events"
    pepa_api GET "/deployments/$DEPLOY_ID/events"
    if [[ "$API_STATUS" == "200" ]]; then
        log_test "22.11" "Deployment events endpoint accessible" "pass"
    else
        log_test "22.11" "Deployment events" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "22.11" "GET /deployments/:id/events" "skip" "no deployment"
fi

# ── 22.12 Delete deployments ───────────────────────────────────────────────
for did in "$DEPLOY_ID" "$DEPLOY2_ID"; do
    if [[ -n "$did" ]]; then
        log_test_start "22.12" "DELETE /deployments/$did"
        pepa_api DELETE "/deployments/$did"
        if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
            log_test "22.12" "DELETE /deployments/${did:0:8}... OK" "pass"
        else
            log_test "22.12" "DELETE /deployments/$did" "fail" "HTTP $API_STATUS"
        fi
    fi
done

# ── 22.13 Verify deletion ──────────────────────────────────────────────────
if [[ -n "$DEPLOY_ID" ]]; then
    log_test_start "22.13" "GET /deployments/:id returns 404 after deletion"
    pepa_api GET "/deployments/$DEPLOY_ID"
    if [[ "$API_STATUS" == "404" ]]; then
        log_test "22.13" "Deleted deployment returns 404" "pass"
    else
        log_test "22.13" "Deleted deployment" "fail" "expected 404, got $API_STATUS"
    fi
else
    log_test "22.13" "Verify deletion" "skip" "no deployment"
fi

# ── 22.14 Create deployment with invalid body (400) ────────────────────────
log_test_start "22.14" "POST /deployments with invalid JSON returns 400"
pepa_api POST "/deployments" '{"invalid": }'
if [[ "$API_STATUS" == "400" || "$API_STATUS" == "422" ]]; then
    log_test "22.14" "Invalid body returns $API_STATUS" "pass"
else
    log_test "22.14" "Invalid body" "fail" "expected 400/422, got $API_STATUS"
fi

# ── 22.15 Get non-existent deployment (404) ────────────────────────────────
log_test_start "22.15" "GET /deployments/00000000-0000-0000-0000-000000000099 returns 404"
pepa_api GET "/deployments/00000000-0000-0000-0000-000000000099"
if [[ "$API_STATUS" == "404" ]]; then
    log_test "22.15" "Non-existent deployment returns 404" "pass"
else
    log_test "22.15" "Non-existent deployment" "fail" "expected 404, got $API_STATUS"
fi

print_summary
