package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pepa/pepa/internal/database"
)

// ScanTarget represents a target configured for security scanning.
type ScanTarget struct {
	ID              uuid.UUID        `json:"id"`
	TenantID        uuid.UUID        `json:"tenant_id"`
	Name            string           `json:"name"`
	ScannerType     string           `json:"scanner_type"`    // trivy|sonarqube|both
	TargetType      string           `json:"target_type"`     // image|git_repo|filesystem|container|service|sonarqube_project
	TargetRef       string           `json:"target_ref"`      // image name, repo URL, service ID, project key
	ConnectionID    *uuid.UUID       `json:"connection_id,omitempty"`
	ScanConfig      map[string]any   `json:"scan_config"`
	Enabled         bool             `json:"enabled"`
	LastScanAt      *time.Time       `json:"last_scan_at,omitempty"`
	LastScanStatus  *string          `json:"last_scan_status,omitempty"`
	LastScanSummary map[string]any   `json:"last_scan_summary,omitempty"`
	CreatedBy       *uuid.UUID       `json:"created_by,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// ScanRun represents a single scan execution.
type ScanRun struct {
	ID            uuid.UUID      `json:"id"`
	TenantID      uuid.UUID      `json:"tenant_id"`
	TargetID      uuid.UUID      `json:"target_id"`
	ScannerType   string         `json:"scanner_type"`
	Status        string         `json:"status"`        // pending|running|completed|failed|cancelled
	TriggerType   string         `json:"trigger_type"`  // manual|schedule|pipeline|webhook
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
	DurationMs    *int           `json:"duration_ms,omitempty"`
	ResultSummary map[string]any `json:"result_summary,omitempty"`
	ResultFull    map[string]any `json:"result_full,omitempty"`
	ErrorMessage  *string        `json:"error_message,omitempty"`
	ReportURL     *string        `json:"report_url,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	// Joined from scan_targets
	TargetName string `json:"target_name,omitempty"`
	TargetRef  string `json:"target_ref,omitempty"`
}

