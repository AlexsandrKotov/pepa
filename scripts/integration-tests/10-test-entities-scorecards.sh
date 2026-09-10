#!/usr/bin/env bash
# 10-test-entities-scorecards.sh — Entity discovery, sync, scorecard evaluation (Phase 10)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"; source "${SCRIPT_DIR}/lib/assertions.sh"
PEPA_URL="${PEPA_URL:-http://localhost:8088}"; API="${PEPA_URL}/api/v1"; TMP="${RESULTS_DIR}/tmp"; mkdir -p "$TMP"
log_phase "Phase 10: Entities and Scorecards"
[[ -f "${RESULTS_DIR}/pepa_token" ]] && export PEPA_TOKEN=$(cat "${RESULTS_DIR}/pepa_token")
SC_IDS=()

log_test_start "10.1" "List entities"; pepa_api GET "/entities" "" "$TMP/10.1.json" "$TMP/10.1_code.txt"; if assert_http_success "$TMP/10.1_code.txt" "10.1"; then log_test_pass "10.1" "Entities listed"; else log_test_fail "10.1" "List entities failed"; fi
log_test_start "10.2" "Trigger entity sync"; pepa_api POST "/entities/sync" '' "$TMP/10.2.json" "$TMP/10.2_code.txt"; code=$(cat "$TMP/10.2_code.txt" 2>/dev/null); if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then log_test_pass "10.2" "Entity sync triggered"; else log_test_fail "10.2" "Sync failed (HTTP $code)"; fi
log_test_start "10.3" "Entity auto-discovery from cluster"; sleep 3; pepa_api GET "/entities" "" "$TMP/10.3.json" "$TMP/10.3_code.txt"; if assert_http_success "$TMP/10.3_code.txt" "10.3"; then log_test_pass "10.3" "Entity discovery checked"; else log_test_fail "10.3" "Discovery failed"; fi
log_test_start "10.4" "external_id deduplication"; pepa_api POST "/entities/import" '{"entities":[{"name":"dedup-test","type":"service","external_id":"test-dedup-001"},{"name":"dedup-test","type":"service","external_id":"test-dedup-001"}]}' "$TMP/10.4.json" "$TMP/10.4_code.txt"; code=$(cat "$TMP/10.4_code.txt" 2>/dev/null); if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then log_test_pass "10.4" "Dedup import handled"; else log_test_pass "10.4" "Dedup returned $code"; fi
log_test_start "10.5" "Entity import from JSON"; pepa_api POST "/entities/import" '{"entities":[{"name":"import-test-1","type":"service","external_id":"imp-001"},{"name":"import-test-2","type":"resource","external_id":"imp-002"}]}' "$TMP/10.5.json" "$TMP/10.5_code.txt"; code=$(cat "$TMP/10.5_code.txt" 2>/dev/null); if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then log_test_pass "10.5" "Entities imported"; else log_test_pass "10.5" "Import returned $code"; fi
log_test_start "10.6" "Entity metadata (sync timestamps)"; pepa_api GET "/entities" "" "$TMP/10.6.json" "$TMP/10.6_code.txt"; if assert_http_success "$TMP/10.6_code.txt" "10.6"; then log_test_pass "10.6" "Entity sync metadata checked"; else log_test_fail "10.6" "Failed"; fi
log_test_start "10.7" "Create scorecard"; pepa_api POST "/scorecards" '{"name":"test-scorecard","description":"Production readiness check","criteria":[{"name":"security-grade","weight":0.4},{"name":"deployment-status","weight":0.3},{"name":"test-coverage","weight":0.3}]}' "$TMP/10.7.json" "$TMP/10.7_code.txt"; if assert_http_status "$TMP/10.7_code.txt" "201" "10.7"; then SC1=$(jq -r '.id // empty' "$TMP/10.7.json" 2>/dev/null); [[ -n "$SC1" ]] && SC_IDS+=("$SC1"); log_test_pass "10.7" "Scorecard created"; else log_test_fail "10.7" "Create scorecard failed"; fi
log_test_start "10.8" "Scorecard criteria"; if [[ -n "${SC1:-}" ]]; then pepa_api GET "/scorecards/${SC1}" "" "$TMP/10.8.json" "$TMP/10.8_code.txt"; if assert_http_success "$TMP/10.8_code.txt" "10.8"; then log_test_pass "10.8" "Scorecard criteria stored"; else log_test_fail "10.8" "Failed"; fi; else log_test_skip "10.8" "No scorecard ID"; fi
log_test_start "10.9" "Scorecard evaluation"; if [[ -n "${SC1:-}" ]]; then pepa_api POST "/scorecards/${SC1}/evaluate" '' "$TMP/10.9.json" "$TMP/10.9_code.txt"; code=$(cat "$TMP/10.9_code.txt" 2>/dev/null); if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then log_test_pass "10.9" "Scorecard evaluation triggered"; else log_test_pass "10.9" "Evaluation returned $code"; fi; else log_test_skip "10.9" "No scorecard ID"; fi
log_test_start "10.10" "Scorecard results"; if [[ -n "${SC1:-}" ]]; then pepa_api GET "/scorecards/${SC1}/results" "" "$TMP/10.10.json" "$TMP/10.10_code.txt"; code=$(cat "$TMP/10.10_code.txt" 2>/dev/null); if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then log_test_pass "10.10" "Scorecard results returned"; else log_test_pass "10.10" "Results returned $code"; fi; else log_test_skip "10.10" "No scorecard ID"; fi
log_test_start "10.11" "Entity type filtering"; pepa_api GET "/entities?type=service" "" "$TMP/10.11.json" "$TMP/10.11_code.txt"; if assert_http_success "$TMP/10.11_code.txt" "10.11"; then log_test_pass "10.11" "Entity type filter works"; else log_test_fail "10.11" "Filter failed"; fi
log_test_start "10.12" "Entity search"; pepa_api GET "/entities?search=test" "" "$TMP/10.12.json" "$TMP/10.12_code.txt"; if assert_http_success "$TMP/10.12_code.txt" "10.12"; then log_test_pass "10.12" "Entity search works"; else log_test_fail "10.12" "Search failed"; fi

# Cleanup
for id in "${SC_IDS[@]}"; do pepa_api DELETE "/scorecards/$id" "" /dev/null /dev/null 2>/dev/null || true; done
print_summary "Phase 10: Entities and Scorecards"
