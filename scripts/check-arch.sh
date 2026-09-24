#!/bin/bash
# check-arch.sh — Architecture constraint verification script
# This script checks for violations of architectural boundaries in the PEPA codebase.

set -euo pipefail

cd "$(dirname "$0")/.."

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

ERRORS=0
WARNINGS=0

echo "=== PEPA Architecture Constraint Checks ==="
echo ""

# Check 1: No direct SQL in REST handlers (should be in repositories)
echo "Check 1: SQL queries in REST handlers (should be in repositories)"
SQL_IN_HANDLERS=$(grep -rn "Pool\.Query\|Pool\.Exec\|Pool\.QueryRow" --include="*.go" internal/api/rest/ 2>/dev/null | wc -l || true)
if [ "$SQL_IN_HANDLERS" -gt 0 ]; then
    echo -e "${YELLOW}WARNING${NC}: Found $SQL_IN_HANDLERS SQL queries in REST handlers"
    echo "  SQL queries should be moved to repository layer (internal/repository/)"
    WARNINGS=$((WARNINGS + 1))
else
    echo -e "${GREEN}OK${NC}: No SQL queries in REST handlers"
fi
echo ""

# Check 2: No os.Getenv outside config/cmd/plugins
echo "Check 2: os.Getenv usage outside config/cmd/plugins"
ENV_GET=$(grep -rn "os\.Getenv" --include="*.go" . 2>/dev/null | grep -v "vendor/" | grep -v "internal/config/" | grep -v "cmd/" | grep -v "plugins/" | grep -v "_test.go" | wc -l || true)
if [ "$ENV_GET" -gt 0 ]; then
    echo -e "${RED}ERROR${NC}: Found $ENV_GET os.Getenv calls outside config/cmd/plugins"
    echo "  Environment variables should be read via internal/config"
    ERRORS=$((ERRORS + 1))
else
    echo -e "${GREEN}OK${NC}: No os.Getenv outside config/cmd/plugins"
fi
echo ""

# Check 3: No fmt.Println/Print/Printf in production code (use slog)
echo "Check 3: fmt.Print* usage (should use slog for logging)"
FMT_PRINT=$(grep -rn "fmt\.Print\|fmt\.Println" --include="*.go" . 2>/dev/null | grep -v "vendor/" | grep -v "_test.go" | grep -v "cmd/" | wc -l || true)
if [ "$FMT_PRINT" -gt 0 ]; then
    echo -e "${YELLOW}WARNING${NC}: Found $FMT_PRINT fmt.Print* calls (should use slog)"
    WARNINGS=$((WARNINGS + 1))
else
    echo -e "${GREEN}OK${NC}: No fmt.Print* in production code"
fi
echo ""

# Check 4: No direct http.ListenAndServe (use proper server setup)
echo "Check 4: Direct http.ListenAndServe usage"
LISTEN=$(grep -rn "http\.ListenAndServe" --include="*.go" . 2>/dev/null | grep -v "vendor/" | wc -l || true)
if [ "$LISTEN" -gt 0 ]; then
    echo -e "${YELLOW}WARNING${NC}: Found $LISTEN http.ListenAndServe calls"
    WARNINGS=$((WARNINGS + 1))
else
    echo -e "${GREEN}OK${NC}: No direct http.ListenAndServe"
fi
echo ""

# Check 5: No panic() in production code (except cmd/)
echo "Check 5: panic() usage in production code"
PANIC=$(grep -rn "panic(" --include="*.go" . 2>/dev/null | grep -v "vendor/" | grep -v "cmd/" | grep -v "_test.go" | wc -l || true)
if [ "$PANIC" -gt 0 ]; then
    echo -e "${YELLOW}WARNING${NC}: Found $PANIC panic() calls outside cmd/"
    WARNINGS=$((WARNINGS + 1))
else
    echo -e "${GREEN}OK${NC}: No panic() in production code"
fi
echo ""

# Summary
echo "=== Summary ==="
if [ "$ERRORS" -gt 0 ]; then
    echo -e "${RED}$ERRORS error(s)${NC} found"
fi
if [ "$WARNINGS" -gt 0 ]; then
    echo -e "${YELLOW}$WARNINGS warning(s)${NC} found"
fi
if [ "$ERRORS" -eq 0 ] && [ "$WARNINGS" -eq 0 ]; then
    echo -e "${GREEN}All architecture checks passed!${NC}"
fi

# Exit with error if there are any errors
if [ "$ERRORS" -gt 0 ]; then
    exit 1
fi
exit 0
