package ai

// This file contains the core RAG data types used across the AI package.
// The RAG pipeline implementation is in rag_pipeline.go.
// The ingestion engine is in rag_ingest.go.
// The chunker is in rag_chunker.go.

// Document represents a document for RAG ingestion.
type Document struct {
	ID       string            `json:"id"`
	Source   string            `json:"source"` // kubernetes, logs, entity, documentation, service, pipeline
	Type     string            `json:"type"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata"`
}

// Chunk represents a chunk of a document for embedding.
type Chunk struct {
	ID         string    `json:"id"`
	DocumentID string    `json:"document_id"`
	Content    string    `json:"content"`
	Index      int       `json:"index"`
	Embedding  []float32 `json:"embedding,omitempty"`
}
