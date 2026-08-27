# Flexible Workflow Engine

## 1. Design Philosophy

The Workflow Engine is the **automation backbone** of PEPA. It provides a declarative, DAG-based execution framework for orchestrating multi-step processes across any combination of plugins — from simple CI/CD pipelines to complex multi-environment deployment orchestration with approval gates, rollback strategies, and conditional branching.

### Key Goals

- **Declarative YAML** — all workflows defined as code, stored in Git
- **Visual Designer** — React Flow-based drag-and-drop builder for non-code users
- **Plugin-Agnostic** — any step can invoke any plugin action
- **Resilient** — automatic retries, rollback, timeout handling, dead-letter queues
- **Observable** — full execution traces, step-level logs, duration metrics
- **Composable** — workflows can call other workflows (nested orchestration)

---

## 2. Workflow Specification (YAML)

### 2.1 Core Schema

```yaml
apiVersion: pepa.io/v1alpha1
kind: Workflow
metadata:
  name: deploy-service-pipeline
  labels:
    team: platform
    environment: production
  annotations:
    description: "Full deployment pipeline with testing and approval gates"
    owner: "platform-team@company.com"

spec:
  # Trigger configuration
  triggers:
    - type: webhook
      config:
        secret: deploy-webhook-secret
    - type: schedule
      config:
        cron: "0 2 * * 1-5"  # Weekdays at 2 AM
    - type: entity_event
      config:
        entityType: git_pull_request
        event: merged
        filter:
          "metadata.target_branch": "main"
    - type: manual
      config:
        parameters:
          - name: version
            type: string
            required: true
          - name: skip_tests
            type: boolean
            default: false
  
  # Global settings
  settings:
    timeout: 60m
    concurrency: 1           # Max parallel executions
    onConflict: queue        # queue | reject | replace
    retryPolicy:
      maxRetries: 3
      backoff: exponential
      initialInterval: 10s
      maxInterval: 5m
    
  # Execution context — available to all steps
  context:
    service: "{{ trigger.entity.name }}"
    repository: "{{ trigger.entity.metadata.repository_url }}"
    version: "{{ trigger.params.version | default(trigger.entity.metadata.latest_release) }}"
    deployer: "{{ trigger.user.email }}"
  
  # Step definitions (DAG)
  steps:
    # ── Phase 1: Validation ──────────────────────────────
    - name: validate-manifests
      description: "Validate Kubernetes manifests"
      plugin: cd_engine:argocd
      action: validateManifests
      params:
        path: "k8s/{{.service}}/"
        kubernetesVersion: "1.29"
      timeout: 2m
    
    - name: check-deploy-window
      description: "Verify we're in an allowed deployment window"
      type: condition
      condition: "now().Hour() >= 6 && now().Hour() <= 20 && !isHoliday(now())"
      onFalse:
        action: abort
        message: "Deployments only allowed between 06:00-20:00 on business days"
    
    # ── Phase 2: Testing ─────────────────────────────────
    - name: run-unit-tests
      description: "Execute unit test suite"
      plugin: ci_engine:github-actions
      action: triggerWorkflow
      params:
        workflow: ci-unit-tests.yml
        ref: "{{.version}}"
      waitFor:
        condition: "status == 'completed'"
        timeout: 15m
      retry:
        maxRetries: 2
      # Conditional skip
      skipWhen: "{{.skip_tests}} == true"
    
    - name: run-integration-tests
      description: "Execute integration tests against staging"
      plugin: ci_engine:github-actions
      action: triggerWorkflow
      params:
        workflow: ci-integration.yml
        ref: "{{.version}}"
        inputs:
          target_env: staging
      waitFor:
        condition: "status == 'completed' && conclusion == 'success'"
        timeout: 30m
      depends_on:
        - run-unit-tests
    
    - name: run-security-scan
      description: "Run container security scan"
      plugin: ci_engine:github-actions
      action: triggerWorkflow
      params:
        workflow: security-scan.yml
        ref: "{{.version}}"
      waitFor:
        condition: "status == 'completed'"
        timeout: 10m
      # Runs in parallel with integration tests
      depends_on:
        - run-unit-tests
    
    # ── Phase 3: Staging Deploy ──────────────────────────
    - name: deploy-staging
      description: "Deploy to staging environment"
      plugin: cd_engine:argocd
      action: deploy
      params:
        application: "{{.service}}-staging"
        revision: "{{.version}}"
        autoSync: true
      waitFor:
        condition: "status == 'Healthy' && syncStatus == 'Synced'"
        timeout: 10m
      depends_on:
        - run-integration-tests
        - run-security-scan
        - validate-manifests
    
    - name: staging-smoke-tests
      description: "Run smoke tests against staging"
      plugin: ci_engine:github-actions
      action: triggerWorkflow
      params:
        workflow: smoke-tests.yml
        inputs:
          environment: staging
          version: "{{.version}}"
      waitFor:
        condition: "status == 'completed' && conclusion == 'success'"
        timeout: 10m
      depends_on:
        - deploy-staging
    
    # ── Phase 4: Approval Gate ───────────────────────────
    - name: production-approval
      description: "Request approval for production deployment"
      type: approval
      config:
        approvers:
          - type: user
            value: "sre-on-call@company.com"
          - type: role
            value: "team-lead"
          - type: team
            value: "platform-approvers"
        quorum: 2          # Need 2 approvals
        timeout: 24h
        message: |
          ## Production Deployment Request
          
          **Service:** {{.service}}
          **Version:** {{.version}}
          **Deployer:** {{.deployer}}
          
          ### Test Results
          - Unit Tests: {{.steps.run-unit-tests.status}}
          - Integration: {{.steps.run-integration-tests.status}}
          - Security: {{.steps.run-security-scan.status}}
      depends_on:
        - staging-smoke-tests
    
    # ── Phase 5: Production Deploy ───────────────────────
    - name: deploy-production
      description: "Deploy to production (canary)"
      plugin: cd_engine:argocd
      action: deploy
      params:
        application: "{{.service}}-production"
        revision: "{{.version}}"
        strategy: canary
        canary:
          steps:
            - weight: 10
              pause: {duration: 5m}
            - weight: 25
              pause: {duration: 10m}
            - weight: 50
              pause: {duration: 15m}
            - weight: 100
      depends_on:
        - production-approval
      # Rollback on failure
      rollback:
        automatic: true
        condition: "status == 'Degraded' || errorRate > 0.05"
        action:
          plugin: cd_engine:argocd
          action: rollback
          params:
            application: "{{.service}}-production"
    
    # ── Phase 6: Post-Deploy ─────────────────────────────
    - name: production-verification
      description: "Run production verification tests"
      plugin: monitoring:prometheus
      action: queryMetrics
      params:
        query: "rate(http_requests_total{service='{{.service}}',code=~'5..'}[5m])"
        threshold: 0.01
        comparison: "less_than"
      waitFor:
        duration: 15m      # Monitor for 15 minutes
        interval: 1m        # Check every minute
      depends_on:
        - deploy-production
    
    - name: notify-team
      description: "Notify team of deployment result"
      plugin: notification:slack
      action: sendMessage
      params:
        channel: "#deployments"
        template: "deployment-summary"
        data:
          service: "{{.service}}"
          version: "{{.version}}"
          deployer: "{{.deployer}}"
          duration: "{{.workflow.duration}}"
          status: "{{.workflow.status}}"
      depends_on:
        - production-verification
      # Runs even if previous steps fail
      runWhen: always
    
    - name: update-entity-status
      description: "Update service entity with new version"
      type: entity_update
      params:
        entityType: service
        name: "{{.service}}"
        updates:
          metadata.current_version: "{{.version}}"
          metadata.last_deployed: "{{now()}}"
          metadata.deployed_by: "{{.deployer}}"
      depends_on:
        - production-verification
```

