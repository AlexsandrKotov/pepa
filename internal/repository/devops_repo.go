package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/pkg/models"
)

// DevOpsRepository handles DevOps/DevSecOps data persistence.
type DevOpsRepository struct {
	pool *pgxpool.Pool
}

// NewDevOpsRepository creates a new DevOps repository.
func NewDevOpsRepository(db *database.DB) *DevOpsRepository {
	return &DevOpsRepository{pool: db.Pool}
}

// ============================================================
// Deployment Windows
// ============================================================

// ListDeploymentWindows returns all deployment windows for a tenant.
func (r *DevOpsRepository) ListDeploymentWindows(ctx context.Context, tenantID uuid.UUID) ([]models.DeploymentWindow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), window_type,
		       COALESCE(cron_expression,''), start_at, end_at, timezone,
		       environments, service_ids, enabled, priority, COALESCE(reason,''),
		       override_roles, created_by, created_at, updated_at
		FROM deployment_windows
		WHERE tenant_id = $1
		ORDER BY priority DESC, created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list deployment windows: %w", err)
	}
	defer rows.Close()

	var windows []models.DeploymentWindow
	for rows.Next() {
		var w models.DeploymentWindow
		if err := rows.Scan(
			&w.ID, &w.TenantID, &w.Name, &w.Description, &w.WindowType,
			&w.CronExpression, &w.StartAt, &w.EndAt, &w.Timezone,
			&w.Environments, &w.ServiceIDs, &w.Enabled, &w.Priority, &w.Reason,
			&w.OverrideRoles, &w.CreatedBy, &w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan deployment window: %w", err)
		}
		windows = append(windows, w)
	}
	return windows, nil
}

// GetDeploymentWindow returns a deployment window by ID.
func (r *DevOpsRepository) GetDeploymentWindow(ctx context.Context, id, tenantID uuid.UUID) (*models.DeploymentWindow, error) {
	var w models.DeploymentWindow
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), window_type,
		       COALESCE(cron_expression,''), start_at, end_at, timezone,
		       environments, service_ids, enabled, priority, COALESCE(reason,''),
		       override_roles, created_by, created_at, updated_at
		FROM deployment_windows
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(
		&w.ID, &w.TenantID, &w.Name, &w.Description, &w.WindowType,
		&w.CronExpression, &w.StartAt, &w.EndAt, &w.Timezone,
		&w.Environments, &w.ServiceIDs, &w.Enabled, &w.Priority, &w.Reason,
		&w.OverrideRoles, &w.CreatedBy, &w.CreatedAt, &w.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("deployment window not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get deployment window: %w", err)
	}
	return &w, nil
}

// CreateDeploymentWindow creates a new deployment window.
func (r *DevOpsRepository) CreateDeploymentWindow(ctx context.Context, w *models.DeploymentWindow) error {
	w.ID = uuid.New()
	now := time.Now().UTC()
	w.CreatedAt = now
	w.UpdatedAt = now

	_, err := r.pool.Exec(ctx, `
		INSERT INTO deployment_windows (
			id, tenant_id, name, description, window_type, cron_expression,
			start_at, end_at, timezone, environments, service_ids, enabled,
			priority, reason, override_roles, created_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
	`, w.ID, w.TenantID, w.Name, w.Description, w.WindowType, w.CronExpression,
		w.StartAt, w.EndAt, w.Timezone, w.Environments, w.ServiceIDs, w.Enabled,
		w.Priority, w.Reason, w.OverrideRoles, w.CreatedBy, w.CreatedAt, w.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create deployment window: %w", err)
	}
	return nil
}

