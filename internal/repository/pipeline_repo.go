package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/pkg/models"
)

// ─────────────────────────────────────────────────────────────
// PipelineSourceRepository
// ─────────────────────────────────────────────────────────────

// PipelineSourceRepository handles pipeline source persistence.
type PipelineSourceRepository struct {
	pool *pgxpool.Pool
}

// NewPipelineSourceRepository creates a new pipeline source repository.
func NewPipelineSourceRepository(db *database.DB) *PipelineSourceRepository {
	return &PipelineSourceRepository{pool: db.Pool}
}

// List returns pipeline sources with pagination.
func (r *PipelineSourceRepository) List(ctx context.Context, tenantID uuid.UUID, page, perPage int) ([]models.PipelineSource, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	var total int64
	if err := r.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM pipeline_sources WHERE tenant_id = $1", tenantID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count pipeline sources: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, source_type, COALESCE(description,''),
		       connection_id, COALESCE(config,'{}'::jsonb),
		       COALESCE(parameter_schema,'{}'::jsonb), schema_fetched_at,
		       status, COALESCE(last_error,''), created_by, created_at, updated_at
		FROM pipeline_sources
		WHERE tenant_id = $1
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query pipeline sources: %w", err)
	}
	defer rows.Close()

	sources := make([]models.PipelineSource, 0)
	for rows.Next() {
		s, err := scanPipelineSource(rows)
		if err != nil {
			return nil, 0, err
		}
		sources = append(sources, *s)
	}
	return sources, total, nil
}

// Get returns a single pipeline source by ID.
func (r *PipelineSourceRepository) Get(ctx context.Context, id uuid.UUID) (*models.PipelineSource, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, source_type, COALESCE(description,''),
		       connection_id, COALESCE(config,'{}'::jsonb),
		       COALESCE(parameter_schema,'{}'::jsonb), schema_fetched_at,
		       status, COALESCE(last_error,''), created_by, created_at, updated_at
		FROM pipeline_sources WHERE id = $1
	`, id)

	var s models.PipelineSource
	var configJSON, schemaJSON []byte
	var connID sql.NullString
	var schemaAt sql.NullTime
	var lastErr, createdBy sql.NullString

	err := row.Scan(&s.ID, &s.TenantID, &s.Name, &s.SourceType, &s.Description,
		&connID, &configJSON, &schemaJSON, &schemaAt, &s.Status, &lastErr,
		&createdBy, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("pipeline source not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get pipeline source: %w", err)
	}

	if connID.Valid {
		uid := uuid.MustParse(connID.String)
		s.ConnectionID = &uid
	}
	if configJSON != nil {
		s.Config = configJSON
	}
	if schemaJSON != nil {
		s.ParameterSchema = schemaJSON
	}
	if schemaAt.Valid {
		s.SchemaFetchedAt = &schemaAt.Time
	}
	if lastErr.Valid {
		s.LastError = lastErr.String
	}
	if createdBy.Valid {
		uid := uuid.MustParse(createdBy.String)
		s.CreatedBy = &uid
	}
	return &s, nil
}

// Create inserts a new pipeline source.
func (r *PipelineSourceRepository) Create(ctx context.Context, s *models.PipelineSource) error {
	id := uuid.New()
	now := time.Now().UTC()
	s.ID = id
	s.CreatedAt = now
	s.UpdatedAt = now

	config := s.Config
	if config == nil {
		config = json.RawMessage("{}")
	}
	schema := s.ParameterSchema
	if schema == nil {
		schema = json.RawMessage("{}")
	}
	if s.Status == "" {
		s.Status = "active"
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO pipeline_sources (id, tenant_id, name, source_type, description,
		                              connection_id, config, parameter_schema,
		                              schema_fetched_at, status, last_error, created_by,
		                              created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, id, s.TenantID, s.Name, s.SourceType, s.Description,
		s.ConnectionID, config, schema, s.SchemaFetchedAt, s.Status, s.LastError,
		s.CreatedBy, now, now)
	if err != nil {
		return fmt.Errorf("create pipeline source: %w", err)
	}
	return nil
}

// Update modifies an existing pipeline source.
func (r *PipelineSourceRepository) Update(ctx context.Context, id uuid.UUID, s *models.PipelineSource) error {
	now := time.Now().UTC()

	config := s.Config
	if config == nil {
		config = json.RawMessage("{}")
	}

	tag, err := r.pool.Exec(ctx, `
		UPDATE pipeline_sources SET name=$1, source_type=$2, description=$3,
		                            connection_id=$4, config=$5, status=$6,
		                            last_error=$7, updated_at=$8
		WHERE id=$9
	`, s.Name, s.SourceType, s.Description, s.ConnectionID, config, s.Status,
		s.LastError, now, id)
	if err != nil {
		return fmt.Errorf("update pipeline source: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("pipeline source not found: %s", id)
	}
	return nil
}

// UpdateSchema updates the parameter schema for a pipeline source.
func (r *PipelineSourceRepository) UpdateSchema(ctx context.Context, id uuid.UUID, schema json.RawMessage) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE pipeline_sources SET parameter_schema=$1, schema_fetched_at=$2, updated_at=$2
		WHERE id=$3
	`, schema, now, id)
	if err != nil {
		return fmt.Errorf("update pipeline source schema: %w", err)
	}
	return nil
}

