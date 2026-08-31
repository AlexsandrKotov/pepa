package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PipelineSourceType defines supported pipeline source types.
type PipelineSourceType string

const (
	PipelineSourceGitLabCI      PipelineSourceType = "gitlab_ci"
	PipelineSourceGitLab        PipelineSourceType = "gitlab"
	PipelineSourceAnsible       PipelineSourceType = "ansible"
	PipelineSourceTerraform     PipelineSourceType = "terraform"
	PipelineSourceGitHubActions PipelineSourceType = "github_actions"
	PipelineSourceTrivy         PipelineSourceType = "trivy"
)

// PipelineSource represents a connection to an external pipeline definition.
type PipelineSource struct {
	ID              uuid.UUID       `json:"id" db:"id"`
	TenantID        uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Name            string          `json:"name" db:"name"`
	SourceType      string          `json:"source_type" db:"source_type"`
	Description     string          `json:"description" db:"description"`
	ConnectionID    *uuid.UUID      `json:"connection_id,omitempty" db:"connection_id"`
	Config          json.RawMessage `json:"config" db:"config"`
	ParameterSchema json.RawMessage `json:"parameter_schema,omitempty" db:"parameter_schema"`
	SchemaFetchedAt *time.Time      `json:"schema_fetched_at,omitempty" db:"schema_fetched_at"`
	Status          string          `json:"status" db:"status"`
	LastError       string          `json:"last_error,omitempty" db:"last_error"`
	CreatedBy       *uuid.UUID      `json:"created_by,omitempty" db:"created_by"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at" db:"updated_at"`
}

// PipelinePreset is a saved set of parameter values for quick re-use.
type PipelinePreset struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	TenantID    uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	SourceID    uuid.UUID       `json:"source_id" db:"source_id"`
	Name        string          `json:"name" db:"name"`
	Description string          `json:"description" db:"description"`
	Parameters  json.RawMessage `json:"parameters" db:"parameters"`
	CreatedBy   *uuid.UUID      `json:"created_by,omitempty" db:"created_by"`
	UseCount    int             `json:"use_count" db:"use_count"`
	LastUsedAt  *time.Time      `json:"last_used_at,omitempty" db:"last_used_at"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

// PipelineRunStatus defines pipeline run lifecycle states.
type PipelineRunStatus string

const (
	PipelineRunPending   PipelineRunStatus = "pending"
	PipelineRunRunning   PipelineRunStatus = "running"
	PipelineRunSuccess   PipelineRunStatus = "success"
	PipelineRunFailed    PipelineRunStatus = "failed"
	PipelineRunCancelled PipelineRunStatus = "cancelled"
	PipelineRunTimeout   PipelineRunStatus = "timeout"
	PipelineRunError     PipelineRunStatus = "error"
)

// PipelineRun records a single execution of a pipeline source.
type PipelineRun struct {
	ID             uuid.UUID         `json:"id" db:"id"`
	TenantID       uuid.UUID         `json:"tenant_id" db:"tenant_id"`
	SourceID       uuid.UUID         `json:"source_id" db:"source_id"`
	PresetID       *uuid.UUID        `json:"preset_id,omitempty" db:"preset_id"`
	ExternalRunID  string            `json:"external_run_id,omitempty" db:"external_run_id"`
	ExternalURL    string            `json:"external_url,omitempty" db:"external_url"`
	Parameters     json.RawMessage   `json:"parameters" db:"parameters"`
	Status         PipelineRunStatus `json:"status" db:"status"`
	ExternalStatus string            `json:"external_status,omitempty" db:"external_status"`
	StartedAt      *time.Time        `json:"started_at,omitempty" db:"started_at"`
	CompletedAt    *time.Time        `json:"completed_at,omitempty" db:"completed_at"`
	DurationMs     *int              `json:"duration_ms,omitempty" db:"duration_ms"`
	Logs           string            `json:"logs,omitempty" db:"logs"`
	LogsURL        string            `json:"logs_url,omitempty" db:"logs_url"`
	JobDetails     json.RawMessage   `json:"job_details,omitempty" db:"job_details"`
	TriggeredBy    *uuid.UUID        `json:"triggered_by,omitempty" db:"triggered_by"`
	TriggerType    string            `json:"trigger_type" db:"trigger_type"`
	ErrorMessage   string            `json:"error_message,omitempty" db:"error_message"`
	CreatedAt      time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at" db:"updated_at"`
}

// PipelineRunJob represents a single job/step within a pipeline run.
type PipelineRunJob struct {
	ID            uuid.UUID       `json:"id" db:"id"`
	RunID         uuid.UUID       `json:"run_id" db:"run_id"`
	ExternalJobID string          `json:"external_job_id,omitempty" db:"external_job_id"`
	Name          string          `json:"name" db:"name"`
	Stage         string          `json:"stage,omitempty" db:"stage"`
	Status        string          `json:"status" db:"status"`
	StartedAt     *time.Time      `json:"started_at,omitempty" db:"started_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	DurationMs    *int            `json:"duration_ms,omitempty" db:"duration_ms"`
	LogText       string          `json:"log_text,omitempty" db:"log_text"`
	LogURL        string          `json:"log_url,omitempty" db:"log_url"`
	RunnerName    string          `json:"runner_name,omitempty" db:"runner_name"`
	AllowFailure  bool            `json:"allow_failure" db:"allow_failure"`
	Steps         json.RawMessage `json:"steps,omitempty" db:"steps"`
	CreatedAt     time.Time       `json:"created_at" db:"created_at"`
}

// EngineStats holds aggregated run statistics for a pipeline source.
type EngineStats struct {
	SourceID      uuid.UUID  `json:"source_id"`
	TotalRuns     int64      `json:"total_runs"`
	SuccessCount  int64      `json:"success_count"`
	FailedCount   int64      `json:"failed_count"`
	RunningCount  int64      `json:"running_count"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	LastRunStatus string     `json:"last_run_status,omitempty"`
}

// ── Request types ────────────────────────────────────────────

// RunPipelineRequest is the payload for triggering a pipeline run.
type RunPipelineRequest struct {
	Parameters map[string]any `json:"parameters,omitempty"`
	PresetID   *uuid.UUID     `json:"preset_id,omitempty"`
}

// CreatePipelineSourceRequest is the payload for creating a pipeline source.
type CreatePipelineSourceRequest struct {
	Name         string          `json:"name" binding:"required"`
	SourceType   string          `json:"source_type" binding:"required"`
	Description  string          `json:"description"`
	ConnectionID *uuid.UUID      `json:"connection_id,omitempty"`
	Config       json.RawMessage `json:"config"`
}

// CreatePipelinePresetRequest is the payload for creating a preset.
type CreatePipelinePresetRequest struct {
	Name        string          `json:"name" binding:"required"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}
