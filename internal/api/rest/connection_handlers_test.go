package rest

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
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

func TestConnectionTestConfigRejectsUnsafeVaultReferences(t *testing.T) {
	for _, tc := range []struct {
		kind           repository.ConnectionType
		key, ref, want string
	}{
		{repository.ConnectionSecret, "backend_mode", "vault:checks/private/value", "credential fields"},
		{repository.ConnectionSecret, "address", "vault:checks/private/value", "credential fields"},
		{repository.ConnectionDocker, "host_type", "vault:checks/private/value", "credential fields"},
		{repository.ConnectionDocker, "host", "vault:checks/private/value", "credential fields"},
		{repository.ConnectionDocker, "unknown_token", "vault:checks/private/value", "credential fields"},
		{repository.ConnectionSecret, "token", "vault:path", "invalid vault reference"},
		{repository.ConnectionSecret, "token", "vault:path/", "invalid vault reference"},
		{repository.ConnectionSecret, "token", "vault:/key", "invalid vault reference"},
		{repository.ConnectionSecret, "token", "vault:allowed/../private/key", "invalid vault reference"},
		{repository.ConnectionSecret, "token", "vault:allowed/%2e%2e/private/key", "invalid vault reference"},
		{repository.ConnectionSecret, "token", "vault:allowed//private/key", "invalid vault reference"},
		{repository.ConnectionSecret, "token", "vault:allowed?other/key", "invalid vault reference"},
	} {
		t.Run(string(tc.kind)+"/"+tc.key+"/"+tc.ref, func(t *testing.T) {
			conn := &repository.Connection{Type: tc.kind, Config: map[string]any{tc.key: tc.ref}}
			// No identity or repositories: validation must precede any secret read.
			_, err := resolveConnectionTestConfig(Dependencies{}, nil, t.Context(), conn)
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), tc.ref) {
				t.Fatalf("unexpected validation result: %v", err)
			}
		})
	}
}

func TestConnectionTestConfigFailsClosedWithoutAuthorization(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	tenantID := uuid.New()
	c.Set(auth.CtxTenantID, tenantID)
	for _, key := range []string{"tls_key", "tls_cert", "tls_ca_cert", "ssh_key", "ssh_host_key"} {
		conn := &repository.Connection{TenantID: tenantID, Type: repository.ConnectionDocker, Config: map[string]any{key: "vault:checks/private/value"}}
		if _, err := resolveConnectionTestConfig(Dependencies{}, c, t.Context(), conn); err == nil {
			t.Fatal("anonymous secret resolution was accepted")
		}
		c.Set(auth.CtxUserID, uuid.New())
		if _, err := resolveConnectionTestConfig(Dependencies{}, c, t.Context(), conn); err == nil {
			t.Fatal("secret resolution without authorization dependencies was accepted")
		}
		delete(c.Keys, auth.CtxUserID)
	}
}

func TestConnectionTestConfigCopiesLiteralCredentials(t *testing.T) {
	conn := &repository.Connection{Type: repository.ConnectionSecret, Config: map[string]any{"backend_mode": "vault", "token": "literal-test-value"}}
	config, err := resolveConnectionTestConfig(Dependencies{}, nil, t.Context(), conn)
	if err != nil || !reflect.DeepEqual(config, conn.Config) {
		t.Fatalf("literal config changed: %v", err)
	}
	config["token"] = "changed"
	if conn.Config["token"] != "literal-test-value" {
		t.Fatal("test config aliases stored config")
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
