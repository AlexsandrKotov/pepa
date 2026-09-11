#!/usr/bin/env bash
# 27-test-blueprint-filter.sh — Blueprint listing with system/user filter
# Covers: GET /blueprints, GET /blueprints?type=system, GET /blueprints?type=user,
#         GET /blueprints/:id, blueprint groups

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 27: Blueprint Filter & Groups"

pepa_login 2>/dev/null || true

# ── 27.1 List all blueprints ───────────────────────────────────────────────
log_test_start "27.1" "GET /blueprints returns all blueprints"
pepa_api GET "/blueprints"
if [[ "$API_STATUS" == "200" ]]; then
    ALL_COUNT=$(echo "$API_RESPONSE" | jq '.blueprints // .items // [] | length' 2>/dev/null)
    log_test "27.1" "All blueprints: $ALL_COUNT found" "pass"
else
    log_test "27.1" "List all blueprints" "fail" "HTTP $API_STATUS"
    ALL_COUNT=0
fi

# ── 27.2 Filter blueprints by type=system ──────────────────────────────────
log_test_start "27.2" "GET /blueprints?type=system filters system blueprints"
pepa_api GET "/blueprints?type=system"
if [[ "$API_STATUS" == "200" ]]; then
    SYS_COUNT=$(echo "$API_RESPONSE" | jq '.blueprints // .items // [] | length' 2>/dev/null)
    ALL_SYSTEM=true
    for is_sys in $(echo "$API_RESPONSE" | jq -r '(.blueprints // .items // [])[]?.is_system // empty' 2>/dev/null); do
        if [[ "$is_sys" != "true" ]]; then ALL_SYSTEM=false; break; fi
    done
    if $ALL_SYSTEM; then
        log_test "27.2" "System filter: $SYS_COUNT blueprints, all is_system=true" "pass"
    else
        log_test "27.2" "System filter" "fail" "non-system blueprint found"
    fi
else
    log_test "27.2" "System filter" "fail" "HTTP $API_STATUS"
fi

# ── 27.3 Filter blueprints by type=user ────────────────────────────────────
log_test_start "27.3" "GET /blueprints?type=user filters user blueprints"
pepa_api GET "/blueprints?type=user"
if [[ "$API_STATUS" == "200" ]]; then
    USR_COUNT=$(echo "$API_RESPONSE" | jq '.blueprints // .items // [] | length' 2>/dev/null)
    log_test "27.3" "User filter: $USR_COUNT blueprints" "pass"
else
    log_test "27.3" "User filter" "fail" "HTTP $API_STATUS"
fi

# ── 27.4 Get blueprint by ID ───────────────────────────────────────────────
FIRST_BP_ID=$(echo "$API_RESPONSE" | jq -r '(.blueprints // .items // [])[0].id // empty' 2>/dev/null)
if [[ -z "$FIRST_BP_ID" ]]; then
    # Try from all blueprints
    pepa_api GET "/blueprints"
    FIRST_BP_ID=$(echo "$API_RESPONSE" | jq -r '(.blueprints // .items // [])[0].id // empty' 2>/dev/null)
fi
if [[ -n "$FIRST_BP_ID" ]]; then
    log_test_start "27.4" "GET /blueprints/:id returns blueprint details"
    pepa_api GET "/blueprints/$FIRST_BP_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        BP_NAME=$(echo "$API_RESPONSE" | jq -r '.name // .blueprint.name // empty')
        BP_SRC=$(echo "$API_RESPONSE" | jq -r '.source_type // .blueprint.source_type // empty')
        log_test "27.4" "Blueprint: name='$BP_NAME', source_type=$BP_SRC" "pass"
    else
        log_test "27.4" "Get blueprint by ID" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "27.4" "Get blueprint by ID" "skip" "no blueprints available"
fi

