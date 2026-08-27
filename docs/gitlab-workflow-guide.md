# PEPA GitLab Deployment Workflow Guide

## Ваш текущий флоу

### Структура GitLab
```
Group: charts/
├── Project: nginx/
│   ├── Branch: dev
│   ├── Branch: testing
│   ├── Branch: staging
│   └── Branch: main
├── Project: postgresql/
│   ├── Branch: dev
│   ├── Branch: testing
│   └── ...
└── Project: redis/
    └── ...
```

### Процесс разработки
```
1. Создать задачу (Jira/GitLab Issue)
   └─> Создать ветку: feature/PEPA-123-add-auth
   
2. Разработка в ветке
   └─> Commits, tests, etc.
   
3. Создать Merge Request в dev
   └─> Code review
   └─> Approve
```

### Процесс деплоя

#### Stage 1: Dev Environment
```
Merge в dev
    ↓
Автоматический CI (если есть)
    ↓
РУЧНОЙ деплой как Helm release
    └─> helm upgrade --install nginx ./charts/nginx \
            --namespace dev \
            --set image.tag=dev
    ↓
Проверка работоспособности на dev кластере
    └─> kubectl get pods -n dev
    └─> kubectl logs -n dev -l app=nginx
    └─> Test endpoints
    ↓
Если все ОК → переход к testing
```

#### Stage 2: Testing Environment
```
Merge в testing
    ↓
Автоматический CI
    └─> Build Helm tar release
    └─> Push to FluxCD group/project
    ↓
PEPA отслеживает:
    └─> Helm chart создан ✓
    └─> Push в FluxCD repo ✓
    └─> FluxCD detected changes ✓
    └─> FluxCD deployed to cluster ✓
    ↓
Verification
    └─> kubectl get pods -n testing
    └─> Helm list -n testing
    └─> Check FluxCD logs
    ↓
Если все ОК → переход к staging
```

#### Stage 3: Staging Environment
```
Merge в staging
    ↓
Автоматический CI
    └─> Build Helm tar release
    └─> Push to FluxCD group/project
    ↓
PEPA отслеживает весь процесс
    ↓
Verification на staging кластере
    ↓
Если все ОК → переход к main
```

#### Stage 4: Production (main)
```
Merge в main
    ↓
Автоматический CI
    └─> Build Helm tar release
    └─> Push to FluxCD group/project
    ↓
PEPA отслеживает весь процесс
    ↓
Verification на production кластере
    ↓
Monitor metrics and logs
```

---

## PEPA Implementation

### 1. GitLab Integration

#### API Endpoints
```go
// internal/api/rest/gitlab_workflow_handlers.go

// Get workflow configuration
GET /api/v1/workflow/config
    - Returns deployment stages
    - Returns branch mapping
    - Returns cluster mapping

// Get merge requests
GET /api/v1/workflow/mrs
    - List all MRs across projects
    - Filter by branch, status, project
    - Show deployment status

// Get MR details
GET /api/v1/workflow/mrs/:id
    - MR details
    - Deployment history
    - Verification status

// Deploy to environment
POST /api/v1/workflow/deploy
    - Manual deploy for dev
    - Trigger Helm release
    - Specify cluster

// Track deployment
GET /api/v1/workflow/deployments/:id/status
    - Helm release status
    - FluxCD sync status
    - Pod status
    - Verification results

// Verify deployment
POST /api/v1/workflow/verify
    - Run verification checks
    - Health checks
    - Smoke tests
```

