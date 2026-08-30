package ai

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/repository"
)

// IngestionEngine handles document ingestion into the RAG knowledge base.
type IngestionEngine struct {
	pool    *pgxpool.Pool
	ragRepo *repository.RAGRepository
	chunker Chunker
	provider LLMProvider

	// Per-source-type locks to avoid serializing unrelated ingestions.
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewIngestionEngine creates a new ingestion engine.
func NewIngestionEngine(pool *pgxpool.Pool, ragRepo *repository.RAGRepository, provider LLMProvider) *IngestionEngine {
	return &IngestionEngine{
		pool:     pool,
		ragRepo:  ragRepo,
		chunker:  NewSemanticChunker(),
		provider: provider,
		locks:    make(map[string]*sync.Mutex),
	}
}

// sourceLock returns a per-source mutex, creating one if needed.
func (e *IngestionEngine) sourceLock(source string) *sync.Mutex {
	e.mu.Lock()
	defer e.mu.Unlock()
	l, ok := e.locks[source]
	if !ok {
		l = &sync.Mutex{}
		e.locks[source] = l
	}
	return l
}

// IngestDocument chunks, embeds, and stores a document.
func (e *IngestionEngine) IngestDocument(ctx context.Context, doc *Document, tenantID uuid.UUID) error {
	// Lock per source type to avoid serializing unrelated ingestions.
	l := e.sourceLock(doc.Source)
	l.Lock()
	defer l.Unlock()

	// 1. Store the document
	ragDoc := &repository.RAGDocument{
		TenantID:   tenantID,
		Source:     doc.Source,
		SourceType: doc.Type,
		SourceID:   doc.ID,
		Content:    doc.Content,
		Metadata:   stringMapToInterface(doc.Metadata),
	}

	docUUID, err := e.ragRepo.UpsertDocument(ctx, ragDoc)
	if err != nil {
		return fmt.Errorf("store document: %w", err)
	}

	// 2. Delete existing chunks (will be replaced)
	if err := e.ragRepo.DeleteChunksByDocument(ctx, docUUID); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}

	// 3. Chunk the document
	aiDoc := &Document{
		ID:       docUUID.String(),
		Source:   doc.Source,
		Type:     doc.Type,
		Content:  doc.Content,
		Metadata: doc.Metadata,
	}
	chunks, err := e.chunker.Chunk(aiDoc)
	if err != nil {
		return fmt.Errorf("chunk document: %w", err)
	}

	// 4. Generate embeddings in batches
	if e.provider != nil {
		batchSize := 20
		for i := 0; i < len(chunks); i += batchSize {
			end := i + batchSize
			if end > len(chunks) {
				end = len(chunks)
			}
			batch := chunks[i:end]

			texts := make([]string, len(batch))
			for j, ch := range batch {
				texts[j] = ch.Content
			}

			embedResp, err := e.provider.Embed(ctx, texts, &EmbedOptions{})
			if err != nil {
				slog.Warn("embedding failed for batch, storing without embeddings", "error", err, "batch_start", i)
				continue
			}

			for j, ch := range batch {
				if j < len(embedResp.Vectors) {
					ch.Embedding = embedResp.Vectors[j]
				}
			}
		}
	}

	// 5. Store chunks with embeddings
	for _, ch := range chunks {
		ragChunk := &repository.RAGChunk{
			DocumentID: docUUID,
			TenantID:   tenantID,
			Content:    ch.Content,
			ChunkIndex: ch.Index,
			Embedding:  ch.Embedding,
			Metadata:   stringMapToInterface(doc.Metadata),
		}
		if err := e.ragRepo.InsertChunk(ctx, ragChunk); err != nil {
			slog.Warn("failed to store chunk", "error", err, "document_id", docUUID, "index", ch.Index)
		}
	}

	slog.Info("document ingested", "source", doc.Source, "type", doc.Type, "chunks", len(chunks), "doc_id", docUUID)
	return nil
}

// IngestBatch ingests multiple documents.
func (e *IngestionEngine) IngestBatch(ctx context.Context, docs []*Document, tenantID uuid.UUID) (int, error) {
	ingested := 0
	for _, doc := range docs {
		if err := e.IngestDocument(ctx, doc, tenantID); err != nil {
			slog.Warn("failed to ingest document in batch", "error", err, "doc_id", doc.ID)
			continue
		}
		ingested++
	}
	return ingested, nil
}

// ReindexAll re-ingests all documents from a specific source.
func (e *IngestionEngine) ReindexAll(ctx context.Context, loader DocumentLoader, tenantID uuid.UUID) (int, error) {
	docs, err := loader.Load(ctx)
	if err != nil {
		return 0, fmt.Errorf("load documents: %w", err)
	}
	return e.IngestBatch(ctx, docs, tenantID)
}

// ExpireOld removes expired documents.
func (e *IngestionEngine) ExpireOld(ctx context.Context) (int64, error) {
	return e.ragRepo.ExpireOldDocuments(ctx)
}

// DocumentLoader loads documents from an external source.
type DocumentLoader interface {
	Load(ctx context.Context) ([]*Document, error)
}

// EntityDocumentLoader loads entities as RAG documents.
type EntityDocumentLoader struct {
	pool     *pgxpool.Pool
	tenantID uuid.UUID
}

// NewEntityDocumentLoader creates a loader for entity graph data.
func NewEntityDocumentLoader(pool *pgxpool.Pool, tenantID uuid.UUID) *EntityDocumentLoader {
	return &EntityDocumentLoader{pool: pool, tenantID: tenantID}
}

