#!/usr/bin/env bash
# lib/common.sh — Shared utilities for PEPA integration tests.
# Source this file from every test script:
#   SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
#   source "$SCRIPT_DIR/lib/common.sh"

set -uo pipefail  # Don't use -e; we handle errors manually via test tracking

# ── Colours ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# ── Configuration ─────────────────────────────────────────────────────────────
# PEPA_URL is used by test scripts; PEPA_API_URL is the base for pepa_api()
PEPA_URL="${PEPA_URL:-http://localhost:8088}"
PEPA_API_URL="${PEPA_API_URL:-${PEPA_URL}}"
PEPA_ADMIN_USER="${PEPA_ADMIN_USER:-admin@local}"
PEPA_ADMIN_PASSWORD="${PEPA_ADMIN_PASSWORD:-Admin123!}"
PEPA_TOKEN="${PEPA_TOKEN:-}"
GITEA_URL="${GITEA_URL:-http://localhost:3001}"
GITEA_ADMIN_USER="${GITEA_ADMIN_USER:-pepa}"
GITEA_ADMIN_PASSWORD="${GITEA_ADMIN_PASSWORD:-PepaTest2026!}"
GITEA_TOKEN="${GITEA_TOKEN:-}"
K3D_PRIMARY="${K3D_PRIMARY:-pepa-test-primary}"
K3D_SECONDARY="${K3D_SECONDARY:-pepa-test-secondary}"
TEST_NAMESPACE="${TEST_NAMESPACE:-pepa-test}"
RESULTS_DIR="${RESULTS_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/results}"
TMP_DIR="${TMP_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/tmp}"
TMP="${TMP:-$TMP_DIR}"  # Alias for convenience
LOG_FILE="${RESULTS_DIR}/test-$(date +%Y%m%d-%H%M%S).log"

# Counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0
CURRENT_PHASE=""

# ── Ensure directories exist ──────────────────────────────────────────────────
mkdir -p "$RESULTS_DIR" "$TMP_DIR"

# ── Logging ───────────────────────────────────────────────────────────────────
log() {
    local msg="[$(date '+%H:%M:%S')] $*"
    echo -e "$msg" | tee -a "$LOG_FILE"
}

log_info()  { log "${BLUE}[INFO]${NC}  $*"; }
log_ok()    { log "${GREEN}[PASS]${NC}  $*"; }
log_fail()  { log "${RED}[FAIL]${NC}  $*"; }
log_skip()  { log "${YELLOW}[SKIP]${NC}  $*"; }
log_step()  { log "${CYAN}[STEP]${NC}  $*"; }
log_warn()  { log "${YELLOW}[WARN]${NC}  $*"; }
log_error() { log "${RED}[ERROR]${NC} $*"; }
log_phase() {
    CURRENT_PHASE="$1"
    log ""
    log "${BOLD}${CYAN}══════════════════════════════════════════════════════════${NC}"
    log "${BOLD}${CYAN}  Phase: $1${NC}"
    log "${BOLD}${CYAN}══════════════════════════════════════════════════════════${NC}"
    log ""
}

# ── Test tracking (used by all phase scripts) ────────────────────────────────
log_test_start() {
    local test_id="$1"
    local test_name="$2"
    log_info "[$test_id] $test_name"
}

log_test_pass() {
    local test_id="$1"
    local test_name="$2"
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
    log_ok "[$test_id] $test_name"
}

log_test_fail() {
    local test_id="$1"
    local test_name="$2"
    local detail="${3:-}"
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
    log_fail "[$test_id] $test_name${detail:+ — $detail}"
}

log_test_skip() {
    local test_id="$1"
    local test_name="$2"
    local detail="${3:-}"
    TESTS_RUN=$((TESTS_RUN + 1))
    TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
    log_skip "[$test_id] $test_name${detail:+ — $detail}"
}

# ── Test tracking ─────────────────────────────────────────────────────────────
log_test() {
    local test_id="$1"
    local test_name="$2"
    local result="$3"  # pass, fail, skip
    local detail="${4:-}"

    TESTS_RUN=$((TESTS_RUN + 1))
    case "$result" in
        pass)
            TESTS_PASSED=$((TESTS_PASSED + 1))
            log_ok "[$test_id] $test_name"
            ;;
        fail)
            TESTS_FAILED=$((TESTS_FAILED + 1))
            log_fail "[$test_id] $test_name${detail:+ — $detail}"
            ;;
        skip)
            TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
            log_skip "[$test_id] $test_name${detail:+ — $detail}"
            ;;
    esac
}

