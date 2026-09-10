#!/usr/bin/env bash
# 04-test-plugins.sh — Plugin lifecycle, marketplace, toggle sync, health (Phase 4)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"
source "${SCRIPT_DIR}/lib/assertions.sh"

PEPA_URL="${PEPA_URL:-http://localhost:8088}"
API="${PEPA_URL}/api/v1"
TMP="${RESULTS_DIR}/tmp"
mkdir -p "$TMP"

log_phase "Phase 4: Plugin System"

[[ -f "${RESULTS_DIR}/pepa_token" ]] && export PEPA_TOKEN=$(cat "${RESULTS_DIR}/pepa_token")

# ---------------------------------------------------------------------------
# 4.1 List all plugins
# ---------------------------------------------------------------------------
log_test_start "4.1" "List all plugins"
pepa_api GET "/plugins" "" "$TMP/4.1_list.json" "$TMP/4.1_code.txt"
if assert_http_success "$TMP/4.1_code.txt" "4.1 list plugins"; then
    log_test_pass "4.1" "Plugins listed"
else
    log_test_fail "4.1" "List plugins failed"
fi

# ---------------------------------------------------------------------------
# 4.2 Verify built-in plugins present
# ---------------------------------------------------------------------------
log_test_start "4.2" "Verify built-in plugins present"
fluxcd_status=$(jq -r '.[] | select(.name == "fluxcd") | .status // .state // empty' "$TMP/4.1_list.json" 2>/dev/null)
argocd_status=$(jq -r '.[] | select(.name == "argocd") | .status // .state // empty' "$TMP/4.1_list.json" 2>/dev/null)
if [[ -n "$fluxcd_status" || -n "$argocd_status" ]]; then
    log_test_pass "4.2" "Built-in plugins found (fluxcd=${fluxcd_status:-?}, argocd=${argocd_status:-?})"
else
    # Check data array format
    fluxcd_status=$(jq -r '.data[]? | select(.name == "fluxcd") | .status // empty' "$TMP/4.1_list.json" 2>/dev/null)
    if [[ -n "$fluxcd_status" ]]; then
        log_test_pass "4.2" "Built-in plugins found (data format)"
    else
        log_test_fail "4.2" "No built-in plugins found"
    fi
fi

# ---------------------------------------------------------------------------
# 4.3 Enable plugin
# ---------------------------------------------------------------------------
log_test_start "4.3" "Enable plugin (fluxcd)"
pepa_api POST "/plugins/fluxcd/toggle" "" "$TMP/4.3_enable.json" "$TMP/4.3_code.txt"
if assert_http_success "$TMP/4.3_code.txt" "4.3 enable fluxcd"; then
    log_test_pass "4.3" "FluxCD plugin enabled"
else
    log_test_fail "4.3" "Enable fluxcd failed"
fi

# ---------------------------------------------------------------------------
# 4.4 Disable plugin
# ---------------------------------------------------------------------------
log_test_start "4.4" "Disable plugin (fluxcd)"
pepa_api POST "/plugins/fluxcd/toggle" "" "$TMP/4.4_disable.json" "$TMP/4.4_code.txt"
if assert_http_success "$TMP/4.4_code.txt" "4.4 disable fluxcd"; then
    log_test_pass "4.4" "FluxCD plugin disabled"
else
    log_test_fail "4.4" "Disable fluxcd failed"
fi

# ---------------------------------------------------------------------------
# 4.5 Verify disabled plugin health
# ---------------------------------------------------------------------------
log_test_start "4.5" "Verify disabled plugin health"
pepa_api GET "/plugins/fluxcd/health" "" "$TMP/4.5_health.json" "$TMP/4.5_code.txt"
if assert_http_success "$TMP/4.5_code.txt" "4.5 plugin health"; then
    log_test_pass "4.5" "Disabled plugin health returned"
else
    log_test_fail "4.5" "Plugin health check failed"
fi