// ScanSchedule represents a cron-based recurring scan configuration.
type ScanSchedule struct {
	ID             uuid.UUID  `json:"id"`
	TenantID       uuid.UUID  `json:"tenant_id"`
	TargetID       uuid.UUID  `json:"target_id"`
	CronExpression string     `json:"cron_expression"`
	Enabled        bool       `json:"enabled"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	NextRunAt      *time.Time `json:"next_run_at,omitempty"`
	WorkflowID     *uuid.UUID `json:"workflow_id,omitempty"`
	CreatedBy      *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	// Joined from scan_targets
	TargetName  string `json:"target_name,omitempty"`
	TargetRef   string `json:"target_ref,omitempty"`
	ScannerType string `json:"scanner_type,omitempty"`
}

// SecurityScanRepository provides data access for security scanning.
type SecurityScanRepository struct {
	db *database.DB
}

// NewSecurityScanRepository creates a new SecurityScanRepository.
func NewSecurityScanRepository(db *database.DB) *SecurityScanRepository {
	return &SecurityScanRepository{db: db}
}

// ── Scan Targets ──────────────────────────────────────────────

// ListScanTargets returns all scan targets for a tenant.
func (r *SecurityScanRepository) ListScanTargets(ctx context.Context, tenantID uuid.UUID) ([]ScanTarget, error) {
	query := `
		SELECT id, tenant_id, name, scanner_type, target_type, target_ref,
		       connection_id, scan_config, enabled, last_scan_at, last_scan_status,
		       last_scan_summary, created_by, created_at, updated_at
		FROM scan_targets
		WHERE tenant_id = @tenant_id
		ORDER BY created_at DESC
	`
	rows, err := r.db.Pool.Query(ctx, query, pgx.NamedArgs{"tenant_id": tenantID})
	if err != nil {
		return nil, fmt.Errorf("list scan targets: %w", err)
	}
	defer rows.Close()

	var targets []ScanTarget
	for rows.Next() {
		var t ScanTarget
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.Name, &t.ScannerType, &t.TargetType, &t.TargetRef,
			&t.ConnectionID, &t.ScanConfig, &t.Enabled, &t.LastScanAt, &t.LastScanStatus,
			&t.LastScanSummary, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan scan target: %w", err)
		}
		targets = append(targets, t)
	}
	return targets, nil
}

// GetScanTarget returns a scan target by ID.
func (r *SecurityScanRepository) GetScanTarget(ctx context.Context, id, tenantID uuid.UUID) (*ScanTarget, error) {
	query := `
		SELECT id, tenant_id, name, scanner_type, target_type, target_ref,
		       connection_id, scan_config, enabled, last_scan_at, last_scan_status,
		       last_scan_summary, created_by, created_at, updated_at
		FROM scan_targets
		WHERE id = @id AND tenant_id = @tenant_id
	`
	var t ScanTarget
	err := r.db.Pool.QueryRow(ctx, query, pgx.NamedArgs{"id": id, "tenant_id": tenantID}).Scan(
		&t.ID, &t.TenantID, &t.Name, &t.ScannerType, &t.TargetType, &t.TargetRef,
		&t.ConnectionID, &t.ScanConfig, &t.Enabled, &t.LastScanAt, &t.LastScanStatus,
		&t.LastScanSummary, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("scan target not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get scan target: %w", err)
	}
	return &t, nil
}

// CreateScanTarget creates a new scan target.
func (r *SecurityScanRepository) CreateScanTarget(ctx context.Context, t *ScanTarget) error {
	query := `
		INSERT INTO scan_targets (
			tenant_id, name, scanner_type, target_type, target_ref,
			connection_id, scan_config, enabled, created_by
		) VALUES (
			@tenant_id, @name, @scanner_type, @target_type, @target_ref,
			@connection_id, @scan_config, @enabled, @created_by
		) RETURNING id, created_at, updated_at
	`
	return r.db.Pool.QueryRow(ctx, query, pgx.NamedArgs{
		"tenant_id":     t.TenantID,
		"name":          t.Name,
		"scanner_type":  t.ScannerType,
		"target_type":   t.TargetType,
		"target_ref":    t.TargetRef,
		"connection_id": t.ConnectionID,
		"scan_config":   t.ScanConfig,
		"enabled":       t.Enabled,
		"created_by":    t.CreatedBy,
	}).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

// UpdateScanTarget updates an existing scan target.
func (r *SecurityScanRepository) UpdateScanTarget(ctx context.Context, t *ScanTarget) error {
	query := `
		UPDATE scan_targets SET
			name = @name, scanner_type = @scanner_type, target_type = @target_type,
			target_ref = @target_ref, connection_id = @connection_id,
			scan_config = @scan_config, enabled = @enabled, updated_at = now()
		WHERE id = @id AND tenant_id = @tenant_id
		RETURNING updated_at
	`
	return r.db.Pool.QueryRow(ctx, query, pgx.NamedArgs{
		"id":            t.ID,
		"tenant_id":     t.TenantID,
		"name":          t.Name,
		"scanner_type":  t.ScannerType,
		"target_type":   t.TargetType,
		"target_ref":    t.TargetRef,
		"connection_id": t.ConnectionID,
		"scan_config":   t.ScanConfig,
		"enabled":       t.Enabled,
	}).Scan(&t.UpdatedAt)
}

// DeleteScanTarget deletes a scan target.
func (r *SecurityScanRepository) DeleteScanTarget(ctx context.Context, id, tenantID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM scan_targets WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return err
}

// UpdateScanTargetLastScan updates the last scan info on a target.
func (r *SecurityScanRepository) UpdateScanTargetLastScan(ctx context.Context, id, tenantID uuid.UUID, status string, summary map[string]any) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE scan_targets SET
			last_scan_at = now(), last_scan_status = @status,
			last_scan_summary = @summary, updated_at = now()
		WHERE id = @id AND tenant_id = @tenant_id
	`, pgx.NamedArgs{"id": id, "tenant_id": tenantID, "status": status, "summary": summary})
	return err
}

// ── Scan Runs ─────────────────────────────────────────────────

