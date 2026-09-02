# Changelog

All notable changes to PEPA are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Remote Console** — SSH terminal access to hosts with session logging and host groups
- **Authentication Providers** — LDAP, Azure AD, OIDC, GitHub OAuth, Google OAuth
- **Plugin Activity Logging** — Track SSH commands and plugin actions with detailed audit trail
- **Knowledge Base** — RAG document management and search interface
- **Blueprint Groups** — Organize blueprints into groups with many-to-many relationships
- **Jira Integration Expansion** — Sprints, components, worklogs, issue links, create issue modal
- **Docker Services Local** — Deploy to local Docker socket or remote hosts
- **Automation Page** — Centralized automation hub
- **Export Page** — Export platform data
- **Quickstart Page** — Interactive setup wizard
- **Get Started Page** — Onboarding guide
- **Plugin: VMware** — vSphere VM management (community plugin)
- **Plugin: Syslog** — Syslog notification channel
- **Plugin: AI Bot** — AI-powered bot integration
- **Plugin: Remote Console** — SSH terminal with command logging
- **Plugin: Trivy** — Vulnerability scanning for images, filesystem, repos, IaC
- Unified plugin distribution system (all plugins free and open-source)
- Plugin build system with builtin and community targets
- Enhanced documentation for plugin installation
- Production-ready deployment package with build-production.sh

### Changed
- Restructured plugin directory structure
- Updated Makefile with plugin tier support
- Improved build-production.sh for new plugin structure
- Enhanced README with plugin distribution documentation
- Merged service templates into unified blueprints concept
- Updated Go version to 1.26
- Enhanced Jira integration with sprints, worklogs, and issue links
- Improved Docker services with local socket support

### Security
- Implemented multi-tenant isolation across all resources
- Added RBAC with fine-grained permissions
- Implemented plugin signature verification
- Added rate limiting middleware
- Implemented audit logging for all actions
- Added LDAP and Azure AD authentication with secure token handling
- Implemented SSH host isolation with RLS policies
- Added plugin activity logging for compliance

### Database
- 19 new migrations (029-047) for auth, SSH hosts, plugin activity, blueprint groups, Jira expansion
- Total migrations: 47

### Documentation
- Added observability guide
- Added Vault integration guide
- Updated implementation status with new features
- Updated user guides with authentication providers

---

**Release Date:** 2026-09-02
**Version:** 0.1.1 (Unreleased)
**Status:** Development

---

## [0.1.0] — 2026-08-27

### 🎉 Initial Public Release

PEPA (Platform Engineering & Pipeline Automator) — open-source platform for service catalog, GitOps deployments, Kubernetes management, and CI/CD automation.

### ✨ Core Features

#### Service Management
- **Service Catalog** — register, browse, and manage services with metadata, health status, and ownership
- **Service Templates** — pre-built templates (Node.js, Go, Python, Static, Helm Import)
- **Scorecards** — production readiness scoring (Bronze → Silver → Gold → Platinum)
- **Entity Graph** — interactive topology map of all services and their relationships
- **Service Discovery** — auto-discover services from Git repositories and Kubernetes clusters

#### GitOps & Deployments
- **GitOps Engine** — FluxCD and ArgoCD integration with drift detection and auto-heal
- **Deployment Management** — multi-environment (dev → staging → production) with approval gates
- **Pipeline Builder** — visual CI/CD pipeline editor with blueprints
- **Helm Repositories** — manage and import Helm charts from remote repositories
- **Kubernetes Clusters** — connect clusters, view nodes, namespaces, resources, and health
- **Docker Hosts** — manage Docker hosts, containers, images, and volumes

#### Infrastructure
- **Proxmox Virtualization** — VMs, containers, storage, node management via Proxmox VE
- **Environments** — manage deployment targets with variables and constraints
- **Workflow Engine** — DAG-based automation with steps, conditions, approvals, and rollback
- **Visual Workflow Designer** — SVG-based canvas with node palette, edges, properties panel

#### Security & Access Control
- **Vault Integration** — HashiCorp Vault KV v2, secret browser, encryption at rest
- **RBAC** — role-based access control with custom roles, permissions, and team management
- **Multi-tenant Architecture** — complete tenant isolation across all resources
- **Audit Log** — immutable trail of all platform actions
- **Bootstrap Token** — secure first-login setup with mandatory password change
- **Rate Limiting** — token-bucket per-IP rate limiter middleware

