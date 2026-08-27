# Universal Data Model & Dynamic Entity Graph

## 1. Design Philosophy

Traditional platforms hardcode entity schemas (Service, Repository, Deployment). PEPA takes a fundamentally different approach: a **Universal Entity Model** where entity types are **dynamically defined** by plugins and administrators, while maintaining a stable core schema for system entities.

### Key Principles

- **Schema-on-Read**: Core tables store generic entities; typed metadata lives in JSONB columns
- **Graph-Native**: Every entity can relate to any other entity through typed edges
- **Plugin-Defined Types**: Plugins register their entity types and relationships at initialization
- **Queryable**: PGvector enables both relational queries and vector similarity search in one store
- **Multi-Tenant**: Row-level security (RLS) ensures tenant isolation at the database level

---

## 2. Core Schema (PostgreSQL)

### 2.1 Entity Types Registry

```sql
-- ============================================================
-- Dynamic Entity Type Registry
-- Plugins register their entity types here
-- ============================================================
CREATE TABLE entity_types (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type_key        VARCHAR(128) NOT NULL UNIQUE,  -- e.g., "service", "k8s_pod", "jira_issue"
    display_name    VARCHAR(256) NOT NULL,
    description     TEXT,
    plugin_name     VARCHAR(128) REFERENCES plugins(name),  -- NULL for core types
    icon            VARCHAR(256),
    category        VARCHAR(64),   -- "compute", "source", "deployment", "incident", etc.
    
    -- JSON Schema defining allowed metadata fields
    metadata_schema JSONB NOT NULL DEFAULT '{}',
    
    -- UI configuration
    ui_config JSONB DEFAULT '{}',  -- display columns, colors, groupings
    
    -- Lifecycle
    is_system       BOOLEAN DEFAULT FALSE,  -- Cannot be deleted by users
    is_enabled      BOOLEAN DEFAULT TRUE,
    
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Core entity types (always available)
INSERT INTO entity_types (type_key, display_name, category, is_system, metadata_schema) VALUES
    ('service',       'Service',          'compute',     TRUE, '{"type":"object","properties":{"tier":{"type":"string"},"language":{"type":"string"},"framework":{"type":"string"},"owner_team":{"type":"string"},"lifecycle":{"type":"string","enum":["development","staging","production","deprecated"]}}}'),
    ('team',          'Team',             'organization',TRUE, '{"type":"object","properties":{"department":{"type":"string"},"slack_channel":{"type":"string"},"email":{"type":"string"}}}'),
    ('environment',   'Environment',      'deployment',  TRUE, '{"type":"object","properties":{"cluster":{"type":"string"},"namespace":{"type":"string"},"cloud_region":{"type":"string"},"type":{"type":"string","enum":["dev","staging","production","dr"]}}}'),
    ('api_endpoint',  'API Endpoint',     'interface',   TRUE, '{"type":"object","properties":{"method":{"type":"string"},"path":{"type":"string"},"auth_type":{"type":"string"},"rate_limit":{"type":"integer"}}}');
```

### 2.2 Universal Entity Table

