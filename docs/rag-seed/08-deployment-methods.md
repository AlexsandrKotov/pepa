# PEPA — Deployment Methods & Workflows

## Overview

PEPA supports 4 deployment methods, from simple one-click deploys to full GitOps automation. Choose the method that fits your team's workflow.

## Method 1: Direct Deploy (UI)

**Best for:** Quick deploys, development, testing

Deploy any application directly from the UI:

1. **Deployments** → **New Deployment**
2. Select a blueprint (container image, Helm chart, or Docker Compose)
3. Choose target cluster and namespace
4. Set environment variables
5. Click **Deploy**

PEPA creates Kubernetes manifests and deploys directly. No Git repository or CI/CD pipeline needed.

### Deployment Strategies

| Strategy | Description | Risk |
|----------|-------------|------|
| `rolling` | Replace pods gradually (default) | Low |
| `canary` | Route small traffic to new version first | Very Low |
| `blue-green` | Switch between two full environments | Zero downtime |

## Method 2: Service Deploy (Full Lifecycle)

**Best for:** New services that need a complete setup

1. **Services** → **Create Service**
2. Choose a template (Go, Node.js, Python, Static, Helm)
3. Configure: name, namespace, resources, environment variables
4. Select target clusters
5. Click **Deploy**

This creates:
- Service record in catalog
- Deployment to selected cluster(s)
- Tracking of deployment history

### Service Lifecycle

```
configured → deploying → deployed → verified → promoted
                ↓
              failed
```

- **configured** — service is set up but not deployed
- **deploying** — deployment in progress
- **deployed** — running in cluster
- **verified** — health checks passed
- **promoted** — promoted to next environment (e.g., staging → production)

## Method 3: Workflow Deploy (Automated)

**Best for:** Multi-step deployments, approval gates, multi-environment

Workflows are DAG-based automation pipelines. Each step can invoke a plugin action.

### Creating a Workflow

1. **Workflows** → **Create Workflow**
2. Define steps in YAML:

```yaml
name: Deploy to Production
steps:
  - name: Scan for vulnerabilities
    plugin: trivy
    action: scan_image
    params:
      image: "myapp:latest"

  - name: Deploy to staging
    plugin: argocd
    action: sync_app
    params:
      app: myapp-staging
    depends_on: [Scan for vulnerabilities]

  - name: Run smoke tests
    plugin: webhook
    action: trigger
    params:
      url: "https://staging.example.com/tests/run"
    depends_on: [Deploy to staging]

  - name: Approval gate
    type: approval
    assignee: platform-team
    depends_on: [Run smoke tests]

  - name: Deploy to production
    plugin: argocd
    action: sync_app
    params:
      app: myapp-production
    depends_on: [Approval gate]
```

### Workflow Triggers

| Trigger | Description |
|---------|-------------|
| `manual` | User clicks "Run" in UI |
| `webhook` | External HTTP trigger |
| `schedule` | Cron-based (e.g., `0 2 * * 1-5`) |
| `entity_event` | Triggered by platform events (e.g., PR merged) |

### AI Workflow Builder

Describe your workflow in natural language and PEPA generates the YAML:

```bash
curl -X POST /api/v1/ai/workflow/build -d '{
  "description": "Deploy payment-api to staging, run tests, then promote to production after approval"
}'
```

## Method 4: GitOps Deploy (ArgoCD/FluxCD)

**Best for:** Production-grade, auditable, drift-free deployments

GitOps uses Git as the single source of truth. PEPA integrates with ArgoCD and FluxCD:

1. Push changes to your Git repository
2. ArgoCD/FluxCD detects changes and syncs to cluster
3. PEPA monitors sync status and shows it in the UI

### Setting Up GitOps

1. Install `argocd` plugin from Marketplace
2. Configure ArgoCD connection in **Connections**
3. Create a GitOps workflow:

```yaml
name: GitOps Deploy
steps:
  - name: Update image tag
    plugin: github
    action: create_pr
    params:
      repo: myorg/k8s-manifests
      branch: update-payment-api-v2
      changes:
        - file: "apps/payment-api/values.yaml"
          set: "image.tag=v2.0"

  - name: Wait for ArgoCD sync
    plugin: argocd
    action: sync_app
    params:
      app: payment-api
      timeout: 300
    depends_on: [Update image tag]
```

## Deployment Management

### View Deployment Logs

Every deployment records full logs:

```bash
curl /api/v1/deployments/<id>/logs
```

Or in UI: **Deployments** → click on deployment → **View Logs**

### Promote Deployment

After verifying a deployment in staging, promote it to production:

```bash
curl -X POST /api/v1/deployments/<id>/promote
```

### Rollback

If something goes wrong, rollback to previous version:

```bash
curl -X POST /api/v1/deployments/<id>/rollback
```

### Cancel

Cancel an in-progress deployment:

```bash
curl -X POST /api/v1/deployments/<id>/cancel
```

## Deployment Status

| Status | Meaning |
|--------|---------|
| `pending` | Deployment created, waiting to start |
| `syncing` | Deploying to cluster |
| `deployed` | Successfully deployed |
| `promoted` | Promoted to next environment |
| `verified` | Health checks passed |
| `failed` | Deployment failed (check logs) |
| `cancelled` | Cancelled by user |
| `rolled_back` | Rolled back to previous version |

## Comparing Methods

| Feature | Direct | Service | Workflow | GitOps |
|---------|--------|---------|----------|--------|
| Speed | Fast | Medium | Medium | Slow |
| Audit trail | Basic | Good | Full | Full |
| Approval gates | No | No | Yes | Via Git |
| Rollback | Manual | Manual | Automated | Git revert |
| Multi-cluster | One at a time | Parallel | Sequential | Per-app |
| Best for | Dev/test | New services | Complex pipelines | Production |