### 2.2 Workflow DSL Reference

```
Expression Language: CEL (Common Expression Language)
─────────────────────────────────────────────────────
Available in conditions, params, and waitFor blocks:

  Context Variables:
    .trigger          — Trigger payload (entity, params, user)
    .steps.{name}     — Output of a specific step
    .workflow         — Workflow metadata (id, name, startedAt)
    .entity           — Shorthand for trigger.entity
    .env              — Environment variables
    
  Functions:
    now()             — Current timestamp
    isHoliday(t)      — Check if date is a holiday
    duration(d)       — Parse duration string
    jsonpath(p, obj)  — JSONPath evaluation
    regex(p, s)       — Regex match
    semver(v)         — Semantic version parsing
    
  Operators:
    ==, !=, >, <, >=, <=    — Comparison
    &&, ||, !               — Logical
    | (pipe)                — Template piping
    ? :                     — Ternary
```

---

## 3. Workflow Execution Engine

### 3.1 Architecture

```
┌────────────────────────────────────────────────────────────────────┐
│                    WORKFLOW ENGINE                                   │
│                                                                     │
│  ┌───────────────┐    ┌───────────────┐    ┌───────────────────┐  │
│  │ Trigger       │    │ Workflow      │    │ DAG               │  │
│  │ Dispatcher    │───▶│ Compiler      │───▶│ Scheduler         │  │
│  │               │    │               │    │                   │  │
│  │ - Webhook     │    │ - Parse YAML  │    │ - Topological     │  │
│  │ - Schedule    │    │ - Validate    │    │   sort            │  │
│  │ - Event       │    │ - Resolve     │    │ - Parallel        │  │
│  │ - Manual      │    │   templates   │    │   execution       │  │
│  │ - API         │    │ - Build DAG   │    │ - Dependency      │  │
│  └───────────────┘    └───────────────┘    │   resolution      │  │
│                                             └────────┬──────────┘  │
│                                                      │              │
│  ┌───────────────────────────────────────────────────┴───────────┐ │
│  │                    Step Executor Pool                          │ │
│  │                                                                │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐     │ │
│  │  │ Plugin   │  │ Plugin   │  │ Plugin   │  │ Internal │     │ │
│  │  │ Action   │  │ Action   │  │ Action   │  │ Step     │     │ │
│  │  │ Executor │  │ Executor │  │ Executor │  │ Executor │     │ │
│  │  │          │  │          │  │          │  │          │     │ │
│  │  │ argocd:  │  │ github:  │  │ slack:   │  │ cond,    │     │ │
│  │  │ deploy   │  │ trigger  │  │ sendMsg  │  │ approval │     │ │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘     │ │
│  └────────────────────────────────────────────────────────────────┘ │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Execution State Store (Redis + PostgreSQL)                    │  │
│  │                                                               │  │
│  │  Redis:  Real-time step state, progress, WebSocket fan-out   │  │
│  │  PG:     Execution history, audit trail, step outputs        │  │
│  └──────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────┘
```

