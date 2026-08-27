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

// WorkflowRepository handles workflow persistence.
type WorkflowRepository struct {
	pool *pgxpool.Pool
}

// NewWorkflowRepository creates a new workflow repository.
func NewWorkflowRepository(db *database.DB) *WorkflowRepository {
	return &WorkflowRepository{pool: db.Pool}
}

// List returns workflows with pagination.
func (r *WorkflowRepository) List(ctx context.Context, tenantID uuid.UUID, page, perPage int) ([]models.Workflow, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	var total int64
	err := r.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM workflows WHERE tenant_id = $1", tenantID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count workflows: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, name, tenant_id, spec, version, source, git_path,
		       is_enabled, is_locked, created_by, created_at, updated_at
		FROM workflows
		WHERE tenant_id = $1
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query workflows: %w", err)
	}
	defer rows.Close()

	workflows := make([]models.Workflow, 0)
	for rows.Next() {
		w, err := scanWorkflowRows(rows)
		if err != nil {
			return nil, 0, err
		}
		workflows = append(workflows, *w)
	}

	return workflows, total, nil
}

// Get returns a single workflow by ID.
func (r *WorkflowRepository) Get(ctx context.Context, id uuid.UUID) (*models.Workflow, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, name, tenant_id, spec, version, source, git_path,
		       is_enabled, is_locked, created_by, created_at, updated_at
		FROM workflows WHERE id = $1
	`, id)

	var w models.Workflow
	var specJSON []byte
	var source, gitPath sql.NullString

	err := row.Scan(&w.ID, &w.Name, &w.TenantID, &specJSON, &w.Version,
		&source, &gitPath, &w.IsEnabled, &w.IsLocked, &w.CreatedBy,
		&w.CreatedAt, &w.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}

	if specJSON != nil {
		w.Spec = specJSON
	}
	if source.Valid {
		w.Source = source.String
	}
	if gitPath.Valid {
		w.GitPath = gitPath.String
	}

	return &w, nil
}

// Create inserts a new workflow.
func (r *WorkflowRepository) Create(ctx context.Context, w *models.Workflow) error {
	id := uuid.New()
	now := time.Now().UTC()
	w.ID = id
	w.CreatedAt = now
	w.UpdatedAt = now

	spec := w.Spec
	if spec == nil {
		spec = json.RawMessage("{}")
	}
	if w.Version == 0 {
		w.Version = 1
	}
	if w.Source == "" {
		w.Source = "yaml"
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO workflows (id, name, tenant_id, spec, version, source, git_path,
		                       is_enabled, is_locked, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, id, w.Name, w.TenantID, spec, w.Version, w.Source, w.GitPath,
		w.IsEnabled, w.IsLocked, w.CreatedBy, now, now)
	if err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	return nil
}

// Update modifies an existing workflow.
func (r *WorkflowRepository) Update(ctx context.Context, id uuid.UUID, w *models.Workflow) error {
	now := time.Now().UTC()

	spec := w.Spec
	if spec == nil {
		spec = json.RawMessage("{}")
	}

	// Only update name and spec; preserve version, source, is_enabled, is_locked
	tag, err := r.pool.Exec(ctx, `
		UPDATE workflows SET name = $1, spec = $2, updated_at = $3
		WHERE id = $4
	`, w.Name, spec, now, id)
	if err != nil {
		return fmt.Errorf("update workflow: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("workflow not found: %s", id)
	}

	return nil
}

// Delete removes a workflow.
func (r *WorkflowRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// Delete step executions for all executions of this workflow
	if _, err := r.pool.Exec(ctx, `
		DELETE FROM step_executions
		WHERE execution_id IN (SELECT id FROM workflow_executions WHERE workflow_id = $1)
	`, id); err != nil {
		return fmt.Errorf("delete step executions: %w", err)
	}

	// Delete executions of this workflow
	if _, err := r.pool.Exec(ctx, "DELETE FROM workflow_executions WHERE workflow_id = $1", id); err != nil {
		return fmt.Errorf("delete workflow executions: %w", err)
	}

	tag, err := r.pool.Exec(ctx, "DELETE FROM workflows WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete workflow: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("workflow not found: %s", id)
	}
	return nil
}

// CreateExecution records a workflow execution.
func (r *WorkflowRepository) CreateExecution(ctx context.Context, exec *models.WorkflowExecution) error {
	id := uuid.New()
	now := time.Now().UTC()
	exec.ID = id
	exec.StartedAt = &now
	exec.Status = models.ExecutionPending
	exec.CreatedAt = now

	payload := exec.TriggerPayload
	if payload == nil {
		payload = json.RawMessage("{}")
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO workflow_executions (id, workflow_id, tenant_id, trigger_type,
		                                 trigger_payload, triggered_by, status,
		                                 started_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, exec.WorkflowID, exec.TenantID, exec.TriggerType, payload,
		exec.TriggeredBy, exec.Status, now, now)
	if err != nil {
		return fmt.Errorf("create execution: %w", err)
	}

	return nil
}

// GetExecution loads a single workflow execution by ID.
func (r *WorkflowRepository) GetExecution(ctx context.Context, id uuid.UUID) (*models.WorkflowExecution, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, workflow_id, tenant_id, trigger_type, trigger_payload,
		       triggered_by, status, started_at, completed_at, duration_ms,
		       context, result, error, created_at
		FROM workflow_executions
		WHERE id = $1
	`, id)

	var exec models.WorkflowExecution
	var payloadJSON, ctxJSON, resultJSON []byte
	var errMsg sql.NullString

	err := row.Scan(&exec.ID, &exec.WorkflowID, &exec.TenantID, &exec.TriggerType,
		&payloadJSON, &exec.TriggeredBy, &exec.Status, &exec.StartedAt,
		&exec.CompletedAt, &exec.DurationMs, &ctxJSON, &resultJSON, &errMsg, &exec.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get execution: %w", err)
	}
	if payloadJSON != nil {
		exec.TriggerPayload = payloadJSON
	}
	if ctxJSON != nil {
		exec.Context = ctxJSON
	}
	if resultJSON != nil {
		exec.Result = resultJSON
	}
	if errMsg.Valid {
		exec.Error = errMsg.String
	}
	return &exec, nil
}

