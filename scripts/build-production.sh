#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# PEPA — Production Build & Package Script
# ============================================================
# Builds Docker images and creates a self-contained tar archive
# for quick production deployment on any Docker host.
#
# Usage: ./scripts/build-production.sh [OUTPUT_DIR]
#
# Output: pepa-production-<version>.tar.gz containing:
#   - Pre-built Docker images (tar)
#   - docker-compose.yml (no build required)
#   - .env with auto-generated secrets
#   - Nginx configs, init-db.sql
#   - deploy.sh for one-command deployment
# ============================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
OUTPUT_DIR="${1:-$PROJECT_DIR/dist}"
VERSION="${VERSION:-$(git -C "$PROJECT_DIR" describe --tags --always --dirty 2>/dev/null || echo "dev")}"
BUILD_TIME="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
PACKAGE_NAME="pepa-production-${VERSION}"
PACKAGE_DIR="${OUTPUT_DIR}/${PACKAGE_NAME}"

# Tool versions — single source of truth
# shellcheck disable=SC1091
source "$PROJECT_DIR/.env.versions"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log()  { echo -e "${BLUE}[PEPA]${NC} $*"; }
ok()   { echo -e "${GREEN}[OK]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()  { echo -e "${RED}[ERROR]${NC} $*"; }
step() { echo -e "\n${CYAN}==> $*${NC}"; }

# ── Preflight checks ─────────────────────────────────────────
step "Preflight checks"

if ! command -v docker &> /dev/null; then
  err "Docker is not installed."
  exit 1
fi

if ! docker info &> /dev/null; then
  err "Docker daemon is not running."
  exit 1
fi
ok "Docker is running"

# ── Clean output directory ────────────────────────────────────
step "Preparing output directory"
rm -rf "$PACKAGE_DIR"
mkdir -p "$PACKAGE_DIR/images"
ok "Created $PACKAGE_DIR"

# ── Detect target architecture ────────────────────────────────
step "Detecting target architecture"

# Detect host architecture so plugin binaries match the Docker images.
HOST_ARCH="$(uname -m)"
case "$HOST_ARCH" in
  x86_64|amd64)  PLUGIN_ARCH="amd64" ;;
  aarch64|arm64)  PLUGIN_ARCH="arm64" ;;
  *)              PLUGIN_ARCH="amd64"; warn "Unknown arch $HOST_ARCH, defaulting to amd64" ;;
esac
ok "Host architecture: $HOST_ARCH → plugin arch: $PLUGIN_ARCH"

# ── Build plugins for target architecture ─────────────────────
step "Building plugins (linux/$PLUGIN_ARCH)"

make -C "$PROJECT_DIR" plugins PLUGIN_GOARCH="$PLUGIN_ARCH"
ok "Plugins built for linux/$PLUGIN_ARCH"

# ── Sign plugins ──────────────────────────────────────────────
step "Signing plugins"

make -C "$PROJECT_DIR" sign-plugins
ok "Plugins signed"

# ── Build Docker images ───────────────────────────────────────
step "Building Docker images"

log "Building API server image..."
docker build \
  -f "$PROJECT_DIR/deployments/docker/Dockerfile.api" \
  --build-arg HELM_VERSION="${HELM_VERSION}" \
  --build-arg OPENTOFU_VERSION="${OPENTOFU_VERSION}" \
  --build-arg TRIVY_VERSION="${TRIVY_VERSION}" \
  -t "ghcr.io/alexsandrkotov/pepa/pepa-api-server:${VERSION}" \
  -t "ghcr.io/alexsandrkotov/pepa/pepa-api-server:latest" \
  "$PROJECT_DIR"

log "Building Worker image..."
docker build \
  -f "$PROJECT_DIR/deployments/docker/Dockerfile.worker" \
  -t "ghcr.io/alexsandrkotov/pepa/pepa-worker:${VERSION}" \
  -t "ghcr.io/alexsandrkotov/pepa/pepa-worker:latest" \
  "$PROJECT_DIR"

