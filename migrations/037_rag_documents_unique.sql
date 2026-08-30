-- Add unique constraint for RAG document upserts.
-- The UpsertDocument query uses ON CONFLICT (tenant_id, source, source_type, source_id)
-- which requires a matching unique constraint.

-- Only enforce uniqueness when source_id is actually set (not null/empty).
CREATE UNIQUE INDEX IF NOT EXISTS idx_rag_documents_unique
    ON rag_documents (tenant_id, source, source_type, source_id)
    WHERE source_id IS NOT NULL;
