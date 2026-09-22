//go:build integration

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/service"
	"github.com/pepa/pepa/internal/testenv"
)

func connectionHTTPTestDeps(t *testing.T) Dependencies {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", "connection-http-regression-test-key")
	pg, err := testenv.StartPostgres(t.Context(), t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Terminate(context.Background()) })
	return Dependencies{
		DB: pg.DB(),
		Repos: &Repositories{
			Connection: repository.NewConnectionRepository(pg.DB()),
			Vault:      repository.NewVaultRepository(pg.DB()),
		},
		Services: &Services{Connection: service.NewConnectionService()},
	}
}

// Inject a verified identity to exercise the handlers and database, not the login middleware.
func callConnectionHTTP(t *testing.T, tenantID uuid.UUID, admin bool, handler gin.HandlerFunc, method, pattern, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return callHandler(t, func(c *gin.Context) {
		c.Set(auth.CtxTenantID, tenantID)
		if admin {
			c.Set(auth.CtxRoles, []string{"admin"})
		}
		handler(c)
	}, method, pattern, path, body, nil)
}

func TestConnectionHTTPSecretRoundTrip(t *testing.T) {
	deps := connectionHTTPTestDeps(t)
	tenantID := uuid.New()
	const base = "/api/v1/connections"
	const payload = `{"type":"docker","name":"secret-round-trip","fallback_to_admin":false,"config":{"host_type":"tcp","host":"tcp://docker.example.com:2376","tls_key":"private-http-test-value","tls_cert":"public certificate","ssh_key":"ssh-http-test-value"}}`
	created := callConnectionHTTP(t, tenantID, true, createConnection(deps), http.MethodPost, base, base, payload)
	if created.Code != http.StatusCreated {
		t.Fatalf("create returned HTTP %d: %s", created.Code, created.Body)
	}
	var conn repository.Connection
	if err := json.Unmarshal(created.Body.Bytes(), &conn); err != nil {
		t.Fatal(err)
	}
	assertMasked := func(config map[string]any, admin bool) {
		t.Helper()
		if !admin {
			if !reflect.DeepEqual(config, map[string]any{"_masked": true}) {
				t.Fatal("non-admin response exposed connection configuration")
			}
			return
		}
		if config["tls_key"] != "***" || config["ssh_key"] != "***" {
			t.Fatal("response exposed private keys")
		}
		if config["tls_cert"] != "public certificate" {
			t.Fatal("public certificate was modified")
		}
	}
	assertMasked(conn.Config, true)
	if conn.FallbackToAdmin {
		t.Fatal("create ignored the disabled fallback policy")
	}
	before, err := deps.Repos.Connection.Get(t.Context(), conn.ID, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := before.Config["tls_key"].(string)
	if !crypto.IsEncrypted(key) {
		t.Fatal("HTTP create stored a plaintext TLS key")
	}
	path := base + "/" + conn.ID.String()
	for _, admin := range []bool{true, false} {
		response := callConnectionHTTP(t, tenantID, admin, getConnection(deps), http.MethodGet, base+"/:id", path, "")
		if response.Code != http.StatusOK {
			t.Fatalf("get returned HTTP %d", response.Code)
		}
		var detail repository.Connection
		if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
			t.Fatal(err)
		}
		assertMasked(detail.Config, admin)
		response = callConnectionHTTP(t, tenantID, admin, listConnections(deps), http.MethodGet, base, base, "")
		if response.Code != http.StatusOK {
			t.Fatalf("list returned HTTP %d", response.Code)
		}
		var list struct {
			Connections []repository.Connection `json:"connections"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		if len(list.Connections) != 1 {
			t.Fatalf("expected one connection, got %d", len(list.Connections))
		}
		assertMasked(list.Connections[0].Config, admin)
	}
	const update = `{"name":"edited","config":{"tls_key":"***","ssh_key":"***","token":"***","tls_cert":"public certificate","host_type":"tcp","host":"tcp://docker.example.com:2376"}}`
	response := callConnectionHTTP(t, tenantID, true, updateConnection(deps), http.MethodPut, base+"/:id", path, update)
	if response.Code != http.StatusOK {
		t.Fatalf("update returned HTTP %d: %s", response.Code, response.Body)
	}
	var updated repository.Connection
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	assertMasked(updated.Config, true)
	after, err := deps.Repos.Connection.Get(t.Context(), conn.ID, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Config["tls_key"] != key || after.Config["ssh_key"] != before.Config["ssh_key"] {
		t.Fatal("masked update overwrote stored private keys")
	}
	if _, exists := after.Config["token"]; exists {
		t.Fatal("masked update created a placeholder credential")
	}
	if after.FallbackToAdmin {
		t.Fatal("update changed the disabled fallback policy")
	}
	decrypted, err := deps.Repos.Connection.GetDecrypted(t.Context(), conn.ID, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted.Config["tls_key"] != "private-http-test-value" {
		t.Fatal("stored key failed to round-trip")
	}
}

func TestConnectionHTTPTestPreservesVaultReferences(t *testing.T) {
	deps := connectionHTTPTestDeps(t)
	tenantID := uuid.New()
	const token = "vault-http-regression-token"
	if _, err := deps.Repos.Vault.Set(t.Context(), tenantID, "checks/vault", map[string]string{"token": token}, "", nil); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/sys/health":
			_, _ = w.Write([]byte(`{"initialized":true,"sealed":false}`))
		case "/v1/auth/token/lookup-self":
			if r.Header.Get("X-Vault-Token") != token {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"id":"fixture"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	for _, tc := range []struct {
		name, ref, status string
		wantCalls         int32
	}{
		{"resolved", "vault:checks/vault/token", "connected", 2},
		{"missing", "vault:checks/missing/token", "error", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls.Store(0)
			conn := &repository.Connection{
				TenantID: tenantID, Type: repository.ConnectionSecret, Name: tc.name,
				Status: "disconnected", FallbackToAdmin: false,
				Config: map[string]any{"address": server.URL, "token": tc.ref},
				Labels: map[string]string{},
			}
			if err := deps.Repos.Connection.Create(t.Context(), conn); err != nil {
				t.Fatal(err)
			}
			before, err := deps.Repos.Connection.Get(t.Context(), conn.ID, tenantID)
			if err != nil {
				t.Fatal(err)
			}
			path := "/api/v1/connections/" + conn.ID.String() + "/test"
			response := callConnectionHTTP(t, tenantID, true, testConnection(deps), http.MethodPost, "/api/v1/connections/:id/test", path, "")
			if response.Code != http.StatusOK {
				t.Fatalf("test returned HTTP %d: %s", response.Code, response.Body)
			}
			var result struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Status != tc.status || calls.Load() != tc.wantCalls {
				t.Fatalf("status=%s calls=%d; want status=%s calls=%d", result.Status, calls.Load(), tc.status, tc.wantCalls)
			}
			if strings.Contains(response.Body.String(), token) {
				t.Fatal("test response leaked the resolved token")
			}
			after, err := deps.Repos.Connection.Get(t.Context(), conn.ID, tenantID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Config, after.Config) || after.FallbackToAdmin {
				t.Fatal("testing changed stored credentials or fallback policy")
			}
			if after.Status != tc.status || after.LastCheckAt == nil {
				t.Fatal("test result or check timestamp was not persisted")
			}
			decrypted, err := deps.Repos.Connection.GetDecrypted(t.Context(), conn.ID, tenantID)
			if err != nil {
				t.Fatal(err)
			}
			if decrypted.Config["token"] != tc.ref {
				t.Fatal("test replaced the Vault reference")
			}
		})
	}
}
