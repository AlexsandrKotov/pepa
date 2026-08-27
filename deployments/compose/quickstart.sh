#!/usr/bin/env bash
set -euo pipefail

# PEPA — Platform Engineering & Pipeline Automator
# Quickstart script for development and demo environments

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.yml"

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

# ── Parse arguments ──────────────────────────────────────────
WITH_AI=false
DETACH=true

while [[ $# -gt 0 ]]; do
  case $1 in
    --all)        WITH_AI=true; shift ;;
    --ai)         WITH_AI=true; shift ;;
    --interactive) DETACH=false; shift ;;
    --help|-h)
      echo "Usage: quickstart.sh [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --all              Start everything (AI)"
      echo "  --ai               Include local LLM (Ollama)"
      echo "  --interactive      Run in foreground (don't detach)"
      echo "  --help, -h         Show this help"
      exit 0
      ;;
    *) err "Unknown option: $1"; exit 1 ;;
  esac
done

# ── Preflight checks ─────────────────────────────────────────
log "PEPA Quickstart"
log "================"
echo ""

# Check Docker
if ! command -v docker &> /dev/null; then
  err "Docker is not installed. Please install Docker first."
  exit 1
fi

if ! docker info &> /dev/null; then
  err "Docker daemon is not running. Please start Docker."
  exit 1
fi

ok "Docker is running"

# Check Docker Compose
if docker compose version &> /dev/null; then
  COMPOSE_CMD="docker compose"
elif command -v docker-compose &> /dev/null; then
  COMPOSE_CMD="docker-compose"
else
  err "Docker Compose is not available."
  exit 1
fi

ok "Docker Compose available ($COMPOSE_CMD)"

# ── Setup .env ────────────────────────────────────────────────
# Docker Compose reads .env from the same directory as the compose file.
COMPOSE_ENV="$SCRIPT_DIR/.env"
if [ ! -f "$COMPOSE_ENV" ]; then
  if [ -f "$SCRIPT_DIR/.env.example" ]; then
    cp "$SCRIPT_DIR/.env.example" "$COMPOSE_ENV"
    ok "Created deployments/compose/.env from .env.example"
  else
    warn "deployments/compose/.env.example not found, using defaults"
  fi
else
  ok "deployments/compose/.env already exists"
fi

# ── Build profiles ────────────────────────────────────────────
PROFILES=""
if $WITH_AI; then
  PROFILES="$PROFILES --profile ai"
  log "AI profile enabled (local LLM via Ollama)"
fi

# ── Start services ────────────────────────────────────────────
echo ""
log "Starting PEPA services..."
echo ""

cd "$SCRIPT_DIR"

# Build images
$COMPOSE_CMD -f "$COMPOSE_FILE" $PROFILES build

# Start services
if $DETACH; then
  $COMPOSE_CMD -f "$COMPOSE_FILE" $PROFILES up -d
else
  $COMPOSE_CMD -f "$COMPOSE_FILE" $PROFILES up
fi

# ── Wait for services ─────────────────────────────────────────
if $DETACH; then
  echo ""
  log "Waiting for services to be ready..."

  # Wait for API server
  MAX_RETRIES=30
  RETRY=0
  while [ $RETRY -lt $MAX_RETRIES ]; do
    if curl -sf http://localhost:8088/healthz > /dev/null 2>&1; then
      break
    fi
    RETRY=$((RETRY + 1))
    sleep 2
  done

  if [ $RETRY -eq $MAX_RETRIES ]; then
    warn "API server did not become ready in time"
  else
    ok "API server is ready"
  fi

  # Print status
  echo ""
  log "PEPA is running!"
  echo ""
  echo -e "  ${GREEN}API Server:${NC}  http://localhost:8088"
  echo -e "  ${GREEN}Frontend:${NC}    http://localhost:3000"
  echo -e "  ${GREEN}PostgreSQL:${NC}  localhost:5432"
  echo -e "  ${GREEN}Redis:${NC}       localhost:6379"

  echo ""
  log "Useful commands:"
  echo "  $COMPOSE_CMD -f $COMPOSE_FILE logs -f     # Follow logs"
  echo "  $COMPOSE_CMD -f $COMPOSE_FILE down        # Stop all services"
  echo "  $COMPOSE_CMD -f $COMPOSE_FILE ps          # Show service status"
  echo ""
fi
