package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/database"
)

// RAGDocument represents a document stored in the RAG knowledge base.
type RAGDocument struct {
	ID         uuid.UUID              `json:"id"`
	TenantID   uuid.UUID              `json:"tenant_id"`
	Source     string                 `json:"source"`
	SourceType string                 `json:"source_type"`
	SourceID   string                 `json:"source_id"`
	SourceURL  string                 `json:"source_url"`
	Content    string                 `json:"content"`
	Metadata   map[string]interface{} `json:"metadata"`
	IngestedAt time.Time              `json:"ingested_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	ExpiresAt  *time.Time             `json:"expires_at"`
	CreatedAt  time.Time              `json:"created_at"`
}

// RAGChunk represents a chunk of a document with embedding.
type RAGChunk struct {
	ID         uuid.UUID              `json:"id"`
	DocumentID uuid.UUID              `json:"document_id"`
	TenantID   uuid.UUID              `json:"tenant_id"`
	Content    string                 `json:"content"`
	ChunkIndex int                    `json:"chunk_index"`
	Embedding  []float32              `json:"embedding,omitempty"`
	Metadata   map[string]interface{} `json:"metadata"`
	CreatedAt  time.Time              `json:"created_at"`
}

// RAGSearchResult is a single result from a RAG search.
type RAGSearchResult struct {
	ChunkID    uuid.UUID              `json:"chunk_id"`
	DocumentID uuid.UUID              `json:"document_id"`
	Content    string                 `json:"content"`
	ChunkIndex int                    `json:"chunk_index"`
	Metadata   map[string]interface{} `json:"metadata"`
	Source     string                 `json:"source"`
	SourceType string                 `json:"source_type"`
	SourceID   string                 `json:"source_id"`
	SourceURL  string                 `json:"source_url"`
	Score      float64                `json:"score"`
}

// RAGRepository handles RAG document and chunk persistence.
type RAGRepository struct {
	pool *pgxpool.Pool
}

// NewRAGRepository creates a new RAG repository.
func NewRAGRepository(db *database.DB) *RAGRepository {
	return &RAGRepository{pool: db.Pool}
}

// UpsertDocument inserts or updates a document based on source identity.
// Returns the document ID (existing or new).
func (r *RAGRepository) UpsertDocument(ctx context.Context, doc *RAGDocument) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO rag_documents (tenant_id, source, source_type, source_id, source_url, content, metadata, ingested_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW() + INTERVAL '30 days')
		ON CONFLICT (tenant_id, source, source_type, source_id) WHERE source_id IS NOT NULL
		DO UPDATE SET content = $6, metadata = $7, updated_at = NOW(), ingested_at = NOW(), expires_at = NOW() + INTERVAL '30 days'
		RETURNING id
	`,
		doc.TenantID, doc.Source, doc.SourceType, doc.SourceID, doc.SourceURL,
		doc.Content, mustJSON(doc.Metadata),
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upsert rag document: %w", err)
	}
	return id, nil
}

// InsertDocument inserts a new document without upsert.
func (r *RAGRepository) InsertDocument(ctx context.Context, doc *RAGDocument) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO rag_documents (tenant_id, source, source_type, source_id, source_url, content, metadata, ingested_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW() + INTERVAL '30 days')
		RETURNING id
	`,
		doc.TenantID, doc.Source, doc.SourceType, doc.SourceID, doc.SourceURL,
		doc.Content, mustJSON(doc.Metadata),
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert rag document: %w", err)
	}
	return id, nil
}

// DeleteDocument removes a document and its chunks.
func (r *RAGRepository) DeleteDocument(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM rag_documents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete rag document: %w", err)
	}
	return nil
}

// DeleteDocumentsBySource removes all documents matching a source filter.
func (r *RAGRepository) DeleteDocumentsBySource(ctx context.Context, tenantID uuid.UUID, source, sourceType, sourceID string) (int64, error) {
	query := `DELETE FROM rag_documents WHERE tenant_id = $1 AND source = $2`
	args := []interface{}{tenantID, source}
	argIdx := 3

	if sourceType != "" {
		query += fmt.Sprintf(` AND source_type = $%d`, argIdx)
		args = append(args, sourceType)
		argIdx++
	}
	if sourceID != "" {
		query += fmt.Sprintf(` AND source_id = $%d`, argIdx)
		args = append(args, sourceID)
	}

	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete rag documents by source: %w", err)
	}
	return tag.RowsAffected(), nil
}

// InsertChunk inserts a chunk with its embedding vector.
func (r *RAGRepository) InsertChunk(ctx context.Context, chunk *RAGChunk) error {
	embeddingStr := formatEmbedding(chunk.Embedding)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO rag_chunks (document_id, tenant_id, content, chunk_index, embedding, metadata)
		VALUES ($1, $2, $3, $4, $5::vector, $6)
		ON CONFLICT (document_id, chunk_index)
		DO UPDATE SET content = $3, embedding = $5::vector, metadata = $6
	`,
		chunk.DocumentID, chunk.TenantID, chunk.Content,
		chunk.ChunkIndex, embeddingStr, mustJSON(chunk.Metadata),
	)
	if err != nil {
		return fmt.Errorf("insert rag chunk: %w", err)
	}
	return nil
}