### 3.2 Execution State Machine

```
                    ┌──────────┐
                    │ Pending  │  (workflow triggered, queued)
                    └────┬─────┘
                         │ scheduler picks up
                         ▼
                    ┌──────────┐
                    │ Running  │
                    └────┬─────┘
                         │
            ┌────────────┼────────────┐
            │            │            │
            ▼            ▼            ▼
     ┌──────────┐ ┌──────────┐ ┌──────────┐
     │ Waiting  │ │ Failed   │ │Cancelled │
     │ (approval│ │          │ │          │
     │  /timer) │ └────┬─────┘ └──────────┘
     └────┬─────┘      │
          │ resolved     │ retry
          ▼              ▼
     ┌──────────┐ ┌──────────┐
     │ Running  │ │ Retrying │
     └────┬─────┘ └──────────┘
          │
    ┌─────┴─────┐
    │           │
    ▼           ▼
┌────────┐ ┌────────┐
│Success │ │Failed  │
└────────┘ └───┬────┘
               │ rollback
               ▼
          ┌──────────┐
          │Rolling   │
          │Back      │
          └────┬─────┘
               │
               ▼
          ┌──────────┐
          │Rolled    │
          │Back      │
          └──────────┘
```

### 3.3 Step Execution (Go)

```go
package workflow

// StepExecutor handles individual step execution
type StepExecutor interface {
    Execute(ctx context.Context, step *Step, execCtx *ExecutionContext) (*StepResult, error)
    Rollback(ctx context.Context, step *Step, execCtx *ExecutionContext) error
}

// PluginActionExecutor — executes actions via plugins
type PluginActionExecutor struct {
    pluginManager *PluginManager
}

func (e *PluginActionExecutor) Execute(ctx context.Context, step *Step, execCtx *ExecutionContext) (*StepResult, error) {
    // Parse plugin:action format
    pluginName, actionName := parsePluginAction(step.Plugin)
    
    // Get plugin client
    plugin, err := e.pluginManager.GetPlugin(ctx, pluginName, execCtx.TenantID)
    if err != nil {
        return nil, fmt.Errorf("plugin %s not available: %w", pluginName, err)
    }
    
    // Resolve template parameters
    resolvedParams, err := resolveTemplates(step.Params, execCtx)
    if err != nil {
        return nil, fmt.Errorf("parameter resolution failed: %w", err)
    }
    
    // Execute the action
    result, err := plugin.ExecuteAction(ctx, &ActionRequest{
        Action: actionName,
        Params: resolvedParams,
    })
    if err != nil {
        return nil, err
    }
    
    // Handle waitFor (polling for completion)
    if step.WaitFor != nil {
        result, err = e.waitForCondition(ctx, plugin, result, step.WaitFor)
        if err != nil {
            return nil, fmt.Errorf("waitFor condition not met: %w", err)
        }
    }
    
    return result, nil
}

// DAG Scheduler — manages parallel execution of steps
type DAGScheduler struct {
    graph    *DAG
    executor *StepExecutorPool
    state    *ExecutionState
}

func (s *DAGScheduler) Run(ctx context.Context) error {
    // Topological sort to determine execution order
    order := s.graph.TopologicalSort()
    
    // Group steps into parallel waves
    waves := s.groupIntoWaves(order)
    
    for _, wave := range waves {
        // Execute all steps in a wave concurrently
        g, gCtx := errgroup.WithContext(ctx)
        
        for _, step := range wave {
            step := step
            g.Go(func() error {
                // Check skip condition
                if shouldSkip, _ := evaluateCondition(step.SkipWhen, s.state); shouldSkip {
                    s.state.MarkSkipped(step.Name)
                    return nil
                }
                
                // Execute step
                s.state.MarkRunning(step.Name)
                result, err := s.executor.Execute(gCtx, step, s.state.Context())
                if err != nil {
                    s.state.MarkFailed(step.Name, err)
                    return err
                }
                
                s.state.MarkCompleted(step.Name, result)
                return nil
            })
        }
        
        if err := g.Wait(); err != nil {
            // Handle failure — check for rollback steps
            return s.handleFailure(ctx, err)
        }
    }
    
    return nil
}
```

