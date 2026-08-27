package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ============================================================
// Workflow — declarative pipeline definition
// ============================================================

type Workflow struct {
	ID        uuid.UUID       `json:"id" db:"id"`
	Name      string          `json:"name" db:"name"`
	TenantID  uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Spec      json.RawMessage `json:"spec" db:"spec"`
	Version   int             `json:"version" db:"version"`
	Source    string          `json:"source" db:"source"` // yaml, visual, template
	GitPath   string          `json:"git_path,omitempty" db:"git_path"`
	IsEnabled bool            `json:"is_enabled" db:"is_enabled"`
	IsLocked  bool            `json:"is_locked" db:"is_locked"`
	CreatedBy *uuid.UUID      `json:"created_by,omitempty" db:"created_by"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}

// WorkflowSpec — parsed workflow specification
type WorkflowSpec struct {
	Triggers []TriggerSpec  `json:"triggers" yaml:"triggers"`
	Settings SettingsSpec   `json:"settings" yaml:"settings"`
	Context  map[string]any `json:"context,omitempty" yaml:"context"`
	Steps    []StepSpec     `json:"steps" yaml:"steps"`
}

type TriggerSpec struct {
	Type   string          `json:"type" yaml:"type"` // webhook, schedule, entity_event, manual
	Config json.RawMessage `json:"config" yaml:"config"`
}

type SettingsSpec struct {
	Timeout     string     `json:"timeout,omitempty" yaml:"timeout"`
	Concurrency int        `json:"concurrency,omitempty" yaml:"concurrency"`
	OnConflict  string     `json:"on_conflict,omitempty" yaml:"onConflict"` // queue, reject, replace
	RetryPolicy *RetrySpec `json:"retry_policy,omitempty" yaml:"retryPolicy"`
}

type RetrySpec struct {
	MaxRetries      int     `json:"max_retries" yaml:"maxRetries"`
	Backoff         string  `json:"backoff" yaml:"backoff"` // constant, exponential, linear
	InitialInterval string  `json:"initial_interval" yaml:"initialInterval"`
	MaxInterval     string  `json:"max_interval" yaml:"maxInterval"`
	Multiplier      float64 `json:"multiplier,omitempty" yaml:"multiplier"`
}

type StepSpec struct {
	Name        string          `json:"name" yaml:"name"`
	Description string          `json:"description,omitempty" yaml:"description"`
	Plugin      string          `json:"plugin,omitempty" yaml:"plugin"` // e.g., "cd_engine:argocd"
	Action      string          `json:"action,omitempty" yaml:"action"`
	Type        string          `json:"type,omitempty" yaml:"type"` // condition, approval, entity_update
	Params      json.RawMessage `json:"params,omitempty" yaml:"params"`
	DependsOn   []string        `json:"depends_on,omitempty" yaml:"depends_on"`
	Condition   string          `json:"condition,omitempty" yaml:"condition"`
	SkipWhen    string          `json:"skip_when,omitempty" yaml:"skipWhen"`
	Timeout     string          `json:"timeout,omitempty" yaml:"timeout"`
	WaitFor     *WaitForSpec    `json:"wait_for,omitempty" yaml:"waitFor"`
	Retry       *RetrySpec      `json:"retry,omitempty" yaml:"retry"`
	Rollback    *RollbackSpec   `json:"rollback,omitempty" yaml:"rollback"`
	RunWhen     string          `json:"run_when,omitempty" yaml:"runWhen"` // always, on_failure
}

type WaitForSpec struct {
	Condition string `json:"condition,omitempty" yaml:"condition"`
	Timeout   string `json:"timeout,omitempty" yaml:"timeout"`
	Duration  string `json:"duration,omitempty" yaml:"duration"`
	Interval  string `json:"interval,omitempty" yaml:"interval"`
}

type RollbackSpec struct {
	Automatic bool      `json:"automatic" yaml:"automatic"`
	Condition string    `json:"condition,omitempty" yaml:"condition"`
	Action    *StepSpec `json:"action,omitempty" yaml:"action"`
}

// ── Execution ────────────────────────────────────────────────

type ExecutionStatus string

const (
	ExecutionPending    ExecutionStatus = "pending"
	ExecutionRunning    ExecutionStatus = "running"
	ExecutionWaiting    ExecutionStatus = "waiting"
	ExecutionSuccess    ExecutionStatus = "success"
	ExecutionFailed     ExecutionStatus = "failed"
	ExecutionCancelled  ExecutionStatus = "cancelled"
	ExecutionRolling    ExecutionStatus = "rolling_back"
	ExecutionRolledBack ExecutionStatus = "rolled_back"
)

type WorkflowExecution struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	WorkflowID     uuid.UUID       `json:"workflow_id" db:"workflow_id"`
	TenantID       uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	TriggerType    string          `json:"trigger_type" db:"trigger_type"`
	TriggerPayload json.RawMessage `json:"trigger_payload,omitempty" db:"trigger_payload"`
	TriggeredBy    *uuid.UUID      `json:"triggered_by,omitempty" db:"triggered_by"`
	Status         ExecutionStatus `json:"status" db:"status"`
	StartedAt      *time.Time      `json:"started_at,omitempty" db:"started_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	DurationMs     *int            `json:"duration_ms,omitempty" db:"duration_ms"`
	Context        json.RawMessage `json:"context,omitempty" db:"context"`
	Result         json.RawMessage `json:"result,omitempty" db:"result"`
	Error          string          `json:"error,omitempty" db:"error"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
}

type StepExecution struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	ExecutionID uuid.UUID       `json:"execution_id" db:"execution_id"`
	StepName    string          `json:"step_name" db:"step_name"`
	StepType    string          `json:"step_type" db:"step_type"`
	PluginName  string          `json:"plugin_name,omitempty" db:"plugin_name"`
	ActionName  string          `json:"action_name,omitempty" db:"action_name"`
	Params      json.RawMessage `json:"params,omitempty" db:"params"`
	Status      ExecutionStatus `json:"status" db:"status"`
	StartedAt   *time.Time      `json:"started_at,omitempty" db:"started_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	DurationMs  *int            `json:"duration_ms,omitempty" db:"duration_ms"`
	Output      json.RawMessage `json:"output,omitempty" db:"output"`
	Error       string          `json:"error,omitempty" db:"error"`
	RetryCount  int             `json:"retry_count" db:"retry_count"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
}

// ── Request types ────────────────────────────────────────────

type ExecuteWorkflowRequest struct {
	Parameters map[string]any `json:"parameters,omitempty"`
	DryRun     bool           `json:"dry_run,omitempty"`
}
