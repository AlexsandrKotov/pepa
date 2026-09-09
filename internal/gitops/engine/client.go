// Package engine provides a unified interface for interacting with GitOps engines
// (ArgoCD and FluxCD). This abstraction allows the REST API and workflow engine
// to work with both engines through a single API.
package engine

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/gitops"
	"github.com/pepa/pepa/internal/provider"
)

// Client provides a unified interface for GitOps engine operations.
type Client struct {
	registry       *provider.Registry
	credResolver   *gitops.CredentialResolver
}

// NewClient creates a new engine client.
func NewClient(registry *provider.Registry, credResolver *gitops.CredentialResolver) *Client {
	return &Client{
		registry:     registry,
		credResolver: credResolver,
	}
}

// AppRef uniquely identifies a GitOps application.
type AppRef struct {
	ConnectionID uuid.UUID
	Namespace    string
	AppName      string
}

// String returns a human-readable representation of the app reference.
func (r AppRef) String() string {
	return fmt.Sprintf("%s/%s/%s", r.ConnectionID, r.Namespace, r.AppName)
}

// AppSummary represents a high-level summary of a GitOps application.
type AppSummary struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	EngineType  string            `json:"engine_type"` // "argocd" or "fluxcd"
	Health      string            `json:"health"`      // Healthy/Degraded/Progressing/Missing (Argo) or Ready/NotReady (Flux)
	SyncStatus  string            `json:"sync_status"` // Synced/OutOfSync/Unknown (Argo) or Reconciling/Suspended (Flux)
	Revision    string            `json:"revision"`    // Git commit SHA or revision
	Environment string            `json:"environment,omitempty"`
	Project     string            `json:"project,omitempty"` // ArgoCD project
	Labels      map[string]string `json:"labels,omitempty"`
}

// AppDetail represents detailed information about a GitOps application.
type AppDetail struct {
	AppSummary
	// ArgoCD-specific fields
	OperationState  *ArgoOperationState  `json:"operation_state,omitempty"`
	SyncPolicy      *ArgoSyncPolicy      `json:"sync_policy,omitempty"`
	Source          *ArgoSource          `json:"source,omitempty"`
	Destination     *ArgoDestination     `json:"destination,omitempty"`
	// FluxCD-specific fields
	Interval        string               `json:"interval,omitempty"`
	Suspend         bool                 `json:"suspend,omitempty"`
	Prune           bool                 `json:"prune,omitempty"`
	SourceRef       *FluxSourceRef       `json:"source_ref,omitempty"`
	Values          map[string]interface{} `json:"values,omitempty"`
	// Common fields
	Conditions      []Condition          `json:"conditions,omitempty"`
	Images          []string             `json:"images,omitempty"`
	Capabilities    Capabilities         `json:"capabilities"`
}

// Capabilities describes what operations are supported for an application.
type Capabilities struct {
	Diff         bool   `json:"diff"`          // Can show Git vs cluster diff
	History      string `json:"history"`       // "full", "partial", or "none"
	ResourceTree bool   `json:"resource_tree"` // Can show resource tree
	Events       bool   `json:"events"`        // Can show events
	Logs         bool   `json:"logs"`          // Can show pod logs
	Refresh      bool   `json:"refresh"`       // Can trigger refresh/reconcile
	AutoSync     bool   `json:"auto_sync"`     // Can enable/disable auto-sync
	Projects     bool   `json:"projects"`      // Can manage projects (ArgoCD only)
}

// Condition represents a status condition.
type Condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"last_transition_time,omitempty"`
}

// HistoryEntry represents a deployment history entry.
type HistoryEntry struct {
	ID         int64  `json:"id"`
	Revision   string `json:"revision"`
	DeployedAt string `json:"deployed_at"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
}

// ResourceNode represents a node in the resource tree.
type ResourceNode struct {
	Kind      string         `json:"kind"`
	Name      string         `json:"name"`
	Namespace string         `json:"namespace"`
	UID       string         `json:"uid"`
	Health    string         `json:"health,omitempty"`
	Status    string         `json:"status,omitempty"`
	Children  []ResourceNode `json:"children,omitempty"`
}

