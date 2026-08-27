# PEPA — Platform Engineering & Pipeline Automator

## Architecture Design Document v1.0

> **Project Codename:** PEPA
> **Target Maturity:** CNCF Sandbox → Incubating
> **License:** Apache 2.0
> **Status:** Design Phase

---

## 1. Executive Summary

PEPA is a **vendor-neutral, plugin-driven Internal Developer Portal (IDP)** combined with a **GitOps Orchestration Layer**. It provides a unified control plane for service catalog management, self-service infrastructure, deployment orchestration, and developer experience — while remaining fully extensible through a gRPC-based plugin system.

### Core Design Principles

| Principle | Description |
|-----------|-------------|
| **Plugin-First** | Every external integration (Git, CI/CD, Cloud, Monitoring) is a plugin. The core ships with zero vendor lock-in. |
| **Declarative Everything** | All configurations — workflows, policies, dashboards — are YAML/JSON declarative resources stored in Git. |
| **Entity-Centric** | A universal Dynamic Entity Graph connects all resources (services, repos, deployments, incidents) into a navigable topology. |
| **Multi-Tenant by Default** | Teams, environments, and clusters are first-class isolation boundaries with independent RBAC. |
| **AI-Augmented** | Built-in RAG framework connects LLMs to platform telemetry for intelligent assistance. |
| **Cloud Native** | Kubernetes-native deployment, Helm-packaged, Observable (OpenTelemetry), and horizontally scalable. |

---

## 2. High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         PRESENTATION LAYER                               │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  ┌────────────┐ │
│  │ Service      │  │ Workflow     │  │ Entity Graph  │  │ Custom     │ │
│  │ Catalog UI   │  │ Visualizer   │  │ Explorer      │  │ Dashboards │ │
│  │ (Next.js)    │  │ (React Flow) │  │ (React Flow)  │  │ (Widgets)  │ │
│  └──────┬───────┘  └──────┬───────┘  └──────┬────────┘  └─────┬──────┘ │
│         └──────────────────┴─────────────────┴─────────────────┘        │
│                              GraphQL / REST API Gateway                   │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
┌──────────────────────────────────┴──────────────────────────────────────┐
│                         CORE PLATFORM (Go)                               │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  ┌────────────┐ │
│  │ Plugin       │  │ Workflow     │  │ Entity        │  │ RBAC &     │ │
│  │ Engine       │  │ Engine       │  │ Graph Engine  │  │ Multi-     │ │
│  │ (gRPC Host)  │  │ (DAG Exec)   │  │ (PG + Cache)  │  │ Tenant     │ │
│  └──────┬───────┘  └──────┬───────┘  └──────┬────────┘  └─────┬──────┘ │
│  ┌──────┴───────┐  ┌──────┴───────┐  ┌──────┴────────┐  ┌─────┴──────┐ │
│  │ Event Bus    │  │ Policy       │  │ AI/RAG        │  │ Scorecard  │ │
│  │ (Redis       │  │ Engine       │  │ Framework     │  │ & Audit    │ │
│  │  Pub/Sub)    │  │ (OPA/Cedar)  │  │ (LLM Agnostic)│  │            │ │
│  └──────────────┘  └──────────────┘  └───────────────┘  └────────────┘ │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │ gRPC / HTTP
┌──────────────────────────────────┴──────────────────────────────────────┐
│                         PLUGIN LAYER                                      │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐ │
│  │ Git      │ │ Task     │ │ CD       │ │ Cloud    │ │ Monitoring   │ │
│  │ Provider │ │ Tracker  │ │ Engine   │ │ Provider │ │ & APM        │ │
│  │ Plugin   │ │ Plugin   │ │ Plugin   │ │ Plugin   │ │ Plugin       │ │
│  ├──────────┤ ├──────────┤ ├──────────┤ ├──────────┤ ├──────────────┤ │
│  │ GitHub   │ │ Jira     │ │ ArgoCD   │ │ AWS      │ │ Prometheus   │ │
│  │ GitLab   │ │ Linear   │ │ FluxCD   │ │ GCP      │ │ Datadog      │ │
│  │ Bitbucket│ │ GitHub   │ │ Tekton   │ │ Azure    │ │ Grafana      │ │
│  │ Gitea    │ │ Issues   │ │ Spinnaker│ │ Terraform│ │ PagerDuty    │ │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────────┘ │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
┌──────────────────────────────────┴──────────────────────────────────────┐
│                         DATA LAYER                                        │
│  ┌───────────────────┐  ┌──────────────┐  ┌──────────────────────────┐ │
│  │ PostgreSQL        │  │ Redis        │  │ Object Storage           │ │
│  │ + PGvector        │  │ (Pub/Sub,    │  │ (S3/MinIO — artifacts,   │ │
│  │ (Entities, RBAC,  │  │  Cache,      │  │  backups, RAG docs)      │ │
│  │  Workflows, RAG)  │  │  Sessions)   │  │                          │ │
│  └───────────────────┘  └──────────────┘  └──────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Component Overview