---

## 4. Visual Workflow Designer (React Flow)

### 4.1 Frontend Architecture

```
┌────────────────────────────────────────────────────────────────┐
│              WORKFLOW DESIGNER (React Flow)                     │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                    Canvas Area                            │  │
│  │                                                           │  │
│  │   ┌─────────┐     ┌─────────┐     ┌─────────┐          │  │
│  │   │ Trigger  │────▶│ Step A   │────▶│ Step B   │          │  │
│  │   │ (webhook)│     │ (deploy) │     │ (test)   │          │  │
│  │   └─────────┘     └─────────┘     └────┬────┘          │  │
│  │                                         │                │  │
│  │                    ┌────────────────────┤                │  │
│  │                    ▼                    ▼                │  │
│  │              ┌─────────┐         ┌─────────┐            │  │
│  │              │ Step C   │         │ Step D   │            │  │
│  │              │ (approve)│         │ (notify) │            │  │
│  │              └─────────┘         └─────────┘            │  │
│  │                                                           │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────┐  ┌──────────────────────────────────────┐   │
│  │ Step Palette │  │ Properties Panel                      │   │
│  │              │  │                                       │   │
│  │ 📦 Triggers  │  │ Step: deploy-staging                  │   │
│  │ 🔧 Actions   │  │ Plugin: cd_engine:argocd              │   │
│  │ ⚡ Conditions│  │ Action: deploy                        │   │
│  │ 👤 Approval  │  │ ─────────────────────────────────     │   │
│  │ 📢 Notify    │  │ Parameters:                           │   │
│  │ 🔄 Loop      │  │   application: [input field]          │   │
│  │              │  │   revision: [input field]             │   │
│  │              │  │   strategy: [dropdown: canary/rolling]│   │
│  │              │  │ ─────────────────────────────────     │   │
│  │              │  │ Wait For:                             │   │
│  │              │  │   condition: [CEL expression input]   │   │
│  │              │  │   timeout: [duration input]           │   │
│  │              │  │ ─────────────────────────────────     │   │
│  │              │  │ Retry: max=3, backoff=exponential     │   │
│  └──────────────┘  └──────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────┘
```

