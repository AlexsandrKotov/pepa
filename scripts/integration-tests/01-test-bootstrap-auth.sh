#!/usr/bin/env bash
# 01-test-bootstrap-auth.sh — Bootstrap and authentication tests (Phase 1)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"
source "${SCRIPT_DIR}/lib/assertions.sh"

PEPA_URL="${PEPA_URL:-http://localhost:8088}"
API="${PEPA_URL}/api/v1"
TMP="${RESULTS_DIR}/tmp"
mkdir -p "$TMP"

log_phase "Phase 1: Bootstrap and Authentication"

# ---------------------------------------------------------------------------
# Test state tracking
# ---------------------------------------------------------------------------
BOOTSTRAP_TOKEN=""
ADMIN_JWT=""
TEST_USER_ID=""

# ---------------------------------------------------------------------------
# 1.1 Check bootstrap status
# ---------------------------------------------------------------------------
log_test_start "1.1" "Check bootstrap status"
resp=$(pepa_api GET "/auth/bootstrap/status" "" "$TMP/1.1_status.json" "$TMP/1.1_code.txt" true)
if assert_http_success "$TMP/1.1_code.txt" "1.1 bootstrap status endpoint"; then
    log_test_pass "1.1" "Bootstrap status returned"
    # Save bootstrap token if available
    BOOTSTRAP_TOKEN=$(jq -r '.token // .bootstrap_token // empty' "$TMP/1.1_status.json" 2>/dev/null)
else
    log_test_fail "1.1" "Bootstrap status check failed"
fi

# ---------------------------------------------------------------------------
# 1.2 Activate bootstrap token (only if bootstrap is needed)
# ---------------------------------------------------------------------------
log_test_start "1.2" "Activate bootstrap token"
needs_bootstrap=$(jq -r '.needs_bootstrap // .needsBootstrap // false' "$TMP/1.1_status.json" 2>/dev/null)
if [[ "$needs_bootstrap" == "true" && -n "$BOOTSTRAP_TOKEN" ]]; then
    pepa_api POST "/auth/bootstrap/activate" \
        "{\"token\":\"${BOOTSTRAP_TOKEN}\",\"username\":\"admin\",\"password\":\"Admin123!\",\"email\":\"admin@local\"}" \
        "$TMP/1.2_activate.json" "$TMP/1.2_code.txt"
    if assert_http_success "$TMP/1.2_code.txt" "1.2 bootstrap activate"; then
        log_test_pass "1.2" "Bootstrap activated"
    else
        log_test_fail "1.2" "Bootstrap activation failed"
    fi
else
    log_test_skip "1.2" "Bootstrap already completed"
fi

# ---------------------------------------------------------------------------
# 1.3 Login with initial admin
# ---------------------------------------------------------------------------
log_test_start "1.3" "Login with initial admin"
pepa_api POST "/auth/login" \
    '{"email":"admin@local","password":"Admin123!"}' \
    "$TMP/1.3_login.json" "$TMP/1.3_code.txt"
if assert_http_success "$TMP/1.3_code.txt" "1.3 login"; then
    ADMIN_JWT=$(jq -r '.token // .access_token // .jwt // empty' "$TMP/1.3_login.json" 2>/dev/null)
    if [[ -n "$ADMIN_JWT" ]]; then
        export PEPA_TOKEN="$ADMIN_JWT"
        echo "$PEPA_TOKEN" > "${RESULTS_DIR}/pepa_token"
        log_test_pass "1.3" "Login successful, JWT acquired"
    else
        log_test_fail "1.3" "Login returned 200 but no JWT found"
    fi
else
    log_test_fail "1.3" "Login failed"
fi

# ---------------------------------------------------------------------------
# 1.4 Get current user
# ---------------------------------------------------------------------------
log_test_start "1.4" "Get current user (GET /auth/me)"
pepa_api GET "/auth/me" "" "$TMP/1.4_me.json" "$TMP/1.4_code.txt"
if assert_http_success "$TMP/1.4_code.txt" "1.4 get current user"; then
    assert_json_field "$TMP/1.4_me.json" ".user.email" "admin@local" "1.4 email=admin@local" && \
        log_test_pass "1.4" "Current user is admin" || \
        log_test_fail "1.4" "User email mismatch"
else
    log_test_fail "1.4" "GET /auth/me failed"
fi

# ---------------------------------------------------------------------------
# 1.5 Refresh token
# ---------------------------------------------------------------------------
log_test_start "1.5" "Refresh token (POST /auth/refresh)"
pepa_api POST "/auth/refresh" "" "$TMP/1.5_refresh.json" "$TMP/1.5_code.txt"
if assert_http_success "$TMP/1.5_code.txt" "1.5 refresh token"; then
    new_jwt=$(jq -r '.token // .access_token // empty' "$TMP/1.5_refresh.json" 2>/dev/null)
    if [[ -n "$new_jwt" ]]; then
        export PEPA_TOKEN="$new_jwt"
        ADMIN_JWT="$new_jwt"
        log_test_pass "1.5" "Token refreshed"
    else
        log_test_pass "1.5" "Refresh returned 200"
    fi
