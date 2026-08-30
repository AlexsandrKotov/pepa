package rest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/ai"
	"github.com/pepa/pepa/internal/repository"
)

// RAGHandlers handles RAG knowledge base endpoints.
type RAGHandlers struct {
	ragRepo    *repository.RAGRepository
	aiManager  *ai.Manager
	ingestion  *ai.IngestionEngine
	pipeline   *ai.RAGPipeline
	tenantID   uuid.UUID
}

// NewRAGHandlers creates new RAG handlers.
func NewRAGHandlers(ragRepo *repository.RAGRepository, aiMgr *ai.Manager, tenantID uuid.UUID) *RAGHandlers {
	return &RAGHandlers{
		ragRepo:   ragRepo,
		aiManager: aiMgr,
		tenantID:  tenantID,
	}
}

// SetIngestionEngine sets the ingestion engine (called after bootstrap).
func (h *RAGHandlers) SetIngestionEngine(engine *ai.IngestionEngine) {
	h.ingestion = engine
}

// SetPipeline sets the RAG pipeline (called after bootstrap).
func (h *RAGHandlers) SetPipeline(pipeline *ai.RAGPipeline) {
	h.pipeline = pipeline
}

// IngestDocument handles manual document ingestion.
func (h *RAGHandlers) IngestDocument(c *gin.Context) {
	if h.ingestion == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RAG ingestion engine not initialized"})
		return
	}

	var req struct {
		Source     string            `json:"source" binding:"required"`
		SourceType string            `json:"source_type"`
		SourceID   string            `json:"source_id"`
		Content    string            `json:"content" binding:"required"`
		Metadata   map[string]string `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	doc := &ai.Document{
		ID:       req.SourceID,
		Source:   req.Source,
		Type:     req.SourceType,
		Content:  req.Content,
		Metadata: req.Metadata,
	}
	if doc.ID == "" {
		doc.ID = uuid.NewString()
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	if err := h.ingestion.IngestDocument(ctx, doc, h.tenantID); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps(), c, "rag_ingest", "rag_document", doc.ID, nil, gin.H{
		"source": req.Source, "source_type": req.SourceType,
	})

	c.JSON(http.StatusCreated, gin.H{
		"message": "Document ingested successfully",
		"source":  req.Source,
	})
}

// Search performs a standalone RAG search without generation.
func (h *RAGHandlers) Search(c *gin.Context) {
	var req struct {
		Query   string            `json:"query" binding:"required"`
		TopK    int               `json:"top_k"`
		Filters map[string]string `json:"filters"`
		Mode    string            `json:"mode"` // vector, keyword, hybrid
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.TopK <= 0 {
		req.TopK = 10
	}

	ctx := c.Request.Context()
	var results []repository.RAGSearchResult
	var err error

	switch req.Mode {
	case "keyword":
		results, err = h.ragRepo.KeywordSearch(ctx, req.Query, h.tenantID, req.TopK, req.Filters)
	case "vector":
		// For vector-only, we need to embed first
		provider, pErr := h.aiManager.DefaultProvider()
		if pErr != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI provider not configured"})
			return
		}
		embedResp, eErr := provider.Embed(ctx, []string{req.Query}, &ai.EmbedOptions{})
		if eErr != nil || len(embedResp.Vectors) == 0 {
			respondInternalError(c, fmt.Errorf("embedding failed: %v", eErr))
			return
		}
		results, err = h.ragRepo.VectorSearch(ctx, embedResp.Vectors[0], h.tenantID, req.TopK, req.Filters)
	default: // hybrid
		results, err = h.ragRepo.VectorSearch(ctx, nil, h.tenantID, req.TopK, req.Filters)
		if err == nil {
			keywordResults, kErr := h.ragRepo.KeywordSearch(ctx, req.Query, h.tenantID, req.TopK, req.Filters)
			if kErr == nil && len(keywordResults) > 0 {
				results = append(results, keywordResults...)
			}
		}
	}

	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"total":   len(results),
		"query":   req.Query,
	})
}

// ListDocuments returns all documents in the knowledge base.
func (h *RAGHandlers) ListDocuments(c *gin.Context) {
	source := c.Query("source")
	limit := 50
	offset := 0

	docs, total, err := h.ragRepo.ListDocuments(c.Request.Context(), h.tenantID, source, limit, offset)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"documents": docs,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

// DeleteDocument removes a document from the knowledge base.
func (h *RAGHandlers) DeleteDocument(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid document ID"})
		return
	}

	if err := h.ragRepo.DeleteDocument(c.Request.Context(), id); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps(), c, "rag_delete", "rag_document", idStr, nil, nil)

	c.JSON(http.StatusOK, gin.H{"message": "Document deleted", "id": idStr})
}

// GetDocument retrieves a single document by ID.
func (h *RAGHandlers) GetDocument(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid document ID"})
		return
	}

	doc, err := h.ragRepo.GetDocument(c.Request.Context(), id)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, doc)
}

// UpdateDocument updates a document's content and re-chunks/re-embeds it.
func (h *RAGHandlers) UpdateDocument(c *gin.Context) {
	if h.ingestion == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RAG ingestion engine not initialized"})
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid document ID"})
		return
	}

	var req struct {
		Content  string                 `json:"content" binding:"required"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	// Update the document content
	if err := h.ragRepo.UpdateDocumentContent(ctx, id, req.Content, req.Metadata); err != nil {
		respondInternalError(c, err)
		return
	}

	// Delete old chunks and re-ingest to re-chunk and re-embed
	_ = h.ragRepo.DeleteChunksByDocument(ctx, id)

	// Re-fetch the updated document to get the new content for ingestion
	doc, err := h.ragRepo.GetDocument(ctx, id)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	aiDoc := &ai.Document{
		ID:       doc.ID.String(),
		Source:   doc.Source,
		Type:     doc.SourceType,
		Content:  doc.Content,
		Metadata: toStringMap(doc.Metadata),
	}
	if err := h.ingestion.IngestDocument(ctx, aiDoc, h.tenantID); err != nil {
		slog.Warn("RAG: re-ingestion after update failed", "error", err)
	}

	logAudit(h.deps(), c, "rag_update", "rag_document", idStr, nil, gin.H{"content_length": len(req.Content)})

	c.JSON(http.StatusOK, gin.H{"message": "Document updated and re-indexed", "id": idStr})
}