// UpdateDeploymentWindow updates a deployment window.
func (r *DevOpsRepository) UpdateDeploymentWindow(ctx context.Context, w *models.DeploymentWindow) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE deployment_windows SET
			name = $3, description = $4, window_type = $5, cron_expression = $6,
			start_at = $7, end_at = $8, timezone = $9, environments = $10,
			service_ids = $11, enabled = $12, priority = $13, reason = $14,
			override_roles = $15, updated_at = $16
		WHERE id = $1 AND tenant_id = $2
	`, w.ID, w.TenantID, w.Name, w.Description, w.WindowType, w.CronExpression,
		w.StartAt, w.EndAt, w.Timezone, w.Environments, w.ServiceIDs, w.Enabled,
		w.Priority, w.Reason, w.OverrideRoles, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("update deployment window: %w", err)
	}
	return nil
}

// DeleteDeploymentWindow deletes a deployment window.
func (r *DevOpsRepository) DeleteDeploymentWindow(ctx context.Context, id, tenantID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM deployment_windows WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return err
}

// GetActiveWindows returns windows that are currently active (for checking if deployment is allowed).
func (r *DevOpsRepository) GetActiveWindows(ctx context.Context, tenantID uuid.UUID, environment string, serviceID *uuid.UUID) ([]models.DeploymentWindow, error) {
	query := `
		SELECT id, tenant_id, name, COALESCE(description,''), window_type,
		       COALESCE(cron_expression,''), start_at, end_at, timezone,
		       environments, service_ids, enabled, priority, COALESCE(reason,''),
		       override_roles, created_by, created_at, updated_at
		FROM deployment_windows
		WHERE tenant_id = $1 AND enabled = true
	`
	args := []interface{}{tenantID}
	argIdx := 2

	// Filter by environment if specified
	query += fmt.Sprintf(` AND (array_length(environments, 1) IS NULL OR environments @> $%d)`, argIdx)
	args = append(args, []string{environment})
	argIdx++

	// Filter by service if specified
	if serviceID != nil {
		query += fmt.Sprintf(` AND (array_length(service_ids, 1) IS NULL OR service_ids @> $%d)`, argIdx)
		args = append(args, []uuid.UUID{*serviceID})
		argIdx++
	}

	// Check for active date-range windows
	query += ` AND (
		(start_at IS NOT NULL AND end_at IS NOT NULL AND start_at <= now() AND end_at >= now())
		OR (start_at IS NULL AND end_at IS NULL) -- recurring windows always "active" for cron check
	)`

	query += ` ORDER BY priority DESC`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get active windows: %w", err)
	}
	defer rows.Close()

	var windows []models.DeploymentWindow
	for rows.Next() {
		var w models.DeploymentWindow
		if err := rows.Scan(
			&w.ID, &w.TenantID, &w.Name, &w.Description, &w.WindowType,
			&w.CronExpression, &w.StartAt, &w.EndAt, &w.Timezone,
			&w.Environments, &w.ServiceIDs, &w.Enabled, &w.Priority, &w.Reason,
			&w.OverrideRoles, &w.CreatedBy, &w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan deployment window: %w", err)
		}
		windows = append(windows, w)
	}
	return windows, nil
}

// ============================================================
// Batch Operations
// ============================================================

// ListBatchOperations returns all batch operations for a tenant.
func (r *DevOpsRepository) ListBatchOperations(ctx context.Context, tenantID uuid.UUID) ([]models.BatchOperation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, operation_type, status, service_ids,
		       COALESCE(parameters,'{}'::jsonb), COALESCE(results,'{}'::jsonb),
		       total_count, completed_count, failed_count, initiated_by,
		       COALESCE(reason,''), COALESCE(incident_id,''), timeout_seconds,
		       started_at, completed_at, COALESCE(error_message,''),
		       created_at, updated_at
		FROM batch_operations
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list batch operations: %w", err)
	}
	defer rows.Close()

	var ops []models.BatchOperation
	for rows.Next() {
		var op models.BatchOperation
		if err := rows.Scan(
			&op.ID, &op.TenantID, &op.Name, &op.OperationType, &op.Status,
			&op.ServiceIDs, &op.Parameters, &op.Results,
			&op.TotalCount, &op.CompletedCount, &op.FailedCount,
			&op.InitiatedBy, &op.Reason, &op.IncidentID, &op.TimeoutSeconds,
			&op.StartedAt, &op.CompletedAt, &op.ErrorMessage,
			&op.CreatedAt, &op.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan batch operation: %w", err)
		}
		ops = append(ops, op)
	}
	return ops, nil
}

// GetBatchOperation returns a batch operation by ID.
func (r *DevOpsRepository) GetBatchOperation(ctx context.Context, id, tenantID uuid.UUID) (*models.BatchOperation, error) {
	var op models.BatchOperation
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, operation_type, status, service_ids,
		       COALESCE(parameters,'{}'::jsonb), COALESCE(results,'{}'::jsonb),
		       total_count, completed_count, failed_count, initiated_by,
		       COALESCE(reason,''), COALESCE(incident_id,''), timeout_seconds,
		       started_at, completed_at, COALESCE(error_message,''),
		       created_at, updated_at
		FROM batch_operations
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(
		&op.ID, &op.TenantID, &op.Name, &op.OperationType, &op.Status,
		&op.ServiceIDs, &op.Parameters, &op.Results,
		&op.TotalCount, &op.CompletedCount, &op.FailedCount,
		&op.InitiatedBy, &op.Reason, &op.IncidentID, &op.TimeoutSeconds,
		&op.StartedAt, &op.CompletedAt, &op.ErrorMessage,
		&op.CreatedAt, &op.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("batch operation not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get batch operation: %w", err)
	}
	return &op, nil
}

// CreateBatchOperation creates a new batch operation.
func (r *DevOpsRepository) CreateBatchOperation(ctx context.Context, op *models.BatchOperation) error {
	op.ID = uuid.New()
	now := time.Now().UTC()
	op.CreatedAt = now
	op.UpdatedAt = now
	op.TotalCount = len(op.ServiceIDs)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO batch_operations (
			id, tenant_id, name, operation_type, status, service_ids,
			parameters, results, total_count, completed_count, failed_count,
			initiated_by, reason, incident_id, timeout_seconds,
			started_at, completed_at, error_message, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
	`, op.ID, op.TenantID, op.Name, op.OperationType, op.Status, op.ServiceIDs,
		op.Parameters, op.Results, op.TotalCount, op.CompletedCount, op.FailedCount,
		op.InitiatedBy, op.Reason, op.IncidentID, op.TimeoutSeconds,
		op.StartedAt, op.CompletedAt, op.ErrorMessage, op.CreatedAt, op.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create batch operation: %w", err)
	}
	return nil
}

// UpdateBatchOperation updates a batch operation.
func (r *DevOpsRepository) UpdateBatchOperation(ctx context.Context, op *models.BatchOperation) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE batch_operations SET
			status = $3, results = $4, completed_count = $5, failed_count = $6,
			started_at = $7, completed_at = $8, error_message = $9, updated_at = $10
		WHERE id = $1 AND tenant_id = $2
	`, op.ID, op.TenantID, op.Status, op.Results, op.CompletedCount, op.FailedCount,
		op.StartedAt, op.CompletedAt, op.ErrorMessage, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("update batch operation: %w", err)
	}
	return nil
}

// CancelBatchOperation cancels a pending/running batch operation.
func (r *DevOpsRepository) CancelBatchOperation(ctx context.Context, id, tenantID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE batch_operations SET status = 'cancelled', completed_at = now(), updated_at = now()
		WHERE id = $1 AND tenant_id = $2 AND status IN ('pending', 'running')
	`, id, tenantID)
	return err
}

// ============================================================
// Compliance Policies
// ============================================================

