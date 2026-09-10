#!/usr/bin/env bash
# run-e2e.sh — PEPA E2E Test Master Runner
# Executes all E2E test phases in sequence.
#
# Usage:
#   ./run-e2e.sh              # Run all phases
#   ./run-e2e.sh --setup-only # Only run infrastructure setup
#   ./run-e2e.sh --skip-setup # Skip infrastructure setup
#   ./run-e2e.sh --cleanup    # Only run cleanup

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export RESULTS_DIR="${RESULTS_DIR:-$SCRIPT_DIR/results}"
export TMP_DIR="${TMP_DIR:-$SCRIPT_DIR/tmp}"
mkdir -p "$RESULTS_DIR" "$TMP_DIR"

# Colours
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# Parse arguments
MODE="full"
for arg in "$@"; do
    case "$arg" in
        --setup-only)   MODE="setup" ;;
        --skip-setup)   MODE="no-setup" ;;
        --cleanup)      MODE="cleanup" ;;
        --help|-h)
            echo "Usage: $0 [--setup-only|--skip-setup|--cleanup]"
            echo "  --setup-only   Only run infrastructure setup (00-setup-e2e.sh)"
            echo "  --skip-setup   Skip infrastructure setup"
            echo "  --cleanup      Only run cleanup (09-cleanup-e2e.sh)"
            exit 0
            ;;
    esac
done

START_TIME=$(date +%s)
LOG_FILE="${RESULTS_DIR}/e2e-$(date +%Y%m%d-%H%M%S).log"

log() {
    local msg="[$(date '+%H:%M:%S')] $*"
    echo -e "$msg" | tee -a "$LOG_FILE"
}

# Track overall results
TOTAL_PASSED=0
TOTAL_FAILED=0
TOTAL_SKIPPED=0
declare -a PHASE_RESULTS=()

run_phase() {
    local phase_name="$1"
    local script="$2"
    local phase_start=$(date +%s)

    log ""
    log "${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    log "${BOLD}${CYAN}  Running: ${phase_name}${NC}"
    log "${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    log ""

    if [[ ! -f "$script" ]]; then
        log "${RED}[SKIP]${NC} Script not found: $script"
        PHASE_RESULTS+=("${phase_name}:SKIP")
        return 0
    fi

    chmod +x "$script"

    # Run the script and capture its summary
    local phase_output
    if phase_output=$(bash "$script" 2>&1 | tee -a "$LOG_FILE"); then
        # Extract test counts from the script's output
        local passed failed skipped
        passed=$(echo "$phase_output" | grep -oP 'Passed:\s+\K\d+' | tail -1 || echo "0")
        failed=$(echo "$phase_output" | grep -oP 'Failed:\s+\K\d+' | tail -1 || echo "0")
        skipped=$(echo "$phase_output" | grep -oP 'Skipped:\s+\K\d+' | tail -1 || echo "0")
        TOTAL_PASSED=$((TOTAL_PASSED + ${passed:-0}))
        TOTAL_FAILED=$((TOTAL_FAILED + ${failed:-0}))
        TOTAL_SKIPPED=$((TOTAL_SKIPPED + ${skipped:-0}))
        PHASE_RESULTS+=("${phase_name}:PASS(+${passed:-0}/-${failed:-0}/~${skipped:-0})")
        log "${GREEN}${phase_name}: PASSED${NC} (+${passed:-0}/-${failed:-0}/~${skipped:-0})"
    else
        local passed failed skipped
        passed=$(echo "$phase_output" | grep -oP 'Passed:\s+\K\d+' | tail -1 || echo "0")
        failed=$(echo "$phase_output" | grep -oP 'Failed:\s+\K\d+' | tail -1 || echo "0")
        skipped=$(echo "$phase_output" | grep -oP 'Skipped:\s+\K\d+' | tail -1 || echo "0")
        TOTAL_PASSED=$((TOTAL_PASSED + ${passed:-0}))
        TOTAL_FAILED=$((TOTAL_FAILED + ${failed:-0}))
        TOTAL_SKIPPED=$((TOTAL_SKIPPED + ${skipped:-0}))
        PHASE_RESULTS+=("${phase_name}:FAIL(+${passed:-0}/-${failed:-0}/~${skipped:-0})")
        log "${RED}${phase_name}: FAILED${NC} (+${passed:-0}/-${failed:-0}/~${skipped:-0})"
    fi

    local phase_end=$(date +%s)
    log "  Duration: $((phase_end - phase_start))s"
}