#### Backend Implementation
```go
// internal/service/gitlab_workflow.go

type GitLabWorkflowService struct {
    gitlab    *gitlab.Client
    k8s       *kubernetes.Clientset
    fluxcd    *FluxCDClient
    helm      *HelmClient
}

// Get deployment workflow for project
func (s *GitLabWorkflowService) GetWorkflow(projectID string) (*Workflow, error) {
    // Get project branches
    branches, err := s.gitlab.ListBranches(projectID)
    
    // Map branches to environments
    workflow := &Workflow{
        Stages: []Stage{
            {
                Name: "dev",
                Branch: "dev",
                Cluster: "dev-cluster",
                DeployType: "manual", // manual Helm release
            },
            {
                Name: "testing",
                Branch: "testing",
                Cluster: "testing-cluster",
                DeployType: "automatic", // FluxCD
            },
            {
                Name: "staging",
                Branch: "staging",
                Cluster: "staging-cluster",
                DeployType: "automatic",
            },
            {
                Name: "production",
                Branch: "main",
                Cluster: "prod-cluster",
                DeployType: "automatic",
            },
        },
    }
    
    return workflow, nil
}

// Deploy to dev environment (manual)
func (s *GitLabWorkflowService) DeployToDev(ctx context.Context, mrID string, cluster string) error {
    // Get MR details
    mr, err := s.gitlab.GetMergeRequest(mrID)
    
    // Get chart from branch
    chartPath := fmt.Sprintf("charts/%s", mr.SourceBranch)
    
    // Deploy using Helm
    err = s.helm.Upgrade(ctx, helm.UpgradeOptions{
        ReleaseName: mr.Project.Name,
        ChartPath:   chartPath,
        Namespace:   "dev",
        Values: map[string]interface{}{
            "image.tag": mr.SourceBranch,
        },
    })
    
    // Track deployment
    deployment := &Deployment{
        MR_ID:       mrID,
        Environment: "dev",
        Cluster:     cluster,
        Status:      "deploying",
        HelmRelease: mr.Project.Name,
    }
    s.repo.CreateDeployment(ctx, deployment)
    
    return nil
}

// Track FluxCD deployment
func (s *GitLabWorkflowService) TrackFluxCDDeployment(ctx context.Context, deploymentID string) (*DeploymentStatus, error) {
    deployment, err := s.repo.GetDeployment(ctx, deploymentID)
    
    // Check FluxCD sync status
    fluxStatus, err := s.fluxcd.GetSyncStatus(ctx, deployment.HelmRelease)
    
    // Check pod status
    pods, err := s.k8s.CoreV1().Pods(deployment.Namespace).List(ctx, metav1.ListOptions{
        LabelSelector: fmt.Sprintf("app=%s", deployment.HelmRelease),
    })
    
    status := &DeploymentStatus{
        FluxSynced: fluxStatus.Synced,
        PodsReady:  countReadyPods(pods),
        PodsTotal:  len(pods.Items),
        Health:     checkHealth(pods),
    }
    
    return status, nil
}

// Verify deployment
func (s *GitLabWorkflowService) VerifyDeployment(ctx context.Context, deploymentID string) (*VerificationResult, error) {
    deployment, err := s.repo.GetDeployment(ctx, deploymentID)
    
    // Run health checks
    healthChecks := []HealthCheck{
        s.checkPodsRunning,
        s.checkServicesAvailable,
        s.checkIngressAccessible,
        s.runSmokeTests,
    }
    
    results := make([]CheckResult, 0)
    for _, check := range healthChecks {
        result := check(ctx, deployment)
        results = append(results, result)
    }
    
    return &VerificationResult{
        Passed:  allPassed(results),
        Checks:  results,
        Message: generateMessage(results),
    }, nil
}
```

---

### 2. Frontend - Workflow Dashboard

