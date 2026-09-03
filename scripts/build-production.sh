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

# ── Build Docker images ───────────────────────────────────────
step "Building Docker images"

log "Building API server image..."
docker build \
  -f "$PROJECT_DIR/deployments/docker/Dockerfile.api" \
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

# ── Create docker-compose.yml (production, no build) ──────────
step "Creating production docker-compose.yml"

cat > "$PACKAGE_DIR/docker-compose.yml" << 'COMPOSE_EOF'
# ============================================================
# PEPA — Production Docker Compose
# ============================================================
# This file uses pre-built images — no Dockerfile or source
# code required. Just load images and run.
#
# Usage:
#   docker compose up -d
# ============================================================

name: pepa

networks:
  pepa-net:
    driver: bridge

volumes:
  postgres-data:
    driver: local
  redis-data:
    driver: local
  custom-plugins:
    driver: local
  nginx-logs:
    driver: local

# ── Shared environment ──────────────────────────────────────
x-common-env: &common-env
  POSTGRES_HOST: postgres
  POSTGRES_PORT: "5432"
  POSTGRES_DB: ${POSTGRES_DB:-pepa}
  POSTGRES_USER: ${POSTGRES_USER:-pepa}
  POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
  POSTGRES_SSLMODE: disable
  REDIS_HOST: redis
  REDIS_PORT: "6379"
  REDIS_PASSWORD: ${REDIS_PASSWORD}
  ENCRYPTION_KEY: ${ENCRYPTION_KEY}
  AUTH_JWT_SECRET: ${AUTH_JWT_SECRET}
  PLUGIN_DIR: /plugins
  PLUGIN_SIGNATURE_VERIFY: "true"
  PLUGIN_SIGNATURE_ENFORCE: "true"
  VAULT_ALLOW_PRIVATE_IPS: ${VAULT_ALLOW_PRIVATE_IPS:-false}

