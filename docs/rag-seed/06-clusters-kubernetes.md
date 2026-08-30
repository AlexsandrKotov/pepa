# PEPA — Clusters & Kubernetes Management

## Overview

PEPA connects to your Kubernetes clusters to deploy and manage applications. You can add multiple clusters (dev, staging, production) and deploy services to any combination of them.

## Adding a Cluster

### Via UI (Recommended)

1. Go to **Clusters** in the sidebar
2. Click **+ Add Cluster**
3. Choose method:
   - **Kubeconfig** — paste or upload your kubeconfig file
   - **Manual** — enter API server URL and credentials

### Kubeconfig Method

PEPA parses your kubeconfig and auto-detects:
- Cluster name
- API server URL
- If multi-cluster kubeconfig — offers to add all clusters at once

Steps:
1. Paste kubeconfig content or upload `.yaml` file
2. PEPA shows detected clusters
3. For each cluster: set name, environment (dev/staging/production), labels
4. Click **Save**

### Manual Method

1. Enter cluster name (e.g., `production-east`)
2. Select environment
3. Enter API server URL (e.g., `https://k8s.example.com:6443`)
4. Add labels (optional): `region:eu-west, team:platform`
5. Upload kubeconfig (stored encrypted in database)
6. Click **Save**

## Cluster Environments

Clusters are associated with environments:

| Environment | Purpose | Typical Clusters |
|------------|---------|-----------------|
| `dev` | Development and testing | minikube, kind, k3s |
| `staging` | Pre-production validation | staging-east, staging-west |
| `production` | Live traffic | prod-us-east, prod-eu-west |

Environments are configured in **Environments** page. Each environment can have multiple clusters.

## Cluster Status

| Status | Meaning |
|--------|---------|
| `connected` | Cluster is reachable and kubeconfig is valid |
| `disconnected` | Cluster is unreachable or kubeconfig expired |
| `error` | Last connection attempt failed |

PEPA periodically checks cluster health. Failed clusters show an error badge.

## Viewing Cluster Resources

After connecting a cluster, you can browse:
- **Namespaces** — list all namespaces
- **Workloads** — Deployments, StatefulSets, DaemonSets
- **Pods** — running containers with logs
- **Services** — ClusterIP, NodePort, LoadBalancer
- **ConfigMaps & Secrets** — configuration data

Go to **Clusters → [cluster name]** to explore.

## Deploying to a Cluster

There are multiple ways to deploy to a connected cluster:

### 1. Service Deploy (UI)
**Services → Create Service → Deploy**
- Choose a template or blueprint
- Select target cluster(s)
- Configure resources (CPU, memory, replicas)
- Click Deploy

### 2. Blueprint Deploy
**Deployments → New Deployment**
- Select a blueprint (Helm chart, container image, or Docker Compose)
- Choose target cluster and namespace
- Set environment variables
- Click Deploy

### 3. Workflow Deploy
**Workflows → Run Workflow**
- Use a workflow that includes a deploy step
- Workflows can deploy to multiple clusters in sequence
- Supports approval gates between environments

### 4. GitOps Deploy
**GitOps → Sync**
- ArgoCD or FluxCD syncs from Git repository
- PEPA triggers sync and monitors status
- Automatic drift detection and correction

## Multi-Cluster Deploy

When creating a service, you can select multiple target clusters:
- PEPA deploys to all selected clusters in parallel
- Each cluster gets its own deployment record
- Status is tracked per-cluster

Example: deploy to both `dev` and `staging` clusters simultaneously.

## Cluster Labels

Labels help organize and filter clusters:

```
region: eu-west
team: platform
tier: production
cloud: aws
```

Use labels in workflows to target clusters dynamically (e.g., "deploy to all clusters with label `tier: production`").

## Troubleshooting

**Cluster shows "disconnected"**: 
- Check kubeconfig is valid and not expired
- Verify network connectivity from PEPA to cluster API server
- For self-signed certificates: ensure TLS settings are correct

**Deployment fails with "namespace not found"**:
- PEPA auto-creates namespaces, but if RBAC restricts this, create the namespace manually first

**"Forbidden" errors during deploy**:
- The kubeconfig user needs permissions to create Deployments, Services, and Namespaces in the target namespace
