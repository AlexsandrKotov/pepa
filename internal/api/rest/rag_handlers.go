package rest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/ai"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/pkg/utils"
)

// RAGHandlers handles RAG knowledge base endpoints.
type RAGHandlers struct {
	ragRepo    *repository.RAGRepository
	aiManager  *ai.Manager
	ingestion  *ai.IngestionEngine
	pipeline   *ai.RAGPipeline
	deps       Dependencies
	// platformTenant owns the seeded, read-only corpus (PEPA docs, runbooks).
	// It is a read scope for every workspace, never a write target for a
	// caller whose token carries a different tenant.
	platformTenant uuid.UUID
}

// NewRAGHandlers creates new RAG handlers.
func NewRAGHandlers(ragRepo *repository.RAGRepository, aiMgr *ai.Manager, platformTenant uuid.UUID, deps Dependencies) *RAGHandlers {
	return &RAGHandlers{
		ragRepo:        ragRepo,
		aiManager:      aiMgr,
		platformTenant: platformTenant,
		deps:           deps,
	}
}

// readScope is the set of tenants whose documents the caller may read: their own
// workspace plus the platform-seeded corpus. The tenant always comes from the
// verified JWT, never from a request parameter.
func (h *RAGHandlers) readScope(c *gin.Context) []uuid.UUID {
	return repository.TenantScope(auth.GetTenantID(c), h.platformTenant)
}

// writeTenant is the single tenant the caller may create, update or delete
// documents in. Platform admins keep managing the seeded corpus in the platform
// tenant; everyone else can only touch their own workspace's rows.
func (h *RAGHandlers) writeTenant(c *gin.Context) uuid.UUID {
	return auth.GetTenantID(c)
}