// Delete removes a pipeline source.
func (r *PipelineSourceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM pipeline_sources WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete pipeline source: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("pipeline source not found: %s", id)
	}
	return nil
}

func scanPipelineSource(rows pgx.Rows) (*models.PipelineSource, error) {
	var s models.PipelineSource
	var configJSON, schemaJSON []byte
	var connID, lastErr, createdBy sql.NullString
	var schemaAt sql.NullTime

	err := rows.Scan(&s.ID, &s.TenantID, &s.Name, &s.SourceType, &s.Description,
		&connID, &configJSON, &schemaJSON, &schemaAt, &s.Status, &lastErr,
		&createdBy, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan pipeline source: %w", err)
	}
	if connID.Valid {
		uid := uuid.MustParse(connID.String)
		s.ConnectionID = &uid
	}
	if configJSON != nil {
		s.Config = configJSON
	}
	if schemaJSON != nil {
		s.ParameterSchema = schemaJSON
	}
	if schemaAt.Valid {
		s.SchemaFetchedAt = &schemaAt.Time
	}
	if lastErr.Valid {
		s.LastError = lastErr.String
	}
	if createdBy.Valid {
		uid := uuid.MustParse(createdBy.String)
		s.CreatedBy = &uid
	}
	return &s, nil
}

// ─────────────────────────────────────────────────────────────
// PipelinePresetRepository
// ─────────────────────────────────────────────────────────────

// PipelinePresetRepository handles pipeline preset persistence.
type PipelinePresetRepository struct {
	pool *pgxpool.Pool
}

// NewPipelinePresetRepository creates a new pipeline preset repository.
func NewPipelinePresetRepository(db *database.DB) *PipelinePresetRepository {
	return &PipelinePresetRepository{pool: db.Pool}
}

// List returns presets for a given source.
func (r *PipelinePresetRepository) List(ctx context.Context, sourceID uuid.UUID) ([]models.PipelinePreset, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, source_id, name, COALESCE(description,''),
		       COALESCE(parameters,'{}'::jsonb), created_by, use_count,
		       last_used_at, created_at, updated_at
		FROM pipeline_presets
		WHERE source_id = $1
		ORDER BY use_count DESC, name ASC
	`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("query presets: %w", err)
	}
	defer rows.Close()

	presets := make([]models.PipelinePreset, 0)
	for rows.Next() {
		p, err := scanPreset(rows)
		if err != nil {
			return nil, err
		}
		presets = append(presets, *p)
	}
	return presets, nil
}

// Get returns a single preset by ID.
func (r *PipelinePresetRepository) Get(ctx context.Context, id uuid.UUID) (*models.PipelinePreset, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, source_id, name, COALESCE(description,''),
		       COALESCE(parameters,'{}'::jsonb), created_by, use_count,
		       last_used_at, created_at, updated_at
		FROM pipeline_presets WHERE id = $1
	`, id)

	var p models.PipelinePreset
	var paramsJSON []byte
	var createdBy sql.NullString
	var lastUsed sql.NullTime

	err := row.Scan(&p.ID, &p.TenantID, &p.SourceID, &p.Name, &p.Description,
		&paramsJSON, &createdBy, &p.UseCount, &lastUsed, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("preset not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get preset: %w", err)
	}
	if paramsJSON != nil {
		p.Parameters = paramsJSON
	}
	if createdBy.Valid {
		uid := uuid.MustParse(createdBy.String)
		p.CreatedBy = &uid
	}
	if lastUsed.Valid {
		p.LastUsedAt = &lastUsed.Time
	}
	return &p, nil
}

