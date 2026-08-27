# Multi-Tenant RBAC & Custom Dashboards

## 1. Design Philosophy

PEPA implements a **hierarchical, attribute-based access control (ABAC) system** layered on top of role-based access control (RBAC). This hybrid approach provides the simplicity of role assignment with the granularity of attribute-based policies — enabling fine-grained permissions scoped to teams, environments, clusters, and entity types.

### Key Goals

- **Multi-tenant isolation** — data and access boundaries enforced at every layer
- **Hierarchical roles** — organization → tenant → team → individual
- **Attribute-based scoping** — permissions can be scoped by entity type, environment, cluster, or custom attributes
- **Custom dashboards** — role-specific UIs with drag-and-drop widget builder
- **Audit-first** — every permission check and access decision is logged

---

## 2. RBAC Data Model

### 2.1 Core Tables

```sql
-- ============================================================
-- Users & Identity
-- ============================================================
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           VARCHAR(256) NOT NULL UNIQUE,
    name            VARCHAR(256) NOT NULL,
    avatar_url      VARCHAR(512),
    
    -- Authentication
    auth_provider   VARCHAR(32),    -- oidc, saml, github, google
    external_id     VARCHAR(256),   -- Subject ID from IdP
    
    -- Status
    is_active       BOOLEAN DEFAULT TRUE,
    last_login_at   TIMESTAMPTZ,
    
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- API keys for service accounts and automation
CREATE TABLE api_keys (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(256) NOT NULL,
    key_hash    VARCHAR(512) NOT NULL,
    key_prefix  VARCHAR(16) NOT NULL,   -- First 8 chars for display
    
    -- Scope
    tenant_id   UUID NOT NULL,
    created_by  UUID REFERENCES users(id),
    
    -- Lifecycle
    expires_at  TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    is_active   BOOLEAN DEFAULT TRUE,
    
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================
-- Teams & Membership
-- ============================================================
CREATE TABLE teams (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    name            VARCHAR(256) NOT NULL,
    slug            VARCHAR(128) NOT NULL,
    description     TEXT,
    parent_team_id  UUID REFERENCES teams(id),  -- Hierarchical teams
    
    -- Metadata
    metadata        JSONB DEFAULT '{}',
    
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

CREATE TABLE team_memberships (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id     UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        VARCHAR(64) NOT NULL DEFAULT 'member',  -- lead, member, viewer
    joined_at   TIMESTAMPTZ DEFAULT NOW(),
    
    UNIQUE (team_id, user_id)
);

-- ============================================================
-- Roles & Permissions
-- ============================================================
CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    name        VARCHAR(128) NOT NULL,
    slug        VARCHAR(128) NOT NULL,
    description TEXT,
    
    -- Role type
    is_system   BOOLEAN DEFAULT FALSE,  -- Built-in roles cannot be deleted
    scope       VARCHAR(32) DEFAULT 'tenant',  -- org, tenant, team
    
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

-- Permissions (what actions are allowed)
CREATE TABLE permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id     UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    
    -- Permission definition
    resource    VARCHAR(128) NOT NULL,  -- entity, workflow, plugin, dashboard, setting, etc.
    action      VARCHAR(64) NOT NULL,   -- create, read, update, delete, execute, admin
    effect      VARCHAR(16) DEFAULT 'allow',  -- allow | deny
    
    -- Attribute-based scoping (ABAC layer)
    conditions  JSONB DEFAULT '{}',
    -- Examples:
    -- {"entity_type": ["service", "k8s_deployment"]}
    -- {"environment": ["staging", "dev"]}
    -- {"cluster": ["us-east-1"]}
    -- {"plugin_type": ["git_provider"]}
    
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Role assignments (who gets what role, where)
CREATE TABLE role_assignments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    
    -- Subject (who)
    user_id     UUID REFERENCES users(id),
    team_id     UUID REFERENCES teams(id),
    
    -- Role (what)
    role_id     UUID NOT NULL REFERENCES roles(id),
    
    -- Scope (where) — optional restrictions
    scope_type  VARCHAR(32),   -- entity_type, environment, cluster, plugin
    scope_value VARCHAR(256),  -- Specific value within scope_type
    
    -- Lifecycle
    granted_by  UUID REFERENCES users(id),
    expires_at  TIMESTAMPTZ,   -- Time-limited access
    is_active   BOOLEAN DEFAULT TRUE,
    
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_role_assign_user ON role_assignments(user_id, tenant_id);
CREATE INDEX idx_role_assign_team ON role_assignments(team_id, tenant_id);
CREATE INDEX idx_permissions_role ON permissions(role_id);
```