```sql
-- ============================================================
-- Universal Entity Store
-- All entities (core + plugin-defined) live here
-- ============================================================
CREATE TABLE entities (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type_id         UUID NOT NULL REFERENCES entity_types(id),
    type_key        VARCHAR(128) NOT NULL,  -- Denormalized for query performance
    
    -- Identity
    name            VARCHAR(512) NOT NULL,
    description     TEXT,
    external_id     VARCHAR(512),  -- ID in the external system (e.g., Jira issue key)
    
    -- Multi-tenancy
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    
    -- Flexible metadata (validated against entity_types.metadata_schema)
    metadata        JSONB NOT NULL DEFAULT '{}',
    
    -- Status tracking
    status          VARCHAR(64) DEFAULT 'active',
    status_detail   TEXT,
    
    -- Provenance
    plugin_name     VARCHAR(128),  -- Which plugin manages this entity (NULL = manual)
    sync_status     VARCHAR(32) DEFAULT 'synced',  -- synced, pending, error, stale
    last_synced_at  TIMESTAMPTZ,
    
    -- Vector embedding for AI/RAG similarity search
    embedding       vector(1536),  -- PGvector — for semantic search
    
    -- Audit
    created_by      UUID REFERENCES users(id),
    updated_by      UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,  -- Soft delete
    
    -- Constraints
    UNIQUE (type_key, external_id, tenant_id)
);

-- Performance indexes
CREATE INDEX idx_entities_type ON entities(type_key, tenant_id);
CREATE INDEX idx_entities_external ON entities(type_key, external_id, tenant_id);
CREATE INDEX idx_entities_metadata ON entities USING GIN(metadata);
CREATE INDEX idx_entities_status ON entities(tenant_id, status);
CREATE INDEX idx_entities_embedding ON entities USING ivfflat(embedding vector_cosine_ops) WITH (lists = 100);
CREATE INDEX idx_entities_search ON entities USING GIN(to_tsvector('english', name || ' ' || COALESCE(description, '')));

-- Row-Level Security for multi-tenancy
ALTER TABLE entities ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON entities
    USING (tenant_id = current_setting('app.current_tenant')::UUID);
```

### 2.3 Entity Relationships (Dynamic Graph Edges)

```sql
-- ============================================================
-- Relationship Types Registry
-- Defines what relationships are valid between entity types
-- ============================================================
CREATE TABLE relationship_types (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type_key        VARCHAR(128) NOT NULL UNIQUE,  -- e.g., "depends_on", "deployed_to", "owned_by"
    display_name    VARCHAR(256) NOT NULL,
    description     TEXT,
    
    -- Source and target entity type constraints
    source_types    VARCHAR(128)[] NOT NULL,  -- Allowed source type_keys (or ['*'] for any)
    target_types    VARCHAR(128)[] NOT NULL,  -- Allowed target type_keys (or ['*'] for any)
    
    -- Cardinality
    cardinality     VARCHAR(16) DEFAULT 'many_to_many',  -- one_to_one, one_to_many, many_to_many
    
    -- UI
    display_color   VARCHAR(7),    -- Hex color for graph visualization
    display_label   VARCHAR(64),   -- Short label for edges in graph
    is_directional  BOOLEAN DEFAULT TRUE,
    
    plugin_name     VARCHAR(128),
    is_system       BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Core relationship types
INSERT INTO relationship_types (type_key, display_name, source_types, target_types, cardinality, display_color, is_directional, is_system) VALUES
    ('owns',           'Owned By',          ARRAY['*'],              ARRAY['team'],           'many_to_one',  '#3B82F6', TRUE,  TRUE),
    ('depends_on',     'Depends On',        ARRAY['*'],              ARRAY['*'],              'many_to_many', '#EF4444', TRUE,  TRUE),
    ('deployed_to',    'Deployed To',       ARRAY['service'],        ARRAY['environment'],    'many_to_many', '#10B981', TRUE,  TRUE),
    ('has_repository', 'Has Repository',    ARRAY['service'],        ARRAY['git_repository'], 'one_to_many',  '#8B5CF6', TRUE,  TRUE),
    ('has_issue',      'Has Issue',         ARRAY['service'],        ARRAY['task_tracker_issue'], 'many_to_many', '#F59E0B', TRUE, TRUE),
    ('runs_on',        'Runs On',           ARRAY['service'],        ARRAY['k8s_deployment', 'k8s_pod'], 'many_to_many', '#06B6D4', TRUE, TRUE),
    ('monitored_by',   'Monitored By',      ARRAY['service'],        ARRAY['monitoring_dashboard'], 'many_to_many', '#EC4899', FALSE, TRUE),
    ('part_of',        'Part Of',           ARRAY['*'],              ARRAY['*'],              'many_to_one',  '#6366F1', TRUE,  TRUE),
    ('managed_by',     'Managed By',        ARRAY['*'],              ARRAY['team'],           'many_to_one',  '#14B8A6', TRUE,  TRUE);

-- ============================================================
-- Entity Relationships (Graph Edges)
-- ============================================================
CREATE TABLE entity_relationships (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_type_id UUID NOT NULL REFERENCES relationship_types(id),
    type_key        VARCHAR(128) NOT NULL,  -- Denormalized
    
    -- Edge endpoints
    source_id       UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    target_id       UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    
    -- Edge metadata (e.g., deployment relationship can have version, timestamp)
    metadata        JSONB DEFAULT '{}',
    
    -- Multi-tenancy
    tenant_id       UUID NOT NULL,
    
    -- Provenance
    plugin_name     VARCHAR(128),
    
    -- Audit
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    
    -- Constraints
    UNIQUE (source_id, target_id, relationship_type_id),
    CHECK (source_id != target_id)  -- No self-loops
);

-- Graph traversal indexes
CREATE INDEX idx_rel_source ON entity_relationships(source_id, tenant_id);
CREATE INDEX idx_rel_target ON entity_relationships(target_id, tenant_id);
CREATE INDEX idx_rel_type ON entity_relationships(type_key, tenant_id);
CREATE INDEX idx_rel_composite ON entity_relationships(source_id, target_id, type_key);

-- RLS
ALTER TABLE entity_relationships ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_rel ON entity_relationships
    USING (tenant_id = current_setting('app.current_tenant')::UUID);
```