else
    log_test_fail "1.5" "Token refresh failed"
fi

# ---------------------------------------------------------------------------
# 1.6 Reset own password
# ---------------------------------------------------------------------------
log_test_start "1.6" "Reset own password (POST /auth/me/reset-password)"
pepa_api POST "/auth/me/reset-password" \
    '{"current_password":"Admin123!","new_password":"NewPass456!"}' \
    "$TMP/1.6_resetpw.json" "$TMP/1.6_code.txt"
if assert_http_success "$TMP/1.6_code.txt" "1.6 reset password"; then
    log_test_pass "1.6" "Password reset successful"
else
    log_test_fail "1.6" "Password reset failed"
fi

# ---------------------------------------------------------------------------
# 1.7 Login after password reset
# ---------------------------------------------------------------------------
log_test_start "1.7" "Login after password reset"
pepa_api POST "/auth/login" \
    '{"email":"admin@local","password":"NewPass456!"}' \
    "$TMP/1.7_login2.json" "$TMP/1.7_code.txt"
if assert_http_success "$TMP/1.7_code.txt" "1.7 login with new password"; then
    ADMIN_JWT=$(jq -r '.token // .access_token // .jwt // empty' "$TMP/1.7_login2.json" 2>/dev/null)
    export PEPA_TOKEN="$ADMIN_JWT"
    log_test_pass "1.7" "Login with new password successful"
else
    log_test_fail "1.7" "Login with new password failed"
    # Reset password back for subsequent tests
    pepa_api POST "/auth/me/reset-password" \
        '{"current_password":"NewPass456!","new_password":"Admin123!"}' \
        "$TMP/1.7_resetback.json" "$TMP/1.7_resetback_code.txt" 2>/dev/null
fi

# ---------------------------------------------------------------------------
# 1.8 Admin: create user
# ---------------------------------------------------------------------------
log_test_start "1.8" "Admin: create user (POST /auth/users)"
pepa_api POST "/auth/users" \
    '{"name":"Test Developer","password":"TestDev123!","email":"testdev@pepa.local"}' \
    "$TMP/1.8_create_user.json" "$TMP/1.8_code.txt"
if assert_http_status "$TMP/1.8_code.txt" "201" "1.8 create user"; then
    TEST_USER_ID=$(jq -r '.id // .user_id // empty' "$TMP/1.8_create_user.json" 2>/dev/null)
    log_test_pass "1.8" "User created (id=${TEST_USER_ID:-unknown})"
else
    log_test_fail "1.8" "User creation failed"
fi

# ---------------------------------------------------------------------------
# 1.9 Admin: list users
# ---------------------------------------------------------------------------
log_test_start "1.9" "Admin: list users (GET /auth/users)"
pepa_api GET "/auth/users" "" "$TMP/1.9_users.json" "$TMP/1.9_code.txt"
if assert_http_success "$TMP/1.9_code.txt" "1.9 list users"; then
    assert_json_field_exists "$TMP/1.9_users.json" ".users[0].id" "1.9 has users" && \
        log_test_pass "1.9" "Users listed" || \
        log_test_fail "1.9" "No users in response"
else
    log_test_fail "1.9" "List users failed"
fi

# ---------------------------------------------------------------------------
# 1.10 Admin: get user by ID
# ---------------------------------------------------------------------------
log_test_start "1.10" "Admin: get user by ID"
if [[ -n "$TEST_USER_ID" ]]; then
    pepa_api GET "/auth/users/${TEST_USER_ID}" "" "$TMP/1.10_user.json" "$TMP/1.10_code.txt"
    if assert_http_success "$TMP/1.10_code.txt" "1.10 get user"; then
        log_test_pass "1.10" "User retrieved by ID"
    else
        log_test_fail "1.10" "Get user by ID failed"
    fi
else
    log_test_skip "1.10" "No test user ID available"
fi

# ---------------------------------------------------------------------------
# 1.11 Admin: update user
# ---------------------------------------------------------------------------
log_test_start "1.11" "Admin: update user (PUT /auth/users/:id)"
if [[ -n "$TEST_USER_ID" ]]; then
    pepa_api PUT "/auth/users/${TEST_USER_ID}" \
        '{"email":"dev-updated@pepa.local"}' \
        "$TMP/1.11_update.json" "$TMP/1.11_code.txt"
    if assert_http_success "$TMP/1.11_code.txt" "1.11 update user"; then
        log_test_pass "1.11" "User updated"
    else
        log_test_fail "1.11" "User update failed"
    fi
