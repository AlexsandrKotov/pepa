#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
if docker compose version &> /dev/null; then COMPOSE_CMD="docker compose"; else COMPOSE_CMD="docker-compose"; fi
echo "Stopping PEPA..."
$COMPOSE_CMD down
echo "PEPA stopped."