// ListExecutions returns executions for a workflow.
func (r *WorkflowRepository) ListExecutions(ctx context.Context, workflowID uuid.UUID) ([]models.WorkflowExecution, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, workflow_id, tenant_id, trigger_type, trigger_payload,
		       triggered_by, status, started_at, completed_at, duration_ms,
		       context, result, error, created_at
		FROM workflow_executions
		WHERE workflow_id = $1
		ORDER BY started_at DESC
		LIMIT 50
	`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("query executions: %w", err)
	}
	defer rows.Close()

	execs := make([]models.WorkflowExecution, 0)
	for rows.Next() {
		var exec models.WorkflowExecution
		var payloadJSON, ctxJSON, resultJSON []byte
		var errMsg sql.NullString

		err := rows.Scan(&exec.ID, &exec.WorkflowID, &exec.TenantID, &exec.TriggerType,
			&payloadJSON, &exec.TriggeredBy, &exec.Status, &exec.StartedAt,
			&exec.CompletedAt, &exec.DurationMs, &ctxJSON, &resultJSON, &errMsg, &exec.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		if payloadJSON != nil {
			exec.TriggerPayload = payloadJSON
		}
		if ctxJSON != nil {
			exec.Context = ctxJSON
		}
		if resultJSON != nil {
			exec.Result = resultJSON
		}
		if errMsg.Valid {
			exec.Error = errMsg.String
		}
		execs = append(execs, exec)
	}

	return execs, nil
}

// UpdateExecutionStatus updates the status of a workflow execution.
func (r *WorkflowRepository) UpdateExecutionStatus(ctx context.Context, id uuid.UUID, status models.ExecutionStatus, durationMs *int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE workflow_executions SET status = $1, duration_ms = $2
		WHERE id = $3
	`, status, durationMs, id)
	if err != nil {
		return fmt.Errorf("update execution status: %w", err)
	}
	return nil
}

