# Migrating from Port to PEPA

This guide helps you migrate your Internal Developer Portal from [Port](https://getport.io/) to PEPA.

## Why Migrate?

| Feature | Port | PEPA |
|---------|------|------|
| Hosting | SaaS only | Self-hosted (air-gapped ready) |
| Source code | Closed source | Open source (Apache 2.0) |
| GitOps orchestration | Basic | Native FluxCD/ArgoCD with drift detection |
| Workflow engine | JSON-based | Visual DAG designer with YAML |
| Plugin system | Closed | Open SDK (Go, Python, TS) |
| AI assistant | Basic | Built-in RAG with LLM-agnostic support |
| Data ownership | Port cloud | Your infrastructure |
| Cost | Per-user pricing | Free (open source) |

## Migration Steps

### 1. Export Data from Port

```bash
# Export all blueprints
curl -s "https://api.getport.io/v1/blueprints" \
  -H "Authorization: Bearer $PORT_CLIENT_ID:$PORT_CLIENT_SECRET" \
  | jq '.' > port-blueprints.json

# Export all entities
curl -s "https://api.getport.io/v1/entities" \
  -H "Authorization: Bearer $PORT_CLIENT_ID:$PORT_CLIENT_SECRET" \
  | jq '.' > port-entities.json

# Export scorecards
curl -s "https://api.getport.io/v1/scorecards" \
  -H "Authorization: Bearer $PORT_CLIENT_ID:$PORT_CLIENT_SECRET" \
  | jq '.' > port-scorecards.json
```

### 2. Map Blueprints to PEPA Entities

| Port Concept | PEPA Equivalent | Notes |
|-------------|-----------------|-------|
| Blueprint | Entity type definition | PEPA uses dynamic schema-on-read |
| Entity | Entity (Service, Resource, etc.) | Map properties to PEPA metadata |
| Relation | Entity relationship | Use PEPA entity graph edges |
| Scorecard | Scorecard | Direct mapping with rule conversion |
| Action | Workflow / Pipeline | Convert to PEPA workflow YAML |
| Integration | Plugin / Connection | Map to PEPA plugins |
| Page | Dashboard widget | Configure in PEPA dashboards |
| Team | Team (RBAC) | Map to PEPA RBAC teams |

### 3. Import Entities

```bash
# Import entities via PEPA API
for entity in $(jq -r '.entities[] | @base64' port-entities.json); do
  _jq() { echo "$entity" | base64 --decode | jq -r "$1"; }
  
  blueprint="$(_jq '.blueprint')"
  
  curl -X POST "http://pepa:8088/api/v1/entities" \
    -H "Authorization: Bearer $PEPA_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"$(_jq '.identifier')\",
      \"type\": \"$blueprint\",
      \"title\": \"$(_jq '.title')\",
      \"metadata\": $(_jq '.properties')
    }"
done
```

### 4. Convert Scorecards

Port scorecards map directly to PEPA scorecards:

```bash
# Create PEPA scorecards from Port definitions
for sc in $(jq -r '.scorecards[] | @base64' port-scorecards.json); do
  _jq() { echo "$sc" | base64 --decode | jq -r "$1"; }
  
  curl -X POST "http://pepa:8088/api/v1/scorecards" \
    -H "Authorization: Bearer $PEPA_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"$(_jq '.identifier')\",
      \"title\": \"$(_jq '.title')\",
      \"rules\": $(_jq '.rules')
    }"
done
```

### 5. Convert Actions to Workflows

Port Actions become PEPA Workflows:

```yaml
# Port Action
{
  "identifier": "deploy-service",
  "title": "Deploy Service",
  "invocationMethod": { "type": "WEBHOOK" },
  "userProperties": { "version": { "type": "string" } }
}

# PEPA Workflow equivalent
name: deploy-service
title: Deploy Service
trigger:
  type: webhook
parameters:
  - name: version
    type: string
    required: true
steps:
  - name: validate
    type: approval
    message: "Confirm deployment of version {{ .params.version }}"
  - name: deploy
    type: gitops
    action: sync
    target: "{{ .params.cluster }}"
```

### 6. Set Up Integrations

| Port Integration | PEPA Equivalent |
|-----------------|-----------------|
| GitHub/GitLab | Connections + Plugins |
| AWS/GCP/Azure | Plugin (planned) or Connection |
| Slack/Teams | Notification Plugin |
| ArgoCD/FluxCD | GitOps Engine (built-in) |
| Kafka/RabbitMQ | Connection (webhook) |
| Datadog/Prometheus | Prometheus Plugin |

### 7. Verify Migration

- [ ] All entities visible in PEPA catalog
- [ ] Entity relationships preserved in graph
- [ ] Scorecards scoring correctly
- [ ] Workflows execute from triggers
- [ ] RBAC teams and permissions working
- [ ] GitOps sync operational

## Need Help?

- [GitHub Discussions](https://github.com/AlexsandrKotov/pepa/discussions)
- [PEPA Documentation](../)