log "Building Frontend image..."
docker build \
  -f "$PROJECT_DIR/deployments/docker/Dockerfile.frontend" \
  --build-arg NEXT_PUBLIC_API_URL="" \
  -t "ghcr.io/alexsandrkotov/pepa/pepa-frontend:${VERSION}" \
  -t "ghcr.io/alexsandrkotov/pepa/pepa-frontend:latest" \
  "$PROJECT_DIR/frontend"

ok "All images built successfully"

# ── Export images to tar ──────────────────────────────────────
step "Exporting Docker images"

log "Exporting pepa-api-server..."
docker save "ghcr.io/alexsandrkotov/pepa/pepa-api-server:${VERSION}" | gzip > "$PACKAGE_DIR/images/api-server.tar.gz"

log "Exporting pepa-worker..."
docker save "ghcr.io/alexsandrkotov/pepa/pepa-worker:${VERSION}" | gzip > "$PACKAGE_DIR/images/worker.tar.gz"

log "Exporting pepa-frontend..."
docker save "ghcr.io/alexsandrkotov/pepa/pepa-frontend:${VERSION}" | gzip > "$PACKAGE_DIR/images/frontend.tar.gz"

ok "Images exported"

# ── Generate production secrets ───────────────────────────────
step "Generating production secrets"

generate_secret() {
  openssl rand -hex "${1:-32}" 2>/dev/null || head -c "$(( ${1:-32} * 2 ))" /dev/urandom | od -An -tx1 | tr -d ' \n'
}

JWT_SECRET="$(generate_secret 32)"
ENCRYPTION_KEY="$(generate_secret 32)"
POSTGRES_PASSWORD="$(generate_secret 16)"
REDIS_PASSWORD="$(generate_secret 16)"

ok "Secrets generated"

# ── Create .env file ──────────────────────────────────────────
step "Creating .env file"

cat > "$PACKAGE_DIR/.env" << EOF
# ============================================================
# PEPA Production Environment
# ============================================================
# Generated: ${BUILD_TIME}
# Version: ${VERSION}
#
# IMPORTANT: Review and customize these values before deployment!
# ============================================================

# ── Server ───────────────────────────────────────────────────
SERVER_ENV=production

# ── PostgreSQL ───────────────────────────────────────────────
POSTGRES_DB=pepa
POSTGRES_USER=pepa
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
POSTGRES_PORT=5432

# ── Redis ────────────────────────────────────────────────────
REDIS_PASSWORD=${REDIS_PASSWORD}
REDIS_PORT=6379

# ── Authentication ───────────────────────────────────────────
AUTH_JWT_SECRET=${JWT_SECRET}

# ── Encryption ───────────────────────────────────────────────
ENCRYPTION_KEY=${ENCRYPTION_KEY}

# ── Ports (host mapping) ────────────────────────────────────
API_PORT=8088
FRONTEND_PORT=3000

# ── CORS ─────────────────────────────────────────────────────
# Adjust to your actual domain(s)
CORS_ORIGINS=https://localhost,https://127.0.0.1

# ── Plugin Security ───────────────────────────────────────────
PLUGIN_SIGNATURE_VERIFY=true
PLUGIN_SIGNATURE_ENFORCE=true

# ── Vault ────────────────────────────────────────────────────
VAULT_ALLOW_PRIVATE_IPS=false

# ── Docker Socket (optional) ─────────────────────────────────
# Set to /var/run/docker.sock ONLY if you need Docker host management
MOUNT_DOCK_SOCKET=/dev/null
EOF

ok ".env created with auto-generated secrets"

# ── Copy docker-compose.yml (single source of truth) ──────────
step "Copying production docker-compose.yml"

# The base docker-compose.yml is production-ready: no build contexts,
# uses pre-built images. Development overrides live in
# docker-compose.override.yml (gitignored, see .example).
cp "$PROJECT_DIR/deployments/compose/docker-compose.yml" "$PACKAGE_DIR/docker-compose.yml"

