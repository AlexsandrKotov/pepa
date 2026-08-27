# Migrating from Backstage to PEPA

This guide helps you migrate your Internal Developer Portal from [Backstage](https://backstage.io/) to PEPA.

## Why Migrate?

| Feature | Backstage | PEPA |
|---------|-----------|------|
| Plugin language | TypeScript only | Go, Python, TypeScript, Rust |
| Plugin isolation | In-process | gRPC process isolation |
| GitOps orchestration | Not built-in | Native FluxCD/ArgoCD integration |
| Visual workflow designer | Not available | DAG-based visual builder |
| AI/RAG assistant | Not built-in | Built-in LLM-agnostic AI |
| Self-hosted (air-gapped) | Possible but complex | Designed for air-gapped |
| Dynamic entity model | Static catalog schema | Schema-on-read entity graph |
| Deployment footprint | Heavy (Node.js monolith) | Lightweight (Go + Next.js) |

## Migration Steps

### 1. Export Your Backstage Catalog

```bash
# Export entities from Backstage catalog API
curl -s http://your-backstage:7007/api/catalog/entities \
  -H "Authorization: Bearer $TOKEN" \
  | jq '.' > backstage-catalog-export.json
```

### 2. Map Entity Types

| Backstage Type | PEPA Equivalent | Notes |
|---------------|-----------------|-------|
| `Component` | Service (Entity) | Map `metadata.name` to PEPA service name |
| `API` | Service relationship | Link via entity relationships |
| `Resource` | Entity (type: resource) | Map to infrastructure entities |
| `System` | Entity group/tag | Use PEPA entity labels |
| `Domain` | Workspace | Map to PEPA workspaces |
| `Template` | Service Template / Workflow Template | Convert `scaffold` actions to PEPA templates |
| `Location` | Import source | Use PEPA Discovery / Import Wizard |

### 3. Import into PEPA

#### Option A: Via PEPA API

```bash
# Create services from exported catalog
for entity in $(jq -r '.[] | select(.kind=="Component") | @base64' backstage-catalog-export.json); do
  _jq() { echo "$entity" | base64 --decode | jq -r "$1"; }
  
  curl -X POST http://pepa:8088/api/v1/entities \
    -H "Authorization: Bearer $PEPA_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"$(_jq '.metadata.name')\",
      \"type\": \"service\",
      \"metadata\": {
        \"description\": \"$(_jq '.metadata.description // empty')\",
        \"owner\": \"$(_jq '.spec.owner // empty')\",
        \"lifecycle\": \"$(_jq '.spec.lifecycle // empty')\",
        \"language\": \"$(_jq '.spec.language // empty')\"
      }
    }"
done
```

#### Option B: Via PEPA Import Wizard

1. Navigate to **Import** in PEPA sidebar
2. Select your Git provider (GitHub/GitLab)
3. Point to the repositories that contained Backstage `catalog-info.yaml` files
4. PEPA will auto-discover and import services

### 4. Migrate Plugins

| Backstage Plugin | PEPA Equivalent | Migration |
|-----------------|-----------------|-----------|
| `@backstage/plugin-catalog` | Built-in Service Catalog | Automatic |
| `@backstage/plugin-scaffolder` | Service Templates + Workflow Engine | Convert templates to PEPA format |
| `@backstage/plugin-techdocs` | Built-in Documentation Portal | Copy docs to PEPA `/docs` |
| `@backstage/plugin-kubernetes` | Built-in Cluster Management | Reuse existing kubeconfig |
| `@backstage/plugin-ci-cd` | Built-in Pipeline Builder | Map CI configs to PEPA pipelines |
| `@backstage/plugin-search` | Built-in Search | Automatic |
| `@backstage/plugin-auth` | Built-in RBAC + SSO | Configure RBAC roles |
| Custom plugins | PEPA Plugin SDK | Rewrite using Go/Python SDK |

### 5. Migrate Scaffolder Templates

Convert Backstage `Template` entities to PEPA Service Templates:

```yaml
# Backstage template (template.yaml)
apiVersion: scaffolder.backstage.io/v1beta3
kind: Template
metadata:
  name: nodejs-service
spec:
  steps:
    - id: fetch-template
      name: Fetch Template
      action: fetch:template
      input:
        url: ./skeleton
    - id: publish
      name: Publish
      action: publish:github
      input:
        repoUrl: github.com?repo={{ parameters.repoName }}
```

PEPA equivalent — create via **Services > New** or API:

```json
{
  "name": "nodejs-service",
  "type": "service",
  "template": {
    "runtime": "nodejs",
    "repository": "your-org/nodejs-template",
    "pipeline": "nodejs-ci",
    "helm_chart": "your-org/charts/nodejs"
  }
}
```

### 6. Configure GitOps

Backstage does not have native GitOps. In PEPA:

1. **Connections** > Add your FluxCD/ArgoCD instance
2. **GitOps** > Configure repository sync
3. **Clusters** > Connect your Kubernetes clusters
4. **Environments** > Define dev/staging/production targets

### 7. Set Up RBAC

Map Backstage roles to PEPA RBAC:

| Backstage Role | PEPA Role | Permissions |
|---------------|-----------|-------------|
| Admin | Platform Admin | Full access |
| Developer | Developer | Services, pipelines, workflows |
| Viewer | Viewer | Read-only access |

### 8. Verify Migration

- [ ] All services visible in PEPA catalog
- [ ] Entity relationships preserved
- [ ] GitOps sync working for all services
- [ ] RBAC roles correctly assigned
- [ ] Pipelines execute successfully
- [ ] AI assistant can query the catalog

## Post-Migration

1. **Decommission Backstage**: Once verified, stop the Backstage instance
2. **Update bookmarks**: Share the new PEPA URL with your team
3. **Train users**: Share the [PEPA User Guide](../user-guide-en.md)
4. **Monitor**: Check PEPA audit logs for any access issues

## Need Help?

- [GitHub Discussions](https://github.com/akotau/pepa/discussions)
- [PEPA Documentation](../)
- Email: support@pepa.io
