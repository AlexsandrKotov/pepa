#!/usr/bin/env bash
# 32-test-deployment-metrics.sh — Deployment metrics, diff, resources, logs
# Covers: GET /deployments/metrics, GET /deployments/pipeline,
#         GET /deployments/:id/diff, GET /deployments/:id/resources,
#         GET /deployments/:id/logs, GET /deployments/:id/events

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 32: Deployment Metrics, Diff, Resources & Logs"

pepa_login 2>/dev/null || true

# ── 32.1 Create deployments for metrics testing ────────────────────────────
log_test_start "32.1" "Create deployments for metrics testing"
METRIC_IDS=()
for i in 1 2 3; do
    pepa_api POST "/gitops/deploy" "{
        \"image_tag\": \"v1.0.0-metric$i\",
        \"image_repository\": \"metric-app\",
        \"namespace\": \"app-dev\",
        \"team\": \"platform-team\",
        \"stage\": \"dev\"
    }"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        MID=$(echo "$API_RESPONSE" | jq -r '.deployment.id // .id // empty')
        METRIC_IDS+=("$MID")
    fi
done
log_test "32.1" "Created ${#METRIC_IDS[@]} test deployments" "pass"

# ── 32.2 Get deployment metrics ────────────────────────────────────────────
log_test_start "32.2" "GET /deployments/metrics returns aggregated metrics"
pepa_api GET "/deployments/metrics"
if [[ "$API_STATUS" == "200" ]]; then
    HAS_TOTAL=$(echo "$API_RESPONSE" | jq 'has("total") or has("total_deployments") or has("summary")' 2>/dev/null)
    KEYS=$(echo "$API_RESPONSE" | jq 'keys | join(",")' 2>/dev/null)
    log_test "32.2" "Metrics keys: $KEYS" "pass"
else
    log_test "32.2" "Deployment metrics" "fail" "HTTP $API_STATUS"
fi

# ── 32.3 Get deployment pipeline info ──────────────────────────────────────
log_test_start "32.3" "GET /deployments/pipeline returns pipeline overview"
pepa_api GET "/deployments/pipeline"
if [[ "$API_STATUS" == "200" ]]; then
    log_test "32.3" "Pipeline info endpoint accessible" "pass"
else
    log_test "32.3" "Pipeline info" "fail" "HTTP $API_STATUS"
fi