// Load fetches entities and converts them to documents.
func (l *EntityDocumentLoader) Load(ctx context.Context) ([]*Document, error) {
	rows, err := l.pool.Query(ctx, `
		SELECT e.id, e.name, e.type_key, COALESCE(e.description, ''),
		       COALESCE(e.metadata::text, '{}'),
		       COALESCE(e.labels::text, '{}')
		FROM entities e
		WHERE e.tenant_id = $1
	`, l.tenantID)
	if err != nil {
		return nil, fmt.Errorf("query entities: %w", err)
	}
	defer rows.Close()

	var docs []*Document
	for rows.Next() {
		var id, name, typeKey, description, metadata, labels string
		if err := rows.Scan(&id, &name, &typeKey, &description, &metadata, &labels); err != nil {
			continue
		}
		content := fmt.Sprintf("Entity: %s\nType: %s\nDescription: %s\nMetadata: %s\nLabels: %s",
			name, typeKey, description, metadata, labels)
		docs = append(docs, &Document{
			ID:     id,
			Source: "entity",
			Type:   typeKey,
			Content: content,
			Metadata: map[string]string{
				"name":     name,
				"type_key": typeKey,
			},
		})
	}
	return docs, nil
}

// ServiceDocumentLoader loads services as RAG documents.
type ServiceDocumentLoader struct {
	pool     *pgxpool.Pool
	tenantID uuid.UUID
}

// NewServiceDocumentLoader creates a loader for service catalog data.
func NewServiceDocumentLoader(pool *pgxpool.Pool, tenantID uuid.UUID) *ServiceDocumentLoader {
	return &ServiceDocumentLoader{pool: pool, tenantID: tenantID}
}

// Load fetches services and converts them to documents.
func (l *ServiceDocumentLoader) Load(ctx context.Context) ([]*Document, error) {
	rows, err := l.pool.Query(ctx, `
		SELECT s.id, s.name, COALESCE(s.description, ''),
		       COALESCE(s.language, ''), COALESCE(s.framework, ''),
		       COALESCE(s.owner, ''), COALESCE(s.status, ''),
		       COALESCE(s.tags::text, '[]')
		FROM services s
		WHERE s.tenant_id = $1
	`, l.tenantID)
	if err != nil {
		return nil, fmt.Errorf("query services: %w", err)
	}
	defer rows.Close()

	var docs []*Document
	for rows.Next() {
		var id, name, description, language, framework, owner, status, tags string
		if err := rows.Scan(&id, &name, &description, &language, &framework, &owner, &status, &tags); err != nil {
			continue
		}
		content := fmt.Sprintf("Service: %s\nDescription: %s\nLanguage: %s\nFramework: %s\nOwner: %s\nStatus: %s\nTags: %s",
			name, description, language, framework, owner, status, tags)
		docs = append(docs, &Document{
			ID:      id,
			Source:  "service",
			Type:    "service",
			Content: content,
			Metadata: map[string]string{
				"name":   name,
				"owner":  owner,
				"status": status,
			},
		})
	}
	return docs, nil
}

// PipelineDocumentLoader loads pipeline run history as RAG documents.
type PipelineDocumentLoader struct {
	pool     *pgxpool.Pool
	tenantID uuid.UUID
	Limit    int
}

// NewPipelineDocumentLoader creates a loader for pipeline history.
func NewPipelineDocumentLoader(pool *pgxpool.Pool, tenantID uuid.UUID) *PipelineDocumentLoader {
	return &PipelineDocumentLoader{pool: pool, tenantID: tenantID, Limit: 100}
}

// Load fetches recent pipeline runs and converts them to documents.
func (l *PipelineDocumentLoader) Load(ctx context.Context) ([]*Document, error) {
	rows, err := l.pool.Query(ctx, `
		SELECT pr.id, pr.pipeline_source_id, COALESCE(pr.status, ''),
		       COALESCE(pr.trigger::text, '{}'),
		       COALESCE(pr.result::text, '{}'),
		       pr.started_at, pr.finished_at,
		       COALESCE(ps.name, ''), COALESCE(ps.source_type, '')
		FROM pipeline_runs pr
		LEFT JOIN pipeline_sources ps ON ps.id = pr.pipeline_source_id
		WHERE pr.tenant_id = $1
		ORDER BY pr.started_at DESC
		LIMIT $2
	`, l.tenantID, l.Limit)
	if err != nil {
		return nil, fmt.Errorf("query pipeline runs: %w", err)
	}
	defer rows.Close()

	var docs []*Document
	for rows.Next() {
		var id, sourceID, status, trigger, result, name, sourceType string
		var startedAt, finishedAt time.Time
		if err := rows.Scan(&id, &sourceID, &status, &trigger, &result,
			&startedAt, &finishedAt, &name, &sourceType); err != nil {
			continue
		}
		duration := finishedAt.Sub(startedAt).String()
		content := fmt.Sprintf("Pipeline Run: %s\nSource: %s (%s)\nStatus: %s\nStarted: %s\nDuration: %s\nResult: %s",
			id, name, sourceType, status, startedAt.Format(time.RFC3339), duration, result)
		docs = append(docs, &Document{
			ID:      id,
			Source:  "pipeline",
			Type:    sourceType,
			Content: content,
			Metadata: map[string]string{
				"source_name": name,
				"status":      status,
			},
		})
	}
	return docs, nil
}

// stringMapToInterface converts map[string]string to map[string]interface{}.
func stringMapToInterface(m map[string]string) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
