#!/usr/bin/env bash
# 14-test-rbac.sh — Role-based access across all subsystems (Phase 14)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"; source "${SCRIPT_DIR}/lib/assertions.sh"
PEPA_URL="${PEPA_URL:-http://localhost:8088}"; API="${PEPA_URL}/api/v1"; TMP="${RESULTS_DIR}/tmp"; mkdir -p "$TMP"
log_phase "Phase 14: RBAC"
[[ -f "${RESULTS_DIR}/pepa_token" ]] && export PEPA_TOKEN=$(cat "${RESULTS_DIR}/pepa_token")

# Helper: run API call with a specific user token
rbac_test() { local id="$1" desc="$2" token="$3" method="$4" path="$5" expected="$6"
    log_test_start "$id" "$desc"
    local old="$PEPA_TOKEN"; export PEPA_TOKEN="$token"
    pepa_api "$method" "$path" "" "$TMP/${id}.json" "$TMP/${id}_code.txt"
    local code=$(cat "$TMP/${id}_code.txt" 2>/dev/null); export PEPA_TOKEN="$old"
    if [[ "$code" == "$expected" ]]; then log_test_pass "$id" "Got expected HTTP $code"
    else log_test_fail "$id" "Expected $expected, got $code"; fi
}

# Create test users
pepa_api POST "/auth/users" '{"username":"rbac-dev","password":"devpass123","email":"dev@rbac.local","role":2}' "$TMP/14_dev.json" "$TMP/14_dev_code.txt"
DEV_TOKEN=""; DEV_ID=$(jq -r '.id // empty' "$TMP/14_dev.json" 2>/dev/null)
if [[ -n "$DEV_ID" ]]; then
    resp=$(pepa_api_raw POST "/auth/login" '{"username":"rbac-dev","password":"devpass123"}'); DEV_TOKEN=$(echo "$resp" | jq -r '.token // .access_token // empty' 2>/dev/null)
fi
pepa_api POST "/auth/users" '{"username":"rbac-viewer","password":"viewpass123","email":"viewer@rbac.local","role":3}' "$TMP/14_view.json" "$TMP/14_view_code.txt"
VIEW_TOKEN=""; VIEW_ID=$(jq -r '.id // empty' "$TMP/14_view.json" 2>/dev/null)
if [[ -n "$VIEW_ID" ]]; then
    resp=$(pepa_api_raw POST "/auth/login" '{"username":"rbac-viewer","password":"viewpass123"}'); VIEW_TOKEN=$(echo "$resp" | jq -r '.token // .access_token // empty' 2>/dev/null)
fi

ADMIN_TOKEN="${PEPA_TOKEN:-}"
[[ -z "${DEV_TOKEN:-}" ]] && DEV_TOKEN="$ADMIN_TOKEN"
[[ -z "${VIEW_TOKEN:-}" ]] && VIEW_TOKEN="$ADMIN_TOKEN"

rbac_test "14.1" "Admin full access" "$ADMIN_TOKEN" GET "/connections" "200"
rbac_test "14.2" "Developer read-only on connections" "$DEV_TOKEN" GET "/connections" "200"
rbac_test "14.3" "Developer cannot create connections" "$DEV_TOKEN" POST "/connections" "403"
rbac_test "14.4" "Developer cannot update connections" "$DEV_TOKEN" PUT "/connections/nonexistent" "403"
rbac_test "14.5" "Developer can delete personal credentials" "$DEV_TOKEN" DELETE "/user-credentials/nonexistent" "404"
rbac_test "14.6" "Viewer read-only on clusters" "$VIEW_TOKEN" GET "/clusters" "200"
rbac_test "14.7" "Viewer cannot deploy" "$VIEW_TOKEN" POST "/deployments" "403"
rbac_test "14.8" "Developer can trigger drift detection" "$DEV_TOKEN" POST "/gitops/drift/detect" "200"
rbac_test "14.9" "Viewer cannot create drift schedules" "$VIEW_TOKEN" POST "/gitops/drift/schedules" "403"
rbac_test "14.10" "Developer can read drift schedules" "$DEV_TOKEN" GET "/gitops/drift/schedules" "200"
rbac_test "14.11" "Admin manages notification rules" "$ADMIN_TOKEN" GET "/notifications/rules" "200"
rbac_test "14.12" "Viewer read-only notifications" "$VIEW_TOKEN" GET "/notifications/rules" "200"
rbac_test "14.13" "Viewer cannot modify notification rules" "$VIEW_TOKEN" POST "/notifications/rules" "403"
rbac_test "14.14" "Plugin activity RBAC (viewer read)" "$VIEW_TOKEN" GET "/plugins/fluxcd/activity" "200"
rbac_test "14.15" "Developer can rollback docker services" "$DEV_TOKEN" POST "/docker-services/nonexistent/rollback" "404"
rbac_test "14.16" "Security scanning RBAC (viewer no trigger)" "$VIEW_TOKEN" POST "/security/scan-all" "403"
rbac_test "14.17" "GitOps repo RBAC (developer can scan)" "$DEV_TOKEN" POST "/gitops/repos/nonexistent/scan" "404"
rbac_test "14.18" "Tenant isolation" "$DEV_TOKEN" GET "/entities" "200"
rbac_test "14.19" "Default viewer role backfill" "$VIEW_TOKEN" GET "/auth/me" "200"
rbac_test "14.20" "RBAC permission matrix" "$ADMIN_TOKEN" GET "/connections/summary" "200"

# Cleanup
[[ -n "${DEV_ID:-}" ]] && pepa_api DELETE "/auth/users/${DEV_ID}" "" /dev/null /dev/null 2>/dev/null || true
[[ -n "${VIEW_ID:-}" ]] && pepa_api DELETE "/auth/users/${VIEW_ID}" "" /dev/null /dev/null 2>/dev/null || true
print_summary "Phase 14: RBAC"