// UpdateExecutionError updates execution with error message.
func (r *WorkflowRepository) UpdateExecutionError(ctx context.Context, id uuid.UUID, errMsg string, durationMs *int) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE workflow_executions SET status = $1, error = $2, duration_ms = $3,
		                               completed_at = $4
		WHERE id = $5
	`, models.ExecutionFailed, errMsg, durationMs, now, id)
	if err != nil {
		return fmt.Errorf("update execution error: %w", err)
	}
	return nil
}

// UpdateExecutionResult updates execution with result and completion.
func (r *WorkflowRepository) UpdateExecutionResult(ctx context.Context, id uuid.UUID, status models.ExecutionStatus, result json.RawMessage, durationMs *int) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE workflow_executions SET status = $1, result = $2, duration_ms = $3,
		                               completed_at = $4
		WHERE id = $5
	`, status, result, durationMs, now, id)
	if err != nil {
		return fmt.Errorf("update execution result: %w", err)
	}
	return nil
}

// CreateStepExecution records a step execution.
func (r *WorkflowRepository) CreateStepExecution(ctx context.Context, se *models.StepExecution) error {
	now := time.Now().UTC()
	if se.ID == uuid.Nil {
		se.ID = uuid.New()
	}
	if se.CreatedAt.IsZero() {
		se.CreatedAt = now
	}

	params := se.Params
	if params == nil {
		params = json.RawMessage("{}")
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO step_executions (id, execution_id, step_name, step_type, plugin_name,
		                              action_name, params, status, started_at, completed_at,
		                              duration_ms, output, error, retry_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, se.ID, se.ExecutionID, se.StepName, se.StepType, se.PluginName,
		se.ActionName, params, se.Status, se.StartedAt, se.CompletedAt,
		se.DurationMs, se.Output, se.Error, se.RetryCount, se.CreatedAt)
	if err != nil {
		return fmt.Errorf("create step execution: %w", err)
	}
	return nil
}

// ListStepExecutions returns step executions for a workflow execution.
func (r *WorkflowRepository) ListStepExecutions(ctx context.Context, executionID uuid.UUID) ([]models.StepExecution, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, execution_id, step_name, step_type, plugin_name, action_name,
		       params, status, started_at, completed_at, duration_ms, output,
		       error, retry_count, created_at
		FROM step_executions
		WHERE execution_id = $1
		ORDER BY created_at ASC
	`, executionID)
	if err != nil {
		return nil, fmt.Errorf("query step executions: %w", err)
	}
	defer rows.Close()

	steps := make([]models.StepExecution, 0)
	for rows.Next() {
		var se models.StepExecution
		var paramsJSON, outputJSON []byte
		var errMsg, pluginName, actionName sql.NullString

		err := rows.Scan(&se.ID, &se.ExecutionID, &se.StepName, &se.StepType,
			&pluginName, &actionName, &paramsJSON, &se.Status, &se.StartedAt,
			&se.CompletedAt, &se.DurationMs, &outputJSON, &errMsg, &se.RetryCount,
			&se.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan step execution: %w", err)
		}
		if pluginName.Valid {
			se.PluginName = pluginName.String
		}
		if actionName.Valid {
			se.ActionName = actionName.String
		}
		if paramsJSON != nil {
			se.Params = paramsJSON
		}
		if outputJSON != nil {
			se.Output = outputJSON
		}
		if errMsg.Valid {
			se.Error = errMsg.String
		}
		steps = append(steps, se)
	}

	return steps, nil
}

// --- scan helpers ---

func scanWorkflowRows(rows pgx.Rows) (*models.Workflow, error) {
	var w models.Workflow
	var specJSON []byte
	var source, gitPath sql.NullString

	err := rows.Scan(&w.ID, &w.Name, &w.TenantID, &specJSON, &w.Version,
		&source, &gitPath, &w.IsEnabled, &w.IsLocked, &w.CreatedBy,
		&w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan workflow: %w", err)
	}

	if specJSON != nil {
		w.Spec = specJSON
	}
	if source.Valid {
		w.Source = source.String
	}
	if gitPath.Valid {
		w.GitPath = gitPath.String
	}

	return &w, nil
}