### 2.4 Entity Graph Query Patterns

```sql
-- ============================================================
-- Graph Traversal Queries
-- ============================================================

-- 1. Get all entities related to a service (1-hop)
SELECT 
    e.*,
    et.display_name AS type_name,
    rt.display_name AS relationship_name,
    rt.type_key AS relationship_type,
    CASE WHEN er.source_id = $1 THEN 'outgoing' ELSE 'incoming' END AS direction
FROM entity_relationships er
JOIN entities e ON (e.id = er.target_id AND er.source_id = $1)
               OR (e.id = er.source_id AND er.target_id = $1)
JOIN entity_types et ON et.id = e.type_id
JOIN relationship_types rt ON rt.id = er.relationship_type_id
WHERE er.tenant_id = $2
ORDER BY rt.display_name, e.name;

-- 2. Recursive graph traversal (N-hop) — find full dependency tree
WITH RECURSIVE graph_traversal AS (
    -- Base case: start entity
    SELECT 
        e.id, e.name, e.type_key, e.status, e.metadata,
        ARRAY[e.name] AS path,
        0 AS depth
    FROM entities e
    WHERE e.id = $1
    
    UNION ALL
    
    -- Recursive: follow relationships
    SELECT 
        next_e.id, next_e.name, next_e.type_key, next_e.status, next_e.metadata,
        gt.path || next_e.name,
        gt.depth + 1
    FROM graph_traversal gt
    JOIN entity_relationships er ON er.source_id = gt.id
    JOIN entities next_e ON next_e.id = er.target_id
    WHERE gt.depth < $3  -- Max depth parameter
      AND NOT next_e.name = ANY(gt.path)  -- Prevent cycles
)
SELECT * FROM graph_traversal ORDER BY depth, name;

-- 3. Impact analysis — "What breaks if this service goes down?"
WITH RECURSIVE downstream AS (
    SELECT e.id, e.name, e.type_key, 0 AS depth,
           ARRAY[e.id] AS visited
    FROM entities e WHERE e.id = $1
    
    UNION ALL
    
    SELECT e.id, e.name, e.type_key, d.depth + 1,
           d.visited || e.id
    FROM downstream d
    JOIN entity_relationships er ON er.target_id = d.id
        AND er.type_key IN ('depends_on', 'runs_on')
    JOIN entities e ON e.id = er.source_id
    WHERE d.depth < 5
      AND NOT e.id = ANY(d.visited)
)
SELECT type_key, COUNT(*) AS affected_count,
       ARRAY_AGG(name ORDER BY depth) AS affected_entities
FROM downstream
WHERE depth > 0
GROUP BY type_key;

-- 4. Vector similarity search — "Find entities similar to this one"
SELECT 
    e.id, e.name, e.type_key, e.metadata,
    1 - (e.embedding <=> $1) AS similarity
FROM entities e
WHERE e.tenant_id = $2
  AND e.id != $3  -- Exclude self
ORDER BY e.embedding <=> $1
LIMIT 20;
```

