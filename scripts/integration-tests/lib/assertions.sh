#!/usr/bin/env bash
# lib/assertions.sh — Test assertion functions for PEPA integration tests
# Source this file after common.sh

# ---------------------------------------------------------------------------
# Basic assertions
# ---------------------------------------------------------------------------

# assert_eq <actual> <expected> [message]
assert_eq() {
    local actual="$1" expected="$2" msg="${3:-values should be equal}"
    if [[ "$actual" == "$expected" ]]; then
        return 0
    fi
    log_fail "$msg" "expected='$expected' actual='$actual'"
    return 1
}

# assert_ne <actual> <not_expected> [message]
assert_ne() {
    local actual="$1" not_expected="$2" msg="${3:-values should differ}"
    if [[ "$actual" != "$not_expected" ]]; then
        return 0
    fi
    log_fail "$msg" "values should differ but both are '$actual'"
    return 1
}

# assert_contains <haystack> <needle> [message]
assert_contains() {
    local haystack="$1" needle="$2" msg="${3:-should contain substring}"
    if [[ "$haystack" == *"$needle"* ]]; then
        return 0
    fi
    log_fail "$msg" "expected to contain '$needle' in: ${haystack:0:200}"
    return 1
}

# assert_not_contains <haystack> <needle> [message]
assert_not_contains() {
    local haystack="$1" needle="$2" msg="${3:-should not contain substring}"
    if [[ "$haystack" != *"$needle"* ]]; then
        return 0
    fi
    log_fail "$msg" "expected NOT to contain '$needle'"
    return 1
}

# assert_not_empty <value> [message]
assert_not_empty() {
    local value="$1" msg="${2:-should not be empty}"
    if [[ -n "$value" ]]; then
        return 0
    fi
    log_fail "$msg" "value is empty"
    return 1
}

# assert_empty <value> [message]
assert_empty() {
    local value="$1" msg="${2:-should be empty}"
    if [[ -z "$value" ]]; then
        return 0
    fi
    log_fail "$msg" "expected empty but got: ${value:0:200}"
    return 1
}

# ---------------------------------------------------------------------------
# Numeric / comparison assertions
# ---------------------------------------------------------------------------

# assert_gt <actual> <threshold> [message]  — greater than
assert_gt() {
    local actual="$1" threshold="$2" msg="${3:-should be greater}"
    if (( $(echo "$actual > $threshold" | bc -l 2>/dev/null || echo 0) )); then
        return 0
    fi
    log_fail "$msg" "$actual is not > $threshold"
    return 1
}

# assert_ge <actual> <threshold> [message]  — greater or equal
assert_ge() {
    local actual="$1" threshold="$2" msg="${3:-should be >= threshold}"
    if (( $(echo "$actual >= $threshold" | bc -l 2>/dev/null || echo 0) )); then
        return 0
    fi
    log_fail "$msg" "$actual is not >= $threshold"
    return 1
}

# ---------------------------------------------------------------------------
# HTTP assertions
# ---------------------------------------------------------------------------

# assert_http_status <response_file> <expected_code> [message]
#   response_file: file containing HTTP status code (first line) or full curl -w output
assert_http_status() {
    local file="$1" expected="$2" msg="${3:-HTTP status check}"
    local actual
    actual=$(head -1 "$file" 2>/dev/null | tr -d '[:space:]')
    if [[ "$actual" == "$expected" ]]; then
        return 0
    fi
    log_fail "$msg" "expected HTTP $expected, got $actual"
    return 1
}

# assert_http_success <response_file> [message]  — 2xx status
assert_http_success() {
    local file="$1" msg="${2:-HTTP 2xx expected}"
    local actual
    actual=$(head -1 "$file" 2>/dev/null | tr -d '[:space:]')
    if [[ "$actual" =~ ^2[0-9][0-9]$ ]]; then
        return 0
    fi
    log_fail "$msg" "expected HTTP 2xx, got $actual"
    return 1
}

# assert_http_error <response_file> <expected_code> [message]
assert_http_error() {
    local file="$1" expected="$2" msg="${3:-HTTP error expected}"
    local actual
    actual=$(head -1 "$file" 2>/dev/null | tr -d '[:space:]')
    if [[ "$actual" == "$expected" ]]; then
        return 0
    fi
    log_fail "$msg" "expected HTTP $expected, got $actual"
    return 1
}

