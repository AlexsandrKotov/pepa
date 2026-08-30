# PEPA — Service Blueprints

## What is a Blueprint?

A **blueprint** is a reusable deployment template that defines what to deploy and how. Instead of configuring deployment parameters every time, you create a blueprint once and deploy it with one click.

Blueprints support multiple source types:

| Source Type | Description | Example |
|------------|-------------|---------|
| `container` | Direct container image | `nginx:1.25`, `myapp:v2.0` |
| `helm_http` | Helm chart from HTTP repository | `https://charts.bitnami.com/bitnami` |
| `helm_oci` | Helm chart from OCI registry | `oci://ghcr.io/myorg/charts/myapp` |
| `helm_git` | Helm chart from Git repository | `https://github.com/org/charts` |
| `docker_compose` | Docker Compose YAML | Multi-container application |

## Creating a Blueprint

### Via UI

1. Go to **Deployments** → **Blueprints** tab
2. Click **+ Create Blueprint**
3. Fill in:
   - **Name** — human-readable name (e.g., "Nginx Web Server")
   - **Source Type** — container, helm_http, helm_oci, helm_git, or docker_compose
   - **Image / Chart URL** — depending on source type
   - **Namespace** — default Kubernetes namespace
   - **Resources** — CPU, memory, replicas
   - **Ports** — container ports to expose
   - **values.yaml** — Helm values (for Helm-based blueprints)
   - **Compose YAML** — Docker Compose definition (for docker_compose type)
4. Click **Save**

### Via API

```bash
# Container blueprint
curl -X POST /api/v1/blueprints -d '{
  "name": "Redis Cache",
  "description": "Redis 7 in-memory cache",
  "source_type": "container",
  "image": "redis:7-alpine",
  "namespace": "default",
  "cpu": "200m",
  "memory": "256Mi",
  "replicas": 1,
  "ports": [6379],
  "category": "database"
}'

# Helm chart blueprint
curl -X POST /api/v1/blueprints -d '{
  "name": "WordPress",
  "description": "WordPress CMS via Bitnami chart",
  "source_type": "helm_http",
  "chart_url": "https://charts.bitnami.com/bitnami",
  "chart_name": "wordpress",
  "chart_version": "18.0.0",
  "namespace": "wordpress",
  "values_yaml": "service:\n  type: ClusterIP\nreplicaCount: 2",
  "category": "web"
}'

# Docker Compose blueprint
curl -X POST /api/v1/blueprints -d '{
  "name": "Monitoring Stack",
  "description": "Prometheus + Grafana",
  "source_type": "docker_compose",
  "compose_yaml": "version: \"3\"\nservices:\n  prometheus:\n    image: prom/prometheus\n    ports:\n      - \"9090:9090\"\n  grafana:\n    image: grafana/grafana\n    ports:\n      - \"3000:3000\"",
  "category": "monitoring"
}'
```

## Deploying a Blueprint

### To Kubernetes

```bash
curl -X POST /api/v1/blueprints/<id>/deploy -d '{
  "cluster_id": "<cluster-uuid>",
  "namespace": "production",
  "env_vars": {"DB_HOST": "postgres.default.svc"}
}'
```

### To Docker Host

```bash
curl -X POST /api/v1/blueprints/<id>/deploy-docker -d '{
  "docker_host_id": "<docker-host-uuid>",
  "env_vars": {"LOG_LEVEL": "info"}
}'
```

### Group Deploy

Deploy multiple blueprints from a group at once:

```bash
# Deploy group to Kubernetes
curl -X POST /api/v1/blueprint-groups/<group-id>/deploy-k8s -d '{
  "cluster_id": "<cluster-uuid>",
  "namespace": "production",
  "env_vars": {}
}'

# Deploy group to Docker
curl -X POST /api/v1/blueprint-groups/<group-id>/deploy-docker -d '{
  "docker_host_id": "<docker-host-uuid>",
  "env_vars": {}
}'
```

## Blueprint Categories

Organize blueprints with categories:

| Category | Examples |
|----------|---------|
| `web` | Nginx, WordPress, Next.js apps |
| `api` | Express, FastAPI, Go APIs |
| `database` | PostgreSQL, Redis, MongoDB |
| `monitoring` | Prometheus, Grafana, Alertmanager |
| `messaging` | RabbitMQ, Kafka, NATS |
| `general` | Custom applications |

## Blueprint Groups

Group related blueprints for batch deployment:

- A "Monitoring Stack" group might include Prometheus + Grafana + Alertmanager
- A "Full Stack" group might include Frontend + API + Database

Create groups via the UI or API. Assign blueprints to a group using the `group_id` field.

## Editing a Blueprint

Blueprints can be updated at any time:

```bash
curl -X PUT /api/v1/blueprints/<id> -d '{
  "image": "myapp:v2.1",
  "replicas": 3,
  "cpu": "500m"
}'
```

Changes affect future deployments. Already-running instances are not affected unless you redeploy.

## Deleting a Blueprint

```bash
curl -X DELETE /api/v1/blueprints/<id>
```

This removes the blueprint definition. Running deployments are not affected.

## Blueprint vs Service Template

| | Blueprint | Service Template |
|---|---|---|
| **Purpose** | Reusable deployable unit | Scaffold for new services |
| **Contains** | Image/chart + config + resources | Code template + CI/CD + deployment |
| **Deploy** | One-click deploy | Creates service + repo + pipeline |
| **Use when** | Deploying existing applications | Creating new services from scratch |
