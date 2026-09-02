#!/usr/bin/env bash
# add-plugin.sh — Scaffold a new PEPA plugin.
#
# Usage:
#   ./scripts/add-plugin.sh <name> [--type <type>] [--description <desc>]
#
# Example:
#   ./scripts/add-plugin.sh myservice --type notification --description "Send alerts to MyService"
#
# Creates:
#   plugins/<name>/main.go          — Go source with plugin skeleton
#   plugins/builtin/<name>/plugin.yaml — Manifest for Marketplace
#
# After running:
#   1. Edit the generated files to implement your plugin logic
#   2. Run: make plugins          (build binary)
#   3. Run: make sign-plugins     (sign with Ed25519 key)
#   4. Run: make verify-plugins   (confirm signature)
#   5. Restart: docker compose restart api-server

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# ── Parse arguments ──────────────────────────────────────────────

PLUGIN_NAME=""
PLUGIN_TYPE="integration"
PLUGIN_DESC=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --type)
            PLUGIN_TYPE="$2"
            shift 2
            ;;
        --description|--desc)
            PLUGIN_DESC="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 <name> [--type <type>] [--description <desc>]"
            echo ""
            echo "  <name>         Plugin name (lowercase, no spaces, e.g. myservice)"
            echo "  --type         Plugin type: notification, logging, integration,"
            echo "                 monitoring, cicd, infrastructure, security (default: integration)"
            echo "  --description  Short description of the plugin"
            echo ""
            echo "Examples:"
            echo "  $0 pagerduty --type notification --description 'Alert via PagerDuty'"
            echo "  $0 datadog --type monitoring --description 'Send metrics to Datadog'"
            exit 0
            ;;
        -*)
            echo "ERROR: Unknown option: $1" >&2
            exit 1
            ;;
        *)
            PLUGIN_NAME="$1"
            shift
            ;;
    esac
done

if [[ -z "$PLUGIN_NAME" ]]; then
    echo "ERROR: Plugin name is required." >&2
    echo "Usage: $0 <name> [--type <type>] [--description <desc>]" >&2
    exit 1
fi

# Validate name (lowercase, alphanumeric + hyphens)
if [[ ! "$PLUGIN_NAME" =~ ^[a-z][a-z0-9-]*$ ]]; then
    echo "ERROR: Plugin name must be lowercase alphanumeric with hyphens, starting with a letter." >&2
    exit 1
fi

if [[ -z "$PLUGIN_DESC" ]]; then
    PLUGIN_DESC="PEPA ${PLUGIN_NAME} plugin"
fi

# Convert name to Go identifiers (e.g. my-service -> MyService)
go_name() {
    echo "$1" | sed -E 's/(^|-)([a-z])/\U\2/g'
}

PLUGIN_STRUCT_NAME="$(go_name "$PLUGIN_NAME")Plugin"
SRC_DIR="${PROJECT_DIR}/plugins/${PLUGIN_NAME}"
YAML_DIR="${PROJECT_DIR}/plugins/builtin/${PLUGIN_NAME}"

# ── Preflight checks ────────────────────────────────────────────

if [[ -d "$SRC_DIR" ]]; then
    echo "ERROR: Source directory already exists: $SRC_DIR" >&2
    exit 1
fi

if [[ -d "$YAML_DIR" ]]; then
    echo "ERROR: Metadata directory already exists: $YAML_DIR" >&2
    exit 1
fi

# ── Create source directory ──────────────────────────────────────

echo "→ Creating plugin source: $SRC_DIR"
mkdir -p "$SRC_DIR"

cat > "$SRC_DIR/main.go" << GOEOF
// PEPA ${PLUGIN_NAME} plugin — ${PLUGIN_DESC}
package main

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// ${PLUGIN_STRUCT_NAME} implements provider.Provider for ${PLUGIN_NAME}.
type ${PLUGIN_STRUCT_NAME} struct {
	config map[string]string
}

var _ provider.Provider = (*${PLUGIN_STRUCT_NAME})(nil)

func (p *${PLUGIN_STRUCT_NAME}) Name() string    { return "${PLUGIN_NAME}" }
func (p *${PLUGIN_STRUCT_NAME}) Version() string { return "0.1.0" }
func (p *${PLUGIN_STRUCT_NAME}) Description() string {
	return "${PLUGIN_DESC}"
}
func (p *${PLUGIN_STRUCT_NAME}) PluginType() string { return "${PLUGIN_TYPE}" }

func (p *${PLUGIN_STRUCT_NAME}) Actions() []string {
	return []string{
		"ping",
	}
}

func (p *${PLUGIN_STRUCT_NAME}) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	p.config = config

	switch action {
	case "ping":
		return p.ping(ctx, params, config)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *${PLUGIN_STRUCT_NAME}) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "${PLUGIN_STRUCT_NAME} ready",
	}, nil
}

func (p *${PLUGIN_STRUCT_NAME}) ping(ctx context.Context, params []byte, config map[string]string) ([]byte, error) {
	// TODO: Implement plugin logic here
	return json.Marshal(map[string]string{
		"status":  "ok",
		"message": "pong from ${PLUGIN_NAME}",
	})
}

func main() {
	sdk.Serve(&${PLUGIN_STRUCT_NAME}{})
}
GOEOF

echo "  Created $SRC_DIR/main.go"

# ── Create metadata directory ────────────────────────────────────

echo "→ Creating plugin metadata: $YAML_DIR"
mkdir -p "$YAML_DIR"

cat > "$YAML_DIR/plugin.yaml" << YAMLEOF
name: ${PLUGIN_NAME}
version: "0.1.0"
display_name: "${PLUGIN_STRUCT_NAME}"
description: "${PLUGIN_DESC}"
category: ${PLUGIN_TYPE}
type: builtin
author: PEPA Team
license: Apache-2.0

config_schema:
  type: object
  properties:
    # TODO: Add configuration properties
    # api_key:
    #   type: string
    #   description: "API key for authentication"
  required: []

actions:
  - name: ping
    description: "Test connectivity"

health_check:
  interval: 30s
YAMLEOF

echo "  Created $YAML_DIR/plugin.yaml"

# ── Summary ──────────────────────────────────────────────────────

echo ""
echo "✓ Plugin scaffolded: ${PLUGIN_NAME}"
echo ""
echo "Next steps:"
echo "  1. Edit plugins/${PLUGIN_NAME}/main.go — implement your plugin logic"
echo "  2. Edit plugins/builtin/${PLUGIN_NAME}/plugin.yaml — configure schema & actions"
echo "  3. Build:       make plugins"
echo "  4. Sign:        make sign-plugins    (requires private key)"
echo "  5. Verify:      make verify-plugins"
echo "  6. Restart:     docker compose restart api-server"
echo ""