# ── Main execution ──────────────────────────────────────────────────────────

log "${BOLD}══════════════════════════════════════════════════════════${NC}"
log "${BOLD}  PEPA E2E Test Suite${NC}"
log "${BOLD}  Mode: ${MODE}${NC}"
log "${BOLD}  Started: $(date)${NC}"
log "${BOLD}══════════════════════════════════════════════════════════${NC}"

case "$MODE" in
    setup)
        run_phase "00-Setup" "$SCRIPT_DIR/00-setup-e2e.sh"
        ;;
    cleanup)
        run_phase "09-Cleanup" "$SCRIPT_DIR/09-cleanup-e2e.sh"
        ;;
    full|no-setup)
        if [[ "$MODE" == "full" ]]; then
            run_phase "00-Setup" "$SCRIPT_DIR/00-setup-e2e.sh"
        fi
        run_phase "01-Blueprint Deploy" "$SCRIPT_DIR/01-test-blueprint-deploy.sh"
        run_phase "02-FluxCD Deploy" "$SCRIPT_DIR/02-test-fluxcd-deploy.sh"
        run_phase "03-ArgoCD Deploy" "$SCRIPT_DIR/03-test-argocd-deploy.sh"
        run_phase "04-Lifecycle" "$SCRIPT_DIR/04-test-lifecycle.sh"
        run_phase "05-Scorecard" "$SCRIPT_DIR/05-test-scorecard.sh"
        run_phase "06-Security" "$SCRIPT_DIR/06-test-security.sh"
        run_phase "07-Release Tracking" "$SCRIPT_DIR/07-test-release-tracking.sh"
        run_phase "08-Drift Detection" "$SCRIPT_DIR/08-test-drift.sh"
        run_phase "09-Cleanup" "$SCRIPT_DIR/09-cleanup-e2e.sh"
        ;;
esac

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))
TOTAL=$((TOTAL_PASSED + TOTAL_FAILED + TOTAL_SKIPPED))

# ── Final Summary ───────────────────────────────────────────────────────────
log ""
log "${BOLD}══════════════════════════════════════════════════════════${NC}"
log "${BOLD}  E2E Test Suite Complete${NC}"
log "${BOLD}══════════════════════════════════════════════════════════${NC}"
log "  Duration: ${DURATION}s"
log "  ${GREEN}Total Passed:  $TOTAL_PASSED${NC}"
log "  ${RED}Total Failed:  $TOTAL_FAILED${NC}"
log "  ${YELLOW}Total Skipped: $TOTAL_SKIPPED${NC}"
log "  Total Tests:   $TOTAL"
log ""
log "  Phase Results:"
for result in "${PHASE_RESULTS[@]}"; do
    local phase="${result%%:*}"
    local status="${result#*:}"
    if [[ "$status" == PASS* ]]; then
        log "    ${GREEN}✓${NC} $phase: $status"
    elif [[ "$status" == FAIL* ]]; then
        log "    ${RED}✗${NC} $phase: $status"
    else
        log "    ${YELLOW}~${NC} $phase: $status"
    fi
done
log ""
log "  Log file: $LOG_FILE"
log "  Results:  $RESULTS_DIR"
log "${BOLD}══════════════════════════════════════════════════════════${NC}"

# Exit with failure if any tests failed
if [[ $TOTAL_FAILED -gt 0 ]]; then
    exit 1
fi
exit 0
