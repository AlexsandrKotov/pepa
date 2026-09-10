# Scorecard Rules Reference

Scorecards evaluate entities (services, teams, environments, etc.) against weighted rules. Each rule uses an expression to check entity properties and metadata.

## Expression Syntax

### Basic Comparisons

```
field == value    # Equals
field != value    # Not equals
field >= number   # Greater than or equal (numeric)
field <= number   # Less than or equal (numeric)
field > number    # Greater than (numeric)
field < number    # Less than (numeric)
```

### Logical Operators

```
expr1 && expr2    # AND — both must be true
expr1 || expr2    # OR — at least one must be true
(expr1 || expr2) && expr3   # Parentheses for grouping
```

### Existence Checks

```
has_metadata.field_name          # Metadata field exists
not_empty.field_name             # Field exists and is not empty
field == null                    # Field is missing or null
field != null                    # Field exists and is not null
```

### String Operations

```
field contains "substring"       # Field contains substring
field starts_with "prefix"       # Field starts with prefix
```

### Special Literals

```
always_true     # Always passes (useful for testing)
always_false    # Always fails (useful for testing)
```

## Available Fields

### Top-Level Entity Fields

These fields are directly accessible from the entity:

| Field | Type | Description | Example Values |
|-------|------|-------------|----------------|
| `type_key` | string | Entity type identifier | `service`, `team`, `environment`, `api_endpoint` |
| `name` | string | Entity name | `payment-api`, `backend-team`, `production` |
| `description` | string | Entity description | Any text |
| `status` | string | Entity status | `active`, `inactive`, `deprecated` |
| `sync_status` | string | Synchronization status | `synced`, `pending`, `error` |
| `plugin_name` | string | Source plugin name | `kubernetes`, `gitops`, `manual` |
| `external_id` | string | External identifier | `k8s:cluster-1:default:my-service` |

### Metadata Fields

Metadata is a flexible JSON object attached to each entity. Access metadata fields using:

- `has_metadata.field_name` — check if field exists
- `not_empty.field_name` — check if field exists and is non-empty
- `metadata.field_name` — access field value for comparison
- `field_name` — shorthand for `metadata.field_name` in comparisons

#### Common Metadata Fields

These are commonly used metadata fields (actual fields depend on your entity configuration):

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `health_endpoint` | string | Health check URL | `/health`, `http://svc/health` |
| `owner` | string | Owner team or person | `backend-team`, `john@example.com` |
| `repository` | string | Source code repository URL | `https://github.com/org/repo` |
| `replicas` | number | Number of replicas/instances | `3`, `5` |
| `image` | string | Container image | `nginx:1.25`, `myapp:v2.1.0` |
| `source` | string | Source system | `kubernetes`, `gitops`, `manual` |
| `cluster` | string | Cluster name | `production`, `staging`, `dev` |
| `namespace` | string | Namespace/namespace | `default`, `production`, `kube-system` |
| `monitoring` | boolean/object | Monitoring configured | `true`, `{"prometheus": true}` |
| `documentation` | string/object | Documentation link/config | `https://docs.example.com` |
| `ci_cd` | string/object | CI/CD pipeline config | `github-actions`, `{"provider": "gitlab"}` |
| `vault_secrets` | boolean/object | Vault integration | `true`, `{"path": "secret/myapp"}` |
| `resource_limits` | object | Resource limits config | `{"cpu": "500m", "memory": "512Mi"}` |
| `tags` | array | Entity tags | `["critical", "production"]` |
| `version` | string | Version identifier | `v1.2.3`, `2024.01.15` |
| `environment` | string | Environment name | `production`, `staging`, `development` |

## Examples

### Production Readiness Checks

**1. Service has health endpoint and is active**
```
has_metadata.health_endpoint && status == active
```

**2. At least 2 replicas for high availability**
```
metadata.replicas >= 2
```

**3. Owner assigned and repository linked**
```
not_empty.owner && not_empty.repository
```

**4. Production service with monitoring and resource limits**
```
metadata.environment == production && has_metadata.monitoring && has_metadata.resource_limits
```

### Security Checks

**5. Uses Vault for secrets**
```
has_metadata.vault_secrets
```

**6. Resource limits defined**
```
has_metadata.resource_limits
```

**7. Image from approved registry**
```
metadata.image starts_with "registry.company.com/"
```

**8. Repository from approved host**
```
metadata.repository contains "github.com"
```

### Best Practices

**9. Has documentation and CI/CD configured**
```
has_metadata.documentation && has_metadata.ci_cd
```

**10. Service type entity with description**
```
type_key == service && not_empty.description
```

**11. Kubernetes service with namespace and cluster**
```
type_key == service && has_metadata.namespace && has_metadata.cluster
```

**12. Version tag follows semver**
```
metadata.version starts_with "v" && metadata.version contains "."
```

### Complex Conditions

**13. Production or staging with full setup**
```
(metadata.environment == production || metadata.environment == staging) && has_metadata.monitoring && has_metadata.health_endpoint
```

**14. Critical service requirements**
```
type_key == service && metadata.replicas >= 3 && has_metadata.vault_secrets && has_metadata.resource_limits && not_empty.owner
```

**15. GitOps-managed service with CI/CD**
```
metadata.source == gitops && has_metadata.ci_cd && has_metadata.repository
```

## Weight and Severity

### Weight (1-10)
Determines how much a rule contributes to the overall score.

- **1-3**: Low impact — nice-to-have checks
- **4-6**: Medium impact — important but not critical
- **7-10**: High impact — critical requirements

### Severity
Indicates the importance level of the rule.

- **info**: Informational check, no urgency
- **warning**: Should be addressed, may cause issues
- **critical**: Must be fixed, blocks production readiness

## Score Levels

| Level | Threshold | Meaning |
|-------|-----------|---------|
| Platinum | ≥90% | Excellent — exceeds all requirements |
| Gold | ≥75% | Very good — meets most requirements |
| Silver | ≥50% | Acceptable — meets basic requirements |
| Bronze | ≥25% | Minimal — needs improvement |
| None | <25% | Poor — significant gaps |

## Tips

1. **Start with presets** — use the "From Template" button to create scorecards from predefined templates
2. **Test expressions** — use `always_true` and `always_false` to test scoring before writing real rules
3. **Use parentheses** — group conditions clearly when mixing `&&` and `||`
4. **Check metadata first** — use `has_metadata.field` before comparing values to avoid false negatives
5. **Combine related checks** — use `&&` to require multiple conditions in a single rule
6. **Provide clear messages** — write descriptive pass/fail messages to help users understand results

## Expression Builder

The visual builder helps construct expressions without memorizing syntax:

1. **Select field** — choose from top-level fields or metadata checks
2. **Select operator** — choose comparison type
3. **Enter value** — type or select from presets
4. **Add conditions** — combine multiple checks with AND/OR
5. **Preview** — see the generated expression before saving

Switch to **raw mode** for advanced syntax not supported by the visual builder (parentheses, numeric comparisons, string operations).