// ListCompliancePolicies returns all compliance policies for a tenant.
func (r *DevOpsRepository) ListCompliancePolicies(ctx context.Context, tenantID uuid.UUID) ([]models.CompliancePolicy, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), policy_type,
		       policy_spec, severity, blocking, environments, service_ids,
		       enabled, last_evaluated_at, last_violation_count,
		       created_by, created_at, updated_at
		FROM compliance_policies
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list compliance policies: %w", err)
	}
	defer rows.Close()

	var policies []models.CompliancePolicy
	for rows.Next() {
		var p models.CompliancePolicy
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.Name, &p.Description, &p.PolicyType,
			&p.PolicySpec, &p.Severity, &p.Blocking, &p.Environments, &p.ServiceIDs,
			&p.Enabled, &p.LastEvaluatedAt, &p.LastViolationCount,
			&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan compliance policy: %w", err)
		}
		policies = append(policies, p)
	}
	return policies, nil
}

// GetCompliancePolicy returns a compliance policy by ID.
func (r *DevOpsRepository) GetCompliancePolicy(ctx context.Context, id, tenantID uuid.UUID) (*models.CompliancePolicy, error) {
	var p models.CompliancePolicy
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), policy_type,
		       policy_spec, severity, blocking, environments, service_ids,
		       enabled, last_evaluated_at, last_violation_count,
		       created_by, created_at, updated_at
		FROM compliance_policies
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(
		&p.ID, &p.TenantID, &p.Name, &p.Description, &p.PolicyType,
		&p.PolicySpec, &p.Severity, &p.Blocking, &p.Environments, &p.ServiceIDs,
		&p.Enabled, &p.LastEvaluatedAt, &p.LastViolationCount,
		&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("compliance policy not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get compliance policy: %w", err)
	}
	return &p, nil
}

// CreateCompliancePolicy creates a new compliance policy.
func (r *DevOpsRepository) CreateCompliancePolicy(ctx context.Context, p *models.CompliancePolicy) error {
	p.ID = uuid.New()
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := r.pool.Exec(ctx, `
		INSERT INTO compliance_policies (
			id, tenant_id, name, description, policy_type, policy_spec,
			severity, blocking, environments, service_ids, enabled,
			created_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, p.ID, p.TenantID, p.Name, p.Description, p.PolicyType, p.PolicySpec,
		p.Severity, p.Blocking, p.Environments, p.ServiceIDs, p.Enabled,
		p.CreatedBy, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create compliance policy: %w", err)
	}
	return nil
}

// UpdateCompliancePolicy updates a compliance policy.
func (r *DevOpsRepository) UpdateCompliancePolicy(ctx context.Context, p *models.CompliancePolicy) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE compliance_policies SET
			name = $3, description = $4, policy_type = $5, policy_spec = $6,
			severity = $7, blocking = $8, environments = $9, service_ids = $10,
			enabled = $11, updated_at = $12
		WHERE id = $1 AND tenant_id = $2
	`, p.ID, p.TenantID, p.Name, p.Description, p.PolicyType, p.PolicySpec,
		p.Severity, p.Blocking, p.Environments, p.ServiceIDs, p.Enabled, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("update compliance policy: %w", err)
	}
	return nil
}

// DeleteCompliancePolicy deletes a compliance policy.
func (r *DevOpsRepository) DeleteCompliancePolicy(ctx context.Context, id, tenantID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM compliance_policies WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return err
}

// GetApplicablePolicies returns policies that apply to a given service/environment.
func (r *DevOpsRepository) GetApplicablePolicies(ctx context.Context, tenantID uuid.UUID, environment string, serviceID *uuid.UUID) ([]models.CompliancePolicy, error) {
	query := `
		SELECT id, tenant_id, name, COALESCE(description,''), policy_type,
		       policy_spec, severity, blocking, environments, service_ids,
		       enabled, last_evaluated_at, last_violation_count,
		       created_by, created_at, updated_at
		FROM compliance_policies
		WHERE tenant_id = $1 AND enabled = true
	`
	args := []interface{}{tenantID}
	argIdx := 2

	// Filter by environment
	query += fmt.Sprintf(` AND (array_length(environments, 1) IS NULL OR environments @> $%d)`, argIdx)
	args = append(args, []string{environment})
	argIdx++

	// Filter by service
	if serviceID != nil {
		query += fmt.Sprintf(` AND (array_length(service_ids, 1) IS NULL OR service_ids @> $%d)`, argIdx)
		args = append(args, []uuid.UUID{*serviceID})
	}

	query += ` ORDER BY CASE severity WHEN 'block' THEN 1 WHEN 'warn' THEN 2 ELSE 3 END`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get applicable policies: %w", err)
	}
	defer rows.Close()

	var policies []models.CompliancePolicy
	for rows.Next() {
		var p models.CompliancePolicy
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.Name, &p.Description, &p.PolicyType,
			&p.PolicySpec, &p.Severity, &p.Blocking, &p.Environments, &p.ServiceIDs,
			&p.Enabled, &p.LastEvaluatedAt, &p.LastViolationCount,
			&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan compliance policy: %w", err)
		}
		policies = append(policies, p)
	}
	return policies, nil
}

