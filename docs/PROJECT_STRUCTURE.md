# PEPA — Project Structure

> Platform Engineering & Pipeline Automator

```
pepa/
├── cmd/                              # Application entry points
│   ├── api-server/                   # Main API server binary
│   │   └── main.go
│   ├── worker/                       # Background worker binary
│   │   └── main.go
│   └── cli/                          # CLI tool (pepa)
│       ├── main.go
│       └── cmd/
│           ├── root.go               # Root command, global flags
│           └── commands.go           # entity, workflow, role commands
│
├── internal/                         # Private application code
│   ├── api/
│   │   ├── rest/                     # REST API handlers
│   │   │   ├── router.go             # Route definitions, Dependencies struct
│   │   │   ├── entity_handlers.go    # Entity CRUD + graph + relationships
│   │   │   ├── workflow_handlers.go  # Workflow CRUD + templates + execution
│   │   │   ├── scorecard_handlers.go # Scorecard CRUD + evaluation
│   │   │   ├── plugin_handlers.go    # Plugin management
│   │   │   ├── rbac_handlers.go      # Roles, permissions, assignments
│   │   │   ├── audit_handlers.go     # Audit log + stats
│   │   │   ├── ai_handlers.go        # AI chat, stream, status, tools
│   │   │   ├── gitops_handlers.go    # GitOps repos, drift, scan
│   │   │   ├── vault_handlers.go     # Vault secret browser, KV operations
│   │   │   ├── docker_handlers.go    # Docker hosts, containers, services
│   │   │   ├── virtualization_handlers.go  # Proxmox VMs, containers
│   │   │   ├── connection_handlers.go      # Connections CRUD + health
│   │   │   ├── cluster_handlers.go   # Kubernetes clusters
│   │   │   ├── pipeline_handlers.go  # CI/CD pipelines
│   │   │   ├── environment_handlers.go     # Environments
│   │   │   ├── helm_handlers.go      # Helm repositories
│   │   │   ├── import_handlers.go    # Import from GitLab/GitHub
│   │   │   ├── discovery_handlers.go # Service discovery
│   │   │   └── sse_handlers.go       # Server-Sent Events (real-time)
│   │   ├── graphql/                  # GraphQL (planned)
│   │   └── middleware/               # HTTP middleware
│   │       └── ratelimit.go          # Token-bucket rate limiter
│   │
│   ├── auth/                         # JWT authentication
│   │   └── jwt.go                    # Middleware, token helpers
│   ├── config/                       # Configuration loading
│   │   └── config.go
│   ├── database/                     # Database connections
│   │   ├── db.go                     # PostgreSQL (pgx)
│   │   └── redis.go                  # Redis client
│   │
│   ├── plugin/                       # Plugin system
│   │   ├── engine/
│   │   │   └── manager.go            # Plugin lifecycle, enable/disable, health
│   │   ├── sdk-go/
│   │   │   └── sdk.go                # Go Plugin SDK: Plugin interface, Serve()
│   │   └── runtime/                  # Plugin runtime (planned)
│   │
│   ├── workflow/                     # Workflow engine
│   │   ├── engine.go                 # Core workflow execution
│   │   ├── templates/
│   │   │   └── templates.go          # 5 built-in workflow templates
│   │   ├── dag/                      # DAG data structures (planned)
│   │   └── scheduler/                # Step scheduling (planned)
│   │
│   ├── entity/                       # Entity management
│   │   ├── graph/
│   │   │   └── engine.go             # Graph operations, traversal
│   │   ├── sync/                     # Entity sync (planned)
│   │   └── types/                    # Entity type registry (planned)
│   │
│   ├── rbac/                         # Access control
│   │   ├── engine/
│   │   │   └── engine.go             # RBAC data layer, seeding
│   │   ├── policy/                   # Policy definitions (planned)
│   │   └── opa/                      # OPA integration (planned)
│   │
│   ├── gitops/                       # GitOps engine
│   │   ├── scanner.go                # Repository scanner
│   │   ├── drift.go                  # Drift detection
│   │   ├── reconciler.go             # State reconciliation
│   │   └── repository.go             # Git repository management
│   │
│   ├── vault/                        # Vault integration
│   │   ├── client.go                 # HashiCorp Vault client
│   │   └── kv.go                     # KV v2 operations
│   │
│   ├── docker/                       # Docker management
│   │   └── client.go                 # Docker host client
│   │
│   ├── k8s/                          # Kubernetes client
│   │   ├── client.go                 # K8s client wrapper
│   │   ├── cluster.go                # Cluster operations
│   │   └── resources.go              # Resource management
│   │
│   ├── provider/                     # Provider abstractions
│   │   ├── git.go                    # Git provider interface
│   │   ├── cd.go                     # CD engine interface
│   │   ├── tracker.go                # Task tracker interface
│   │   └── monitoring.go             # Monitoring interface
│   │
│   ├── bootstrap/                    # Bootstrap & setup
│   │   ├── bootstrap.go              # Initial setup logic
│   │   └── token.go                  # Bootstrap token management
│   │
│   ├── crypto/                       # Cryptography
│   │   ├── encryption.go             # AES-256 encryption
│   │   └── hash.go                   # Hashing utilities
│   │
│   ├── storage/                      # Object storage
│   │   ├── s3.go                     # S3/MinIO client
│   │   └── bucket.go                 # Bucket operations
│   │
│   ├── events/                       # Event bus
│   │   └── bus.go                    # Redis Pub/Sub event bus
│   ├── queue/                        # Job queue
│   │   └── queue.go                  # Redis-backed job queue
│   │
│   ├── ai/                           # AI/RAG framework
│   │   ├── provider.go               # LLM provider interface + types
│   │   ├── manager.go                # Provider registry, tool management
│   │   ├── tools.go                  # 5 built-in AI tools + ToolRegistry
│   │   ├── rag.go                    # RAG pipeline scaffold
│   │   ├── openai.go                 # OpenAI provider
│   │   ├── anthropic.go              # Anthropic provider
│   │   ├── ollama.go                 # Ollama provider
│   │   └── lmstudio.go               # LM Studio provider
│   │
│   └── repository/                   # Data access layer
│       ├── entity_repo.go
│       ├── workflow_repo.go
│       ├── plugin_repo.go
│       ├── scorecard_repo.go
│       ├── audit_repo.go
│       ├── connection_repo.go
│       ├── cluster_repo.go
│       ├── pipeline_repo.go
│       ├── environment_repo.go
│       ├── helm_repo.go
│       ├── gitops_repo.go
│       ├── vault_repo.go
│       ├── docker_repo.go
│       ├── virtualization_repo.go
│       ├── user_repo.go
│       ├── role_repo.go
│       └── credential_repo.go
│
├── pkg/                              # Public reusable packages
│   ├── models/                       # Shared data models
│   │   ├── entity.go                 # Entity, EntityType, Relationship
│   │   ├── workflow.go               # Workflow, WorkflowSpec, StepSpec
│   │   ├── plugin.go                 # Plugin, PluginHealth
│   │   ├── scorecard.go              # Scorecard, ScorecardRule, ScorecardResult
│   │   ├── audit.go                  # AuditEntry
│   │   ├── connection.go             # Connection, ConnectionType
│   │   ├── cluster.go                # Cluster, Node, Namespace
│   │   └── pipeline.go               # Pipeline, PipelineRun
│   └── utils/                        # Utility functions
│
├── plugins/                          # Plugin implementations
│   ├── github/                       # GitHub Git provider
│   │   ├── main.go
│   │   ├── plugin.go
│   │   └── actions.go                # repos, PRs, webhooks, commits
│   ├── gitlab/                       # GitLab Git + CI/CD provider
│   │   ├── main.go
│   │   ├── plugin.go
│   │   └── actions.go                # repos, CI/CD, merge requests
│   ├── bitbucket/                    # Bitbucket Git provider
│   │   ├── main.go
│   │   ├── plugin.go
│   │   └── actions.go                # repos, PRs, branches
│   ├── gitea/                        # Gitea Git provider
│   │   ├── main.go
│   │   ├── plugin.go
│   │   └── actions.go                # repos, PRs, issues, CI/CD
│   ├── argocd/                       # ArgoCD CD engine
│   │   ├── main.go
│   │   └── actions.go                # sync, rollback, history
│   ├── fluxcd/                       # FluxCD GitOps engine
│   │   ├── main.go
│   │   ├── plugin.go
│   │   └── actions.go                # reconcile, suspend, resume
│   ├── jira/                         # Jira task tracker
│   │   └── main.go                   # issues, projects, transitions
│   ├── slack/                        # Slack notifications
│   │   ├── main.go
│   │   └── plugin.go                 # channel messages, alerts
│   ├── prometheus/                   # Prometheus monitoring
│   │   └── main.go                   # metrics, alerts, health
│   ├── proxmox/                      # Proxmox virtualization
│   │   ├── main.go
│   │   ├── plugin.go
│   │   ├── client.go                 # Proxmox API client
│   │   └── deploy.go                 # VM/container deployment
│   ├── builtin/                      # Built-in plugin manifests
│   │   ├── github/plugin.yaml
│   │   ├── argocd/plugin.yaml
│   │   ├── fluxcd/plugin.yaml
│   │   ├── slack/plugin.yaml
│   │   ├── prometheus/plugin.yaml
│   │   └── jira/plugin.yaml
│   ├── examples/
│   │   └── example_plugin.go         # Example plugin using SDK
│   └── bin/                          # Compiled plugin binaries
│       ├── github/
│       ├── gitlab/
│       ├── bitbucket/
│       ├── gitea/
│       ├── argocd/
│       ├── fluxcd/
│       ├── jira/
│       ├── slack/
│       ├── prometheus/
│       └── proxmox/
│
├── frontend/                         # Next.js 14 frontend
│   ├── app/                          # App Router pages (52 pages)
│   │   ├── layout.tsx                # Root layout with Sidebar + TopBar
│   │   ├── page.tsx                  # Dashboard
│   │   ├── login/page.tsx            # Login page
│   │   ├── setup/page.tsx            # Initial setup
│   │   ├── services/
│   │   │   ├── page.tsx              # Service catalog
│   │   │   └── new/page.tsx          # Deploy new service
│   │   ├── deployments/
│   │   │   ├── page.tsx              # Deployment history
│   │   │   └── new/page.tsx          # New deployment
│   │   ├── clusters/page.tsx         # Kubernetes clusters
│   │   ├── connections/page.tsx      # Integration connections
│   │   ├── gitops/
│   │   │   ├── page.tsx              # GitOps overview
│   │   │   ├── repos/page.tsx        # Repository management
│   │   │   └── drift/page.tsx        # Drift detection
│   │   ├── pipelines/
│   │   │   ├── page.tsx              # Pipeline list
│   │   │   ├── new/page.tsx          # New pipeline
│   │   │   └── edit/page.tsx         # Edit pipeline
│   │   ├── pipeline-builder/page.tsx # Visual pipeline editor
│   │   ├── pipeline-blueprints/page.tsx  # Pipeline templates
│   │   ├── workflows/
│   │   │   ├── page.tsx              # Workflow management
│   │   │   └── designer/page.tsx     # Visual workflow designer
│   │   ├── virtualization/
│   │   │   ├── page.tsx              # Virtualization overview
│   │   │   ├── vms/page.tsx          # Virtual machines
│   │   │   ├── containers/page.tsx   # Containers
│   │   │   ├── hosts/page.tsx        # Proxmox hosts
│   │   │   └── logs/page.tsx         # VM/container logs
│   │   ├── docker-hosts/page.tsx     # Docker hosts
│   │   ├── docker-services/page.tsx  # Docker services
│   │   ├── environments/page.tsx     # Deployment environments
│   │   ├── helm-repositories/page.tsx # Helm repos
│   │   ├── scorecards/page.tsx       # Scorecard management
│   │   ├── marketplace/page.tsx      # Plugin marketplace
│   │   ├── plugins/page.tsx          # Plugin management
│   │   ├── roles/page.tsx            # RBAC roles
│   │   ├── settings/
│   │   │   ├── page.tsx              # Settings overview
│   │   │   ├── users/page.tsx        # User management
│   │   │   ├── teams/page.tsx        # Team management
│   │   │   ├── workspaces/page.tsx   # Workspace settings
│   │   │   └── plugins/page.tsx      # Plugin settings
│   │   ├── ai/page.tsx               # AI Assistant
│   │   ├── audit/page.tsx            # Audit log
│   │   ├── credentials/page.tsx      # Vault credentials
│   │   ├── jira/page.tsx             # Jira integration
│   │   ├── discovery/page.tsx        # Service discovery
│   │   ├── import/page.tsx           # Import wizard
│   │   ├── security/page.tsx         # Security scanning
│   │   ├── docs/page.tsx             # Documentation
│   │   ├── entities/page.tsx         # Entity management
│   │   ├── deploy/page.tsx           # Deploy overview
│   │   ├── automation/page.tsx       # Automation workflows
│   │   ├── cicd/page.tsx             # CI/CD overview
│   │   └── get-started/page.tsx      # Getting started guide
│   ├── components/                   # React components (30+)
│   │   ├── Sidebar.tsx               # Navigation sidebar
│   │   ├── TopBar.tsx                # Top bar with breadcrumbs
│   │   ├── GraphExplorer.tsx         # Entity graph visualization
│   │   ├── Interactive.tsx           # Interactive components
│   │   ├── DashboardWidgets.tsx      # Dashboard stat widgets
│   │   ├── DashboardAIWidget.tsx     # AI chat widget
│   │   ├── CommandPalette.tsx        # Command palette (Cmd+K)
│   │   ├── Breadcrumbs.tsx           # Breadcrumb navigation
│   │   ├── Pagination.tsx            # Pagination component
│   │   ├── EmptyState.tsx            # Empty state placeholder
│   │   ├── ConfirmModal.tsx          # Confirmation modal
│   │   ├── CreateRoleModal.tsx       # Create role modal
│   │   ├── EditRoleModal.tsx         # Edit role modal
│   │   ├── PermissionGuard.tsx       # Permission wrapper
│   │   ├── VaultInput.tsx            # Vault secret picker
│   │   ├── GitRepoPicker.tsx         # Git repository picker
│   │   ├── OnboardingTour.tsx        # Onboarding tour
│   │   ├── QuickActions.tsx          # Quick action buttons
│   │   ├── ServiceManagementPanel.tsx # Service management
│   │   ├── BrandIcon.tsx             # Brand icons
│   │   ├── AiMarkdown.tsx            # AI markdown renderer
│   │   ├── ClientLayout.tsx          # Client layout wrapper
│   │   ├── CollapsibleSection.tsx    # Collapsible sections
│   │   ├── ConceptHelp.tsx           # Concept help tooltips
│   │   ├── DynamicComponents.tsx     # Dynamic component loader
│   │   ├── ResizableTable.tsx        # Resizable table
│   │   └── Skeleton.tsx              # Loading skeleton
│   ├── hooks/                        # React hooks
│   │   ├── useApi.ts                 # API client hook
│   │   ├── useDashboardProfile.ts    # Dashboard preferences
│   │   ├── useDebounce.ts            # Debounce hook
│   │   ├── useEscapeKey.ts           # Escape key handler
│   │   └── useLocalStorage.ts        # Local storage hook
│   └── lib/
│       ├── api.ts                    # API client (all endpoints)
│       ├── utils.ts                  # Utility functions
│       ├── constants.ts              # Constants
│       └── types.ts                  # TypeScript types
│
├── deployments/
│   ├── helm/
│   │   └── pepa/                     # Helm chart
│   │       ├── Chart.yaml
│   │       ├── values.yaml
│   │       └── templates/
│   │           ├── _helpers.tpl
│   │           ├── deployments.yaml  # API, Worker, Frontend
│   │           ├── services.yaml
│   │           ├── ingress.yaml
│   │           └── serviceaccount.yaml
│   ├── docker/
│   │   ├── Dockerfile.api
│   │   ├── Dockerfile.worker
│   │   └── Dockerfile.frontend
│   └── compose/
│       ├── docker-compose.yml        # Full dev/prod deployment
│       ├── docker-compose.prod.yml   # Production override
│       ├── .env                      # Environment config
│       ├── .env.example              # Environment template
│       ├── quickstart.sh             # Interactive quickstart
│       ├── init-db.sql               # Database initialization
│       ├── nginx.conf                # Nginx config
│       ├── prometheus.yml            # Prometheus config
│       ├── grafana-datasources.yml   # Grafana datasources
│       └── nginx/
│           ├── nginx.conf            # Nginx reverse proxy
│           ├── conf.d/               # Additional configs
│           └── ssl/                  # TLS certificates
│
├── migrations/                       # PostgreSQL migrations (31 files)
│   ├── 001_initial_schema.sql        # entities, workflows, plugins
│   ├── 002_plugins_workflows.sql     # plugin enhancements
│   ├── 003_dashboards_rag.sql        # dashboards, RAG
│   ├── 004_scorecards.sql            # scorecards
│   ├── 005_connections_clusters.sql  # connections, clusters
│   ├── 006_service_portal.sql        # service portal
│   ├── 007_infrastructure.sql        # infrastructure
│   ├── 008_pipeline_sources.sql      # pipeline sources
│   ├── 009_fix_pipeline_runs_cascade.sql
│   ├── 010_vault_security.sql        # vault integration
│   ├── 011_pipeline_run_jobs_unique.sql
│   ├── 012_deployment_logs.sql       # deployment logs
│   ├── 013_rls_security.sql          # row-level security
│   ├── 014_gitops_repositories.sql   # gitops repos
│   ├── 015_deployment_timeout.sql    # deployment timeout
│   ├── 016_jira_automation.sql       # jira integration
│   ├── 017_auth_system.sql           # authentication
│   ├── 018_environments_enhanced.sql # environments
│   ├── 019_performance_indexes.sql   # performance
│   ├── 020_fix_vault_rbac.sql        # vault RBAC
│   ├── 021_bootstrap_token.sql       # bootstrap token
│   ├── 022_service_blueprints.sql    # service blueprints
│   ├── 023_rls_remaining_tables.sql  # RLS completion
│   ├── 024_rbac_permissions.sql      # RBAC permissions
│   ├── 025_fix_bootstrap_must_change_password.sql
│   ├── 026_users_token_version.sql   # token versioning
│   ├── 027_gitops_workflow_chain.sql # gitops workflows
│   ├── 028_proxmox_virtualization.sql # proxmox
│   └── embed.go                      # Migration embed
│
├── .github/
│   └── workflows/
│       └── ci.yml                    # GitHub Actions CI
│
├── docs/                             # Documentation (16+ files)
│   ├── 01-architecture.md            # System architecture
│   ├── 02-plugin-architecture.md     # Plugin engine design
│   ├── 03-data-model.md              # Entity data model
│   ├── 04-workflow-engine.md         # Workflow engine design
│   ├── 05-rbac-dashboards.md         # RBAC & dashboard design
│   ├── 06-ai-rag-framework.md        # AI/RAG framework design
│   ├── 07-roadmap.md                 # CNCF journey & roadmap
│   ├── 08-implementation-status.md   # Implementation tracker
│   ├── PROJECT_STRUCTURE.md          # This file
│   ├── README.md                     # Documentation index
│   ├── user-guide-en.md              # User guide (English)
│   ├── user-guide-ru.md              # User guide (Russian)
│   ├── vault-integration-guide.md    # Vault integration
│   ├── gitlab-workflow-guide.md      # GitLab workflow
│   ├── opentelemetry-signoz-guide.md # OpenTelemetry
│   ├── pepa-master-plan.md           # Master plan
│   ├── phase1-core-foundation.md     # Phase 1 plan
│   ├── phase2-automation-security.md # Phase 2 plan
│   ├── phase3-observability-optimization.md # Phase 3 plan
│   ├── phase4-advanced-features.md   # Phase 4 plan
│   ├── connection-testing-plan.md    # Connection testing
│   ├── CHANGELOG.md                  # Plan updates
│   └── openapi.yaml                  # OpenAPI 3.0 specification
│
├── scripts/
│   └── build-production.sh           # Production build script
│
├── go.mod                            # Go module definition
├── go.sum
├── Makefile                          # Build, test, docker, helm targets
├── README.md                         # Project overview (365 lines)
├── LICENSE                           # Apache 2.0
├── CONTRIBUTING.md                   # Contribution guide
├── .env.example                      # Environment configuration template
├── .golangci.yml                     # Linter configuration
└── .gitignore
```

## Key Directories

| Directory | Purpose | Files |
|-----------|---------|-------|
| `cmd/` | Application entry points | 3 binaries (api-server, worker, cli) |
| `internal/` | Private application code | ~50 Go files |
| `pkg/` | Public reusable packages | Models, utils |
| `plugins/` | Plugin implementations | 10 plugins (~6,000 lines) |
| `frontend/` | Next.js 14 frontend | 52 pages, 30+ components |
| `deployments/` | Deployment configs | Docker, Helm, Compose |
| `migrations/` | Database migrations | 31 SQL files |
| `docs/` | Documentation | 16+ markdown files |

## Entry Points

| Binary | Path | Purpose |
|--------|------|---------|
| API Server | `cmd/api-server/main.go` | Main REST API server |
| Worker | `cmd/worker/main.go` | Background job processor |
| CLI | `cmd/cli/main.go` | Command-line tool |

## Plugin Binaries

All plugins compile to `plugins/bin/<name>/<name>`:

```
plugins/bin/
├── github/github
├── gitlab/gitlab
├── bitbucket/bitbucket
├── gitea/gitea
├── argocd/argocd
├── fluxcd/fluxcd
├── jira/jira
├── slack/slack
├── prometheus/prometheus
└── proxmox/proxmox
```