// CreateDocument creates a new custom document in the knowledge base.
func (h *RAGHandlers) CreateDocument(c *gin.Context) {
	if h.ingestion == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RAG ingestion engine not initialized"})
		return
	}

	var req struct {
		Title      string            `json:"title" binding:"required"`
		Source     string            `json:"source"`
		SourceType string            `json:"source_type"`
		Content    string            `json:"content" binding:"required"`
		Metadata   map[string]string `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	source := req.Source
	if source == "" {
		source = "custom"
	}
	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = "documentation"
	}

	meta := req.Metadata
	if meta == nil {
		meta = make(map[string]string)
	}
	meta["title"] = req.Title

	doc := &ai.Document{
		ID:       "custom-" + sanitizeID(req.Title),
		Source:   source,
		Type:     sourceType,
		Content:  req.Content,
		Metadata: meta,
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	if err := h.ingestion.IngestDocument(ctx, doc, h.tenantID); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps(), c, "rag_create", "rag_document", doc.ID, nil, gin.H{
		"title": req.Title, "source": source,
	})

	c.JSON(http.StatusCreated, gin.H{
		"message": "Document created and indexed",
		"id":      doc.ID,
		"title":   req.Title,
	})
}

// sanitizeID creates a safe document ID from a title.
func sanitizeID(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r == ' ' {
			return '-'
		}
		return -1
	}, s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// toStringMap converts map[string]interface{} to map[string]string.
func toStringMap(m map[string]interface{}) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case string:
			result[k] = val
		default:
			result[k] = fmt.Sprintf("%v", v)
		}
	}
	return result
}

// GetStats returns knowledge base statistics.
func (h *RAGHandlers) GetStats(c *gin.Context) {
	stats, err := h.ragRepo.GetDocumentStats(c.Request.Context(), h.tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	providers := h.aiManager.ListProviders()

	// Sum all source types dynamically (includes pepa-docs, custom, etc.)
	totalDocs := 0
	for k, v := range stats {
		if !strings.HasPrefix(k, "_") {
			totalDocs += v
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"stats":           stats,
		"total_documents": totalDocs,
		"total_chunks":    stats["_chunks"],
		"ai_providers":    len(providers),
		"rag_enabled":     h.ingestion != nil && len(providers) > 0,
	})
}

// Reindex triggers a full re-index of all sources.
func (h *RAGHandlers) Reindex(c *gin.Context) {
	if h.ingestion == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RAG ingestion engine not initialized"})
		return
	}

	var req struct {
		Sources []string `json:"sources"` // optional: limit to specific sources
	}
	_ = c.ShouldBindJSON(&req)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	// Run re-index in background to avoid blocking
	go func() {
		if err := ai.IngestAll(ctx, h.ingestion, h.tenantID); err != nil {
			slog.Warn("RAG: manual re-index failed", "error", err)
		}
	}()

	logAudit(h.deps(), c, "rag_reindex", "rag_document", "all", nil, gin.H{"sources": req.Sources})

	c.JSON(http.StatusAccepted, gin.H{
		"message": "Re-indexing started in background",
	})
}

// ChatWithRAG handles AI chat with RAG context injection.
func (h *RAGHandlers) ChatWithRAG(c *gin.Context) {
	if h.pipeline == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RAG pipeline not initialized"})
		return
	}

	var req struct {
		Message     string            `json:"message" binding:"required"`
		TopK        int               `json:"top_k"`
		EnableTools bool              `json:"enable_tools"`
		Filters     map[string]string `json:"filters"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.TopK <= 0 {
		req.TopK = 10
	}

	query := &ai.RAGQuery{
		Text:        req.Message,
		TenantID:    h.tenantID.String(),
		TopK:        req.TopK,
		EnableTools: req.EnableTools,
		Filters:     req.Filters,
	}

	resp, err := h.pipeline.Query(c.Request.Context(), query)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps(), c, "rag_chat", "ai_message", uuid.NewString(), nil, map[string]interface{}{
		"query":      req.Message,
		"sources":    len(resp.Sources),
		"tokens_used": resp.TokensUsed.TotalTokens,
	})

	c.JSON(http.StatusOK, gin.H{
		"response":    resp.Answer,
		"sources":     resp.Sources,
		"tokens_used": resp.TokensUsed,
	})
}