print_summary() {
    log ""
    log "${BOLD}══════════════════════════════════════════════════════════${NC}"
    log "${BOLD}  Test Summary${NC}"
    log "${BOLD}══════════════════════════════════════════════════════════${NC}"
    log "  Total:   $TESTS_RUN"
    log "  ${GREEN}Passed:  $TESTS_PASSED${NC}"
    log "  ${RED}Failed:  $TESTS_FAILED${NC}"
    log "  ${YELLOW}Skipped: $TESTS_SKIPPED${NC}"
    log "${BOLD}══════════════════════════════════════════════════════════${NC}"
    log ""

    # Write JUnit XML
    local xml_file="$RESULTS_DIR/junit-$(date +%Y%m%d-%H%M%S).xml"
    _write_junit_xml "$xml_file"
    log "  JUnit XML: $xml_file"
    log "  Log file:  $LOG_FILE"
}

_write_junit_xml() {
    local file="$1"
    cat > "$file" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="PEPA Integration Tests" tests="$TESTS_RUN" failures="$TESTS_FAILED" skipped="$TESTS_SKIPPED" timestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)">
    <properties>
      <property name="pepa_api_url" value="$PEPA_API_URL"/>
      <property name="primary_cluster" value="$K3D_PRIMARY"/>
    </properties>
    <!-- Individual test cases are written by log_test_junit() -->
  </testsuite>
</testsuites>
EOF
}

log_test_junit() {
    local test_id="$1"
    local test_name="$2"
    local result="$3"
    local detail="${4:-}"
    local classname="${CURRENT_PHASE:-unknown}"

    # Append testcase element before closing </testsuite>
    local tc=""
    case "$result" in
        pass)
            tc="    <testcase classname=\"$classname\" name=\"[$test_id] $test_name\" time=\"0\"/>"
            ;;
        fail)
            tc="    <testcase classname=\"$classname\" name=\"[$test_id] $test_name\" time=\"0\">
      <failure message=\"${detail:-Test failed}\">$detail</failure>
    </testcase>"
            ;;
        skip)
            tc="    <testcase classname=\"$classname\" name=\"[$test_id] $test_name\" time=\"0\">
      <skipped message=\"${detail:-Skipped}\"/>
    </testcase>"
            ;;
    esac
    echo "$tc" >> "$RESULTS_DIR/junit-pending.tmp"
}

# ── PEPA API helpers ──────────────────────────────────────────────────────────

# pepa_api METHOD PATH [DATA] [OUT_JSON_FILE] [OUT_CODE_FILE]
# When called with 5 args, writes JSON body to OUT_JSON_FILE and HTTP code to OUT_CODE_FILE.
# When called with 2-3 args, sets global API_RESPONSE and API_STATUS.
pepa_api() {
    local method="$1"
    local path="$2"
    local data="${3:-}"
    local out_json="${4:-}"
    local out_code="${5:-}"

    # Auto-prepend /api/v1 if path doesn't already start with it
    if [[ "$path" != /api/v1* ]]; then
        path="/api/v1${path}"
    fi
    local url="${PEPA_API_URL}${path}"

    local curl_args=(
        -s -w "\n%{http_code}"
        -X "$method"
        -H "Content-Type: application/json"
        -H "Accept: application/json"
    )

    if [[ -n "$PEPA_TOKEN" ]]; then
        curl_args+=(-H "Authorization: Bearer $PEPA_TOKEN")
    fi

    if [[ -n "$data" ]]; then
        curl_args+=(-d "$data")
    fi

    local response
    response=$(curl "${curl_args[@]}" "$url" 2>>"$LOG_FILE") || true

    local http_code
    http_code=$(echo "$response" | tail -1)
    local body
    body=$(echo "$response" | sed '$d')

    # If output files specified (5-arg mode), write to files
    if [[ -n "$out_json" && "$out_json" != "/dev/null" ]]; then
        echo "$body" > "$out_json" 2>/dev/null || true
    fi
    if [[ -n "$out_code" && "$out_code" != "/dev/null" ]]; then
        echo "$http_code" > "$out_code" 2>/dev/null || true
    fi

    # Always set global variables for backward compatibility
    API_RESPONSE="$body"
    API_STATUS="$http_code"
}

# Convenience wrappers
pepa_get()    { pepa_api GET "$1"; }
pepa_post()   { pepa_api POST "$1" "${2:-}"; }
pepa_put()    { pepa_api PUT "$1" "${2:-}"; }
pepa_delete() { pepa_api DELETE "$1"; }

