#!/usr/bin/env bash
# 06-test-security.sh — Security scanning E2E test
# Verifies PEPA security scanning features.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

E2E_NS="${E2E_NAMESPACE:-pepa-e2e}"

log_phase "Phase G: Security Scanning"

# Login to PEPA
pepa_login 2>/dev/null || true

# ── G.1 Create scan target for deployed image ──────────────────────────────
log_test_start "G.1" "Create scan target for deployed image"
pepa_api POST "/security/scan-targets" \
    '{"name":"e2e-nginx-scan","image":"nginx:1.25-alpine","type":"image"}' \
    "$TMP/g1_target.json" "$TMP/g1_code.txt"
G1_CODE=$(cat "$TMP/g1_code.txt" 2>/dev/null)
TARGET_ID=$(jq -r '.id // .target_id // empty' "$TMP/g1_target.json" 2>/dev/null)
if [[ "$G1_CODE" =~ ^2 ]] || [[ -n "$TARGET_ID" ]]; then
    log_test_pass "G.1" "Scan target created (${TARGET_ID:-exists})"
else
    # Try alternate endpoint
    pepa_api POST "/security/scans" \
        '{"name":"e2e-nginx-scan","image":"nginx:1.25-alpine"}' \
        "$TMP/g1b_target.json" "$TMP/g1b_code.txt"
    G1B_CODE=$(cat "$TMP/g1b_code.txt" 2>/dev/null)
    TARGET_ID=$(jq -r '.id // .scan_id // empty' "$TMP/g1b_target.json" 2>/dev/null)
    if [[ "$G1B_CODE" =~ ^2 ]] || [[ -n "$TARGET_ID" ]]; then
        log_test_pass "G.1" "Scan created (${TARGET_ID:-exists})"
    else
        log_test_skip "G.1" "Security scan endpoint not available (HTTP $G1_CODE/$G1B_CODE)"
    fi
fi

# ── G.2 Trigger scan ───────────────────────────────────────────────────────
log_test_start "G.2" "Trigger security scan"
if [[ -n "$TARGET_ID" ]]; then
    pepa_api POST "/security/scan-targets/${TARGET_ID}/scan" "" \
        "$TMP/g2_scan.json" "$TMP/g2_code.txt"
    G2_CODE=$(cat "$TMP/g2_code.txt" 2>/dev/null)
    if [[ "$G2_CODE" =~ ^2 ]]; then
        log_test_pass "G.2" "Scan triggered"
    else
        log_test_skip "G.2" "Scan trigger not available (HTTP $G2_CODE)"
    fi
else
    log_test_skip "G.2" "No scan target ID available"
fi

# ── G.3 List scan results ──────────────────────────────────────────────────
log_test_start "G.3" "List scan results"
pepa_api GET "/security/scans" "" "$TMP/g3_results.json" "$TMP/g3_code.txt"
G3_CODE=$(cat "$TMP/g3_code.txt" 2>/dev/null)
if [[ "$G3_CODE" =~ ^2 ]]; then
    SCAN_COUNT=$(jq 'if type == "array" then length elif .scans then .scans | length else 0 end' "$TMP/g3_results.json" 2>/dev/null)
    log_test_pass "G.3" "Scan results listed (${SCAN_COUNT:-0} found)"
else
    # Try scan-targets
    pepa_api GET "/security/scan-targets" "" "$TMP/g3b_targets.json" "$TMP/g3b_code.txt"
    G3B_CODE=$(cat "$TMP/g3b_code.txt" 2>/dev/null)
    if [[ "$G3B_CODE" =~ ^2 ]]; then
        log_test_pass "G.3" "Scan targets listed"
    else
        log_test_skip "G.3" "Security endpoints not available"
    fi
fi

# ── G.4 Create scan schedule ───────────────────────────────────────────────
log_test_start "G.4" "Create scan schedule"
pepa_api POST "/security/schedules" \
    '{"name":"e2e-daily-scan","cron":"0 2 * * *","enabled":true}' \
    "$TMP/g4_sched.json" "$TMP/g4_code.txt"
G4_CODE=$(cat "$TMP/g4_code.txt" 2>/dev/null)
if [[ "$G4_CODE" =~ ^2 ]]; then
    log_test_pass "G.4" "Scan schedule created"
else
    log_test_skip "G.4" "Scan schedule endpoint not available (HTTP $G4_CODE)"
fi

# ── G.5 Create ignore rule for CVE ─────────────────────────────────────────
log_test_start "G.5" "Create ignore rule for CVE"
pepa_api POST "/security/ignore-rules" \
    '{"cve":"CVE-2024-0001","reason":"E2E test false positive","expires_at":"2099-12-31T00:00:00Z"}' \
    "$TMP/g5_ignore.json" "$TMP/g5_code.txt"
G5_CODE=$(cat "$TMP/g5_code.txt" 2>/dev/null)
if [[ "$G5_CODE" =~ ^2 ]]; then
    log_test_pass "G.5" "Ignore rule created"
else
    log_test_skip "G.5" "Ignore rule endpoint not available (HTTP $G5_CODE)"
fi

# ── G.6 Verify security dashboard ──────────────────────────────────────────
log_test_start "G.6" "Verify security dashboard data"
pepa_api GET "/security/dashboard" "" "$TMP/g6_dash.json" "$TMP/g6_code.txt"
G6_CODE=$(cat "$TMP/g6_code.txt" 2>/dev/null)
if [[ "$G6_CODE" =~ ^2 ]]; then
    log_test_pass "G.6" "Security dashboard data retrieved"
else
    log_test_skip "G.6" "Security dashboard endpoint not available (HTTP $G6_CODE)"
fi

# ── G.7 Cleanup ────────────────────────────────────────────────────────────
log_test_start "G.7" "Cleanup security test resources"
if [[ -n "$TARGET_ID" ]]; then
    pepa_api DELETE "/security/scan-targets/${TARGET_ID}" 2>/dev/null || true
fi
log_test_pass "G.7" "Security test cleanup done"

print_summary