### 2.2 Built-in System Roles

```sql
-- Organization-level roles
INSERT INTO roles (tenant_id, name, slug, is_system, scope) VALUES
    ('00000000-0000-0000-0000-000000000000', 'Organization Admin', 'org-admin', TRUE, 'org'),
    ('00000000-0000-0000-0000-000000000000', 'Organization Member', 'org-member', TRUE, 'org');

-- Tenant-level roles (per-tenant)
-- These are templates; actual roles are created per tenant
INSERT INTO roles (tenant_id, name, slug, is_system, scope, description) VALUES
    ('00000000-0000-0000-0000-000000000000', 'Platform Admin',   'platform-admin',   TRUE, 'tenant', 'Full access to all platform resources'),
    ('00000000-0000-0000-0000-000000000000', 'Developer',        'developer',        TRUE, 'tenant', 'Read services, trigger workflows, view dashboards'),
    ('00000000-0000-0000-0000-000000000000', 'DevOps Engineer',  'devops-engineer',  TRUE, 'tenant', 'Manage deployments, workflows, and infrastructure'),
    ('00000000-0000-0000-0000-000000000000', 'Security Engineer','security-engineer',TRUE, 'tenant', 'View security scans, manage policies, audit logs'),
    ('00000000-0000-0000-0000-000000000000', 'QA Engineer',      'qa-engineer',      TRUE, 'tenant', 'Run tests, view deployments, manage test environments'),
    ('00000000-0000-0000-0000-000000000000', 'Viewer',           'viewer',           TRUE, 'tenant', 'Read-only access to assigned resources'),
    ('00000000-0000-0000-0000-000000000000', 'Team Lead',        'team-lead',        TRUE, 'team',   'Manage team resources, approve deployments');
```

### 2.3 Permission Matrix (Built-in Roles)

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          PERMISSION MATRIX                                       │
├──────────────────────┬──────────┬──────────┬──────────┬──────────┬──────────────┤
│ Resource / Action    │ Platform │ Developer│ DevOps   │ Security │ QA Engineer  │
│                      │ Admin    │          │ Engineer │ Engineer │              │
├──────────────────────┼──────────┼──────────┼──────────┼──────────┼──────────────┤
│ Services             │          │          │          │          │              │
│   ├─ View all        │    ✓     │    ✓*    │    ✓     │    ✓     │    ✓*        │
│   ├─ Create          │    ✓     │    ✗     │    ✓     │    ✗     │    ✗         │
│   ├─ Edit            │    ✓     │    ✗     │    ✓     │    ✗     │    ✗         │
│   └─ Delete          │    ✓     │    ✗     │    ✗     │    ✗     │    ✗         │
├──────────────────────┼──────────┼──────────┼──────────┼──────────┼──────────────┤
│ Workflows            │          │          │          │          │              │
│   ├─ View            │    ✓     │    ✓     │    ✓     │    ✓     │    ✓         │
│   ├─ Create/Edit     │    ✓     │    ✗     │    ✓     │    ✗     │    ✗         │
│   ├─ Execute         │    ✓     │    ✓*    │    ✓     │    ✗     │    ✓*        │
│   └─ Approve         │    ✓     │    ✗     │    ✓     │    ✓     │    ✗         │
├──────────────────────┼──────────┼──────────┼──────────┼──────────┼──────────────┤
│ Plugins              │          │          │          │          │              │
│   ├─ View            │    ✓     │    ✓     │    ✓     │    ✓     │    ✓         │
│   ├─ Install         │    ✓     │    ✗     │    ✓     │    ✗     │    ✗         │
│   ├─ Configure       │    ✓     │    ✗     │    ✓*    │    ✗     │    ✗         │
│   └─ Uninstall       │    ✓     │    ✗     │    ✗     │    ✗     │    ✗         │
├──────────────────────┼──────────┼──────────┼──────────┼──────────┼──────────────┤
│ Dashboards           │          │          │          │          │              │
│   ├─ View own        │    ✓     │    ✓     │    ✓     │    ✓     │    ✓         │
│   ├─ Create/Edit own │    ✓     │    ✓     │    ✓     │    ✓     │    ✓         │
│   ├─ View shared     │    ✓     │    ✓*    │    ✓     │    ✓     │    ✓*        │
│   └─ Edit shared     │    ✓     │    ✗     │    ✓     │    ✗     │    ✗         │
├──────────────────────┼──────────┼──────────┼──────────┼──────────┼──────────────┤
│ Settings             │          │          │          │          │              │
│   ├─ View            │    ✓     │    ✓     │    ✓     │    ✓     │    ✓         │
│   ├─ Edit            │    ✓     │    ✗     │    ✗     │    ✗     │    ✗         │
│   └─ Manage RBAC     │    ✓     │    ✗     │    ✗     │    ✗     │    ✗         │
├──────────────────────┼──────────┼──────────┼──────────┼──────────┼──────────────┤
│ Audit Logs           │          │          │          │          │              │
│   ├─ View own        │    ✓     │    ✓     │    ✓     │    ✓     │    ✓         │
│   ├─ View all        │    ✓     │    ✗     │    ✗     │    ✓     │    ✗         │
│   └─ Export          │    ✓     │    ✗     │    ✗     │    ✓     │    ✗         │
└──────────────────────┴──────────┴──────────┴──────────┴──────────┴──────────────┘

  ✓* = scoped to team or assigned resources only
