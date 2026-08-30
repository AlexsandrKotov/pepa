// Package docs embeds PEPA documentation for RAG knowledge base seeding.
package docs

import "embed"

// SeedDocs contains the RAG seed documents (focused, RAG-optimized guides).
//
//go:embed rag-seed/*.md
var SeedDocs embed.FS

// AllDocs contains all PEPA documentation (architecture, guides, user guides).
//
//go:embed *.md
var AllDocs embed.FS
