package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ============================================================
// Scorecard — Service maturity scoring engine
// ============================================================

type Scorecard struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	TenantID    uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Name        string          `json:"name" db:"name"`
	Description string          `json:"description,omitempty" db:"description"`
	Enabled     bool            `json:"enabled" db:"enabled"`
	Config      json.RawMessage `json:"config,omitempty" db:"config"`
	CreatedBy   *uuid.UUID      `json:"created_by,omitempty" db:"created_by"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

// ScorecardRule — individual scoring rule within a scorecard
type ScorecardRule struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	ScorecardID  uuid.UUID       `json:"scorecard_id" db:"scorecard_id"`
	Name         string          `json:"name" db:"name"`
	Description  string          `json:"description,omitempty" db:"description"`
	Expression   string          `json:"expression" db:"expression"` // CEL-like expression
	Weight       int             `json:"weight" db:"weight"`         // 1-10
	PassMessage  string          `json:"pass_message,omitempty" db:"pass_message"`
	FailMessage  string          `json:"fail_message,omitempty" db:"fail_message"`
	Severity     string          `json:"severity" db:"severity"` // critical, warning, info
	Metadata     json.RawMessage `json:"metadata,omitempty" db:"metadata"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
}

// ScorecardResult — evaluation result for a single entity
type ScorecardResult struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	ScorecardID  uuid.UUID       `json:"scorecard_id" db:"scorecard_id"`
	EntityID     uuid.UUID       `json:"entity_id" db:"entity_id"`
	TenantID     uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Score        int             `json:"score" db:"score"`           // 0-100
	MaxScore     int             `json:"max_score" db:"max_score"`   // maximum possible
	PassCount    int             `json:"pass_count" db:"pass_count"`
	FailCount    int             `json:"fail_count" db:"fail_count"`
	TotalRules   int             `json:"total_rules" db:"total_rules"`
	Level        string          `json:"level" db:"level"` // bronze, silver, gold, platinum
	Details      json.RawMessage `json:"details" db:"details"`
	EvaluatedAt  time.Time       `json:"evaluated_at" db:"evaluated_at"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	EntityName   string          `json:"entity_name,omitempty" db:"entity_name"` // denormalized for list views
}

// ScorecardRuleResult — result of evaluating a single rule
type ScorecardRuleResult struct {
	RuleID    uuid.UUID `json:"rule_id"`
	RuleName  string    `json:"rule_name"`
	Passed    bool      `json:"passed"`
	Weight    int       `json:"weight"`
	Score     int       `json:"score"` // weight * (passed ? 1 : 0)
	Message   string    `json:"message"`
	Severity  string    `json:"severity"`
}

// ── Request types ────────────────────────────────────────────

type CreateScorecardRequest struct {
	Name        string          `json:"name" binding:"required"`
	Description string          `json:"description,omitempty"`
	Enabled     bool            `json:"enabled"`
	Config      json.RawMessage `json:"config,omitempty"`
	Rules       []CreateScorecardRuleRequest `json:"rules,omitempty"`
}

type UpdateScorecardRequest struct {
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Enabled     *bool           `json:"enabled,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
}

type CreateScorecardRuleRequest struct {
	Name        string          `json:"name" binding:"required"`
	Description string          `json:"description,omitempty"`
	Expression  string          `json:"expression" binding:"required"`
	Weight      int             `json:"weight" binding:"required,min=1,max=10"`
	PassMessage string          `json:"pass_message,omitempty"`
	FailMessage string          `json:"fail_message,omitempty"`
	Severity    string          `json:"severity" binding:"required,oneof=critical warning info"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

// ScorecardLevel returns the maturity level based on score percentage
func ScorecardLevel(score, maxScore int) string {
	if maxScore == 0 {
		return "unknown"
	}
	pct := float64(score) / float64(maxScore) * 100
	switch {
	case pct >= 90:
		return "platinum"
	case pct >= 75:
		return "gold"
	case pct >= 50:
		return "silver"
	case pct >= 25:
		return "bronze"
	default:
		return "none"
	}
}
