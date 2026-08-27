# Plugin Architecture & Modularity

## 1. Design Philosophy

The plugin system is the **backbone** of PEPA. Every external integration — whether it's a Git provider, task tracker, CI/CD engine, cloud provider, or monitoring tool — is implemented as an out-of-process plugin communicating over gRPC. The core platform ships with **zero hardcoded vendor dependencies**.

### Key Goals

- **Zero core modifications** to add new integrations
- **Process isolation** — a crashing plugin cannot bring down the platform
- **Hot-reloadable** — enable, disable, upgrade plugins at runtime without restart
- **Multi-language support** — plugins can be written in Go, Python, Node.js, Rust, Java (anything that speaks gRPC)
- **Versioned & signed** — plugin registry enforces semver and cryptographic signatures

---

## 2. Plugin Engine Architecture

### 2.1 Host Process (Go Core)

The Plugin Engine uses **HashiCorp go-plugin** as the foundation, providing:

```
┌────────────────────────────────────────────────────────────────┐
│                     PLUGIN ENGINE (Host)                        │
│                                                                 │
│  ┌─────────────────┐    ┌──────────────────────────────────┐  │
│  │ Plugin Registry │    │ Plugin Manager                    │  │
│  │ (Metadata DB)   │    │                                   │  │
│  │                 │    │  ┌──────────┐  ┌──────────┐      │  │
│  │  - name         │    │  │ Lifecycle │  │ Health   │      │  │
│  │  - version      │    │  │ Controller│  │ Monitor  │      │  │
│  │  - type         │    │  └──────────┘  └──────────┘      │  │
│  │  - capabilities │    │  ┌──────────┐  ┌──────────┐      │  │
│  │  - config       │    │  │ Config   │  │ Event    │      │  │
│  │  - enabled      │    │  │ Resolver │  │ Router   │      │  │
│  └─────────────────┘    │  └──────────┘  └──────────┘      │  │
│                          └──────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                Plugin Pool (Process Per Plugin)           │  │
│  │                                                           │  │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐    │  │
│  │  │ Jira    │  │ ArgoCD  │  │ AWS     │  │ GitHub  │    │  │
│  │  │ Plugin  │  │ Plugin  │  │ Plugin  │  │ Plugin  │    │  │
│  │  │ (proc)  │  │ (proc)  │  │ (proc)  │  │ (proc)  │    │  │
│  │  │ :10001  │  │ :10002  │  │ :10003  │  │ :10004  │    │  │
│  │  └─────────┘  └─────────┘  └─────────┘  └─────────┘    │  │
│  └──────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────┘
```

### 2.2 Plugin Process Model

Each plugin runs as an **isolated subprocess**:

```go
// Plugin interface definitions (Go)

package plugin

// BasePlugin — every plugin must implement this
type BasePlugin interface {
    // Init is called once at plugin startup
    Init(ctx context.Context, config *PluginConfig) error
    
    // HealthCheck returns the current health status
    HealthCheck(ctx context.Context) (*HealthStatus, error)
    
    // Capabilities returns what this plugin can do
    Capabilities() *Capabilities
    
    // Shutdown gracefully stops the plugin
    Shutdown(ctx context.Context) error
}

// PluginType enumerates the integration categories
type PluginType string

const (
    PluginTypeGitProvider    PluginType = "git_provider"
    PluginTypeTaskTracker    PluginType = "task_tracker"
    PluginTypeCDEngine       PluginType = "cd_engine"
    PluginTypeCloudProvider  PluginType = "cloud_provider"
    PluginTypeMonitoring     PluginType = "monitoring"
    PluginTypeSecretManager  PluginType = "secret_manager"
    PluginTypeNotification   PluginType = "notification"
    PluginTypeCustom         PluginType = "custom"
)
```

---

## 3. Provider Abstraction Interfaces

### 3.1 GitProvider Interface