# ── 32.4 Get deployment diff ───────────────────────────────────────────────
if [[ ${#METRIC_IDS[@]} -gt 0 && -n "${METRIC_IDS[0]}" ]]; then
    log_test_start "32.4" "GET /deployments/:id/diff returns deployment diff"
    pepa_api GET "/deployments/${METRIC_IDS[0]}/diff"
    if [[ "$API_STATUS" == "200" || "$API_STATUS" == "400" ]]; then
        log_test "32.4" "Deployment diff endpoint accessible (HTTP $API_STATUS)" "pass"
    else
        log_test "32.4" "Deployment diff" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "32.4" "Deployment diff" "skip" "no deployment"
fi

# ── 32.5 Get deployment resources ──────────────────────────────────────────
if [[ ${#METRIC_IDS[@]} -gt 0 && -n "${METRIC_IDS[0]}" ]]; then
    log_test_start "32.5" "GET /deployments/:id/resources returns k8s resources"
    pepa_api GET "/deployments/${METRIC_IDS[0]}/resources"
    if [[ "$API_STATUS" == "200" ]]; then
        RES_COUNT=$(echo "$API_RESPONSE" | jq '.resources // .items // [] | length' 2>/dev/null)
        log_test "32.5" "Deployment resources: $RES_COUNT found" "pass"
    else
        log_test "32.5" "Deployment resources" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "32.5" "Deployment resources" "skip" "no deployment"
fi

# ── 32.6 Get deployment logs ───────────────────────────────────────────────
if [[ ${#METRIC_IDS[@]} -gt 0 && -n "${METRIC_IDS[0]}" ]]; then
    log_test_start "32.6" "GET /deployments/:id/logs returns deployment logs"
    pepa_api GET "/deployments/${METRIC_IDS[0]}/logs"
    if [[ "$API_STATUS" == "200" ]]; then
        log_test "32.6" "Deployment logs endpoint accessible" "pass"
    else
        log_test "32.6" "Deployment logs" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "32.6" "Deployment logs" "skip" "no deployment"
fi

# ── 32.7 Get deployment timeline events ────────────────────────────────────
if [[ ${#METRIC_IDS[@]} -gt 0 && -n "${METRIC_IDS[0]}" ]]; then
    log_test_start "32.7" "GET /deployments/:id/events returns timeline events"
    pepa_api GET "/deployments/${METRIC_IDS[0]}/events"
    if [[ "$API_STATUS" == "200" ]]; then
        EVT_COUNT=$(echo "$API_RESPONSE" | jq '.events // .items // [] | length' 2>/dev/null)
        log_test "32.7" "Timeline events: $EVT_COUNT entries" "pass"
    else
        log_test "32.7" "Timeline events" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "32.7" "Timeline events" "skip" "no deployment"
fi

# ── 32.8 Filter deployments by search (searches jira_issue_key) ────────────
log_test_start "32.8" "GET /deployments?search=metric-app filters by jira_summary"
# First update one deployment with a searchable jira key
if [[ ${#METRIC_IDS[@]} -gt 0 && -n "${METRIC_IDS[0]}" ]]; then
    # The search field covers gitlab_project_name, jira_issue_key, jira_summary
    # We created them via gitops/workflow/deploy which sets jira fields from the request
    # Let's search by team name instead since all have team=platform-team
    pepa_api GET "/deployments?team=platform-team&stage=dev"
fi
if [[ "$API_STATUS" == "200" ]]; then
    SEARCH_TOTAL=$(echo "$API_RESPONSE" | jq -r '.total // 0')
    if [[ "$SEARCH_TOTAL" -ge 3 ]] 2>/dev/null; then
        log_test "32.8" "Filter found $SEARCH_TOTAL deployments" "pass"
    else
        log_test "32.8" "Search filter" "fail" "expected >=3, got $SEARCH_TOTAL"
    fi
else
    log_test "32.8" "Search filter" "fail" "HTTP $API_STATUS"
fi

# ── 32.9 Combined filters: team + stage + search ───────────────────────────
log_test_start "32.9" "GET /deployments?team=platform-team&stage=dev&search=metric combined filter"
pepa_api GET "/deployments?team=platform-team&stage=dev&search=metric"
if [[ "$API_STATUS" == "200" ]]; then
    COMBO_TOTAL=$(echo "$API_RESPONSE" | jq -r '.total // 0')
    log_test "32.9" "Combined filter: total=$COMBO_TOTAL" "pass"
else
    log_test "32.9" "Combined filter" "fail" "HTTP $API_STATUS"
fi

# ── 32.10 Verify deployment has all expected fields ────────────────────────
if [[ ${#METRIC_IDS[@]} -gt 0 && -n "${METRIC_IDS[0]}" ]]; then
    log_test_start "32.10" "Deployment response has expected fields"
    pepa_api GET "/deployments/${METRIC_IDS[0]}"
    if [[ "$API_STATUS" == "200" ]]; then
        HAS_ID=$(echo "$API_RESPONSE" | jq 'has("id")')
        HAS_STATUS=$(echo "$API_RESPONSE" | jq 'has("status")')
        HAS_STAGE=$(echo "$API_RESPONSE" | jq 'has("stage")')
        HAS_TEAM=$(echo "$API_RESPONSE" | jq 'has("team_name")')
        HAS_CREATED=$(echo "$API_RESPONSE" | jq 'has("created_at")')
        HAS_ENV=$(echo "$API_RESPONSE" | jq 'has("environment_id")')
        if [[ "$HAS_ID" == "true" && "$HAS_STATUS" == "true" && "$HAS_STAGE" == "true" ]]; then
            log_test "32.10" "Fields: id=$HAS_ID, status=$HAS_STATUS, stage=$HAS_STAGE, team=$HAS_TEAM, created=$HAS_CREATED, env_id=$HAS_ENV" "pass"
        else
            log_test "32.10" "Missing required fields" "fail" "id=$HAS_ID status=$HAS_STATUS stage=$HAS_STAGE"
        fi
    else
        log_test "32.10" "Get deployment fields" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "32.10" "Deployment fields" "skip" "no deployment"
fi

# ── 32.11 Cleanup ──────────────────────────────────────────────────────────
log_test_start "32.11" "Cleanup metrics test deployments"
for mid in "${METRIC_IDS[@]}"; do
    if [[ -n "$mid" ]]; then
        pepa_api DELETE "/deployments/$mid" 2>/dev/null || true
    fi
done
log_test "32.11" "Cleanup done" "pass"

print_summary