```

---

## 3. Policy Engine (OPA Integration)

### 3.1 Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                   POLICY ENGINE                                  │
│                                                                   │
│  ┌─────────────────┐    ┌──────────────────────────────────┐   │
│  │ Policy Loader   │    │ OPA Engine (embedded)            │   │
│  │                  │    │                                  │   │
│  │ - Load from DB  │    │  ┌──────────┐  ┌──────────┐    │   │
│  │ - Load from Git │    │  │ Rego     │  │ Decision │    │   │
│  │ - Compile       │    │  │ Policies │  │ Cache    │    │   │
│  │ - Watch changes │    │  └──────────┘  └──────────┘    │   │
│  └─────────────────┘    └──────────────────────────────────┘   │
│                                                                   │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ Decision Points                                           │   │
│  │                                                           │   │
│  │  1. API Request → Authentication → Authorization         │   │
│  │  2. Workflow Trigger → Policy Check → Execute/Skip       │   │
│  │  3. Plugin Action → Permission Check → Allow/Deny        │   │
│  │  4. Entity Access → RLS + OPA → Return Data              │   │
│  │  5. Dashboard Render → Filter by Visible Entities         │   │
│  └──────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────┘
```

### 3.2 Rego Policies

```rego
# policies/rbac/authz.rego
package pepa.authz

import future.keywords.if
import future.keywords.in

# Default deny
default allow := false

# ── Platform Admin: full access ──────────────────────────────
allow if {
    input.user.has_role("platform-admin", input.tenant_id)
}

# ── Service access ──────────────────────────────────────────
allow if {
    input.action in {"read", "list"}
    input.resource == "service"
    input.user.has_role_with_scope("developer", input.tenant_id)
}

allow if {
    input.action in {"create", "update"}
    input.resource == "service"
    input.user.has_role_with_scope("devops-engineer", input.tenant_id)
}

allow if {
    input.action == "delete"
    input.resource == "service"
    input.user.has_role("platform-admin", input.tenant_id)
}

# ── Workflow execution ──────────────────────────────────────
allow if {
    input.action == "execute"
    input.resource == "workflow"
    input.user.has_role_with_scope("developer", input.tenant_id)
    # Developer can only execute workflows tagged with their team
    workflow_belongs_to_team(input.entity, input.user.teams)
}

allow if {
    input.action == "execute"
    input.resource == "workflow"
    input.user.has_role_with_scope("devops-engineer", input.tenant_id)
}

# ── Approval gates ──────────────────────────────────────────
allow if {
    input.action == "approve"
    input.resource == "workflow"
    is_approver(input.user, input.approval_request)
}

is_approver(user, request) if {
    user.id in request.approvers.users
}

is_approver(user, request) if {
    some team_id in request.approvers.teams
    user.teams[_].id == team_id
    user.teams[_].role in ["lead", "member"]
}

is_approver(user, request) if {
    some role in request.approvers.roles
    user.has_role(role, input.tenant_id)
}

# ── Environment-scoped permissions ──────────────────────────
allow if {
    input.action == "deploy"
    input.resource == "environment"
    input.user.has_role_with_scope("qa-engineer", input.tenant_id)
    # QA can only deploy to test environments
    input.environment.type in ["dev", "staging", "test"]
}

# ── Plugin configuration scoping ────────────────────────────
allow if {
    input.action == "configure"
    input.resource == "plugin"
    input.user.has_role_with_scope("devops-engineer", input.tenant_id)
    # DevOps can configure non-security plugins
    not input.plugin.type in ["secret_manager", "identity"]
}

# ── Dashboard access ────────────────────────────────────────
allow if {
    input.action == "read"
    input.resource == "dashboard"
    # Users can read their own dashboards
    input.dashboard.owner_id == input.user.id
}

allow if {
    input.action == "read"
    input.resource == "dashboard"
    # Users can read dashboards shared with their team
    input.dashboard.shared_with_teams[_] in input.user.team_ids
}

# ── Helper functions ────────────────────────────────────────
workflow_belongs_to_team(workflow, user_teams) if {
    some team in workflow.labels.team
    user_teams[_].slug == team
}
```