// ListScanRuns returns scan runs for a tenant with optional filters.
func (r *SecurityScanRepository) ListScanRuns(ctx context.Context, tenantID uuid.UUID, targetID *uuid.UUID, status string, limit, offset int) ([]ScanRun, error) {
	query := `
		SELECT sr.id, sr.tenant_id, sr.target_id, sr.scanner_type, sr.status,
		       sr.trigger_type, sr.started_at, sr.completed_at, sr.duration_ms,
		       sr.result_summary, sr.error_message, sr.report_url, sr.created_at,
		       COALESCE(st.name, ''), COALESCE(st.target_ref, '')
		FROM scan_runs sr
		LEFT JOIN scan_targets st ON st.id = sr.target_id
		WHERE sr.tenant_id = @tenant_id
	`
	args := pgx.NamedArgs{"tenant_id": tenantID}

	if targetID != nil {
		query += ` AND sr.target_id = @target_id`
		args["target_id"] = *targetID
	}
	if status != "" {
		query += ` AND sr.status = @status`
		args["status"] = status
	}

	query += ` ORDER BY sr.created_at DESC`
	if limit > 0 {
		query += ` LIMIT @limit`
		args["limit"] = limit
	}
	if offset > 0 {
		query += ` OFFSET @offset`
		args["offset"] = offset
	}

	rows, err := r.db.Pool.Query(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("list scan runs: %w", err)
	}
	defer rows.Close()

	var runs []ScanRun
	for rows.Next() {
		var s ScanRun
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.TargetID, &s.ScannerType, &s.Status,
			&s.TriggerType, &s.StartedAt, &s.CompletedAt, &s.DurationMs,
			&s.ResultSummary, &s.ErrorMessage, &s.ReportURL, &s.CreatedAt,
			&s.TargetName, &s.TargetRef,
		); err != nil {
			return nil, fmt.Errorf("scan scan run: %w", err)
		}
		runs = append(runs, s)
	}
	return runs, nil
}

// GetScanRun returns a scan run with full results.
func (r *SecurityScanRepository) GetScanRun(ctx context.Context, id, tenantID uuid.UUID) (*ScanRun, error) {
	query := `
		SELECT sr.id, sr.tenant_id, sr.target_id, sr.scanner_type, sr.status,
		       sr.trigger_type, sr.started_at, sr.completed_at, sr.duration_ms,
		       sr.result_summary, sr.result_full, sr.error_message, sr.report_url, sr.created_at,
		       COALESCE(st.name, ''), COALESCE(st.target_ref, '')
		FROM scan_runs sr
		LEFT JOIN scan_targets st ON st.id = sr.target_id
		WHERE sr.id = @id AND sr.tenant_id = @tenant_id
	`
	var s ScanRun
	err := r.db.Pool.QueryRow(ctx, query, pgx.NamedArgs{"id": id, "tenant_id": tenantID}).Scan(
		&s.ID, &s.TenantID, &s.TargetID, &s.ScannerType, &s.Status,
		&s.TriggerType, &s.StartedAt, &s.CompletedAt, &s.DurationMs,
		&s.ResultSummary, &s.ResultFull, &s.ErrorMessage, &s.ReportURL, &s.CreatedAt,
		&s.TargetName, &s.TargetRef,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("scan run not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get scan run: %w", err)
	}
	return &s, nil
}

// CreateScanRun creates a new scan run.
func (r *SecurityScanRepository) CreateScanRun(ctx context.Context, s *ScanRun) error {
	query := `
		INSERT INTO scan_runs (
			tenant_id, target_id, scanner_type, status, trigger_type
		) VALUES (
			@tenant_id, @target_id, @scanner_type, @status, @trigger_type
		) RETURNING id, created_at
	`
	return r.db.Pool.QueryRow(ctx, query, pgx.NamedArgs{
		"tenant_id":    s.TenantID,
		"target_id":    s.TargetID,
		"scanner_type": s.ScannerType,
		"status":       s.Status,
		"trigger_type": s.TriggerType,
	}).Scan(&s.ID, &s.CreatedAt)
}

// UpdateScanRun updates a scan run (status, results, etc.).
func (r *SecurityScanRepository) UpdateScanRun(ctx context.Context, s *ScanRun) error {
	query := `
		UPDATE scan_runs SET
			status = @status, started_at = @started_at, completed_at = @completed_at,
			duration_ms = @duration_ms, result_summary = @result_summary,
			result_full = @result_full, error_message = @error_message,
			report_url = @report_url
		WHERE id = @id AND tenant_id = @tenant_id
	`
	_, err := r.db.Pool.Exec(ctx, query, pgx.NamedArgs{
		"id":            s.ID,
		"tenant_id":     s.TenantID,
		"status":        s.Status,
		"started_at":    s.StartedAt,
		"completed_at":  s.CompletedAt,
		"duration_ms":   s.DurationMs,
		"result_summary": s.ResultSummary,
		"result_full":   s.ResultFull,
		"error_message": s.ErrorMessage,
		"report_url":    s.ReportURL,
	})
	return err
}

// ── Scan Schedules ────────────────────────────────────────────

