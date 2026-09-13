package gitops

import (
	"testing"
)

func TestMaskGitSecrets_EmptyToken(t *testing.T) {
	output := "fatal: could not read password"
	got := maskGitSecrets(output, "")
	if got != output {
		t.Errorf("empty token should return output unchanged, got %q", got)
	}
}

func TestMaskGitSecrets_ReplacesToken(t *testing.T) {
	token := "ghp_abc123secret"
	output := "fatal: Authentication failed for 'https://oauth2:" + token + "@github.com/org/repo.git/'"
	got := maskGitSecrets(output, token)
	if containsStr(got, token) {
		t.Errorf("token should be masked, but found in output: %s", got)
	}
	if !containsStr(got, "***") {
		t.Errorf("expected *** replacement, got: %s", got)
	}
}

func TestMaskGitSecrets_ReplacesMultipleOccurrences(t *testing.T) {
	token := "secret-token-xyz"
	output := "url: " + token + " and again " + token
	got := maskGitSecrets(output, token)
	if containsStr(got, token) {
		t.Errorf("all occurrences should be masked, got: %s", got)
	}
}

func TestMaskGitSecrets_Oauth2Prefix(t *testing.T) {
	token := "mytoken123"
	output := "oauth2:" + token
	got := maskGitSecrets(output, token)
	if containsStr(got, token) {
		t.Errorf("oauth2:token should be masked, got: %s", got)
	}
	if !containsStr(got, "oauth2:***") {
		t.Errorf("expected oauth2:*** replacement, got: %s", got)
	}
}

func TestValidBranchName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"main", true},
		{"feature/my-branch", true},
		{"release-1.0", true},
		{"v2.0.0", true},
		{"hotfix_fix_123", true},
		{"", false},
		{"-leading-dash", false},     // option injection
		{"..parent", false},          // path traversal
		{"branch//double", false},    // double slash
		{"branch/", false},           // trailing slash
		{"branch.", false},           // trailing dot
		{"branch.lock", false},       // .lock suffix
		{"branch/name.lock", false},  // .lock suffix
		{"branch with space", false}, // spaces
		{"branch;rm -rf", false},     // injection
		{"branch|pipe", false},       // pipe
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidBranchName(tt.name)
			if got != tt.valid {
				t.Errorf("ValidBranchName(%q) = %v, want %v", tt.name, got, tt.valid)
			}
		})
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
