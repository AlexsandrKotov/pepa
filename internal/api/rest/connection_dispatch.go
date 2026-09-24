package rest

import (
	"github.com/pepa/pepa/internal/provider"
	"github.com/pepa/pepa/internal/repository"
)

// connectionDispatch describes how a connection type maps to a plugin, provider,
// and URL config key. It centralises the dispatch logic that was previously
// scattered across 6+ switch statements in connection_handlers.go.
type connectionDispatch struct {
	// plugin is the name of the plugin that handles this connection type.
	plugin string
	// provider is the provider name used for credential resolution.
	provider string
	// urlKey is the config key that holds the server/base URL (e.g. "url", "server_url").
	urlKey string
	// supportsBrowse indicates whether the connection supports browsing resources.
	supportsBrowse bool
	// supportsExecute indicates whether the connection supports executing actions.
	supportsExecute bool
}

// connectionDispatchTable maps each connection type to its dispatch metadata.
// Adding a new connection type only requires adding an entry here.
var connectionDispatchTable = map[repository.ConnectionType]connectionDispatch{
	repository.ConnectionGit: {
		plugin:         provider.PluginGitLab, // default; resolved dynamically from config["provider"]
		provider:       provider.PluginGitLab,
		urlKey:         "url",
		supportsBrowse: true,
	},
	repository.ConnectionGitLab: {
		plugin:         provider.PluginGitLab,
		provider:       provider.PluginGitLab,
		urlKey:         "url",
		supportsBrowse: true,
	},
	repository.ConnectionJira: {
		plugin:         provider.PluginJira,
		provider:       provider.PluginJira,
		urlKey:         "url",
		supportsBrowse: true,
	},
	repository.ConnectionJenkins: {
		plugin:          provider.PluginJenkins,
		provider:        provider.PluginJenkins,
		urlKey:          "url",
		supportsBrowse:  false,
		supportsExecute: true,
	},
	repository.ConnectionArgoCD: {
		plugin:          provider.PluginArgoCD,
		provider:        provider.PluginArgoCD,
		urlKey:          "server_url",
		supportsBrowse:  false,
		supportsExecute: true,
	},
	repository.ConnectionFluxCD: {
		plugin:          provider.PluginFluxCD,
		provider:        provider.PluginFluxCD,
		urlKey:          "",
		supportsExecute: true,
	},
	repository.ConnectionKubernetes: {
		plugin:         provider.PluginKubernetes,
		provider:       provider.PluginKubernetes,
		urlKey:         "",
		supportsBrowse: false,
	},
	repository.ConnectionProxmox: {
		plugin:         provider.PluginProxmox,
		provider:       provider.PluginProxmox,
		urlKey:         "url",
		supportsBrowse: false,
	},
	repository.ConnectionSonarQube: {
		plugin:         provider.PluginSonarQube,
		provider:       provider.PluginSonarQube,
		urlKey:         "url",
		supportsBrowse: false,
	},
}

// ResolvePluginName returns the plugin name for a connection, taking into
// account the config (e.g. git provider resolution).
func ResolvePluginName(conn repository.Connection) string {
	d, ok := connectionDispatchTable[conn.Type]
	if !ok {
		return ""
	}
	// Special case: git connections resolve the plugin from the provider config.
	if conn.Type == repository.ConnectionGit {
		prov, _ := conn.Config["provider"].(string)
		switch prov {
		case provider.PluginGitLab:
			return provider.PluginGitLab
		case provider.PluginGitHub:
			return provider.PluginGitHub
		case provider.PluginGitea:
			return provider.PluginGitea
		case provider.PluginBitbucket:
			return provider.PluginBitbucket
		default:
			return provider.PluginGitLab // default
		}
	}
	return d.plugin
}

// ProviderInfo returns the provider name and URL config key for credential
// resolution during connection testing. Returns ("", "") for types that do
// not support per-user credential override.
func ProviderInfo(connType repository.ConnectionType, config map[string]any) (string, string) {
	d, ok := connectionDispatchTable[connType]
	if !ok {
		return "", ""
	}
	prov := d.provider
	// Special case: git connections resolve the provider from config.
	if connType == repository.ConnectionGit {
		p, _ := config["provider"].(string)
		if p == "" {
			p = provider.PluginGitLab
		}
		prov = p
	}
	return prov, d.urlKey
}

// CredentialLookup returns the provider name and URL for credential lookup.
func CredentialLookup(conn repository.Connection) (string, string) {
	d, ok := connectionDispatchTable[conn.Type]
	if !ok {
		return "", ""
	}
	prov := d.provider
	// Special case: git connections resolve the provider from config.
	if conn.Type == repository.ConnectionGit {
		p, _ := conn.Config["provider"].(string)
		if p == "" {
			p = provider.PluginGitLab
		}
		prov = p
	}
	url, _ := conn.Config[d.urlKey].(string)
	return prov, url
}
