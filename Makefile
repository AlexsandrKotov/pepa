# ============================================================
# PEPA — Platform Engineering & Pipeline Automator
# "Delivery without pain, GitOps with joy."
# ============================================================

.PHONY: all build test lint clean docker-build docker-up docker-down docker-logs deploy plugins clean-plugins

# Variables
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS     := -ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"
GOFILES     := $(shell find . -name '*.go' -not -path './vendor/*')

# ── Build ────────────────────────────────────────────────────

all: build

build: build-api build-worker build-cli plugins

build-api:
	@echo "→ Building API server..."
	@CGO_ENABLED=0 go build $(LDFLAGS) -o bin/api-server ./cmd/api-server

build-worker:
	@echo "→ Building worker..."
	@CGO_ENABLED=0 go build $(LDFLAGS) -o bin/worker ./cmd/worker

build-cli:
	@echo "→ Building CLI..."
	@CGO_ENABLED=0 go build $(LDFLAGS) -o bin/pepa ./cmd/cli

# ── Development (local, without Docker) ──────────────────────

run-api:
	@echo "→ Starting API server locally..."
	@go run ./cmd/api-server

run-worker:
	@echo "→ Starting worker locally..."
	@go run ./cmd/worker

# ── Test & Lint ──────────────────────────────────────────────

test:
	@echo "→ Running tests..."
	@go test -race -coverprofile=coverage.out ./...

test-cover: test
	@go tool cover -html=coverage.out -o coverage.html

lint:
	@echo "→ Running linter..."
	@golangci-lint run ./...

fmt:
	@echo "→ Formatting code..."
	@gofmt -s -w $(GOFILES)

vet:
	@echo "→ Running go vet..."
	@go vet ./...

# ── Database ─────────────────────────────────────────────────

