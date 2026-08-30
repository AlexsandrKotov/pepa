package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/repository"
)

// RAGPipeline orchestrates the Retrieval-Augmented Generation flow.
type RAGPipeline struct {
	embedder  LLMProvider
	generator LLMProvider
	tools     *ToolRegistry
	ragRepo   *repository.RAGRepository
	tenantID  uuid.UUID
}

// NewRAGPipeline creates a new RAG pipeline.
func NewRAGPipeline(embedder, generator LLMProvider, tools *ToolRegistry, ragRepo *repository.RAGRepository, tenantID uuid.UUID) *RAGPipeline {
	return &RAGPipeline{
		embedder:  embedder,
		generator: generator,
		tools:     tools,
		ragRepo:   ragRepo,
		tenantID:  tenantID,
	}
}

// Query executes a RAG query with hybrid retrieval (vector + keyword + RRF fusion).
func (p *RAGPipeline) Query(ctx context.Context, query *RAGQuery) (*RAGResponse, error) {
	// 1. Retrieve relevant chunks via hybrid search
	results, err := p.hybridSearch(ctx, query.Text, query.TopK, query.Filters)
	if err != nil {
		return nil, fmt.Errorf("hybrid search: %w", err)
	}

	if len(results) == 0 {
		// No relevant context found — fall back to regular chat
		slog.Debug("RAG: no relevant chunks found, falling back to plain chat")
		return p.plainChat(ctx, query)
	}

	// 2. Build context from retrieved chunks
	contextText := p.buildContext(results)

	// 3. Generate response with context
	systemPrompt := buildRAGSystemPrompt(contextText, results)

	messages := []Message{
		{Role: "system", Content: systemPrompt},
	}
	if len(query.ConversationID) > 0 {
		// Could load conversation history here; for now keep it simple
	}
	messages = append(messages, Message{Role: "user", Content: query.Text})

	opts := &ChatOptions{
		MaxTokens: 4096,
	}

	// If tools are enabled, include them
	if query.EnableTools && p.tools != nil {
		selectedTools := selectToolsForMessage(p.tools, query.Text)
		if len(selectedTools) > 0 {
			opts.Tools = selectedTools
			opts.ToolChoice = "auto"
		}
	}

	resp, err := p.generator.Chat(ctx, messages, opts)
	if err != nil {
		return nil, fmt.Errorf("RAG generation failed: %w", err)
	}

	// 4. Build citations from sources
	citations := buildCitations(results)

	return &RAGResponse{
		Answer:     StripThinkBlocks(resp.Content),
		Sources:    citations,
		TokensUsed: resp.TokensUsed,
	}, nil
}

