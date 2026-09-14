#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# PEPA — Bulk Plugin Installer
# ============================================================
# Installs all built-in plugins via the Marketplace API.
# Reads plugin versions from plugins/builtin/*/plugin.yaml so
# the versions stay in sync with the source of truth.
#
# Usage:
#   ./scripts/install-plugins.sh                         # defaults
#   PEPA_URL=http://host:8080 PEPA_TOKEN=xxx ./scripts/install-plugins.sh
#
# Prerequisites:
#   - api-server is running and healthy
#   - PEPA_TOKEN is a valid admin JWT (from login or bootstrap)
#   - Plugin binaries are built (make plugins) or embedded
# ============================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILTIN_DIR="$PROJECT_DIR/plugins/builtin"

PEPA_URL="${PEPA_URL:-http://localhost:8080}"
PEPA_TOKEN="${PEPA_TOKEN:-}"

# ── Helpers ──────────────────────────────────────────────────

log()  { printf '\033[1;34m[install]\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m  ✓\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m  ⚠\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m  ✗\033[0m %s\n' "$*" >&2; }

# ── Pre-flight ───────────────────────────────────────────────

if [ -z "$PEPA_TOKEN" ]; then
  fail "PEPA_TOKEN is not set."
  echo ""
  echo "  Obtain an admin JWT via the login API or use the bootstrap token:"
  echo "    curl -s -X POST $PEPA_URL/api/v1/auth/login \\"
  echo "      -H 'Content-Type: application/json' \\"
  echo "      -d '{\"username\":\"admin\",\"password\":\"...\"}' | jq -r .token"
  echo ""
  exit 1
fi

if [ ! -d "$BUILTIN_DIR" ]; then
  fail "Built-in plugins directory not found: $BUILTIN_DIR"
  exit 1
fi

# Check api-server is reachable.
if ! curl -sf -o /dev/null --max-time 5 "$PEPA_URL/api/v1/marketplace" \
     -H "Authorization: Bearer $PEPA_TOKEN"; then
  fail "Cannot reach api-server at $PEPA_URL (or token is invalid)"
  exit 1
fi
ok "api-server reachable at $PEPA_URL"

# ── Discover & Install ───────────────────────────────────────

installed=0
skipped=0
failed=0

for plugin_dir in "$BUILTIN_DIR"/*/; do
  [ -d "$plugin_dir" ] || continue

  yaml_file="$plugin_dir/plugin.yaml"
  if [ ! -f "$yaml_file" ]; then
    continue  # no plugin.yaml → not a marketplace plugin (e.g. ai_bot, prometheus)
  fi

  # Extract name and version from YAML (simple grep — no yq dependency).
  name=$(grep '^name:' "$yaml_file" | head -1 | awk '{print $2}' | tr -d '"' | tr -d "'")
  version=$(grep '^version:' "$yaml_file" | head -1 | awk '{print $2}' | tr -d '"' | tr -d "'")

  if [ -z "$name" ]; then
    warn "skip $(basename "$plugin_dir"): no name in plugin.yaml"
    skipped=$((skipped + 1))
    continue
  fi

  log "Installing $name (version ${version:-unknown})..."

  http_code=$(curl -sf -o /tmp/pepa-install-resp.json -w '%{http_code}' \
    -X POST "$PEPA_URL/api/v1/marketplace/$name/install" \
    -H "Authorization: Bearer $PEPA_TOKEN" \
    -H "Content-Type: application/json" \
    --max-time 30 2>/dev/null) || http_code="000"

  case "$http_code" in
    200)
      ok "$name installed"
      installed=$((installed + 1))
      ;;
    400)
      # "plugin already installed" — not an error
      warn "$name already installed"
      skipped=$((skipped + 1))
      ;;
    409)
      warn "$name binary not available (build with 'make plugins')"
      skipped=$((skipped + 1))
      ;;
    *)
      resp=$(cat /tmp/pepa-install-resp.json 2>/dev/null || echo "")
      fail "$name install failed (HTTP $http_code): $resp"
      failed=$((failed + 1))
      ;;
  esac
done

# ── Summary ──────────────────────────────────────────────────

echo ""
log "────────────────────────────────"
log "Installed: $installed | Skipped: $skipped | Failed: $failed"

if [ "$failed" -gt 0 ]; then
  fail "$failed plugin(s) failed to install"
  exit 1
fi

ok "All plugins installed successfully"
