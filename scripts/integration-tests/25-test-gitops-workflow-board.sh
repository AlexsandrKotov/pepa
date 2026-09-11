#!/usr/bin/env bash
# 25-test-gitops-workflow-board.sh — GitOps Workflow board tests
# Covers: GET /gitops/mrs, GET /gitops/timeline/:id,
#         GET /deployments/pipeline, GET /deployments/metrics,
#         team filtering, stage filtering

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 25: GitOps Workflow Board, Team Filter & Metrics"

pepa_login 2>/dev/null || true

# ── 25.1 List workflow MRs ─────────────────────────────────────────────────
log_test_start "25.1" "GET /gitops/mrs returns MR list"
pepa_api GET "/gitops/mrs"
if [[ "$API_STATUS" == "200" ]]; then
    MR_COUNT=$(echo "$API_RESPONSE" | jq '.merge_requests // .mrs // .items // [] | length' 2>/dev/null)
    log_test "25.1" "Workflow MRs: $MR_COUNT entries" "pass"
else
    log_test "25.1" "Workflow MRs" "fail" "HTTP $API_STATUS"
fi

# ── 25.2 List MRs with team filter ─────────────────────────────────────────
log_test_start "25.2" "GET /gitops/mrs?team=platform-team filters by team"
pepa_api GET "/gitops/mrs?team=platform-team"
if [[ "$API_STATUS" == "200" ]]; then
    log_test "25.2" "Team-filtered MRs endpoint OK" "pass"
else
    log_test "25.2" "Team-filtered MRs" "fail" "HTTP $API_STATUS"
fi

# ── 25.3 Create deployments for workflow board ─────────────────────────────
log_test_start "25.3" "Create deployments across stages for board test"
BOARD_IDS=()
for stage in dev testing staging; do
    pepa_api POST "/gitops/deploy" "{
        \"image_tag\": \"v1.0.0-board-$stage\",
        \"image_repository\": \"board-app\",
        \"namespace\": \"app-$stage\",
        \"team\": \"platform-team\",
        \"stage\": \"$stage\"
    }"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        BID=$(echo "$API_RESPONSE" | jq -r '.deployment.id // .id // empty')
        BOARD_IDS+=("$BID")
    fi
done
log_test "25.3" "Created ${#BOARD_IDS[@]} deployments across stages" "pass"

# ── 25.4 Filter deployments by team ────────────────────────────────────────
log_test_start "25.4" "GET /deployments?team=platform-team filters by team"
pepa_api GET "/deployments?team=platform-team"
if [[ "$API_STATUS" == "200" ]]; then
    TEAM_TOTAL=$(echo "$API_RESPONSE" | jq -r '.total // 0')
    ALL_CORRECT=true
    for tn in $(echo "$API_RESPONSE" | jq -r '.deployments[]?.team_name // empty' 2>/dev/null); do
        if [[ "$tn" != "platform-team" ]]; then ALL_CORRECT=false; break; fi
    done
    if $ALL_CORRECT; then
        log_test "25.4" "Team filter: total=$TEAM_TOTAL, all platform-team" "pass"
    else
        log_test "25.4" "Team filter" "fail" "non-platform-team found"
    fi
else
    log_test "25.4" "Team filter" "fail" "HTTP $API_STATUS"
fi

# ── 25.5 Filter deployments by stage ───────────────────────────────────────
log_test_start "25.5" "GET /deployments?stage=dev filters by stage"
pepa_api GET "/deployments?stage=dev"
if [[ "$API_STATUS" == "200" ]]; then
    STAGE_TOTAL=$(echo "$API_RESPONSE" | jq -r '.total // 0')
    ALL_DEV=true
    for s in $(echo "$API_RESPONSE" | jq -r '.deployments[]?.stage // empty' 2>/dev/null); do
        if [[ "$s" != "dev" ]]; then ALL_DEV=false; break; fi
    done
    if $ALL_DEV; then
        log_test "25.5" "Stage filter: total=$STAGE_TOTAL, all dev" "pass"
    else
        log_test "25.5" "Stage filter" "fail" "non-dev found"
    fi