#### Plugin System
- **15 Open-Source Plugins** — GitHub, GitLab, Bitbucket, Gitea, ArgoCD, FluxCD, Telegram, Slack, Teams, Email, Webhook, Proxmox, Prometheus, S3, Jira
- **Plugin SDK** — Go SDK for custom plugin development
- **Marketplace** — browse and install plugins from curated catalog
- **Plugin Signing** — cryptographic signature verification for all plugins

#### Intelligence & Automation
- **AI Assistant** — context-aware chat for infrastructure analysis and recommendations
- **RAG Framework** — LLM-agnostic retrieval-augmented generation with PGvector embeddings
- **Jira Integration** — link issues to deployments, automate issue workflows
- **Credentials Management** — Vault-backed credential storage with sharing capabilities

### 🏗️ Architecture

#### Backend
- **Language:** Go 1.26
- **Framework:** Gin HTTP framework
- **Database:** PostgreSQL 18 with PGVector
- **Cache:** Redis
- **Architecture:** Repository pattern with service layer
- **API:** 30+ RESTful endpoints with OpenAPI specification

#### Frontend
- **Framework:** Next.js 14 with React 18
- **Language:** TypeScript
- **Pages:** 52 comprehensive pages
- **UI:** Modern, responsive design with Tailwind CSS

#### Infrastructure
- **Containerization:** Docker with multi-stage builds
- **Orchestration:** Kubernetes with Helm charts
- **Deployment:** Docker Compose for dev/prod
- **CI/CD:** GitHub Actions for automated testing and releases

### 🔒 Security

- **Multi-tenant Isolation** — complete data separation between tenants
- **RBAC** — fine-grained access control with role-based permissions
- **Vault Integration** — secrets encrypted at rest with AES-256
- **Plugin Signing** — all plugins cryptographically signed
- **Audit Logging** — comprehensive audit trail for all actions
- **Rate Limiting** — protection against abuse
- **CORS Configuration** — configurable cross-origin resource sharing
- **Security Headers** — HSTS, X-Frame-Options, X-Content-Type-Options

### 📊 Database

- **32 Migrations** — comprehensive schema with RLS, indexes, and audit trails
- **Row Level Security** — database-level tenant isolation
- **Performance Indexes** — optimized queries for high performance
- **Audit Trails** — complete history of all changes

### 🚀 Deployment

#### Docker Compose
- Full stack deployment with nginx reverse proxy
- Health checks for all services
- Security hardening (read-only containers, no-new-privileges)
- Automatic SSL certificate generation
- Production-ready configuration

#### Kubernetes (Helm)
- High availability with configurable replicas
- PodDisruptionBudgets for zero-downtime updates
- Horizontal Pod Autoscaler
- Security contexts and network policies
- Ingress configuration with TLS

#### Production Package
- Self-contained tar.gz archive
- Pre-built Docker images
- Auto-generated secrets
- One-command deployment script
- Comprehensive documentation

### 📚 Documentation

- **Architecture Guide** — system architecture and design decisions
- **Plugin Architecture** — plugin system design and development guide
- **Data Model** — database schema and relationships
- **Workflow Engine** — workflow automation documentation
- **RBAC & Dashboards** — access control and dashboard documentation
- **AI/RAG Framework** — AI integration documentation
- **User Guides** — comprehensive user documentation (EN/RU)
- **API Documentation** — OpenAPI specification
- **Migration Guides** — guides for migrating from Backstage, Port, Cortex

### 🛠️ Developer Experience

- **CLI Tool** — command-line interface for entity, workflow, and role management
- **SSE Events** — real-time Server-Sent Events for live updates
- **Import Wizard** — import from GitLab, GitHub
- **Pipeline Blueprints** — pre-built pipeline templates
- **Security Scanning** — security scan integration
- **Documentation Portal** — built-in documentation viewer

### 🔧 Technical Details

- **Go Modules** — proper dependency management
- **TypeScript** — type-safe frontend code
- **Repository Pattern** — clean separation of concerns
- **Service Layer** — business logic separation
- **Dependency Injection** — clean dependency management
- **Error Handling** — comprehensive error handling throughout
- **Context Propagation** — proper context handling for cancellation and timeouts

### 📦 Plugin Ecosystem