services:

  # ── PostgreSQL + PGvector ──────────────────────────────────
  postgres:
    image: pgvector/pgvector:pg18
    container_name: pepa-postgres
    restart: unless-stopped
    environment:
      POSTGRES_DB: ${POSTGRES_DB:-pepa}
      POSTGRES_USER: ${POSTGRES_USER:-pepa}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      PGDATA: /var/lib/postgresql/data/pgdata
    ports:
      - "127.0.0.1:${POSTGRES_PORT:-5432}:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data
      - ./init-db.sql:/docker-entrypoint-initdb.d/01-init.sql:ro
    networks:
      - pepa-net
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-pepa} -d ${POSTGRES_DB:-pepa}"]
      interval: 5s
      timeout: 3s
      retries: 10
      start_period: 10s
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 2G
        reservations:
          cpus: '0.5'
          memory: 512M
    command: >
      postgres
        -c max_connections=200
        -c shared_buffers=256MB
        -c effective_cache_size=768MB
        -c work_mem=16MB
        -c maintenance_work_mem=128MB
        -c wal_buffers=8MB
        -c log_min_duration_statement=1000

  # ── Redis ──────────────────────────────────────────────────
  redis:
    image: redis:7.4-alpine
    container_name: pepa-redis
    restart: unless-stopped
    command: >
      redis-server
      --requirepass ${REDIS_PASSWORD}
      --maxmemory 200mb
      --maxmemory-policy allkeys-lru
      --appendonly yes
    ports:
      - "127.0.0.1:${REDIS_PORT:-6379}:6379"
    volumes:
      - redis-data:/data
    networks:
      - pepa-net
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "${REDIS_PASSWORD}", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 256M

  # ── PEPA API Server ────────────────────────────────────────
  api-server:
    image: ghcr.io/alexsandrkotov/pepa/pepa-api-server:latest
    container_name: pepa-api
    restart: unless-stopped
    environment:
      <<: *common-env
      SERVER_PORT: "8080"
      SERVER_HOST: "0.0.0.0"
      SERVER_ENV: production
      LOG_LEVEL: info
      CUSTOM_PLUGIN_DIR: /custom-plugins
      CORS_ORIGINS: ${CORS_ORIGINS:-https://localhost}
    ports:
      - "${API_PORT:-8088}:8080"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/healthz"]
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 30s
    volumes:
      - ./plugins:/plugins:ro
      - custom-plugins:/custom-plugins
      - ${MOUNT_DOCK_SOCKET:-/dev/null}:/var/run/docker.sock:ro
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    networks:
      - pepa-net
    tmpfs:
      - /tmp:size=256m,exec
    security_opt:
      - no-new-privileges:true
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 1G
        reservations:
          cpus: '0.5'
          memory: 256M

  # ── PEPA Background Worker ─────────────────────────────────
  worker:
    image: ghcr.io/alexsandrkotov/pepa/pepa-worker:latest
    container_name: pepa-worker
    restart: unless-stopped
    environment:
      <<: *common-env
      CUSTOM_PLUGIN_DIR: /custom-plugins
      SERVER_ENV: production
      LOG_LEVEL: info
    volumes:
      - ./plugins:/plugins:ro
      - custom-plugins:/custom-plugins
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    networks:
      - pepa-net
    read_only: true
    tmpfs:
      - /tmp:noexec,nosuid,size=64m
    security_opt:
      - no-new-privileges:true
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 512M
        reservations:
          cpus: '0.25'
          memory: 128M

  # ── Frontend (Next.js) ────────────────────────────────────
  frontend:
    image: ghcr.io/alexsandrkotov/pepa/pepa-frontend:latest
    container_name: pepa-frontend
    restart: unless-stopped
    ports:
      - "${FRONTEND_PORT:-3000}:3000"
    depends_on:
      - api-server
    networks:
      - pepa-net
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://127.0.0.1:3000/"]
      interval: 15s
      timeout: 5s
      retries: 3
      start_period: 10s
    tmpfs:
      - /var/cache/nginx:size=32m
      - /var/run:size=8m
    security_opt:
      - no-new-privileges:true
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 128M
        reservations:
          cpus: '0.1'
          memory: 32M

  # ── Nginx Reverse Proxy ────────────────────────────────────
  nginx:
    image: nginx:1.31-alpine
    container_name: pepa-nginx
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/ssl:/etc/nginx/ssl:ro
      - nginx-logs:/var/log/nginx
    depends_on:
      api-server:
        condition: service_started
      frontend:
        condition: service_healthy
    networks:
      - pepa-net
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://127.0.0.1/health"]
      interval: 10s
      timeout: 5s
      retries: 3
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 128M
COMPOSE_EOF

ok "docker-compose.yml created"

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

# Nginx config — generate a Docker-friendly version that uses
# the embedded DNS resolver so upstream names don't have to
# resolve at nginx startup (prevents "host not found" crashes).
mkdir -p "$PACKAGE_DIR/nginx/ssl"

cat > "$PACKAGE_DIR/nginx/nginx.conf" << 'NGINX_EOF'
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 1024;
    multi_accept on;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    # Logging
    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for" '
                    'rt=$request_time';
    access_log /var/log/nginx/access.log main;

    # Performance
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;
    client_max_body_size 50m;

    # Gzip
    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_comp_level 6;
    gzip_types text/plain text/css text/xml application/json application/javascript application/xml+rss application/atom+xml image/svg+xml;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    # Rate limiting
    limit_req_zone $binary_remote_addr zone=api:10m rate=30r/s;
    limit_req_zone $binary_remote_addr zone=login:10m rate=5r/m;

    # Docker embedded DNS — allows runtime resolution of container names
    # so nginx does NOT crash when an upstream is not yet available.
    resolver 127.0.0.11 valid=10s ipv6=off;

    # ── HTTP server (redirect to HTTPS) ──────────────────────
    server {
        listen 80;
        server_name _;

        location /health {
            access_log off;
            return 200 'OK';
            add_header Content-Type text/plain;
        }

        location / {
            return 301 https://$host$request_uri;
        }
    }

    # ── Main HTTPS Server ────────────────────────────────────
    server {
        listen 443 ssl;
        http2 on;
        server_name _;

        # SSL
        ssl_certificate /etc/nginx/ssl/cert.pem;
        ssl_certificate_key /etc/nginx/ssl/key.pem;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers HIGH:!aNULL:!MD5;
        ssl_prefer_server_ciphers on;

        # Security headers
        add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

        # ── AI endpoints (long timeouts for LLM inference) ──
        location /api/v1/ai/ {
            limit_req zone=api burst=20 nodelay;

            set $api_backend "http://api-server:8080";
            proxy_pass $api_backend;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_set_header Connection "";

            proxy_connect_timeout 30s;
            proxy_send_timeout 600s;
            proxy_read_timeout 600s;
        }

        # ── API routes ───────────────────────────────────────
        location /api/ {
            limit_req zone=api burst=50 nodelay;

            set $api_backend "http://api-server:8080";
            proxy_pass $api_backend;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_set_header Connection "";

            proxy_connect_timeout 10s;
            proxy_send_timeout 120s;
            proxy_read_timeout 120s;
        }

        # ── Auth endpoints (stricter rate limit) ─────────────
        location /api/v1/auth/ {
            limit_req zone=login burst=10 nodelay;

            set $api_backend "http://api-server:8080";
            proxy_pass $api_backend;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }

        # ── GraphQL endpoint ─────────────────────────────────
        location /graphql {
            limit_req zone=api burst=20 nodelay;

            set $api_backend "http://api-server:8080";
            proxy_pass $api_backend;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_set_header Connection "";
        }

        # ── WebSocket (real-time events) ─────────────────────
        location /ws {
            set $api_backend "http://api-server:8080";
            proxy_pass $api_backend;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;

            proxy_read_timeout 3600s;
            proxy_send_timeout 3600s;
        }

        # ── SSE streaming ────────────────────────────────────
        location /api/v1/events/ {
            limit_req zone=api burst=10 nodelay;

            set $api_backend "http://api-server:8080";
            proxy_pass $api_backend;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_set_header Connection '';
            proxy_buffering off;
            proxy_cache off;
            chunked_transfer_encoding off;
            proxy_read_timeout 3600s;
        }

        # ── Health check ─────────────────────────────────────
        location /healthz {
            set $api_backend "http://api-server:8080";
            proxy_pass $api_backend;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
        }

        # ── Frontend (Next.js) ──────────────────────────────
        location / {
            set $frontend_backend "http://frontend:3000";
            proxy_pass $frontend_backend;
            proxy_http_version 1.1;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_hide_header ETag;
            proxy_hide_header Last-Modified;
            add_header Cache-Control "no-cache, no-store, must-revalidate" always;
            add_header Pragma "no-cache" always;
        }

        # Static assets caching
        location ~* ^/_next/static/ {
            set $frontend_backend "http://frontend:3000";
            proxy_pass $frontend_backend;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            expires 365d;
            add_header Cache-Control "public, immutable";
        }

        # Deny access to hidden files
        location ~ /\. {
            deny all;
            access_log off;
            log_not_found off;
        }
    }
}
NGINX_EOF

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