migrate-up:
	@echo "→ Running migrations..."
	@for f in $$(ls migrations/*.sql | sort); do \
		echo "  applying $$f..."; \
		psql "$(DATABASE_URL)" -f "$$f"; \
	done
	@echo "✓ Migrations complete"

migrate-init:
	@echo "→ Initializing database from compose schema..."
	@psql "$(DATABASE_URL)" -f deployments/compose/init-db.sql

# ── Docker ───────────────────────────────────────────────────

COMPOSE_FILE := deployments/compose/docker-compose.yml
COMPOSE      := docker compose -f $(COMPOSE_FILE) --profile production

docker-build:
	@echo "→ Building Docker images..."
	@docker build -f deployments/docker/Dockerfile.api -t pepa/api-server:latest .
	@docker build -f deployments/docker/Dockerfile.worker -t pepa/worker:latest .
	@docker build -f deployments/docker/Dockerfile.frontend -t pepa/frontend:latest frontend/

docker-up:
	@echo "→ Starting PEPA stack..."
	@$(COMPOSE) up -d

docker-down:
	@echo "→ Stopping PEPA stack..."
	@$(COMPOSE) down

docker-logs:
	@$(COMPOSE) logs -f

# ── Full clean deploy ────────────────────────────────────────

deploy: clean plugins docker-build
	@echo ""
	@echo "→ Stopping old containers and volumes..."
	@$(COMPOSE) down -v --remove-orphans 2>/dev/null || true
	@echo "→ Ensuring .env exists..."
	@test -f deployments/compose/.env || cp deployments/compose/.env.example deployments/compose/.env
	@echo "→ Starting PEPA stack..."
	@$(COMPOSE) up -d
	@echo ""
	@echo "✓ PEPA deployed!"
	@echo ""
	@echo "  API:       https://localhost (via nginx)"
	@echo "  Frontend:  https://localhost"
	@echo "  Postgres:  localhost:$${POSTGRES_PORT:-5432}"
	@echo "  Redis:     localhost:$${REDIS_PORT:-6379}"
	@echo ""
	@echo "  Logs:  make docker-logs"
	@echo "  Stop:  make docker-down"
	@echo ""

# ── Clean ────────────────────────────────────────────────────

clean:
	@echo "→ Cleaning..."
	@rm -rf bin/ coverage.out coverage.html

# ── Dependencies ─────────────────────────────────────────────

deps:
	@echo "→ Downloading dependencies..."
	@go mod download && go mod tidy

deps-ui:
	@echo "→ Installing frontend dependencies..."
	@cd frontend && npm ci

# ── Helm ────────────────────────────────────────────────────

helm-lint:
	@echo "→ Linting Helm chart..."
	@helm lint deployments/helm/pepa/

helm-template:
	@echo "→ Rendering Helm templates..."
	@helm template pepa deployments/helm/pepa/

helm-install:
	@echo "→ Installing Helm chart..."
	@helm install pepa deployments/helm/pepa/ --create-namespace -n pepa

helm-upgrade:
	@echo "→ Upgrading Helm chart..."
	@helm upgrade pepa deployments/helm/pepa/ -n pepa

helm-uninstall:
	@echo "→ Uninstalling Helm chart..."
	@helm uninstall pepa -n pepa

# ── Verify ──────────────────────────────────────────────────

verify: build build-plugin-example
	@echo "→ Running full verification..."
	@cd frontend && npx next build
	@echo "✓ All builds passed"

build-plugin-example:
	@echo "→ Building example plugin..."
	@go build -o bin/example-plugin ./plugins/examples

# ── Plugins ──────────────────────────────────────────────────

# Plugin directories by category
PLUGIN_DIRS_BUILTIN := $(shell find plugins/builtin -mindepth 1 -maxdepth 1 -type d 2>/dev/null)
PLUGIN_DIRS_COMMUNITY := $(shell find plugins/community -mindepth 1 -maxdepth 1 -type d 2>/dev/null)
PLUGIN_DIRS_PREMIUM := $(shell find plugins/premium -mindepth 1 -maxdepth 1 -type d 2>/dev/null)

# Legacy support - all plugins except builtin/community/premium
PLUGIN_DIRS := $(shell find plugins -mindepth 1 -maxdepth 1 -type d ! -name bin ! -name examples ! -name sdk-go ! -name builtin ! -name community ! -name premium ! -name README.md)

# Plugins default to Linux/amd64 for Docker containers.
# Override: make plugins GOOS= GOARCH=
PLUGIN_GOOS   ?= linux
PLUGIN_GOARCH ?= amd64

# Build all public plugins (builtin + community)
plugins: plugins-builtin plugins-community
	@echo "✓ All public plugins built"

# Build built-in plugins (source in plugins/<name>/, metadata in plugins/builtin/<name>/)
plugins-builtin:
	@echo "→ Building built-in plugins ($(PLUGIN_GOOS)/$(PLUGIN_GOARCH))..."
	@for dir in $(PLUGIN_DIRS_BUILTIN); do \
		name=$$(basename $$dir); \
		src="plugins/$$name"; \
		if [ ! -d "$$src" ] || [ -z "$$(ls $$src/*.go 2>/dev/null)" ]; then \
			echo "  ⚠ $$name — no Go source, skipping"; \
			continue; \
		fi; \
		echo "  → $$name (builtin)"; \
		mkdir -p plugins/bin/builtin/$$name; \
		CGO_ENABLED=0 GOOS=$(PLUGIN_GOOS) GOARCH=$(PLUGIN_GOARCH) go build -o plugins/bin/builtin/$$name/$$name ./$$src; \
	done
	@echo "✓ Built-in plugins built in plugins/bin/builtin/<name>/"

# Build community plugins (free, downloadable)
plugins-community:
	@echo "→ Building community plugins ($(PLUGIN_GOOS)/$(PLUGIN_GOARCH))..."
	@for dir in $(PLUGIN_DIRS_COMMUNITY); do \
		name=$$(basename $$dir); \
		echo "  → $$name (community)"; \
		mkdir -p plugins/bin/community/$$name; \
		CGO_ENABLED=0 GOOS=$(PLUGIN_GOOS) GOARCH=$(PLUGIN_GOARCH) go build -o plugins/bin/community/$$name/$$name ./$$dir; \
	done
	@echo "✓ Community plugins built in plugins/bin/community/<name>/"

# Build premium plugins (commercial, private)
plugins-premium:
	@echo "→ Building premium plugins ($(PLUGIN_GOOS)/$(PLUGIN_GOARCH))..."
	@if [ -z "$(PLUGIN_DIRS_PREMIUM)" ]; then \
		echo "  ⚠ No premium plugins found"; \
	else \
		for dir in $(PLUGIN_DIRS_PREMIUM); do \
			name=$$(basename $$dir); \
			echo "  → $$name (premium)"; \
			mkdir -p plugins/premium-bin/$$name; \
			CGO_ENABLED=0 GOOS=$(PLUGIN_GOOS) GOARCH=$(PLUGIN_GOARCH) go build -o plugins/premium-bin/$$name/$$name ./$$dir; \
		done; \
		echo "✓ Premium plugins built in plugins/premium-bin/<name>/"; \
	fi

# Build all plugins (including premium)
plugins-all: plugins plugins-premium
	@echo "✓ All plugins built (builtin + community + premium)"

clean-plugins:
	@echo "→ Cleaning plugins..."
	@rm -rf plugins/bin/*

# ── Plugin Signing ─────────────────────────────────────────────

sign-plugins:
	@echo "→ Signing all plugins..."
	@bash scripts/sign-plugin.sh

verify-plugins:
	@echo "→ Verifying plugin signatures..."
	@bash scripts/sign-plugin.sh --verify

# ── Release ────────────────────────────────────────────────────

RELEASE_TAG ?= $(shell git describe --tags --always 2>/dev/null || echo "v0.0.0")

release-check:
	@echo "→ Pre-release checks..."
	@echo "  Tag: $(RELEASE_TAG)"
	@git diff --quiet || (echo "  WARNING: Uncommitted changes!" && exit 1)
	@echo "  Running tests..."
	@go test -race ./...
	@echo "  Running linter..."
	@golangci-lint run ./...
	@echo "  Building frontend..."
	@cd frontend && npm run build
	@echo "✓ All pre-release checks passed"

release-tag:
	@echo "→ Creating release tag $(RELEASE_TAG)..."
	git tag -s "$(RELEASE_TAG)" -m "PEPA $(RELEASE_TAG)"
	@echo "✓ Tag created. Push with: git push origin $(RELEASE_TAG)"

release-checksums:
	@echo "→ Generating checksums..."
	@mkdir -p release/
	@find bin/ -type f -name 'pepa-*' -exec sha256sum {} \; > release/sha256sums.txt 2>/dev/null || true
	@find plugins/bin/ -type f -exec sha256sum {} \; >> release/sha256sums.txt 2>/dev/null || true
	@cat release/sha256sums.txt
	@echo "✓ Checksums in release/sha256sums.txt"

release: release-check docker-build plugins
	@echo ""
	@echo "→ Release $(RELEASE_TAG) ready!"
	@echo ""
	@echo "  Next steps:"
	@echo "  1. make release-tag"
	@echo "  2. git push origin $(RELEASE_TAG)"
	@echo "  3. GitHub Actions will build and publish the release"
	@echo ""