// CreateComplianceEvaluation creates a compliance evaluation record.
func (r *DevOpsRepository) CreateComplianceEvaluation(ctx context.Context, e *models.ComplianceEvaluation) error {
	e.ID = uuid.New()
	e.EvaluatedAt = time.Now().UTC()

	_, err := r.pool.Exec(ctx, `
		INSERT INTO compliance_evaluations (
			id, tenant_id, policy_id, service_id, deployment_id,
			result, violations, context, evaluated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, e.ID, e.TenantID, e.PolicyID, e.ServiceID, e.DeploymentID,
		e.Result, e.Violations, e.Context, e.EvaluatedAt)
	if err != nil {
		return fmt.Errorf("create compliance evaluation: %w", err)
	}
	return nil
}

// GetComplianceEvaluations returns recent evaluations for a service.
func (r *DevOpsRepository) GetComplianceEvaluations(ctx context.Context, tenantID uuid.UUID, serviceID *uuid.UUID, limit int) ([]models.ComplianceEvaluation, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT ce.id, ce.tenant_id, ce.policy_id, ce.service_id, ce.deployment_id,
		       ce.result, COALESCE(ce.violations,'[]'::jsonb), COALESCE(ce.context,'{}'::jsonb),
		       ce.evaluated_at, COALESCE(cp.name,'')
		FROM compliance_evaluations ce
		LEFT JOIN compliance_policies cp ON cp.id = ce.policy_id
		WHERE ce.tenant_id = $1
	`
	args := []interface{}{tenantID}

	if serviceID != nil {
		query += ` AND ce.service_id = $2`
		args = append(args, *serviceID)
	}

	query += ` ORDER BY ce.evaluated_at DESC LIMIT $`
	if serviceID != nil {
		query += `3`
	} else {
		query += `2`
	}
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get compliance evaluations: %w", err)
	}
	defer rows.Close()

	var evals []models.ComplianceEvaluation
	for rows.Next() {
		var e models.ComplianceEvaluation
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.PolicyID, &e.ServiceID, &e.DeploymentID,
			&e.Result, &e.Violations, &e.Context, &e.EvaluatedAt, &e.PolicyName,
		); err != nil {
			return nil, fmt.Errorf("scan compliance evaluation: %w", err)
		}
		evals = append(evals, e)
	}
	return evals, nil
}

// ============================================================
// Security Findings
// ============================================================

