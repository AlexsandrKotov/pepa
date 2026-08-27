# Migrating from Cortex to PEPA

This guide helps you migrate your Internal Developer Portal from [Cortex](https://www.cortex.io/) to PEPA.

## Why Migrate?

| Feature | Cortex | PEPA |
|---------|--------|------|
| Hosting | SaaS only | Self-hosted (air-gapped ready) |
| Source code | Closed source | Open source (Apache 2.0) |
| GitOps orchestration | Not available | Native FluxCD/ArgoCD with drift detection |
| Workflow engine | Not available | Visual DAG designer |
| Plugin ecosystem | Closed | Open SDK (Go, Python, TS) |
| AI assistant | Basic | Built-in RAG with LLM-agnostic support |
| Virtualization | Not available | Proxmox VE, Docker host management |
| Cost | Per-service pricing | Free (open source) |

## Migration Steps

### 1. Export Data from Cortex

```bash
# Export catalogs (entities)
curl -s "https://api.cortex.io/v1/catalog" \
  -H "Authorization: Bearer $CORTEX_API_KEY" \
  | jq '.' > cortex-catalog.json

# Export scorecards
curl -s "https://api.cortex.io/v1/scorecards" \
  -H "Authorization: Bearer $CORTEX_API_KEY" \
  | jq '.' > cortex-scorecards.json
```

### 2. Map Cortex Concepts to PEPA

| Cortex Concept | PEPA Equivalent | Notes |
|---------------|-----------------|-------|
| Catalog Entity | Entity (Service) | Map `tag` to PEPA entity type |
| Tag | Entity label/type | Group services by tags |
| Scorecard | Scorecard | Direct mapping with rule conversion |
| Custom Dashboard | Dashboard | Recreate with PEPA dashboard widgets |
| Git Integration | Connection + Plugin | Configure Git provider plugin |
| Cloud Resource | Entity (type: resource) | Map to infrastructure entities |
| Dependency | Entity relationship | Use PEPA entity graph |
| Audit Log | Audit Log | PEPA has built-in immutable audit trail |

### 3. Import Entities

```bash
# Import catalog entities
for entity in $(jq -r '.[] | @base64' cortex-catalog.json); do
  _jq() { echo "$entity" | base64 --decode | jq -r "$1"; }
  
  curl -X POST "http://pepa:8088/api/v1/entities" \
    -H "Authorization: Bearer $PEPA_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"$(_jq '.tag')\",
      \"type\": \"service\",
      \"title\": \"$(_jq '.name // .tag')\",
      \"metadata\": {
        \"description\": \"$(_jq '.description // empty')\",
        \"owner\": \"$(_jq '.owner // empty')\"
      }
    }"
done
```

### 4. Convert Scorecards

Cortex scorecards map to PEPA scorecards:

```bash
for sc in $(jq -r '.[] | @base64' cortex-scorecards.json); do
  _jq() { echo "$sc" | base64 --decode | jq -r "$1"; }
  
  curl -X POST "http://pepa:8088/api/v1/scorecards" \
    -H "Authorization: Bearer $PEPA_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"$(_jq '.tag')\",
      \"title\": \"$(_jq '.name')\",
      \"rules\": $(_jq '.rules')
    }"
done
```

### 5. Set Up Integrations

| Cortex Integration | PEPA Equivalent |
|-------------------|-----------------|
| GitHub / GitLab / Bitbucket | Plugin (GitHub, GitLab, Bitbucket, Gitea) |
| AWS / GCP / Azure | Connection or future plugin |
| ArgoCD / FluxCD | GitOps Engine (built-in) |
| Datadog / Prometheus | Prometheus Plugin |
| Slack / PagerDuty | Slack Plugin + Webhook |
| Jenkins / CircleCI | Pipeline Builder |
| Kubernetes | Cluster Management (built-in) |
| Docker | Docker Host Management (built-in) |

### 6. Recreate Dashboards

Cortex custom dashboards can be recreated in PEPA:

1. **Dashboard** > Create new dashboard
2. Add widgets for:
   - Service catalog stats
   - Scorecard progress (Bronze/Silver/Gold/Platinum)
   - Deployment frequency
   - GitOps sync status
   - Pipeline health

### 7. Verify Migration

- [ ] All services visible in PEPA catalog
- [ ] Scorecards scoring correctly
- [ ] Entity relationships in graph view
- [ ] GitOps sync working
- [ ] Dashboards showing correct data
- [ ] RBAC roles assigned

## Need Help?

- [GitHub Discussions](https://github.com/AlexsandrKotov/pepa/discussions)
- [PEPA Documentation](../)