# ── Create deploy.sh ──────────────────────────────────────────
step "Creating deploy.sh"

cat > "$PACKAGE_DIR/deploy.sh" << 'DEPLOY_EOF'
#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# PEPA — One-Command Production Deployment
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
log "Loading Docker images..."

for img in images/*.tar.gz; do
  [ -f "$img" ] || continue
  name="$(basename "$img" .tar.gz)"
  log "  Loading $name..."
  gunzip -c "$img" | docker load
done

ok "All images loaded"

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
# If the postgres volume already has data from a previous deployment
# with a different password, the new POSTGRES_PASSWORD in .env will
# not work. Detect this and offer to reset.
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
echo -e "${GREEN}  PEPA Production Deployment Complete${NC}"
echo -e "${GREEN}════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "  ${BLUE}Frontend:${NC}   https://localhost"
echo -e "  ${BLUE}API:${NC}        https://localhost/api/"
echo -e "  ${BLUE}API (direct):${NC} http://localhost:${API_PORT:-8088}"
echo -e "  ${BLUE}PostgreSQL:${NC} localhost:${POSTGRES_PORT:-5432}"
echo -e "  ${BLUE}Redis:${NC}      localhost:${REDIS_PORT:-6379}"
echo ""
echo -e "  ${YELLOW}Note:${NC} Using self-signed SSL certificate."
echo -e "  For production, replace nginx/ssl/cert.pem and key.pem"
echo -e "  with your domain certificate (Let's Encrypt, etc.)"
echo ""
echo -e "  ${BLUE}Useful commands:${NC}"
echo "    $COMPOSE_CMD logs -f          # Follow logs"
echo "    $COMPOSE_CMD ps               # Service status"
echo "    $COMPOSE_CMD restart api-server  # Restart a service"
echo "    $COMPOSE_CMD down             # Stop everything"
echo ""
DEPLOY_EOF

chmod +x "$PACKAGE_DIR/deploy.sh"
ok "deploy.sh created"

# ── Create stop.sh ────────────────────────────────────────────
cat > "$PACKAGE_DIR/stop.sh" << 'STOP_EOF'
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

if docker compose version &> /dev/null; then
  COMPOSE_CMD="docker compose"
else
  COMPOSE_CMD="docker-compose"
fi

echo "Stopping PEPA..."
$COMPOSE_CMD down
echo "PEPA stopped."
STOP_EOF
chmod +x "$PACKAGE_DIR/stop.sh"

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