#### Main Workflow Page
```typescript
// frontend/app/workflow/page.tsx

export default function WorkflowDashboard() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [selectedProject, setSelectedProject] = useState<string | null>(null);
  const [mergeRequests, setMergeRequests] = useState<MergeRequest[]>([]);
  
  return (
    <div className="workflow-dashboard">
      <div className="header">
        <h1>Deployment Workflow</h1>
        <ProjectSelector
          projects={projects}
          selected={selectedProject}
          onSelect={setSelectedProject}
        />
      </div>
      
      <div className="workflow-pipeline">
        <StageColumn
          stage="dev"
          title="Development"
          mergeRequests={mergeRequests.filter(mr => mr.targetBranch === 'dev')}
          onDeploy={handleDeploy}
          onVerify={handleVerify}
        />
        
        <StageColumn
          stage="testing"
          title="Testing"
          mergeRequests={mergeRequests.filter(mr => mr.targetBranch === 'testing')}
          autoDeploy={true}
        />
        
        <StageColumn
          stage="staging"
          title="Staging"
          mergeRequests={mergeRequests.filter(mr => mr.targetBranch === 'staging')}
          autoDeploy={true}
        />
        
        <StageColumn
          stage="production"
          title="Production"
          mergeRequests={mergeRequests.filter(mr => mr.targetBranch === 'main')}
          autoDeploy={true}
        />
      </div>
    </div>
  );
}
```

#### Stage Column Component
```typescript
// frontend/components/StageColumn.tsx

interface StageColumnProps {
  stage: string;
  title: string;
  mergeRequests: MergeRequest[];
  autoDeploy?: boolean;
  onDeploy?: (mr: MergeRequest) => void;
  onVerify?: (mr: MergeRequest) => void;
}

export function StageColumn({ stage, title, mergeRequests, autoDeploy, onDeploy, onVerify }: StageColumnProps) {
  return (
    <div className={`stage-column stage-${stage}`}>
      <div className="stage-header">
        <h2>{title}</h2>
        <span className="stage-badge">
          {autoDeploy ? 'Auto' : 'Manual'}
        </span>
      </div>
      
      <div className="merge-requests">
        {mergeRequests.map(mr => (
          <MergeRequestCard
            key={mr.id}
            mr={mr}
            stage={stage}
            onDeploy={onDeploy}
            onVerify={onVerify}
          />
        ))}
      </div>
    </div>
  );
}
```

#### Merge Request Card
```typescript
// frontend/components/MergeRequestCard.tsx

interface MergeRequestCardProps {
  mr: MergeRequest;
  stage: string;
  onDeploy?: (mr: MergeRequest) => void;
  onVerify?: (mr: MergeRequest) => void;
}

export function MergeRequestCard({ mr, stage, onDeploy, onVerify }: MergeRequestCardProps) {
  const [deploymentStatus, setDeploymentStatus] = useState<DeploymentStatus | null>(null);
  
  useEffect(() => {
    // Poll deployment status
    const interval = setInterval(async () => {
      const status = await workflowAPI.getDeploymentStatus(mr.deploymentId);
      setDeploymentStatus(status);
    }, 5000);
    
    return () => clearInterval(interval);
  }, [mr.deploymentId]);
  
  return (
    <div className="mr-card">
      <div className="mr-header">
        <a href={mr.webUrl} target="_blank" rel="noopener noreferrer">
          <h3>{mr.title}</h3>
        </a>
        <span className={`mr-status status-${mr.status}`}>
          {mr.status}
        </span>
      </div>
      
      <div className="mr-info">
        <span>#{mr.iid}</span>
        <span>{mr.author.name}</span>
        <span>{mr.sourceBranch} → {mr.targetBranch}</span>
      </div>
      
      {deploymentStatus && (
        <div className="deployment-status">
          <div className="status-indicator">
            <span className={`dot ${deploymentStatus.healthy ? 'green' : 'red'}`} />
            <span>{deploymentStatus.message}</span>
          </div>
          
          <div className="deployment-details">
            <span>Helm: {deploymentStatus.helmRelease}</span>
            <span>Pods: {deploymentStatus.podsReady}/{deploymentStatus.podsTotal}</span>
            <span>FluxCD: {deploymentStatus.fluxSynced ? '✓' : '✗'}</span>
          </div>
        </div>
      )}
      
      <div className="mr-actions">
        {stage === 'dev' && (
          <button onClick={() => onDeploy?.(mr)} className="btn-primary">
            Deploy to Dev
          </button>
        )}
        
        <button onClick={() => onVerify?.(mr)} className="btn-secondary">
          Verify
        </button>
        
        <a href={mr.webUrl} target="_blank" rel="noopener noreferrer" className="btn-link">
          View in GitLab
        </a>
      </div>
    </div>
  );
}
```