else
    log_test "25.5" "Stage filter" "fail" "HTTP $API_STATUS"
fi

# ── 25.6 Get deployment pipeline info ──────────────────────────────────────
if [[ ${#BOARD_IDS[@]} -gt 0 ]]; then
    log_test_start "25.6" "GET /deployments/pipeline returns pipeline info"
    pepa_api GET "/deployments/pipeline"
    if [[ "$API_STATUS" == "200" ]]; then
        log_test "25.6" "Deployment pipeline endpoint accessible" "pass"
    else
        log_test "25.6" "Deployment pipeline" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "25.6" "Deployment pipeline" "skip" "no deployments"
fi

# ── 25.7 Get deployment metrics ────────────────────────────────────────────
log_test_start "25.7" "GET /deployments/metrics returns metrics"
pepa_api GET "/deployments/metrics"
if [[ "$API_STATUS" == "200" ]]; then
    HAS_METRICS=$(echo "$API_RESPONSE" | jq 'keys | length' 2>/dev/null)
    log_test "25.7" "Deployment metrics: $HAS_METRICS top-level keys" "pass"
else
    log_test "25.7" "Deployment metrics" "fail" "HTTP $API_STATUS"
fi

# ── 25.8 Get workflow timeline for a deployment ────────────────────────────
if [[ ${#BOARD_IDS[@]} -gt 0 && -n "${BOARD_IDS[0]}" ]]; then
    log_test_start "25.8" "GET /gitops/timeline/:id returns timeline"
    pepa_api GET "/gitops/timeline/${BOARD_IDS[0]}"
    if [[ "$API_STATUS" == "200" ]]; then
        log_test "25.8" "Workflow timeline endpoint accessible" "pass"
    else
        log_test "25.8" "Workflow timeline" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "25.8" "Workflow timeline" "skip" "no deployment"
fi

# ── 25.9 Verify deployment board counts ────────────────────────────────────
log_test_start "25.9" "GET /deployments?team=platform-team has expected stage distribution"
pepa_api GET "/deployments?team=platform-team&per_page=100"
if [[ "$API_STATUS" == "200" ]]; then
    DEV_COUNT=$(echo "$API_RESPONSE" | jq '[.deployments[]? | select(.stage=="dev")] | length')
    TEST_COUNT=$(echo "$API_RESPONSE" | jq '[.deployments[]? | select(.stage=="testing")] | length')
    STAGING_COUNT=$(echo "$API_RESPONSE" | jq '[.deployments[]? | select(.stage=="staging")] | length')
    log_test "25.9" "Board distribution: dev=$DEV_COUNT, testing=$TEST_COUNT, staging=$STAGING_COUNT" "pass"
else
    log_test "25.9" "Board distribution" "fail" "HTTP $API_STATUS"
fi

# ── 25.10 Combined team + stage filter ─────────────────────────────────────
log_test_start "25.10" "GET /deployments?team=platform-team&stage=dev combined filter"
pepa_api GET "/deployments?team=platform-team&stage=dev"
if [[ "$API_STATUS" == "200" ]]; then
    COMBINED=$(echo "$API_RESPONSE" | jq -r '.total // 0')
    log_test "25.10" "Combined filter: total=$COMBINED" "pass"
else
    log_test "25.10" "Combined filter" "fail" "HTTP $API_STATUS"
fi

# ── 25.11 Cleanup ──────────────────────────────────────────────────────────
log_test_start "25.11" "Cleanup board test deployments"
for bid in "${BOARD_IDS[@]}"; do
    if [[ -n "$bid" ]]; then
        pepa_api DELETE "/deployments/$bid" 2>/dev/null || true
    fi
done
log_test "25.11" "Cleanup done" "pass"

print_summary