### 4.2 Custom Node Types

```typescript
// React Flow custom node definitions

// Base node interface
interface WorkflowNode {
  id: string;
  type: 'trigger' | 'action' | 'condition' | 'approval' | 'notification' | 'loop' | 'subworkflow';
  data: {
    name: string;
    description?: string;
    plugin?: string;      // e.g., "cd_engine:argocd"
    action?: string;      // e.g., "deploy"
    params?: Record<string, unknown>;
    dependsOn?: string[];
    condition?: string;   // CEL expression
    skipWhen?: string;
    retry?: RetryConfig;
    timeout?: string;
  };
}

// Custom React Flow node component
const ActionNode: React.FC<NodeProps<WorkflowNode>> = ({ data, selected }) => {
  const statusIcon = {
    pending: '⏳',
    running: '🔄',
    success: '✅',
    failed: '❌',
    skipped: '⏭️',
  };
  
  return (
    <div className={`workflow-node action-node ${selected ? 'selected' : ''}`}>
      <div className="node-header">
        <span className="node-icon">{getPluginIcon(data.plugin)}</span>
        <span className="node-name">{data.name}</span>
      </div>
      <div className="node-body">
        <div className="node-plugin">{data.plugin}</div>
        <div className="node-action">{data.action}</div>
      </div>
      <div className="node-footer">
        <span className="node-status">{statusIcon[data.status]}</span>
        {data.timeout && <span className="node-timeout">⏱ {data.timeout}</span>}
      </div>
    </div>
  );
};
```

### 4.3 YAML ↔ Visual Sync

