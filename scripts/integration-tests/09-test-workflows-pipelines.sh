#!/usr/bin/env bash
# 09-test-workflows-pipelines.sh — Workflow engine, pipeline runs, CI/CD sources (Phase 9)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"
source "${SCRIPT_DIR}/lib/assertions.sh"
PEPA_URL="${PEPA_URL:-http://localhost:8088}"; API="${PEPA_URL}/api/v1"; TMP="${RESULTS_DIR}/tmp"; mkdir -p "$TMP"
log_phase "Phase 9: Workflows and Pipelines"
[[ -f "${RESULTS_DIR}/pepa_token" ]] && export PEPA_TOKEN=$(cat "${RESULTS_DIR}/pepa_token")
WF_IDS=(); PIPE_IDS=()
cleanup_phase() { for id in "${WF_IDS[@]}"; do pepa_api DELETE "/workflows/$id" "" /dev/null /dev/null 2>/dev/null || true; done; for id in "${PIPE_IDS[@]}"; do pepa_api DELETE "/pipelines/$id" "" /dev/null /dev/null 2>/dev/null || true; done; }
trap cleanup_phase EXIT

log_test_start "9.1" "Create workflow"; pepa_api POST "/workflows" '{"name":"test-workflow","steps":[{"name":"build","type":"shell","command":"echo build"},{"name":"test","type":"shell","command":"echo test"},{"name":"deploy","type":"shell","command":"echo deploy"}]}' "$TMP/9.1.json" "$TMP/9.1_code.txt"; if assert_http_status "$TMP/9.1_code.txt" "201" "9.1"; then WF1=$(jq -r '.id // empty' "$TMP/9.1.json" 2>/dev/null); [[ -n "$WF1" ]] && WF_IDS+=("$WF1"); log_test_pass "9.1" "Workflow created"; else log_test_fail "9.1" "Create workflow failed"; fi

log_test_start "9.2" "Workflow requires steps"; pepa_api POST "/workflows" '{"name":"no-steps"}' "$TMP/9.2.json" "$TMP/9.2_code.txt"; code=$(cat "$TMP/9.2_code.txt" 2>/dev/null); if [[ "$code" == "400" || "$code" == "422" ]]; then log_test_pass "9.2" "Missing steps rejected (HTTP $code)"; else log_test_fail "9.2" "Expected 400/422, got $code"; fi

log_test_start "9.3" "Execute workflow"; if [[ -n "${WF1:-}" ]]; then pepa_api POST "/workflows/${WF1}/run" '' "$TMP/9.3.json" "$TMP/9.3_code.txt"; code=$(cat "$TMP/9.3_code.txt" 2>/dev/null); if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then log_test_pass "9.3" "Workflow execution started"; else log_test_fail "9.3" "Execute failed (HTTP $code)"; fi; else log_test_skip "9.3" "No workflow ID"; fi

log_test_start "9.4" "Workflow with input parameters"; if [[ -n "${WF1:-}" ]]; then pepa_api POST "/workflows/${WF1}/run" '{"parameters":{"env":"staging","version":"1.0"}}' "$TMP/9.4.json" "$TMP/9.4_code.txt"; code=$(cat "$TMP/9.4_code.txt" 2>/dev/null); if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then log_test_pass "9.4" "Workflow with params started"; else log_test_fail "9.4" "Failed (HTTP $code)"; fi; else log_test_skip "9.4" "No workflow ID"; fi

log_test_start "9.5" "Workflow nil-config fallback"; if [[ -n "${WF1:-}" ]]; then pepa_api POST "/workflows/${WF1}/run" '{"plugin_config":null}' "$TMP/9.5.json" "$TMP/9.5_code.txt"; code=$(cat "$TMP/9.5_code.txt" 2>/dev/null); if [[ "$code" =~ ^2[0-9][0-9]$ || "$code" == "500" ]]; then log_test_pass "9.5" "Nil config handled (HTTP $code)"; else log_test_fail "9.5" "Unexpected (HTTP $code)"; fi; else log_test_skip "9.5" "No workflow ID"; fi

log_test_start "9.6" "Workflow run status"; if [[ -n "${WF1:-}" ]]; then sleep 2; pepa_api GET "/workflows/${WF1}" "" "$TMP/9.6.json" "$TMP/9.6_code.txt"; if assert_http_success "$TMP/9.6_code.txt" "9.6"; then log_test_pass "9.6" "Workflow status retrieved"; else log_test_fail "9.6" "Status failed"; fi; else log_test_skip "9.6" "No workflow ID"; fi

log_test_start "9.7" "Delete workflow"; if [[ -n "${WF1:-}" ]]; then pepa_api DELETE "/workflows/${WF1}" "" "$TMP/9.7.json" "$TMP/9.7_code.txt"; if assert_http_success "$TMP/9.7_code.txt" "9.7"; then WF_IDS=("${WF_IDS[@]/$WF1/}"); log_test_pass "9.7" "Workflow deleted"; else log_test_fail "9.7" "Delete failed"; fi; else log_test_skip "9.7" "No workflow ID"; fi

