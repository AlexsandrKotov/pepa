package ai

// ProviderPreset describes the default configuration for a known AI provider.
type ProviderPreset struct {
	// Name is the canonical provider identifier (e.g. "openai", "anthropic").
	Name string
	// DefaultBaseURL is used when the user does not supply a custom base_url.
	DefaultBaseURL string
	// DefaultModel is the fallback model when none is configured.
	DefaultModel string
	// RequiresKey indicates whether the provider needs an API key.
	RequiresKey bool
}

// ProviderPresets is the single source of truth for known AI provider defaults.
// Adding a new provider only requires appending an entry here.
var ProviderPresets = []ProviderPreset{
	{Name: "openai", DefaultBaseURL: "https://api.openai.com/v1", DefaultModel: "gpt-4o-mini", RequiresKey: true},
	{Name: "anthropic", DefaultBaseURL: "https://api.anthropic.com/v1", DefaultModel: "claude-3-haiku-20240307", RequiresKey: true},
	{Name: "groq", DefaultBaseURL: "https://api.groq.com/openai/v1", DefaultModel: "openai/gpt-oss-120b", RequiresKey: true},
	{Name: "qoder", DefaultBaseURL: "https://api.qoder.com/v1", DefaultModel: "qoder-coder", RequiresKey: true},
	{Name: "lmstudio", DefaultBaseURL: "http://host.docker.internal:1234/v1", DefaultModel: "local-model", RequiresKey: false},
	{Name: "ollama", DefaultBaseURL: "", DefaultModel: "llama3", RequiresKey: false},
}

// LookupProvider returns the preset for a named provider, or false if unknown.
func LookupProvider(name string) (ProviderPreset, bool) {
	for _, p := range ProviderPresets {
		if p.Name == name {
			return p, true
		}
	}
	return ProviderPreset{}, false
}

// DefaultBaseURLFor returns the default base URL for a provider, or "" if unknown.
func DefaultBaseURLFor(name string) string {
	if p, ok := LookupProvider(name); ok {
		return p.DefaultBaseURL
	}
	return ""
}

// DefaultModelFor returns the default model for a provider, or "default" if unknown.
func DefaultModelFor(name string) string {
	if p, ok := LookupProvider(name); ok {
		return p.DefaultModel
	}
	return "default"
}

// ProviderRequiresKey returns true if the provider needs an API key.
func ProviderRequiresKey(name string) bool {
	if p, ok := LookupProvider(name); ok {
		return p.RequiresKey
	}
	return true // safe default: require key for unknown providers
}