### 3.3 Deployment Policy Example

```rego
# policies/deployment/gate.rego
package pepa.deployment

import future.keywords.if

# Deployment policy: production deployments require
# 1. Successful staging verification
# 2. No critical security vulnerabilities
# 3. Within deployment window
# 4. Approved by at least 2 approvers

allow_production_deploy if {
    input.environment == "production"
    staging_verified
    no_critical_vulnerabilities
    within_deploy_window
    has_sufficient_approvals
}

staging_verified if {
    input.staging_verification.status == "passed"
    input.staging_verification.timestamp > time.now_ns() - 24 * 3600 * 1e9
}

no_critical_vulnerabilities if {
    count([v | v := input.security_scan.vulnerabilities[_]; v.severity == "critical"]) == 0
}

within_deploy_window if {
    hour := time.clock(time.now_ns())[0]
    weekday := time.weekday(time.now_ns())
    hour >= 6
    hour <= 20
    weekday != "saturday"
    weekday != "sunday"
}

has_sufficient_approvals if {
    approved := [a | a := input.approvals[_]; a.status == "approved"]
    count(approved) >= 2
}
```

---

## 4. Authentication Flow

### 4.1 SSO Integration

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│ Browser  │     │ PEPA│    │ IdP      │     │ Backend  │
│ (Next.js)│     │ Frontend │     │ (OIDC/   │     │ (Go API) │
│          │     │          │     │  SAML)   │     │          │
└────┬─────┘     └────┬─────┘     └────┬─────┘     └────┬─────┘
     │                │                │                │
     │ 1. Click Login │                │                │
     │───────────────▶│                │                │
     │                │                │                │
     │ 2. Redirect to IdP              │                │
     │◀───────────────│                │                │
     │                │                │                │
     │ 3. Authenticate with IdP        │                │
     │────────────────────────────────▶│                │
     │                │                │                │
     │ 4. Redirect back with code      │                │
     │◀───────────────────────────────│                │
     │                │                │                │
     │ 5. Exchange code for token      │                │
     │───────────────▶│                │                │
     │                │ 6. Validate & exchange          │
     │                │───────────────▶│                │
     │                │ 7. ID Token + Access Token      │
     │                │◀───────────────│                │
     │                │                │                │
     │                │ 8. Create session, resolve roles│
     │                │───────────────────────────────▶│
     │                │ 9. Session token + user context │
     │                │◀───────────────────────────────│
     │                │                │                │
     │ 10. Set session cookie         │                │
     │◀───────────────│                │                │
     │                │                │                │
```

### 4.2 Supported Authentication Providers

```yaml
# auth-config.yaml
apiVersion: pepa.github.io/v1alpha1
kind: AuthConfig
metadata:
  name: default