# Login and store token
pepa_login() {
    local user="${1:-$PEPA_ADMIN_USER}"
    local pass="${2:-$PEPA_ADMIN_PASSWORD}"

    _pepa_login_attempt() {
        local u="$1" p="$2"
        pepa_api POST "/api/v1/auth/login" "{\"email\":\"$u\",\"password\":\"$p\"}"
        if [[ "$API_STATUS" == "200" ]]; then
            PEPA_TOKEN=$(echo "$API_RESPONSE" | jq -r '.token // .access_token // empty' 2>/dev/null || true)
            if [[ -z "$PEPA_TOKEN" ]]; then
                local cookies
                cookies=$(curl -s -D - -X POST \
                    -H "Content-Type: application/json" \
                    -d "{\"email\":\"$u\",\"password\":\"$p\"}" \
                    "${PEPA_API_URL}/api/v1/auth/login" 2>/dev/null | grep -i 'set-cookie' || true)
                PEPA_TOKEN=$(echo "$cookies" | grep -oP 'pepa_token=\K[^;]+' || true)
            fi
            if [[ -n "$PEPA_TOKEN" ]]; then
                export PEPA_TOKEN
                log_info "Logged in as $u (token: ${PEPA_TOKEN:0:20}...)"
                return 0
            fi
        fi
        return 1
    }

    if _pepa_login_attempt "$user" "$pass"; then
        return 0
    fi
    # Try fallback passwords from previous test runs
    for fallback in "Admin123!" "NewPass456!"; do
        if [[ "$pass" != "$fallback" ]]; then
            if _pepa_login_attempt "$user" "$fallback"; then
                return 0
            fi
        fi
    done
    log_fail "Login failed: tried all passwords"
    return 1
}

# ── kubectl helpers ───────────────────────────────────────────────────────────

# Use the correct k3d context
k8s() {
    local cluster="${1:-$K3D_PRIMARY}"
    shift
    kubectl --context "k3d-$cluster" "$@"
}

# Create test namespace in target cluster
setup_test_ns() {
    local cluster="${1:-$K3D_PRIMARY}"
    local ns="${2:-$TEST_NAMESPACE}"
    k8s "$cluster" create namespace "$ns" --dry-run=client -o yaml | k8s "$cluster" apply -f -
    log_info "Namespace '$ns' ready in cluster '$cluster'"
}

# Delete test namespace
cleanup_ns() {
    local cluster="${1:-$K3D_PRIMARY}"
    local ns="${2:-$TEST_NAMESPACE}"
    k8s "$cluster" delete namespace "$ns" --ignore-not-found=true --wait=false 2>/dev/null || true
    log_info "Namespace '$ns' cleanup initiated in cluster '$cluster'"
}

# ── Wait helpers ──────────────────────────────────────────────────────────────

# wait_for "condition" timeout_seconds description
wait_for() {
    local condition="$1"
    local timeout="${2:-60}"
    local desc="${3:-condition}"
    local elapsed=0

    log_info "Waiting for $desc (timeout: ${timeout}s)..."
    while [[ $elapsed -lt $timeout ]]; do
        if eval "$condition" >/dev/null 2>&1; then
            log_ok "Condition met: $desc (${elapsed}s)"
            return 0
        fi
        sleep 2
        elapsed=$((elapsed + 2))
    done
    log_fail "Timeout waiting for $desc after ${timeout}s"
    return 1
}

# Wait for k8s resource to reach a condition
wait_for_k8s() {
    local cluster="${1:-$K3D_PRIMARY}"
    local resource="$2"
    local condition="${3:-condition=Available}"
    local namespace="${4:-$TEST_NAMESPACE}"
    local timeout="${5:-120}"

    k8s "$cluster" wait "$resource" \
        --for="$condition" \
        -n "$namespace" \
        --timeout="${timeout}s" 2>>"$LOG_FILE"
}

# Wait for PEPA API health
wait_for_pepa() {
    wait_for 'curl -sf "${PEPA_API_URL}/healthz" >/dev/null 2>&1' 60 "PEPA API health"
}

# ── k3d helpers ───────────────────────────────────────────────────────────────

k3d_cluster_exists() {
    k3d cluster get "$1" >/dev/null 2>&1
}

k3d_get_kubeconfig() {
    local cluster="$1"
    k3d kubeconfig get "$cluster" 2>/dev/null
}

