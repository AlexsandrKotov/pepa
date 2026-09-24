package rest

import (
	"testing"

	"github.com/pepa/pepa/internal/repository"
)

// TestConnectionDispatchTableCoverage verifies that all connection types that
// require a plugin have an entry in the dispatch table.
func TestConnectionDispatchTableCoverage(t *testing.T) {
	// These types must have a dispatch entry because they map to plugins.
	required := []repository.ConnectionType{
		repository.ConnectionGit,
		repository.ConnectionGitLab,
		repository.ConnectionJira,
		repository.ConnectionJenkins,
		repository.ConnectionArgoCD,
		repository.ConnectionFluxCD,
		repository.ConnectionKubernetes,
		repository.ConnectionProxmox,
		repository.ConnectionSonarQube,
	}
	for _, ct := range required {
		if _, ok := connectionDispatchTable[ct]; !ok {
			t.Errorf("connection type %q missing from dispatch table", ct)
		}
	}
}

// TestResolvePluginName verifies plugin name resolution including git provider.
func TestResolvePluginName(t *testing.T) {
	tests := []struct {
		conn repository.Connection
		want string
	}{
		{repository.Connection{Type: repository.ConnectionGitLab}, "gitlab"},
		{repository.Connection{Type: repository.ConnectionJenkins}, "jenkins"},
		{repository.Connection{Type: repository.ConnectionJira}, "jira"},
		{repository.Connection{Type: repository.ConnectionGit, Config: map[string]any{"provider": "github"}}, "github"},
		{repository.Connection{Type: repository.ConnectionGit, Config: map[string]any{"provider": "gitea"}}, "gitea"},
		{repository.Connection{Type: repository.ConnectionGit, Config: map[string]any{"provider": "bitbucket"}}, "bitbucket"},
		{repository.Connection{Type: repository.ConnectionGit, Config: map[string]any{}}, "gitlab"}, // default
		{repository.Connection{Type: repository.ConnectionDocker}, ""},                              // no plugin
	}
	for _, tc := range tests {
		got := ResolvePluginName(tc.conn)
		if got != tc.want {
			t.Errorf("ResolvePluginName(%v) = %q, want %q", tc.conn.Type, got, tc.want)
		}
	}
}

// TestProviderInfo verifies provider name and URL key resolution.
func TestProviderInfo(t *testing.T) {
	provider, urlKey := ProviderInfo(repository.ConnectionArgoCD, nil)
	if provider != "argocd" || urlKey != "server_url" {
		t.Errorf("ProviderInfo(argocd) = (%q, %q), want (\"argocd\", \"server_url\")", provider, urlKey)
	}

	provider, urlKey = ProviderInfo(repository.ConnectionGit, map[string]any{"provider": "github"})
	if provider != "github" || urlKey != "url" {
		t.Errorf("ProviderInfo(git/github) = (%q, %q), want (\"github\", \"url\")", provider, urlKey)
	}

	provider, urlKey = ProviderInfo(repository.ConnectionDocker, nil)
	if provider != "" || urlKey != "" {
		t.Errorf("ProviderInfo(docker) = (%q, %q), want (\"\", \"\")", provider, urlKey)
	}
}

// TestCredentialLookup verifies credential lookup returns provider and URL.
func TestCredentialLookup(t *testing.T) {
	conn := repository.Connection{
		Type:   repository.ConnectionGitLab,
		Config: map[string]any{"url": "https://gitlab.example.com"},
	}
	provider, url := CredentialLookup(conn)
	if provider != "gitlab" || url != "https://gitlab.example.com" {
		t.Errorf("CredentialLookup(gitlab) = (%q, %q), want (\"gitlab\", \"https://gitlab.example.com\")", provider, url)
	}

	conn = repository.Connection{
		Type:   repository.ConnectionGit,
		Config: map[string]any{"provider": "github", "url": "https://github.com"},
	}
	provider, url = CredentialLookup(conn)
	if provider != "github" || url != "https://github.com" {
		t.Errorf("CredentialLookup(git/github) = (%q, %q), want (\"github\", \"https://github.com\")", provider, url)
	}
}
