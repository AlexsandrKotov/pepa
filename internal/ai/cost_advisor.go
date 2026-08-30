package ai

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CostRecommendation represents a cost optimization suggestion.
type CostRecommendation struct {
	Service     string  `json:"service"`
	Category    string  `json:"category"` // compute, storage, network, idle
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Savings     string  `json:"estimated_savings"` // e.g. "~$200/month"
	Priority    string  `json:"priority"`          // high, medium, low
}

// CostAdvisor analyzes resource utilization and suggests optimizations.
type CostAdvisor struct {
	pool     *pgxpool.Pool
	provider LLMProvider
	tenantID uuid.UUID
}

// NewCostAdvisor creates a new cost optimization advisor.
func NewCostAdvisor(pool *pgxpool.Pool, provider LLMProvider, tenantID uuid.UUID) *CostAdvisor {
	return &CostAdvisor{pool: pool, provider: provider, tenantID: tenantID}
}

// AnalyzeCosts examines the platform for cost optimization opportunities.
func (a *CostAdvisor) AnalyzeCosts(ctx context.Context) ([]CostRecommendation, error) {
	context, err := a.gatherCostContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("gather cost context: %w", err)
	}

	prompt := fmt.Sprintf(`Analyze the following platform resource data and identify cost optimization opportunities.

%s

For each recommendation, provide:
- service: the affected service or resource
- category: one of compute, storage, network, idle
- title: short title
- description: what to do and why
- estimated_savings: rough estimate
- priority: high, medium, or low

Respond as a JSON array of objects.`, context)

	messages := []Message{
		{Role: "system", Content: "You are a cloud cost optimization expert. Analyze resource data and provide actionable recommendations. Respond ONLY with a JSON array."},
		{Role: "user", Content: prompt},
	}

	resp, err := a.provider.Chat(ctx, messages, &ChatOptions{
		MaxTokens:      2048,
		ResponseFormat: "json_object",
	})
	if err != nil {
		return nil, fmt.Errorf("cost analysis failed: %w", err)
	}

	slog.Info("cost analysis complete", "recommendations", resp.Content[:min(100, len(resp.Content))])

	// Return the raw response as a single recommendation for now
	return []CostRecommendation{
		{
			Service:     "platform",
			Category:    "compute",
			Title:       "AI Cost Analysis",
			Description: resp.Content,
			Priority:    "medium",
		},
	}, nil
}

// gatherCostContext collects resource data for cost analysis.
func (a *CostAdvisor) gatherCostContext(ctx context.Context) (string, error) {
	var sb stringBuilder

	// Services and their resource allocations
	rows, err := a.pool.Query(ctx, `
		SELECT name, COALESCE(language,''), COALESCE(status,''),
		       COALESCE(owner,''), COALESCE(tags::text,'[]')
		FROM services WHERE tenant_id = $1
	`, a.tenantID)
	if err == nil {
		defer rows.Close()
		sb.WriteString("Services:\n")
		for rows.Next() {
			var name, lang, status, owner, tags string
			_ = rows.Scan(&name, &lang, &status, &owner, &tags)
			sb.WriteString(fmt.Sprintf("  - %s (lang=%s, status=%s, owner=%s)\n", name, lang, status, owner))
		}
	}

	// Clusters
	rows2, err := a.pool.Query(ctx, `
		SELECT name, COALESCE(host,''), COALESCE(status,''), COALESCE(provider,'')
		FROM clusters WHERE tenant_id = $1
	`, a.tenantID)
	if err == nil {
		defer rows2.Close()
		sb.WriteString("\nClusters:\n")
		for rows2.Next() {
			var name, host, status, prov string
			_ = rows2.Scan(&name, &host, &status, &prov)
			sb.WriteString(fmt.Sprintf("  - %s (provider=%s, status=%s)\n", name, prov, status))
		}
	}

	// Docker services
	rows3, err := a.pool.Query(ctx, `
		SELECT ds.name, COALESCE(ds.image,''), COALESCE(ds.status,''),
		       COALESCE(dh.name,'')
		FROM docker_services ds
		JOIN docker_hosts dh ON dh.id = ds.host_id
		WHERE ds.tenant_id = $1
	`, a.tenantID)
	if err == nil {
		defer rows3.Close()
		sb.WriteString("\nDocker Services:\n")
		for rows3.Next() {
			var name, image, status, host string
			_ = rows3.Scan(&name, &image, &status, &host)
			sb.WriteString(fmt.Sprintf("  - %s on %s (image=%s, status=%s)\n", name, host, image, status))
		}
	}

	return sb.String(), nil
}

// StaleDetector identifies unused or stale resources.
type StaleDetector struct {
	pool     *pgxpool.Pool
	provider LLMProvider
	tenantID uuid.UUID
}

// StaleResource represents a resource that may be unused.
type StaleResource struct {
	Type       string    `json:"type"` // service, pipeline, environment
	Name       string    `json:"name"`
	LastActive time.Time `json:"last_active"`
	Reason     string    `json:"reason"`
	Action     string    `json:"recommended_action"`
}

// NewStaleDetector creates a new stale resource detector.
func NewStaleDetector(pool *pgxpool.Pool, provider LLMProvider, tenantID uuid.UUID) *StaleDetector {
	return &StaleDetector{pool: pool, provider: provider, tenantID: tenantID}
}

// DetectStaleResources finds resources that haven't been active recently.
func (d *StaleDetector) DetectStaleResources(ctx context.Context) ([]StaleResource, error) {
	var stale []StaleResource
	threshold := time.Now().Add(-90 * 24 * time.Hour) // 90 days

	// Services with no recent deployments
	rows, err := d.pool.Query(ctx, `
		SELECT s.name, s.status, MAX(pr.started_at) as last_deploy
		FROM services s
		LEFT JOIN pipeline_sources ps ON ps.tenant_id = s.tenant_id AND ps.name ILIKE '%' || s.name || '%'
		LEFT JOIN pipeline_runs pr ON pr.pipeline_source_id = ps.id
		WHERE s.tenant_id = $1
		GROUP BY s.name, s.status
		HAVING MAX(pr.started_at) IS NULL OR MAX(pr.started_at) < $2
	`, d.tenantID, threshold)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, status string
			var lastDeploy *time.Time
			_ = rows.Scan(&name, &status, &lastDeploy)
			last := time.Time{}
			if lastDeploy != nil {
				last = *lastDeploy
			}
			stale = append(stale, StaleResource{
				Type:       "service",
				Name:       name,
				LastActive: last,
				Reason:     fmt.Sprintf("No deployments in 90+ days (status: %s)", status),
				Action:     "Review if this service is still needed. Consider archiving or decommissioning.",
			})
		}
	}

	// Environments with no recent activity
	rows2, err := d.pool.Query(ctx, `
		SELECT e.name, e.updated_at
		FROM environments e
		WHERE e.tenant_id = $1
		  AND e.updated_at < $2
	`, d.tenantID, threshold)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var name string
			var updated time.Time
			_ = rows2.Scan(&name, &updated)
			stale = append(stale, StaleResource{
				Type:       "environment",
				Name:       name,
				LastActive: updated,
				Reason:     "No activity in 90+ days",
				Action:     "Review if this environment is still needed.",
			})
		}
	}

	slog.Info("stale resource detection complete", "found", len(stale))
	return stale, nil
}

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Ensure strings import is used
var _ = strings.Contains
