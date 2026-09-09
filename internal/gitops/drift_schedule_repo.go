package gitops

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pepa/pepa/internal/database"
)

// DriftSchedule represents a cron-based drift detection configuration.
type DriftSchedule struct {
	ID                   uuid.UUID  `json:"id"`
	TenantID             uuid.UUID  `json:"tenant_id"`
	RepoID               uuid.UUID  `json:"repo_id"`
	ClusterID            uuid.UUID  `json:"cluster_id"`
	ScopePath            *string    `json:"scope_path,omitempty"`
	Name                 string     `json:"name"`
	Description          *string    `json:"description,omitempty"`
	CronExpression       string     `json:"cron_expression"`
	Enabled              bool       `json:"enabled"`
	AlertOnDrift         bool       `json:"alert_on_drift"`
	AlertSeverityThreshold string   `json:"alert_severity_threshold"`
	LastRunAt            *time.Time `json:"last_run_at,omitempty"`
	LastRunStatus        *string    `json:"last_run_status,omitempty"`
	LastDriftCount       int        `json:"last_drift_count"`
	NextRunAt            *time.Time `json:"next_run_at,omitempty"`
	CreatedBy            *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	// Joined fields
	RepoName    string `json:"repo_name,omitempty"`
	ClusterName string `json:"cluster_name,omitempty"`
}