---

### 3. Deployment Tracking

#### Real-time Updates
```typescript
// frontend/hooks/useDeploymentTracking.ts

export function useDeploymentTracking(deploymentId: string) {
  const [status, setStatus] = useState<DeploymentStatus | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  
  useEffect(() => {
    // WebSocket for real-time updates
    const ws = new WebSocket(`ws://localhost:8080/ws/deployments/${deploymentId}`);
    
    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      
      if (data.type === 'status') {
        setStatus(data.status);
      } else if (data.type === 'log') {
        setLogs(prev => [...prev, data.log]);
      }
    };
    
    return () => ws.close();
  }, [deploymentId]);
  
  return { status, logs };
}
```

#### Deployment Timeline
```typescript
// frontend/components/DeploymentTimeline.tsx

export function DeploymentTimeline({ deploymentId }: { deploymentId: string }) {
  const { status, logs } = useDeploymentTracking(deploymentId);
  
  const events = [
    { time: status.startedAt, event: 'Deployment started', status: 'info' },
    { time: status.helmDeployedAt, event: 'Helm release created', status: 'success' },
    { time: status.fluxSyncedAt, event: 'FluxCD synced', status: 'success' },
    { time: status.podsReadyAt, event: 'All pods ready', status: 'success' },
    { time: status.verifiedAt, event: 'Verification passed', status: 'success' },
  ];
  
  return (
    <div className="deployment-timeline">
      <h3>Deployment Timeline</h3>
      
      <div className="timeline">
        {events.map((event, index) => (
          <div key={index} className={`timeline-event ${event.status}`}>
            <div className="event-time">
              {new Date(event.time).toLocaleTimeString()}
            </div>
            <div className="event-dot" />
            <div className="event-content">
              {event.event}
            </div>
          </div>
        ))}
      </div>
      
      <div className="deployment-logs">
        <h4>Logs</h4>
        <pre className="log-output">
          {logs.map((log, index) => (
            <div key={index}>{log}</div>
          ))}
        </pre>
      </div>
    </div>
  );
}
```

---

### 4. Customization

#### Workflow Configuration
```yaml
# config/workflow.yaml
workflow:
  stages:
    - name: dev
      branch: dev
      cluster: dev-cluster
      deploy:
        type: manual
        method: helm
        namespace: dev
      verify:
        - pods_running
        - service_available
        - smoke_tests
    
    - name: testing
      branch: testing
      cluster: testing-cluster
      deploy:
        type: automatic
        method: fluxcd
        helm_chart_repo: fluxcd/charts
        helm_chart_path: testing/
      verify:
        - pods_running
        - fluxcd_synced
        - integration_tests
    
    - name: staging
      branch: staging
      cluster: staging-cluster
      deploy:
        type: automatic
        method: fluxcd
        helm_chart_repo: fluxcd/charts
        helm_chart_path: staging/
      verify:
        - pods_running
        - fluxcd_synced
        - e2e_tests
    
    - name: production
      branch: main
      cluster: prod-cluster
      deploy:
        type: automatic
        method: fluxcd
        helm_chart_repo: fluxcd/charts
        helm_chart_path: production/
      verify:
        - pods_running
        - fluxcd_synced
        - smoke_tests
        - monitoring_healthy