// Create inserts a new preset.
func (r *PipelinePresetRepository) Create(ctx context.Context, p *models.PipelinePreset) error {
	id := uuid.New()
	now := time.Now().UTC()
	p.ID = id
	p.CreatedAt = now
	p.UpdatedAt = now

	params := p.Parameters
	if params == nil {
		params = json.RawMessage("{}")
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO pipeline_presets (id, tenant_id, source_id, name, description,
		                              parameters, created_by, use_count, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, id, p.TenantID, p.SourceID, p.Name, p.Description, params, p.CreatedBy, 0, now, now)
	if err != nil {
		return fmt.Errorf("create preset: %w", err)
	}
	return nil
}

// Update modifies an existing preset.
func (r *PipelinePresetRepository) Update(ctx context.Context, id uuid.UUID, p *models.PipelinePreset) error {
	now := time.Now().UTC()
	params := p.Parameters
	if params == nil {
		params = json.RawMessage("{}")
	}

	tag, err := r.pool.Exec(ctx, `
		UPDATE pipeline_presets SET name=$1, description=$2, parameters=$3, updated_at=$4
		WHERE id=$5
	`, p.Name, p.Description, params, now, id)
	if err != nil {
		return fmt.Errorf("update preset: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("preset not found: %s", id)
	}
	return nil
}

// Delete removes a preset.
func (r *PipelinePresetRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM pipeline_presets WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete preset: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("preset not found: %s", id)
	}
	return nil
}

// IncrementUseCount bumps use_count and sets last_used_at.
func (r *PipelinePresetRepository) IncrementUseCount(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE pipeline_presets SET use_count = use_count + 1, last_used_at = $1, updated_at = $1
		WHERE id = $2
	`, now, id)
	if err != nil {
		return fmt.Errorf("increment preset use count: %w", err)
	}
	return nil
}

func scanPreset(rows pgx.Rows) (*models.PipelinePreset, error) {
	var p models.PipelinePreset
	var paramsJSON []byte
	var createdBy sql.NullString
	var lastUsed sql.NullTime

	err := rows.Scan(&p.ID, &p.TenantID, &p.SourceID, &p.Name, &p.Description,
		&paramsJSON, &createdBy, &p.UseCount, &lastUsed, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan preset: %w", err)
	}
	if paramsJSON != nil {
		p.Parameters = paramsJSON
	}
	if createdBy.Valid {
		uid := uuid.MustParse(createdBy.String)
		p.CreatedBy = &uid
	}
	if lastUsed.Valid {
		p.LastUsedAt = &lastUsed.Time
	}
	return &p, nil
}

// ─────────────────────────────────────────────────────────────
// PipelineRunRepository
// ─────────────────────────────────────────────────────────────

// PipelineRunRepository handles pipeline run persistence.
type PipelineRunRepository struct {
	pool *pgxpool.Pool
}

// NewPipelineRunRepository creates a new pipeline run repository.
func NewPipelineRunRepository(db *database.DB) *PipelineRunRepository {
	return &PipelineRunRepository{pool: db.Pool}
}

// List returns runs for a source with pagination.
func (r *PipelineRunRepository) List(ctx context.Context, sourceID uuid.UUID, page, perPage int) ([]models.PipelineRun, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	var total int64
	if err := r.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM pipeline_runs WHERE source_id = $1", sourceID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count pipeline runs: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, source_id, preset_id, COALESCE(external_run_id,''),
		       COALESCE(external_url,''), COALESCE(parameters,'{}'::jsonb), status,
		       COALESCE(external_status,''), started_at, completed_at, duration_ms,
		       COALESCE(logs,''), COALESCE(logs_url,''), job_details,
		       triggered_by, trigger_type, COALESCE(error_message,''),
		       created_at, updated_at
		FROM pipeline_runs
		WHERE source_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, sourceID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query pipeline runs: %w", err)
	}
	defer rows.Close()

	var runs []models.PipelineRun
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, 0, err
		}
		runs = append(runs, *run)
	}
	return runs, total, nil
}

// Get returns a single run by ID.
func (r *PipelineRunRepository) Get(ctx context.Context, id uuid.UUID) (*models.PipelineRun, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, source_id, preset_id, COALESCE(external_run_id,''),
		       COALESCE(external_url,''), COALESCE(parameters,'{}'::jsonb), status,
		       COALESCE(external_status,''), started_at, completed_at, duration_ms,
		       COALESCE(logs,''), COALESCE(logs_url,''), job_details,
		       triggered_by, trigger_type, COALESCE(error_message,''),
		       created_at, updated_at
		FROM pipeline_runs WHERE id = $1
	`, id)

	var run models.PipelineRun
	var paramsJSON, jobDetails []byte
	var presetID, extRunID, extURL, extStatus, logs, logsURL, triggeredBy, errMsg sql.NullString

	err := row.Scan(&run.ID, &run.TenantID, &run.SourceID, &presetID, &extRunID,
		&extURL, &paramsJSON, &run.Status, &extStatus, &run.StartedAt,
		&run.CompletedAt, &run.DurationMs, &logs, &logsURL, &jobDetails,
		&triggeredBy, &run.TriggerType, &errMsg, &run.CreatedAt, &run.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("pipeline run not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get pipeline run: %w", err)
	}

	if presetID.Valid {
		uid := uuid.MustParse(presetID.String)
		run.PresetID = &uid
	}
	if extRunID.Valid {
		run.ExternalRunID = extRunID.String
	}
	if extURL.Valid {
		run.ExternalURL = extURL.String
	}
	if paramsJSON != nil {
		run.Parameters = paramsJSON
	}
	if extStatus.Valid {
		run.ExternalStatus = extStatus.String
	}
	if logs.Valid {
		run.Logs = logs.String
	}
	if logsURL.Valid {
		run.LogsURL = logsURL.String
	}
	if jobDetails != nil {
		run.JobDetails = jobDetails
	}
	if triggeredBy.Valid {
		uid := uuid.MustParse(triggeredBy.String)
		run.TriggeredBy = &uid
	}
	if errMsg.Valid {
		run.ErrorMessage = errMsg.String
	}
	return &run, nil
}