// ListSecurityFindings returns security findings with filtering.
func (r *DevOpsRepository) ListSecurityFindings(ctx context.Context, tenantID uuid.UUID, filter models.SecurityFindingFilter) ([]models.SecurityFinding, int64, error) {
	query := `
		SELECT id, tenant_id, scan_run_id, finding_type, severity, title,
		       COALESCE(description,''), COALESCE(identifier,''),
		       COALESCE(resource_type,''), COALESCE(resource_name,''),
		       fix_available, COALESCE(fix_version,''), COALESCE(fix_instructions,''),
		       status, resolved_by, resolved_at, COALESCE(resolution_notes,''),
		       first_seen_at, last_seen_at, created_at, updated_at
		FROM security_findings
		WHERE tenant_id = $1
	`
	countQuery := `SELECT COUNT(*) FROM security_findings WHERE tenant_id = $1`
	args := []interface{}{tenantID}
	argIdx := 2

	if filter.Severity != "" {
		query += fmt.Sprintf(` AND severity = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND severity = $%d`, argIdx)
		args = append(args, filter.Severity)
		argIdx++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(` AND status = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND status = $%d`, argIdx)
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.FindingType != "" {
		query += fmt.Sprintf(` AND finding_type = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND finding_type = $%d`, argIdx)
		args = append(args, filter.FindingType)
		argIdx++
	}
	if filter.Search != "" {
		query += fmt.Sprintf(` AND (title ILIKE $%d OR description ILIKE $%d OR identifier ILIKE $%d)`, argIdx, argIdx, argIdx)
		countQuery += fmt.Sprintf(` AND (title ILIKE $%d OR description ILIKE $%d OR identifier ILIKE $%d)`, argIdx, argIdx, argIdx)
		search := "%" + filter.Search + "%"
		args = append(args, search)
		argIdx++
	}

	// Get total count
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count security findings: %w", err)
	}

	// Add pagination
	query += ` ORDER BY CASE severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 WHEN 'low' THEN 4 ELSE 5 END, created_at DESC`
	offset := (filter.Page - 1) * filter.PerPage
	query += fmt.Sprintf(` LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, filter.PerPage, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list security findings: %w", err)
	}
	defer rows.Close()

	var findings []models.SecurityFinding
	for rows.Next() {
		var f models.SecurityFinding
		if err := rows.Scan(
			&f.ID, &f.TenantID, &f.ScanRunID, &f.FindingType, &f.Severity, &f.Title,
			&f.Description, &f.Identifier, &f.ResourceType, &f.ResourceName,
			&f.FixAvailable, &f.FixVersion, &f.FixInstructions,
			&f.Status, &f.ResolvedBy, &f.ResolvedAt, &f.ResolutionNotes,
			&f.FirstSeenAt, &f.LastSeenAt, &f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan security finding: %w", err)
		}
		findings = append(findings, f)
	}
	return findings, total, nil
}

// GetSecurityFinding returns a security finding by ID.
func (r *DevOpsRepository) GetSecurityFinding(ctx context.Context, id, tenantID uuid.UUID) (*models.SecurityFinding, error) {
	var f models.SecurityFinding
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, scan_run_id, finding_type, severity, title,
		       COALESCE(description,''), COALESCE(identifier,''),
		       COALESCE(resource_type,''), COALESCE(resource_name,''),
		       fix_available, COALESCE(fix_version,''), COALESCE(fix_instructions,''),
		       status, resolved_by, resolved_at, COALESCE(resolution_notes,''),
		       first_seen_at, last_seen_at, created_at, updated_at
		FROM security_findings
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(
		&f.ID, &f.TenantID, &f.ScanRunID, &f.FindingType, &f.Severity, &f.Title,
		&f.Description, &f.Identifier, &f.ResourceType, &f.ResourceName,
		&f.FixAvailable, &f.FixVersion, &f.FixInstructions,
		&f.Status, &f.ResolvedBy, &f.ResolvedAt, &f.ResolutionNotes,
		&f.FirstSeenAt, &f.LastSeenAt, &f.CreatedAt, &f.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("security finding not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get security finding: %w", err)
	}
	return &f, nil
}

// CreateSecurityFinding creates a new security finding.
func (r *DevOpsRepository) CreateSecurityFinding(ctx context.Context, f *models.SecurityFinding) error {
	f.ID = uuid.New()
	now := time.Now().UTC()
	f.CreatedAt = now
	f.UpdatedAt = now
	f.FirstSeenAt = now
	f.LastSeenAt = now

	_, err := r.pool.Exec(ctx, `
		INSERT INTO security_findings (
			id, tenant_id, scan_run_id, finding_type, severity, title,
			description, identifier, resource_type, resource_name,
			fix_available, fix_version, fix_instructions,
			status, resolved_by, resolved_at, resolution_notes,
			first_seen_at, last_seen_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
	`, f.ID, f.TenantID, f.ScanRunID, f.FindingType, f.Severity, f.Title,
		f.Description, f.Identifier, f.ResourceType, f.ResourceName,
		f.FixAvailable, f.FixVersion, f.FixInstructions,
		f.Status, f.ResolvedBy, f.ResolvedAt, f.ResolutionNotes,
		f.FirstSeenAt, f.LastSeenAt, f.CreatedAt, f.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create security finding: %w", err)
	}
	return nil
}

// UpdateSecurityFinding updates a security finding.
func (r *DevOpsRepository) UpdateSecurityFinding(ctx context.Context, f *models.SecurityFinding) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE security_findings SET
			status = $3, resolved_by = $4, resolved_at = $5,
			resolution_notes = $6, last_seen_at = $7, updated_at = $8
		WHERE id = $1 AND tenant_id = $2
	`, f.ID, f.TenantID, f.Status, f.ResolvedBy, f.ResolvedAt,
		f.ResolutionNotes, f.LastSeenAt, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("update security finding: %w", err)
	}
	return nil
}

// GetSecurityFindingSummary returns aggregated finding counts.
func (r *DevOpsRepository) GetSecurityFindingSummary(ctx context.Context, tenantID uuid.UUID) (*models.SecurityFindingSummary, error) {
	summary := &models.SecurityFindingSummary{
		BySeverity: make(map[string]int),
		ByStatus:   make(map[string]int),
		ByType:     make(map[string]int),
	}

	// Total and by severity
	rows, err := r.pool.Query(ctx, `
		SELECT severity, COUNT(*) FROM security_findings
		WHERE tenant_id = $1 GROUP BY severity
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("count by severity: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sev string
		var cnt int
		if err := rows.Scan(&sev, &cnt); err != nil {
			return nil, err
		}
		summary.BySeverity[sev] = cnt
		summary.Total += cnt
		if sev == "critical" {
			summary.CriticalCount = cnt
		}
	}
	rows.Close()

	// By status
	rows, err = r.pool.Query(ctx, `
		SELECT status, COUNT(*) FROM security_findings
		WHERE tenant_id = $1 GROUP BY status
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("count by status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var cnt int
		if err := rows.Scan(&status, &cnt); err != nil {
			return nil, err
		}
		summary.ByStatus[status] = cnt
		if status == "open" {
			summary.OpenCount = cnt
		}
	}
	rows.Close()

	// By type
	rows, err = r.pool.Query(ctx, `
		SELECT finding_type, COUNT(*) FROM security_findings
		WHERE tenant_id = $1 GROUP BY finding_type
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("count by type: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ft string
		var cnt int
		if err := rows.Scan(&ft, &cnt); err != nil {
			return nil, err
		}
		summary.ByType[ft] = cnt
	}

	return summary, nil
}

// GetBlockingSecurityFindings returns critical/high open findings that would block deployment.
func (r *DevOpsRepository) GetBlockingSecurityFindings(ctx context.Context, tenantID uuid.UUID, resourceName string) ([]models.SecurityFinding, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, scan_run_id, finding_type, severity, title,
		       COALESCE(description,''), COALESCE(identifier,''),
		       COALESCE(resource_type,''), COALESCE(resource_name,''),
		       fix_available, COALESCE(fix_version,''), COALESCE(fix_instructions,''),
		       status, resolved_by, resolved_at, COALESCE(resolution_notes,''),
		       first_seen_at, last_seen_at, created_at, updated_at
		FROM security_findings
		WHERE tenant_id = $1 AND status = 'open'
		  AND severity IN ('critical', 'high')
		  AND (resource_name = $2 OR resource_name = '')
	`, tenantID, resourceName)
	if err != nil {
		return nil, fmt.Errorf("get blocking findings: %w", err)
	}
	defer rows.Close()

	var findings []models.SecurityFinding
	for rows.Next() {
		var f models.SecurityFinding
		if err := rows.Scan(
			&f.ID, &f.TenantID, &f.ScanRunID, &f.FindingType, &f.Severity, &f.Title,
			&f.Description, &f.Identifier, &f.ResourceType, &f.ResourceName,
			&f.FixAvailable, &f.FixVersion, &f.FixInstructions,
			&f.Status, &f.ResolvedBy, &f.ResolvedAt, &f.ResolutionNotes,
			&f.FirstSeenAt, &f.LastSeenAt, &f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan security finding: %w", err)
		}
		findings = append(findings, f)
	}
	return findings, nil
}

// ============================================================
// Secret Rotations
// ============================================================

// ListSecretRotations returns all secret rotations for a tenant.
func (r *DevOpsRepository) ListSecretRotations(ctx context.Context, tenantID uuid.UUID) ([]models.SecretRotation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), secret_path,
		       rotation_type, COALESCE(cron_expression,''), COALESCE(rotation_interval_days,0),
		       last_rotated_at, last_rotated_by, next_rotation_at, expires_at,
		       status, rotation_count, service_ids, enabled,
		       COALESCE(last_error,''), last_error_at,
		       created_by, created_at, updated_at
		FROM secret_rotations
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list secret rotations: %w", err)
	}
	defer rows.Close()

	var rotations []models.SecretRotation
	for rows.Next() {
		var rot models.SecretRotation
		if err := rows.Scan(
			&rot.ID, &rot.TenantID, &rot.Name, &rot.Description, &rot.SecretPath,
			&rot.RotationType, &rot.CronExpression, &rot.RotationIntervalDays,
			&rot.LastRotatedAt, &rot.LastRotatedBy, &rot.NextRotationAt, &rot.ExpiresAt,
			&rot.Status, &rot.RotationCount, &rot.ServiceIDs, &rot.Enabled,
			&rot.LastError, &rot.LastErrorAt,
			&rot.CreatedBy, &rot.CreatedAt, &rot.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan secret rotation: %w", err)
		}
		rotations = append(rotations, rot)
	}
	return rotations, nil
}

