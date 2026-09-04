// Package pipeline defines the PipelineProvider interface and adapter registry
// for launching and tracking external pipelines (GitLab CI, GitHub Actions,
// Ansible, Terraform, Trivy).
package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	IsInput     bool     `json:"is_input,omitempty"` // true for GitLab CI spec.inputs
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
	// params may contain backend_* keys needed to initialize the backend before reading state.
	State(ctx context.Context, config json.RawMessage, params map[string]any) (*StateResult, error)
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

// ── InspectableProvider (optional capability) ──────────────────

// InspectableProvider is implemented by adapters that can introspect
// their source code to discover structure (playbooks, modules, roles).
type InspectableProvider interface {
	Provider

	// Inspect returns structured metadata about the pipeline source
	// (e.g. Ansible playbooks/roles, Terraform modules/resources).
	Inspect(ctx context.Context, config json.RawMessage) (json.RawMessage, error)
}

// ── WorkflowGraphProvider (optional capability) ────────────────

// WorkflowGraphProvider is implemented by adapters that can parse their
// workflow YAML to return a visual job dependency graph.
type WorkflowGraphProvider interface {
	Provider

	// GetWorkflowGraph returns the parsed job graph from the workflow YAML.
	GetWorkflowGraph(ctx context.Context, config json.RawMessage) (*WorkflowGraph, error)
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

// AnsibleInspection contains parsed playbook structure.
type AnsibleInspection struct {
	Playbooks   []AnsiblePlaybook `json:"playbooks"`
	Roles       []AnsibleRole     `json:"roles"`
	Inventories []string          `json:"inventories"`
}

// AnsiblePlaybook describes a single playbook file.
type AnsiblePlaybook struct {
	Name  string        `json:"name"`
	File  string        `json:"file"`
	Plays []AnsiblePlay `json:"plays"`
}

// AnsiblePlay describes a single play within a playbook.
type AnsiblePlay struct {
	Name  string   `json:"name"`
	Hosts string   `json:"hosts"`
	Roles []string `json:"roles"`
	Tasks int      `json:"task_count"`
	Tags  []string `json:"tags,omitempty"`
}

// AnsibleRole describes a discovered Ansible role.
type AnsibleRole struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path"`
	Tasks       int    `json:"task_count"`
}

// ── Terraform-specific inspection types ─────────────────────────

// TerraformInspection contains parsed module and resource structure.
type TerraformInspection struct {
	Modules     []TerraformModule     `json:"modules"`
	Resources   []TerraformResourceDef `json:"resources"`
	DataSources []TerraformResourceDef `json:"data_sources"`
	Outputs     []TerraformOutputDef   `json:"outputs"`
	Backend     string                 `json:"backend,omitempty"`
	Workspaces  []string               `json:"workspaces,omitempty"`
}

// TerraformModule describes a module block.
type TerraformModule struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Version string `json:"version,omitempty"`
}

// TerraformResourceDef describes a resource or data source block.
type TerraformResourceDef struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// TerraformOutputDef describes an output block.
type TerraformOutputDef struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Sensitive   bool   `json:"sensitive"`
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

// Stoppable is an optional interface that providers with background
// goroutines can implement to support graceful shutdown.
type Stoppable interface {
	Stop()
}

// Close stops all registered providers that implement the Stoppable interface.
// This ensures background goroutines are terminated during graceful shutdown.
func (r *Registry) Close() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[Stoppable]bool)
	for _, p := range r.providers {
		if s, ok := p.(Stoppable); ok && !seen[s] {
			s.Stop()
			seen[s] = true
		}
	}
}

// randomRunID generates a short unique identifier for tracking pipeline runs.
func randomRunID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// hostHomePrefixes are common absolute-path prefixes found on host machines.
// When PEPA runs inside Docker the host home directory is bind-mounted at
// /host-home so these paths become accessible after translation.
var hostHomePrefixes = []string{"/Users/", "/home/"}

// resolveContainerPath resolves a local_path entered in the UI to a path
// accessible inside the current process.  When PEPA runs natively the path
// is returned as-is.  When running inside Docker the host home directory is
// mounted read-only at /host-home, so a host path like
// /Users/alice/projects/ansible is translated to /host-home/projects/ansible.
func resolveContainerPath(p string) (string, error) {
	absPath, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("invalid local_path: %w", err)
	}

	// 1. Direct access (native mode or path already inside container).
	if info, statErr := os.Stat(absPath); statErr == nil && info.IsDir() {
		return absPath, nil
	}

	// 2. Docker mode — translate host home path → /host-home/...
	//    /Users/alice/projects/ansible → /host-home/projects/ansible
	//    /home/alice/projects/ansible  → /host-home/projects/ansible
	if filepath.IsAbs(p) {
		for _, prefix := range hostHomePrefixes {
			if strings.HasPrefix(p, prefix) {
				// Strip prefix + username segment: /Users/<user>/rest → rest
				rest := strings.TrimPrefix(p, prefix)
				if idx := strings.Index(rest, "/"); idx >= 0 {
					rest = rest[idx+1:]
				} else {
					continue // no sub-path after username
				}
				containerPath := filepath.Join("/host-home", rest)
				if info, statErr := os.Stat(containerPath); statErr == nil && info.IsDir() {
					return containerPath, nil
				}
			}
		}
	}

	return "", fmt.Errorf("local_path does not exist or is not a directory: %s", p)
}