# ---------------------------------------------------------------------------
# JSON assertions (require jq)
# ---------------------------------------------------------------------------

# assert_json_field <json_file> <jq_expression> <expected_value> [message]
assert_json_field() {
    local file="$1" expr="$2" expected="$3" msg="${4:-JSON field check}"
    local actual
    actual=$(jq -r "$expr" "$file" 2>/dev/null)
    if [[ "$actual" == "$expected" ]]; then
        return 0
    fi
    log_fail "$msg" "jq='$expr' expected='$expected' actual='$actual'"
    return 1
}

# assert_json_field_exists <json_file> <jq_expression> [message]
assert_json_field_exists() {
    local file="$1" expr="$2" msg="${3:-JSON field should exist}"
    local actual
    actual=$(jq -e "$expr" "$file" 2>/dev/null)
    if [[ $? -eq 0 && -n "$actual" && "$actual" != "null" ]]; then
        return 0
    fi
    log_fail "$msg" "jq='$expr' field missing or null"
    return 1
}

# assert_json_field_null <json_file> <jq_expression> [message]
assert_json_field_null() {
    local file="$1" expr="$2" msg="${3:-JSON field should be null}"
    local actual
    actual=$(jq -r "$expr" "$file" 2>/dev/null)
    if [[ "$actual" == "null" ]]; then
        return 0
    fi
    log_fail "$msg" "jq='$expr' expected null, got '$actual'"
    return 1
}

# assert_json_array_length <json_file> <jq_array_expression> <expected_length> [message]
assert_json_array_length() {
    local file="$1" expr="$2" expected="$3" msg="${4:-JSON array length}"
    local actual
    actual=$(jq "$expr | length" "$file" 2>/dev/null)
    if [[ "$actual" == "$expected" ]]; then
        return 0
    fi
    log_fail "$msg" "array length expected=$expected actual=$actual"
    return 1
}

# assert_json_contains_string <json_file> <jq_expression> <substring> [message]
assert_json_contains_string() {
    local file="$1" expr="$2" substring="$3" msg="${4:-JSON string contains}"
    local actual
    actual=$(jq -r "$expr" "$file" 2>/dev/null)
    if [[ "$actual" == *"$substring"* ]]; then
        return 0
    fi
    log_fail "$msg" "expected '$substring' in jq result: ${actual:0:200}"
    return 1
}

# ---------------------------------------------------------------------------
# Kubernetes assertions
# ---------------------------------------------------------------------------

# assert_k8s_resource <type> <name> <namespace> <expected_status> [message]
assert_k8s_resource() {
    local type="$1" name="$2" ns="$3" expected="${4:-Running}" msg="${5:-k8s resource check}"
    local actual
    actual=$(k8s get "$type" "$name" -n "$ns" -o jsonpath='{.status.phase}' 2>/dev/null)
    if [[ -z "$actual" ]]; then
        # Try status.conditions for Deployments etc.
        actual=$(k8s get "$type" "$name" -n "$ns" -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null)
        if [[ "$actual" == "True" ]]; then
            actual="Available"
        fi
    fi
    if [[ "$actual" == "$expected" ]]; then
        return 0
    fi
    log_fail "$msg" "$type/$name in $ns: expected=$expected actual=$actual"
    return 1
}

# assert_resource_exists <type> <name> <namespace> [message]
assert_resource_exists() {
    local type="$1" name="$2" ns="$3" msg="${4:-resource should exist}"
    if k8s get "$type" "$name" -n "$ns" &>/dev/null; then
        return 0
    fi
    log_fail "$msg" "$type/$name not found in namespace $ns"
    return 1
}

# assert_resource_absent <type> <name> <namespace> [message]
assert_resource_absent() {
    local type="$1" name="$2" ns="$3" msg="${4:-resource should not exist}"
    if ! k8s get "$type" "$name" -n "$ns" &>/dev/null; then
        return 0
    fi
    log_fail "$msg" "$type/$name still exists in namespace $ns"
    return 1
}