// Create inserts a new pipeline run.
func (r *PipelineRunRepository) Create(ctx context.Context, run *models.PipelineRun) error {
	id := uuid.New()
	now := time.Now().UTC()
	run.ID = id
	run.CreatedAt = now
	run.UpdatedAt = now

	params := run.Parameters
	if params == nil {
		params = json.RawMessage("{}")
	}
	if run.Status == "" {
		run.Status = models.PipelineRunPending
	}
	if run.TriggerType == "" {
		run.TriggerType = "manual"
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO pipeline_runs (id, tenant_id, source_id, preset_id, external_run_id,
		                          external_url, parameters, status, external_status,
		                          started_at, completed_at, duration_ms, logs, logs_url,
		                          job_details, triggered_by, trigger_type, error_message,
		                          created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
	`, id, run.TenantID, run.SourceID, run.PresetID, run.ExternalRunID,
		run.ExternalURL, params, run.Status, run.ExternalStatus, run.StartedAt,
		run.CompletedAt, run.DurationMs, run.Logs, run.LogsURL, run.JobDetails,
		run.TriggeredBy, run.TriggerType, run.ErrorMessage, now, now)
	if err != nil {
		return fmt.Errorf("create pipeline run: %w", err)
	}
	return nil
}

// Update modifies an existing pipeline run.
func (r *PipelineRunRepository) Update(ctx context.Context, id uuid.UUID, run *models.PipelineRun) error {
	now := time.Now().UTC()

	tag, err := r.pool.Exec(ctx, `
		UPDATE pipeline_runs SET external_run_id=$1, external_url=$2, status=$3,
		                         external_status=$4, started_at=$5, completed_at=$6,
		                         duration_ms=$7, logs=$8, logs_url=$9,
		                         job_details=$10, error_message=$11, updated_at=$12
		WHERE id=$13
	`, run.ExternalRunID, run.ExternalURL, run.Status, run.ExternalStatus,
		run.StartedAt, run.CompletedAt, run.DurationMs, run.Logs, run.LogsURL,
		run.JobDetails, run.ErrorMessage, now, id)
	if err != nil {
		return fmt.Errorf("update pipeline run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("pipeline run not found: %s", id)
	}
	return nil
}

// UpdateStatus updates just the status and optional fields.
func (r *PipelineRunRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.PipelineRunStatus, extStatus string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE pipeline_runs SET status=$1, external_status=$2, updated_at=$3
		WHERE id=$4
	`, status, extStatus, now, id)
	if err != nil {
		return fmt.Errorf("update pipeline run status: %w", err)
	}
	return nil
}

// FindActive returns runs that are still pending or running for a source.
func (r *PipelineRunRepository) FindActive(ctx context.Context, sourceID uuid.UUID) ([]models.PipelineRun, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, source_id, preset_id, COALESCE(external_run_id,''),
		       COALESCE(external_url,''), COALESCE(parameters,'{}'::jsonb), status,
		       COALESCE(external_status,''), started_at, completed_at, duration_ms,
		       COALESCE(logs,''), COALESCE(logs_url,''), job_details,
		       triggered_by, trigger_type, COALESCE(error_message,''),
		       created_at, updated_at
		FROM pipeline_runs
		WHERE source_id = $1 AND status IN ('pending','running')
		ORDER BY created_at DESC
		LIMIT 50
	`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("query active runs: %w", err)
	}
	defer rows.Close()

	var runs []models.PipelineRun
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *run)
	}
	return runs, nil
}

// CreateJob inserts a run job record.
func (r *PipelineRunRepository) CreateJob(ctx context.Context, job *models.PipelineRunJob) error {
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	now := time.Now().UTC()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}

	steps := job.Steps
	if steps == nil {
		steps = json.RawMessage("[]")
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO pipeline_run_jobs (id, run_id, external_job_id, name, stage, status,
		                              started_at, completed_at, duration_ms, log_text,
		                              log_url, runner_name, allow_failure, steps, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, job.ID, job.RunID, job.ExternalJobID, job.Name, job.Stage, job.Status,
		job.StartedAt, job.CompletedAt, job.DurationMs, job.LogText, job.LogURL,
		job.RunnerName, job.AllowFailure, steps, job.CreatedAt)
	if err != nil {
		return fmt.Errorf("create run job: %w", err)
	}
	return nil
}

// UpsertJob inserts or updates a run job record, matching by run_id + external_job_id.
func (r *PipelineRunRepository) UpsertJob(ctx context.Context, job *models.PipelineRunJob) error {
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	now := time.Now().UTC()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}

	steps := job.Steps
	if steps == nil {
		steps = json.RawMessage("[]")
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO pipeline_run_jobs (id, run_id, external_job_id, name, stage, status,
		                              started_at, completed_at, duration_ms, log_text,
		                              log_url, runner_name, allow_failure, steps, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (run_id, external_job_id) WHERE external_job_id != ''
		DO UPDATE SET status=$6, started_at=$7, completed_at=$8, duration_ms=$9,
		              log_text=$10, log_url=$11, runner_name=$12, steps=$14
	`, job.ID, job.RunID, job.ExternalJobID, job.Name, job.Stage, job.Status,
		job.StartedAt, job.CompletedAt, job.DurationMs, job.LogText, job.LogURL,
		job.RunnerName, job.AllowFailure, steps, job.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert run job: %w", err)
	}
	return nil
}