spec:
  providers:
    # OIDC (Keycloak, Auth0, Okta, etc.)
    - type: oidc
      name: corporate-sso
      config:
        issuer: "https://sso.company.com/realms/platform"
        clientID: "pepa"
        clientSecret: "ref:vault://oidc/client-secret"
        scopes: ["openid", "profile", "email", "groups"]
        # Auto-map IdP groups to PEPA teams
        groupMapping:
          "platform-team": "team:platform"
          "sre-team": "team:sre"
          "security-team": "team:security"
      
    # GitHub OAuth
    - type: github
      name: github-oauth
      config:
        clientID: "ref:vault://github/oauth-client-id"
        clientSecret: "ref:vault://github/oauth-client-secret"
        org: "mycompany"  # Restrict to organization members
      
    # SAML (for enterprise IdPs)
    - type: saml
      name: enterprise-saml
      config:
        idpMetadataURL: "https://idp.company.com/saml/metadata"
        entityID: "pepa"
        attributeMapping:
          email: "urn:oid:0.9.2342.19200300.100.1.3"
          name: "urn:oid:2.5.4.3"
          groups: "memberOf"
  
  # Session configuration
  session:
    duration: 24h
    refreshEnabled: true
    refreshDuration: 7d
    cookieSecure: true
    cookieSameSite: lax
  
  # RBAC auto-provisioning from IdP claims
  autoProvisioning:
    enabled: true
    rules:
      - claim: "groups"
        value: "platform-admins"
        role: "platform-admin"
      - claim: "groups"
        value: "developers"
        role: "developer"
      - claim: "department"
        value: "engineering"
        role: "developer"
```

---

## 5. Custom Dashboard System

### 5.1 Dashboard Data Model

```sql
-- Dashboard definitions
CREATE TABLE dashboards (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    
    -- Identity
    name        VARCHAR(256) NOT NULL,
    slug        VARCHAR(128) NOT NULL,
    description TEXT,
    
    -- Ownership & sharing
    owner_id    UUID NOT NULL REFERENCES users(id),
    is_public   BOOLEAN DEFAULT FALSE,        -- Visible to entire tenant
    shared_with JSONB DEFAULT '[]',           -- Team IDs or user IDs
    
    -- Layout (React Grid Layout configuration)
    layout      JSONB NOT NULL DEFAULT '{}',
    -- Format: {
    --   "lg": [{"i":"w1","x":0,"y":0,"w":6,"h":4}, ...],
    --   "md": [{"i":"w1","x":0,"y":0,"w":4,"h":3}, ...],
    --   "sm": [{"i":"w1","x":0,"y":0,"w":12,"h":6}, ...]
    -- }
    
    -- Settings
    settings    JSONB DEFAULT '{}',
    -- {
    --   "refreshInterval": 30,
    --   "theme": "auto",
    --   "timeRange": {"from":"now-24h","to":"now"},
    --   "variables": [{"name":"env","type":"dropdown","options":["staging","production"]}]
    -- }
    
    -- Template (if created from template)
    template_id UUID,
    
    is_system   BOOLEAN DEFAULT FALSE,  -- Built-in dashboards
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    
    UNIQUE (tenant_id, slug)
);

-- Dashboard widgets
CREATE TABLE dashboard_widgets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dashboard_id UUID NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
    
    -- Widget type and configuration
    widget_type VARCHAR(64) NOT NULL,
    title       VARCHAR(256),
    
    -- Widget-specific configuration
    config      JSONB NOT NULL DEFAULT '{}',
    
    -- Position (overridden by dashboard layout, but stored for reference)
    position    JSONB DEFAULT '{}',
    -- {"x":0,"y":0,"w":6,"h":4,"minW":2,"minH":2}
    
    -- Data source
    data_source JSONB NOT NULL,
    -- {
    --   "type": "graphql" | "rest" | "entity_query" | "metric" | "static",
    --   "query": "...",
    --   "params": {},
    --   "refreshInterval": 30
    -- }
    
    sort_order  INTEGER DEFAULT 0,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);
