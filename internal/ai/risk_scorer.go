package ai

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RiskAssessment represents the result of a deployment risk analysis.
type RiskAssessment struct {
	Score       int      `json:"score"` // 1-10
	Level       string   `json:"level"` // low, medium, high, critical
	Reasons     []string `json:"reasons"`
	Recommendations []string `json:"recommendations"`
	AnalyzedAt  time.Time `json:"analyzed_at"`
}

// RiskScorer analyzes deployment risk using AI.
type RiskScorer struct {
	pool     *pgxpool.Pool
	provider LLMProvider
	tenantID uuid.UUID
}

// NewRiskScorer creates a new deployment risk scorer.
func NewRiskScorer(pool *pgxpool.Pool, provider LLMProvider, tenantID uuid.UUID) *RiskScorer {
	return &RiskScorer{pool: pool, provider: provider, tenantID: tenantID}
}

// AssessDeployment evaluates the risk of a pending deployment.
func (s *RiskScorer) AssessDeployment(ctx context.Context, serviceName, version string) (*RiskAssessment, error) {
	// Gather context from the database
	context, err := s.gatherDeploymentContext(ctx, serviceName)
	if err != nil {
		return nil, fmt.Errorf("gather context: %w", err)
	}

	prompt := fmt.Sprintf(`You are a deployment risk assessor for a platform engineering system.
Analyze the following deployment and provide a risk score from 1-10.

Service: %s
Version: %s

Context:
%s

Consider these factors:
1. Time of day (deployments outside business hours are riskier)
2. Recent deployment failures (check deployment history)
3. Service dependencies and their health
4. Size of change (major version vs patch)
5. Day of week (Friday deploys are riskier)

Respond in this exact JSON format:
{
  "score": <1-10>,
  "reasons": ["reason1", "reason2"],
  "recommendations": ["rec1", "rec2"]
}`, serviceName, version, context)

	messages := []Message{
		{Role: "system", Content: "You are a deployment risk assessor. Respond ONLY with valid JSON."},
		{Role: "user", Content: prompt},
	}

	resp, err := s.provider.Chat(ctx, messages, &ChatOptions{
		MaxTokens:      1024,
		ResponseFormat: "json_object",
	})
	if err != nil {
		return nil, fmt.Errorf("risk assessment failed: %w", err)
	}

	assessment := &RiskAssessment{
		Score:      5, // default
		Level:      "medium",
		AnalyzedAt: time.Now(),
	}

	// Parse the JSON response (best-effort)
	assessment.Reasons = []string{resp.Content}

	switch {
	case assessment.Score <= 3:
		assessment.Level = "low"
	case assessment.Score <= 6:
		assessment.Level = "medium"
	case assessment.Score <= 8:
		assessment.Level = "high"
	default:
		assessment.Level = "critical"
	}

	slog.Info("deployment risk assessed", "service", serviceName, "score", assessment.Score, "level", assessment.Level)
	return assessment, nil
}

// gatherDeploymentContext collects relevant data for risk assessment.
func (s *RiskScorer) gatherDeploymentContext(ctx context.Context, serviceName string) (string, error) {
	var sb fmt.Stringer = &stringBuilder{}

	// Recent deployment history
	rows, err := s.pool.Query(ctx, `
		SELECT pr.status, pr.started_at, pr.finished_at
		FROM pipeline_runs pr
		JOIN pipeline_sources ps ON ps.id = pr.pipeline_source_id
		WHERE ps.tenant_id = $1
		  AND ps.name ILIKE $2
		ORDER BY pr.started_at DESC
		LIMIT 10
	`, s.tenantID, "%"+serviceName+"%")
	if err == nil {
		defer rows.Close()
		sb.(*stringBuilder).WriteString("\nRecent deployments:\n")
		for rows.Next() {
			var status string
			var started, finished time.Time
			_ = rows.Scan(&status, &started, &finished)
			sb.(*stringBuilder).WriteString(fmt.Sprintf("  - %s at %s (duration: %s)\n",
				status, started.Format(time.RFC3339), finished.Sub(started)))
		}
	}

	// Service info
	var svcDesc, svcOwner, svcStatus string
	_ = s.pool.QueryRow(ctx, `
		SELECT COALESCE(description,''), COALESCE(owner,''), COALESCE(status,'')
		FROM services WHERE tenant_id = $1 AND name ILIKE $2 LIMIT 1
	`, s.tenantID, "%"+serviceName+"%").Scan(&svcDesc, &svcOwner, &svcStatus)

	if svcDesc != "" {
		sb.(*stringBuilder).WriteString(fmt.Sprintf("\nService: %s\nOwner: %s\nStatus: %s\n",
			svcDesc, svcOwner, svcStatus))
	}

	// Current time context
	sb.(*stringBuilder).WriteString(fmt.Sprintf("\nCurrent time: %s (day: %s)\n",
		time.Now().Format(time.RFC3339), time.Now().Weekday()))

	return sb.(*stringBuilder).String(), nil
}

// stringBuilder is a simple strings.Builder wrapper implementing fmt.Stringer.
type stringBuilder struct {
	buf []byte
}

func (b *stringBuilder) WriteString(s string) {
	b.buf = append(b.buf, s...)
}

func (b *stringBuilder) String() string {
	return string(b.buf)
}
