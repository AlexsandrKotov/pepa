// Package pipeline defines the PipelineProvider interface and adapter registry
// for launching and tracking external pipelines (GitLab CI, GitHub Actions,
// Ansible, Terraform, Trivy).
package pipeline

import (
	"context"
	"crypto/rand"
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
	ExternalRunID string      `json:"external_run_id"`
	Status        string      `json:"status"` // pending, running, success, failed, cancelled
	ExternalURL   string      `json:"external_url,omitempty"`
	DurationMs    *int        `json:"duration_ms,omitempty"`
	Logs          string      `json:"logs,omitempty"`
	LogsURL       string      `json:"logs_url,omitempty"`
	Jobs          []JobInfo   `json:"jobs,omitempty"`
	HeadBranch    string      `json:"head_branch,omitempty"`
	Event         string      `json:"event,omitempty"`
	CreatedAt     string      `json:"created_at,omitempty"`
}

// JobInfo describes a single job/step within a pipeline run.
type JobInfo struct {
	ExternalJobID string      `json:"external_job_id"`
	Name          string      `json:"name"`
	Stage         string      `json:"stage,omitempty"`
	Status        string      `json:"status"`
	LogText       string      `json:"log_text,omitempty"`
	LogURL        string      `json:"log_url,omitempty"`
	RunnerName    string      `json:"runner_name,omitempty"`
	AllowFailure  bool        `json:"allow_failure"`
	Steps         []StepInfo  `json:"steps,omitempty"`
}

// StepInfo describes a single step within a job (GitHub Actions).
type StepInfo struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Number      int    `json:"number"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
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

// ── Enhanced Provider (backward-compatible optional interface) ────

// EnhancedProvider is an optional interface that adapters may implement
// to expose additional capabilities such as plan previews, state browsing,
// or structured output parsing.
type EnhancedProvider interface {
	Provider

	// Plan returns a preview of changes without applying them (Terraform plan, Ansible --check).
	Plan(ctx context.Context, config json.RawMessage, params map[string]any) (*PlanResult, error)

	// State returns the current state of managed resources.
	State(ctx context.Context, config json.RawMessage) (*StateResult, error)
}

// PlanResult represents a preview of infrastructure changes.
type PlanResult struct {
	HasChanges   bool   `json:"has_changes"`
	AddCount     int    `json:"add_count"`
	ChangeCount  int    `json:"change_count"`
	DestroyCount int    `json:"destroy_count"`
	OutputJSON   string `json:"output_json,omitempty"` // machine-readable plan (e.g. terraform show -json)
	OutputText   string `json:"output_text,omitempty"` // human-readable plan output
}

// StateResult represents the current state of managed resources.
type StateResult struct {
	Resources []StateResource `json:"resources"`
	RawJSON   string          `json:"raw_json,omitempty"`
}

// StateResource describes a single resource in the state.
type StateResource struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	ID       string `json:"id"`
	Provider string `json:"provider,omitempty"`
	Status   string `json:"status"`
}

// ── ListRunsProvider (optional capability) ─────────────────────

// ListRunsProvider is an optional interface that adapters may implement
// to support syncing historical runs from a remote CI system.
type ListRunsProvider interface {
	Provider

	// ListRemoteRuns fetches recent pipeline runs from the remote system.
	// Each RunStatus is augmented with Jobs populated where available.
	ListRemoteRuns(ctx context.Context, config json.RawMessage, perPage int) ([]RunStatus, error)
}

// ── Ansible-specific result types ───────────────────────────────

// AnsibleResult holds parsed output from an ansible-playbook run.
type AnsibleResult struct {
	Hosts    map[string]HostResult `json:"hosts"`
	Playbook string                `json:"playbook"`
	Duration string                `json:"duration,omitempty"`
}

// HostResult holds per-host task summary from an Ansible run.
type HostResult struct {
	OK      int `json:"ok"`
	Changed int `json:"changed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

// ── Trivy-specific result types ─────────────────────────────────

// TrivyScanResult holds the output of a Trivy vulnerability scan.
type TrivyScanResult struct {
	Target          string          `json:"target"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
	Summary         VulnSummary     `json:"summary"`
	ScanTime        string          `json:"scan_time,omitempty"`
}

// Vulnerability represents a single CVE finding.
type Vulnerability struct {
	ID          string `json:"id"`
	PkgName     string `json:"pkg_name"`
	Installed   string `json:"installed_version"`
	FixedIn     string `json:"fixed_version,omitempty"`
	Severity    string `json:"severity"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// VulnSummary holds vulnerability counts by severity.
type VulnSummary struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Unknown  int `json:"unknown"`
	Total    int `json:"total"`
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

// randomRunID generates a short unique identifier for tracking pipeline runs.
func randomRunID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
