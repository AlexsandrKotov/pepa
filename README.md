# PEPA — Platform Engineering & Pipeline Automator

> *Delivery without pain, GitOps with joy.*

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go)](https://go.dev)
[![Next.js](https://img.shields.io/badge/Next.js-14-black?style=flat&logo=next.js)](https://nextjs.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=flat&logo=postgresql)](https://www.postgresql.org)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat&logo=docker)](deployments/compose/)
[![Helm](https://img.shields.io/badge/Helm-3-0F1689?style=flat&logo=helm)](deployments/helm/)
[![Release](https://img.shields.io/badge/Release-v0.1.0-green)](https://github.com/akotau/pepa/releases)

PEPA is an open-source **Platform Engineering & Pipeline Automator** for service catalog, GitOps deployments, Kubernetes management, CI/CD pipelines, workflow automation, infrastructure virtualization, and developer self-service.

---

## Why PEPA?

| Challenge | PEPA Solution |
|-----------|---------------|
| Scattered service ownership | **Service Catalog** with owners, teams, health, and scorecards |
| Manual deployments | **GitOps pipelines** — Git as single source of truth, FluxCD/ArgoCD sync |
| Tool sprawl | **Plugin system** — unify Git, CI/CD, monitoring, secrets in one portal |
| No guardrails | **Scorecards** — production readiness scoring with weighted rules |
| Slow onboarding | **Self-service** — templates, environments, and one-click deploys |
| No visibility | **Dashboards** — role-based views, deployment frequency, DORA metrics |

---

## Key Features

### Service Management
- **Service Catalog** — register, browse, and manage services with metadata, health status, and ownership
- **Service Templates** — pre-built templates (Node.js, Go, Python, Static, Helm Import)
- **Scorecards** — production readiness scoring (Bronze → Silver → Gold → Platinum)
- **Entity Graph** — interactive topology map of all services and their relationships

### GitOps & Deployments
- **GitOps Engine** — FluxCD and ArgoCD integration with drift detection and auto-heal
- **Deployment Management** — multi-environment (dev → staging → production) with approval gates
- **Pipeline Builder** — visual CI/CD pipeline editor with blueprints
- **Helm Repositories** — manage and import Helm charts from remote repositories
- **Service Discovery** — auto-discover services from Git repositories and Kubernetes clusters

### Infrastructure
- **Kubernetes Clusters** — connect clusters, view nodes, namespaces, resources, and health
- **Docker Hosts** — manage Docker hosts, containers, images, and volumes
- **Virtualization** — Proxmox VE integration for VMs, containers, and storage
- **Environments** — manage deployment targets with variables and constraints

### Automation & Security
- **Workflow Engine** — DAG-based automation with steps, conditions, approvals, and rollback
- **Visual Workflow Designer** — drag-and-drop canvas for building complex workflows
- **Vault Integration** — secret management with HashiCorp Vault (KV v2) and encrypted credential storage
- **RBAC** — role-based access control with custom roles, permissions, and team management
- **Audit Log** — immutable trail of all platform actions
- **Jira Integration** — link issues to deployments, automate issue workflows

### Intelligence
- **AI Assistant** — context-aware chat for infrastructure analysis and recommendations
- **RAG Framework** — LLM-agnostic retrieval-augmented generation with PGvector embeddings
- **AI Tools** — built-in tools for catalog queries, deployment actions, and diagnostics

### Extensibility
- **Plugin System** — 10 built-in plugins, Go SDK for custom integrations
- **Marketplace** — browse and install plugins from a curated catalog
- **Webhooks** — event-driven integrations with external systems
- **OpenAPI** — REST API with comprehensive endpoint documentation

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        FRONTEND (Next.js 14)                      │
│   Dashboard │ Catalog │ Pipelines │ GitOps │ AI │ Settings       │
└──────────────────────────┬──────────────────────────────────────┘
                           │ REST API
┌──────────────────────────┴──────────────────────────────────────┐
│                       API SERVER (Go/Gin)                          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐          │
│  │ Plugin   │ │ Workflow │ │ RBAC     │ │ AI/RAG   │          │
│  │ Engine   │ │ Engine   │ │ Engine   │ │ Framework│          │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐          │
│  │ GitOps   │ │ Vault    │ │ Entity   │ │ Scorecard│          │
│  │ Engine   │ │Integration│ │ Graph    │ │ Engine   │          │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘          │
└──────────────────────────┬──────────────────────────────────────┘
                           │
┌──────────────────────────┴──────────────────────────────────────┐
│                        DATA LAYER                                  │
│  ┌──────────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐  │
│  │ PostgreSQL 16│  │ Redis    │  │ MinIO    │  │ Vault      │  │
│  │ + PGvector   │  │ (Queue)  │  │ (S3)     │  │ (Secrets)  │  │
│  └──────────────┘  └──────────┘  └──────────┘  └────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                           │
┌──────────────────────────┴──────────────────────────────────────┐
│                      PLUGIN LAYER                                  │
│  GitHub │ GitLab │ Bitbucket │ Gitea │ ArgoCD │ FluxCD │ Jira   │
│  Slack  │ Prometheus │ Proxmox                                   │
└─────────────────────────────────────────────────────────────────┘
```

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| **Frontend** | Next.js 14, React 18, Tailwind CSS, App Router |
| **API** | Go 1.26, Gin, pgx v5 |
| **Database** | PostgreSQL 16 + PGvector (31 migrations) |
| **Queue & Cache** | Redis (events bus, job queue, sessions) |
| **Object Storage** | MinIO (S3-compatible) |
| **Secrets** | HashiCorp Vault (KV v2), AES-256 encryption at rest |
| **Kubernetes** | client-go, Helm 3 |
| **Auth** | JWT, RBAC, bootstrap token |
| **Plugins** | HashiCorp go-plugin, gRPC |
| **Deploy** | Docker, Docker Compose, Helm 3, Kubernetes |
| **CI/CD** | GitHub Actions |
| **Monitoring** | Prometheus, OpenTelemetry (planned) |

---

## Plugins

PEPA includes a powerful plugin system with three tiers of plugins:

### Built-in Plugins (Free - Included)
These plugins are included in the box with PEPA:

| Plugin | Type | Description |
|--------|------|-------------|
| **GitHub** | Git Provider | Repositories, PRs, webhooks, commit history |
| **GitLab** | Git Provider | Repositories, CI/CD, merge requests, pipelines |
| **ArgoCD** | CD Engine | GitOps sync, rollback, history, resource management |
| **Telegram** | Notification | Telegram bot messages and alerts |
| **Email** | Notification | Email notifications and alerts |
| **Webhook** | Notification | Generic webhook integrations |
| **Slack** | Notification | Slack messages and alerts |
| **Prometheus** | Monitoring | Metrics queries, alerts, service health |
| **S3** | Infrastructure | S3-compatible storage (MinIO, AWS S3) |

### Community Plugins (Free - Downloadable)
Community-maintained plugins available for download:

| Plugin | Type | Description |
|--------|------|-------------|
| **Gitea** | Git Provider | Self-hosted Git service, repositories, PRs, issues |

Download community plugins from [GitHub Releases](https://github.com/akotau/pepa/releases) and place them in `plugins/bin/community/`.

### Premium Plugins (Commercial)
Enterprise-grade plugins for professional use:

| Plugin | Type | Description |
|--------|------|-------------|
| **Bitbucket** | Git Provider | Bitbucket repositories, PRs, branches, webhooks |
| **FluxCD** | CD Engine | GitOps reconcile, suspend, resume, drift detection |
| **Microsoft Teams** | Notification | Teams messages and alerts |
| **Jira** | Task Tracker | Issues, projects, transitions, sprint management |
| **Proxmox** | Infrastructure | VMs, containers, storage, node management |

Contact us for licensing information about premium plugins.

### Installing Plugins

**Built-in Plugins:** No installation required - they're included and ready to use.

**Community Plugins:**
1. Download the plugin binary from GitHub Releases
2. Place it in `plugins/bin/community/<plugin-name>/<plugin-name>`
3. Restart PEPA: `docker compose restart api-server worker`

**Premium Plugins:**
1. Contact sales for licensing
2. Receive plugin binaries
3. Place in `plugins/premium-bin/<plugin-name>/<plugin-name>`
4. Restart PEPA: `docker compose restart api-server worker`

---

## Quick Start

### Docker Compose (Recommended)

```bash
git clone https://github.com/akotau/pepa.git && cd pepa

# Start the full stack (API, Worker, Frontend, PostgreSQL, Redis, MinIO)
make docker-up

# Frontend:  http://localhost:3000
# API:       http://localhost:8088
```

### CLI Install

```bash
# macOS / Linux
curl -fsSL https://get.pepa.io | sh

# Or via Homebrew
brew install pepa/tap/pepa-cli

# Then install PEPA
pepa install --compose    # Docker Compose
pepa install              # Kubernetes (Helm)
```

### Helm (Kubernetes)

```bash
helm install pepa oci://ghcr.io/akotau/charts/pepa --namespace pepa --create-namespace
```

### Local Development

```bash
# 1. Start infrastructure
docker compose -f deployments/compose/docker-compose.yml up -d postgres redis minio minio-init

# 2. Build and run API
make build && make run-api

# 3. Run worker (separate terminal)
make run-worker

# 4. Run frontend (separate terminal)
cd frontend && npm install && npm run dev
```

---

## Default Credentials

| Email | Password | Role |
|-------|----------|------|
| `admin@pepa.dev` | Set on first login | Platform Admin (Super Admin) |

> After bootstrap, the admin must change the password on first login.

---

## Documentation

| Document | Description |
|----------|-------------|
| [User Guide (EN)](docs/user-guide-en.md) | Complete user guide |
| [Руководство (RU)](docs/user-guide-ru.md) | Руководство пользователя |
| [Architecture](docs/01-architecture.md) | System architecture design |
| [Plugin Architecture](docs/02-plugin-architecture.md) | Plugin engine, interfaces, lifecycle |
| [Data Model](docs/03-data-model.md) | Entity model, dynamic graph schema |
| [Workflow Engine](docs/04-workflow-engine.md) | DAG execution, YAML spec, designer |
| [RBAC & Dashboards](docs/05-rbac-dashboards.md) | Multi-tenant RBAC, dashboards |
| [AI/RAG Framework](docs/06-ai-rag-framework.md) | LLM abstraction, RAG pipeline |
| [Roadmap](docs/07-roadmap.md) | CNCF journey, community strategy |
| [Implementation Status](docs/08-implementation-status.md) | What's done and what's planned |
| [OpenAPI Spec](docs/openapi.yaml) | REST API specification |
| [Vault Guide](docs/vault-integration-guide.md) | Secret management with Vault |
| [GitOps Guide](docs/gitlab-workflow-guide.md) | GitLab deployment workflow |
| [OpenTelemetry Guide](docs/opentelemetry-signoz-guide.md) | Observability setup |
| [Advanced Workflows](docs/advanced-workflows-guide.md) | Complete deployment methods & patterns |
| [Migration: Backstage](docs/migration-guides/from-backstage.md) | Migrate from Backstage |
| [Migration: Port](docs/migration-guides/from-port.md) | Migrate from Port |
| [Migration: Cortex](docs/migration-guides/from-cortex.md) | Migrate from Cortex |

---

## Community

- [GOVERNANCE.md](GOVERNANCE.md) — Merit-based governance with TOC and SIGs
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — Contributor Covenant v2.1
- [SECURITY.md](SECURITY.md) — Vulnerability disclosure policy
- [CONTRIBUTING.md](CONTRIBUTING.md) — Development setup and PR process
- [GitHub Discussions](https://github.com/akotau/pepa/discussions) — Q&A, ideas, show & tell

---

## Development

### Prerequisites

- Go 1.26+
- Node.js 22+
- Docker & Docker Compose
- PostgreSQL 16 (or use Docker)

### Build

```bash
# All binaries (API, Worker, CLI, Plugins)
make build

# Frontend
cd frontend && npm run build
```

### Test

```bash
make test           # Go tests with race detection
make lint           # golangci-lint
make verify         # Full verification (build + frontend)
```

### Database

```bash
make migrate-up     # Run all migrations
make migrate-init   # Initialize from compose schema
```

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, coding standards, and PR process.

---

## License

[Apache License 2.0](LICENSE)

---

## Links

- **Repository**: https://github.com/akotau/pepa
- **Documentation**: [docs/](docs/)
- **Issues**: https://github.com/akotau/pepa/issues
- **Discussions**: https://github.com/akotau/pepa/discussions
- **Security**: [SECURITY.md](SECURITY.md)