```go
// GitProvider — abstracts source code management
type GitProvider interface {
    BasePlugin
    
    // Repository operations
    ListRepositories(ctx context.Context, opts *ListOptions) (*RepositoryList, error)
    GetRepository(ctx context.Context, id string) (*Repository, error)
    
    // Pull/Merge Request operations
    ListPullRequests(ctx context.Context, repoID string, opts *PROptions) (*PRList, error)
    GetPullRequest(ctx context.Context, repoID string, prID string) (*PullRequest, error)
    MergePullRequest(ctx context.Context, repoID string, prID string, opts *MergeOptions) error
    
    // Branch & Commit operations
    ListBranches(ctx context.Context, repoID string, opts *ListOptions) (*BranchList, error)
    GetCommitHistory(ctx context.Context, repoID string, opts *CommitOptions) (*CommitList, error)
    GetDiff(ctx context.Context, repoID string, ref string) (*Diff, error)
    
    // Webhook management
    CreateWebhook(ctx context.Context, repoID string, hook *WebhookConfig) (*Webhook, error)
    DeleteWebhook(ctx context.Context, repoID string, hookID string) error
    HandleWebhookPayload(ctx context.Context, payload []byte, headers map[string]string) (*WebhookEvent, error)
    
    // File operations
    GetFileContent(ctx context.Context, repoID string, path string, ref string) ([]byte, error)
    ListFiles(ctx context.Context, repoID string, path string, ref string) (*FileList, error)
}
```

### 3.2 TaskTracker Interface

```go
// TaskTracker — abstracts issue/project tracking
type TaskTracker interface {
    BasePlugin
    
    // Project operations
    ListProjects(ctx context.Context, opts *ListOptions) (*ProjectList, error)
    GetProject(ctx context.Context, id string) (*Project, error)
    
    // Issue operations
    ListIssues(ctx context.Context, opts *IssueFilter) (*IssueList, error)
    GetIssue(ctx context.Context, id string) (*Issue, error)
    CreateIssue(ctx context.Context, issue *IssueCreate) (*Issue, error)
    UpdateIssue(ctx context.Context, id string, update *IssueUpdate) (*Issue, error)
    TransitionIssue(ctx context.Context, id string, transitionID string) error
    
    // Sprint/Board operations
    ListSprints(ctx context.Context, projectID string) (*SprintList, error)
    GetBoard(ctx context.Context, projectID string) (*Board, error)
    
    // Search
    SearchIssues(ctx context.Context, query string, opts *SearchOptions) (*IssueList, error)
    
    // Webhook/event subscription
    SubscribeEvents(ctx context.Context, filter *EventFilter) (<-chan *IssueEvent, error)
}
```

### 3.3 CDEngine Interface

```go
// CDEngine — abstracts continuous deployment systems
type CDEngine interface {
    BasePlugin
    
    // Environment management
    ListEnvironments(ctx context.Context, opts *ListOptions) (*EnvironmentList, error)
    GetEnvironment(ctx context.Context, id string) (*Environment, error)
    
    // Deployment operations
    Deploy(ctx context.Context, req *DeployRequest) (*Deployment, error)
    GetDeploymentStatus(ctx context.Context, deployID string) (*DeploymentStatus, error)
    Rollback(ctx context.Context, deployID string, opts *RollbackOptions) (*Deployment, error)
    CancelDeployment(ctx context.Context, deployID string) error
    
    // Application/Service management
    ListApplications(ctx context.Context, opts *ListOptions) (*ApplicationList, error)
    SyncApplication(ctx context.Context, appID string, opts *SyncOptions) error
    
    // Promotion
    PromoteToEnvironment(ctx context.Context, appID string, fromEnv string, toEnv string) (*Promotion, error)
    
    // Deployment history
    GetDeploymentHistory(ctx context.Context, appID string, opts *HistoryOptions) (*DeploymentList, error)
}
```

### 3.4 CloudProvider Interface

```go
// CloudProvider — abstracts infrastructure provisioning
type CloudProvider interface {
    BasePlugin
    
    // Resource lifecycle
    ProvisionResource(ctx context.Context, req *ProvisionRequest) (*Resource, error)
    GetResource(ctx context.Context, id string) (*Resource, error)
    UpdateResource(ctx context.Context, id string, update *ResourceUpdate) (*Resource, error)
    DestroyResource(ctx context.Context, id string) error
    
    // Resource discovery
    ListResources(ctx context.Context, opts *ResourceFilter) (*ResourceList, error)
    ListResourceTypes(ctx context.Context) (*ResourceTypeList, error)
    DescribeResourceType(ctx context.Context, typeID string) (*ResourceTypeSchema, error)
    
    // Cost estimation
    EstimateCost(ctx context.Context, req *ProvisionRequest) (*CostEstimate, error)
    GetCostBreakdown(ctx context.Context, opts *CostOptions) (*CostBreakdown, error)
    
    // Region/Zone operations
    ListRegions(ctx context.Context) (*RegionList, error)
    ListZones(ctx context.Context, regionID string) (*ZoneList, error)
}
```