```

### 5.2 Widget Types

```typescript
// Widget type registry
const WIDGET_TYPES = {
  // ── Data Display ──────────────────────────────────────
  'entity_list': {
    name: 'Entity List',
    description: 'Display entities in a table or card view',
    config: {
      entityType: string,      // Which entity type to display
      filters: EntityFilter,   // Filter criteria
      columns: string[],       // Which metadata fields to show
      viewMode: 'table' | 'cards' | 'list',
      sortBy: string,
      limit: number,
    },
  },
  
  'entity_graph': {
    name: 'Entity Graph',
    description: 'Visual graph of entity relationships',
    config: {
      rootEntityId: string,
      depth: number,
      relationshipTypes: string[],
      layout: 'force' | 'hierarchical' | 'radial',
    },
  },
  
  'metric_chart': {
    name: 'Metric Chart',
    description: 'Time-series chart from monitoring plugin',
    config: {
      pluginName: string,      // monitoring:prometheus, etc.
      query: string,           // PromQL or equivalent
      chartType: 'line' | 'area' | 'bar' | 'gauge',
      timeRange: TimeRange,
      thresholds: Threshold[],
    },
  },
  
  'deployment_status': {
    name: 'Deployment Status',
    description: 'Current deployment status for services',
    config: {
      services: string[],      // Service names to track
      environments: string[],  // Environments to show
      showHistory: boolean,
    },
  },
  
  // ── Workflow ──────────────────────────────────────────
  'workflow_pipeline': {
    name: 'Pipeline View',
    description: 'Visual pipeline status (like CI/CD view)',
    config: {
      workflowIds: string[],
      showSteps: boolean,
      autoRefresh: number,
    },
  },
  
  'recent_executions': {
    name: 'Recent Executions',
    description: 'List of recent workflow executions',
    config: {
      workflowId?: string,     // Optional filter
      limit: number,
      showStatus: boolean,
    },
  },
  
  // ── Issue Tracking ────────────────────────────────────
  'issue_board': {
    name: 'Issue Board',
    description: 'Kanban-style issue board',
    config: {
      pluginName: string,      // task_tracker:jira, etc.
      projectKey: string,
      columns: string[],       // Status columns
    },
  },
  
  'issue_summary': {
    name: 'Issue Summary',
    description: 'Issue statistics and trends',
    config: {
      pluginName: string,
      projectKey: string,
      chartType: 'burndown' | 'velocity' | 'cumulative',
      sprintFilter: 'current' | 'last_5',
    },
  },
  
  // ── Alerts & Incidents ────────────────────────────────
  'alert_list': {
    name: 'Active Alerts',
    description: 'Current active alerts from monitoring',
    config: {
      pluginName: string,
      severityFilter: string[],
      maxAge: string,
    },
  },
  
  // ── Custom ────────────────────────────────────────────
  'markdown': {
    name: 'Markdown',
    description: 'Free-form markdown content',
    config: {
      content: string,
    },
  },
  
  'iframe': {
    name: 'Embedded Page',
    description: 'Embed external content',
    config: {
      url: string,
      height: number,
    },
  },
  
  'scorecard': {
    name: 'Service Scorecard',
    description: 'Maturity score for services',
    config: {
      scorecardId: string,
      services: string[],
    },
  },
} as const;
```

### 5.3 Role-Based Default Dashboards

```yaml
# Default dashboards per role (auto-created for new users)
apiVersion: pepa.github.io/v1alpha1
kind: DashboardTemplate
metadata:
  name: developer-home
  role: developer
spec:
  displayName: "Developer Home"
  description: "Default dashboard for developers"
  layout:
    lg:
      - {i: "my-services", x: 0, y: 0, w: 8, h: 6}
      - {i: "my-issues", x: 8, y: 0, w: 4, h: 6}
      - {i: "recent-deploys", x: 0, y: 6, w: 6, h: 4}
      - {i: "team-alerts", x: 6, y: 6, w: 6, h: 4}
  widgets:
    - id: my-services
      type: entity_list
      title: "My Services"
      config:
        entityType: service
        filters:
          "metadata.owner_team": "{{.user.teams[*].slug}}"
        columns: [name, status, metadata.current_version]
        viewMode: cards
      dataSource:
        type: graphql
        query: "entities(type: 'service', filter: {ownerTeam: {in: $userTeams}})"
    
    - id: my-issues
      type: issue_summary
      title: "My Issues"
      config:
        pluginName: "{{.tenant.default_task_tracker}}"
        filter:
          assignee: "{{.user.email}}"
          status: [open, in_progress]
    
    - id: recent-deploys
      type: recent_executions
      title: "Recent Deployments"
      config:
        limit: 10
        showStatus: true
    
    - id: team-alerts
      type: alert_list
      title: "Team Alerts"
      config:
        severityFilter: [critical, warning]
        maxAge: 24h

---
apiVersion: pepa.github.io/v1alpha1
kind: DashboardTemplate
metadata:
  name: devops-home
  role: devops-engineer