### 3.1 Frontend — Next.js Application

| Module | Technology | Purpose |
|--------|-----------|---------|
| Service Catalog | Next.js + Tailwind | Browse/register services with metadata |
| Workflow Designer | React Flow | Visual DAG editor for pipelines |
| Entity Graph Explorer | React Flow + D3 | Interactive topology map of all entities |
| Dashboard Builder | Drag-and-drop widgets | Role-specific customizable dashboards |
| Plugin Marketplace | Extension store UI | Browse, install, configure plugins |
| AI Assistant Panel | Chat UI + RAG | Conversational interface to platform data |

### 3.2 Backend — Go Core Platform

| Component | Responsibility |
|-----------|---------------|
| **API Gateway** | REST + GraphQL endpoint, authentication, rate limiting |
| **Plugin Engine** | HashiCorp go-plugin host — lifecycle management of plugin processes |
| **Workflow Engine** | DAG execution, step retry/rollback, approval gates |
| **Entity Graph Engine** | CRUD for dynamic entities + relationships, graph traversal |
| **RBAC Engine** | Policy evaluation (OPA/Cedar), role resolution, tenant isolation |
| **Event Bus** | Redis Pub/Sub for real-time events, WebSocket fan-out |
| **Policy Engine** | OPA integration for deployment policies, compliance checks |
| **AI/RAG Framework** | LLM provider abstraction, document ingestion, vector search |
| **Scorecard Engine** | Maturity scoring, SLI/SLO tracking, compliance reporting |

### 3.3 Storage Layer

| Store | Usage |
|-------|-------|
| **PostgreSQL + PGvector** | Primary store: entities, workflows, RBAC, vector embeddings |
| **Redis** | Session cache, real-time pub/sub, job queue, rate limiting |
| **Object Storage (S3/MinIO)** | Plugin artifacts, RAG document blobs, backup archives |

---

## 4. Key API Surfaces

### 4.1 Northbound API (Frontend → Core)

```
GraphQL Schema (primary)
├── Query: entities, workflows, plugins, catalog, dashboards, ai
├── Mutation: createEntity, triggerWorkflow, installPlugin, ...
└── Subscription: entityEvents, workflowProgress, pluginLogs

REST (secondary, for webhooks and external integrations)
├── /api/v1/entities/{type}
├── /api/v1/workflows/{id}/execute
├── /api/v1/plugins/{id}/configure
├── /api/v1/catalog/services
└── /api/v1/ai/chat
```

### 4.2 Plugin Interface (Core → Plugins)

```protobuf
// Each plugin type implements a specific gRPC service
service TaskTrackerPlugin {
  rpc ListIssues(ListIssuesRequest) returns (ListIssuesResponse);
  rpc GetIssue(GetIssueRequest) returns (Issue);
  rpc CreateIssue(CreateIssueRequest) returns (Issue);
  rpc SubscribeEvents(SubscribeRequest) returns (stream IssueEvent);
}

service GitProviderPlugin {
  rpc ListRepositories(ListReposRequest) returns (ListReposResponse);
  rpc GetPullRequests(GetPRsRequest) returns (PullRequestList);
  rpc CreateWebhook(CreateWebhookRequest) returns (Webhook);
  rpc GetCommitHistory(GetCommitsRequest) returns (CommitList);
}

service CDEnginePlugin {
  rpc Deploy(DeployRequest) returns (DeployResponse);
  rpc GetDeploymentStatus(StatusRequest) returns (DeploymentStatus);
  rpc Rollback(RollbackRequest) returns (RollbackResponse);
  rpc ListEnvironments(ListEnvRequest) returns (ListEnvResponse);
}

service CloudProviderPlugin {
  rpc ProvisionResource(ProvisionRequest) returns (ProvisionResponse);
  rpc GetResourceStatus(GetStatusRequest) returns (ResourceStatus);
  rpc DestroyResource(DestroyRequest) returns (DestroyResponse);
  rpc ListResourceTypes(ListTypesRequest) returns (ListTypesResponse);
}
```