// GetSecretRotation returns a secret rotation by ID.
func (r *DevOpsRepository) GetSecretRotation(ctx context.Context, id, tenantID uuid.UUID) (*models.SecretRotation, error) {
	var rot models.SecretRotation
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), secret_path,
		       rotation_type, COALESCE(cron_expression,''), COALESCE(rotation_interval_days,0),
		       last_rotated_at, last_rotated_by, next_rotation_at, expires_at,
		       status, rotation_count, service_ids, enabled,
		       COALESCE(last_error,''), last_error_at,
		       created_by, created_at, updated_at
		FROM secret_rotations
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(
		&rot.ID, &rot.TenantID, &rot.Name, &rot.Description, &rot.SecretPath,
		&rot.RotationType, &rot.CronExpression, &rot.RotationIntervalDays,
		&rot.LastRotatedAt, &rot.LastRotatedBy, &rot.NextRotationAt, &rot.ExpiresAt,
		&rot.Status, &rot.RotationCount, &rot.ServiceIDs, &rot.Enabled,
		&rot.LastError, &rot.LastErrorAt,
		&rot.CreatedBy, &rot.CreatedAt, &rot.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("secret rotation not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get secret rotation: %w", err)
	}
	return &rot, nil
}

// CreateSecretRotation creates a new secret rotation.
func (r *DevOpsRepository) CreateSecretRotation(ctx context.Context, rot *models.SecretRotation) error {
	rot.ID = uuid.New()
	now := time.Now().UTC()
	rot.CreatedAt = now
	rot.UpdatedAt = now

	_, err := r.pool.Exec(ctx, `
		INSERT INTO secret_rotations (
			id, tenant_id, name, description, secret_path, rotation_type,
			cron_expression, rotation_interval_days, last_rotated_at, last_rotated_by,
			next_rotation_at, expires_at, status, rotation_count, service_ids,
			enabled, last_error, last_error_at, created_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
	`, rot.ID, rot.TenantID, rot.Name, rot.Description, rot.SecretPath, rot.RotationType,
		rot.CronExpression, rot.RotationIntervalDays, rot.LastRotatedAt, rot.LastRotatedBy,
		rot.NextRotationAt, rot.ExpiresAt, rot.Status, rot.RotationCount, rot.ServiceIDs,
		rot.Enabled, rot.LastError, rot.LastErrorAt, rot.CreatedBy, rot.CreatedAt, rot.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create secret rotation: %w", err)
	}
	return nil
}

// UpdateSecretRotation updates a secret rotation.
func (r *DevOpsRepository) UpdateSecretRotation(ctx context.Context, rot *models.SecretRotation) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE secret_rotations SET
			name = $3, description = $4, secret_path = $5, rotation_type = $6,
			cron_expression = $7, rotation_interval_days = $8,
			next_rotation_at = $9, expires_at = $10, status = $11,
			service_ids = $12, enabled = $13, updated_at = $14
		WHERE id = $1 AND tenant_id = $2
	`, rot.ID, rot.TenantID, rot.Name, rot.Description, rot.SecretPath, rot.RotationType,
		rot.CronExpression, rot.RotationIntervalDays, rot.NextRotationAt, rot.ExpiresAt,
		rot.Status, rot.ServiceIDs, rot.Enabled, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("update secret rotation: %w", err)
	}
	return nil
}

// DeleteSecretRotation deletes a secret rotation.
func (r *DevOpsRepository) DeleteSecretRotation(ctx context.Context, id, tenantID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM secret_rotations WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return err
}

// MarkRotationExecuted updates rotation after successful execution.
func (r *DevOpsRepository) MarkRotationExecuted(ctx context.Context, id, tenantID uuid.UUID, nextRotationAt *time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE secret_rotations SET
			last_rotated_at = now(), next_rotation_at = $3,
			rotation_count = rotation_count + 1, last_error = '', last_error_at = NULL,
			status = 'active', updated_at = now()
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID, nextRotationAt)
	return err
}

// MarkRotationFailed updates rotation after failed execution.
func (r *DevOpsRepository) MarkRotationFailed(ctx context.Context, id, tenantID uuid.UUID, errMsg string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE secret_rotations SET
			last_error = $3, last_error_at = now(), status = 'failed', updated_at = now()
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID, errMsg)
	return err
}

// CreateSecretRotationLog creates a rotation execution log entry.
func (r *DevOpsRepository) CreateSecretRotationLog(ctx context.Context, log *models.SecretRotationLog) error {
	log.ID = uuid.New()
	log.ExecutedAt = time.Now().UTC()

	_, err := r.pool.Exec(ctx, `
		INSERT INTO secret_rotation_logs (
			id, tenant_id, rotation_id, status, details, error_message,
			triggered_by, trigger_type, executed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, log.ID, log.TenantID, log.RotationID, log.Status, log.Details, log.ErrorMessage,
		log.TriggeredBy, log.TriggerType, log.ExecutedAt)
	if err != nil {
		return fmt.Errorf("create rotation log: %w", err)
	}
	return nil
}