ok "docker-compose.yml copied"

# ── Verify plugin signatures ──────────────────────────────────
step "Verifying plugin signatures"

PUBLIC_KEY_PATH="$PROJECT_DIR/internal/plugin/signature/pepa-plugins-public.pem"
if [ -d "$PROJECT_DIR/plugins/bin" ] && [ "$(ls -A "$PROJECT_DIR/plugins/bin" 2>/dev/null)" ]; then
  if [ -f "$PUBLIC_KEY_PATH" ]; then
    if bash "$PROJECT_DIR/scripts/sign-plugin.sh" --verify; then
      ok "All plugin signatures verified"
    else
      warn "Plugin signature verification failed. Plugins will be included unsigned."
    fi
  else
    warn "No signing key found at $PUBLIC_KEY_PATH. Skipping signature verification."
  fi
else
  warn "No plugin binaries found. Run 'make plugins' first."
fi

# ── Copy plugins ──────────────────────────────────────────────
step "Copying plugins"

# Copy plugin binaries (plugins/bin/<name>/<name> — flat structure)
if [ -d "$PROJECT_DIR/plugins/bin" ] && [ "$(ls -A "$PROJECT_DIR/plugins/bin" 2>/dev/null)" ]; then
  mkdir -p "$PACKAGE_DIR/plugins/bin"
  cp -r "$PROJECT_DIR/plugins/bin/"* "$PACKAGE_DIR/plugins/bin/"
  PLUGIN_BIN_COUNT=$(ls -1 "$PACKAGE_DIR/plugins/bin" | wc -l | tr -d ' ')
  ok "$PLUGIN_BIN_COUNT plugin binaries copied"
else
  warn "No plugin binaries found. Run 'make plugins' first."
fi

# Copy plugin definitions (plugin.yaml) — required for Marketplace
if [ -d "$PROJECT_DIR/plugins/builtin" ]; then
  mkdir -p "$PACKAGE_DIR/plugins/builtin"
  cp -r "$PROJECT_DIR/plugins/builtin/"* "$PACKAGE_DIR/plugins/builtin/"
  BUILTIN_DEF_COUNT=$(ls -1 "$PACKAGE_DIR/plugins/builtin" | wc -l | tr -d ' ')
  ok "$BUILTIN_DEF_COUNT plugin definitions copied"
fi

# ── Copy supporting files ─────────────────────────────────────
step "Copying configuration files"

# Nginx config — single source of truth in deployments/compose/nginx/.
# Uses Docker embedded DNS resolver for runtime upstream resolution.
mkdir -p "$PACKAGE_DIR/nginx/ssl"
cp "$PROJECT_DIR/deployments/compose/nginx/nginx.conf" "$PACKAGE_DIR/nginx/nginx.conf"

# Init DB
cp "$PROJECT_DIR/deployments/compose/init-db.sql" "$PACKAGE_DIR/init-db.sql"

ok "Configuration files copied"

# ── Generate self-signed SSL cert ─────────────────────────────
step "Generating self-signed SSL certificate"

openssl req -x509 -nodes -days 365 \
  -newkey rsa:2048 \
  -keyout "$PACKAGE_DIR/nginx/ssl/key.pem" \
  -out "$PACKAGE_DIR/nginx/ssl/cert.pem" \
  -subj "/C=US/ST=State/L=City/O=PEPA/CN=localhost" \
  2>/dev/null

ok "Self-signed SSL certificate generated"

# ── Copy deploy.sh and stop.sh ────────────────────────────────
step "Copying deployment scripts"

cp "$PROJECT_DIR/deployments/compose/deploy.sh" "$PACKAGE_DIR/deploy.sh"
chmod +x "$PACKAGE_DIR/deploy.sh"

cp "$PROJECT_DIR/deployments/compose/stop.sh" "$PACKAGE_DIR/stop.sh"
chmod +x "$PACKAGE_DIR/stop.sh"

ok "Deployment scripts copied"