// DeleteChunksByDocument removes all chunks for a document.
func (r *RAGRepository) DeleteChunksByDocument(ctx context.Context, documentID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM rag_chunks WHERE document_id = $1`, documentID)
	if err != nil {
		return fmt.Errorf("delete rag chunks: %w", err)
	}
	return nil
}

// VectorSearch performs cosine similarity search.
func (r *RAGRepository) VectorSearch(ctx context.Context, queryEmbedding []float32, tenantID uuid.UUID, topK int, filters map[string]string) ([]RAGSearchResult, error) {
	embeddingStr := formatEmbedding(queryEmbedding)
	filtersJSON, _ := json.Marshal(filters)

	rows, err := r.pool.Query(ctx, `
		SELECT chunk_id, document_id, content, chunk_index, metadata,
		       source, source_type, source_id, source_url, similarity
		FROM rag_search($1::vector, $2, $3, $4::jsonb)
	`, embeddingStr, tenantID, topK, string(filtersJSON))
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	return r.scanSearchResults(rows)
}

// KeywordSearch performs full-text keyword search.
func (r *RAGRepository) KeywordSearch(ctx context.Context, query string, tenantID uuid.UUID, topK int, filters map[string]string) ([]RAGSearchResult, error) {
	filtersJSON, _ := json.Marshal(filters)

	rows, err := r.pool.Query(ctx, `
		SELECT chunk_id, document_id, content, chunk_index, metadata,
		       source, source_type, source_id, source_url, relevance
		FROM rag_keyword_search($1, $2, $3, $4::jsonb)
	`, query, tenantID, topK, string(filtersJSON))
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}
	defer rows.Close()

	return r.scanSearchResults(rows)
}

// ListDocuments returns documents for a tenant with pagination.
func (r *RAGRepository) ListDocuments(ctx context.Context, tenantID uuid.UUID, source string, limit, offset int) ([]RAGDocument, int, error) {
	countQuery := `SELECT COUNT(*) FROM rag_documents WHERE tenant_id = $1`
	countArgs := []interface{}{tenantID}
	if source != "" {
		countQuery += ` AND source = $2`
		countArgs = append(countArgs, source)
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count rag documents: %w", err)
	}

	query := `
		SELECT id, tenant_id, source, source_type, COALESCE(source_id,''), COALESCE(source_url,''),
		       LEFT(content, 500), metadata, ingested_at, updated_at, expires_at, created_at
		FROM rag_documents WHERE tenant_id = $1
	`
	args := []interface{}{tenantID}
	if source != "" {
		query += ` AND source = $2`
		args = append(args, source)
	}
	query += ` ORDER BY ingested_at DESC LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list rag documents: %w", err)
	}
	defer rows.Close()

	var docs []RAGDocument
	for rows.Next() {
		var d RAGDocument
		var metaBytes []byte
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Source, &d.SourceType,
			&d.SourceID, &d.SourceURL, &d.Content, &metaBytes,
			&d.IngestedAt, &d.UpdatedAt, &d.ExpiresAt, &d.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan rag document: %w", err)
		}
		_ = json.Unmarshal(metaBytes, &d.Metadata)
		docs = append(docs, d)
	}
	return docs, total, nil
}

// GetDocumentStats returns document and chunk counts by source.
func (r *RAGRepository) GetDocumentStats(ctx context.Context, tenantID uuid.UUID) (map[string]int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT source, COUNT(*) FROM rag_documents
		WHERE tenant_id = $1
		GROUP BY source
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("rag document stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			return nil, err
		}
		stats[source] = count
	}

	// Total chunks
	var totalChunks int
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM rag_chunks WHERE tenant_id = $1
	`, tenantID).Scan(&totalChunks); err != nil {
		return nil, err
	}
	stats["_chunks"] = totalChunks

	return stats, nil
}

// ExpireOldDocuments removes documents past their expiration date.
func (r *RAGRepository) ExpireOldDocuments(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM rag_documents WHERE expires_at IS NOT NULL AND expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("expire old rag documents: %w", err)
	}
	return tag.RowsAffected(), nil
}

// scanSearchResults scans rows from a search function into results.
func (r *RAGRepository) scanSearchResults(rows pgx.Rows) ([]RAGSearchResult, error) {
	var results []RAGSearchResult
	for rows.Next() {
		var res RAGSearchResult
		var metaBytes []byte
		if err := rows.Scan(&res.ChunkID, &res.DocumentID, &res.Content,
			&res.ChunkIndex, &metaBytes,
			&res.Source, &res.SourceType, &res.SourceID, &res.SourceURL,
			&res.Score); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		_ = json.Unmarshal(metaBytes, &res.Metadata)
		results = append(results, res)
	}
	return results, nil
}

// formatEmbedding converts a float32 slice to pgvector string format "[0.1,0.2,...]".
func formatEmbedding(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	b := make([]byte, 0, len(v)*10+2)
	b = append(b, '[')
	for i, f := range v {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, fmt.Sprintf("%g", f)...)
	}
	b = append(b, ']')
	return string(b)
}

// mustJSON marshals a value to JSON or returns "{}".
func mustJSON(v interface{}) []byte {
	if v == nil {
		return []byte("{}")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}