```typescript
// Bidirectional sync between YAML and visual representation

class WorkflowSyncEngine {
  // YAML → React Flow nodes/edges
  yamlToFlow(yaml: WorkflowSpec): FlowData {
    const nodes: WorkflowNode[] = [];
    const edges: Edge[] = [];
    
    // Create trigger nodes
    yaml.spec.triggers.forEach((trigger, i) => {
      nodes.push({
        id: `trigger-${i}`,
        type: 'trigger',
        data: { name: `Trigger (${trigger.type})`, ...trigger },
        position: { x: 0, y: i * 120 },
      });
    });
    
    // Create step nodes with auto-layout
    const layout = this.dagLayout(yaml.spec.steps);
    layout.nodes.forEach(node => nodes.push(node));
    layout.edges.forEach(edge => edges.push(edge));
    
    return { nodes, edges };
  }
  
  // React Flow nodes/edges → YAML
  flowToYaml(flow: FlowData): WorkflowSpec {
    const steps: StepSpec[] = [];
    
    flow.nodes
      .filter(n => n.type !== 'trigger')
      .forEach(node => {
        const incomingEdges = flow.edges.filter(e => e.target === node.id);
        const dependsOn = incomingEdges.map(e => {
          const sourceNode = flow.nodes.find(n => n.id === e.source);
          return sourceNode?.data.name;
        });
        
        steps.push({
          name: node.data.name,
          plugin: node.data.plugin,
          action: node.data.action,
          params: node.data.params,
          depends_on: dependsOn,
          condition: node.data.condition,
          retry: node.data.retry,
          timeout: node.data.timeout,
        });
      });
    
    return { steps };
  }
  
  // Auto-layout using dagre algorithm
  private dagLayout(steps: StepSpec[]): LayoutResult {
    const g = new dagre.graphlib.Graph();
    g.setGraph({ rankdir: 'LR', nodesep: 50, ranksep: 100 });
    g.setDefaultEdgeLabel(() => ({}));
    
    steps.forEach(step => {
      g.setNode(step.name, { width: 200, height: 80 });
    });
    
    steps.forEach(step => {
      (step.depends_on || []).forEach(dep => {
        g.setEdge(dep, step.name);
      });
    });
    
    dagre.layout(g);
    
    // Convert dagre positions to React Flow positions
    // ...
  }
}
```

---

## 5. Workflow Templates & Reusability

### 5.1 Template System

```yaml
# Reusable workflow template
apiVersion: pepa.io/v1alpha1
kind: WorkflowTemplate
metadata:
  name: standard-deploy
  labels:
    category: deployment
    maturity: stable
spec:
  # Template parameters (caller must provide)
  parameters:
    - name: service_name
      type: string
      required: true
    - name: version
      type: string
      required: true
    - name: environment
      type: string
      default: staging
      enum: [dev, staging, production]
    - name: deploy_strategy
      type: string
      default: rolling
      enum: [rolling, canary, blue_green]
    - name: run_tests
      type: boolean
      default: true
  
  steps:
    - name: deploy
      plugin: "cd_engine:{{.cd_plugin}}"
      action: deploy
      params:
        application: "{{.service_name}}-{{.environment}}"
        revision: "{{.version}}"
        strategy: "{{.deploy_strategy}}"
    
    - name: verify
      plugin: "monitoring:{{.monitoring_plugin}}"
      action: queryMetrics
      params:
        query: "up{service='{{.service_name}}'}"
      waitFor:
        duration: 5m
      depends_on: [deploy]
```

### 5.2 Using Templates

```yaml
# Concrete workflow that uses a template
apiVersion: pepa.io/v1alpha1
kind: Workflow
metadata:
  name: deploy-payment-service
spec:
  template:
    name: standard-deploy
    parameters:
      service_name: payment-service
      version: "v2.3.1"
      environment: production
      deploy_strategy: canary
      run_tests: true
  
  # Additional steps beyond the template
  extensions:
    after:
      - name: notify-payment-team
        plugin: notification:slack
        action: sendMessage
        params:
          channel: "#payment-team"
          message: "Payment service v2.3.1 deployed to production"
```

---

## 6. Workflow Execution Storage

### 6.1 PostgreSQL Schema

