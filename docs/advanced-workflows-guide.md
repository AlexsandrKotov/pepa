# PEPA — Advanced Workflows & Deployment Guide

> Comprehensive guide to all deployment methods, workflow patterns, and monitoring strategies.

---

## Table of Contents

1. [Complete Deployment Workflows](#complete-deployment-workflows)
2. [Advanced Workflow Patterns](#advanced-workflow-patterns)
3. [Service Monitoring & Observability](#service-monitoring--observability)
4. [Multi-Environment Strategies](#multi-environment-strategies)
5. [Rollback & Recovery](#rollback--recovery)

---

## Complete Deployment Workflows

PEPA supports multiple deployment methods to fit different workflows and requirements. This guide covers all possible approaches from simple to enterprise-grade.

---

### Method 1: Manual Deployment (Direct)

**Best for**: Development, testing, quick experiments

#### Workflow
```
User → PEPA UI → Direct K8s Deploy → Cluster
```

#### Steps
1. Navigate to **Services → Deploy Service**
2. Choose a template (Node.js, Go, Python, Static, Helm)
3. Configure service parameters:
   - Name, team, environment
   - Resource limits (CPU, memory)
   - Replicas
   - Environment variables
4. Click **Deploy**
5. PEPA generates manifests and deploys directly to cluster

#### When to Use
- ✅ Quick prototyping
- ✅ Development environments
- ✅ Testing configurations
- ❌ Production deployments
- ❌ Multi-team coordination

#### Example: Deploy Node.js API
```yaml
# PEPA generates this automatically
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-node-api
  namespace: development
spec:
  replicas: 1
  selector:
    matchLabels:
      app: my-node-api
  template:
    metadata:
      labels:
        app: my-node-api
    spec:
      containers:
      - name: my-node-api
        image: node:18-alpine
        resources:
          requests:
            cpu: 200m
            memory: 256Mi
          limits:
            cpu: 400m
            memory: 512Mi
        env:
        - name: NODE_ENV
          value: development
---
apiVersion: v1
kind: Service
metadata:
  name: my-node-api
spec:
  selector:
    app: my-node-api
  ports:
  - port: 3000
    targetPort: 3000
```

---

### Method 2: GitOps Deployment (FluxCD)

**Best for**: Production, audit trails, version control

#### Workflow
```
User → PEPA → Git Repository → FluxCD → K8s Cluster
```

#### Steps
1. **Configure GitOps Repository**
   - Go to **GitOps → Add Repository**
   - Select Git provider (GitHub, GitLab, Gitea, Bitbucket)
   - Choose repository and branch
   - Set path (e.g., `clusters/production/`)
   - Enable auto-sync

2. **Deploy via PEPA**
   - Navigate to **Services → Deploy Service**
   - Configure service as usual
   - Select **GitOps Mode** (FluxCD)
   - PEPA pushes manifests to Git repository

3. **FluxCD Syncs**
   - FluxCD detects changes in Git
   - Applies manifests to cluster
   - Reports status back to PEPA

4. **Monitor**
   - View sync status in **GitOps → Sync Status**
   - Check drift detection
   - Review deployment history

#### When to Use
- ✅ Production deployments
- ✅ Audit requirements
- ✅ Version control for infrastructure
- ✅ Team collaboration
- ✅ Disaster recovery

#### Example: GitOps Structure
```
git-repo/
├── clusters/
│   ├── production/
│   │   ├── my-node-api/
│   │   │   ├── deployment.yaml
│   │   │   ├── service.yaml
│   │   │   └── kustomization.yaml
│   │   └── kustomization.yaml
│   ├── staging/
│   │   └── ...
│   └── development/
│       └── ...
└── base/
    └── my-node-api/
        ├── deployment.yaml
        ├── service.yaml
        └── kustomization.yaml
```

---

### Method 3: GitOps Deployment (ArgoCD)

**Best for**: Multi-cluster, visual management

#### Workflow
```
User → PEPA → Git Repository → ArgoCD → K8s Cluster
```

#### Steps
1. **Connect ArgoCD**
   - Go to **Connections → Add Connection**
   - Select **ArgoCD**
   - Enter ArgoCD server URL and token
   - Test connection

2. **Configure Application**
   - Navigate to **Services → Deploy Service**
   - Select **GitOps Mode** (ArgoCD)
   - Choose ArgoCD project
   - Configure sync policy (auto/manual)
   - Set health checks

3. **Deploy**
   - PEPA creates ArgoCD Application manifest
   - Pushes to Git repository
   - ArgoCD syncs application

4. **Monitor in ArgoCD**
   - View application tree
   - Check sync status
   - Review resource health

#### When to Use
- ✅ Multi-cluster deployments
- ✅ Visual application management
- ✅ Complex dependency graphs
- ✅ Progressive delivery

---

### Method 4: CI/CD Pipeline Deployment

**Best for**: Automated testing, quality gates

#### Workflow
```
Code Push → CI Pipeline → Test → Build → PEPA Deploy → Cluster
```

#### Steps
1. **Configure CI/CD Connection**
   - Go to **Connections → Add Connection**
   - Select **GitLab CI** or **GitHub Actions**
   - Enter credentials

2. **Create Pipeline**
   - Navigate to **Pipelines → New Pipeline**
   - Choose blueprint (CI/CD Pipeline)
   - Configure stages:
     ```
     checkout → build → test → security → deploy → notify
     ```
   - Set environment variables
   - Configure deployment target

3. **Pipeline Execution**
   - Developer pushes code
   - CI pipeline triggers automatically
   - Runs tests and builds
   - Calls PEPA API to deploy
   - Sends notifications

4. **Monitor Pipeline**
   - View pipeline runs in **Pipelines**
   - Check logs for each stage
   - Review deployment status

#### Example: GitLab CI Pipeline
```yaml
# .gitlab-ci.yml
stages:
  - build
  - test
  - security
  - deploy

build:
  stage: build
  script:
    - docker build -t my-app:$CI_COMMIT_SHA .
    - docker push my-app:$CI_COMMIT_SHA

test:
  stage: test
  script:
    - npm test
    - npm run lint

security:
  stage: security
  script:
    - npm audit
    - trivy image my-app:$CI_COMMIT_SHA

deploy:
  stage: deploy
  script:
    - curl -X POST http://pepa:8080/api/v1/deployments \
        -H "Authorization: Bearer $PEPA_TOKEN" \
        -d '{
          "service": "my-app",
          "version": "'$CI_COMMIT_SHA'",
          "environment": "production"
        }'
  only:
    - main
```

#### When to Use
- ✅ Automated testing required
- ✅ Quality gates
- ✅ Security scanning
- ✅ Compliance checks

---

### Method 5: Workflow-Based Deployment

**Best for**: Complex orchestration, approvals, multi-step processes

#### Workflow
```
User → PEPA Workflow → Multiple Steps → Approvals → Deploy → Notify
```

#### Steps
1. **Create Workflow**
   - Navigate to **Workflows → Create Workflow**
   - Use visual designer or YAML
   - Define steps:
     ```
     validate → build → test → approval → deploy → verify → notify
     ```

2. **Configure Steps**
   - Each step can be:
     - **Action**: Execute plugin action
     - **Approval**: Manual approval gate
     - **Condition**: Check condition
     - **Rollback**: Automatic rollback on failure

3. **Execute Workflow**
   - Trigger workflow manually or via webhook
   - Monitor progress in real-time
   - Approve when needed
   - View logs for each step

#### Example: Enterprise Deployment Workflow
```yaml
apiVersion: pepa.io/v1alpha1
kind: Workflow
metadata:
  name: enterprise-deploy
spec:
  steps:
    - name: validate-config
      plugin: builtin
      action: validate
      params:
        service: "{{ inputs.service }}"
        environment: "{{ inputs.environment }}"
    
    - name: run-tests
      plugin: ci_engine:gitlab
      action: triggerPipeline
      params:
        project: "{{ inputs.project }}"
        ref: "{{ inputs.version }}"
      waitFor:
        condition: status == "success"
    
    - name: security-scan
      plugin: ci_engine:gitlab
      action: triggerPipeline
      params:
        project: security-scanner
        variables:
          IMAGE: "{{ inputs.image }}"
      waitFor:
        condition: status == "success"
    
    - name: request-approval
      type: approval
      params:
        approvers:
          - team-lead
          - sre-on-call
        message: "Approve deployment of {{ inputs.service }} to {{ inputs.environment }}?"
        timeout: 24h
    
    - name: deploy-to-k8s
      plugin: cd_engine:argocd
      action: deploy
      params:
        application: "{{ inputs.service }}"
        revision: "{{ inputs.version }}"
      waitFor:
        condition: status == "Healthy"
        timeout: 10m
    
    - name: verify-health
      plugin: monitoring:prometheus
      action: queryMetrics
      params:
        query: "up{service='{{ inputs.service }}'}"
        threshold: 1
        duration: 5m
    
    - name: notify-team
      plugin: notification:slack
      action: sendMessage
      params:
        channel: "#deployments"
        message: "✅ {{ inputs.service }} v{{ inputs.version }} deployed to {{ inputs.environment }}"
```

#### When to Use
- ✅ Multi-step processes
- ✅ Manual approvals required
- ✅ Complex orchestration
- ✅ Integration with multiple systems

---

### Method 6: Helm Chart Deployment

**Best for**: Existing Helm charts, complex applications

#### Workflow
```
User → PEPA → Helm Repository → K8s Cluster
```

#### Steps
1. **Add Helm Repository**
   - Navigate to **Helm Repositories → Add Repository**
   - Enter repository URL
   - Authenticate if private
   - PEPA indexes available charts

2. **Deploy Chart**
   - Go to **Services → Deploy Service**
   - Select **Helm Import** template
   - Choose chart from repository
   - Configure values:
     - Override default values
     - Set environment-specific values
     - Configure secrets via Vault
   - Click **Deploy**

3. **Monitor Release**
   - View Helm release status
   - Check deployed resources
   - Upgrade or rollback as needed

#### Example: Deploy WordPress
```yaml
# PEPA generates Helm values
chart: wordpress
repo: https://charts.bitnami.com/bitnami
version: 15.2.0

values:
  wordpressUsername: admin
  wordpressPassword:
    vault:
      path: secret/data/wordpress
      key: password
  
  service:
    type: ClusterIP
    port: 80
  
  ingress:
    enabled: true
    hostname: blog.example.com
  
  resources:
    requests:
      memory: 512Mi
      cpu: 300m
    limits:
      memory: 1Gi
      cpu: 500m
  
  mariadb:
    auth:
      rootPassword:
        vault:
          path: secret/data/wordpress
          key: db-password
```

#### When to Use
- ✅ Existing Helm charts
- ✅ Complex applications (databases, message queues)
- ✅ Community charts
- ✅ Reusable configurations

---

## Advanced Workflow Patterns

### Pattern 1: Blue-Green Deployment

**Goal**: Zero-downtime deployment with instant rollback

#### Workflow
```
1. Deploy new version (green) alongside old (blue)
2. Run health checks on green
3. Switch traffic to green
4. Keep blue for rollback
5. Delete blue after verification
```

#### Implementation
```yaml
apiVersion: pepa.io/v1alpha1
kind: Workflow
metadata:
  name: blue-green-deploy
spec:
  steps:
    - name: deploy-green
      plugin: cd_engine:argocd
      action: deploy
      params:
        application: "{{ inputs.service }}-green"
        revision: "{{ inputs.new_version }}"
    
    - name: wait-green-healthy
      plugin: monitoring:prometheus
      action: queryMetrics
      params:
        query: "up{service='{{ inputs.service }}-green'}"
        threshold: 1
        duration: 2m
    
    - name: switch-traffic
      plugin: cd_engine:argocd
      action: patchResource
      params:
        resource: service/{{ inputs.service }}
        patch:
          spec:
            selector:
              version: green
    
    - name: verify-traffic
      plugin: monitoring:prometheus
      action: queryMetrics
      params:
        query: "rate(http_requests_total{version='green'}[5m])"
        threshold: 100
        duration: 5m
    
    - name: delete-blue
      plugin: cd_engine:argocd
      action: deleteResource
      params:
        resource: deployment/{{ inputs.service }}-blue
```

---

### Pattern 2: Canary Deployment

**Goal**: Gradual rollout with automatic rollback

#### Workflow
```
1. Deploy canary (5% traffic)
2. Monitor metrics
3. If healthy, increase to 25%
4. If healthy, increase to 50%
5. If healthy, increase to 100%
6. Delete old version
```

#### Implementation
```yaml
apiVersion: pepa.io/v1alpha1
kind: Workflow
metadata:
  name: canary-deploy
spec:
  steps:
    - name: deploy-canary-5
      plugin: cd_engine:argocd
      action: deploy
      params:
        application: "{{ inputs.service }}-canary"
        revision: "{{ inputs.new_version }}"
        weight: 5
    
    - name: monitor-5
      plugin: monitoring:prometheus
      action: queryMetrics
      params:
        query: "rate(http_errors_total{version='canary'}[5m]) / rate(http_requests_total{version='canary'}[5m])"
        threshold: 0.01  # 1% error rate
        duration: 10m
    
    - name: increase-to-25
      plugin: cd_engine:argocd
      action: patchResource
      params:
        resource: deployment/{{ inputs.service }}-canary
        patch:
          spec:
            weight: 25
    
    - name: monitor-25
      # ... similar to monitor-5
    
    - name: increase-to-50
      # ...
    
    - name: increase-to-100
      # ...
    
    - name: cleanup-old
      plugin: cd_engine:argocd
      action: deleteResource
      params:
        resource: deployment/{{ inputs.service }}-old
```

---

### Pattern 3: Multi-Environment Promotion

**Goal**: Promote through dev → staging → production

#### Workflow
```
1. Deploy to development
2. Run automated tests
3. If passed, deploy to staging
4. Run integration tests
5. Manual approval
6. Deploy to production
7. Verify health
8. Notify team
```

#### Implementation
```yaml
apiVersion: pepa.io/v1alpha1
kind: Workflow
metadata:
  name: multi-env-promotion
spec:
  steps:
    - name: deploy-dev
      plugin: cd_engine:argocd
      action: deploy
      params:
        application: "{{ inputs.service }}"
        environment: development
        revision: "{{ inputs.version }}"
    
    - name: test-dev
      plugin: ci_engine:gitlab
      action: triggerPipeline
      params:
        project: test-suite
        variables:
          ENV: development
          SERVICE: "{{ inputs.service }}"
      waitFor:
        condition: status == "success"
    
    - name: deploy-staging
      plugin: cd_engine:argocd
      action: deploy
      params:
        application: "{{ inputs.service }}"
        environment: staging
        revision: "{{ inputs.version }}"
    
    - name: test-staging
      plugin: ci_engine:gitlab
      action: triggerPipeline
      params:
        project: integration-tests
        variables:
          ENV: staging
          SERVICE: "{{ inputs.service }}"
      waitFor:
        condition: status == "success"
    
    - name: approval
      type: approval
      params:
        approvers:
          - release-manager
          - product-owner
        message: "Approve promotion to production?"
    
    - name: deploy-production
      plugin: cd_engine:argocd
      action: deploy
      params:
        application: "{{ inputs.service }}"
        environment: production
        revision: "{{ inputs.version }}"
    
    - name: verify-production
      plugin: monitoring:prometheus
      action: queryMetrics
      params:
        query: "up{service='{{ inputs.service }}', env='production'}"
        threshold: 1
        duration: 5m
    
    - name: notify
      plugin: notification:slack
      action: sendMessage
      params:
        channel: "#releases"
        message: "🚀 {{ inputs.service }} v{{ inputs.version }} released to production"
```

---

## Service Monitoring & Observability

### Monitoring Stack

PEPA integrates with multiple monitoring tools:

#### 1. Prometheus + Grafana

**Setup**:
1. Go to **Connections → Add Connection**
2. Select **Prometheus**
3. Enter Prometheus URL
4. Test connection

**Capabilities**:
- Query metrics via PromQL
- View alert rules
- Check service health
- Create dashboards

**Example Queries**:
```promql
# Service health
up{service="my-app"}

# Request rate
rate(http_requests_total{service="my-app"}[5m])

# Error rate
rate(http_errors_total{service="my-app"}[5m]) / rate(http_requests_total{service="my-app"}[5m])

# Latency
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket{service="my-app"}[5m]))

# Resource usage
container_cpu_usage_seconds_total{pod=~"my-app.*"}
container_memory_usage_bytes{pod=~"my-app.*"}
```

#### 2. Kubernetes Native Monitoring

**Built-in Metrics**:
- Pod status (Running, Pending, Failed)
- Container resource usage
- Node health
- Deployment replicas

**View in PEPA**:
- Navigate to **Clusters → [Cluster Name]**
- View pod status
- Check resource usage
- Monitor events

#### 3. Application Logs

**Log Aggregation**:
- View logs directly from pods
- Filter by container, time, keywords
- Stream logs in real-time

**Access Logs**:
```bash
# Via PEPA UI
Clusters → [Cluster] → Pods → [Pod] → Logs

# Via kubectl
kubectl logs -n development deployment/my-app -f
```

---

### Health Checks

PEPA performs automatic health checks:

#### 1. Kubernetes Health

**Checks**:
- Pod readiness probes
- Liveness probes
- Resource limits
- Restart count

**Alerts**:
- Pod not ready for > 5 minutes
- High restart count (> 3 in 1 hour)
- Resource limits exceeded

#### 2. Application Health

**HTTP Health Check**:
```yaml
# PEPA checks these endpoints
health:
  endpoint: /healthz
  interval: 30s
  timeout: 5s
  expectedStatus: 200

readiness:
  endpoint: /readyz
  interval: 10s
  timeout: 3s
  expectedStatus: 200
```

#### 3. Custom Health Checks

**Define custom checks**:
```yaml
healthChecks:
  - name: database
    type: tcp
    host: postgres
    port: 5432
    interval: 30s
  
  - name: redis
    type: tcp
    host: redis
    port: 6379
    interval: 30s
  
  - name: external-api
    type: http
    url: https://api.external.com/health
    interval: 60s
    expectedStatus: 200
```

---

### Alerting

#### Configure Alerts

1. **Prometheus Alerts**
   - Define alert rules in Prometheus
   - PEPA displays active alerts
   - Integrate with Alertmanager

2. **PEPA Alerts**
   - Navigate to **Settings → Alerts**
   - Configure alert rules:
     ```yaml
     alerts:
       - name: high-error-rate
         condition: "error_rate > 0.05"
         duration: 5m
         severity: critical
         notify:
           - slack: "#alerts"
           - email: "oncall@example.com"
       
       - name: pod-crash-loop
         condition: "restart_count > 5"
         duration: 10m
         severity: warning
         notify:
           - slack: "#alerts"
     ```

3. **Notification Channels**
   - Slack
   - Email
   - Webhook
   - PagerDuty (via plugin)

---

### Dashboards

#### Built-in Dashboards

1. **Service Overview**
   - Request rate
   - Error rate
   - Latency (p50, p95, p99)
   - Resource usage

2. **Deployment Dashboard**
   - Deployment frequency
   - Success rate
   - Rollback count
   - Lead time

3. **Infrastructure Dashboard**
   - Cluster health
   - Node resources
   - Pod count
   - Network I/O

#### Custom Dashboards

Create custom dashboards:
1. Navigate to **Dashboards → Create Dashboard**
2. Add widgets:
   - Graph (Prometheus query)
   - Stat (single value)
   - Table (multi-value)
   - Text (description)
3. Arrange layout
4. Save and share

---

## Multi-Environment Strategies

### Environment Hierarchy

```
Development
  └─ Quick iterations
  └─ Auto-deploy on push
  └─ Minimal resources

Staging
  └─ Pre-production testing
  └─ Manual approval required
  └─ Production-like config

Production
  └─ Live traffic
  └─ Strict approval process
  └─ High availability
  └─ Monitoring & alerting
```

### Environment-Specific Configuration

**Using Kustomize**:
```yaml
# base/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: my-app
        image: my-app:latest
        resources:
          requests:
            cpu: 100m
            memory: 128Mi

# overlays/development/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
bases:
  - ../../base
patchesStrategicMerge:
  - replicas.yaml
  - resources.yaml

# overlays/development/replicas.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 1

# overlays/production/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
bases:
  - ../../base
patchesStrategicMerge:
  - replicas.yaml
  - resources.yaml

# overlays/production/replicas.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 3
```

---

## Rollback & Recovery

### Automatic Rollback

**Trigger Conditions**:
- Health check fails
- Error rate > threshold
- Latency > threshold
- Pod crash loop

**Configuration**:
```yaml
rollback:
  enabled: true
  conditions:
    - metric: error_rate
      threshold: 0.05
      duration: 5m
    - metric: latency_p95
      threshold: 2s
      duration: 5m
    - metric: pod_restarts
      threshold: 5
      duration: 10m
  
  action:
    type: rollback
    notify:
      - slack: "#alerts"
      - email: "oncall@example.com"
```

### Manual Rollback

**Via PEPA UI**:
1. Navigate to **Deployments**
2. Find the deployment to rollback
3. Click **Rollback**
4. Select previous version
5. Confirm rollback

**Via Workflow**:
```yaml
steps:
  - name: rollback
    plugin: cd_engine:argocd
    action: rollback
    params:
      application: "{{ inputs.service }}"
      revision: "{{ inputs.previous_version }}"
```

### Disaster Recovery

**Backup Strategy**:
1. **Database Backup**
   ```bash
   # Daily backup
   kubectl exec -n pepa postgres-0 -- pg_dump -U pepa pepa > backup_$(date +%Y%m%d).sql
   ```

2. **Git Repository**
   - All manifests in Git
   - Version controlled
   - Easy to restore

3. **Vault Secrets**
   - Enable Vault snapshots
   - Store backup securely
   - Test restore procedure

**Recovery Procedure**:
1. Restore database from backup
2. Restore Vault from snapshot
3. GitOps will reconcile cluster state
4. Verify all services healthy

---

## Best Practices

### 1. Start Simple
- Begin with manual deployments
- Add CI/CD when comfortable
- Introduce GitOps for production
- Use workflows for complex scenarios

### 2. Use Version Control
- Store all manifests in Git
- Use branches for environments
- Tag releases
- Review changes via PRs

### 3. Automate Testing
- Unit tests in CI
- Integration tests in staging
- Smoke tests in production
- Automated rollback on failure

### 4. Monitor Everything
- Application metrics
- Infrastructure metrics
- Deployment metrics
- Business metrics

### 5. Document Procedures
- Deployment runbooks
- Rollback procedures
- Disaster recovery plans
- On-call procedures

---

## Conclusion

PEPA provides a comprehensive set of deployment methods to fit any workflow:

- **Manual**: Quick and simple
- **GitOps**: Version controlled and auditable
- **CI/CD**: Automated and tested
- **Workflows**: Complex orchestration
- **Helm**: Reusable and configurable

Choose the method that fits your needs, and evolve as your requirements grow. Start simple, add complexity when needed, and always prioritize reliability and observability.

---

**Created**: 2026-08-24  
**Version**: 1.0  
**Status**: ✅ Complete
