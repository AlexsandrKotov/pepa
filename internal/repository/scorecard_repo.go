package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/pkg/models"
)

// ScorecardRepository handles scorecard CRUD and evaluation persistence.
type ScorecardRepository struct {
	db *database.DB
}

func NewScorecardRepository(db *database.DB) *ScorecardRepository {
	return &ScorecardRepository{db: db}
}

// CreateScorecard inserts a new scorecard.
func (r *ScorecardRepository) CreateScorecard(ctx context.Context, sc *models.Scorecard) error {
	sc.ID = uuid.New()
	query := `INSERT INTO scorecards (id, tenant_id, name, description, enabled, config, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at`
	return r.db.Pool.QueryRow(ctx, query,
		sc.ID, sc.TenantID, sc.Name, sc.Description, sc.Enabled, sc.Config, sc.CreatedBy,
	).Scan(&sc.CreatedAt, &sc.UpdatedAt)
}

// ListScorecards returns all scorecards for a tenant.
func (r *ScorecardRepository) ListScorecards(ctx context.Context, tenantID uuid.UUID) ([]models.Scorecard, error) {
	query := `SELECT id, tenant_id, name, description, enabled, config, created_by, created_at, updated_at
		FROM scorecards WHERE tenant_id = $1 ORDER BY created_at DESC`
	rows, err := r.db.Pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.Scorecard
	for rows.Next() {
		var sc models.Scorecard
		if err := rows.Scan(&sc.ID, &sc.TenantID, &sc.Name, &sc.Description, &sc.Enabled,
			&sc.Config, &sc.CreatedBy, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, sc)
	}
	return items, rows.Err()
}