// ChatStreamWithRAG handles streaming AI chat with RAG context.
func (h *RAGHandlers) ChatStreamWithRAG(c *gin.Context) {
	if h.pipeline == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RAG pipeline not initialized"})
		return
	}

	var req struct {
		Message     string            `json:"message" binding:"required"`
		TopK        int               `json:"top_k"`
		EnableTools bool              `json:"enable_tools"`
		Filters     map[string]string `json:"filters"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.TopK <= 0 {
		req.TopK = 10
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	query := &ai.RAGQuery{
		Text:        req.Message,
		TenantID:    h.tenantID.String(),
		TopK:        req.TopK,
		EnableTools: req.EnableTools,
		Filters:     req.Filters,
	}

	stream, err := h.pipeline.StreamQuery(c.Request.Context(), query)
	if err != nil {
		_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", mustJSON(gin.H{"type": "error", "error": err.Error()}))
		flusher.Flush()
		return
	}

	for chunk := range stream {
		if chunk.Type == "text" && chunk.Content != "" {
			chunk.Content = ai.StripThinkBlocks(chunk.Content)
			if chunk.Content == "" {
				continue
			}
		}
		_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", mustJSON(chunk))
		flusher.Flush()
	}
}

// deps returns the Dependencies for audit logging.
// This is a workaround since RAGHandlers doesn't store deps directly.
func (h *RAGHandlers) deps() Dependencies {
	return Dependencies{}
}