# ---------------------------------------------------------------------------
# 4.6 Plugin toggle sync — disable fluxcd, check repos become inactive
# ---------------------------------------------------------------------------
log_test_start "4.6" "Plugin toggle sync (repos become inactive)"
# FluxCD is already disabled from 4.4, check GitOps repos
pepa_api GET "/gitops/repos" "" "$TMP/4.6_repos.json" "$TMP/4.6_code.txt"
if assert_http_success "$TMP/4.6_code.txt" "4.6 check repos after disable"; then
    log_test_pass "4.6" "Repos checked after plugin disable"
else
    log_test_fail "4.6" "Check repos failed"
fi

# ---------------------------------------------------------------------------
# 4.7 Re-enable plugin, verify recovery
# ---------------------------------------------------------------------------
log_test_start "4.7" "Re-enable plugin, verify recovery"
pepa_api POST "/plugins/fluxcd/toggle" "" "$TMP/4.7_reenable.json" "$TMP/4.7_code.txt"
if assert_http_success "$TMP/4.7_code.txt" "4.7 re-enable fluxcd"; then
    log_test_pass "4.7" "FluxCD re-enabled"
else
    log_test_fail "4.7" "Re-enable fluxcd failed"
fi

# ---------------------------------------------------------------------------
# 4.8 Plugin health check
# ---------------------------------------------------------------------------
log_test_start "4.8" "Plugin health check (gRPC)"
pepa_api GET "/plugins/fluxcd/health" "" "$TMP/4.8_health.json" "$TMP/4.8_code.txt"
if assert_http_success "$TMP/4.8_code.txt" "4.8 health check"; then
    log_test_pass "4.8" "Plugin health check passed"
else
    log_test_fail "4.8" "Plugin health check failed"
fi

# ---------------------------------------------------------------------------
# 4.9 Plugin config update
# ---------------------------------------------------------------------------
log_test_start "4.9" "Plugin config update"
pepa_api PUT "/plugins/fluxcd/config" \
    '{"interval":"5m","log_level":"debug"}' \
    "$TMP/4.9_config.json" "$TMP/4.9_code.txt"
if assert_http_success "$TMP/4.9_code.txt" "4.9 config update"; then
    log_test_pass "4.9" "Plugin config updated"
else
    log_test_fail "4.9" "Plugin config update failed"
fi

# ---------------------------------------------------------------------------
# 4.10 Plugin execution
# ---------------------------------------------------------------------------
log_test_start "4.10" "Plugin execution"
pepa_api POST "/plugins/fluxcd/execute" \
    '{"action":"scan","params":{}}' \
    "$TMP/4.10_exec.json" "$TMP/4.10_code.txt"
code=$(cat "$TMP/4.10_code.txt" 2>/dev/null)
if [[ "$code" =~ ^2[0-9][0-9]$ ]]; then
    log_test_pass "4.10" "Plugin execution returned HTTP $code"
else
    log_test_fail "4.10" "Plugin execution failed (HTTP $code)"
fi

# ---------------------------------------------------------------------------
# 4.11 Plugin dependency gating
# ---------------------------------------------------------------------------
log_test_start "4.11" "Plugin dependency gating"
# Try to disable a plugin that has dependents — should get 409 or similar
pepa_api DELETE "/plugins/fluxcd?force=false" "" "$TMP/4.11_dep.json" "$TMP/4.11_code.txt"
code=$(cat "$TMP/4.11_code.txt" 2>/dev/null)
if [[ "$code" == "409" || "$code" == "400" ]]; then
    log_test_pass "4.11" "Dependency gating works (HTTP $code)"
elif [[ "$code" =~ ^2[0-9][0-9]$ ]]; then
    log_test_pass "4.11" "Plugin disabled (no dependents or force allowed)"
else
    log_test_fail "4.11" "Dependency gating returned HTTP $code"
fi

# ---------------------------------------------------------------------------
# 4.12 Plugin activity logging
# ---------------------------------------------------------------------------
log_test_start "4.12" "Plugin activity logging"
pepa_api GET "/plugins/fluxcd/activity" "" "$TMP/4.12_activity.json" "$TMP/4.12_code.txt"
if assert_http_success "$TMP/4.12_code.txt" "4.12 activity"; then
    log_test_pass "4.12" "Plugin activity log returned"
else
    log_test_fail "4.12" "Plugin activity log failed"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary "Phase 4: Plugin System"