# ── 27.5 Get non-existent blueprint (404) ──────────────────────────────────
log_test_start "27.5" "GET /blueprints/00000000-...-0099 returns 404"
pepa_api GET "/blueprints/00000000-0000-0000-0000-000000000099"
if [[ "$API_STATUS" == "404" ]]; then
    log_test "27.5" "Non-existent blueprint returns 404" "pass"
else
    log_test "27.5" "Non-existent blueprint" "fail" "expected 404, got $API_STATUS"
fi

# ── 27.6 List blueprint groups ─────────────────────────────────────────────
log_test_start "27.6" "GET /blueprint-groups returns groups"
pepa_api GET "/blueprint-groups"
if [[ "$API_STATUS" == "200" ]]; then
    GRP_COUNT=$(echo "$API_RESPONSE" | jq '.groups // .items // [] | length' 2>/dev/null)
    log_test "27.6" "Blueprint groups: $GRP_COUNT found" "pass"
else
    log_test "27.6" "List blueprint groups" "fail" "HTTP $API_STATUS"
fi

# ── 27.7 Create blueprint group ────────────────────────────────────────────
log_test_start "27.7" "POST /blueprint-groups creates a new group"
GRP_BODY="{\"name\":\"test-integration-group\",\"description\":\"Integration test group\"}"
pepa_api POST "/blueprint-groups" "$GRP_BODY"
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    NEW_GRP_ID=$(echo "$API_RESPONSE" | jq -r '.id // .group.id // empty')
    log_test "27.7" "Blueprint group created: ${NEW_GRP_ID:0:8}..." "pass"
else
    log_test "27.7" "Create blueprint group" "fail" "HTTP $API_STATUS"
    NEW_GRP_ID=""
fi

# ── 27.8 Verify group was created via list ──────────────────────────────────
if [[ -n "$NEW_GRP_ID" ]]; then
    log_test_start "27.8" "Verify group exists in list"
    pepa_api GET "/blueprint-groups"
    if [[ "$API_STATUS" == "200" ]]; then
        FOUND=$(echo "$API_RESPONSE" | jq --arg id "$NEW_GRP_ID" '[.groups // .items // [] | .[] | select(.id == $id)] | length')
        if [[ "$FOUND" -ge 1 ]] 2>/dev/null; then
            log_test "27.8" "Group found in list" "pass"
        else
            log_test "27.8" "Group in list" "fail" "group $NEW_GRP_ID not found"
        fi
    else
        log_test "27.8" "List groups" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "27.8" "Verify group" "skip" "no group created"
fi

# ── 27.9 Delete blueprint group ────────────────────────────────────────────
if [[ -n "$NEW_GRP_ID" ]]; then
    log_test_start "27.9" "DELETE /blueprint-groups/:id"
    pepa_api DELETE "/blueprint-groups/$NEW_GRP_ID"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "27.9" "Blueprint group deleted" "pass"
    else
        log_test "27.9" "Delete blueprint group" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "27.9" "Delete blueprint group" "skip" "no group"
fi

# ── 27.10 Verify system vs user counts add up ─────────────────────────────
log_test_start "27.10" "System + user counts == total blueprints"
pepa_api GET "/blueprints"
ALL=$(echo "$API_RESPONSE" | jq '.blueprints // .items // [] | length' 2>/dev/null)
pepa_api GET "/blueprints?type=system"
SYS=$(echo "$API_RESPONSE" | jq '.blueprints // .items // [] | length' 2>/dev/null)
pepa_api GET "/blueprints?type=user"
USR=$(echo "$API_RESPONSE" | jq '.blueprints // .items // [] | length' 2>/dev/null)
SUM=$((SYS + USR))
if [[ "$SUM" -eq "$ALL" ]] 2>/dev/null; then
    log_test "27.10" "system($SYS) + user($USR) = total($ALL)" "pass"
else
    log_test "27.10" "Count mismatch" "fail" "system($SYS) + user($USR) = $SUM != total($ALL)"
fi

print_summary
