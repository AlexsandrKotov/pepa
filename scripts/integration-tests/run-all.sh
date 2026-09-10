#!/usr/bin/env bash
# run-all.sh — Master runner for PEPA integration tests
# Usage: ./run-all.sh [--skip-setup] [--skip-cleanup] [--phase N] [--parallel]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

# ---------------------------------------------------------------------------
# Parse arguments
# ---------------------------------------------------------------------------
SKIP_SETUP=false
SKIP_CLEANUP=false
SINGLE_PHASE=""
PARALLEL=false
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --skip-setup) SKIP_SETUP=true; shift ;;
        --skip-cleanup) SKIP_CLEANUP=true; shift ;;
        --phase) SINGLE_PHASE="$2"; shift 2 ;;
        --parallel) PARALLEL=true; shift ;;
        --verbose|-v) VERBOSE=true; shift ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --skip-setup     Skip Phase 0 (setup)"
            echo "  --skip-cleanup   Skip Phase 21 (cleanup)"
            echo "  --phase N        Run only phase N (0-21)"
            echo "  --parallel       Run independent phases in parallel"
            echo "  --verbose, -v    Enable verbose output"
            echo "  --help, -h       Show this help"
            echo ""
            echo "Environment variables:"
            echo "  PEPA_URL              PEPA API URL (default: http://localhost:8088)"
            echo "  PRIMARY_CLUSTER       Primary k3d cluster name (default: pepa-test-primary)"
            echo "  SECONDARY_CLUSTER     Secondary k3d cluster name (default: pepa-test-secondary)"
            echo "  TEST_NAMESPACE        Test namespace (default: pepa-test)"
            echo "  SKIP_CLUSTER          Skip cluster creation (default: false)"
            echo "  DESTROY_CLUSTERS      Destroy clusters on cleanup (default: false)"
            echo "  GITEA_URL             Gitea URL (default: http://localhost:3001)"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

if [[ "$VERBOSE" == "true" ]]; then
    set -x
fi

# ---------------------------------------------------------------------------
# Initialize
# ---------------------------------------------------------------------------
START_TIME=$(date +%s)
log_phase "PEPA Integration Test Suite"
log_info "Started at: $(date)"
log_info "Results dir: ${RESULTS_DIR}"
log_info ""

# Ensure results directory exists
mkdir -p "${RESULTS_DIR}/tmp"

# Track overall results
TOTAL_PASS=0
TOTAL_FAIL=0
TOTAL_SKIP=0
PHASE_RESULTS=()

run_phase() {
    local phase_num="$1"
    local script="$2"
    local desc="$3"
    
    if [[ ! -f "$script" ]]; then
        log_warn "Script not found: $script"
        return 0
    fi
    
    log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_info "Running Phase ${phase_num}: ${desc}"
    log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    local phase_start=$(date +%s)
    if bash "$script" 2>&1 | tee "${RESULTS_DIR}/phase-${phase_num}.log"; then
        local phase_end=$(date +%s)
        local duration=$((phase_end - phase_start))
        log_info "Phase ${phase_num} completed in ${duration}s"
        PHASE_RESULTS+=("Phase ${phase_num}: OK (${duration}s)")
    else
        local phase_end=$(date +%s)
        local duration=$((phase_end - phase_start))
        log_warn "Phase ${phase_num} had failures (${duration}s)"
        PHASE_RESULTS+=("Phase ${phase_num}: FAILURES (${duration}s)")
    fi
    log_info ""
}

# ---------------------------------------------------------------------------
# Phase 0: Setup
# ---------------------------------------------------------------------------
if [[ "$SKIP_SETUP" != "true" && -z "$SINGLE_PHASE" ]]; then
    run_phase "0" "${SCRIPT_DIR}/00-setup.sh" "Infrastructure Setup"
elif [[ "$SINGLE_PHASE" == "0" ]]; then
    run_phase "0" "${SCRIPT_DIR}/00-setup.sh" "Infrastructure Setup"
fi