# assert_k8s_replicas <deployment> <namespace> <expected> [message]
assert_k8s_replicas() {
    local deploy="$1" ns="$2" expected="$3" msg="${4:-replica count}"
    local ready
    ready=$(k8s get deploy "$deploy" -n "$ns" -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
    ready="${ready:-0}"
    if [[ "$ready" == "$expected" ]]; then
        return 0
    fi
    log_fail "$msg" "deploy/$deploy readyReplicas: expected=$expected actual=$ready"
    return 1
}

# assert_pod_running <name_prefix> <namespace> [message]
assert_pod_running() {
    local prefix="$1" ns="$2" msg="${3:-pod should be running}"
    local status
    status=$(k8s get pods -n "$ns" --field-selector=status.phase=Running -o json 2>/dev/null \
        | jq -r ".items[] | select(.metadata.name | startswith(\"$prefix\")) | .status.phase" | head -1)
    if [[ "$status" == "Running" ]]; then
        return 0
    fi
    log_fail "$msg" "no Running pod matching '$prefix' in $ns"
    return 1
}

# ---------------------------------------------------------------------------
# Drift detection assertions
# ---------------------------------------------------------------------------

# assert_drift_detected <json_response_file> [message]
assert_drift_detected() {
    local file="$1" msg="${2:-drift should be detected}"
    local count
    count=$(jq '.drifts | length // .drift_count // 0' "$file" 2>/dev/null)
    if [[ -n "$count" && "$count" -gt 0 ]] 2>/dev/null; then
        return 0
    fi
    # Also check if response has drift array with items
    count=$(jq 'if type == "array" then length else 0 end' "$file" 2>/dev/null)
    if [[ -n "$count" && "$count" -gt 0 ]] 2>/dev/null; then
        return 0
    fi
    log_fail "$msg" "no drift detected in response"
    return 1
}

# assert_no_drift <json_response_file> [message]
assert_no_drift() {
    local file="$1" msg="${2:-no drift expected}"
    local count
    count=$(jq '.drifts | length // .drift_count // 0' "$file" 2>/dev/null)
    if [[ "$count" == "0" ]]; then
        return 0
    fi
    count=$(jq 'if type == "array" then length else 0 end' "$file" 2>/dev/null)
    if [[ "$count" == "0" ]]; then
        return 0
    fi
    log_fail "$msg" "drift detected but should be clean (count=$count)"
    return 1
}

# assert_drift_type <json_response_file> <expected_type> [message]
assert_drift_type() {
    local file="$1" expected="$2" msg="${3:-drift type check}"
    local actual
    actual=$(jq -r ".drifts[0].type // .drift_type // empty" "$file" 2>/dev/null)
    if [[ "$actual" == "$expected" ]]; then
        return 0
    fi
    log_fail "$msg" "expected drift type '$expected', got '$actual'"
    return 1
}

# ---------------------------------------------------------------------------
# File / filesystem assertions
# ---------------------------------------------------------------------------

# assert_file_exists <path> [message]
assert_file_exists() {
    local path="$1" msg="${2:-file should exist}"
    if [[ -f "$path" ]]; then
        return 0
    fi
    log_fail "$msg" "file not found: $path"
    return 1
}

# assert_dir_exists <path> [message]
assert_dir_exists() {
    local path="$1" msg="${2:-directory should exist}"
    if [[ -d "$path" ]]; then
        return 0
    fi
    log_fail "$msg" "directory not found: $path"
    return 1
}

# ---------------------------------------------------------------------------
# Composite / convenience assertions
# ---------------------------------------------------------------------------

# assert_pepa_created <response_file> [message]  — 201 + id field present
assert_pepa_created() {
    local file="$1" msg="${2:-resource should be created (201)}"
    assert_http_status "$file" "201" "$msg" || return 1
    assert_json_field_exists "$file" ".id" "$msg" || return 1
}

# assert_pepa_ok <response_file> [message]  — 200
assert_pepa_ok() {
    local file="$1" msg="${2:-request should succeed (200)}"
    assert_http_status "$file" "200" "$msg"
}

# assert_pepa_bad_request <response_file> [message]  — 400
assert_pepa_bad_request() {
    local file="$1" msg="${2:-should return 400 Bad Request}"
    assert_http_status "$file" "400" "$msg"
}

# assert_pepa_forbidden <response_file> [message]  — 403
assert_pepa_forbidden() {
    local file="$1" msg="${2:-should return 403 Forbidden}"
    assert_http_status "$file" "403" "$msg"
}

# assert_pepa_not_found <response_file> [message]  — 404
assert_pepa_not_found() {
    local file="$1" msg="${2:-should return 404 Not Found}"
    assert_http_status "$file" "404" "$msg"
}
