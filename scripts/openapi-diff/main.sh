#!/bin/bash
# openapi-diff: Compare registered Gin routes with OpenAPI specification.
# Usage: ./scripts/openapi-diff/main.sh [--allowlist docs/openapi-coverage.txt]
#
# Exit codes:
#   0 - All routes are documented (or only allowlisted routes are missing)
#   1 - New undocumented routes found (not in allowlist)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ROUTER_DIR="$PROJECT_ROOT/internal/api/rest"
OPENAPI_FILE="$PROJECT_ROOT/docs/openapi.yaml"
ALLOWLIST_FILE="${1:-$PROJECT_ROOT/docs/openapi-coverage.txt}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "=== OpenAPI Coverage Check ==="
echo ""

# Step 1: Extract all routes from Go handlers.
# Pattern: r.GET("/path", ...) or v1.POST("/path", ...)
echo "Extracting routes from $ROUTER_DIR..."
ROUTES_FILE=$(mktemp)
grep -rhoE '\.(GET|POST|PUT|DELETE|PATCH)\("[^"]+"' "$ROUTER_DIR"/*.go 2>/dev/null | \
    sed -E 's/.*\.(GET|POST|PUT|DELETE|PATCH)\("([^"]+)".*/\1 \2/' | \
    sort -u > "$ROUTES_FILE"

TOTAL_ROUTES=$(wc -l < "$ROUTES_FILE" | tr -d ' ')
echo "Found $TOTAL_ROUTES routes in handlers."
echo ""

# Step 2: Extract paths from OpenAPI spec.
echo "Extracting paths from $OPENAPI_FILE..."
OPENAPI_PATHS_FILE=$(mktemp)
if [[ -f "$OPENAPI_FILE" ]]; then
    # Extract paths like "/api/v1/connections:" from openapi.yaml
    grep -E '^  /[^:]+:' "$OPENAPI_FILE" | \
        sed -E 's/^  (\/[^:]+):.*/\1/' | \
        sort -u > "$OPENAPI_PATHS_FILE"
    
    DOCUMENTED_PATHS=$(wc -l < "$OPENAPI_PATHS_FILE" | tr -d ' ')
    echo "Found $DOCUMENTED_PATHS documented paths in OpenAPI spec."
else
    echo "WARNING: OpenAPI file not found at $OPENAPI_FILE"
    DOCUMENTED_PATHS=0
fi
echo ""

# Step 3: Compare routes with documented paths.
echo "Comparing routes with OpenAPI spec..."
UNDOCUMENTED_FILE=$(mktemp)

while IFS=' ' read -r method path; do
    # Normalize path: remove trailing slash for comparison
    normalized_path="${path%/}"
    
    # Check if this path is documented (any method)
    if ! grep -q "^${normalized_path}$" "$OPENAPI_PATHS_FILE" 2>/dev/null; then
        echo "$method $path" >> "$UNDOCUMENTED_FILE"
    fi
done < "$ROUTES_FILE"

UNDOCUMENTED_COUNT=0
if [[ -f "$UNDOCUMENTED_FILE" ]]; then
    UNDOCUMENTED_COUNT=$(wc -l < "$UNDOCUMENTED_FILE" | tr -d ' ')
fi

echo ""
echo "=== Results ==="
echo "Total routes: $TOTAL_ROUTES"
echo "Documented paths: $DOCUMENTED_PATHS"
echo "Undocumented routes: $UNDOCUMENTED_COUNT"
echo ""

# Step 4: Check against allowlist.
NEW_UNDOCUMENTED=0
if [[ $UNDOCUMENTED_COUNT -gt 0 ]]; then
    if [[ -f "$ALLOWLIST_FILE" ]]; then
        echo "Checking against allowlist: $ALLOWLIST_FILE"
        NEW_ROUTES_FILE=$(mktemp)
        
        while IFS=' ' read -r method path; do
            # Check if this route is in the allowlist
            if ! grep -qF "$path" "$ALLOWLIST_FILE" 2>/dev/null; then
                echo "$method $path" >> "$NEW_ROUTES_FILE"
            fi
        done < "$UNDOCUMENTED_FILE"
        
        if [[ -f "$NEW_ROUTES_FILE" ]]; then
            NEW_UNDOCUMENTED=$(wc -l < "$NEW_ROUTES_FILE" | tr -d ' ')
        fi
        
        if [[ $NEW_UNDOCUMENTED -gt 0 ]]; then
            echo ""
            echo -e "${RED}ERROR: Found $NEW_UNDOCUMENTED new undocumented routes (not in allowlist):${NC}"
            cat "$NEW_ROUTES_FILE"
            echo ""
            echo "Please document these routes in $OPENAPI_FILE or add them to $ALLOWLIST_FILE"
            rm -f "$ROUTES_FILE" "$OPENAPI_PATHS_FILE" "$UNDOCUMENTED_FILE" "$NEW_ROUTES_FILE"
            exit 1
        else
            echo -e "${GREEN}OK: All undocumented routes are in the allowlist.${NC}"
            echo "Allowlisted routes: $((UNDOCUMENTED_COUNT - NEW_UNDOCUMENTED))"
        fi
        
        rm -f "$NEW_ROUTES_FILE"
    else
        echo -e "${YELLOW}WARNING: No allowlist file found at $ALLOWLIST_FILE${NC}"
        echo "Creating initial allowlist with all undocumented routes..."
        cp "$UNDOCUMENTED_FILE" "$ALLOWLIST_FILE"
        echo "Allowlist created: $ALLOWLIST_FILE"
        echo ""
        echo -e "${YELLOW}Please review and document these routes in $OPENAPI_FILE${NC}"
        echo "Or keep them in the allowlist if they are internal/deprecated."
    fi
else
    echo -e "${GREEN}SUCCESS: All routes are documented in OpenAPI spec!${NC}"
fi

# Cleanup
rm -f "$ROUTES_FILE" "$OPENAPI_PATHS_FILE" "$UNDOCUMENTED_FILE"

exit 0