# ── Create README ─────────────────────────────────────────────
step "Creating README"

cat > "$PACKAGE_DIR/README.md" << EOF
# PEPA Production Deployment Package

**Version:** ${VERSION}
**Build Time:** ${BUILD_TIME}

## Quick Start

\`\`\`bash
# 1. Extract the archive
tar xzf pepa-production-*.tar.gz
cd pepa-production-${VERSION}

# 2. (Optional) Review/edit .env — secrets are auto-generated
# vim .env

# 3. Deploy
./deploy.sh
\`\`\`

That's it! The script will:
- Load all Docker images
- Start PostgreSQL, Redis, API Server, Worker, Frontend, Nginx
- Wait for health checks
- Print access URLs

## Access

| Service    | URL                              |
|------------|----------------------------------|
| Frontend   | https://localhost                |
| API        | https://localhost/api/           |
| API direct | http://localhost:${API_PORT:-8088}           |
| PostgreSQL | localhost:${POSTGRES_PORT:-5432}               |
| Redis      | localhost:${REDIS_PORT:-6379}                 |

## SSL Certificate

A self-signed certificate is included for quick start.
For production, replace \`nginx/ssl/cert.pem\` and \`nginx/ssl/key.pem\`
with a real certificate (e.g. from Let's Encrypt).

## Configuration

All settings are in \`.env\`. Key variables:

| Variable | Description |
|----------|-------------|
| \`POSTGRES_PASSWORD\` | PostgreSQL password |
| \`REDIS_PASSWORD\` | Redis password |
| \`AUTH_JWT_SECRET\` | JWT signing secret |
| \`ENCRYPTION_KEY\` | Vault encryption key |
| \`CORS_ORIGINS\` | Allowed CORS origins |
| \`API_PORT\` | Host port for API |
| \`FRONTEND_PORT\` | Host port for Frontend |

## Managing

\`\`\`bash
# View logs
docker compose logs -f

# Restart a service
docker compose restart api-server

# Stop everything
./stop.sh

# Full reset (removes data!)
docker compose down -v
./deploy.sh
\`\`\`

## System Requirements

- Docker 24+
- Docker Compose v2+
- 4 GB RAM minimum (8 GB recommended)
- 10 GB disk space

## Adding Plugins

Place compiled plugin binaries into the \`plugins/\` directory:

\`\`\`bash
mkdir -p plugins/bin/myplugin
cp myplugin-binary plugins/bin/myplugin/myplugin
docker compose restart api-server worker
\`\`\`

## Troubleshooting

\`\`\`bash
# Check service status
docker compose ps

# View specific service logs
docker compose logs -f api-server

# Check API health
curl -k https://localhost/healthz

# Restart everything
docker compose down && ./deploy.sh
\`\`\`
EOF

ok "README.md created"

# ── Create tar archive ────────────────────────────────────────
step "Creating tar archive"

cd "$OUTPUT_DIR"
tar czf "${PACKAGE_NAME}.tar.gz" "$PACKAGE_NAME"

ARCHIVE_SIZE="$(du -sh "${PACKAGE_NAME}.tar.gz" | cut -f1)"

# ── Cleanup extracted dir ─────────────────────────────────────
rm -rf "$PACKAGE_NAME"

echo ""
echo -e "${GREEN}════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}  PEPA Production Package Built Successfully!${NC}"
echo -e "${GREEN}════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "  ${BLUE}Archive:${NC}  ${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz"
echo -e "  ${BLUE}Size:${NC}     ${ARCHIVE_SIZE}"
echo -e "  ${BLUE}Version:${NC}  ${VERSION}"
echo ""
echo -e "  ${YELLOW}Deploy on target server:${NC}"
echo "    scp ${PACKAGE_NAME}.tar.gz user@server:/opt/"
echo "    ssh user@server 'cd /opt && tar xzf ${PACKAGE_NAME}.tar.gz && cd ${PACKAGE_NAME} && ./deploy.sh'"
echo ""
