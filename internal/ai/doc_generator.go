package ai

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DocGenerator auto-generates service documentation using AI.
type DocGenerator struct {
	pool     *pgxpool.Pool
	provider LLMProvider
	tenantID uuid.UUID
}

// NewDocGenerator creates a new documentation generator.
func NewDocGenerator(pool *pgxpool.Pool, provider LLMProvider, tenantID uuid.UUID) *DocGenerator {
	return &DocGenerator{pool: pool, provider: provider, tenantID: tenantID}
}

// GeneratedDoc represents AI-generated documentation.
type GeneratedDoc struct {
	ServiceName string    `json:"service_name"`
	Content     string    `json:"content"`
	GeneratedAt time.Time `json:"generated_at"`
	Model       string    `json:"model"`
}

// GenerateServiceDocs creates comprehensive documentation for a service.
func (g *DocGenerator) GenerateServiceDocs(ctx context.Context, serviceName string) (*GeneratedDoc, error) {
	context, err := g.gatherServiceContext(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("gather context: %w", err)
	}

	prompt := fmt.Sprintf(`Generate comprehensive documentation for the following service.

%s

Include these sections:
1. **Overview** - What the service does and its purpose
2. **Architecture** - Dependencies, upstream/downstream services
3. **Configuration** - Key environment variables and settings
4. **API Endpoints** - If applicable
5. **Deployment** - How it's deployed, environments
6. **Monitoring** - Key metrics and alerts to watch
7. **Runbook** - Common issues and how to resolve them
8. **SLIs/SLOs** - Suggested service level indicators

Format as Markdown. Be specific and actionable.`, context)

	messages := []Message{
		{Role: "system", Content: "You are a technical documentation writer for a platform engineering system. Generate clear, actionable documentation."},
		{Role: "user", Content: prompt},
	}

	resp, err := g.provider.Chat(ctx, messages, &ChatOptions{MaxTokens: 4096})
	if err != nil {
		return nil, fmt.Errorf("doc generation failed: %w", err)
	}

	doc := &GeneratedDoc{
		ServiceName: serviceName,
		Content:     resp.Content,
		GeneratedAt: time.Now(),
		Model:       resp.ModelUsed,
	}

	// Store the generated docs in the service metadata
	_, _ = g.pool.Exec(ctx, `
		UPDATE services
		SET metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object(
			'ai_documentation', $1::text,
			'ai_docs_generated_at', NOW()::text,
			'ai_docs_model', $2::text
		)
		WHERE tenant_id = $3 AND name ILIKE $4
	`, doc.Content, doc.Model, g.tenantID, "%"+serviceName+"%")

	slog.Info("service documentation generated", "service", serviceName, "model", doc.Model)
	return doc, nil
}

// gatherServiceContext collects data about a service for documentation.
func (g *DocGenerator) gatherServiceContext(ctx context.Context, serviceName string) (string, error) {
	var sb stringBuilder

	// Service info
	var name, desc, lang, fw, owner, status string
	err := g.pool.QueryRow(ctx, `
		SELECT name, COALESCE(description,''), COALESCE(language,''),
		       COALESCE(framework,''), COALESCE(owner,''), COALESCE(status,'')
		FROM services WHERE tenant_id = $1 AND name ILIKE $2 LIMIT 1
	`, g.tenantID, "%"+serviceName+"%").Scan(&name, &desc, &lang, &fw, &owner, &status)

	if err == nil {
		sb.WriteString(fmt.Sprintf("Service: %s\nDescription: %s\nLanguage: %s\nFramework: %s\nOwner: %s\nStatus: %s\n",
			name, desc, lang, fw, owner, status))
	}

	// Entity relationships
	rows, err := g.pool.Query(ctx, `
		SELECT rt.display_name, te.name, te.type_key
		FROM entity_relationships er
		JOIN entities se ON se.id = er.source_id
		JOIN entities te ON te.id = er.target_id
		LEFT JOIN relationship_types rt ON rt.id = er.relationship_type_id
		WHERE se.name ILIKE $1 OR te.name ILIKE $1
	`, "%"+serviceName+"%")
	if err == nil {
		defer rows.Close()
		sb.WriteString("\nRelationships:\n")
		for rows.Next() {
			var relType, targetName, targetType string
			_ = rows.Scan(&relType, &targetName, &targetType)
			sb.WriteString(fmt.Sprintf("  - %s → %s (%s)\n", relType, targetName, targetType))
		}
	}

	// Recent deployments
	rows2, err := g.pool.Query(ctx, `
		SELECT pr.status, pr.started_at
		FROM pipeline_runs pr
		JOIN pipeline_sources ps ON ps.id = pr.pipeline_source_id
		WHERE ps.tenant_id = $1 AND ps.name ILIKE $2
		ORDER BY pr.started_at DESC LIMIT 5
	`, g.tenantID, "%"+serviceName+"%")
	if err == nil {
		defer rows2.Close()
		sb.WriteString("\nRecent deployments:\n")
		for rows2.Next() {
			var status string
			var started time.Time
			_ = rows2.Scan(&status, &started)
			sb.WriteString(fmt.Sprintf("  - %s at %s\n", status, started.Format(time.RFC3339)))
		}
	}

	return sb.String(), nil
}