// StreamQuery executes a streaming RAG query.
func (p *RAGPipeline) StreamQuery(ctx context.Context, query *RAGQuery) (<-chan *StreamChunk, error) {
	// 1. Retrieve relevant chunks
	results, err := p.hybridSearch(ctx, query.Text, query.TopK, query.Filters)
	if err != nil {
		return nil, fmt.Errorf("hybrid search: %w", err)
	}

	ch := make(chan *StreamChunk, 32)

	go func() {
		defer close(ch)

		// Send citation events first
		if len(results) > 0 {
			citations := buildCitations(results)
			for _, cit := range citations {
				citMap := map[string]interface{}{
					"document_id": cit.DocumentID,
					"source":      cit.Source,
					"source_type": cit.Source,
					"score":       cit.Score,
				}
				citJSON, err := json.Marshal(citMap)
				if err != nil {
					continue
				}
				select {
				case ch <- &StreamChunk{Type: "citation", Content: string(citJSON)}:
				case <-ctx.Done():
					return
				}
			}
		}

		if len(results) == 0 {
			// No context — stream a plain response
			p.streamPlainChat(ctx, query, ch)
			return
		}

		// 2. Build context and generate
		contextText := p.buildContext(results)
		systemPrompt := buildRAGSystemPrompt(contextText, results)

		messages := []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: query.Text},
		}

		opts := &ChatOptions{MaxTokens: 4096}
		if query.EnableTools && p.tools != nil {
			selectedTools := selectToolsForMessage(p.tools, query.Text)
			if len(selectedTools) > 0 {
				opts.Tools = selectedTools
				opts.ToolChoice = "auto"
			}
		}

		stream, err := p.generator.Stream(ctx, messages, opts)
		if err != nil {
			select {
			case ch <- &StreamChunk{Type: "error", Content: err.Error()}:
			case <-ctx.Done():
			}
			return
		}

		for chunk := range stream {
			if chunk.Type == "text" && chunk.Content != "" {
				chunk.Content = StripThinkBlocks(chunk.Content)
				if chunk.Content == "" {
					continue
				}
			}
			select {
			case ch <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// hybridSearch combines vector and keyword search with Reciprocal Rank Fusion.
func (p *RAGPipeline) hybridSearch(ctx context.Context, query string, topK int, filters map[string]string) ([]repository.RAGSearchResult, error) {
	if topK <= 0 {
		topK = 10
	}

	// Fetch more candidates than needed for fusion
	candidateK := topK * 3

	// Run vector and keyword search in parallel
	type searchResult struct {
		results []repository.RAGSearchResult
		err     error
	}

	vectorCh := make(chan searchResult, 1)
	keywordCh := make(chan searchResult, 1)

	// Vector search
	go func() {
		if p.embedder == nil {
			vectorCh <- searchResult{nil, nil}
			return
		}
		embedResp, err := p.embedder.Embed(ctx, []string{query}, &EmbedOptions{})
		if err != nil {
			slog.Warn("RAG: embedding failed", "error", err)
			vectorCh <- searchResult{nil, err}
			return
		}
		if len(embedResp.Vectors) == 0 {
			vectorCh <- searchResult{nil, nil}
			return
		}
		results, err := p.ragRepo.VectorSearch(ctx, embedResp.Vectors[0], p.tenantID, candidateK, filters)
		vectorCh <- searchResult{results, err}
	}()

	// Keyword search
	go func() {
		results, err := p.ragRepo.KeywordSearch(ctx, query, p.tenantID, candidateK, filters)
		keywordCh <- searchResult{results, err}
	}()

	vectorRes := <-vectorCh
	keywordRes := <-keywordCh

	// Handle errors gracefully — use whatever results we have
	var vectorResults, keywordResults []repository.RAGSearchResult
	if vectorRes.err == nil {
		vectorResults = vectorRes.results
	}
	if keywordRes.err == nil {
		keywordResults = keywordRes.results
	}

	// If both failed, return empty
	if len(vectorResults) == 0 && len(keywordResults) == 0 {
		return nil, nil
	}

	// If only one succeeded, use it directly
	if len(vectorResults) == 0 {
		return keywordResults[:min(topK, len(keywordResults))], nil
	}
	if len(keywordResults) == 0 {
		return vectorResults[:min(topK, len(vectorResults))], nil
	}

	// Reciprocal Rank Fusion
	return reciprocalRankFusion(vectorResults, keywordResults, topK), nil
}

// reciprocalRankFusion merges two ranked lists using RRF scoring.
func reciprocalRankFusion(vectorResults, keywordResults []repository.RAGSearchResult, topK int) []repository.RAGSearchResult {
	const k = 60.0 // RRF constant

	scores := make(map[string]float64)  // chunk_id -> RRF score
	resultMap := make(map[string]repository.RAGSearchResult)

	// Score vector results
	for rank, r := range vectorResults {
		rrfScore := 1.0 / (k + float64(rank+1))
		scores[r.ChunkID.String()] += rrfScore
		resultMap[r.ChunkID.String()] = r
	}

	// Score keyword results
	for rank, r := range keywordResults {
		rrfScore := 1.0 / (k + float64(rank+1))
		scores[r.ChunkID.String()] += rrfScore
		if _, exists := resultMap[r.ChunkID.String()]; !exists {
			resultMap[r.ChunkID.String()] = r
		}
	}

	// Sort by RRF score descending
	type scoredResult struct {
		chunkID string
		score   float64
	}
	var sorted []scoredResult
	for id, score := range scores {
		sorted = append(sorted, scoredResult{id, score})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	// Build final results
	var results []repository.RAGSearchResult
	for _, sr := range sorted {
		if len(results) >= topK {
			break
		}
		r := resultMap[sr.chunkID]
		r.Score = sr.score
		results = append(results, r)
	}

	return results
}

// buildContext assembles retrieved chunks into a context string.
func (p *RAGPipeline) buildContext(results []repository.RAGSearchResult) string {
	var sb strings.Builder
	sb.WriteString("RELEVANT KNOWLEDGE BASE CONTENT:\n\n")

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("--- Source %d [%s/%s] (score: %.3f) ---\n",
			i+1, r.Source, r.SourceType, r.Score))
		sb.WriteString(r.Content)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// buildCitations creates citation objects from search results.
func buildCitations(results []repository.RAGSearchResult) []Citation {
	seen := make(map[string]bool)
	var citations []Citation

	for _, r := range results {
		docID := r.DocumentID.String()
		if seen[docID] {
			continue
		}
		seen[docID] = true
		content := r.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		citations = append(citations, Citation{
			DocumentID: docID,
			Source:     r.Source,
			Content:    content,
			Score:      math.Round(r.Score*1000) / 1000,
			URL:        r.SourceURL,
		})
	}
	return citations
}

// buildRAGSystemPrompt creates the system prompt with RAG context.
func buildRAGSystemPrompt(contextText string, results []repository.RAGSearchResult) string {
	var sb strings.Builder
	sb.WriteString(`You are the PEPA AI Assistant with access to the platform's knowledge base.

RULES:
- Use the provided knowledge base content to answer questions accurately.
- Always cite your sources when referencing specific information.
- If the knowledge base doesn't contain relevant information, say so honestly.
- Be concise and structured. Use bullet points and code blocks.
- Respond in the SAME LANGUAGE the user used.
- When suggesting actions, mention which PEPA tools can execute them.

`)
	sb.WriteString(contextText)
	return sb.String()
}

// plainChat falls back to regular chat when no RAG context is found.
func (p *RAGPipeline) plainChat(ctx context.Context, query *RAGQuery) (*RAGResponse, error) {
	messages := []Message{
		{Role: "system", Content: "You are the PEPA AI Assistant. The knowledge base has no relevant information for this query. Answer based on your general knowledge of platform engineering, Kubernetes, CI/CD, and DevOps. Be honest if you don't know something. Respond in the same language the user used."},
		{Role: "user", Content: query.Text},
	}

	resp, err := p.generator.Chat(ctx, messages, &ChatOptions{MaxTokens: 4096})
	if err != nil {
		return nil, err
	}

	return &RAGResponse{
		Answer:     StripThinkBlocks(resp.Content),
		TokensUsed: resp.TokensUsed,
	}, nil
}

// streamPlainChat streams a plain chat response when no RAG context is found.
func (p *RAGPipeline) streamPlainChat(ctx context.Context, query *RAGQuery, ch chan<- *StreamChunk) {
	messages := []Message{
		{Role: "system", Content: "You are the PEPA AI Assistant. The knowledge base has no relevant information for this query. Answer based on your general knowledge. Respond in the same language the user used."},
		{Role: "user", Content: query.Text},
	}

	stream, err := p.generator.Stream(ctx, messages, &ChatOptions{MaxTokens: 4096})
	if err != nil {
		select {
		case ch <- &StreamChunk{Type: "error", Content: err.Error()}:
		case <-ctx.Done():
		}
		return
	}

	for chunk := range stream {
		if chunk.Type == "text" {
			chunk.Content = StripThinkBlocks(chunk.Content)
			if chunk.Content == "" {
				continue
			}
		}
		select {
		case ch <- chunk:
		case <-ctx.Done():
			return
		}
	}
}
