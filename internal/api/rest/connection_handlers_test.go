package rest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/pkg/utils"
)

func TestSanitizeConnectionConfig(t *testing.T) {
	config := map[string]any{
		"tls_key": "private-test-value", "api_key": "api-test-value", "private_key": "ssh-test-value",
		"token": "token-test-value", "host": "tcp://docker.example.com:2376",
		"ssh_host_key": "ssh-ed25519 public", "tls_cert": "public certificate",
	}
	admin := sanitizeConnectionConfig(config, true)
	for key, value := range config {
		if utils.IsSensitiveKey(key) {
			if admin[key] != "***" {
				t.Errorf("%s was not masked", key)
			}
		} else if admin[key] != value {
			t.Errorf("public field %s changed", key)
		}
	}
	body, err := json.Marshal(admin)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "test-value") {
		t.Fatal("response leaked a secret")
	}
	if config["tls_key"] != "private-test-value" {
		t.Fatal("sanitizing mutated stored config")
	}
	nonAdmin := sanitizeConnectionConfig(config, false)
	if len(nonAdmin) != 1 || nonAdmin["_masked"] != true {
		t.Fatal("non-admin config was disclosed")
	}
	for _, key := range []string{"tls_key", "TLS_KEY", "api_key", "ssh_private_key", "password", "host"} {
		if isSensitiveConfigKey(key) != utils.IsSensitiveKey(key) {
			t.Errorf("inconsistent secret policy for %s", key)
		}
	}
}

func TestDockerCredentialFallbackPolicy(t *testing.T) {
	conn := &repository.Connection{
		Type: repository.ConnectionDocker, Config: map[string]any{"host": "tcp://docker.example.com:2376", "tls_key": "test-key"},
	}
	userID := uuid.New()
	deps := Dependencies{Repos: &Repositories{}}
	if _, err := ResolveConnectionCredential(t.Context(), deps, conn, &userID, "docker", "host"); err == nil {
		t.Fatal("disabled administrator fallback must reject missing personal credentials")
	}
	conn.FallbackToAdmin = true
	resolved, err := ResolveConnectionCredential(t.Context(), deps, conn, &userID, "docker", "host")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Source != CredentialSourceAdmin || resolved.Config["tls_key"] != "test-key" {
		t.Fatal("enabled fallback did not supply administrator credentials")
	}
}