// ArgoOperationState represents ArgoCD operation state.
type ArgoOperationState struct {
	Phase     string `json:"phase"` // Running, Failed, Error, Terminating
	Message   string `json:"message,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
}

// ArgoSyncPolicy represents ArgoCD sync policy.
type ArgoSyncPolicy struct {
	Automated *ArgoSyncAutomated `json:"automated,omitempty"`
	SyncOptions []string         `json:"sync_options,omitempty"`
}

// ArgoSyncAutomated represents ArgoCD automated sync settings.
type ArgoSyncAutomated struct {
	Prune    bool `json:"prune"`
	SelfHeal bool `json:"self_heal"`
}

// ArgoSource represents ArgoCD application source.
type ArgoSource struct {
	RepoURL        string `json:"repo_url"`
	Path           string `json:"path,omitempty"`
	TargetRevision string `json:"target_revision"`
	Chart          string `json:"chart,omitempty"`
}

// ArgoDestination represents ArgoCD application destination.
type ArgoDestination struct {
	Server    string `json:"server,omitempty"`
	Namespace string `json:"namespace"`
	Name      string `json:"name,omitempty"`
}

// FluxSourceRef represents FluxCD source reference.
type FluxSourceRef struct {
	Kind      string `json:"kind"` // GitRepository, HelmRepository, Bucket
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// ListOptions specifies options for listing applications.
type ListOptions struct {
	TenantID     uuid.UUID
	ClusterID    *uuid.UUID
	Environment  string
	Project      string
	Health       string
	SyncStatus   string
	EngineType   string // "argocd", "fluxcd", or "" for all
	Limit        int
	Offset       int
}

// List returns a list of application summaries.
func (c *Client) List(ctx context.Context, opts ListOptions) ([]AppSummary, error) {
	// TODO: Implement in Stage 1.2
	// This will query both ArgoCD and FluxCD connections and aggregate results
	return nil, fmt.Errorf("not implemented")
}

// Get returns detailed information about a specific application.
func (c *Client) Get(ctx context.Context, ref AppRef) (*AppDetail, error) {
	// TODO: Implement in Stage 1.2
	return nil, fmt.Errorf("not implemented")
}

// History returns the deployment history for an application.
func (c *Client) History(ctx context.Context, ref AppRef) ([]HistoryEntry, error) {
	// TODO: Implement in Stage 1.2
	return nil, fmt.Errorf("not implemented")
}

// ResourceTree returns the resource tree for an application.
func (c *Client) ResourceTree(ctx context.Context, ref AppRef) (*ResourceNode, error) {
	// TODO: Implement in Stage 1.2
	return nil, fmt.Errorf("not implemented")
}

// Refresh triggers a refresh/reconcile for an application.
func (c *Client) Refresh(ctx context.Context, ref AppRef, hard bool) error {
	// TODO: Implement in Stage 2.1
	return fmt.Errorf("not implemented")
}

// Sync triggers a sync operation for an application (ArgoCD only).
func (c *Client) Sync(ctx context.Context, ref AppRef, opts SyncOptions) error {
	// TODO: Implement in Stage 2.1
	return fmt.Errorf("not implemented")
}

// Rollback rolls back an application to a previous revision.
func (c *Client) Rollback(ctx context.Context, ref AppRef, historyID int64) error {
	// TODO: Implement in Stage 2.1
	return fmt.Errorf("not implemented")
}

// SetAutoSync enables or disables auto-sync for an application.
func (c *Client) SetAutoSync(ctx context.Context, ref AppRef, enabled bool, prune, selfHeal bool) error {
	// TODO: Implement in Stage 2.1
	return fmt.Errorf("not implemented")
}

// Terminate terminates a running operation (ArgoCD) or suspends reconciliation (FluxCD).
func (c *Client) Terminate(ctx context.Context, ref AppRef) error {
	// TODO: Implement in Stage 2.1
	return fmt.Errorf("not implemented")
}

// SyncOptions specifies options for a sync operation.
type SyncOptions struct {
	Revision  string
	Prune     bool
	DryRun    bool
	Force     bool
	Resources []string // subset of resources to sync
}