// GetScorecard returns a scorecard by ID.
func (r *ScorecardRepository) GetScorecard(ctx context.Context, id uuid.UUID) (*models.Scorecard, error) {
	var sc models.Scorecard
	query := `SELECT id, tenant_id, name, description, enabled, config, created_by, created_at, updated_at
		FROM scorecards WHERE id = $1`
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&sc.ID, &sc.TenantID, &sc.Name, &sc.Description, &sc.Enabled,
		&sc.Config, &sc.CreatedBy, &sc.CreatedAt, &sc.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

// UpdateScorecard updates a scorecard.
func (r *ScorecardRepository) UpdateScorecard(ctx context.Context, id uuid.UUID, req models.UpdateScorecardRequest) (*models.Scorecard, error) {
	query := `UPDATE scorecards SET updated_at = NOW()`
	args := []interface{}{}
	argIdx := 0

	if req.Name != nil {
		argIdx++
		query += fmt.Sprintf(", name = $%d", argIdx)
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		argIdx++
		query += fmt.Sprintf(", description = $%d", argIdx)
		args = append(args, *req.Description)
	}
	if req.Enabled != nil {
		argIdx++
		query += fmt.Sprintf(", enabled = $%d", argIdx)
		args = append(args, *req.Enabled)
	}
	if req.Config != nil {
		argIdx++
		query += fmt.Sprintf(", config = $%d", argIdx)
		args = append(args, req.Config)
	}

	argIdx++
	query += fmt.Sprintf(" WHERE id = $%d RETURNING id, tenant_id, name, description, enabled, config, created_by, created_at, updated_at", argIdx)
	args = append(args, id)

	var sc models.Scorecard
	err := r.db.Pool.QueryRow(ctx, query, args...).Scan(
		&sc.ID, &sc.TenantID, &sc.Name, &sc.Description, &sc.Enabled,
		&sc.Config, &sc.CreatedBy, &sc.CreatedAt, &sc.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

// DeleteScorecard deletes a scorecard (cascades to rules and results).
func (r *ScorecardRepository) DeleteScorecard(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM scorecards WHERE id = $1`, id)
	return err
}

// CreateRule inserts a scoring rule.
func (r *ScorecardRepository) CreateRule(ctx context.Context, rule *models.ScorecardRule) error {
	rule.ID = uuid.New()
	query := `INSERT INTO scorecard_rules (id, scorecard_id, name, description, expression, weight, pass_message, fail_message, severity, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at, updated_at`
	return r.db.Pool.QueryRow(ctx, query,
		rule.ID, rule.ScorecardID, rule.Name, rule.Description, rule.Expression,
		rule.Weight, rule.PassMessage, rule.FailMessage, rule.Severity, rule.Metadata,
	).Scan(&rule.CreatedAt, &rule.UpdatedAt)
}

// ListRules returns all rules for a scorecard.
func (r *ScorecardRepository) ListRules(ctx context.Context, scorecardID uuid.UUID) ([]models.ScorecardRule, error) {
	query := `SELECT id, scorecard_id, name, description, expression, weight, pass_message, fail_message, severity, metadata, created_at, updated_at
		FROM scorecard_rules WHERE scorecard_id = $1 ORDER BY created_at`
	rows, err := r.db.Pool.Query(ctx, query, scorecardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.ScorecardRule
	for rows.Next() {
		var rule models.ScorecardRule
		if err := rows.Scan(&rule.ID, &rule.ScorecardID, &rule.Name, &rule.Description,
			&rule.Expression, &rule.Weight, &rule.PassMessage, &rule.FailMessage,
			&rule.Severity, &rule.Metadata, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, rule)
	}
	return items, rows.Err()
}

// DeleteRule deletes a single rule.
func (r *ScorecardRepository) DeleteRule(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM scorecard_rules WHERE id = $1`, id)
	return err
}

// SaveResult persists a scorecard evaluation result.
func (r *ScorecardRepository) SaveResult(ctx context.Context, result *models.ScorecardResult) error {
	result.ID = uuid.New()
	query := `INSERT INTO scorecard_results (id, scorecard_id, entity_id, tenant_id, score, max_score, pass_count, fail_count, total_rules, level, details, evaluated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	_, err := r.db.Pool.Exec(ctx, query,
		result.ID, result.ScorecardID, result.EntityID, result.TenantID,
		result.Score, result.MaxScore, result.PassCount, result.FailCount,
		result.TotalRules, result.Level, result.Details, result.EvaluatedAt,
	)
	return err
}

// GetLatestResult returns the most recent evaluation for an entity+scorecard.
func (r *ScorecardRepository) GetLatestResult(ctx context.Context, scorecardID, entityID uuid.UUID) (*models.ScorecardResult, error) {
	var result models.ScorecardResult
	query := `SELECT id, scorecard_id, entity_id, tenant_id, score, max_score, pass_count, fail_count, total_rules, level, details, evaluated_at, created_at
		FROM scorecard_results
		WHERE scorecard_id = $1 AND entity_id = $2
		ORDER BY evaluated_at DESC LIMIT 1`
	err := r.db.Pool.QueryRow(ctx, query, scorecardID, entityID).Scan(
		&result.ID, &result.ScorecardID, &result.EntityID, &result.TenantID,
		&result.Score, &result.MaxScore, &result.PassCount, &result.FailCount,
		&result.TotalRules, &result.Level, &result.Details, &result.EvaluatedAt, &result.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListResults returns latest results for all entities under a scorecard.
func (r *ScorecardRepository) ListResults(ctx context.Context, scorecardID uuid.UUID) ([]models.ScorecardResult, error) {
	query := `SELECT DISTINCT ON (sr.entity_id) sr.id, sr.scorecard_id, sr.entity_id, sr.tenant_id, sr.score, sr.max_score, sr.pass_count, sr.fail_count, sr.total_rules, sr.level, sr.details, sr.evaluated_at, sr.created_at, COALESCE(e.name, '') AS entity_name
		FROM scorecard_results sr
		LEFT JOIN entities e ON e.id = sr.entity_id
		WHERE sr.scorecard_id = $1
		ORDER BY sr.entity_id, sr.evaluated_at DESC`
	rows, err := r.db.Pool.Query(ctx, query, scorecardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.ScorecardResult
	for rows.Next() {
		var result models.ScorecardResult
		if err := rows.Scan(&result.ID, &result.ScorecardID, &result.EntityID, &result.TenantID,
			&result.Score, &result.MaxScore, &result.PassCount, &result.FailCount,
			&result.TotalRules, &result.Level, &result.Details, &result.EvaluatedAt, &result.CreatedAt,
			&result.EntityName); err != nil {
			return nil, err
		}
		items = append(items, result)
	}
	return items, rows.Err()
}

// GetEntityScores returns latest results for an entity across all scorecards.
func (r *ScorecardRepository) GetEntityScores(ctx context.Context, entityID, tenantID uuid.UUID) ([]models.ScorecardResult, error) {
	query := `SELECT DISTINCT ON (sr.scorecard_id) sr.id, sr.scorecard_id, sr.entity_id, sr.tenant_id, sr.score, sr.max_score, sr.pass_count, sr.fail_count, sr.total_rules, sr.level, sr.details, sr.evaluated_at, sr.created_at, COALESCE(e.name, '') AS entity_name
		FROM scorecard_results sr
		LEFT JOIN entities e ON e.id = sr.entity_id
		WHERE sr.entity_id = $1 AND sr.tenant_id = $2
		ORDER BY sr.scorecard_id, sr.evaluated_at DESC`
	rows, err := r.db.Pool.Query(ctx, query, entityID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.ScorecardResult
	for rows.Next() {
		var result models.ScorecardResult
		if err := rows.Scan(&result.ID, &result.ScorecardID, &result.EntityID, &result.TenantID,
			&result.Score, &result.MaxScore, &result.PassCount, &result.FailCount,
			&result.TotalRules, &result.Level, &result.Details, &result.EvaluatedAt, &result.CreatedAt,
			&result.EntityName); err != nil {
			return nil, err
		}
		items = append(items, result)
	}
	return items, rows.Err()
}

// ListTenantEntityIDs returns IDs of all non-deleted entities in a tenant.
// Checks both entities and services tables for compatibility.
func (r *ScorecardRepository) ListTenantEntityIDs(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error) {
	// First try entities table
	rows, err := r.db.Pool.Query(ctx, `SELECT id FROM entities WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID)
	if err != nil {
		log.Printf("[scorecard] entities query failed, falling back to services: %v", err)
	} else {
		defer rows.Close()
		var ids []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		if len(ids) > 0 {
			return ids, nil
		}
	}

	// Fall back to services table
	rows2, err := r.db.Pool.Query(ctx, `SELECT id FROM services WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	var ids []uuid.UUID
	for rows2.Next() {
		var id uuid.UUID
		if err := rows2.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows2.Err()
}

// EvaluateEntity evaluates all rules of a scorecard against an entity.
// Supports both entities and services tables.
func (r *ScorecardRepository) EvaluateEntity(ctx context.Context, scorecardID, entityID, tenantID uuid.UUID) (*models.ScorecardResult, error) {
	// Load entity into separate variables to avoid struct scanning issues
	var id uuid.UUID
	var typeKey, name, description, status string
	var metadataRaw []byte
	var entityTenantID uuid.UUID

	// Try entities table first
	err := r.db.Pool.QueryRow(ctx, `SELECT id, type_key, name, description, metadata, status, tenant_id FROM entities WHERE id = $1`, entityID).Scan(
		&id, &typeKey, &name, &description, &metadataRaw, &status, &entityTenantID,
	)
	if err != nil {
		// Fall back to services table
		err2 := r.db.Pool.QueryRow(ctx, `SELECT id, 'service', name, COALESCE(description, ''), COALESCE(resource_config, '{}'::jsonb), status, tenant_id FROM services WHERE id = $1`, entityID).Scan(
			&id, &typeKey, &name, &description, &metadataRaw, &status, &entityTenantID,
		)
		if err2 != nil {
			return nil, fmt.Errorf("load entity: %w (also tried services: %w)", err, err2)
		}
	}

	// Tenant isolation: the loaded entity/service must belong to the caller's
	// tenant. Reported as not-found to avoid leaking cross-tenant existence.
	if tenantID != uuid.Nil && entityTenantID != tenantID {
		return nil, fmt.Errorf("entity not found: %s", entityID)
	}

	// Load rules
	rules, err := r.ListRules(ctx, scorecardID)
	if err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}

	// Parse entity metadata for evaluation
	var metadata map[string]interface{}
	if metadataRaw != nil {
		_ = json.Unmarshal(metadataRaw, &metadata)
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}

	// Build evaluation context
	evalCtx := map[string]interface{}{
		"type_key":    typeKey,
		"name":        name,
		"description": description,
		"status":      status,
		"metadata":    metadata,
	}

	// Evaluate each rule
	var ruleResults []models.ScorecardRuleResult
	totalScore := 0
	maxScore := 0
	passCount := 0
	failCount := 0

	for _, rule := range rules {
		passed := evaluateExpression(rule.Expression, evalCtx)
		score := 0
		if passed {
			score = rule.Weight
			passCount++
		} else {
			failCount++
		}
		totalScore += score
		maxScore += rule.Weight

		msg := rule.PassMessage
		if !passed {
			msg = rule.FailMessage
		}

		ruleResults = append(ruleResults, models.ScorecardRuleResult{
			RuleID:   rule.ID,
			RuleName: rule.Name,
			Passed:   passed,
			Weight:   rule.Weight,
			Score:    score,
			Message:  msg,
			Severity: rule.Severity,
		})
	}

	level := models.ScorecardLevel(totalScore, maxScore)

	detailsJSON, _ := json.Marshal(ruleResults)

	result := &models.ScorecardResult{
		ScorecardID: scorecardID,
		EntityID:    entityID,
		TenantID:    tenantID,
		Score:       totalScore,
		MaxScore:    maxScore,
		PassCount:   passCount,
		FailCount:   failCount,
		TotalRules:  len(rules),
		Level:       level,
		Details:     detailsJSON,
		EvaluatedAt: time.Now().UTC(),
	}

	// Save result
	if err := r.SaveResult(ctx, result); err != nil {
		return nil, fmt.Errorf("save result: %w", err)
	}

	return result, nil
}

// evaluateExpression evaluates a rule expression against an entity context.
// Supported syntax:
//   - "a && b" / "a || b" — logical composition of sub-expressions
//   - "has_metadata.field" — metadata field existence
//   - "not_empty.field" — field is present and non-empty
//   - "field == value" / "field != value" — comparison (field may be a top-level
//     attribute like status/name/description/type_key, or a dotted metadata path)
//   - "field == null" / "field != null" — presence checks
//   - "metadata.field", "field" or "a.b.c" — bare existence check (dotted = nested)
//   - "always_true" / "always_false"
func evaluateExpression(expr string, ctx map[string]interface{}) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}
	if expr == "always_true" {
		return true
	}
	if expr == "always_false" {
		return false
	}

	// Logical AND — all parts must pass
	if parts := splitLogical(expr, "&&"); len(parts) > 1 {
		for _, p := range parts {
			if !evaluateExpression(p, ctx) {
				return false
			}
		}
		return true
	}
	// Logical OR — any part must pass
	if parts := splitLogical(expr, "||"); len(parts) > 1 {
		for _, p := range parts {
			if evaluateExpression(p, ctx) {
				return true
			}
		}
		return false
	}

	metadata, _ := ctx["metadata"].(map[string]interface{})

	// Pattern: has_metadata.X (nested paths supported)
	if rest, ok := strings.CutPrefix(expr, "has_metadata."); ok {
		_, exists := resolveMetadataPath(metadata, rest)
		return exists
	}

	// Pattern: not_empty.X
	if rest, ok := strings.CutPrefix(expr, "not_empty."); ok {
		val, exists := resolveValue(rest, ctx)
		if !exists {
			return false
		}
		return !isEmptyValue(val)
	}

	// Pattern: left == right / left != right
	for _, op := range []string{" == ", " != "} {
		idx := strings.Index(expr, op)
		if idx <= 0 {
			continue
		}
		left := strings.TrimSpace(expr[:idx])
		right := strings.TrimSpace(expr[idx+len(op):])
		negate := op == " != "

		// null checks
		if right == "null" {
			val, exists := resolveValue(left, ctx)
			isNull := !exists || val == nil
			if negate {
				return !isNull && !isEmptyValue(val)
			}
			return isNull
		}

		actual, exists := resolveValue(left, ctx)
		if !exists {
			return negate
		}
		return compareValues(actual, negate, right)
	}

	// Bare expression → existence check (top-level attr or metadata path)
	_, exists := resolveValue(expr, ctx)
	return exists
}

// splitLogical splits an expression on a top-level logical operator.
func splitLogical(expr, op string) []string {
	parts := strings.Split(expr, op)
	if len(parts) < 2 {
		return nil
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// resolveValue resolves a field reference against the evaluation context.
// Top-level attributes (type_key, name, description, status) are read from ctx,
// everything else (with or without the "metadata." prefix) is resolved as a
// dotted path inside entity metadata.
func resolveValue(ref string, ctx map[string]interface{}) (interface{}, bool) {
	ref = strings.TrimSpace(ref)
	switch ref {
	case "type_key", "name", "description", "status":
		val, ok := ctx[ref]
		return val, ok
	}
	metadata, _ := ctx["metadata"].(map[string]interface{})
	path, _ := strings.CutPrefix(ref, "metadata.")
	return resolveMetadataPath(metadata, path)
}

// resolveMetadataPath walks a dotted path through nested metadata maps.
func resolveMetadataPath(metadata map[string]interface{}, path string) (interface{}, bool) {
	var current interface{} = metadata
	for _, segment := range strings.Split(path, ".") {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		val, ok := m[segment]
		if !ok {
			return nil, false
		}
		current = val
	}
	return current, true
}

// compareValues compares an actual value against the expected literal.
func compareValues(actual interface{}, negate bool, expected string) bool {
	expected = strings.Trim(expected, `"'`)

	if isEmptyValue(actual) == (expected == "") {
		emptyMatch := expected == ""
		if negate {
			return !emptyMatch
		}
		return emptyMatch
	}

	// Numeric comparison when both sides are numeric
	if af, errA := toFloat(actual); errA == nil {
		if ef, errE := strconv.ParseFloat(expected, 64); errE == nil {
			if negate {
				return af != ef
			}
			return af == ef
		}
	}

	actualStr := fmt.Sprintf("%v", actual)
	if negate {
		return actualStr != expected
	}
	return actualStr == expected
}

func isEmptyValue(val interface{}) bool {
	if val == nil {
		return true
	}
	s := fmt.Sprintf("%v", val)
	return s == "" || s == "<nil>" || s == "null" || s == "map[]" || s == "[]"
}

func toFloat(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case json.Number:
		return v.Float64()
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("not numeric")
	}
}