// ListJobs returns jobs for a run.
func (r *PipelineRunRepository) ListJobs(ctx context.Context, runID uuid.UUID) ([]models.PipelineRunJob, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, run_id, COALESCE(external_job_id,''), name, COALESCE(stage,''),
		       status, started_at, completed_at, duration_ms, COALESCE(log_text,''),
		       COALESCE(log_url,''), COALESCE(runner_name,''), allow_failure,
		       COALESCE(steps,'[]'::jsonb), created_at
		FROM pipeline_run_jobs
		WHERE run_id = $1
		ORDER BY created_at ASC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("query run jobs: %w", err)
	}
	defer rows.Close()

	var jobs []models.PipelineRunJob
	for rows.Next() {
		var j models.PipelineRunJob
		var extJobID, stage, logText, logURL, runnerName sql.NullString

		err := rows.Scan(&j.ID, &j.RunID, &extJobID, &j.Name, &stage, &j.Status,
			&j.StartedAt, &j.CompletedAt, &j.DurationMs, &logText, &logURL,
			&runnerName, &j.AllowFailure, &j.Steps, &j.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan run job: %w", err)
		}
		if extJobID.Valid {
			j.ExternalJobID = extJobID.String
		}
		if stage.Valid {
			j.Stage = stage.String
		}
		if logText.Valid {
			j.LogText = logText.String
		}
		if logURL.Valid {
			j.LogURL = logURL.String
		}
		if runnerName.Valid {
			j.RunnerName = runnerName.String
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func scanRun(rows pgx.Rows) (*models.PipelineRun, error) {
	var run models.PipelineRun
	var paramsJSON, jobDetails []byte
	var presetID, extRunID, extURL, extStatus, logs, logsURL, triggeredBy, errMsg sql.NullString

	err := rows.Scan(&run.ID, &run.TenantID, &run.SourceID, &presetID, &extRunID,
		&extURL, &paramsJSON, &run.Status, &extStatus, &run.StartedAt,
		&run.CompletedAt, &run.DurationMs, &logs, &logsURL, &jobDetails,
		&triggeredBy, &run.TriggerType, &errMsg, &run.CreatedAt, &run.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan pipeline run: %w", err)
	}
	if presetID.Valid {
		uid := uuid.MustParse(presetID.String)
		run.PresetID = &uid
	}
	if extRunID.Valid {
		run.ExternalRunID = extRunID.String
	}
	if extURL.Valid {
		run.ExternalURL = extURL.String
	}
	if paramsJSON != nil {
		run.Parameters = paramsJSON
	}
	if extStatus.Valid {
		run.ExternalStatus = extStatus.String
	}
	if logs.Valid {
		run.Logs = logs.String
	}
	if logsURL.Valid {
		run.LogsURL = logsURL.String
	}
	if jobDetails != nil {
		run.JobDetails = jobDetails
	}
	if triggeredBy.Valid {
		uid := uuid.MustParse(triggeredBy.String)
		run.TriggeredBy = &uid
	}
	if errMsg.Valid {
		run.ErrorMessage = errMsg.String
	}
	return &run, nil
}

// UpsertByExternalRunID creates or updates a run matched by source_id + external_run_id.
// Uses a deterministic UUID v5 derived from (source_id, external_run_id) so the row ID
// is predictable without a separate lookup query.
func (r *PipelineRunRepository) UpsertByExternalRunID(ctx context.Context, run *models.PipelineRun) error {
	now := time.Now().UTC()

	params := run.Parameters
	if params == nil {
		params = json.RawMessage("{}")
	}
	if run.Status == "" {
		run.Status = models.PipelineRunPending
	}
	if run.TriggerType == "" {
		run.TriggerType = "sync"
	}

	// Deterministic UUID v5 — same (source_id, external_run_id) always maps to the same row ID.
	runID := uuid.NewSHA1(uuid.UUID(run.SourceID), []byte(run.ExternalRunID))

	_, err := r.pool.Exec(ctx, `
		INSERT INTO pipeline_runs (id, tenant_id, source_id, preset_id, external_run_id,
		                          external_url, parameters, status, external_status,
		                          started_at, completed_at, duration_ms, logs, logs_url,
		                          job_details, triggered_by, trigger_type, error_message,
		                          created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		ON CONFLICT (source_id, external_run_id) WHERE external_run_id != ''
		DO UPDATE SET status=$8, external_status=$9, external_url=$6,
		              duration_ms=$12, updated_at=$20
	`, runID, run.TenantID, run.SourceID, run.PresetID, run.ExternalRunID,
		run.ExternalURL, params, run.Status, run.ExternalStatus, run.StartedAt,
		run.CompletedAt, run.DurationMs, run.Logs, run.LogsURL, run.JobDetails,
		run.TriggeredBy, run.TriggerType, run.ErrorMessage, now, now)
	if err != nil {
		return fmt.Errorf("upsert pipeline run by external id: %w", err)
	}
	run.ID = runID
	return nil
}

// FindByExternalRunID looks up a run by source_id + external_run_id.
func (r *PipelineRunRepository) FindByExternalRunID(ctx context.Context, sourceID uuid.UUID, externalRunID string) (*models.PipelineRun, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, source_id, preset_id, COALESCE(external_run_id,''),
		       COALESCE(external_url,''), COALESCE(parameters,'{}'::jsonb), status,
		       COALESCE(external_status,''), started_at, completed_at, duration_ms,
		       COALESCE(logs,''), COALESCE(logs_url,''), job_details,
		       triggered_by, trigger_type, COALESCE(error_message,''),
		       created_at, updated_at
		FROM pipeline_runs
		WHERE source_id = $1 AND external_run_id = $2
	`, sourceID, externalRunID)

	var run models.PipelineRun
	var paramsJSON, jobDetails []byte
	var presetID, extRunID, extURL, extStatus, logs, logsURL, triggeredBy, errMsg sql.NullString

	err := row.Scan(&run.ID, &run.TenantID, &run.SourceID, &presetID, &extRunID,
		&extURL, &paramsJSON, &run.Status, &extStatus, &run.StartedAt,
		&run.CompletedAt, &run.DurationMs, &logs, &logsURL, &jobDetails,
		&triggeredBy, &run.TriggerType, &errMsg, &run.CreatedAt, &run.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find run by external id: %w", err)
	}
	if presetID.Valid {
		uid := uuid.MustParse(presetID.String)
		run.PresetID = &uid
	}
	if extRunID.Valid {
		run.ExternalRunID = extRunID.String
	}
	if extURL.Valid {
		run.ExternalURL = extURL.String
	}
	if paramsJSON != nil {
		run.Parameters = paramsJSON
	}
	if extStatus.Valid {
		run.ExternalStatus = extStatus.String
	}
	if logs.Valid {
		run.Logs = logs.String
	}
	if logsURL.Valid {
		run.LogsURL = logsURL.String
	}
	if jobDetails != nil {
		run.JobDetails = jobDetails
	}
	if triggeredBy.Valid {
		uid := uuid.MustParse(triggeredBy.String)
		run.TriggeredBy = &uid
	}
	if errMsg.Valid {
		run.ErrorMessage = errMsg.String
	}
	return &run, nil
}

// DeleteJobsByRunID removes all jobs for a run (used before re-syncing).
func (r *PipelineRunRepository) DeleteJobsByRunID(ctx context.Context, runID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM pipeline_run_jobs WHERE run_id = $1", runID)
	if err != nil {
		return fmt.Errorf("delete run jobs: %w", err)
	}
	return nil
}
