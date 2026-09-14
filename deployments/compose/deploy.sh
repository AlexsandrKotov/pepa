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
#   ./deploy.sh --fresh --keep-trivy-cache
#                         # Clean start but keep the Trivy vulnerability DB
#                         # (saves re-downloading ~2-3 GB)
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
  # --ignore-buildable: services that are built from this source tree (the dev
  # override adds db-cache, and CI never publishes it) must not abort the pull.
  # A partial pull is not fatal — the build step below produces those images.
  $COMPOSE_CMD pull --ignore-buildable \
    || warn "Registry pull incomplete (some images are not published); continuing"
  ok "Images pulled"
fi

# ── Build local images when the compose files declare build contexts ──
# Pulling first retags :latest to whatever CI published, which would silently
# deploy a different revision than this working tree. Anything buildable is
# rebuilt so the containers match the source that is checked out.
BUILDABLE="$($COMPOSE_CMD config --services --buildable 2>/dev/null || true)"
if [ -n "$BUILDABLE" ]; then
  log "Building local images for: $(echo "$BUILDABLE" | tr '\n' ' ')"
  $COMPOSE_CMD build $BUILDABLE
  ok "Local images built"
fi

# ── Setup .env ────────────────────────────────────────────────
if [ ! -f .env ]; then
  warn ".env not found. Copy .env.example or create one."
  exit 1
fi
ok ".env found"

# ── TLS certificate ───────────────────────────────────────────
# nginx mounts ./nginx/ssl for TLS termination, so a key pair must exist.
# Private keys are never committed — generate a self-signed pair on first run.
if [ ! -f "nginx/ssl/key.pem" ] || [ ! -f "nginx/ssl/cert.pem" ]; then
  log "No TLS certificate found. Generating self-signed certificate..."
  mkdir -p nginx/ssl
  openssl req -x509 -nodes -days 365 \
    -newkey rsa:2048 \
    -keyout nginx/ssl/key.pem \
    -out nginx/ssl/cert.pem \
    -subj "/C=US/ST=State/L=City/O=PEPA/CN=localhost" \
    2>/dev/null
  chmod 600 nginx/ssl/key.pem
  ok "Self-signed certificate generated"
else
  ok "TLS certificate found in nginx/ssl"
fi

# ── Stop existing stack ───────────────────────────────────────
log "Stopping any existing PEPA stack..."
$COMPOSE_CMD down --remove-orphans 2>/dev/null || true

# ── Handle --fresh flag (clean volumes) ───────────────────────
FRESH=false
KEEP_TRIVY_CACHE=false
for arg in "$@"; do
  case $arg in
    --fresh) FRESH=true ;;
    --keep-trivy-cache) KEEP_TRIVY_CACHE=true ;;
  esac
done

if $FRESH; then
  if $KEEP_TRIVY_CACHE; then
    warn "--fresh with --keep-trivy-cache: removing data volumes, keeping the Trivy DB"
    # down -v would take trivy-cache too, so stop the stack and delete only the
    # volumes that actually hold install state.
    $COMPOSE_CMD down --remove-orphans 2>/dev/null || true
    for vol in postgres-data redis-data custom-plugins nginx-logs pepa-token-tmpfs; do
      docker volume rm "pepa_${vol}" >/dev/null 2>&1 || true
    done
  else
    warn "--fresh flag set: removing all data volumes!"
    warn "This includes trivy-cache — the ~2-3 GB vulnerability DB is re-downloaded on boot."
    warn "Use --keep-trivy-cache to preserve it."
    $COMPOSE_CMD down -v --remove-orphans 2>/dev/null || true
  fi
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
