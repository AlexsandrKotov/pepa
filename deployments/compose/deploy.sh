#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# PEPA — One-Command Production Deployment
# ============================================================
# Works with both:
#   - Production archive (pre-built images in images/*.tar.gz)
#   - Source tree (pulls from GHCR registry)
#
# Usage:
#   ./deploy.sh           # Load embedded images (archive) or pull
#   ./deploy.sh --fresh   # Clean start (removes all data volumes)
# ============================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log()  { echo -e "${BLUE}[PEPA]${NC} $*"; }
ok()   { echo -e "${GREEN}[OK]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()  { echo -e "${RED}[ERROR]${NC} $*"; }

echo ""
echo -e "${BLUE}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║   PEPA — Platform Engineering & Pipeline Automator  ║${NC}"
echo -e "${BLUE}║              Production Deployment                  ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════╝${NC}"
echo ""

# ── Preflight ─────────────────────────────────────────────────
if ! command -v docker &> /dev/null; then
  err "Docker is not installed. Install Docker first: https://docs.docker.com/engine/install/"
  exit 1
fi

if ! docker info &> /dev/null; then
  err "Docker daemon is not running."
  exit 1
fi

if docker compose version &> /dev/null; then
  COMPOSE_CMD="docker compose"
elif command -v docker-compose &> /dev/null; then
  COMPOSE_CMD="docker-compose"
else
  err "Docker Compose is not available."
  exit 1
fi

ok "Docker & Compose detected ($COMPOSE_CMD)"

# ── Load images ───────────────────────────────────────────────
# If pre-built image archives exist (production archive), load them.
# Otherwise, pull from registry (source tree deployment).
if ls images/*.tar.gz 1>/dev/null 2>&1; then
  log "Loading Docker images from archive..."
  for img in images/*.tar.gz; do
    [ -f "$img" ] || continue
    name="$(basename "$img" .tar.gz)"
    log "  Loading $name..."
    gunzip -c "$img" | docker load
  done
  ok "All images loaded from archive"
else
  log "Pulling Docker images from registry..."
  $COMPOSE_CMD pull
  ok "Images pulled"
fi

# ── Setup .env ────────────────────────────────────────────────
if [ ! -f .env ]; then
  warn ".env not found. Copy .env.example or create one."
  exit 1
fi
ok ".env found"

# ── Stop existing stack ───────────────────────────────────────
log "Stopping any existing PEPA stack..."
$COMPOSE_CMD down --remove-orphans 2>/dev/null || true

# ── Handle --fresh flag (clean volumes) ───────────────────────
FRESH=false
for arg in "$@"; do
  case $arg in
    --fresh) FRESH=true ;;
  esac
done

if $FRESH; then
  warn "--fresh flag set: removing all data volumes!"
  $COMPOSE_CMD down -v --remove-orphans 2>/dev/null || true
  ok "Volumes removed"
fi

# ── Check for stale Postgres credentials ──────────────────────
if docker volume inspect pepa_postgres-data &> /dev/null 2>&1; then
  log "Existing postgres volume detected."
  log "If API fails to connect, run: ./deploy.sh --fresh"
fi

# ── Start services ────────────────────────────────────────────
log "Starting PEPA services..."
$COMPOSE_CMD up -d

# ── Wait for readiness ────────────────────────────────────────
log "Waiting for services to become ready..."

MAX_RETRIES=60
RETRY=0
while [ $RETRY -lt $MAX_RETRIES ]; do
  if curl -sf http://localhost:${API_PORT:-8088}/healthz > /dev/null 2>&1; then
    break
  fi
  RETRY=$((RETRY + 1))
  sleep 2
done

echo ""
if [ $RETRY -eq $MAX_RETRIES ]; then
  warn "API server did not become ready in time. Check logs: $COMPOSE_CMD logs api-server"
else
  ok "PEPA is up and running!"
fi

# ── Print access info ─────────────────────────────────────────
echo ""
echo -e "${GREEN}════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}  PEPA Deployment Complete${NC}"
echo -e "${GREEN}════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "  ${BLUE}Frontend:${NC}   https://localhost"
echo -e "  ${BLUE}API:${NC}        https://localhost/api/"
echo -e "  ${BLUE}API (direct):${NC} http://localhost:${API_PORT:-8088}"
echo -e "  ${BLUE}PostgreSQL:${NC} localhost:${POSTGRES_PORT:-5432}"
echo -e "  ${BLUE}Redis:${NC}      localhost:${REDIS_PORT:-6379}"
echo ""
if [ -d "nginx/ssl" ]; then
  echo -e "  ${YELLOW}Note:${NC} Using self-signed SSL certificate."
  echo -e "  For production, replace nginx/ssl/cert.pem and key.pem"
  echo -e "  with your domain certificate (Let's Encrypt, etc.)"
  echo ""
fi
echo -e "  ${BLUE}Useful commands:${NC}"
echo "    $COMPOSE_CMD logs -f          # Follow logs"
echo "    $COMPOSE_CMD ps               # Service status"
echo "    $COMPOSE_CMD restart api-server  # Restart a service"
echo "    $COMPOSE_CMD down             # Stop everything"
echo ""