```sql
-- Workflow definitions
CREATE TABLE workflows (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(256) NOT NULL,
    tenant_id   UUID NOT NULL,
    
    -- Definition
    spec        JSONB NOT NULL,       -- Full workflow YAML as JSON
    version     INTEGER DEFAULT 1,
    
    -- Source
    source      VARCHAR(32) DEFAULT 'yaml',  -- yaml, visual, template
    git_path    VARCHAR(512),                -- Path in Git repo (if gitops)
    
    -- Status
    is_enabled  BOOLEAN DEFAULT TRUE,
    is_locked   BOOLEAN DEFAULT FALSE,       -- Locked during editing
    
    created_by  UUID,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    
    UNIQUE (name, tenant_id)
);

-- Workflow executions
CREATE TABLE workflow_executions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID NOT NULL REFERENCES workflows(id),
    tenant_id   UUID NOT NULL,
    
    -- Trigger info
    trigger_type VARCHAR(32),     -- webhook, schedule, manual, entity_event
    trigger_payload JSONB,
    triggered_by UUID,            -- User ID (for manual triggers)
    
    -- Execution state
    status      VARCHAR(32) DEFAULT 'pending',
    started_at  TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration_ms INTEGER,
    
    -- Context (resolved parameters and variables)
    context     JSONB DEFAULT '{}',
    
    -- Result
    result      JSONB,            -- Final output
    error       TEXT,
    
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Step executions
CREATE TABLE step_executions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id    UUID NOT NULL REFERENCES workflow_executions(id) ON DELETE CASCADE,
    
    -- Step identity
    step_name       VARCHAR(256) NOT NULL,
    step_type       VARCHAR(32),  -- action, condition, approval, loop
    
    -- Plugin action
    plugin_name     VARCHAR(128),
    action_name     VARCHAR(128),
    params          JSONB,
    
    -- Execution
    status          VARCHAR(32) DEFAULT 'pending',
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    duration_ms     INTEGER,
    
    -- Result
    output          JSONB,
    error           TEXT,
    retry_count     INTEGER DEFAULT 0,
    
    -- Approval-specific
    approvers       JSONB,        -- For approval steps
    approvals       JSONB,        -- Approval responses
    
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_exec_workflow ON workflow_executions(workflow_id, created_at DESC);
CREATE INDEX idx_exec_status ON workflow_executions(tenant_id, status);
CREATE INDEX idx_step_exec ON step_executions(execution_id, step_name);
```

---

## 7. Error Handling & Resilience

### 7.1 Retry Strategies

```yaml
# Retry configuration options
retryPolicy:
  # Simple retry
  maxRetries: 3
  backoff: constant        # constant | exponential | linear
  initialInterval: 5s
  
  # Exponential backoff with jitter
  maxRetries: 5
  backoff: exponential
  initialInterval: 1s
  maxInterval: 2m
  multiplier: 2.0
  jitterFactor: 0.1        # ±10% randomization
  
  # Conditional retry
  maxRetries: 3
  retryWhen: "error.code in [502, 503, 504] || error.type == 'TimeoutError'"
  retryOn: ["timeout", "connection_error", "rate_limited"]
```

### 7.2 Rollback Strategies

```yaml
rollback:
  # Automatic rollback on failure
  automatic: true
  condition: "step.status == 'failed' || metrics.errorRate > 0.05"
  
  # Rollback actions
  actions:
    - name: rollback-deployment
      plugin: cd_engine:argocd
      action: rollback
      params:
        application: "{{.service}}-production"
        toRevision: "{{.previousVersion}}"
    
    - name: notify-rollback
      plugin: notification:slack
      action: sendMessage
      params:
        channel: "#incidents"
        message: "🚨 Rollback triggered for {{.service}} from {{.version}}"
  
  # Data cleanup
  cleanup:
    - entity: k8s_deployment
      action: revert
      filter: "metadata.version == '{{.version}}'"
```

### 7.3 Dead Letter Queue

Failed workflow executions are sent to a DLQ for analysis and replay:

```sql
CREATE TABLE workflow_dlq (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id    UUID REFERENCES workflow_executions(id),
    tenant_id       UUID NOT NULL,
    
    -- Failure details
    error_type      VARCHAR(64),
    error_message   TEXT,
    failed_step     VARCHAR(256),
    stack_trace     TEXT,
    
    -- Replay
    replay_count    INTEGER DEFAULT 0,
    max_replays     INTEGER DEFAULT 10,
    next_retry_at   TIMESTAMPTZ,
    
    -- Resolution
    resolved_at     TIMESTAMPTZ,
    resolved_by     UUID,
    resolution      TEXT,
    
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
```
