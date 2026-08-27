package ai

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// Manager manages LLM providers and AI tools
type Manager struct {
	mu              sync.RWMutex
	providers       map[string]LLMProvider
	defaultProvider string
	toolRegistry    *ToolRegistry
	agentPolicy     *AgentPolicy
}

// NewManager creates a new AI manager
func NewManager() *Manager {
	return &Manager{
		providers:    make(map[string]LLMProvider),
		toolRegistry: NewToolRegistry(),
		agentPolicy:  NewAgentPolicy(),
	}
}

// RegisterProvider adds an LLM provider
func (m *Manager) RegisterProvider(name string, provider LLMProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[name] = provider
	if m.defaultProvider == "" {
		m.defaultProvider = name
	}
}

// GetProvider returns a provider by name
func (m *Manager) GetProvider(name string) (LLMProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, name)
	}
	return p, nil
}

// DefaultProvider returns the default provider
func (m *Manager) DefaultProvider() (LLMProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.defaultProvider == "" {
		return nil, ErrProviderNotFound
	}
	return m.providers[m.defaultProvider], nil
}

// ListProviders returns all registered provider names
func (m *Manager) ListProviders() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	return names
}

// HealthCheck checks all providers
func (m *Manager) HealthCheck(ctx context.Context) map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	results := make(map[string]error)
	for name, p := range m.providers {
		results[name] = p.HealthCheck(ctx)
	}
	return results
}

// ToolRegistry returns the tool registry
func (m *Manager) ToolRegistry() *ToolRegistry {
	return m.toolRegistry
}

// ConfigureProvider dynamically registers (or replaces) an LLM provider.
func (m *Manager) ConfigureProvider(name, apiKey, baseURL, model string) error {
	if baseURL == "" {
		switch name {
		case "openai":
			baseURL = "https://api.openai.com/v1"
		case "anthropic":
			baseURL = "https://api.anthropic.com/v1"
		case "groq":
			baseURL = "https://api.groq.com/openai/v1"
		case "qoder":
			baseURL = "https://api.qoder.com/v1"
		case "lmstudio":
			baseURL = "http://host.docker.internal:1234/v1"
		default:
			return fmt.Errorf("base_url is required for provider %q", name)
		}
	}
	p := &httpProvider{
		name:    name,
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
	}
	m.RegisterProvider(name, p)
	return nil
}

// SetDefaultProvider sets the default provider name.
func (m *Manager) SetDefaultProvider(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.providers[name]; ok {
		m.defaultProvider = name
	}
}

// UnregisterProvider removes an LLM provider. If it was the default,
// another registered provider (if any) is promoted to default.
func (m *Manager) UnregisterProvider(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.providers, name)
	if m.defaultProvider == name {
		m.defaultProvider = ""
		for n := range m.providers {
			m.defaultProvider = n
			break
		}
	}
}

// AgentPolicy returns the agent policy.
func (m *Manager) AgentPolicy() *AgentPolicy {
	return m.agentPolicy
}

// CreateAgent creates a new Agent using the default provider.
// It auto-detects whether the model supports native function calling and
// selects the appropriate agent mode (native or prompt-based).
func (m *Manager) CreateAgent() (*Agent, error) {
	return m.CreateAgentForProvider("")
}

// providerSupportsFunctionCalling returns true if the provider is known to
// support native OpenAI-style function calling. Local models (Ollama, LM Studio)
// often don't support it reliably.
func providerSupportsFunctionCalling(name string) bool {
	switch name {
	case "openai", "anthropic", "groq", "qoder":
		return true
	default:
		return false
	}
}

// CreateAgentForProvider creates a new Agent using the specified provider name.
// If name is empty, the default provider is used.
func (m *Manager) CreateAgentForProvider(name string) (*Agent, error) {
	var p LLMProvider
	var err error
	if name != "" {
		p, err = m.GetProvider(name)
	} else {
		p, err = m.DefaultProvider()
	}
	if err != nil {
		return nil, err
	}

	// Determine agent mode based on known provider capabilities
	mode := AgentModeNative
	if !providerSupportsFunctionCalling(name) {
		mode = AgentModePrompt
		log.Printf("[AI Manager] Provider %q may not support function calling — using prompt-based agent mode", name)
	} else {
		log.Printf("[AI Manager] Provider %q supports function calling — using native agent mode", name)
	}

	return NewAgent(p, m.toolRegistry, m.agentPolicy, mode), nil
}

// CreateAgentWithMode creates a new Agent with an explicitly specified mode,
// overriding the auto-detection logic. An optional system instruction can be
// provided to override the default system prompt.
func (m *Manager) CreateAgentWithMode(mode AgentMode, systemInstruction ...string) (*Agent, error) {
	return m.CreateAgentWithProviderAndMode("", mode, systemInstruction...)
}

// CreateAgentWithProviderAndMode creates a new Agent using the specified provider
// and mode. If providerName is empty, the default provider is used.
func (m *Manager) CreateAgentWithProviderAndMode(providerName string, mode AgentMode, systemInstruction ...string) (*Agent, error) {
	if mode != AgentModeNative && mode != AgentModePrompt {
		return nil, fmt.Errorf("invalid agent mode: %q, must be %q or %q", mode, AgentModeNative, AgentModePrompt)
	}
	var p LLMProvider
	var err error
	if providerName != "" {
		p, err = m.GetProvider(providerName)
	} else {
		p, err = m.DefaultProvider()
	}
	if err != nil {
		return nil, err
	}
	agent := NewAgent(p, m.toolRegistry, m.agentPolicy, mode)
	if len(systemInstruction) > 0 && systemInstruction[0] != "" {
		agent.SetSystemInstruction(systemInstruction[0])
	}
	return agent, nil
}

// DefaultProviderName returns the name of the default provider.
func (m *Manager) DefaultProviderName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultProvider
}
