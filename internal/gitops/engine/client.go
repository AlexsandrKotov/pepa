// Package engine provides a unified interface for interacting with GitOps engines
// (ArgoCD and FluxCD). This abstraction allows the REST API and workflow engine
// to work with both engines through a single API.
package engine

import (
	"context"
	"encoding/json"
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
	TenantID     uuid.UUID
	Namespace    string
	AppName      string
}

// String returns a human-readable representation of the app reference.
func (r AppRef) String() string {
	return fmt.Sprintf("%s/%s/%s", r.ConnectionID, r.Namespace, r.AppName)
}

// AppSummary represents a high-level summary of a GitOps application.
type AppSummary struct {
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	EngineType   string            `json:"engine_type"` // "argocd" or "fluxcd"
	Health       string            `json:"health"`
	SyncStatus   string            `json:"sync_status"`
	Revision     string            `json:"revision"`
	Environment  string            `json:"environment,omitempty"`
	Project      string            `json:"project,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	ConnectionID string            `json:"connection_id,omitempty"`
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
	var allApps []AppSummary

	// List from ArgoCD if requested
	if opts.EngineType == "" || opts.EngineType == "argocd" {
		argoApps, err := c.listArgoApps(ctx, opts)
		if err != nil {
			// Log but continue — FluxCD may still work
			_ = err
		} else {
			allApps = append(allApps, argoApps...)
		}
	}

	// List from FluxCD if requested
	if opts.EngineType == "" || opts.EngineType == "fluxcd" {
		fluxApps, err := c.listFluxApps(ctx, opts)
		if err != nil {
			_ = err
		} else {
			allApps = append(allApps, fluxApps...)
		}
	}

	// Apply filters
	if opts.Health != "" || opts.SyncStatus != "" || opts.Environment != "" {
		var filtered []AppSummary
		for _, app := range allApps {
			if opts.Health != "" && app.Health != opts.Health {
				continue
			}
			if opts.SyncStatus != "" && app.SyncStatus != opts.SyncStatus {
				continue
			}
			if opts.Environment != "" && app.Environment != opts.Environment {
				continue
			}
			filtered = append(filtered, app)
		}
		allApps = filtered
	}

	// Apply pagination
	if opts.Limit > 0 {
		start := opts.Offset
		if start >= len(allApps) {
			return nil, nil
		}
		end := start + opts.Limit
		if end > len(allApps) {
			end = len(allApps)
		}
		allApps = allApps[start:end]
	}

	return allApps, nil
}

// listArgoApps lists applications from ArgoCD.
func (c *Client) listArgoApps(ctx context.Context, opts ListOptions) ([]AppSummary, error) {
	if c.registry == nil {
		return nil, fmt.Errorf("plugin registry not available")
	}

	// Resolve ArgoCD credentials
	creds, err := c.credResolver.ResolveArgo(ctx, gitops.ResolveOpts{
		TenantID: opts.TenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve argo credentials: %w", err)
	}

	// Build config for plugin
	config := map[string]string{
		"server_url": creds.ServerURL,
		"auth_token": creds.AuthToken,
		"kubeconfig": creds.Kubeconfig,
		"insecure":   fmt.Sprintf("%v", creds.Insecure),
	}

	// Call list_applications action
	resp, err := c.registry.ExecuteAction(ctx, "argocd", "list_applications", nil, config)
	if err != nil {
		return nil, fmt.Errorf("execute list_applications: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("list_applications failed: %s", resp.Error)
	}

	// Parse response
	var apps []provider.CDApplication
	if err := json.Unmarshal(resp.Output, &apps); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Convert to AppSummary
	summaries := make([]AppSummary, 0, len(apps))
	for _, app := range apps {
		summaries = append(summaries, AppSummary{
			Name:         app.Name,
			Namespace:    app.Namespace,
			EngineType:   "argocd",
			Health:       app.Health,
			SyncStatus:   app.SyncStatus,
			Revision:     app.Revision,
			ConnectionID: creds.ConnectionID.String(),
		})
	}
	return summaries, nil
}

// listFluxApps lists Kustomizations and HelmReleases from FluxCD.
func (c *Client) listFluxApps(ctx context.Context, opts ListOptions) ([]AppSummary, error) {
	if c.registry == nil {
		return nil, fmt.Errorf("plugin registry not available")
	}

	// Resolve FluxCD credentials
	creds, err := c.credResolver.ResolveFlux(ctx, gitops.ResolveOpts{
		TenantID: opts.TenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve flux credentials: %w", err)
	}

	config := map[string]string{
		"kubeconfig": creds.Kubeconfig,
	}

	var allApps []AppSummary

	// List Kustomizations
	resp, err := c.registry.ExecuteAction(ctx, "fluxcd", "list_kustomizations", nil, config)
	if err == nil && resp.Success {
		var items []map[string]interface{}
		if err := json.Unmarshal(resp.Output, &items); err == nil {
			for _, item := range items {
				allApps = append(allApps, AppSummary{
					Name:         getStringFromMap(item, "name"),
					Namespace:    getStringFromMap(item, "namespace"),
					EngineType:   "fluxcd",
					Health:       getStringFromMap(item, "health"),
					SyncStatus:   getStringFromMap(item, "sync_status"),
					Revision:     getStringFromMap(item, "revision"),
					ConnectionID: creds.ConnectionID.String(),
				})
			}
		}
	}

	// List HelmReleases
	resp, err = c.registry.ExecuteAction(ctx, "fluxcd", "list_helmreleases", nil, config)
	if err == nil && resp.Success {
		var items []map[string]interface{}
		if err := json.Unmarshal(resp.Output, &items); err == nil {
			for _, item := range items {
				allApps = append(allApps, AppSummary{
					Name:         getStringFromMap(item, "name"),
					Namespace:    getStringFromMap(item, "namespace"),
					EngineType:   "fluxcd",
					Health:       getStringFromMap(item, "health"),
					SyncStatus:   getStringFromMap(item, "sync_status"),
					Revision:     getStringFromMap(item, "revision"),
					ConnectionID: creds.ConnectionID.String(),
				})
			}
		}
	}

	return allApps, nil
}

// Get returns detailed information about a specific application.
func (c *Client) Get(ctx context.Context, ref AppRef) (*AppDetail, error) {
	// Determine engine type by trying ArgoCD first, then FluxCD
	detail, err := c.getArgoApp(ctx, ref)
	if err == nil {
		return detail, nil
	}

	detail, err = c.getFluxApp(ctx, ref)
	if err == nil {
		return detail, nil
	}

	return nil, fmt.Errorf("application %s not found in any engine", ref)
}

// getArgoApp gets an ArgoCD application.
func (c *Client) getArgoApp(ctx context.Context, ref AppRef) (*AppDetail, error) {
	creds, err := c.credResolver.ResolveArgo(ctx, gitops.ResolveOpts{
		ConnectionID: &ref.ConnectionID,
		TenantID:     ref.TenantID,
	})
	if err != nil {
		return nil, err
	}

	config := map[string]string{
		"server_url": creds.ServerURL,
		"auth_token": creds.AuthToken,
		"kubeconfig": creds.Kubeconfig,
		"insecure":   fmt.Sprintf("%v", creds.Insecure),
	}

	params := map[string]string{
		"name":      ref.AppName,
		"namespace": ref.Namespace,
	}

	resp, err := c.registry.ExecuteAction(ctx, "argocd", "get_application", mustMarshal(params), config)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("get_application failed: %s", resp.Error)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(resp.Output, &raw); err != nil {
		return nil, err
	}

	// Get capabilities
	capsResp, err := c.registry.ExecuteAction(ctx, "argocd", "capabilities", nil, config)
	var caps Capabilities
	if err == nil && capsResp.Success {
		var capsRaw map[string]interface{}
		if json.Unmarshal(capsResp.Output, &capsRaw) == nil {
			caps = parseCapabilities(capsRaw)
		}
	}

	return &AppDetail{
		AppSummary: AppSummary{
			Name:       ref.AppName,
			Namespace:  ref.Namespace,
			EngineType: "argocd",
			Health:     getStringFromMap(raw, "health"),
			SyncStatus: getStringFromMap(raw, "syncStatus"),
			Revision:   getStringFromMap(raw, "revision"),
		},
		Capabilities: caps,
	}, nil
}

// getFluxApp gets a FluxCD Kustomization or HelmRelease.
func (c *Client) getFluxApp(ctx context.Context, ref AppRef) (*AppDetail, error) {
	creds, err := c.credResolver.ResolveFlux(ctx, gitops.ResolveOpts{
		ConnectionID: &ref.ConnectionID,
		TenantID:     ref.TenantID,
	})
	if err != nil {
		return nil, err
	}

	config := map[string]string{
		"kubeconfig": creds.Kubeconfig,
	}

	// Try Kustomization first
	params := map[string]string{
		"name":      ref.AppName,
		"namespace": ref.Namespace,
	}

	resp, err := c.registry.ExecuteAction(ctx, "fluxcd", "get_kustomization", mustMarshal(params), config)
	if err == nil && resp.Success {
		var raw map[string]interface{}
		if err := json.Unmarshal(resp.Output, &raw); err == nil {
			caps := Capabilities{
				Diff: false, History: "partial", ResourceTree: true,
				Events: true, Logs: true, Refresh: true, AutoSync: true, Projects: false,
			}
			return &AppDetail{
				AppSummary: AppSummary{
					Name: ref.AppName, Namespace: ref.Namespace,
					EngineType: "fluxcd", Health: getStringFromMap(raw, "health"),
					SyncStatus: getStringFromMap(raw, "sync_status"), Revision: getStringFromMap(raw, "revision"),
				},
				Capabilities: caps,
			}, nil
		}
	}

	// Try HelmRelease
	resp, err = c.registry.ExecuteAction(ctx, "fluxcd", "get_helmrelease", mustMarshal(params), config)
	if err == nil && resp.Success {
		var raw map[string]interface{}
		if err := json.Unmarshal(resp.Output, &raw); err == nil {
			caps := Capabilities{
				Diff: false, History: "partial", ResourceTree: true,
				Events: true, Logs: true, Refresh: true, AutoSync: true, Projects: false,
			}
			return &AppDetail{
				AppSummary: AppSummary{
					Name: ref.AppName, Namespace: ref.Namespace,
					EngineType: "fluxcd", Health: getStringFromMap(raw, "health"),
					SyncStatus: getStringFromMap(raw, "sync_status"), Revision: getStringFromMap(raw, "revision"),
				},
				Capabilities: caps,
			}, nil
		}
	}

	return nil, fmt.Errorf("flux application %s/%s not found", ref.Namespace, ref.AppName)
}

// History returns the deployment history for an application.
func (c *Client) History(ctx context.Context, ref AppRef) ([]HistoryEntry, error) {
	// Try ArgoCD
	creds, err := c.credResolver.ResolveArgo(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{
			"server_url": creds.ServerURL, "auth_token": creds.AuthToken,
			"kubeconfig": creds.Kubeconfig, "insecure": fmt.Sprintf("%v", creds.Insecure),
		}
		params := map[string]string{"name": ref.AppName}
		resp, err := c.registry.ExecuteAction(ctx, "argocd", "history", mustMarshal(params), config)
		if err == nil && resp.Success {
			var entries []HistoryEntry
			if json.Unmarshal(resp.Output, &entries) == nil {
				return entries, nil
			}
		}
	}

	// Try FluxCD
	creds2, err := c.credResolver.ResolveFlux(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{"kubeconfig": creds2.Kubeconfig}
		params := map[string]interface{}{"name": ref.AppName, "namespace": ref.Namespace, "kind": "Kustomization"}
		resp, err := c.registry.ExecuteAction(ctx, "fluxcd", "history", mustMarshal(params), config)
		if err == nil && resp.Success {
			var entries []HistoryEntry
			if json.Unmarshal(resp.Output, &entries) == nil {
				return entries, nil
			}
		}
	}

	return nil, fmt.Errorf("history not available for %s", ref)
}

// ResourceTree returns the resource tree for an application.
func (c *Client) ResourceTree(ctx context.Context, ref AppRef) ([]ResourceNode, error) {
	// Try ArgoCD
	creds, err := c.credResolver.ResolveArgo(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{
			"server_url": creds.ServerURL, "auth_token": creds.AuthToken,
			"kubeconfig": creds.Kubeconfig, "insecure": fmt.Sprintf("%v", creds.Insecure),
		}
		params := map[string]string{"name": ref.AppName}
		resp, err := c.registry.ExecuteAction(ctx, "argocd", "resource_tree", mustMarshal(params), config)
		if err == nil && resp.Success {
			// ArgoCD returns {nodes: [...]} — extract nodes array
			var treeResp struct {
				Nodes []ResourceNode `json:"nodes"`
			}
			if json.Unmarshal(resp.Output, &treeResp) == nil && len(treeResp.Nodes) > 0 {
				return treeResp.Nodes, nil
			}
			// Fallback: try as direct array
			var nodes []ResourceNode
			if json.Unmarshal(resp.Output, &nodes) == nil && len(nodes) > 0 {
				return nodes, nil
			}
		}
	}

	// Try FluxCD
	creds2, err := c.credResolver.ResolveFlux(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{"kubeconfig": creds2.Kubeconfig}
		params := map[string]interface{}{"name": ref.AppName, "namespace": ref.Namespace, "kind": "Kustomization"}
		resp, err := c.registry.ExecuteAction(ctx, "fluxcd", "resource_tree", mustMarshal(params), config)
		if err == nil && resp.Success {
			// FluxCD returns []ResourceNode directly
			var nodes []ResourceNode
			if json.Unmarshal(resp.Output, &nodes) == nil && len(nodes) > 0 {
				return nodes, nil
			}
		}
	}

	return nil, fmt.Errorf("resource tree not available for %s", ref)
}

// Refresh triggers a refresh/reconcile for an application.
func (c *Client) Refresh(ctx context.Context, ref AppRef, hard bool) error {
	// Try ArgoCD first
	creds, err := c.credResolver.ResolveArgo(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{
			"server_url": creds.ServerURL, "auth_token": creds.AuthToken,
			"kubeconfig": creds.Kubeconfig, "insecure": fmt.Sprintf("%v", creds.Insecure),
		}
		// For ArgoCD REST API mode, trigger refresh via sync with refresh flag
		params := map[string]interface{}{
			"application": ref.AppName,
			"namespace":   ref.Namespace,
		}
		_, err := c.registry.ExecuteAction(ctx, "argocd", "sync", mustMarshal(params), config)
		if err == nil {
			return nil
		}
	}

	// Try FluxCD — trigger reconcile annotation
	creds2, err := c.credResolver.ResolveFlux(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{"kubeconfig": creds2.Kubeconfig}
		params := map[string]interface{}{
			"name":      ref.AppName,
			"namespace": ref.Namespace,
			"kind":      "Kustomization",
		}
		_, err := c.registry.ExecuteAction(ctx, "fluxcd", "reconcile_kustomization", mustMarshal(params), config)
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("refresh not available for %s", ref)
}

// Sync triggers a sync operation for an application.
func (c *Client) Sync(ctx context.Context, ref AppRef, opts SyncOptions) error {
	// Try ArgoCD first
	creds, err := c.credResolver.ResolveArgo(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{
			"server_url": creds.ServerURL, "auth_token": creds.AuthToken,
			"kubeconfig": creds.Kubeconfig, "insecure": fmt.Sprintf("%v", creds.Insecure),
		}
		params := map[string]interface{}{
			"application": ref.AppName,
			"prune":       opts.Prune,
			"dryRun":      opts.DryRun,
			"revision":    opts.Revision,
		}
		if opts.Revision != "" {
			params["revision"] = opts.Revision
		}
		_, err := c.registry.ExecuteAction(ctx, "argocd", "sync", mustMarshal(params), config)
		if err == nil {
			return nil
		}
		return fmt.Errorf("argocd sync failed: %w", err)
	}

	// Try FluxCD — trigger reconcile
	creds2, err := c.credResolver.ResolveFlux(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{"kubeconfig": creds2.Kubeconfig}
		params := map[string]interface{}{
			"name":      ref.AppName,
			"namespace": ref.Namespace,
			"kind":      "Kustomization",
		}
		_, err := c.registry.ExecuteAction(ctx, "fluxcd", "reconcile_kustomization", mustMarshal(params), config)
		if err == nil {
			return nil
		}
		return fmt.Errorf("fluxcd reconcile failed: %w", err)
	}

	return fmt.Errorf("sync not available for %s", ref)
}

// Rollback rolls back an application to a previous revision.
func (c *Client) Rollback(ctx context.Context, ref AppRef, historyID int64) error {
	// Try ArgoCD first — use the rollback action with revision from history
	creds, err := c.credResolver.ResolveArgo(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{
			"server_url": creds.ServerURL, "auth_token": creds.AuthToken,
			"kubeconfig": creds.Kubeconfig, "insecure": fmt.Sprintf("%v", creds.Insecure),
		}

		// Get the revision from history
		revision := fmt.Sprintf("%d", historyID)
		historyEntries, histErr := c.History(ctx, ref)
		if histErr == nil {
			for _, h := range historyEntries {
				if h.ID == historyID {
					revision = h.Revision
					break
				}
			}
		}

		params := map[string]interface{}{
			"application": ref.AppName,
			"revision":    revision,
		}
		_, err := c.registry.ExecuteAction(ctx, "argocd", "rollback", mustMarshal(params), config)
		if err == nil {
			return nil
		}
		return fmt.Errorf("argocd rollback failed: %w", err)
	}

	// FluxCD doesn't have a native rollback — use revision update
	return fmt.Errorf("rollback is only supported for ArgoCD applications")
}

// SetAutoSync enables or disables auto-sync for an application.
func (c *Client) SetAutoSync(ctx context.Context, ref AppRef, enabled bool, prune, selfHeal bool) error {
	// Try ArgoCD — set sync policy via CRD annotation or REST API
	creds, err := c.credResolver.ResolveArgo(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{
			"server_url": creds.ServerURL, "auth_token": creds.AuthToken,
			"kubeconfig": creds.Kubeconfig, "insecure": fmt.Sprintf("%v", creds.Insecure),
		}
		params := map[string]interface{}{
			"application": ref.AppName,
			"namespace":   ref.Namespace,
			"enabled":     enabled,
			"prune":       prune,
			"self_heal":   selfHeal,
		}
		// Use sync action with auto-sync configuration
		_, err := c.registry.ExecuteAction(ctx, "argocd", "sync", mustMarshal(params), config)
		if err == nil {
			return nil
		}
	}

	// Try FluxCD — use suspend/resume
	creds2, err := c.credResolver.ResolveFlux(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{"kubeconfig": creds2.Kubeconfig}
		action := "resume"
		if !enabled {
			action = "suspend"
		}
		params := map[string]interface{}{
			"name":      ref.AppName,
			"namespace": ref.Namespace,
			"kind":      "Kustomization",
		}
		_, err := c.registry.ExecuteAction(ctx, "fluxcd", action, mustMarshal(params), config)
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("auto-sync configuration not available for %s", ref)
}

// Terminate terminates a running operation (ArgoCD) or suspends reconciliation (FluxCD).
func (c *Client) Terminate(ctx context.Context, ref AppRef) error {
	// Try ArgoCD — terminate via sync with terminate flag
	creds, err := c.credResolver.ResolveArgo(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{
			"server_url": creds.ServerURL, "auth_token": creds.AuthToken,
			"kubeconfig": creds.Kubeconfig, "insecure": fmt.Sprintf("%v", creds.Insecure),
		}
		params := map[string]interface{}{
			"application": ref.AppName,
			"namespace":   ref.Namespace,
			"terminate":   true,
		}
		_, err := c.registry.ExecuteAction(ctx, "argocd", "sync", mustMarshal(params), config)
		if err == nil {
			return nil
		}
	}

	// Try FluxCD — suspend reconciliation
	creds2, err := c.credResolver.ResolveFlux(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{"kubeconfig": creds2.Kubeconfig}
		params := map[string]interface{}{
			"name":      ref.AppName,
			"namespace": ref.Namespace,
			"kind":      "Kustomization",
		}
		_, err := c.registry.ExecuteAction(ctx, "fluxcd", "suspend", mustMarshal(params), config)
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("terminate not available for %s", ref)
}

// Events returns Kubernetes events for an application.
func (c *Client) Events(ctx context.Context, ref AppRef) ([]map[string]interface{}, error) {
	// Try ArgoCD
	creds, err := c.credResolver.ResolveArgo(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{
			"server_url": creds.ServerURL, "auth_token": creds.AuthToken,
			"kubeconfig": creds.Kubeconfig, "insecure": fmt.Sprintf("%v", creds.Insecure),
		}
		params := map[string]string{"name": ref.AppName, "namespace": ref.Namespace}
		resp, err := c.registry.ExecuteAction(ctx, "argocd", "events", mustMarshal(params), config)
		if err == nil && resp.Success {
			var events []map[string]interface{}
			if json.Unmarshal(resp.Output, &events) == nil {
				return events, nil
			}
			// Try unwrapping from {items: [...]}
			var wrapper struct {
				Items []map[string]interface{} `json:"items"`
			}
			if json.Unmarshal(resp.Output, &wrapper) == nil {
				return wrapper.Items, nil
			}
		}
	}

	// Try FluxCD
	creds2, err := c.credResolver.ResolveFlux(ctx, gitops.ResolveOpts{ConnectionID: &ref.ConnectionID, TenantID: ref.TenantID})
	if err == nil {
		config := map[string]string{"kubeconfig": creds2.Kubeconfig}
		params := map[string]interface{}{
			"name":      ref.AppName,
			"namespace": ref.Namespace,
			"kind":      "Kustomization",
		}
		resp, err := c.registry.ExecuteAction(ctx, "fluxcd", "events", mustMarshal(params), config)
		if err == nil && resp.Success {
			var events []map[string]interface{}
			if json.Unmarshal(resp.Output, &events) == nil {
				return events, nil
			}
		}
	}

	return []map[string]interface{}{}, nil
}

// SyncOptions specifies options for a sync operation.
type SyncOptions struct {
	Revision  string
	Prune     bool
	DryRun    bool
	Force     bool
	Resources []string // subset of resources to sync
}

// ── Helpers ────────────────────────────────────────────────────

func mustMarshal(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}

func getStringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func parseCapabilities(raw map[string]interface{}) Capabilities {
	caps := Capabilities{}
	if v, ok := raw["diff"].(bool); ok {
		caps.Diff = v
	}
	if v, ok := raw["history"].(bool); ok && v {
		caps.History = "full"
	} else if v, ok := raw["history"].(string); ok {
		caps.History = v
	}
	if v, ok := raw["resource_tree"].(bool); ok {
		caps.ResourceTree = v
	}
	if v, ok := raw["events"].(bool); ok {
		caps.Events = v
	}
	if v, ok := raw["logs"].(bool); ok {
		caps.Logs = v
	}
	if v, ok := raw["refresh"].(bool); ok {
		caps.Refresh = v
	}
	if v, ok := raw["auto_sync"].(bool); ok {
		caps.AutoSync = v
	}
	if v, ok := raw["projects"].(bool); ok {
		caps.Projects = v
	}
	return caps
}