---

## 3. Dynamic Entity Type Registration

### 3.1 Plugin-Driven Type Registration

When a plugin starts, it registers its entity types and relationship types:

```go
// Plugin declares its entity types during Init
func (p *JiraPlugin) RegisterEntityTypes() []sdk.EntityTypeRegistration {
    return []sdk.EntityTypeRegistration{
        {
            TypeKey:     "jira_issue",
            DisplayName: "Jira Issue",
            Category:    "incident",
            MetadataSchema: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "issue_key": {"type": "string"},
                    "issue_type": {"type": "string", "enum": ["bug", "story", "task", "epic"]},
                    "priority": {"type": "string", "enum": ["critical", "high", "medium", "low"]},
                    "sprint": {"type": "string"},
                    "story_points": {"type": "number"},
                    "assignee": {"type": "string"},
                    "resolution": {"type": "string"}
                },
                "required": ["issue_key", "issue_type"]
            }`),
            UIConfig: sdk.UIConfig{
                DisplayColumns: []string{"issue_key", "issue_type", "priority", "status"},
                Color:          "#0052CC",
                Icon:           "jira-icon.svg",
            },
        },
        {
            TypeKey:     "jira_sprint",
            DisplayName: "Jira Sprint",
            Category:    "planning",
            MetadataSchema: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "state": {"type": "string", "enum": ["active", "closed", "future"]},
                    "start_date": {"type": "string", "format": "date"},
                    "end_date": {"type": "string", "format": "date"},
                    "velocity": {"type": "number"}
                }
            }`),
        },
    }
}

// Plugin declares relationship types
func (p *JiraPlugin) RegisterRelationshipTypes() []sdk.RelationshipTypeRegistration {
    return []sdk.RelationshipTypeRegistration{
        {
            TypeKey:       "blocked_by",
            DisplayName:   "Blocked By",
            SourceTypes:   []string{"jira_issue"},
            TargetTypes:   []string{"jira_issue"},
            Cardinality:   "many_to_many",
            DisplayColor:  "#DC2626",
            IsDirectional: true,
        },
        {
            TypeKey:       "fixes",
            DisplayName:   "Fixes",
            SourceTypes:   []string{"git_commit", "git_pull_request"},
            TargetTypes:   []string{"jira_issue"},
            Cardinality:   "many_to_many",
            DisplayColor:  "#059669",
            IsDirectional: true,
        },
    }
}
```

### 3.2 Custom Entity Types (User-Defined)

Administrators can also define custom entity types through the UI or YAML:

```yaml
# custom-entity-type.yaml
apiVersion: pepa.github.io/v1alpha1
kind: EntityType
metadata:
  name: database-instance
spec:
  displayName: "Database Instance"
  category: "data"
  metadataSchema:
    type: object
    properties:
      engine:
        type: string
        enum: [postgres, mysql, mongodb, redis, dynamodb]
      version:
        type: string
      instance_class:
        type: string
      storage_gb:
        type: number
      backup_enabled:
        type: boolean
      replication_factor:
        type: integer
    required:
      - engine
      - version
  uiConfig:
    displayColumns: [engine, version, instance_class, storage_gb]
    color: "#7C3AED"
    icon: "database"
  relationships:
    - type: "stores_data_for"
      target: service
      cardinality: many_to_many
    - type: "runs_in"
      target: environment
      cardinality: many_to_one
```