```

#### Project-specific Configuration
```yaml
# projects/nginx/workflow.yaml
project:
  name: nginx
  gitlab_project_id: 123
  
  # Override default workflow
  stages:
    dev:
      namespace: nginx-dev
      values:
        replicaCount: 1
        resources:
          limits:
            cpu: 100m
            memory: 128Mi
    
    testing:
      namespace: nginx-testing
      values:
        replicaCount: 2
    
    production:
      namespace: nginx
      values:
        replicaCount: 3
        resources:
          limits:
            cpu: 500m
            memory: 512Mi
```

---

### 5. Integration Points

#### GitLab Webhooks
```go
// internal/webhook/gitlab.go

func (h *GitLabWebhookHandler) HandleMergeRequest(event *gitlab.MergeRequestEvent) {
    mr := event.MergeRequest
    
    // Determine target environment
    env := branchToEnvironment(mr.TargetBranch)
    
    // Create deployment record
    deployment := &Deployment{
        MR_ID:       mr.ID,
        Project:     mr.ProjectID,
        Environment: env,
        Status:      "pending",
    }
    h.repo.CreateDeployment(context.Background(), deployment)
    
    // Trigger deployment based on environment
    switch env {
    case "dev":
        // Wait for manual trigger
        h.notifyDevReady(deployment)
    case "testing", "staging", "production":
        // Automatic deployment via FluxCD
        h.triggerFluxCDDeployment(deployment)
    }
}
```

#### FluxCD Integration
```go
// internal/service/fluxcd.go

func (s *FluxCDService) TrackDeployment(ctx context.Context, releaseName, namespace string) (*FluxStatus, error) {
    // Get HelmRelease CRD
    helmRelease, err := s.fluxClient.HelmV2().HelmReleases(namespace).Get(ctx, releaseName, metav1.GetOptions{})
    
    status := &FluxStatus{
        Ready:        helmRelease.Status.Conditions[0].Status == "True",
        LastApplied:  helmRelease.Status.LastAppliedRevision,
        LastAttempted: helmRelease.Status.LastAttemptedRevision,
        Failures:     helmRelease.Status.FailureCount,
    }
    
    return status, nil
}
```

---

## Use Cases

### Use Case 1: Developer creates feature
```
1. Developer creates branch: feature/PEPA-123
2. Develops and tests locally
3. Creates MR to dev
4. Code review and approve
5. Merges to dev

PEPA:
- Detects merge
- Shows MR in Dev column
- Developer clicks "Deploy to Dev"
- PEPA deploys via Helm
- Shows deployment status
- Developer verifies on cluster
- If OK, creates MR to testing
```

### Use Case 2: MR merged to testing
```
1. MR merged to testing
2. CI builds Helm chart
3. CI pushes to FluxCD repo

PEPA:
- Detects merge
- Tracks CI pipeline
- Monitors FluxCD repo for new chart
- Detects FluxCD sync
- Shows deployment status
- Runs verification checks
- If OK, creates MR to staging
```

### Use Case 3: Production deployment
```
1. MR merged to main
2. CI builds Helm chart
3. CI pushes to FluxCD repo

PEPA:
- Detects merge
- Tracks CI pipeline
- Monitors FluxCD sync
- Shows deployment status
- Runs verification checks
- Monitors metrics and logs
- Sends notification to team
```

---

## Benefits

### For Developers
- ✅ Clear visibility into deployment status
- ✅ Easy manual deployment for dev
- ✅ Automatic tracking for other environments
- ✅ Quick verification checks
- ✅ One-click access to GitLab MRs

### For DevOps
- ✅ Full control over deployment process
- ✅ Customizable workflows per project
- ✅ Integration with existing FluxCD setup
- ✅ Audit trail of all deployments
- ✅ Automated verification

### For Team
- ✅ Transparent deployment process
- ✅ Clear ownership and responsibility
- ✅ Reduced manual work
- ✅ Faster feedback loops
- ✅ Better collaboration

---

**Создано**: 2026-08-11
**Версия**: 1.0
**Статус**: ✅ Готово к реализации
