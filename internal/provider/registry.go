package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	pb "github.com/pepa/pepa/internal/plugin/proto"
)

// Executor is the interface the registry uses to dispatch actions to plugins.
// This is satisfied by the engine.Manager's gRPC client.
type Executor interface {
	Execute(ctx context.Context, action string, params []byte, tenantID string, config map[string]string) (*pb.ExecuteResponse, error)
	HealthCheck(ctx context.Context) (*pb.HealthCheckResponse, error)
	Info(ctx context.Context) (*pb.InfoResponse, error)
}

// PluginEntry represents a registered plugin in the provider registry.
type PluginEntry struct {
	Name     string
	Type     string // git_provider, task_tracker, cd_engine, etc.
	Info     *pb.InfoResponse
	Executor Executor
	Enabled  bool
	// ConnectionID optionally links this provider to a specific connection
	ConnectionID string
}

// Registry maps plugin names to running instances and supports lookup by type.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]*PluginEntry
}

// NewRegistry creates a new provider registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]*PluginEntry),
	}
}

// Register adds a plugin to the registry.
func (r *Registry) Register(entry *PluginEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[entry.Name] = entry
}

// Unregister removes a plugin from the registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.plugins, name)
}

// Get returns a plugin entry by name.
func (r *Registry) Get(name string) (*PluginEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.plugins[name]
	return entry, ok
}

// GetEnabled returns a plugin entry only if it is registered AND enabled.
func (r *Registry) GetEnabled(name string) (*PluginEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.plugins[name]
	if !ok || !entry.Enabled {
		return nil, false
	}
	return entry, true
}

// GetByType returns all plugins of a given type.
func (r *Registry) GetByType(pluginType string) []*PluginEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*PluginEntry
	for _, entry := range r.plugins {
		if entry.Type == pluginType && entry.Enabled {
			result = append(result, entry)
		}
	}
	return result
}

// List returns all registered plugins.
func (r *Registry) List() []*PluginEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*PluginEntry, 0, len(r.plugins))
	for _, entry := range r.plugins {
		result = append(result, entry)
	}
	return result
}

// ExecuteAction dispatches an action to a named plugin.
func (r *Registry) ExecuteAction(ctx context.Context, name string, action string, params []byte, config map[string]string) (*pb.ExecuteResponse, error) {
	r.mu.RLock()
	entry, ok := r.plugins[name]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("plugin %q not registered", name)
	}
	if !entry.Enabled {
		return nil, fmt.Errorf("plugin %q is disabled", name)
	}

	return entry.Executor.Execute(ctx, action, params, "", config)
}

// ExecuteActionByType finds the first enabled plugin of the given type and executes the action.
func (r *Registry) ExecuteActionByType(ctx context.Context, pluginType string, action string, params []byte, config map[string]string) (*pb.ExecuteResponse, error) {
	plugins := r.GetByType(pluginType)
	if len(plugins) == 0 {
		return nil, fmt.Errorf("no enabled plugin of type %q", pluginType)
	}

	// Use the first available plugin
	entry := plugins[0]
	return entry.Executor.Execute(ctx, action, params, "", config)
}

// HealthCheck checks the health of a named plugin.
func (r *Registry) HealthCheck(ctx context.Context, name string) (*pb.HealthCheckResponse, error) {
	r.mu.RLock()
	entry, ok := r.plugins[name]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("plugin %q not registered", name)
	}

	return entry.Executor.HealthCheck(ctx)
}

// SetEnabled enables or disables a plugin.
func (r *Registry) SetEnabled(name string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not registered", name)
	}
	entry.Enabled = enabled
	return nil
}

// Summary returns a JSON-serializable summary of all registered plugins.
func (r *Registry) Summary() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]map[string]interface{}, 0, len(r.plugins))
	for _, entry := range r.plugins {
		actions := make([]string, 0)
		if entry.Info != nil {
			for _, a := range entry.Info.Actions {
				actions = append(actions, a.Name)
			}
		}
		result = append(result, map[string]interface{}{
			"name":          entry.Name,
			"type":          entry.Type,
			"enabled":       entry.Enabled,
			"actions":       actions,
			"connection_id": entry.ConnectionID,
		})
	}
	return result
}

// ExecuteActionJSON is a convenience method that executes an action and unmarshals the output.
func (r *Registry) ExecuteActionJSON(ctx context.Context, name string, action string, params interface{}, output interface{}, config map[string]string) error {
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	resp, err := r.ExecuteAction(ctx, name, action, paramsBytes, config)
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("plugin action failed: %s", resp.Error)
	}

	if output != nil && len(resp.Output) > 0 {
		if err := json.Unmarshal(resp.Output, output); err != nil {
			return fmt.Errorf("failed to unmarshal output: %w", err)
		}
	}
	return nil
}