# ---------------------------------------------------------------------------
# Test phases (1-20)
# ---------------------------------------------------------------------------
declare -A PHASES
PHASES=(
    [1]="01-test-bootstrap-auth.sh:Bootstrap and Auth"
    [2]="02-test-connections-clusters.sh:Connections and Clusters"
    [3]="03-test-direct-deploy.sh:Direct k8s and Helm Deploy"
    [4]="04-test-plugins.sh:Plugin System"
    [5]="05-test-vault.sh:Vault Integration"
    [6]="06-test-fluxcd.sh:FluxCD"
    [7]="07-test-argocd.sh:ArgoCD"
    [8]="08-test-gitops-advanced.sh:GitOps Advanced"
    [9]="09-test-workflows-pipelines.sh:Workflows and Pipelines"
    [10]="10-test-entities-scorecards.sh:Entities and Scorecards"
    [11]="11-test-docker-services.sh:Docker Services"
    [12]="12-test-security-scanning.sh:Security Scanning"
    [13]="13-test-blueprints.sh:Service Blueprints"
    [14]="14-test-rbac.sh:RBAC"
    [15]="15-test-notifications.sh:Notifications"
    [16]="16-test-ssh-hosts.sh:SSH Hosts"
    [17]="17-test-registries.sh:Registries"
    [18]="18-test-multi-cluster.sh:Multi-Cluster"
    [19]="19-test-credential-resolution.sh:Credential Resolution"
    [20]="20-test-iac.sh:IaC Execution"
)

# Sequential phases (dependencies)
SEQUENTIAL_ORDER=(1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20)

if [[ -n "$SINGLE_PHASE" ]]; then
    if [[ -n "${PHASES[$SINGLE_PHASE]:-}" ]]; then
        IFS=':' read -r script desc <<< "${PHASES[$SINGLE_PHASE]}"
        run_phase "$SINGLE_PHASE" "${SCRIPT_DIR}/${script}" "$desc"
    else
        log_error "Unknown phase: $SINGLE_PHASE"
        exit 1
    fi
elif [[ "$PARALLEL" == "true" ]]; then
    # Run foundational phases sequentially first
    for p in 1 2 4; do
        IFS=':' read -r script desc <<< "${PHASES[$p]}"
        run_phase "$p" "${SCRIPT_DIR}/${script}" "$desc"
    done
    
    # Run independent phases in parallel
    log_info "Running independent phases in parallel..."
    PIDS=()
    for p in 3 5 9 10 11 12 13 15 16 17; do
        IFS=':' read -r script desc <<< "${PHASES[$p]}"
        bash "${SCRIPT_DIR}/${script}" > "${RESULTS_DIR}/phase-${p}.log" 2>&1 &
        PIDS+=($!)
    done
    
    # Wait for parallel phases
    for pid in "${PIDS[@]}"; do
        wait $pid 2>/dev/null || true
    done
    
    # Run dependent phases sequentially
    for p in 6 7 8 14 18 19 20; do
        IFS=':' read -r script desc <<< "${PHASES[$p]}"
        run_phase "$p" "${SCRIPT_DIR}/${script}" "$desc"
    done
else
    # Sequential execution
    for p in "${SEQUENTIAL_ORDER[@]}"; do
        IFS=':' read -r script desc <<< "${PHASES[$p]}"
        run_phase "$p" "${SCRIPT_DIR}/${script}" "$desc"
    done
fi

# ---------------------------------------------------------------------------
# Phase 21: Cleanup
# ---------------------------------------------------------------------------
if [[ "$SKIP_CLEANUP" != "true" && -z "$SINGLE_PHASE" ]]; then
    run_phase "21" "${SCRIPT_DIR}/21-cleanup.sh" "Cleanup"
elif [[ "$SINGLE_PHASE" == "21" ]]; then
    run_phase "21" "${SCRIPT_DIR}/21-cleanup.sh" "Cleanup"
fi

# ---------------------------------------------------------------------------
# Final Summary
# ---------------------------------------------------------------------------
END_TIME=$(date +%s)
TOTAL_DURATION=$((END_TIME - START_TIME))

log_phase "Test Suite Complete"
log_info "Total duration: ${TOTAL_DURATION}s"
log_info ""
log_info "Phase results:"
for result in "${PHASE_RESULTS[@]}"; do
    log_info "  $result"
done
log_info ""

# Aggregate JUnit XML
if command -v xmllint &>/dev/null; then
    log_info "JUnit XML reports in: ${RESULTS_DIR}/"
fi

log_info "Log files in: ${RESULTS_DIR}/phase-*.log"
log_info ""
log_info "Done!"