// GetSecretRotationLogs returns rotation logs for a rotation.
func (r *DevOpsRepository) GetSecretRotationLogs(ctx context.Context, rotationID, tenantID uuid.UUID, limit int) ([]models.SecretRotationLog, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, rotation_id, status, COALESCE(details,'{}'::jsonb),
		       COALESCE(error_message,''), triggered_by, trigger_type, executed_at
		FROM secret_rotation_logs
		WHERE rotation_id = $1 AND tenant_id = $2
		ORDER BY executed_at DESC
		LIMIT $3
	`, rotationID, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("get rotation logs: %w", err)
	}
	defer rows.Close()

	var logs []models.SecretRotationLog
	for rows.Next() {
		var l models.SecretRotationLog
		if err := rows.Scan(
			&l.ID, &l.TenantID, &l.RotationID, &l.Status, &l.Details,
			&l.ErrorMessage, &l.TriggeredBy, &l.TriggerType, &l.ExecutedAt,
		); err != nil {
			return nil, fmt.Errorf("scan rotation log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, nil
}

// GetExpiringSecrets returns secrets that will expire within the given duration.
func (r *DevOpsRepository) GetExpiringSecrets(ctx context.Context, tenantID uuid.UUID, within time.Duration) ([]models.SecretRotation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), secret_path,
		       rotation_type, COALESCE(cron_expression,''), COALESCE(rotation_interval_days,0),
		       last_rotated_at, last_rotated_by, next_rotation_at, expires_at,
		       status, rotation_count, service_ids, enabled,
		       COALESCE(last_error,''), last_error_at,
		       created_by, created_at, updated_at
		FROM secret_rotations
		WHERE tenant_id = $1 AND enabled = true
		  AND expires_at IS NOT NULL
		  AND expires_at <= now() + $2
		ORDER BY expires_at ASC
	`, tenantID, within)
	if err != nil {
		return nil, fmt.Errorf("get expiring secrets: %w", err)
	}
	defer rows.Close()

	var rotations []models.SecretRotation
	for rows.Next() {
		var rot models.SecretRotation
		if err := rows.Scan(
			&rot.ID, &rot.TenantID, &rot.Name, &rot.Description, &rot.SecretPath,
			&rot.RotationType, &rot.CronExpression, &rot.RotationIntervalDays,
			&rot.LastRotatedAt, &rot.LastRotatedBy, &rot.NextRotationAt, &rot.ExpiresAt,
			&rot.Status, &rot.RotationCount, &rot.ServiceIDs, &rot.Enabled,
			&rot.LastError, &rot.LastErrorAt,
			&rot.CreatedBy, &rot.CreatedAt, &rot.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan secret rotation: %w", err)
		}
		rotations = append(rotations, rot)
	}
	return rotations, nil
}

// ============================================================
// Deployment Audit Logs
// ============================================================

// CreateDeploymentAuditLog creates a deployment audit log entry.
func (r *DevOpsRepository) CreateDeploymentAuditLog(ctx context.Context, log *models.DeploymentAuditLog) error {
	log.ID = uuid.New()
	log.CreatedAt = time.Now().UTC()

	_, err := r.pool.Exec(ctx, `
		INSERT INTO deployment_audit_logs (
			id, tenant_id, deployment_id, service_id, action,
			actor_type, actor_id, actor_name, environment, cluster_id,
			namespace, image_tag, previous_state, new_state,
			compliance_results, security_gate_results,
			risk_score, risk_factors, status, error_message,
			metadata, ip_address, user_agent, request_id, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
	`, log.ID, log.TenantID, log.DeploymentID, log.ServiceID, log.Action,
		log.ActorType, log.ActorID, log.ActorName, log.Environment, log.ClusterID,
		log.Namespace, log.ImageTag, log.PreviousState, log.NewState,
		log.ComplianceResults, log.SecurityGateResults,
		log.RiskScore, log.RiskFactors, log.Status, log.ErrorMessage,
		log.Metadata, log.IPAddress, log.UserAgent, log.RequestID, log.CreatedAt)
	if err != nil {
		return fmt.Errorf("create deployment audit log: %w", err)
	}
	return nil
}

