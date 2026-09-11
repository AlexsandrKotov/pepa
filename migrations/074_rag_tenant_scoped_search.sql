-- ============================================================
-- 074: RAG search is scoped by an explicit set of tenant IDs
-- ============================================================
-- rag_search()/rag_keyword_search() took a single p_tenant_id, which forced the
-- Go layer to pass one constant tenant (the seeded platform tenant) for every
-- request: all workspaces shared one knowledge base and could read each other's
-- ingested documents. The functions now accept an array of tenant IDs the caller
-- is allowed to read (own tenant + the platform-seeded tenant), so per-request
-- scoping is possible without duplicating the seed corpus per workspace.
--
-- The old single-UUID overloads are dropped on purpose: a caller that has not
-- been updated must fail loudly instead of quietly reading the wrong scope.

DROP FUNCTION IF EXISTS rag_search(vector(1536), uuid, integer, jsonb, float);
DROP FUNCTION IF EXISTS rag_search(vector(1536), uuid, integer, jsonb);
DROP FUNCTION IF EXISTS rag_keyword_search(text, uuid, integer, jsonb);

CREATE OR REPLACE FUNCTION rag_search(
    p_query_embedding vector(1536),
    p_tenant_ids UUID[],
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
    -- An empty or NULL scope means "no tenant may be read" — return nothing
    -- rather than falling back to an unscoped scan.
    IF p_tenant_ids IS NULL OR COALESCE(array_length(p_tenant_ids, 1), 0) = 0 THEN
        RETURN;
    END IF;

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
        WHERE c.tenant_id = ANY(p_tenant_ids)
          AND d.tenant_id = ANY(p_tenant_ids)
          AND c.embedding IS NOT NULL
          AND (d.expires_at IS NULL OR d.expires_at > NOW())
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

CREATE OR REPLACE FUNCTION rag_keyword_search(
    p_query TEXT,
    p_tenant_ids UUID[],
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
    IF p_tenant_ids IS NULL OR COALESCE(array_length(p_tenant_ids, 1), 0) = 0 THEN
        RETURN;
    END IF;

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
    WHERE c.tenant_id = ANY(p_tenant_ids)
      AND d.tenant_id = ANY(p_tenant_ids)
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

-- List/statistics queries filter by tenant_id = $1; make that cheap for the
-- per-request scope as well.
CREATE INDEX IF NOT EXISTS idx_rag_docs_tenant_source ON rag_documents(tenant_id, source, ingested_at DESC);
CREATE INDEX IF NOT EXISTS idx_rag_chunks_tenant ON rag_chunks(tenant_id);