**All 20 plugins are free and open-source:**
- GitHub — Git provider integration
- GitLab — Git provider integration
- Bitbucket — Git provider integration
- Gitea — Self-hosted Git service
- ArgoCD — GitOps CD engine
- FluxCD — Alternative GitOps engine
- Telegram — Notification channel
- Slack — Notification channel
- Microsoft Teams — Enterprise notifications
- Email — Notification channel
- Webhook — Generic webhook integration
- Syslog — Syslog notification channel
- Proxmox — Virtualization management
- VMware — vSphere VM management (community)
- Prometheus — Monitoring integration
- S3 — Object storage (MinIO/AWS)
- Jira — Task tracker integration
- Trivy — Vulnerability scanning
- AI Bot — AI-powered bot integration
- Remote Console — SSH terminal with command logging

### 🎯 Key Achievements

- ✅ **Clean Architecture** — well-structured, maintainable codebase
- ✅ **Security First** — multi-tenant isolation, RBAC, encryption
- ✅ **Production Ready** — comprehensive deployment options
- ✅ **Well Documented** — extensive documentation in multiple languages
- ✅ **Extensible** — plugin system for custom integrations
- ✅ **Scalable** — designed for high availability and horizontal scaling
- ✅ **Developer Friendly** — CLI tools, SDK, comprehensive documentation

### 📈 Statistics

- **Backend:** ~85,600 lines of Go code
- **Frontend:** ~54,700 lines of TypeScript
- **Plugins:** 20 free, open-source plugins
- **Database:** 47 migrations
- **API Endpoints:** 40+ REST endpoints
- **Frontend Pages:** 60 pages
- **Documentation:** 20+ comprehensive guides

### 🔮 Future Roadmap

- Additional plugins and integrations
- Enhanced AI capabilities
- Advanced monitoring and observability
- Extended integration ecosystem
- Performance optimizations
- Additional deployment targets

---

## [Unreleased]

### Added
- Unified plugin distribution system (all plugins free and open-source)
- Plugin build system with builtin and community targets
- Enhanced documentation for plugin installation
- Production-ready deployment package with build-production.sh

### Changed
- Restructured plugin directory structure
- Updated Makefile with plugin tier support
- Improved build-production.sh for new plugin structure
- Enhanced README with plugin distribution documentation

### Security
- Implemented multi-tenant isolation across all resources
- Added RBAC with fine-grained permissions
- Implemented plugin signature verification
- Added rate limiting middleware
- Implemented audit logging for all actions

---

**Release Date:** 2026-08-27
**Version:** 0.1.0
**Status:** Initial Public Release
- **Plugin Signing** — cryptographic signing and verification of plugin binaries
- **Migration Guides** — from Backstage, Port, Cortex

### Plugins (10 compiled)

| Plugin | Type | Description |
|--------|------|-------------|
| GitHub | Git Provider | Repositories, PRs, webhooks, commit history |
| GitLab | Git Provider | Repositories, CI/CD, merge requests, pipelines |
| Bitbucket | Git Provider | Repositories, PRs, branches, webhooks |
| Gitea | Git Provider | Repositories, PRs, issues, CI/CD integration |
| ArgoCD | CD Engine | GitOps sync, rollback, history, resource management |
| FluxCD | CD Engine | GitOps reconcile, suspend, resume, drift detection |
| Jira | Task Tracker | Issues, projects, transitions, sprint management |
| Slack | Notification | Channel messages, alerts, deployment notifications |
| Prometheus | Monitoring | Metrics queries, alerts, service health |
| Proxmox | Infrastructure | VMs, containers, storage, node management |

### Documentation

- Architecture design (01-architecture.md)
- Plugin architecture (02-plugin-architecture.md)
- Data model (03-data-model.md)
- Workflow engine (04-workflow-engine.md)
- RBAC & dashboards (05-rbac-dashboards.md)
- AI/RAG framework (06-ai-rag-framework.md)
- Roadmap (07-roadmap.md)
- Implementation status (08-implementation-status.md)
- User guide (EN + RU)
- Vault integration guide
- GitOps workflow guide
- OpenTelemetry guide
- Advanced workflows guide
- Migration guides (Backstage, Port, Cortex)
- OpenAPI specification

### Community

- GOVERNANCE.md — merit-based governance with TOC and SIGs
- CODE_OF_CONDUCT.md — Contributor Covenant v2.1
- SECURITY.md — vulnerability disclosure policy
- CONTRIBUTING.md — development setup and PR process
- Apache License 2.0

[0.1.0]: https://github.com/AlexsandrKotov/pepa/releases/tag/v0.1.0