# ── FluxCD CLI helpers ───────────────────────────────────────────────────────

# flux_cli SUBCOMMAND... — run flux CLI against the primary cluster
flux_cli() {
    flux --context "k3d-$K3D_PRIMARY" "$@"
}

# flux_cli_secondary SUBCOMMAND... — run flux CLI against the secondary cluster
flux_cli_secondary() {
    flux --context "k3d-$K3D_SECONDARY" "$@"
}

# Install FluxCD into a cluster
flux_install() {
    local cluster="${1:-$K3D_PRIMARY}"
    log_info "Installing FluxCD into cluster '$cluster'..."
    flux --context "k3d-$cluster" install 2>>"$LOG_FILE"
}

# Wait for FluxCD HelmRelease to be ready
wait_for_flux_helmrelease() {
    local name="$1" namespace="${2:-pepa-e2e}" cluster="${3:-$K3D_PRIMARY}" timeout="${4:-120}"
    log_info "Waiting for FluxCD HelmRelease '$name' in '$namespace' (${timeout}s)..."
    local elapsed=0
    while [[ $elapsed -lt $timeout ]]; do
        local ready
        ready=$(kubectl --context "k3d-$cluster" get helmrelease "$name" -n "$namespace" \
            -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "False")
        if [[ "$ready" == "True" ]]; then
            log_ok "HelmRelease '$name' is Ready (${elapsed}s)"
            return 0
        fi
        sleep 3
        elapsed=$((elapsed + 3))
    done
    log_fail "Timeout waiting for HelmRelease '$name' after ${timeout}s"
    return 1
}

# Wait for FluxCD Kustomization to be ready
wait_for_flux_kustomization() {
    local name="$1" namespace="${2:-pepa-e2e}" cluster="${3:-$K3D_PRIMARY}" timeout="${4:-120}"
    log_info "Waiting for FluxCD Kustomization '$name' in '$namespace' (${timeout}s)..."
    local elapsed=0
    while [[ $elapsed -lt $timeout ]]; do
        local ready
        ready=$(kubectl --context "k3d-$cluster" get kustomization "$name" -n "$namespace" \
            -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "False")
        if [[ "$ready" == "True" ]]; then
            log_ok "Kustomization '$name' is Ready (${elapsed}s)"
            return 0
        fi
        sleep 3
        elapsed=$((elapsed + 3))
    done
    log_fail "Timeout waiting for Kustomization '$name' after ${timeout}s"
    return 1
}

# Force FluxCD reconciliation
flux_reconcile_helmrelease() {
    local name="$1" namespace="${2:-pepa-e2e}" cluster="${3:-$K3D_PRIMARY}"
    flux --context "k3d-$cluster" reconcile helmrelease "$name" -n "$namespace" 2>>"$LOG_FILE"
}

flux_reconcile_kustomization() {
    local name="$1" namespace="${2:-pepa-e2e}" cluster="${3:-$K3D_PRIMARY}"
    flux --context "k3d-$cluster" reconcile kustomization "$name" -n "$namespace" 2>>"$LOG_FILE"
}

# Suspend/Resume FluxCD resources
flux_suspend_helmrelease() {
    local name="$1" namespace="${2:-pepa-e2e}" cluster="${3:-$K3D_PRIMARY}"
    flux --context "k3d-$cluster" suspend helmrelease "$name" -n "$namespace" 2>>"$LOG_FILE"
}

flux_resume_helmrelease() {
    local name="$1" namespace="${2:-pepa-e2e}" cluster="${3:-$K3D_PRIMARY}"
    flux --context "k3d-$cluster" resume helmrelease "$name" -n "$namespace" 2>>"$LOG_FILE"
}

# ── ArgoCD CLI helpers ───────────────────────────────────────────────────────

# ArgoCD server address (in-cluster or port-forwarded)
ARGOCD_SERVER="${ARGOCD_SERVER:-localhost:8090}"
ARGOCD_ADMIN_PASS="${ARGOCD_ADMIN_PASS:-}"

# argocd_cli SUBCOMMAND... — run argocd CLI
argocd_cli() {
    argocd --server "$ARGOCD_SERVER" --insecure --grpc-web "$@"
}