### 3.5 Monitoring Interface

```go
// Monitoring — abstracts observability backends
type Monitoring interface {
    BasePlugin
    
    // Metrics
    QueryMetrics(ctx context.Context, query string, timeRange *TimeRange) (*MetricResult, error)
    ListMetricNames(ctx context.Context, filter *MetricFilter) (*MetricNameList, error)
    
    // Alerts
    ListAlerts(ctx context.Context, opts *AlertFilter) (*AlertList, error)
    GetAlert(ctx context.Context, id string) (*Alert, error)
    AcknowledgeAlert(ctx context.Context, id string) error
    
    // Dashboards (read)
    ListDashboards(ctx context.Context) (*DashboardList, error)
    GetDashboard(ctx context.Context, id string) (*Dashboard, error)
    
    // Log queries
    QueryLogs(ctx context.Context, query *LogQuery) (*LogResult, error)
    
    // Service health
    GetServiceHealth(ctx context.Context, serviceID string) (*ServiceHealth, error)
}
```

---

## 4. Plugin Configuration & Lifecycle

### 4.1 Plugin Manifest (plugin.yaml)

Every plugin ships with a manifest:

```yaml
# plugin.yaml — Jira Task Tracker Plugin
apiVersion: pepa.github.io/v1alpha1
kind: Plugin
metadata:
  name: jira-tracker
  version: 1.2.0
  description: "Atlassian Jira integration for issue tracking"
  author: "PEPA Community"
  license: Apache-2.0
  icon: "jira-icon.svg"
  categories:
    - task_tracker
    - project_management

spec:
  type: task_tracker
  
  # Binary or container reference
  runtime:
    type: binary          # binary | container | wasm
    binary: "./jira-plugin"
    # container:
    #   image: "docker.io/pepa/jira-plugin:1.2.0"
    # wasm:
    #   module: "./jira-plugin.wasm"
  
  # Configuration schema (JSON Schema)
  configSchema:
    type: object
    required:
      - baseURL
      - auth
    properties:
      baseURL:
        type: string
        description: "Jira instance URL"
        example: "https://mycompany.atlassian.net"
      auth:
        type: object
        properties:
          type:
            type: string
            enum: [oauth2, api_token, basic]
          token:
            type: string
            format: password
          clientID:
            type: string
          clientSecret:
            type: string
            format: password
      projectKeys:
        type: array
        items:
          type: string
        description: "Limit to specific Jira project keys"
  
  # What capabilities this plugin exposes
  capabilities:
    - task_tracker
    - webhook_receiver
  
  # Events this plugin emits
  events:
    - issue.created
    - issue.updated
    - issue.transitioned
    - sprint.started
    - sprint.completed
  
  # Health check configuration
  healthCheck:
    interval: 30s
    timeout: 5s
  
  # Resource limits
  resources:
    memory: "128Mi"
    cpu: "100m"
```

### 4.2 Plugin Lifecycle States

```
                    ┌──────────┐
                    │ Available│  (registered in registry)
                    └────┬─────┘
                         │ install
                         ▼
                    ┌──────────┐
                    │Installed │  (binary/image pulled)
                    └────┬─────┘
                         │ enable
                         ▼
              ┌─────────────────────┐
              │    Initializing     │  (config validated, process starting)
              └─────────┬───────────┘
                        │ health check passes
                        ▼
              ┌─────────────────────┐
         ┌────│     Running        │────┐
         │    │   (healthy,        │    │
         │    │    serving)        │    │
         │    └─────────────────────┘    │
         │                    │          │
         │ health fails       │ disable  │ error
         ▼                    ▼          ▼
    ┌──────────┐       ┌──────────┐  ┌──────────┐
    │ Degraded │       │Stopping  │  │ Failed   │
    └────┬─────┘       └────┬─────┘  └────┬─────┘
         │ recover           │ stopped     │ retry
         ▼                   ▼             ▼
    ┌──────────┐       ┌──────────┐  ┌──────────┐
    │ Running  │       │ Disabled │  │Installed │
    └──────────┘       └──────────┘  └──────────┘
```