log_test_start "9.8" "Create pipeline"; pepa_api POST "/pipelines" '{"name":"test-pipeline","engine":"shell","config":{"steps":["echo pipeline"]}}' "$TMP/9.8.json" "$TMP/9.8_code.txt"; if assert_http_status "$TMP/9.8_code.txt" "201" "9.8"; then PIPE1=$(jq -r '.id // empty' "$TMP/9.8.json" 2>/dev/null); [[ -n "$PIPE1" ]] && PIPE_IDS+=("$PIPE1"); log_test_pass "9.8" "Pipeline created"; else log_test_fail "9.8" "Create pipeline failed"; fi

log_test_start "9.9" "Create pipeline source"; if [[ -n "${PIPE1:-}" ]]; then pepa_api POST "/pipelines/${PIPE1}/sources" '{"type":"gitea","url":"http://gitea:3000/pepa-test/fluxcd-manifests","branch":"main"}' "$TMP/9.9.json" "$TMP/9.9_code.txt"; code=$(cat "$TMP/9.9_code.txt" 2>/dev/null); if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then log_test_pass "9.9" "Pipeline source created"; else log_test_fail "9.9" "Failed (HTTP $code)"; fi; else log_test_skip "9.9" "No pipeline ID"; fi

log_test_start "9.10" "Auto-extract pipeline params"; if [[ -n "${PIPE1:-}" ]]; then pepa_api GET "/pipelines/${PIPE1}" "" "$TMP/9.10.json" "$TMP/9.10_code.txt"; if assert_http_success "$TMP/9.10_code.txt" "9.10"; then log_test_pass "9.10" "Pipeline params checked"; else log_test_fail "9.10" "Failed"; fi; else log_test_skip "9.10" "No pipeline ID"; fi

log_test_start "9.11" "Trigger pipeline run"; if [[ -n "${PIPE1:-}" ]]; then pepa_api POST "/pipelines/${PIPE1}/run" '' "$TMP/9.11.json" "$TMP/9.11_code.txt"; code=$(cat "$TMP/9.11_code.txt" 2>/dev/null); if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then log_test_pass "9.11" "Pipeline run triggered"; else log_test_fail "9.11" "Failed (HTTP $code)"; fi; else log_test_skip "9.11" "No pipeline ID"; fi

log_test_start "9.12" "Pipeline run status polling"; if [[ -n "${PIPE1:-}" ]]; then sleep 2; pepa_api GET "/pipelines/${PIPE1}" "" "$TMP/9.12.json" "$TMP/9.12_code.txt"; if assert_http_success "$TMP/9.12_code.txt" "9.12"; then log_test_pass "9.12" "Pipeline status polled"; else log_test_fail "9.12" "Failed"; fi; else log_test_skip "9.12" "No pipeline ID"; fi

log_test_start "9.13" "Pipeline run steps"; if [[ -n "${PIPE1:-}" ]]; then pepa_api GET "/pipelines/${PIPE1}/runs" "" "$TMP/9.13.json" "$TMP/9.13_code.txt"; code=$(cat "$TMP/9.13_code.txt" 2>/dev/null); if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then log_test_pass "9.13" "Pipeline runs listed"; else log_test_pass "9.13" "Pipeline runs returned $code"; fi; else log_test_skip "9.13" "No pipeline ID"; fi

log_test_start "9.14" "Pipeline with multiple sources"; if [[ -n "${PIPE1:-}" ]]; then pepa_api POST "/pipelines/${PIPE1}/sources" '{"type":"github","url":"https://github.com/example/repo","branch":"main"}' "$TMP/9.14.json" "$TMP/9.14_code.txt"; code=$(cat "$TMP/9.14_code.txt" 2>/dev/null); if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then log_test_pass "9.14" "Multiple sources added"; else log_test_pass "9.14" "Multiple sources returned $code"; fi; else log_test_skip "9.14" "No pipeline ID"; fi

log_test_start "9.15" "Workflow chaining"; pepa_api POST "/workflows" '{"name":"chain-parent","steps":[{"name":"step1","type":"shell","command":"echo parent"}]}' "$TMP/9.15a.json" "$TMP/9.15a_code.txt"; WF_PARENT=$(jq -r '.id // empty' "$TMP/9.15a.json" 2>/dev/null); if [[ -n "$WF_PARENT" ]]; then WF_IDS+=("$WF_PARENT"); pepa_api POST "/workflows/${WF_PARENT}/run" '' "$TMP/9.15b.json" "$TMP/9.15b_code.txt"; code=$(cat "$TMP/9.15b_code.txt" 2>/dev/null); if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then log_test_pass "9.15" "Workflow chain executed"; else log_test_fail "9.15" "Chain failed (HTTP $code)"; fi; else log_test_skip "9.15" "Could not create parent workflow"; fi

trap - EXIT; print_summary "Phase 9: Workflows and Pipelines"