---

## 4. Entity Sync Engine

### 4.1 Bidirectional Sync Architecture

```
┌────────────────────────────────────────────────────────────────────┐
│                       ENTITY SYNC ENGINE                            │
│                                                                     │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────────┐ │
│  │ Plugin Event │    │ Sync         │    │ Entity Graph         │ │
│  │ Listener     │───▶│ Transformer  │───▶│ Writer               │ │
│  │              │    │              │    │                      │ │
│  │ - Webhooks   │    │ - Normalize  │    │ - Upsert entities    │ │
│  │ - Polling    │    │ - Map types  │    │ - Create/update      │ │
│  │ - Streaming  │    │ - Resolve    │    │   relationships      │ │
│  │              │    │   references │    │ - Update embeddings  │ │
│  └──────────────┘    └──────────────┘    └──────────────────────┘ │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Conflict Resolution Strategies                                │  │
│  │                                                               │  │
│  │  1. Plugin-Wins: External system is source of truth          │  │
│  │  2. Portal-Wins: Portal overrides external data              │  │
│  │  3. Last-Write-Wins: Timestamp-based resolution              │  │
│  │  4. Manual: Flag conflict for human resolution               │  │
│  └──────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────┘
```

### 4.2 Sync Configuration

```yaml
# sync-config.yaml — Per-plugin sync settings
apiVersion: pepa.github.io/v1alpha1
kind: EntitySyncConfig
metadata:
  name: jira-sync
spec:
  pluginName: jira-tracker
  
  # Sync mode
  mode: bidirectional  # unidirectional | bidirectional
  
  # Polling fallback (when webhooks are unavailable)
  polling:
    enabled: true
    interval: 5m
    batchSize: 100
  
  # Webhook configuration
  webhooks:
    enabled: true
    events:
      - issue.created
      - issue.updated
      - issue.deleted
      - issue.transitioned
  
  # Field mapping (external → portal)
  fieldMapping:
    - externalField: "fields.summary"
      portalField: "name"
    - externalField: "fields.status.name"
      portalField: "status"
    - externalField: "fields.priority.name"
      portalField: "metadata.priority"
    - externalField: "fields.assignee.displayName"
      portalField: "metadata.assignee"
  
  # Auto-relationship discovery
  autoRelationships:
    # Link issues to services based on label convention
    - rule: "entity.metadata.labels contains prefix 'service:'"
      relationship: "has_issue"
      targetExtractor: "label['service:']"
    
    # Link issues to sprints
    - rule: "entity.metadata.sprint is not null"
      relationship: "in_sprint"
      targetExtractor: "metadata.sprint"
  
  # Embedding generation for RAG
  embedding:
    enabled: true
    fields: ["name", "description", "metadata.summary"]
    model: "text-embedding-3-small"  # or any configured model
```

---

## 5. Entity Graph API (GraphQL)

### 5.1 Schema