### 4.3 Dynamic Enable/Disable Flow

```yaml
# Example: Switching from Jira to Linear

# Step 1: Install Linear plugin
pepa plugin install linear-tracker --version 0.1.0

# Step 2: Configure Linear
pepa plugin configure linear-tracker <<EOF
  baseURL: https://linear.app
  auth:
    type: api_token
    token: ${LINEAR_API_TOKEN}
EOF

# Step 3: Enable Linear (runs alongside Jira)
pepa plugin enable linear-tracker

# Step 4: Migrate entity mappings (issues, projects)
pepa entities migrate-type \
  --from task_tracker:jira-tracker \
  --to task_tracker:linear-tracker \
  --strategy link-both   # link issues to both during transition

# Step 5: Disable Jira (graceful — stops webhooks, preserves data)
pepa plugin disable jira-tracker

# Step 6: Uninstall Jira (optional)
pepa plugin uninstall jira-tracker
```

---

## 5. Plugin Communication Protocol

### 5.1 gRPC Service Definition

```protobuf
syntax = "proto3";
package pepa.plugin.v1;

import "google/protobuf/struct.proto";
import "google/protobuf/timestamp.proto";

// ============================================================
// Base Plugin Service — all plugins implement this
// ============================================================
service BasePluginService {
  rpc Init(InitRequest) returns (InitResponse);
  rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
  rpc GetCapabilities(GetCapabilitiesRequest) returns (CapabilitiesResponse);
  rpc Configure(ConfigureRequest) returns (ConfigureResponse);
  rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
}

// ============================================================
// Generic Resource Service — type-agnostic CRUD
// ============================================================
service ResourceService {
  rpc ListResources(ListResourcesRequest) returns (ListResourcesResponse);
  rpc GetResource(GetResourceRequest) returns (GetResourceResponse);
  rpc CreateResource(CreateResourceRequest) returns (CreateResourceResponse);
  rpc UpdateResource(UpdateResourceRequest) returns (UpdateResourceResponse);
  rpc DeleteResource(DeleteResourceRequest) returns (DeleteResourceResponse);
  
  // Streaming subscription for real-time events
  rpc SubscribeEvents(SubscribeEventsRequest) returns (stream ResourceEvent);
}

// ============================================================
// Webhook Handler — for plugins that receive external webhooks
// ============================================================
service WebhookService {
  rpc RegisterWebhook(RegisterWebhookRequest) returns (RegisterWebhookResponse);
  rpc HandleWebhook(WebhookPayload) returns (WebhookResponse);
}

// ============================================================
// Action Executor — for plugins that support mutations
// ============================================================
service ActionService {
  rpc ListActions(ListActionsRequest) returns (ListActionsResponse);
  rpc ExecuteAction(ExecuteActionRequest) returns (ExecuteActionResponse);
  rpc GetActionStatus(GetActionStatusRequest) returns (GetActionStatusResponse);
}

// ============================================================
// Common Messages
// ============================================================
message Resource {
  string id = 1;
  string type = 2;
  string name = 3;
  string external_id = 4;         // ID in the external system
  map<string, string> labels = 5;
  google.protobuf.Struct metadata = 6;  // Flexible metadata
  google.protobuf.Timestamp created_at = 7;
  google.protobuf.Timestamp updated_at = 8;
  string status = 9;
}

message ResourceEvent {
  string event_type = 1;          // e.g., "issue.created", "deployment.completed"
  Resource resource = 2;
  google.protobuf.Struct payload = 3;
  google.protobuf.Timestamp timestamp = 4;
  string plugin_name = 5;
}
```

### 5.2 Event Bus Integration

Plugins emit events to the core's Redis Pub/Sub bus:

```go
// Plugin-side event emission
type EventBusClient interface {
    // Emit sends an event to the central bus
    Emit(ctx context.Context, event *PluginEvent) error
    
    // Subscribe listens for events from other plugins or the core
    Subscribe(ctx context.Context, filter *EventFilter) (<-chan *PluginEvent, error)
}

// Event routing rules (configured per-plugin)
type EventRouting struct {
    // Which events to forward to the core
    Outbound []string  // e.g., ["issue.*", "deployment.completed"]
    
    // Which events to receive from the core
    Inbound []string   // e.g., ["entity.created", "workflow.triggered"]
}
```

---

## 6. Plugin SDK

### 6.1 Go SDK (Primary)

```go
package main

import (
    "context"
    sdk "github.com/pepa/plugin-sdk-go"
)

func main() {
    // Serve the plugin — handles gRPC setup, health checks, lifecycle
    sdk.Serve(&MyGitProvider{
        // inject dependencies
    })
}

type MyGitProvider struct {
    sdk.UnimplementedGitProvider  // embed for forward compatibility
    client *github.Client
}

func (p *MyGitProvider) Init(ctx context.Context, config *sdk.PluginConfig) error {
    // Parse config, initialize client
    baseURL := config.GetString("baseURL")
    token := config.GetString("auth.token")
    p.client = github.NewClient(baseURL, token)
    return nil
}

func (p *MyGitProvider) ListRepositories(ctx context.Context, opts *sdk.ListOptions) (*sdk.RepositoryList, error) {
    repos, err := p.client.ListRepos(ctx, opts.Page, opts.PerPage)
    if err != nil {
        return nil, err
    }
    return convertToSDKRepos(repos), nil
}

// ... implement other methods
```

### 6.2 Python SDK (Secondary)

```python
from pepa_sdk import PluginBase, GitProvider, serve

class MyGitProvider(GitProvider):
    def init(self, config: PluginConfig) -> None:
        self.client = GitHubClient(
            base_url=config.get("baseURL"),
            token=config.get("auth.token"),
        )
    
    def list_repositories(self, opts: ListOptions) -> RepositoryList:
        return self.client.list_repos(page=opts.page, per_page=opts.per_page)
    
    def get_pull_request(self, repo_id: str, pr_id: str) -> PullRequest:
        return self.client.get_pr(repo_id, pr_id)

if __name__ == "__main__":
    serve(MyGitProvider())
```

### 6.3 Plugin Development CLI

```bash
# Scaffold a new plugin
pepa plugin init my-provider --type git_provider --lang go

# Output:
# my-provider/
# ├── plugin.yaml          # Manifest
# ├── main.go              # Entry point with interface stubs
# ├── internal/
# │   ├── client.go        # External API client
# │   └── converter.go     # Type converters
# ├── config.schema.json   # JSON Schema for config
# ├── Makefile
# └── README.md

# Build
pepa plugin build

# Test locally (starts plugin in dev mode with mock core)
pepa plugin dev

# Package for registry
pepa plugin package

# Publish to registry
pepa plugin publish --registry docker.io/pepa/plugins
```

---

## 7. Plugin Registry & Marketplace

### 7.1 Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                   Plugin Registry                             │
│                                                               │
│  ┌─────────────────┐   ┌─────────────────────────────────┐  │
│  │ OCI Registry    │   │ Registry API                     │  │
│  │ (ghcr.io/       │   │                                  │  │
│  │  pepa/    │   │  GET  /v1/plugins                │  │
│  │  plugins/)      │   │  GET  /v1/plugins/{name}         │  │
│  │                 │   │  POST /v1/plugins/{name}/review   │  │
│  │  - jira:1.2.0   │   │  GET  /v1/plugins/{name}/versions│  │
│  │  - argocd:2.0.1 │   │  GET  /v1/categories             │  │
│  │  - aws:1.5.0    │   │  GET  /v1/popular                │  │
│  └─────────────────┘   └─────────────────────────────────┘  │
│                                                               │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │ Verification Pipeline                                    │ │
│  │                                                          │ │
│  │  1. Signature verification (cosign)                     │ │
│  │  2. Security scan (trivy)                               │ │
│  │  3. Interface compliance test (contract testing)        │ │
│  │  4. Integration test suite (against mock services)      │ │
│  │  5. Community review & approval                         │ │
│  └─────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

### 7.2 Plugin Categories

