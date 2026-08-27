-- ============================================================
-- Migration 001: Initial Schema
-- ============================================================
-- Extensions, organizations, tenants, users, teams, RBAC,
-- entity types, entities, relationship types, entity relationships.
-- ============================================================

-- ── Extensions ───────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "vector";       -- PGvector for AI embeddings
CREATE EXTENSION IF NOT EXISTS "pg_trgm";      -- Trigram for text search

-- ============================================================
-- ORGANIZATIONS & TENANTS
-- ============================================================

CREATE TABLE IF NOT EXISTS organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(256) NOT NULL,
    slug        VARCHAR(128) NOT NULL UNIQUE,
    settings    JSONB DEFAULT '{}',
    plan        VARCHAR(32) DEFAULT 'community',
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tenants (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    name            VARCHAR(256) NOT NULL,
    slug            VARCHAR(128) NOT NULL,
    settings        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (organization_id, slug)
);

-- Default organization and tenant
INSERT INTO organizations (id, name, slug) VALUES
    ('00000000-0000-0000-0000-000000000001', 'Default Organization', 'default')
ON CONFLICT DO NOTHING;

INSERT INTO tenants (id, organization_id, name, slug) VALUES
    ('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 'Default Tenant', 'default')
ON CONFLICT DO NOTHING;

-- ============================================================
-- USERS & AUTHENTICATION
-- ============================================================

CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           VARCHAR(256) NOT NULL UNIQUE,
    name            VARCHAR(256) NOT NULL,
    avatar_url      VARCHAR(512),
    auth_provider   VARCHAR(32),
    external_id     VARCHAR(256),
    is_active       BOOLEAN DEFAULT TRUE,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(256) NOT NULL,
    key_hash    VARCHAR(512) NOT NULL,
    key_prefix  VARCHAR(16) NOT NULL,
    tenant_id   UUID NOT NULL,
    created_by  UUID REFERENCES users(id),
    expires_at  TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    is_active   BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================
-- TEAMS
-- ============================================================

CREATE TABLE IF NOT EXISTS teams (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    name            VARCHAR(256) NOT NULL,
    slug            VARCHAR(128) NOT NULL,
    description     TEXT,
    parent_team_id  UUID REFERENCES teams(id),
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

CREATE TABLE IF NOT EXISTS team_memberships (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id     UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        VARCHAR(64) NOT NULL DEFAULT 'member',
    joined_at   TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (team_id, user_id)
);

-- ============================================================
-- RBAC — ROLES & PERMISSIONS
-- ============================================================

CREATE TABLE IF NOT EXISTS roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    name        VARCHAR(128) NOT NULL,
    slug        VARCHAR(128) NOT NULL,
    description TEXT,
    is_system   BOOLEAN DEFAULT FALSE,
    scope       VARCHAR(32) DEFAULT 'tenant',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

CREATE TABLE IF NOT EXISTS permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id     UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    resource    VARCHAR(128) NOT NULL,
    action      VARCHAR(64) NOT NULL,
    effect      VARCHAR(16) DEFAULT 'allow',
    conditions  JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS role_assignments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    user_id     UUID REFERENCES users(id),
    team_id     UUID REFERENCES teams(id),
    role_id     UUID NOT NULL REFERENCES roles(id),
    scope_type  VARCHAR(32),
    scope_value VARCHAR(256),
    granted_by  UUID REFERENCES users(id),
    expires_at  TIMESTAMPTZ,
    is_active   BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_role_assign_user ON role_assignments(user_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_role_assign_team ON role_assignments(team_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_permissions_role ON permissions(role_id);

-- ============================================================
-- ENTITY TYPES REGISTRY
-- ============================================================

CREATE TABLE IF NOT EXISTS entity_types (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type_key        VARCHAR(128) NOT NULL UNIQUE,
    display_name    VARCHAR(256) NOT NULL,
    description     TEXT,
    plugin_name     VARCHAR(128),
    icon            VARCHAR(256),
    category        VARCHAR(64),
    metadata_schema JSONB NOT NULL DEFAULT '{}',
    ui_config       JSONB DEFAULT '{}',
    is_system       BOOLEAN DEFAULT FALSE,
    is_enabled      BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Core entity types
INSERT INTO entity_types (type_key, display_name, category, is_system) VALUES
    ('service',       'Service',          'compute',      TRUE),
    ('team',          'Team',             'organization', TRUE),
    ('environment',   'Environment',      'deployment',   TRUE),
    ('api_endpoint',  'API Endpoint',     'interface',    TRUE)
ON CONFLICT (type_key) DO NOTHING;

-- ============================================================
-- ENTITIES (Universal Entity Store)
-- ============================================================

CREATE TABLE IF NOT EXISTS entities (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type_id         UUID NOT NULL REFERENCES entity_types(id),
    type_key        VARCHAR(128) NOT NULL,
    name            VARCHAR(512) NOT NULL,
    description     TEXT,
    external_id     VARCHAR(512),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    organization_id UUID NOT NULL REFERENCES organizations(id),
    metadata        JSONB NOT NULL DEFAULT '{}',
    status          VARCHAR(64) DEFAULT 'active',
    status_detail   TEXT,
    plugin_name     VARCHAR(128),
    sync_status     VARCHAR(32) DEFAULT 'synced',
    last_synced_at  TIMESTAMPTZ,
    embedding       vector(1536),
    created_by      UUID REFERENCES users(id),
    updated_by      UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (type_key, external_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_entities_type ON entities(type_key, tenant_id);
CREATE INDEX IF NOT EXISTS idx_entities_external ON entities(type_key, external_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_entities_metadata ON entities USING GIN(metadata);
CREATE INDEX IF NOT EXISTS idx_entities_status ON entities(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_entities_search ON entities USING GIN(to_tsvector('english', name || ' ' || COALESCE(description, '')));

-- Row-Level Security
ALTER TABLE entities ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON entities;
CREATE POLICY tenant_isolation ON entities
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- ============================================================
-- RELATIONSHIP TYPES & ENTITY RELATIONSHIPS
-- ============================================================

CREATE TABLE IF NOT EXISTS relationship_types (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type_key        VARCHAR(128) NOT NULL UNIQUE,
    display_name    VARCHAR(256) NOT NULL,
    description     TEXT,
    source_types    VARCHAR(128)[] NOT NULL,
    target_types    VARCHAR(128)[] NOT NULL,
    cardinality     VARCHAR(16) DEFAULT 'many_to_many',
    display_color   VARCHAR(7),
    display_label   VARCHAR(64),
    is_directional  BOOLEAN DEFAULT TRUE,
    plugin_name     VARCHAR(128),
    is_system       BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Core relationship types
INSERT INTO relationship_types (type_key, display_name, source_types, target_types, cardinality, display_color, is_directional, is_system) VALUES
    ('owns',           'Owned By',       ARRAY['*'],       ARRAY['team'],    'many_to_one',  '#3B82F6', TRUE,  TRUE),
    ('depends_on',     'Depends On',     ARRAY['*'],       ARRAY['*'],       'many_to_many', '#EF4444', TRUE,  TRUE),
    ('deployed_to',    'Deployed To',    ARRAY['service'], ARRAY['environment'], 'many_to_many', '#10B981', TRUE, TRUE),
    ('part_of',        'Part Of',        ARRAY['*'],       ARRAY['*'],       'many_to_one',  '#6366F1', TRUE,  TRUE)
ON CONFLICT (type_key) DO NOTHING;

CREATE TABLE IF NOT EXISTS entity_relationships (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_type_id UUID NOT NULL REFERENCES relationship_types(id),
    type_key            VARCHAR(128) NOT NULL,
    source_id           UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    target_id           UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    metadata            JSONB DEFAULT '{}',
    tenant_id           UUID NOT NULL,
    plugin_name         VARCHAR(128),
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (source_id, target_id, relationship_type_id),
    CHECK (source_id != target_id)
);

CREATE INDEX IF NOT EXISTS idx_rel_source ON entity_relationships(source_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_rel_target ON entity_relationships(target_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_rel_type ON entity_relationships(type_key, tenant_id);

ALTER TABLE entity_relationships ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_rel ON entity_relationships;
CREATE POLICY tenant_isolation_rel ON entity_relationships
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (1, 'Initial schema — entities, relationships, RBAC, workflows, RAG, audit')
ON CONFLICT DO NOTHING;