// DriftDetectionLog represents a drift detection run log entry.
type DriftDetectionLog struct {
	ID            uuid.UUID              `json:"id"`
	TenantID      uuid.UUID              `json:"tenant_id"`
	ScheduleID    *uuid.UUID             `json:"schedule_id,omitempty"`
	RepoID        uuid.UUID              `json:"repo_id"`
	ClusterID     uuid.UUID              `json:"cluster_id"`
	ScopePath     *string                `json:"scope_path,omitempty"`
	TriggeredBy   string                 `json:"triggered_by"`
	Status        string                 `json:"status"`
	DriftCount    int                    `json:"drift_count"`
	CriticalCount int                    `json:"critical_count"`
	WarningCount  int                    `json:"warning_count"`
	InfoCount     int                    `json:"info_count"`
	DriftDetails  map[string]interface{} `json:"drift_details,omitempty"`
	StartedAt     time.Time              `json:"started_at"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	ErrorMessage  *string                `json:"error_message,omitempty"`
	// Joined fields
	RepoName    string `json:"repo_name,omitempty"`
	ClusterName string `json:"cluster_name,omitempty"`
}

// DriftScheduleRepository provides data access for drift detection schedules.
type DriftScheduleRepository struct {
	db *database.DB
}

// NewDriftScheduleRepository creates a new DriftScheduleRepository.
func NewDriftScheduleRepository(db *database.DB) *DriftScheduleRepository {
	return &DriftScheduleRepository{db: db}
}

// ── Drift Schedules ────────────────────────────────────────────

// List returns all drift schedules for a tenant.
func (r *DriftScheduleRepository) List(ctx context.Context, tenantID uuid.UUID) ([]DriftSchedule, error) {
	query := `
		SELECT ds.id, ds.tenant_id, ds.repo_id, ds.cluster_id, ds.scope_path,
		       ds.name, ds.description, ds.cron_expression, ds.enabled,
		       ds.alert_on_drift, ds.alert_severity_threshold,
		       ds.last_run_at, ds.last_run_status, ds.last_drift_count,
		       ds.next_run_at, ds.created_by, ds.created_at, ds.updated_at,
		       COALESCE(gr.name, ''), COALESCE(c.name, '')
		FROM drift_schedules ds
		LEFT JOIN gitops_repositories gr ON gr.id = ds.repo_id
		LEFT JOIN clusters c ON c.id = ds.cluster_id
		WHERE ds.tenant_id = @tenant_id
		ORDER BY ds.created_at DESC
	`
	rows, err := r.db.Pool.Query(ctx, query, pgx.NamedArgs{"tenant_id": tenantID})
	if err != nil {
		return nil, fmt.Errorf("list drift schedules: %w", err)
	}
	defer rows.Close()

	var schedules []DriftSchedule
	for rows.Next() {
		var s DriftSchedule
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.RepoID, &s.ClusterID, &s.ScopePath,
			&s.Name, &s.Description, &s.CronExpression, &s.Enabled,
			&s.AlertOnDrift, &s.AlertSeverityThreshold,
			&s.LastRunAt, &s.LastRunStatus, &s.LastDriftCount,
			&s.NextRunAt, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
			&s.RepoName, &s.ClusterName,
		); err != nil {
			return nil, fmt.Errorf("scan drift schedule: %w", err)
		}
		schedules = append(schedules, s)
	}
	return schedules, nil
}

// Get returns a drift schedule by ID.
func (r *DriftScheduleRepository) Get(ctx context.Context, id, tenantID uuid.UUID) (*DriftSchedule, error) {
	query := `
		SELECT ds.id, ds.tenant_id, ds.repo_id, ds.cluster_id, ds.scope_path,
		       ds.name, ds.description, ds.cron_expression, ds.enabled,
		       ds.alert_on_drift, ds.alert_severity_threshold,
		       ds.last_run_at, ds.last_run_status, ds.last_drift_count,
		       ds.next_run_at, ds.created_by, ds.created_at, ds.updated_at,
		       COALESCE(gr.name, ''), COALESCE(c.name, '')
		FROM drift_schedules ds
		LEFT JOIN gitops_repositories gr ON gr.id = ds.repo_id
		LEFT JOIN clusters c ON c.id = ds.cluster_id
		WHERE ds.id = @id AND ds.tenant_id = @tenant_id
	`
	var s DriftSchedule
	err := r.db.Pool.QueryRow(ctx, query, pgx.NamedArgs{"id": id, "tenant_id": tenantID}).Scan(
		&s.ID, &s.TenantID, &s.RepoID, &s.ClusterID, &s.ScopePath,
		&s.Name, &s.Description, &s.CronExpression, &s.Enabled,
		&s.AlertOnDrift, &s.AlertSeverityThreshold,
		&s.LastRunAt, &s.LastRunStatus, &s.LastDriftCount,
		&s.NextRunAt, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
		&s.RepoName, &s.ClusterName,
	)
	if err != nil {
		return nil, fmt.Errorf("get drift schedule: %w", err)
	}
	return &s, nil
}

// Create creates a new drift schedule.
func (r *DriftScheduleRepository) Create(ctx context.Context, s *DriftSchedule) error {
	query := `
		INSERT INTO drift_schedules (
			tenant_id, repo_id, cluster_id, scope_path, name, description,
			cron_expression, enabled, alert_on_drift, alert_severity_threshold,
			next_run_at, created_by
		) VALUES (
			@tenant_id, @repo_id, @cluster_id, @scope_path, @name, @description,
			@cron_expression, @enabled, @alert_on_drift, @alert_severity_threshold,
			@next_run_at, @created_by
		) RETURNING id, created_at, updated_at
	`
	return r.db.Pool.QueryRow(ctx, query, pgx.NamedArgs{
		"tenant_id":              s.TenantID,
		"repo_id":                s.RepoID,
		"cluster_id":             s.ClusterID,
		"scope_path":             s.ScopePath,
		"name":                   s.Name,
		"description":            s.Description,
		"cron_expression":        s.CronExpression,
		"enabled":                s.Enabled,
		"alert_on_drift":         s.AlertOnDrift,
		"alert_severity_threshold": s.AlertSeverityThreshold,
		"next_run_at":            s.NextRunAt,
		"created_by":             s.CreatedBy,
	}).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

// Update updates a drift schedule.
func (r *DriftScheduleRepository) Update(ctx context.Context, s *DriftSchedule) error {
	query := `
		UPDATE drift_schedules SET
			repo_id = @repo_id, cluster_id = @cluster_id, scope_path = @scope_path,
			name = @name, description = @description,
			cron_expression = @cron_expression, enabled = @enabled,
			alert_on_drift = @alert_on_drift,
			alert_severity_threshold = @alert_severity_threshold,
			last_run_at = @last_run_at, last_run_status = @last_run_status,
			last_drift_count = @last_drift_count, next_run_at = @next_run_at,
			updated_at = NOW()
		WHERE id = @id AND tenant_id = @tenant_id
		RETURNING updated_at
	`
	return r.db.Pool.QueryRow(ctx, query, pgx.NamedArgs{
		"id":                     s.ID,
		"tenant_id":              s.TenantID,
		"repo_id":                s.RepoID,
		"cluster_id":             s.ClusterID,
		"scope_path":             s.ScopePath,
		"name":                   s.Name,
		"description":            s.Description,
		"cron_expression":        s.CronExpression,
		"enabled":                s.Enabled,
		"alert_on_drift":         s.AlertOnDrift,
		"alert_severity_threshold": s.AlertSeverityThreshold,
		"last_run_at":            s.LastRunAt,
		"last_run_status":        s.LastRunStatus,
		"last_drift_count":       s.LastDriftCount,
		"next_run_at":            s.NextRunAt,
	}).Scan(&s.UpdatedAt)
}

// Delete deletes a drift schedule.
func (r *DriftScheduleRepository) Delete(ctx context.Context, id, tenantID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM drift_schedules WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return err
}

// GetDueSchedules returns all enabled schedules where next_run_at <= now.
func (r *DriftScheduleRepository) GetDueSchedules(ctx context.Context) ([]DriftSchedule, error) {
	query := `
		SELECT ds.id, ds.tenant_id, ds.repo_id, ds.cluster_id, ds.scope_path,
		       ds.name, ds.description, ds.cron_expression, ds.enabled,
		       ds.alert_on_drift, ds.alert_severity_threshold,
		       ds.last_run_at, ds.last_run_status, ds.last_drift_count,
		       ds.next_run_at, ds.created_by, ds.created_at, ds.updated_at,
		       COALESCE(gr.name, ''), COALESCE(c.name, '')
		FROM drift_schedules ds
		LEFT JOIN gitops_repositories gr ON gr.id = ds.repo_id
		LEFT JOIN clusters c ON c.id = ds.cluster_id
		WHERE ds.enabled = true AND ds.next_run_at IS NOT NULL AND ds.next_run_at <= now()
		ORDER BY ds.next_run_at ASC
	`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get due drift schedules: %w", err)
	}
	defer rows.Close()

	var schedules []DriftSchedule
	for rows.Next() {
		var s DriftSchedule
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.RepoID, &s.ClusterID, &s.ScopePath,
			&s.Name, &s.Description, &s.CronExpression, &s.Enabled,
			&s.AlertOnDrift, &s.AlertSeverityThreshold,
			&s.LastRunAt, &s.LastRunStatus, &s.LastDriftCount,
			&s.NextRunAt, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
			&s.RepoName, &s.ClusterName,
		); err != nil {
			return nil, fmt.Errorf("scan due drift schedule: %w", err)
		}
		schedules = append(schedules, s)
	}
	return schedules, nil
}

// ListAllSchedules returns all enabled schedules across all tenants (for the scheduler).
func (r *DriftScheduleRepository) ListAllSchedules(ctx context.Context) ([]DriftSchedule, error) {
	query := `
		SELECT ds.id, ds.tenant_id, ds.repo_id, ds.cluster_id, ds.scope_path,
		       ds.name, ds.description, ds.cron_expression, ds.enabled,
		       ds.alert_on_drift, ds.alert_severity_threshold,
		       ds.last_run_at, ds.last_run_status, ds.last_drift_count,
		       ds.next_run_at, ds.created_by, ds.created_at, ds.updated_at,
		       COALESCE(gr.name, ''), COALESCE(c.name, '')
		FROM drift_schedules ds
		LEFT JOIN gitops_repositories gr ON gr.id = ds.repo_id
		LEFT JOIN clusters c ON c.id = ds.cluster_id
		WHERE ds.enabled = true AND ds.next_run_at IS NOT NULL
		ORDER BY ds.next_run_at ASC
	`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list all drift schedules: %w", err)
	}
	defer rows.Close()

	var schedules []DriftSchedule
	for rows.Next() {
		var s DriftSchedule
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.RepoID, &s.ClusterID, &s.ScopePath,
			&s.Name, &s.Description, &s.CronExpression, &s.Enabled,
			&s.AlertOnDrift, &s.AlertSeverityThreshold,
			&s.LastRunAt, &s.LastRunStatus, &s.LastDriftCount,
			&s.NextRunAt, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
			&s.RepoName, &s.ClusterName,
		); err != nil {
			return nil, fmt.Errorf("scan all drift schedule: %w", err)
		}
		schedules = append(schedules, s)
	}
	return schedules, nil
}

// ── Drift Detection Logs ──────────────────────────────────────

// CreateLog creates a drift detection log entry.
func (r *DriftScheduleRepository) CreateLog(ctx context.Context, l *DriftDetectionLog) error {
	query := `
		INSERT INTO drift_detection_logs (
			tenant_id, schedule_id, repo_id, cluster_id, scope_path,
			triggered_by, status, drift_count, critical_count, warning_count,
			info_count, drift_details, completed_at, error_message
		) VALUES (
			@tenant_id, @schedule_id, @repo_id, @cluster_id, @scope_path,
			@triggered_by, @status, @drift_count, @critical_count, @warning_count,
			@info_count, @drift_details, @completed_at, @error_message
		) RETURNING id, started_at
	`
	return r.db.Pool.QueryRow(ctx, query, pgx.NamedArgs{
		"tenant_id":      l.TenantID,
		"schedule_id":    l.ScheduleID,
		"repo_id":        l.RepoID,
		"cluster_id":     l.ClusterID,
		"scope_path":     l.ScopePath,
		"triggered_by":   l.TriggeredBy,
		"status":         l.Status,
		"drift_count":    l.DriftCount,
		"critical_count": l.CriticalCount,
		"warning_count":  l.WarningCount,
		"info_count":     l.InfoCount,
		"drift_details":  l.DriftDetails,
		"completed_at":   l.CompletedAt,
		"error_message":  l.ErrorMessage,
	}).Scan(&l.ID, &l.StartedAt)
}

// ListLogs returns drift detection logs for a tenant.
func (r *DriftScheduleRepository) ListLogs(ctx context.Context, tenantID uuid.UUID, limit int) ([]DriftDetectionLog, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT dl.id, dl.tenant_id, dl.schedule_id, dl.repo_id, dl.cluster_id,
		       dl.scope_path, dl.triggered_by, dl.status, dl.drift_count,
		       dl.critical_count, dl.warning_count, dl.info_count,
		       dl.drift_details, dl.started_at, dl.completed_at, dl.error_message,
		       COALESCE(gr.name, ''), COALESCE(c.name, '')
		FROM drift_detection_logs dl
		LEFT JOIN gitops_repositories gr ON gr.id = dl.repo_id
		LEFT JOIN clusters c ON c.id = dl.cluster_id
		WHERE dl.tenant_id = @tenant_id
		ORDER BY dl.started_at DESC
		LIMIT @limit
	`
	rows, err := r.db.Pool.Query(ctx, query, pgx.NamedArgs{"tenant_id": tenantID, "limit": limit})
	if err != nil {
		return nil, fmt.Errorf("list drift logs: %w", err)
	}
	defer rows.Close()

	var logs []DriftDetectionLog
	for rows.Next() {
		var l DriftDetectionLog
		if err := rows.Scan(
			&l.ID, &l.TenantID, &l.ScheduleID, &l.RepoID, &l.ClusterID,
			&l.ScopePath, &l.TriggeredBy, &l.Status, &l.DriftCount,
			&l.CriticalCount, &l.WarningCount, &l.InfoCount,
			&l.DriftDetails, &l.StartedAt, &l.CompletedAt, &l.ErrorMessage,
			&l.RepoName, &l.ClusterName,
		); err != nil {
			return nil, fmt.Errorf("scan drift log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, nil
}