| Category | Description | Examples |
|----------|-------------|---------|
| `git_provider` | Source code management | GitHub, GitLab, Bitbucket, Gitea |
| `task_tracker` | Issue & project tracking | Jira, Linear, GitHub Issues, Azure DevOps |
| `cd_engine` | Deployment orchestration | ArgoCD, FluxCD, Tekton, Spinnaker |
| `cloud_provider` | Infrastructure provisioning | AWS, GCP, Azure, Terraform, Pulumi |
| `monitoring` | Observability & alerting | Prometheus, Datadog, Grafana, PagerDuty |
| `secret_manager` | Secrets & configuration | Vault, AWS Secrets Manager, SOPS |
| `notification` | Alerts & messaging | Slack, Teams, Discord, Email, Webhook |
| `ci_engine` | CI pipeline integration | GitHub Actions, GitLab CI, Jenkins, CircleCI |
| `identity` | Authentication providers | OIDC, LDAP, SAML, GitHub OAuth |
| `custom` | User-defined integrations | Any gRPC-compliant plugin |

---

## 8. Plugin Security Model

### 8.1 Sandboxing

```yaml
# Security policy per plugin
security:
  # Network policy — restrict outbound connections
  networkPolicy:
    allowedHosts:
      - "api.github.com"
      - "*.atlassian.net"
    deniedCIDRs:
      - "10.0.0.0/8"       # Block internal cluster network
      - "169.254.169.254/32" # Block cloud metadata
  
  # Filesystem — read-only root, tmp volume
  filesystem:
    readOnlyRoot: true
    allowedPaths:
      - /tmp
      - /plugin/data
  
  # Resource limits
  resources:
    memory:
      request: "64Mi"
      limit: "256Mi"
    cpu:
      request: "50m"
      limit: "500m"
  
  # Capabilities (Linux)
  capabilities:
    drop:
      - ALL
```

### 8.2 Secret Management

Plugins never see raw secrets. The core provides a **Secret Injection Layer**:

```go
// Plugin receives resolved values, not secret references
type PluginConfig struct {
    Values map[string]string  // Resolved at runtime
    
    // Secret references are resolved by the core:
    // config.auth.token = "ref:vault://jira/token" → resolved to actual token
    // The plugin only sees the resolved value
}
```

Secret resolution chain:
```
plugin.yaml config → Secret Reference → Secret Backend (Vault/K8s Secrets/Env)
                                              ↓
                                    Resolved Value → Plugin process
```

---

## 9. Multi-Plugin Coordination

### 9.1 Plugin Chains (Composing Multiple Plugins)

```yaml
# workflow: deploy-with-approval.yaml
# Demonstrates multi-plugin coordination in a single workflow
apiVersion: pepa.github.io/v1alpha1
kind: Workflow
metadata:
  name: deploy-with-approval
spec:
  steps:
    - name: get-latest-release
      plugin: git_provider:github
      action: getLatestRelease
      params:
        repository: "{{ entity.repository.external_id }}"
    
    - name: create-deploy-ticket
      plugin: task_tracker:jira
      action: createIssue
      params:
        project: PLATFORM
        summary: "Deploy {{ entity.name }} v{{ steps.get-latest-release.version }}"
        type: TASK
    
    - name: deploy-to-staging
      plugin: cd_engine:argocd
      action: deploy
      params:
        application: "{{ entity.name }}-staging"
        revision: "{{ steps.get-latest-release.sha }}"
      waitFor:
        condition: status == "Healthy"
        timeout: 10m
    
    - name: run-smoke-tests
      plugin: ci_engine:github-actions
      action: triggerWorkflow
      params:
        workflow: smoke-tests.yml
        inputs:
          environment: staging
          version: "{{ steps.get-latest-release.version }}"
      waitFor:
        condition: status == "completed" && conclusion == "success"
    
    - name: request-approval
      type: approval
      params:
        approvers: ["team-lead", "sre-on-call"]
        message: "Approve production deploy of {{ entity.name }}?"
    
    - name: deploy-to-production
      plugin: cd_engine:argocd
      action: deploy
      params:
        application: "{{ entity.name }}-production"
        revision: "{{ steps.get-latest-release.sha }}"
    
    - name: notify-team
      plugin: notification:slack
      action: sendMessage
      params:
        channel: "#deployments"
        message: "✅ {{ entity.name }} v{{ steps.get-latest-release.version }} deployed to production"
```
