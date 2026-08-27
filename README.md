# PEPA — Platform Engineering & Pipeline Automator

> *Delivery without pain, GitOps with joy.*

[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?style=flat&logo=go)](https://go.dev)
[![Next.js](https://img.shields.io/badge/Next.js-14-black?style=flat&logo=next.js)](https://nextjs.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=flat&logo=postgresql)](https://www.postgresql.org)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/badge/Release-v0.1.0-green)](https://github.com/AlexsandrKotov/pepa/releases)

PEPA is an open-source **Platform Engineering & Pipeline Automator** for service catalog, GitOps deployments, Kubernetes management, CI/CD pipelines, workflow automation, and developer self-service.

---

## Features

- **Service Catalog** — register and manage services with metadata, health, and scorecards
- **GitOps Engine** — FluxCD and ArgoCD integration with drift detection
- **Multi-tenant Architecture** — complete tenant isolation with RBAC
- **Plugin System** — 9 built-in plugins, Go SDK for custom integrations
- **Vault Integration** — HashiCorp Vault for secret management
- **AI Assistant** — context-aware chat with RAG framework
- **Workflow Engine** — DAG-based automation with visual designer
- **52 Frontend Pages** — comprehensive UI covering all platform features

---

## Quick Start

### From Source (Development)

```bash
git clone https://github.com/AlexsandrKotov/pepa.git && cd pepa
make docker-up
```

Access:
- Frontend: http://localhost:3000
- API: http://localhost:8088

### Production (Tar Archive)

Download the production package from [GitHub Releases](https://github.com/AlexsandrKotov/pepa/releases):

```bash
# Download and extract
wget https://github.com/AlexsandrKotov/pepa/releases/download/v0.1.0/pepa-production-v0.1.0.tar.gz
tar xzf pepa-production-v0.1.0.tar.gz
cd pepa-production-v0.1.0

# Deploy
./deploy.sh
```

The production package includes:
- Pre-built Docker images
- Docker Compose configuration
- Auto-generated secrets
- One-command deployment script

### GHCR Images

Pre-built images are available on GitHub Container Registry:

```bash
docker pull ghcr.io/alexsandrkotov/pepa/pepa-api-server:latest
docker pull ghcr.io/alexsandrkotov/pepa/pepa-worker:latest
docker pull ghcr.io/alexsandrkotov/pepa/pepa-frontend:latest
```

### Helm (Kubernetes)

```bash
helm install pepa oci://ghcr.io/alexsandrkotov/pepa/charts/pepa --namespace pepa --create-namespace
```

### Local Development

```bash
# Start infrastructure
docker compose -f deployments/compose/docker-compose.yml up -d postgres redis minio

# Build and run
make build && make run-api
```

---

## Plugins

PEPA includes a three-tier plugin system:

**Built-in (Free):** GitHub, GitLab, ArgoCD, Telegram, Email, Webhook, Slack, Prometheus, S3

**Community (Free):** Gitea - download from [GitHub Releases](https://github.com/AlexsandrKotov/pepa/releases)

**Premium (Commercial):** Bitbucket, FluxCD, Teams, Jira, Proxmox - contact for licensing

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| **Backend** | Go 1.27, Gin, PostgreSQL 16 |
| **Frontend** | Next.js 14, React 18, TypeScript |
| **Infrastructure** | Docker, Kubernetes, Helm |
| **Security** | Vault, RBAC, JWT, Multi-tenant |

---

## Documentation

- [User Guide (EN)](docs/user-guide-en.md) | [Руководство (RU)](docs/user-guide-ru.md)
- [Architecture](docs/01-architecture.md)
- [Plugin Architecture](docs/02-plugin-architecture.md)
- [API Specification](docs/openapi.yaml)
- [Migration Guides](docs/migration-guides/)

---

## Development

**Prerequisites:** Go 1.27+, Node.js 22+, Docker

```bash
make build        # Build all binaries
make test         # Run tests
make lint         # Run linter
make docker-up    # Start full stack
```

---

## Community

- [GitHub Discussions](https://github.com/AlexsandrKotov/pepa/discussions)
- [Contributing](CONTRIBUTING.md)
- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Security Policy](SECURITY.md)

---

## License

[Apache License 2.0](LICENSE)

---

**Repository:** https://github.com/AlexsandrKotov/pepa
