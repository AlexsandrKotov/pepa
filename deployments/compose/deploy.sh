#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
log()  { echo -e "\033[0;34m[PEPA]\033[0m $*"; }
ok()   { echo -e "\033[0;32m[OK]\033[0m $*"; }
warn() { echo -e "\033[1;33m[WARN]\033[0m $*"; }
err()  { echo -e "\033[0;31m[ERROR]\033[0m $*"; }
echo ""
echo -e "\033[0;34m  PEPA — Production Deployment\033[0m"
echo ""
if ! command -v docker &> /dev/null; then err "Docker is not installed."; exit 1; fi
if ! docker info &> /dev/null; then err "Docker daemon is not running."; exit 1; fi
if docker compose version &> /dev/null; then COMPOSE_CMD="docker compose"; elif command -v docker-compose &> /dev/null; then COMPOSE_CMD="docker-compose"; else err "Docker Compose not available."; exit 1; fi
ok "Docker & Compose detected ($COMPOSE_CMD)"
if [ ! -f .env ]; then warn ".env not found."; exit 1; fi
ok ".env found"
log "Stopping any existing PEPA stack..."
$COMPOSE_CMD down --remove-orphans 2>/dev/null || true
FRESH=false
for arg in "$@"; do case $arg in --fresh) FRESH=true ;; esac; done
if $FRESH; then
  warn "--fresh flag set: removing all data volumes!"
  $COMPOSE_CMD down -v --remove-orphans 2>/dev/null || true
fi
log "Pulling Docker images from GHCR..."
$COMPOSE_CMD pull
ok "Images pulled"
log "Starting PEPA services..."
$COMPOSE_CMD up -d
log "Waiting for services..."
MAX_RETRIES=60; RETRY=0
while [ $RETRY -lt $MAX_RETRIES ]; do
  if curl -sf http://localhost:${API_PORT:-8088}/healthz > /dev/null 2>&1; then break; fi
  RETRY=$((RETRY + 1)); sleep 2
done
if [ $RETRY -eq $MAX_RETRIES ]; then
  warn "API not ready. Check: $COMPOSE_CMD logs api-server"
else
  ok "PEPA is up and running!"
fi
echo ""
echo "  Frontend:   https://localhost"
echo "  API:        http://localhost:${API_PORT:-8088}"
echo ""