// ListScanSchedules returns all schedules for a tenant.
func (r *SecurityScanRepository) ListScanSchedules(ctx context.Context, tenantID uuid.UUID) ([]ScanSchedule, error) {
	query := `
		SELECT ss.id, ss.tenant_id, ss.target_id, ss.cron_expression, ss.enabled,
		       ss.last_run_at, ss.next_run_at, ss.workflow_id, ss.created_by,
		       ss.created_at, ss.updated_at,
		       COALESCE(st.name, ''), COALESCE(st.target_ref, ''), COALESCE(st.scanner_type, '')
		FROM scan_schedules ss
		LEFT JOIN scan_targets st ON st.id = ss.target_id
		WHERE ss.tenant_id = @tenant_id
		ORDER BY ss.created_at DESC
	`
	rows, err := r.db.Pool.Query(ctx, query, pgx.NamedArgs{"tenant_id": tenantID})
	if err != nil {
		return nil, fmt.Errorf("list scan schedules: %w", err)
	}
	defer rows.Close()

	var schedules []ScanSchedule
	for rows.Next() {
		var s ScanSchedule
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.TargetID, &s.CronExpression, &s.Enabled,
			&s.LastRunAt, &s.NextRunAt, &s.WorkflowID, &s.CreatedBy,
			&s.CreatedAt, &s.UpdatedAt,
			&s.TargetName, &s.TargetRef, &s.ScannerType,
		); err != nil {
			return nil, fmt.Errorf("scan scan schedule: %w", err)
		}
		schedules = append(schedules, s)
	}
	return schedules, nil
}

// GetScanSchedule returns a schedule by ID.
func (r *SecurityScanRepository) GetScanSchedule(ctx context.Context, id, tenantID uuid.UUID) (*ScanSchedule, error) {
	query := `
		SELECT ss.id, ss.tenant_id, ss.target_id, ss.cron_expression, ss.enabled,
		       ss.last_run_at, ss.next_run_at, ss.workflow_id, ss.created_by,
		       ss.created_at, ss.updated_at,
		       COALESCE(st.name, ''), COALESCE(st.target_ref, ''), COALESCE(st.scanner_type, '')
		FROM scan_schedules ss
		LEFT JOIN scan_targets st ON st.id = ss.target_id
		WHERE ss.id = @id AND ss.tenant_id = @tenant_id
	`
	var s ScanSchedule
	err := r.db.Pool.QueryRow(ctx, query, pgx.NamedArgs{"id": id, "tenant_id": tenantID}).Scan(
		&s.ID, &s.TenantID, &s.TargetID, &s.CronExpression, &s.Enabled,
		&s.LastRunAt, &s.NextRunAt, &s.WorkflowID, &s.CreatedBy,
		&s.CreatedAt, &s.UpdatedAt,
		&s.TargetName, &s.TargetRef, &s.ScannerType,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("scan schedule not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get scan schedule: %w", err)
	}
	return &s, nil
}

