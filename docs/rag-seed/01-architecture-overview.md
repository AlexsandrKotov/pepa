# PEPA — Architecture Overview

## What is PEPA?

PEPA (Platform Engineering & Pipeline Automator) is a **vendor-neutral Internal Developer Portal (IDP)** with a GitOps orchestration layer. It provides a unified control plane for:

- **Service Catalog** — register and discover all services in one place
- **Self-Service Infrastructure** — developers provision resources without DevOps tickets
- **CI/CD Pipeline Orchestration** — unified view across GitLab CI, GitHub Actions, Jenkins
- **GitOps Workflows** — deploy via ArgoCD, FluxCD, or Helm
- **Scorecards** — measure service maturity (DORA metrics, quality gates)
- **AI Assistant** — LLM-powered assistant with access to platform data via RAG

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.26+, Gin HTTP framework |
| Frontend | Next.js 15, React 19, TypeScript |
| Database | PostgreSQL 18+ with pgvector |
| Cache/Pub-Sub | Redis 7+ |
| Plugin System | HashiCorp go-plugin (gRPC) |
| Auth | JWT + RBAC (role-based access control) |
| Infrastructure | Kubernetes, Helm, Docker |

## Core Components

### 1. Plugin Engine
Every external integration is a plugin running as a separate process. Plugins communicate via gRPC. A crashing plugin cannot bring down the core. Plugins can be enabled, disabled, or upgraded at runtime without restart.

### 2. Workflow Engine
Executes DAG-based workflows. Each workflow has steps (plugin actions) connected by conditions. Supports manual triggers, webhooks, schedules, and event-driven execution.

### 3. Entity Graph
A universal graph connecting all resources: services, repositories, deployments, incidents, clusters. Provides topology visualization and dependency tracking.

### 4. AI/RAG Framework
LLM-agnostic AI assistant. Supports OpenAI, Anthropic, Groq, LM Studio, Ollama. RAG pipeline connects the LLM to real platform data (services, pipelines, documentation) for context-aware answers.

### 5. Multi-Tenant RBAC
Organizations, teams, and users with fine-grained permissions. Super Admin, Admin, Member, Viewer roles. All actions are audit-logged.

## Deployment Options

- **Docker Compose** — quick start for development and small teams
- **Kubernetes + Helm** — production deployment with HA
- **Single binary** — `pepa api-server` for embedded use cases

## Key API Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /healthz` | Health check |
| `POST /api/v1/ai/chat` | AI assistant chat |
| `POST /api/v1/rag/chat` | RAG-powered chat with knowledge base |
| `GET /api/v1/services` | List services |
| `POST /api/v1/workflows` | Create workflow |
| `GET /api/v1/plugins` | List plugins |
| `POST /api/v1/ai/agents/route` | Route query to specialist agent |
