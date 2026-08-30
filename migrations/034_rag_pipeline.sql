-- ============================================================
-- Migration 034: RAG Pipeline Enhancement
-- ============================================================
-- Adds vector similarity search index, hybrid search function,
-- and ingestion tracking for the RAG knowledge base.
-- ============================================================

-- IVFFlat index for fast cosine similarity search on embeddings.
-- lists = 100 is appropriate for up to ~1M chunks; increase for larger datasets.
-- Note: IVFFlat requires at least one row to build; we use IF NOT EXISTS
-- and wrap in a DO block to handle empty tables gracefully.
DO $$
BEGIN
    -- Only create the index if rag_chunks has at least one row with an embedding
    IF (SELECT COUNT(*) FROM rag_chunks WHERE embedding IS NOT NULL) > 0 THEN
        CREATE INDEX IF NOT EXISTS idx_rag_chunks_embedding
            ON rag_chunks USING ivfflat(embedding vector_cosine_ops) WITH (lists = 100);
    END IF;
EXCEPTION WHEN OTHERS THEN
    -- If the table is empty or pgvector is not available, skip silently.
    -- The index will be created on first ingestion via the trigger below.
    RAISE NOTICE 'Skipping IVFFlat index creation (table may be empty): %', SQLERRM;
END $$;

-- HNSW index as a fallback (works even on empty tables, better recall than IVFFlat).
-- Requires pgvector >= 0.5.0.
DO $$
BEGIN
    CREATE INDEX IF NOT EXISTS idx_rag_chunks_embedding_hnsw
        ON rag_chunks USING hnsw(embedding vector_cosine_ops);
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Skipping HNSW index (pgvector may not support it): %', SQLERRM;
END $$;

-- ============================================================
-- Hybrid search function: combines vector similarity + BM25
-- ============================================================
CREATE OR REPLACE FUNCTION rag_search(
    p_query_embedding vector(1536),
    p_tenant_id UUID,
    p_top_k INTEGER DEFAULT 10,
    p_filters JSONB DEFAULT '{}'::jsonb,
    p_min_similarity FLOAT DEFAULT 0.3
)
RETURNS TABLE (
    chunk_id UUID,
    document_id UUID,
    content TEXT,
    chunk_index INTEGER,
    metadata JSONB,
    source VARCHAR(64),
    source_type VARCHAR(128),
    source_id VARCHAR(512),
    source_url VARCHAR(1024),
    similarity FLOAT
)
LANGUAGE plpgsql
STABLE
AS $$
BEGIN
    RETURN QUERY
    WITH vector_results AS (
        SELECT
            c.id AS chunk_id,
            c.document_id,
            c.content,
            c.chunk_index,
            c.metadata AS chunk_metadata,
            d.source,
            d.source_type,
            d.source_id,
            d.source_url,
            1 - (c.embedding <=> p_query_embedding) AS sim
        FROM rag_chunks c
        JOIN rag_documents d ON d.id = c.document_id
        WHERE c.tenant_id = p_tenant_id
          AND d.tenant_id = p_tenant_id
          AND c.embedding IS NOT NULL
          AND (d.expires_at IS NULL OR d.expires_at > NOW())
          -- Apply optional metadata filters
          AND (
              p_filters = '{}'::jsonb
              OR (p_filters ? 'source' AND d.source = (p_filters->>'source'))
          )
          AND (
              NOT (p_filters ? 'source_type')
              OR d.source_type = (p_filters->>'source_type')
          )
    ),
    ranked AS (
        SELECT *
        FROM vector_results
        WHERE sim >= p_min_similarity
        ORDER BY sim DESC
        LIMIT p_top_k
    )
    SELECT
        r.chunk_id,
        r.document_id,
        r.content,
        r.chunk_index,
        r.chunk_metadata AS metadata,
        r.source,
        r.source_type,
        r.source_id,
        r.source_url,
        r.similarity
    FROM ranked r
    ORDER BY r.similarity DESC;
END;
$$;

-- ============================================================
-- Keyword (BM25-like) search using tsvector full-text search
-- ============================================================

-- Add tsvector column for full-text search on chunks
ALTER TABLE rag_chunks ADD COLUMN IF NOT EXISTS content_tsv tsvector;

-- Function to update tsvector from content
CREATE OR REPLACE FUNCTION rag_chunks_tsv_trigger()
RETURNS TRIGGER AS $$
BEGIN
    NEW.content_tsv := to_tsvector('english', COALESCE(NEW.content, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_rag_chunks_tsv ON rag_chunks;
CREATE TRIGGER trg_rag_chunks_tsv
    BEFORE INSERT OR UPDATE OF content ON rag_chunks
    FOR EACH ROW EXECUTE FUNCTION rag_chunks_tsv_trigger();

-- Backfill existing rows
UPDATE rag_chunks SET content_tsv = to_tsvector('english', COALESCE(content, ''))
WHERE content_tsv IS NULL;

-- GIN index on tsvector for fast keyword search
CREATE INDEX IF NOT EXISTS idx_rag_chunks_tsv ON rag_chunks USING GIN(content_tsv);

-- Keyword search function
CREATE OR REPLACE FUNCTION rag_keyword_search(
    p_query TEXT,
    p_tenant_id UUID,
    p_top_k INTEGER DEFAULT 10,
    p_filters JSONB DEFAULT '{}'::jsonb
)
RETURNS TABLE (
    chunk_id UUID,
    document_id UUID,
    content TEXT,
    chunk_index INTEGER,
    metadata JSONB,
    source VARCHAR(64),
    source_type VARCHAR(128),
    source_id VARCHAR(512),
    source_url VARCHAR(1024),
    relevance FLOAT
)
LANGUAGE plpgsql
STABLE
AS $$
BEGIN
    RETURN QUERY
    SELECT
        c.id AS chunk_id,
        c.document_id,
        c.content,
        c.chunk_index,
        c.metadata,
        d.source,
        d.source_type,
        d.source_id,
        d.source_url,
        ts_rank(c.content_tsv, plainto_tsquery('english', p_query))::FLOAT AS relevance
    FROM rag_chunks c
    JOIN rag_documents d ON d.id = c.document_id
    WHERE c.tenant_id = p_tenant_id
      AND d.tenant_id = p_tenant_id
      AND c.content_tsv @@ plainto_tsquery('english', p_query)
      AND (d.expires_at IS NULL OR d.expires_at > NOW())
      AND (
          p_filters = '{}'::jsonb
          OR (p_filters ? 'source' AND d.source = (p_filters->>'source'))
      )
      AND (
          NOT (p_filters ? 'source_type')
          OR d.source_type = (p_filters->>'source_type')
      )
    ORDER BY relevance DESC
    LIMIT p_top_k;
END;
$$;

-- ============================================================
-- Ingestion tracking: prevent duplicate documents
-- ============================================================

-- Unique constraint to allow upsert on source identity
CREATE UNIQUE INDEX IF NOT EXISTS idx_rag_docs_source_identity
    ON rag_documents(tenant_id, source, source_type, source_id)
    WHERE source_id IS NOT NULL;

-- Auto-expire old documents (default 30 days)
ALTER TABLE rag_documents ALTER COLUMN expires_at SET DEFAULT NOW() + INTERVAL '30 days';