// CreateScanSchedule creates a new schedule.
func (r *SecurityScanRepository) CreateScanSchedule(ctx context.Context, s *ScanSchedule) error {
	query := `
		INSERT INTO scan_schedules (
			tenant_id, target_id, cron_expression, enabled, next_run_at, created_by
		) VALUES (
			@tenant_id, @target_id, @cron_expression, @enabled, @next_run_at, @created_by
		) RETURNING id, created_at, updated_at
	`
	return r.db.Pool.QueryRow(ctx, query, pgx.NamedArgs{
		"tenant_id":       s.TenantID,
		"target_id":       s.TargetID,
		"cron_expression": s.CronExpression,
		"enabled":         s.Enabled,
		"next_run_at":     s.NextRunAt,
		"created_by":      s.CreatedBy,
	}).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

// UpdateScanSchedule updates a schedule.
func (r *SecurityScanRepository) UpdateScanSchedule(ctx context.Context, s *ScanSchedule) error {
	query := `
		UPDATE scan_schedules SET
			cron_expression = @cron_expression, enabled = @enabled,
			next_run_at = @next_run_at, last_run_at = @last_run_at, updated_at = now()
		WHERE id = @id AND tenant_id = @tenant_id
		RETURNING updated_at
	`
	return r.db.Pool.QueryRow(ctx, query, pgx.NamedArgs{
		"id":              s.ID,
		"tenant_id":       s.TenantID,
		"cron_expression": s.CronExpression,
		"enabled":         s.Enabled,
		"next_run_at":     s.NextRunAt,
		"last_run_at":     s.LastRunAt,
	}).Scan(&s.UpdatedAt)
}

// DeleteScanSchedule deletes a schedule.
func (r *SecurityScanRepository) DeleteScanSchedule(ctx context.Context, id, tenantID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM scan_schedules WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return err
}

// GetDueSchedules returns all enabled schedules where next_run_at <= now.
func (r *SecurityScanRepository) GetDueSchedules(ctx context.Context) ([]ScanSchedule, error) {
	query := `
		SELECT ss.id, ss.tenant_id, ss.target_id, ss.cron_expression, ss.enabled,
		       ss.last_run_at, ss.next_run_at, ss.workflow_id, ss.created_by,
		       ss.created_at, ss.updated_at,
		       COALESCE(st.name, ''), COALESCE(st.target_ref, ''), COALESCE(st.scanner_type, '')
		FROM scan_schedules ss
		LEFT JOIN scan_targets st ON st.id = ss.target_id
		WHERE ss.enabled = true AND ss.next_run_at IS NOT NULL AND ss.next_run_at <= now()
		ORDER BY ss.next_run_at ASC
	`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get due schedules: %w", err)
	}
	defer rows.Close()

	var schedules []ScanSchedule
	for rows.Next() {
		var s ScanSchedule
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.TargetID, &s.CronExpression, &s.Enabled,
			&s.LastRunAt, &s.NextRunAt, &s.WorkflowID, &s.CreatedBy,
			&s.CreatedAt, &s.UpdatedAt,
			&s.TargetName, &s.TargetRef, &s.ScannerType,
		); err != nil {
			return nil, fmt.Errorf("scan due schedule: %w", err)
		}
		schedules = append(schedules, s)
	}
	return schedules, nil
}

// ListAllScanSchedules returns all schedules across all tenants (for the scheduler).
func (r *SecurityScanRepository) ListAllScanSchedules(ctx context.Context) ([]ScanSchedule, error) {
	query := `
		SELECT ss.id, ss.tenant_id, ss.target_id, ss.cron_expression, ss.enabled,
		       ss.last_run_at, ss.next_run_at, ss.workflow_id, ss.created_by,
		       ss.created_at, ss.updated_at,
		       COALESCE(st.name, ''), COALESCE(st.target_ref, ''), COALESCE(st.scanner_type, '')
		FROM scan_schedules ss
		LEFT JOIN scan_targets st ON st.id = ss.target_id
		WHERE ss.enabled = true AND ss.next_run_at IS NOT NULL
		ORDER BY ss.next_run_at ASC
	`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list all scan schedules: %w", err)
	}
	defer rows.Close()

	var schedules []ScanSchedule
	for rows.Next() {
		var s ScanSchedule
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.TargetID, &s.CronExpression, &s.Enabled,
			&s.LastRunAt, &s.NextRunAt, &s.WorkflowID, &s.CreatedBy,
			&s.CreatedAt, &s.UpdatedAt,
			&s.TargetName, &s.TargetRef, &s.ScannerType,
		); err != nil {
			return nil, fmt.Errorf("scan all schedule: %w", err)
		}
		schedules = append(schedules, s)
	}
	return schedules, nil
}

// GetEnabledTargets returns all enabled scan targets for a tenant.
func (r *SecurityScanRepository) GetEnabledTargets(ctx context.Context, tenantID uuid.UUID) ([]ScanTarget, error) {
	query := `
		SELECT id, tenant_id, name, scanner_type, target_type, target_ref,
		       connection_id, scan_config, enabled, last_scan_at, last_scan_status,
		       last_scan_summary, created_by, created_at, updated_at
		FROM scan_targets
		WHERE tenant_id = @tenant_id AND enabled = true
		ORDER BY name ASC
	`
	rows, err := r.db.Pool.Query(ctx, query, pgx.NamedArgs{"tenant_id": tenantID})
	if err != nil {
		return nil, fmt.Errorf("get enabled targets: %w", err)
	}
	defer rows.Close()

	var targets []ScanTarget
	for rows.Next() {
		var t ScanTarget
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.Name, &t.ScannerType, &t.TargetType, &t.TargetRef,
			&t.ConnectionID, &t.ScanConfig, &t.Enabled, &t.LastScanAt, &t.LastScanStatus,
			&t.LastScanSummary, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan enabled target: %w", err)
		}
		targets = append(targets, t)
	}
	return targets, nil
}
