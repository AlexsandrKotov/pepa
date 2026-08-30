package ai

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/pepa/pepa/docs"
	"github.com/pepa/pepa/internal/repository"
)

// SeedDocuments reads embedded documentation and ingests it into the RAG
// knowledge base. It is called once at startup (after RAG init) and is
// idempotent — documents with the same source identity are upserted.
func SeedDocuments(ctx context.Context, engine *IngestionEngine, tenantID uuid.UUID) error {
	ingested := 0

	// 1. Seed focused RAG docs (rag-seed/*.md)
	if err := seedFromFS(ctx, engine, docs.SeedDocs, "rag-seed", "documentation", tenantID, &ingested); err != nil {
		slog.Warn("RAG: failed to seed focused docs", "error", err)
	}

	// 2. Seed full PEPA documentation (*.md in docs root)
	if err := seedFromFS(ctx, engine, docs.AllDocs, "", "documentation", tenantID, &ingested); err != nil {
		slog.Warn("RAG: failed to seed full docs", "error", err)
	}

	if ingested > 0 {
		slog.Info("RAG: documentation seeded into knowledge base", "documents", ingested)
	}
	return nil
}

// seedFromFS reads markdown files from an embedded filesystem and ingests them.
func seedFromFS(ctx context.Context, engine *IngestionEngine, fsys fs.FS, subdir string, sourceType string, tenantID uuid.UUID, ingested *int) error {
	root := subdir
	if root == "" {
		root = "."
	}

	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return fmt.Errorf("read dir %q: %w", root, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := entry.Name()
		if subdir != "" {
			path = subdir + "/" + entry.Name()
		}

		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			slog.Warn("RAG: failed to read seed doc", "path", path, "error", err)
			continue
		}

		// Extract title from first heading
		title := extractTitle(string(content))
		if title == "" {
			title = entry.Name()
		}

		doc := &Document{
			ID:      "doc-" + strings.TrimSuffix(entry.Name(), ".md"),
			Source:  "pepa-docs",
			Type:    sourceType,
			Content: string(content),
			Metadata: map[string]string{
				"title":    title,
				"filename": entry.Name(),
				"path":     path,
			},
		}

		if err := engine.IngestDocument(ctx, doc, tenantID); err != nil {
			slog.Warn("RAG: failed to ingest seed doc", "path", path, "error", err)
			continue
		}
		*ingested++
	}

	return nil
}

// extractTitle extracts the first markdown heading (# Title) from content.
func extractTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

// SeedDocumentList returns metadata about all seed documents without ingesting.
// Useful for the frontend to show what's in the knowledge base.
func SeedDocumentList() []map[string]string {
	var result []map[string]string

	addFromFS := func(fsys fs.FS, subdir string) {
		root := subdir
		if root == "" {
			root = "."
		}
		entries, err := fs.ReadDir(fsys, root)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			content, err := fs.ReadFile(fsys, entry.Name())
			if err != nil {
				continue
			}
			if subdir != "" {
				content, _ = fs.ReadFile(fsys, subdir+"/"+entry.Name())
			}
			title := extractTitle(string(content))
			if title == "" {
				title = entry.Name()
			}
			result = append(result, map[string]string{
				"title":    title,
				"filename": entry.Name(),
				"source":   "pepa-docs",
			})
		}
	}

	addFromFS(docs.SeedDocs, "rag-seed")
	addFromFS(docs.AllDocs, "")

	return result
}

// IngestCustomDocument allows a user to add a custom document to the knowledge base.
// This is a convenience wrapper around IngestionEngine.IngestDocument.
func IngestCustomDocument(ctx context.Context, engine *IngestionEngine, tenantID uuid.UUID, title, sourceType, content string) error {
	doc := &Document{
		ID:     "custom-" + sanitizeID(title),
		Source: "custom",
		Type:   sourceType,
		Content: content,
		Metadata: map[string]string{
			"title": title,
		},
	}
	return engine.IngestDocument(ctx, doc, tenantID)
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
	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// GetRAGRepository returns the underlying RAG repository from the ingestion engine.
// Used by handlers that need direct repository access.
func (e *IngestionEngine) GetRAGRepository() *repository.RAGRepository {
	return e.ragRepo
}
