// Package pipeline defines the PipelineProvider interface and adapter registry
// for launching and tracking external pipelines (GitLab CI, Ansible, Terraform).
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ParameterSchema is a JSON Schema describing the parameters a pipeline accepts.
type ParameterSchema struct {
	Type       string                 `json:"type"` // "object"
	Properties map[string]PropertyDef `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

// PropertyDef describes a single parameter.
type PropertyDef struct {
	Type        string   `json:"type"` // string, number, boolean, enum
	Description string   `json:"description,omitempty"`
	Default     any      `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// TriggerResult is returned after successfully triggering a pipeline.
type TriggerResult struct {
	ExternalRunID string `json:"external_run_id"`
	ExternalURL   string `json:"external_url"`
	Status        string `json:"status"`
}

// RunStatus is a snapshot of a pipeline run's current state.
type RunStatus struct {
	ExternalRunID string `json:"external_run_id"`
	Status        string `json:"status"` // pending, running, success, failed, cancelled
	ExternalURL   string `json:"external_url,omitempty"`
	DurationMs    *int   `json:"duration_ms,omitempty"`
	Logs          string `json:"logs,omitempty"`
	LogsURL       string `json:"logs_url,omitempty"`
}

// JobInfo describes a single job/step within a pipeline run.
type JobInfo struct {
	ExternalJobID string `json:"external_job_id"`
	Name          string `json:"name"`
	Stage         string `json:"stage,omitempty"`
	Status        string `json:"status"`
	LogText       string `json:"log_text,omitempty"`
	LogURL        string `json:"log_url,omitempty"`
	RunnerName    string `json:"runner_name,omitempty"`
	AllowFailure  bool   `json:"allow_failure"`
}

// Provider is the interface that all pipeline adapters must implement.
type Provider interface {
	// Name returns the adapter type name (e.g. "gitlab_ci").
	Name() string

	// ResolveSchema fetches the parameter schema for a pipeline source config.
	ResolveSchema(ctx context.Context, config json.RawMessage) (*ParameterSchema, error)

	// Trigger starts a pipeline with the given parameters and returns a trigger result.
	Trigger(ctx context.Context, config json.RawMessage, params map[string]any) (*TriggerResult, error)

	// Status returns the current status of a pipeline run.
	Status(ctx context.Context, config json.RawMessage, externalRunID string) (*RunStatus, error)

	// Jobs returns the jobs/steps of a pipeline run.
	Jobs(ctx context.Context, config json.RawMessage, externalRunID string) ([]JobInfo, error)

	// Logs retrieves the logs for a pipeline run (or a specific job).
	Logs(ctx context.Context, config json.RawMessage, externalRunID string, jobID string) (string, error)

	// Cancel attempts to cancel a running pipeline.
	Cancel(ctx context.Context, config json.RawMessage, externalRunID string) error
}

// ── Registry ─────────────────────────────────────────────────

// Registry maps source types to Provider implementations.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry creates an empty pipeline provider registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a provider for a given source type.
func (r *Registry) Register(sourceType string, p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[sourceType] = p
}

// Get returns the provider for a source type.
func (r *Registry) Get(sourceType string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[sourceType]
	if !ok {
		return nil, fmt.Errorf("no pipeline provider registered for type: %s", sourceType)
	}
	return p, nil
}

// List returns all registered source types.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]string, 0, len(r.providers))
	for t := range r.providers {
		types = append(types, t)
	}
	return types
}
