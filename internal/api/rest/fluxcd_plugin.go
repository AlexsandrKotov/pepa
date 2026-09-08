package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	pb "github.com/pepa/pepa/internal/plugin/proto"
	"github.com/pepa/pepa/internal/provider"
)

// fluxcdPluginAvailable checks if a FluxCD plugin is registered and enabled.
func fluxcdPluginAvailable(deps Dependencies) bool {
	if deps.ProviderRegistry == nil {
		return false
	}
	plugins := deps.ProviderRegistry.GetByType("cd_engine")
	for _, p := range plugins {
		if p.Name == "fluxcd" && p.Enabled {
			return true
		}
	}
	return false
}

// fluxcdPluginExecute executes an action via the FluxCD plugin if available.
// Returns (result, true, nil) if plugin handled it.
// Returns (nil, false, nil) if plugin not available (caller should fallback).
// Returns (nil, true, err) if plugin available but action failed.
func fluxcdPluginExecute(ctx context.Context, deps Dependencies, action string, params map[string]interface{}, kubeconfig string) (*pb.ExecuteResponse, bool, error) {
	if deps.ProviderRegistry == nil {
		return nil, false, nil
	}

	// Check if fluxcd plugin is available
	plugins := deps.ProviderRegistry.GetByType("cd_engine")
	var fluxPlugin *provider.PluginEntry
	for _, p := range plugins {
		if p.Name == "fluxcd" && p.Enabled {
			fluxPlugin = p
			break
		}
	}
	if fluxPlugin == nil {
		return nil, false, nil // No plugin, use fallback
	}

	// Build params JSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, true, fmt.Errorf("marshal params: %w", err)
	}

	// Build config with kubeconfig
	config := map[string]string{
		"kubeconfig": kubeconfig,
	}

	// Execute via plugin
	result, err := fluxPlugin.Executor.Execute(ctx, action, paramsJSON, "", config)
	if err != nil {
		return nil, true, fmt.Errorf("plugin execute %q: %w", action, err)
	}

	return result, true, nil
}

// fluxcdSuspendViaPlugin attempts to suspend via plugin. Returns true if handled.
func fluxcdSuspendViaPlugin(ctx context.Context, deps Dependencies, namespace, name, kubeconfig string) (bool, error) {
	params := map[string]interface{}{
		"resource":  "helmrelease",
		"name":      name,
		"namespace": namespace,
	}
	_, handled, err := fluxcdPluginExecute(ctx, deps, "suspend", params, kubeconfig)
	if err != nil {
		slog.Warn("fluxcd plugin suspend failed, will fallback", "error", err)
		return false, nil // Fallback to built-in
	}
	return handled, nil
}

// fluxcdResumeViaPlugin attempts to resume via plugin. Returns true if handled.
func fluxcdResumeViaPlugin(ctx context.Context, deps Dependencies, namespace, name, kubeconfig string) (bool, error) {
	params := map[string]interface{}{
		"resource":  "helmrelease",
		"name":      name,
		"namespace": namespace,
	}
	_, handled, err := fluxcdPluginExecute(ctx, deps, "resume", params, kubeconfig)
	if err != nil {
		slog.Warn("fluxcd plugin resume failed, will fallback", "error", err)
		return false, nil
	}
	return handled, nil
}

// fluxcdReconcileViaPlugin attempts to reconcile via plugin. Returns true if handled.
func fluxcdReconcileViaPlugin(ctx context.Context, deps Dependencies, namespace, name, kubeconfig string) (bool, error) {
	params := map[string]interface{}{
		"resource":  "helmrelease",
		"name":      name,
		"namespace": namespace,
	}
	_, handled, err := fluxcdPluginExecute(ctx, deps, "reconcile_helmrelease", params, kubeconfig)
	if err != nil {
		slog.Warn("fluxcd plugin reconcile failed, will fallback", "error", err)
		return false, nil
	}
	return handled, nil
}