# Login to ArgoCD
argocd_login() {
    local user="${1:-admin}" pass="${2:-$ARGOCD_ADMIN_PASS}"
    if [[ -z "$pass" ]]; then
        # Get admin password from k8s secret
        pass=$(k8s "$K3D_PRIMARY" -n argocd get secret argocd-initial-admin-secret \
            -o jsonpath='{.data.password}' 2>/dev/null | base64 -d 2>/dev/null || echo "")
    fi
    if [[ -z "$pass" ]]; then
        log_warn "Cannot determine ArgoCD admin password"
        return 1
    fi
    ARGOCD_ADMIN_PASS="$pass"
    argocd login "$ARGOCD_SERVER" --username "$user" --password "$pass" --insecure --grpc-web --plaintext 2>>"$LOG_FILE"
}

# Wait for ArgoCD Application to be Synced+Healthy
wait_for_argocd_app() {
    local name="$1" timeout="${2:-120}"
    log_info "Waiting for ArgoCD app '$name' to be Synced+Healthy (${timeout}s)..."
    local elapsed=0
    while [[ $elapsed -lt $timeout ]]; do
        local health sync
        health=$(argocd app get "$name" --insecure --grpc-web --plaintext -o json 2>/dev/null \
            | jq -r '.status.health.status // empty' 2>/dev/null || echo "")
        sync=$(argocd app get "$name" --insecure --grpc-web --plaintext -o json 2>/dev/null \
            | jq -r '.status.sync.status // empty' 2>/dev/null || echo "")
        if [[ "$health" == "Healthy" && "$sync" == "Synced" ]]; then
            log_ok "ArgoCD app '$name' is Synced+Healthy (${elapsed}s)"
            return 0
        fi
        sleep 3
        elapsed=$((elapsed + 3))
    done
    log_fail "Timeout waiting for ArgoCD app '$name' after ${timeout}s"
    return 1
}

# Sync ArgoCD application
argocd_sync() {
    local name="$1"
    argocd app sync "$name" --insecure --grpc-web --plaintext 2>>"$LOG_FILE"
}

# Force ArgoCD app refresh
argocd_refresh() {
    local name="$1"
    argocd app get "$name" --refresh --insecure --grpc-web --plaintext 2>>"$LOG_FILE"
}

# ── Namespace-per-environment helpers ─────────────────────────────────────────

E2E_NAMESPACE="${E2E_NAMESPACE:-pepa-e2e}"
E2E_DEV_NS="pepa-e2e-dev"
E2E_TESTING_NS="pepa-e2e-testing"
E2E_STAGING_NS="pepa-e2e-staging"

setup_e2e_namespaces() {
    local cluster="${1:-$K3D_PRIMARY}"
    for ns in "$E2E_NAMESPACE" "$E2E_DEV_NS" "$E2E_TESTING_NS" "$E2E_STAGING_NS"; do
        k8s "$cluster" create namespace "$ns" --dry-run=client -o yaml | k8s "$cluster" apply -f - 2>/dev/null
    done
    log_info "E2E namespaces created in cluster '$cluster'"
}

cleanup_e2e_namespaces() {
    local cluster="${1:-$K3D_PRIMARY}"
    for ns in "$E2E_NAMESPACE" "$E2E_DEV_NS" "$E2E_TESTING_NS" "$E2E_STAGING_NS"; do
        k8s "$cluster" delete namespace "$ns" --ignore-not-found=true --wait=false 2>/dev/null || true
    done
    log_info "E2E namespaces cleanup initiated in cluster '$cluster'"
}

# ── Misc helpers ──────────────────────────────────────────────────────────────

# Generate a random suffix
random_suffix() {
    head -c 4 /dev/urandom | od -An -tx1 | tr -d ' \n'
}

# Check if a command exists
require_cmd() {
    local cmd="$1"
    if ! command -v "$cmd" &>/dev/null; then
        log_fail "Required command not found: $cmd"
        return 1
    fi
}

# Check all prerequisites (pass command names as args, or uses defaults if none given)
check_prerequisites() {
    local cmds=("$@")
    if [[ ${#cmds[@]} -eq 0 ]]; then
        cmds=("k3d" "kubectl" "helm" "curl" "jq" "git")
    fi
    local missing=()
    for cmd in "${cmds[@]}"; do
        if ! command -v "$cmd" &>/dev/null; then
            missing+=("$cmd")
        fi
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        log_fail "Missing prerequisites: ${missing[*]}"
        return 1
    fi
    log_ok "All prerequisites satisfied (${cmds[*]})"
}

# Cleanup handler for EXIT trap
cleanup_on_exit() {
    local exit_code=$?
    if [[ $exit_code -ne 0 ]]; then
        log "Test suite exited with code $exit_code"
    fi
    print_summary
}