else
    log_test_skip "1.11" "No test user ID available"
fi

# ---------------------------------------------------------------------------
# 1.12 Admin: reset user password
# ---------------------------------------------------------------------------
log_test_start "1.12" "Admin: reset user password"
if [[ -n "$TEST_USER_ID" ]]; then
    pepa_api POST "/auth/users/${TEST_USER_ID}/reset-password" \
        '{"new_password":"ResetPass789!"}' \
        "$TMP/1.12_resetpw.json" "$TMP/1.12_code.txt"
    if assert_http_success "$TMP/1.12_code.txt" "1.12 reset user password"; then
        log_test_pass "1.12" "User password reset"
    else
        log_test_fail "1.12" "User password reset failed"
    fi
else
    log_test_skip "1.12" "No test user ID available"
fi

# ---------------------------------------------------------------------------
# 1.13 Admin: deactivate user
# ---------------------------------------------------------------------------
log_test_start "1.13" "Admin: deactivate user (DELETE /auth/users/:id)"
if [[ -n "$TEST_USER_ID" ]]; then
    pepa_api DELETE "/auth/users/${TEST_USER_ID}" "" "$TMP/1.13_deactivate.json" "$TMP/1.13_code.txt"
    if assert_http_success "$TMP/1.13_code.txt" "1.13 deactivate user"; then
        log_test_pass "1.13" "User deactivated"
    else
        log_test_fail "1.13" "User deactivation failed"
    fi
else
    log_test_skip "1.13" "No test user ID available"
fi

# ---------------------------------------------------------------------------
# 1.14 Expired JWT rejected
# ---------------------------------------------------------------------------
log_test_start "1.14" "Expired JWT rejected"
# Create a fake expired JWT (just a garbage token for testing 401)
OLD_TOKEN="$PEPA_TOKEN"
export PEPA_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjF9.expired"
pepa_api GET "/auth/me" "" "$TMP/1.14_expired.json" "$TMP/1.14_code.txt"
if assert_http_error "$TMP/1.14_code.txt" "401" "1.14 expired JWT"; then
    log_test_pass "1.14" "Expired JWT correctly rejected"
else
    log_test_fail "1.14" "Expired JWT was not rejected"
fi
export PEPA_TOKEN="$OLD_TOKEN"

# ---------------------------------------------------------------------------
# 1.15 Invalid JWT rejected
# ---------------------------------------------------------------------------
log_test_start "1.15" "Invalid JWT rejected"
OLD_TOKEN="$PEPA_TOKEN"
export PEPA_TOKEN="this-is-not-a-valid-jwt-token"
pepa_api GET "/auth/me" "" "$TMP/1.15_invalid.json" "$TMP/1.15_code.txt"
if assert_http_error "$TMP/1.15_code.txt" "401" "1.15 invalid JWT"; then
    log_test_pass "1.15" "Invalid JWT correctly rejected"
else
    log_test_fail "1.15" "Invalid JWT was not rejected"
fi
export PEPA_TOKEN="$OLD_TOKEN"

# ---------------------------------------------------------------------------
# 1.16 Logout
# ---------------------------------------------------------------------------
log_test_start "1.16" "Logout (POST /auth/logout)"
pepa_api POST "/auth/logout" "" "$TMP/1.16_logout.json" "$TMP/1.16_code.txt"
if assert_http_success "$TMP/1.16_code.txt" "1.16 logout"; then
    log_test_pass "1.16" "Logout successful"
else
    log_test_fail "1.16" "Logout failed"
fi

# Re-login for subsequent tests
pepa_login "admin@local" "Admin123!" 2>/dev/null || \
pepa_login "admin@local" "NewPass456!" 2>/dev/null || \
    log_warn "Could not re-login after logout test"

# ---------------------------------------------------------------------------
# 1.17 Bootstrap single-use
# ---------------------------------------------------------------------------
log_test_start "1.17" "Bootstrap single-use (re-activate should fail)"
if [[ -n "$BOOTSTRAP_TOKEN" ]]; then
    pepa_api POST "/auth/bootstrap/activate" \
        "{\"token\":\"${BOOTSTRAP_TOKEN}\",\"username\":\"admin2\",\"password\":\"pass\",\"email\":\"a@b.c\"}" \
        "$TMP/1.17_reactivate.json" "$TMP/1.17_code.txt"
    code=$(cat "$TMP/1.17_code.txt" 2>/dev/null)
    if [[ "$code" == "403" || "$code" == "400" || "$code" == "409" ]]; then
        log_test_pass "1.17" "Bootstrap correctly rejected (HTTP $code)"
    else
        log_test_fail "1.17" "Bootstrap should be rejected but got HTTP $code"
    fi
else
    log_test_skip "1.17" "No bootstrap token available"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary "Phase 1: Bootstrap and Authentication"