spec:
  displayName: "DevOps Command Center"
  layout:
    lg:
      - {i: "cluster-health", x: 0, y: 0, w: 12, h: 4}
      - {i: "deploy-pipeline", x: 0, y: 4, w: 8, h: 6}
      - {i: "active-incidents", x: 8, y: 4, w: 4, h: 6}
      - {i: "resource-usage", x: 0, y: 10, w: 6, h: 4}
      - {i: "plugin-status", x: 6, y: 10, w: 6, h: 4}
  widgets:
    - id: cluster-health
      type: metric_chart
      title: "Cluster Health"
      config:
        pluginName: monitoring:prometheus
        queries:
          - query: "sum(kube_pod_status_phase{phase='Running'})"
            label: "Running Pods"
          - query: "sum(kube_node_status_condition{condition='Ready',status='true'})"
            label: "Ready Nodes"
        chartType: area
    
    - id: deploy-pipeline
      type: workflow_pipeline
      title: "Deployment Pipelines"
      config:
        showSteps: true
        autoRefresh: 15
    
    - id: active-incidents
      type: alert_list
      title: "Active Incidents"
      config:
        severityFilter: [critical, warning, info]
        maxAge: 72h

---
apiVersion: pepa.github.io/v1alpha1
kind: DashboardTemplate
metadata:
  name: security-home
  role: security-engineer
spec:
  displayName: "Security Overview"
  layout:
    lg:
      - {i: "vuln-summary", x: 0, y: 0, w: 6, h: 4}
      - {i: "compliance-score", x: 6, y: 0, w: 6, h: 4}
      - {i: "audit-log", x: 0, y: 4, w: 12, h: 6}
      - {i: "secret-rotation", x: 0, y: 10, w: 6, h: 4}
      - {i: "policy-violations", x: 6, y: 10, w: 6, h: 4}
  widgets:
    - id: vuln-summary
      type: metric_chart
      title: "Vulnerability Summary"
      config:
        chartType: bar
        # ...
    
    - id: compliance-score
      type: scorecard
      title: "Compliance Score"
      config:
        scorecardId: security-compliance
```

### 5.4 Dashboard Builder UI

```
┌────────────────────────────────────────────────────────────────────────┐
│  Dashboard: "My DevOps View"                    [Edit] [Share] [⋮]    │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  ┌──────────────────────────────────────────────────────────────────┐ │
│  │ Cluster Health                                              [⋮] │ │
│  │                                                                  │ │
│  │  ┌─────────────────────────────────────────────────────────┐    │ │
│  │  │  ╭──────╮  ╭──────╮  ╭──────╮  ╭──────╮  ╭──────╮     │    │ │
│  │  │  │██████│  │██████│  │██████│  │██████│  │██████│     │    │ │
│  │  │  │██████│  │██████│  │██████│  │██████│  │██████│     │    │ │
│  │  │  │██████│  │██████│  │██████│  │██████│  │██████│     │    │ │
│  │  │  ╰──────╯  ╰──────╯  ╰──────╯  ╰──────╯  ╰──────╯     │    │ │
│  │  │  Running Pods    Ready Nodes    CPU Usage   Memory      │    │ │
│  │  └─────────────────────────────────────────────────────────┘    │ │
│  └──────────────────────────────────────────────────────────────────┘ │
│                                                                        │
│  ┌──────────────────────────────────────┐ ┌──────────────────────────┐│
│  │ Deployment Pipeline              [⋮]│ │ Active Incidents     [⋮] ││
│  │                                      │ │                          ││
│  │ ┌──────┐  ┌──────┐  ┌──────┐       │ │ 🔴 Critical: 2          ││
│  │ │ Build │─▶│ Test │─▶│Deploy│       │ │ 🟡 Warning:  5          ││
│  │ │  ✅   │  │  🔄  │  │  ⏳  │       │ │ 🔵 Info:    12          ││
│  │ └──────┘  └──────┘  └──────┘       │ │                          ││
│  │                                      │ │ ┌──────────────────────┐││
│  │ ┌──────┐  ┌──────┐  ┌──────┐       │ │ │ payment-api   🔴 2h │││
│  │ │ Build │─▶│ Test │─▶│Deploy│       │ │ │ auth-service 🟡 30m │││
│  │ │  ✅   │  │  ✅  │  │  ✅  │       │ │ │ user-worker 🟡 1h  │││
│  │ └──────┘  └──────┘  └──────┘       │ │ └──────────────────────┘││
│  └──────────────────────────────────────┘ └──────────────────────────┘│
│                                                                        │
│  ┌──────────────────────────────────────┐ ┌──────────────────────────┐│
│  │ Resource Usage                   [⋮]│ │ Plugin Status         [⋮]││
│  │                                      │ │                          ││
│  │  CPU:    ██████████░░░░░░  62%       │ │ ✅ GitHub      synced   ││
│  │  Memory: ██████████████░░  87%       │ │ ✅ ArgoCD      synced   ││
│  │  Disk:   ██████░░░░░░░░░░  38%       │ │ ⚠️ Jira        stale   ││
│  │  Network:████░░░░░░░░░░░░  24%       │ │ ✅ Prometheus  synced   ││
│  └──────────────────────────────────────┘ └──────────────────────────┘│
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 6. API Authorization Middleware