---

## 5. Deployment Architecture

PEPA supports **two deployment modes**: Kubernetes (for scale) and Docker Compose (for simplicity).

### 5.1 Kubernetes Deployment (Production at Scale)

```
┌─────────────────── Kubernetes Cluster ───────────────────┐
│                                                           │
│  ┌─────────────────────────────────────────────────────┐ │
│  │ Namespace: pepa-system                         │ │
│  │                                                      │ │
│  │  ┌────────────┐  ┌────────────┐  ┌──────────────┐  │ │
│  │  │ API Server │  │ Worker     │  │ Plugin       │  │ │
│  │  │ (Deployment│  │ (Deployment│  │ Sidecars     │  │ │
│  │  │  replicas: │  │  replicas: │  │ (per-plugin  │  │ │
│  │  │  3)        │  │  5)        │  │  process)    │  │ │
│  │  └────────────┘  └────────────┘  └──────────────┘  │ │
│  └─────────────────────────────────────────────────────┘ │
│                                                           │
│  ┌─────────────────────────────────────────────────────┐ │
│  │ Namespace: pepa-data                           │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────────────┐  │ │
│  │  │ Postgres │  │ Redis    │  │ MinIO (optional) │  │ │
│  │  │ (HA via  │  │ (Sentinel│  │                  │  │ │
│  │  │  Patroni)│  │  or      │  │                  │  │ │
│  │  │          │  │  Cluster)│  │                  │  │ │
│  │  └──────────┘  └──────────┘  └──────────────────┘  │ │
│  └─────────────────────────────────────────────────────┘ │
│                                                           │
│  ┌─────────────────────────────────────────────────────┐ │
│  │ Ingress: nginx/contour → TLS termination            │ │
│  │ Service Mesh: Istio/Linkerd (optional, mTLS)        │ │
│  │ Observability: OpenTelemetry → Jaeger + Prometheus   │ │
│  └─────────────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────────────┘
```

### 5.2 Docker Compose Deployment (Development & Small-Scale Production)

For development, demos, and smaller deployments, PEPA provides a full Docker Compose setup:

```
┌─────────────────────── Docker Host ────────────────────────┐
│                                                              │
│  docker compose up -d                                        │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ Core Services                                           │ │
│  │                                                         │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐            │ │
│  │  │ API      │  │ Worker   │  │ Frontend │            │ │
│  │  │ Server   │  │          │  │ (Next.js)│            │ │
│  │  │ :8080    │  │          │  │ :3000    │            │ │
│  │  └──────────┘  └──────────┘  └──────────┘            │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ Data Layer                                              │ │
│  │                                                         │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐            │ │
│  │  │ Postgres │  │ Redis    │  │ MinIO    │            │ │
│  │  │ +PGvector│  │          │  │ (S3)     │            │ │
│  │  │ :5432    │  │ :6379    │  │ :9000    │            │ │
│  │  └──────────┘  └──────────┘  └──────────┘            │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ Optional (via --profile)                                │ │
│  │                                                         │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐            │ │
│  │  │ Ollama   │  │Prometheus│  │ Grafana  │            │ │
│  │  │ (local   │  │          │  │          │            │ │
│  │  │  LLM)    │  │ :9090    │  │ :3001    │            │ │
│  │  │ --ai     │  │ --observ.│  │ --observ.│            │ │
│  │  └──────────┘  └──────────┘  └──────────┘            │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                              │
│  Production: docker compose -f ... -f docker-compose.prod.yml│
│  (adds Nginx reverse proxy, TLS, multi-replica, hardening)  │
└──────────────────────────────────────────────────────────────┘
```