// respondRAGError maps the not-found sentinel to 404 and anything else to a
// generic 500, so a foreign or deleted document UUID never surfaces as a
// database error.
func respondRAGError(c *gin.Context, err error) {
	if errors.Is(err, repository.ErrRAGNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
		return
	}
	respondInternalError(c, err)
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

	tenantID := h.writeTenant(c)
	if err := h.ingestion.IngestDocument(ctx, doc, tenantID); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "rag_ingest", "rag_document", doc.ID, nil, gin.H{
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
	scope := h.readScope(c)
	var results []repository.RAGSearchResult
	var err error

	// embedQuery turns the query text into a vector using the default provider.
	// It is best-effort: without a configured embedding model the callers fall
	// back to keyword-only retrieval instead of failing the request.
	embedQuery := func() []float32 {
		provider, pErr := h.aiManager.DefaultProvider()
		if pErr != nil {
			return nil
		}
		embedResp, eErr := provider.Embed(ctx, []string{req.Query}, &ai.EmbedOptions{})
		if eErr != nil || len(embedResp.Vectors) == 0 {
			if eErr != nil {
				slog.Warn("RAG: query embedding failed", "error", eErr)
			}
			return nil
		}
		return embedResp.Vectors[0]
	}

	switch req.Mode {
	case "keyword":
		results, err = h.ragRepo.KeywordSearch(ctx, req.Query, scope, req.TopK, req.Filters)
	case "vector":
		queryVector := embedQuery()
		if queryVector == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI provider not configured"})
			return
		}
		results, err = h.ragRepo.VectorSearch(ctx, queryVector, scope, req.TopK, req.Filters)
	default: // hybrid — vector half is optional, keyword half always runs
		var vectorResults []repository.RAGSearchResult
		if queryVector := embedQuery(); queryVector != nil {
			vectorResults, err = h.ragRepo.VectorSearch(ctx, queryVector, scope, req.TopK, req.Filters)
		}
		keywordResults, kErr := h.ragRepo.KeywordSearch(ctx, req.Query, scope, req.TopK, req.Filters)
		if kErr != nil {
			if err == nil {
				err = kErr
			}
		} else {
			vectorResults = mergeSearchResults(vectorResults, keywordResults, req.TopK)
		}
		results = vectorResults
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

// mergeSearchResults concatenates two ranked result lists, dropping duplicate
// chunks (a chunk retrieved by both search halves keeps its better rank) and
// capping the total.
func mergeSearchResults(primary, secondary []repository.RAGSearchResult, topK int) []repository.RAGSearchResult {
	merged := make([]repository.RAGSearchResult, 0, len(primary)+len(secondary))
	seen := make(map[uuid.UUID]bool, len(primary)+len(secondary))
	for _, list := range [][]repository.RAGSearchResult{primary, secondary} {
		for _, r := range list {
			if seen[r.ChunkID] {
				continue
			}
			seen[r.ChunkID] = true
			merged = append(merged, r)
			if topK > 0 && len(merged) >= topK {
				return merged
			}
		}
	}
	return merged
}

// ListDocuments returns all documents in the knowledge base.
func (h *RAGHandlers) ListDocuments(c *gin.Context) {
	source := c.Query("source")
	page, _ := strconv.Atoi(c.Query("page"))
	perPage, _ := strconv.Atoi(c.Query("per_page"))
	page, perPage = repository.ClampPagination(page, perPage, 50)
	offset := (page - 1) * perPage

	docs, total, err := h.ragRepo.ListDocuments(c.Request.Context(), h.readScope(c), source, perPage, offset)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"documents": docs,
		"total":     total,
		"page":      page,
		"per_page":  perPage,
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

	if err := h.ragRepo.DeleteDocument(c.Request.Context(), id, h.writeTenant(c)); err != nil {
		respondRAGError(c, err)
		return
	}

	logAudit(h.deps, c, "rag_delete", "rag_document", idStr, nil, nil)

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

	doc, err := h.ragRepo.GetDocument(c.Request.Context(), id, h.readScope(c))
	if err != nil {
		respondRAGError(c, err)
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

	tenantID := h.writeTenant(c)

	// Update the document content
	if err := h.ragRepo.UpdateDocumentContent(ctx, id, tenantID, req.Content, req.Metadata); err != nil {
		respondRAGError(c, err)
		return
	}

	// Delete old chunks and re-ingest to re-chunk and re-embed
	_ = h.ragRepo.DeleteChunksByDocument(ctx, id)

	// Re-fetch the updated document to get the new content for ingestion
	doc, err := h.ragRepo.GetDocument(ctx, id, repository.TenantScope(tenantID))
	if err != nil {
		respondRAGError(c, err)
		return
	}

	aiDoc := &ai.Document{
		ID:       doc.ID.String(),
		Source:   doc.Source,
		Type:     doc.SourceType,
		Content:  doc.Content,
		Metadata: toStringMap(doc.Metadata),
	}
	if err := h.ingestion.IngestDocument(ctx, aiDoc, tenantID); err != nil {
		slog.Warn("RAG: re-ingestion after update failed", "error", err)
	}

	logAudit(h.deps, c, "rag_update", "rag_document", idStr, nil, gin.H{"content_length": len(req.Content)})

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

	if err := h.ingestion.IngestDocument(ctx, doc, h.writeTenant(c)); err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "rag_create", "rag_document", doc.ID, nil, gin.H{
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
	stats, err := h.ragRepo.GetDocumentStats(c.Request.Context(), h.readScope(c))
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

	tenantID := h.writeTenant(c)
	if tenantID == uuid.Nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "tenant scope required"})
		return
	}

	// Run re-index in background to avoid blocking. The response is 202, so the
	// request context is cancelled while the work is still running: detach from
	// cancellation and give the job its own deadline instead. The request context
	// is read here, before the handler returns and gin reuses the context object.
	reqCtx := c.Request.Context()
	go func() {
		ctx, cancel := utils.DetachContext(reqCtx, 5*time.Minute)
		defer cancel()
		if err := ai.IngestAll(ctx, h.ingestion, tenantID); err != nil {
			slog.Warn("RAG: manual re-index failed", "error", err, "tenant_id", tenantID.String())
		}
	}()

	logAudit(h.deps, c, "rag_reindex", "rag_document", "all", nil, gin.H{"sources": req.Sources})

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
		TenantID:    auth.GetTenantID(c).String(),
		TenantIDs:   h.readScope(c),
		TopK:        req.TopK,
		EnableTools: req.EnableTools,
		Filters:     req.Filters,
	}

	resp, err := h.pipeline.Query(c.Request.Context(), query)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	logAudit(h.deps, c, "rag_chat", "ai_message", uuid.NewString(), nil, map[string]interface{}{
		"query":       req.Message,
		"sources":     len(resp.Sources),
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
		TenantID:    auth.GetTenantID(c).String(),
		TenantIDs:   h.readScope(c),
		TopK:        req.TopK,
		EnableTools: req.EnableTools,
		Filters:     req.Filters,
	}

	stream, err := h.pipeline.StreamQuery(c.Request.Context(), query)
	if err != nil {
		slog.Warn("RAG: streaming query failed", "error", err)
		_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", mustJSON(gin.H{"type": "error", "error": "RAG query failed"}))
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