```graphql
type Query {
  # Entity queries
  entity(id: ID!): Entity
  entities(
    type: String
    filter: EntityFilter
    first: Int = 20
    after: String
    orderBy: EntityOrder
  ): EntityConnection!
  
  # Graph queries
  entityGraph(
    rootId: ID!
    depth: Int = 3
    relationshipTypes: [String]
    direction: GraphDirection = BOTH
  ): GraphResult!
  
  # Impact analysis
  impactAnalysis(
    entityId: ID!
    maxDepth: Int = 5
    relationshipTypes: [String]
  ): ImpactResult!
  
  # Similarity search
  similarEntities(
    entityId: ID
    vector: [Float]
    type: String
    limit: Int = 20
  ): [SimilarityResult!]!
  
  # Type registry
  entityTypes: [EntityType!]!
  relationshipTypes: [RelationshipType!]!
}

type Mutation {
  # Entity CRUD
  createEntity(input: CreateEntityInput!): Entity!
  updateEntity(id: ID!, input: UpdateEntityInput!): Entity!
  deleteEntity(id: ID!): Boolean!
  
  # Relationships
  createRelationship(input: CreateRelationshipInput!): Relationship!
  deleteRelationship(id: ID!): Boolean!
  
  # Bulk operations
  bulkCreateRelationships(input: [CreateRelationshipInput!]!): [Relationship!]!
  
  # Entity type management
  registerEntityType(input: RegisterEntityTypeInput!): EntityType!
  registerRelationshipType(input: RegisterRelationshipTypeInput!): RelationshipType!
}

type Subscription {
  # Real-time entity events
  entityChanged(type: String, tenantId: ID!): EntityEvent!
  relationshipChanged(tenantId: ID!): RelationshipEvent!
}

# ============================================================
# Types
# ============================================================

type Entity {
  id: ID!
  type: EntityType!
  name: String!
  description: String
  externalId: String
  metadata: JSON!
  status: String!
  
  # Relationships (lazy-loaded)
  relationships(
    direction: GraphDirection
    types: [String]
    first: Int = 50
  ): [RelationshipEdge!]!
  
  # Graph traversal helpers
  upstream(depth: Int = 1): [Entity!]!
  downstream(depth: Int = 1): [Entity!]!
  
  # Audit
  createdBy: User
  createdAt: DateTime!
  updatedAt: DateTime!
}

type EntityType {
  typeKey: String!
  displayName: String!
  category: String
  metadataSchema: JSON!
  uiConfig: UIConfig
  entityCount: Int!
  relationships: [RelationshipType!]!
}

type Relationship {
  id: ID!
  type: RelationshipType!
  source: Entity!
  target: Entity!
  metadata: JSON!
  createdAt: DateTime!
}

type RelationshipEdge {
  relationship: Relationship!
  node: Entity!
  direction: GraphDirection!
}

type GraphResult {
  nodes: [Entity!]!
  edges: [RelationshipEdge!]!
  rootId: ID!
}

type ImpactResult {
  affectedEntities: [Entity!]!
  affectedCount: Int!
  groupedByType: [ImpactGroup!]!
  severity: ImpactSeverity!
}

enum GraphDirection { UPSTREAM, DOWNSTREAM, BOTH }
enum ImpactSeverity { LOW, MEDIUM, HIGH, CRITICAL }
```

---

## 6. Data Model for Supporting Tables

### 6.1 Multi-Tenancy Tables

```sql
-- Organizations (top-level tenant)
CREATE TABLE organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(256) NOT NULL,
    slug        VARCHAR(128) NOT NULL UNIQUE,
    settings    JSONB DEFAULT '{}',
    plan        VARCHAR(32) DEFAULT 'community',  -- community, enterprise
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Tenants (sub-organizations, departments, or teams within an org)
CREATE TABLE tenants (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    name            VARCHAR(256) NOT NULL,
    slug            VARCHAR(128) NOT NULL,
    settings        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, slug)
);
```

### 6.2 Audit Trail

```sql
-- Immutable audit log for all entity operations
CREATE TABLE audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    
    -- Actor
    user_id     UUID,
    api_key_id  UUID,
    plugin_name VARCHAR(128),
    
    -- Action
    action      VARCHAR(32) NOT NULL,  -- create, update, delete, read, login, permission_change
    entity_type VARCHAR(128),
    entity_id   UUID,
    
    -- Change data
    old_values  JSONB,
    new_values  JSONB,
    diff        JSONB,  -- Computed diff
    
    -- Context
    ip_address  INET,
    user_agent  TEXT,
    request_id  UUID,
    
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_audit_tenant ON audit_log(tenant_id, created_at DESC);
CREATE INDEX idx_audit_entity ON audit_log(entity_type, entity_id);
CREATE INDEX idx_audit_user ON audit_log(user_id, created_at DESC);
```

### 6.3 Plugin State Storage