// ListDeploymentAuditLogs returns audit logs with filtering.
func (r *DevOpsRepository) ListDeploymentAuditLogs(ctx context.Context, tenantID uuid.UUID, filter models.DeploymentAuditFilter) ([]models.DeploymentAuditLog, int64, error) {
	query := `
		SELECT dal.id, dal.tenant_id, dal.deployment_id, dal.service_id, dal.action,
		       dal.actor_type, dal.actor_id, dal.actor_name, dal.environment, dal.cluster_id,
		       dal.namespace, dal.image_tag,
		       COALESCE(dal.previous_state,'{}'::jsonb), COALESCE(dal.new_state,'{}'::jsonb),
		       COALESCE(dal.compliance_results,'{}'::jsonb), COALESCE(dal.security_gate_results,'{}'::jsonb),
		       dal.risk_score, COALESCE(dal.risk_factors,'{}'::jsonb),
		       dal.status, COALESCE(dal.error_message,''),
		       COALESCE(dal.metadata,'{}'::jsonb), dal.ip_address, dal.user_agent, dal.request_id,
		       dal.created_at, COALESCE(s.name,'')
		FROM deployment_audit_logs dal
		LEFT JOIN services s ON s.id = dal.service_id
		WHERE dal.tenant_id = $1
	`
	countQuery := `SELECT COUNT(*) FROM deployment_audit_logs WHERE tenant_id = $1`
	args := []interface{}{tenantID}
	argIdx := 2

	if filter.Action != "" {
		query += fmt.Sprintf(` AND dal.action = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND dal.action = $%d`, argIdx)
		args = append(args, filter.Action)
		argIdx++
	}
	if filter.ServiceID != "" {
		query += fmt.Sprintf(` AND dal.service_id = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND dal.service_id = $%d`, argIdx)
		args = append(args, filter.ServiceID)
		argIdx++
	}
	if filter.Environment != "" {
		query += fmt.Sprintf(` AND dal.environment = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND dal.environment = $%d`, argIdx)
		args = append(args, filter.Environment)
		argIdx++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(` AND dal.status = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND dal.status = $%d`, argIdx)
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.ActorType != "" {
		query += fmt.Sprintf(` AND dal.actor_type = $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND dal.actor_type = $%d`, argIdx)
		args = append(args, filter.ActorType)
		argIdx++
	}
	if filter.StartDate != "" {
		query += fmt.Sprintf(` AND dal.created_at >= $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND dal.created_at >= $%d`, argIdx)
		args = append(args, filter.StartDate)
		argIdx++
	}
	if filter.EndDate != "" {
		query += fmt.Sprintf(` AND dal.created_at <= $%d`, argIdx)
		countQuery += fmt.Sprintf(` AND dal.created_at <= $%d`, argIdx)
		args = append(args, filter.EndDate)
		argIdx++
	}

	// Get total count
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}

	// Add pagination
	query += ` ORDER BY dal.created_at DESC`
	offset := (filter.Page - 1) * filter.PerPage
	query += fmt.Sprintf(` LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, filter.PerPage, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()

	var logs []models.DeploymentAuditLog
	for rows.Next() {
		var l models.DeploymentAuditLog
		if err := rows.Scan(
			&l.ID, &l.TenantID, &l.DeploymentID, &l.ServiceID, &l.Action,
			&l.ActorType, &l.ActorID, &l.ActorName, &l.Environment, &l.ClusterID,
			&l.Namespace, &l.ImageTag,
			&l.PreviousState, &l.NewState,
			&l.ComplianceResults, &l.SecurityGateResults,
			&l.RiskScore, &l.RiskFactors,
			&l.Status, &l.ErrorMessage,
			&l.Metadata, &l.IPAddress, &l.UserAgent, &l.RequestID,
			&l.CreatedAt, &l.ServiceName,
		); err != nil {
			return nil, 0, fmt.Errorf("scan audit log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, total, nil
}

// GetDeploymentAuditHistory returns audit logs for a specific deployment.
func (r *DevOpsRepository) GetDeploymentAuditHistory(ctx context.Context, tenantID, deploymentID uuid.UUID) ([]models.DeploymentAuditLog, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT dal.id, dal.tenant_id, dal.deployment_id, dal.service_id, dal.action,
		       dal.actor_type, dal.actor_id, dal.actor_name, dal.environment, dal.cluster_id,
		       dal.namespace, dal.image_tag,
		       COALESCE(dal.previous_state,'{}'::jsonb), COALESCE(dal.new_state,'{}'::jsonb),
		       COALESCE(dal.compliance_results,'{}'::jsonb), COALESCE(dal.security_gate_results,'{}'::jsonb),
		       dal.risk_score, COALESCE(dal.risk_factors,'{}'::jsonb),
		       dal.status, COALESCE(dal.error_message,''),
		       COALESCE(dal.metadata,'{}'::jsonb), dal.ip_address, dal.user_agent, dal.request_id,
		       dal.created_at, COALESCE(s.name,'')
		FROM deployment_audit_logs dal
		LEFT JOIN services s ON s.id = dal.service_id
		WHERE dal.tenant_id = $1 AND dal.deployment_id = $2
		ORDER BY dal.created_at DESC
	`, tenantID, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("get deployment audit history: %w", err)
	}
	defer rows.Close()

	var logs []models.DeploymentAuditLog
	for rows.Next() {
		var l models.DeploymentAuditLog
		if err := rows.Scan(
			&l.ID, &l.TenantID, &l.DeploymentID, &l.ServiceID, &l.Action,
			&l.ActorType, &l.ActorID, &l.ActorName, &l.Environment, &l.ClusterID,
			&l.Namespace, &l.ImageTag,
			&l.PreviousState, &l.NewState,
			&l.ComplianceResults, &l.SecurityGateResults,
			&l.RiskScore, &l.RiskFactors,
			&l.Status, &l.ErrorMessage,
			&l.Metadata, &l.IPAddress, &l.UserAgent, &l.RequestID,
			&l.CreatedAt, &l.ServiceName,
		); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, nil
}

// ============================================================
// Pre-Deploy Gate — combined check
// ============================================================

// CheckDeploymentAllowed checks if a deployment is allowed based on windows.
func (r *DevOpsRepository) CheckDeploymentAllowed(ctx context.Context, tenantID uuid.UUID, environment string, serviceID *uuid.UUID, userRoles []string) (*models.WindowCheckResult, error) {
	result := &models.WindowCheckResult{Allowed: true}

	windows, err := r.GetActiveWindows(ctx, tenantID, environment, serviceID)
	if err != nil {
		return nil, err
	}

	if len(windows) == 0 {
		// No active windows — deployment allowed by default
		return result, nil
	}

	result.ActiveWindows = windows

	// Check for blocking windows (blocked/freeze type)
	for _, w := range windows {
		if w.WindowType == "blocked" || w.WindowType == "freeze" {
			// Check if user can override
			canOverride := false
			for _, role := range userRoles {
				for _, overrideRole := range w.OverrideRoles {
					if role == overrideRole {
						canOverride = true
						break
					}
				}
				if canOverride {
					break
				}
			}

			if !canOverride {
				result.Allowed = false
				result.BlockingWindow = &w
				result.Reason = fmt.Sprintf("Deployment blocked: %s window '%s' is active. %s", w.WindowType, w.Name, w.Reason)
				return result, nil
			}
		}
	}

	// If there are 'allowed' type windows, deployment must be within one
	hasAllowedWindows := false
	for _, w := range windows {
		if w.WindowType == "allowed" {
			hasAllowedWindows = true
			break
		}
	}

	if hasAllowedWindows {
		// At least one allowed window is active, deployment is fine
		return result, nil
	}

	return result, nil
}

// Ensure json import is used
var _ = json.RawMessage{}