### 6.1 Go Middleware Chain

```go
package middleware

// Authorization middleware — evaluates OPA policies for each request
func Authorization(policyEngine *opa.Engine) gin.HandlerFunc {
    return func(c *gin.Context) {
        // Extract user context from JWT/session
        user := GetUserFromContext(c)
        if user == nil {
            c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
            return
        }
        
        // Build evaluation input
        input := map[string]interface{}{
            "user":       user.ToOPAInput(),
            "action":     c.Request.Method,  // GET=read, POST=create, etc.
            "resource":   extractResource(c),
            "tenant_id":  getTenantID(c),
            "entity":     getEntityContext(c),
            "params":     c.Params,
        }
        
        // Evaluate policy
        result, err := policyEngine.Evaluate(c.Request.Context(), "pepa.authz", input)
        if err != nil {
            c.AbortWithStatusJSON(500, gin.H{"error": "policy evaluation failed"})
            return
        }
        
        if !result.Allowed {
            // Log the denial for audit
            audit.Log(audit.Event{
                Type:     "access_denied",
                UserID:   user.ID,
                Action:   input["action"].(string),
                Resource: input["resource"].(string),
                Reason:   result.Reason,
            })
            
            c.AbortWithStatusJSON(403, gin.H{
                "error":   "access denied",
                "reason":  result.Reason,
                "requestId": c.GetString("request_id"),
            })
            return
        }
        
        // Set resolved permissions in context for downstream use
        c.Set("permissions", result.Permissions)
        c.Set("tenant_id", input["tenant_id"])
        
        c.Next()
    }
}

// Method-to-action mapping
func mapMethodToAction(method string) string {
    switch method {
    case "GET", "HEAD":
        return "read"
    case "POST":
        return "create"
    case "PUT", "PATCH":
        return "update"
    case "DELETE":
        return "delete"
    default:
        return "unknown"
    }
}
```

---

## 7. Permission Resolution Algorithm

```
┌─────────────────────────────────────────────────────────────────┐
│              PERMISSION RESOLUTION FLOW                           │
│                                                                   │
│  1. Authentication                                                │
│     └─▶ Validate JWT/Session → Extract User + Tenant             │
│                                                                   │
│  2. Role Collection                                               │
│     ├─▶ Direct role assignments (user → role)                    │
│     ├─▶ Team role assignments (user → team → role)               │
│     ├─▶ IdP claim mappings (groups → role)                       │
│     └─▶ Result: Set of all applicable roles                      │
│                                                                   │
│  3. Permission Aggregation                                        │
│     ├─▶ Collect all permissions from all roles                   │
│     ├─▶ Apply scope restrictions (environment, cluster, etc.)    │
│     └─▶ Result: Permission set with scopes                       │
│                                                                   │
│  4. OPA Policy Evaluation                                         │
│     ├─▶ Input: {user, action, resource, scopes, context}         │
│     ├─▶ Evaluate Rego policies                                   │
│     ├─▶ Check deny rules first (deny overrides allow)            │
│     └─▶ Result: allow/deny + reason                              │
│                                                                   │
│  5. Row-Level Security (Database)                                 │
│     ├─▶ Set session variable: app.current_tenant                 │
│     ├─▶ PostgreSQL RLS filters query results                     │
│     └─▶ Result: Only permitted rows returned                     │
│                                                                   │
│  6. Audit                                                         │
│     └─▶ Log every decision (allow/deny) with context             │
└─────────────────────────────────────────────────────────────────┘
```