```sql
-- Plugins can store state in the core database
CREATE TABLE plugin_state (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plugin_name VARCHAR(128) NOT NULL,
    tenant_id   UUID NOT NULL,
    
    -- Key-value state (scoped per plugin per tenant)
    state_key   VARCHAR(256) NOT NULL,
    state_value JSONB NOT NULL,
    
    -- Expiry (optional)
    expires_at  TIMESTAMPTZ,
    
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    
    UNIQUE (plugin_name, tenant_id, state_key)
);
```

---

## 7. Redis Data Structures

```
# ============================================================
# Redis Key Schema
# ============================================================

# Entity cache (TTL: 5min)
entity:{tenant_id}:{entity_id}           → JSON serialized entity
entity:{tenant_id}:type:{type_key}:list   → Sorted set (score = updated_at)

# Graph adjacency cache (TTL: 1min for hot paths)
graph:{tenant_id}:{entity_id}:outgoing    → Hash {relationship_type → [target_ids]}
graph:{tenant_id}:{entity_id}:incoming    → Hash {relationship_type → [source_ids]}

# Real-time event pub/sub channels
channel:tenant:{tenant_id}:entities       → Entity change events
channel:tenant:{tenant_id}:workflows      → Workflow execution events
channel:tenant:{tenant_id}:plugins        → Plugin lifecycle events
channel:global:plugins                    → Global plugin events

# Session store
session:{session_id}                      → User session data (TTL: 24h)

# Rate limiting
ratelimit:{user_id}:{endpoint}            → Token bucket counter (TTL: 1min)

# Workflow execution state
workflow:{workflow_id}:state              → Current execution state
workflow:{workflow_id}:step:{step_id}     → Step execution result
```

---

## 8. Entity Resolution & Cross-Plugin Linking

### 8.1 Auto-Linking Rules Engine

```yaml
# auto-link-rules.yaml
# Rules for automatically discovering and creating relationships
apiVersion: pepa.github.io/v1alpha1
kind: AutoLinkRule
metadata:
  name: service-to-repository
spec:
  # When a new git_repository entity is created
  trigger:
    entityType: git_repository
    event: created
  
  # Find a service with matching annotation
  match:
    entityType: service
    condition:
      # Match if service.metadata.repository_url equals repo.metadata.clone_url
      - field: "metadata.repository_url"
        operator: "equals"
        compareWith: "$.trigger.metadata.clone_url"
      
      # OR match by naming convention
      - field: "name"
        operator: "equals"
        compareWith: "$.trigger.metadata.service_name"
  
  # Create the relationship
  action:
    relationshipType: has_repository
    direction: source_to_target  # service → repository
    metadata:
      auto_linked: true
      link_method: "auto_link_rule"
```

### 8.2 Entity Reconciliation

When multiple plugins report the same entity (e.g., both ArgoCD and Prometheus know about a Deployment):

```sql
-- Entity reconciliation view — merges data from multiple sources
CREATE MATERIALIZED VIEW entity_reconciled AS
SELECT 
    COALESCE(e1.id, e2.id) AS entity_id,
    e1.type_key,
    e1.name,
    -- Merge metadata from all sources (latest write wins per field)
    jsonb_strip_nulls(
        COALESCE(e1.metadata, '{}') || 
        COALESCE(e2.metadata, '{}')
    ) AS merged_metadata,
    -- Track which plugins contributed data
    ARRAY_REMOVE(ARRAY[e1.plugin_name, e2.plugin_name], NULL) AS source_plugins,
    -- Latest sync timestamp
    GREATEST(e1.last_synced_at, e2.last_synced_at) AS last_synced_at
FROM entities e1
FULL OUTER JOIN entities e2 
    ON e1.type_key = e2.type_key 
    AND e1.external_id = e2.external_id
    AND e1.tenant_id = e2.tenant_id
    AND e1.plugin_name != e2.plugin_name
WHERE e1.deleted_at IS NULL OR e2.deleted_at IS NULL;
```