#### Quick Start

```bash
# Clone and setup
git clone https://github.com/pepa/pepa.git
cd pepa
cp .env.example .env

# Development mode
docker compose -f deployments/compose/docker-compose.yml up -d

# Production mode (with nginx TLS, multi-replica)
docker compose \
  -f deployments/compose/docker-compose.yml \
  -f deployments/compose/docker-compose.prod.yml up -d

# With local AI (Ollama)
docker compose --profile ai up -d

# With observability (Prometheus + Grafana)
docker compose --profile observability up -d

# Or use the quickstart script
./deployments/compose/quickstart.sh --all
```

#### Deployment Comparison

| Feature | Docker Compose | Kubernetes |
|---------|---------------|------------|
| Setup complexity | Low (single command) | Medium (Helm chart) |
| Scaling | Manual (replicas in compose) | Automatic (HPA) |
| High Availability | Single host | Multi-AZ, pod anti-affinity |
| TLS | Nginx (self-signed or cert) | Ingress + cert-manager |
| Plugin isolation | Process-level | Pod-level (stronger) |
| Observability | Optional profiles | Built-in (OpenTelemetry) |
| Best for | Dev, demos, small teams | Enterprise, large scale |

---

## 6. Technology Decisions (ADRs)

| ID | Decision | Rationale |
|----|----------|-----------|
| ADR-001 | Go for backend core | Performance, single binary, strong Kubernetes ecosystem |
| ADR-002 | HashiCorp go-plugin over native gRPC | Process isolation, crash recovery, language-agnostic plugins |
| ADR-003 | PostgreSQL + PGvector | Single source of truth + vector search without separate DB |
| ADR-004 | Redis for pub/sub and cache | Mature, low-latency, native WebSocket integration |
| ADR-005 | Next.js for frontend | SSR for SEO (catalog), React ecosystem, API routes |
| ADR-006 | React Flow for visual components | Best-in-class DAG/graph visualization for React |
| ADR-007 | GraphQL as primary API | Flexible queries for entity graph, reduces over-fetching |
| ADR-008 | OPA for policy engine | Industry standard, Rego language, CNCF graduated project |
| ADR-009 | OpenTelemetry for observability | Vendor-neutral, CNCF standard, unified traces/metrics/logs |

---

## 7. Document Index

| Document | Contents |
|----------|----------|
| [01-architecture.md](./01-architecture.md) | This file — system overview |
| [02-plugin-architecture.md](./02-plugin-architecture.md) | Plugin engine, interfaces, lifecycle, marketplace |
| [03-data-model.md](./03-data-model.md) | Universal entity model, dynamic graph schema |
| [04-workflow-engine.md](./04-workflow-engine.md) | DAG execution, YAML spec, visual designer |
| [05-rbac-dashboards.md](./05-rbac-dashboards.md) | Multi-tenant RBAC, custom dashboards |
| [06-ai-rag-framework.md](./06-ai-rag-framework.md) | LLM abstraction, RAG pipeline, vector search |
| [07-roadmap.md](./07-roadmap.md) | CNCF journey, community strategy, plugin registry |
| [08-implementation-status.md](./08-implementation-status.md) | Implementation progress tracker |
| [PROJECT_STRUCTURE.md](./PROJECT_STRUCTURE.md) | Actual project file structure |

---

## 8. Non-Functional Requirements

| Category | Target |
|----------|--------|
| **Scalability** | 10,000+ entities, 1,000+ concurrent users, 100+ plugins |
| **Availability** | 99.9% SLA for core API (multi-AZ deployment) |
| **Latency** | p99 < 200ms for catalog reads, < 2s for workflow triggers |
| **Security** | OIDC/SAML SSO, mTLS between services, secrets via Vault |
| **Observability** | Full distributed tracing, structured logs, Prometheus metrics |
| **Extensibility** | Any integration achievable via plugin (no core code changes) |
| **Compliance** | SOC2-ready audit trails, data residency controls |
