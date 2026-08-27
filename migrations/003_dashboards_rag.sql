-- ============================================================
-- Migration 003: Dashboards & RAG (AI)
-- ============================================================
-- Dashboards, widgets, RAG documents, chunks, AI conversations,
-- and messages.
-- ============================================================

-- ============================================================
-- DASHBOARDS
-- ============================================================

CREATE TABLE IF NOT EXISTS dashboards (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    name        VARCHAR(256) NOT NULL,
    slug        VARCHAR(128) NOT NULL,
    description TEXT,
    owner_id    UUID NOT NULL REFERENCES users(id),
    is_public   BOOLEAN DEFAULT FALSE,
    shared_with JSONB DEFAULT '[]',
    layout      JSONB NOT NULL DEFAULT '{}',
    settings    JSONB DEFAULT '{}',
    template_id UUID,
    is_system   BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

CREATE TABLE IF NOT EXISTS dashboard_widgets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dashboard_id UUID NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
    widget_type  VARCHAR(64) NOT NULL,
    title        VARCHAR(256),
    config       JSONB NOT NULL DEFAULT '{}',
    position     JSONB DEFAULT '{}',
    data_source  JSONB NOT NULL,
    sort_order   INTEGER DEFAULT 0,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================
-- AI / RAG
-- ============================================================

CREATE TABLE IF NOT EXISTS rag_documents (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    source      VARCHAR(64) NOT NULL,
    source_type VARCHAR(128),
    source_id   VARCHAR(512),
    source_url  VARCHAR(1024),
    content     TEXT NOT NULL,
    metadata    JSONB DEFAULT '{}',
    ingested_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS rag_chunks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES rag_documents(id) ON DELETE CASCADE,
    tenant_id   UUID NOT NULL,
    content     TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,
    embedding   vector(1536),
    metadata    JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (document_id, chunk_index)
);

CREATE INDEX IF NOT EXISTS idx_rag_chunks_doc ON rag_chunks(document_id);
CREATE INDEX IF NOT EXISTS idx_rag_chunks_tenant ON rag_chunks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_rag_chunks_metadata ON rag_chunks USING GIN(metadata);
CREATE INDEX IF NOT EXISTS idx_rag_docs_source ON rag_documents(tenant_id, source, source_type);

ALTER TABLE rag_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE rag_chunks ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_rag_docs ON rag_documents;
CREATE POLICY tenant_rag_docs ON rag_documents
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

DROP POLICY IF EXISTS tenant_rag_chunks ON rag_chunks;
CREATE POLICY tenant_rag_chunks ON rag_chunks
    USING (tenant_id = current_setting('app.current_tenant', true)::UUID);

CREATE TABLE IF NOT EXISTS ai_conversations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    user_id     UUID NOT NULL,
    title       VARCHAR(256),
    status      VARCHAR(32) DEFAULT 'active',
    metadata    JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ai_messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role            VARCHAR(32) NOT NULL,
    content         TEXT NOT NULL,
    tool_calls      JSONB,
    tool_call_id    VARCHAR(256),
    tokens_input    INTEGER,
    tokens_output   INTEGER,
    model_used      VARCHAR(128),
    provider_used   VARCHAR(64),
    cost_usd        DECIMAL(10, 6),
    citations       JSONB,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_conv_user ON ai_conversations(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_msg_conv ON ai_messages(conversation_id, created_at);

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (3, 'Add dashboards, dashboard_widgets, rag_documents, rag_chunks, ai_conversations, ai_messages')
ON CONFLICT DO NOTHING;
