package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Tool — an action the AI can invoke
type Tool interface {
	Definition() ToolDefinition
	Execute(ctx context.Context, params json.RawMessage) (string, error)
}

// ToolRegistry manages available AI tools
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry
func (r *ToolRegistry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Definition().Name] = tool
}

// Get returns a tool by name
func (r *ToolRegistry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return t, nil
}

// List returns all tool definitions
func (r *ToolRegistry) List() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// Execute runs a tool by name
func (r *ToolRegistry) Execute(ctx context.Context, name string, params json.RawMessage) (string, error) {
	tool, err := r.Get(name)
	if err != nil {
		return "", err
	}
	return tool.Execute(ctx, params)
}
