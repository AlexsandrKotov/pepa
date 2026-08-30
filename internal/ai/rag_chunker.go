package ai

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Chunker splits documents into chunks for embedding.
type Chunker interface {
	Chunk(doc *Document) ([]*Chunk, error)
}

// SemanticChunker splits documents based on content type and structure.
type SemanticChunker struct {
	TargetSize  int // Target tokens per chunk (approximate by characters / 4)
	OverlapSize int // Overlap characters between chunks
}

// NewSemanticChunker creates a chunker with sensible defaults.
func NewSemanticChunker() *SemanticChunker {
	return &SemanticChunker{
		TargetSize:  400, // ~100 tokens
		OverlapSize: 50,
	}
}

// Chunk splits a document into chunks based on its source type.
func (c *SemanticChunker) Chunk(doc *Document) ([]*Chunk, error) {
	switch doc.Source {
	case "kubernetes":
		return c.chunkK8sResource(doc)
	case "documentation":
		return c.chunkMarkdown(doc)
	case "logs":
		return c.chunkLogGroup(doc)
	default:
		return c.chunkGeneric(doc)
	}
}

// chunkK8sResource splits Kubernetes resources by logical sections.
func (c *SemanticChunker) chunkK8sResource(doc *Document) ([]*Chunk, error) {
	content := doc.Content
	// Small resources fit in a single chunk
	if utf8.RuneCountInString(content) <= c.TargetSize*2 {
		return []*Chunk{{
			ID:         doc.ID + "-0",
			DocumentID: doc.ID,
			Content:    content,
			Index:      0,
		}}, nil
	}

	// Split by YAML top-level keys (metadata, spec, status, etc.)
	var chunks []*Chunk
	sections := splitYAMLSections(content)
	for i, section := range sections {
		chunks = append(chunks, &Chunk{
			ID:         doc.ID + "-" + strconv.Itoa(i),
			DocumentID: doc.ID,
			Content:    section,
			Index:      i,
		})
	}
	if len(chunks) == 0 {
		chunks = append(chunks, &Chunk{
			ID:         doc.ID + "-0",
			DocumentID: doc.ID,
			Content:    content,
			Index:      0,
		})
	}
	return chunks, nil
}

// chunkMarkdown splits markdown by headings while respecting target size.
func (c *SemanticChunker) chunkMarkdown(doc *Document) ([]*Chunk, error) {
	content := doc.Content
	if utf8.RuneCountInString(content) <= c.TargetSize*2 {
		return []*Chunk{{
			ID:         doc.ID + "-0",
			DocumentID: doc.ID,
			Content:    content,
			Index:      0,
		}}, nil
	}

	// Split by markdown headings
	lines := strings.Split(content, "\n")
	var sections []string
	var current strings.Builder

	for _, line := range lines {
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			if current.Len() > 0 {
				sections = append(sections, current.String())
			}
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if current.Len() > 0 {
		sections = append(sections, current.String())
	}

	// Merge small sections, split large ones
	var chunks []*Chunk
	var buf strings.Builder
	chunkIdx := 0

	for _, section := range sections {
		if buf.Len()+len(section) > c.TargetSize*4 && buf.Len() > 0 {
			chunks = append(chunks, &Chunk{
				ID:         doc.ID + "-" + strconv.Itoa(chunkIdx),
				DocumentID: doc.ID,
				Content:    buf.String(),
				Index:      chunkIdx,
			})
			chunkIdx++
			buf.Reset()
		}
		buf.WriteString(section)
	}
	if buf.Len() > 0 {
		chunks = append(chunks, &Chunk{
			ID:         doc.ID + "-" + strconv.Itoa(chunkIdx),
			DocumentID: doc.ID,
			Content:    buf.String(),
			Index:      chunkIdx,
		})
	}

	if len(chunks) == 0 {
		chunks = append(chunks, &Chunk{
			ID:         doc.ID + "-0",
			DocumentID: doc.ID,
			Content:    content,
			Index:      0,
		})
	}
	return chunks, nil
}

// chunkLogGroup splits log groups into time-bounded chunks.
func (c *SemanticChunker) chunkLogGroup(doc *Document) ([]*Chunk, error) {
	lines := strings.Split(doc.Content, "\n")
	var chunks []*Chunk
	chunkIdx := 0
	var buf strings.Builder

	for _, line := range lines {
		if buf.Len()+len(line) > c.TargetSize*4 && buf.Len() > 0 {
			chunks = append(chunks, &Chunk{
				ID:         doc.ID + "-" + strconv.Itoa(chunkIdx),
				DocumentID: doc.ID,
				Content:    buf.String(),
				Index:      chunkIdx,
			})
			chunkIdx++
			buf.Reset()
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	if buf.Len() > 0 {
		chunks = append(chunks, &Chunk{
			ID:         doc.ID + "-" + strconv.Itoa(chunkIdx),
			DocumentID: doc.ID,
			Content:    buf.String(),
			Index:      chunkIdx,
		})
	}

	if len(chunks) == 0 {
		chunks = append(chunks, &Chunk{
			ID:         doc.ID + "-0",
			DocumentID: doc.ID,
			Content:    doc.Content,
			Index:      0,
		})
	}
	return chunks, nil
}

// chunkGeneric splits any content by target size with overlap.
func (c *SemanticChunker) chunkGeneric(doc *Document) ([]*Chunk, error) {
	content := doc.Content
	targetChars := c.TargetSize * 4 // rough approximation: 1 token ≈ 4 chars

	if utf8.RuneCountInString(content) <= targetChars {
		return []*Chunk{{
			ID:         doc.ID + "-0",
			DocumentID: doc.ID,
			Content:    content,
			Index:      0,
		}}, nil
	}

	runes := []rune(content)
	var chunks []*Chunk
	chunkIdx := 0

	for start := 0; start < len(runes); start += targetChars - c.OverlapSize {
		end := start + targetChars
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, &Chunk{
			ID:         doc.ID + "-" + strconv.Itoa(chunkIdx),
			DocumentID: doc.ID,
			Content:    string(runes[start:end]),
			Index:      chunkIdx,
		})
		chunkIdx++
		if end == len(runes) {
			break
		}
	}

	return chunks, nil
}

// splitYAMLSections splits YAML content by top-level keys.
func splitYAMLSections(content string) []string {
	var sections []string
	var current strings.Builder
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		// Top-level key: starts with a non-space character followed by ':'
		if len(line) > 0 && line[0] != ' ' && line[0] != '#' && strings.Contains(line, ":") {
			if current.Len() > 0 {
				sections = append(sections, current.String())
			}
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if current.Len() > 0 {
		sections = append(sections, current.String())
	}
	return sections
}
