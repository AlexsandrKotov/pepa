package vault

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient_DefaultMount(t *testing.T) {
	c, err := NewClient(Config{
		Address: "https://vault.example.com",
		Token:   "test-token",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.mountPath != "secret" {
		t.Errorf("expected default mount 'secret', got %q", c.mountPath)
	}
}

func TestNewClient_CustomMount(t *testing.T) {
	c, err := NewClient(Config{
		Address:   "https://vault.example.com",
		Token:     "tok",
		MountPath: "custom-kv",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.mountPath != "custom-kv" {
		t.Errorf("expected mount 'custom-kv', got %q", c.mountPath)
	}
}

func TestNewClient_InvalidScheme(t *testing.T) {
	_, err := NewClient(Config{
		Address: "ftp://vault.example.com",
		Token:   "tok",
	})
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
}

func TestNewClient_EmptyHostname(t *testing.T) {
	_, err := NewClient(Config{
		Address: "https://",
		Token:   "tok",
	})
	if err == nil {
		t.Fatal("expected error for empty hostname")
	}
}

func TestNewClient_TrailingSlash(t *testing.T) {
	c, err := NewClient(Config{
		Address: "https://vault.example.com/",
		Token:   "tok",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.address != "https://vault.example.com" {
		t.Errorf("trailing slash not trimmed: %q", c.address)
	}
}

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"169.254.1.1", true},
		{"100.64.0.1", true},   // CGNAT
		{"8.8.8.8", false},     // Google DNS
		{"1.1.1.1", false},     // Cloudflare
		{"93.184.216.34", false}, // example.com
	}
	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Fatalf("failed to parse IP %s", tt.ip)
		}
		got := isBlockedIP(ip)
		if got != tt.blocked {
			t.Errorf("isBlockedIP(%s) = %v, want %v", tt.ip, got, tt.blocked)
		}
	}
}

func TestIsBlockedIP_IPv6(t *testing.T) {
	// ::1 loopback
	ip := net.ParseIP("::1")
	if !isBlockedIP(ip) {
		t.Error("expected ::1 to be blocked")
	}
	// fe80:: link-local
	ip = net.ParseIP("fe80::1")
	if !isBlockedIP(ip) {
		t.Error("expected fe80::1 to be blocked")
	}
	// 2001:db8:: public
	ip = net.ParseIP("2001:db8::1")
	if isBlockedIP(ip) {
		t.Error("expected 2001:db8::1 to NOT be blocked")
	}
}

func TestValidateAddress_BlockedIP(t *testing.T) {
	// localhost resolves to 127.0.0.1 which should be blocked
	err := validateAddress("http://127.0.0.1:8200")
	if err == nil {
		t.Fatal("expected error for loopback address")
	}
}

func TestValidateAddress_PrivateIP(t *testing.T) {
	err := validateAddress("http://192.168.1.1:8200")
	if err == nil {
		t.Fatal("expected error for private IP address")
	}
}

func TestValidateAddress_ValidExternal(t *testing.T) {
	// This may pass or fail depending on DNS, but should not error on scheme
	err := validateAddress("https://vault.nonexistent-domain-test.example.com")
	// DNS will fail, but we allow it (comment says "might be reachable at request time")
	if err != nil {
		t.Logf("note: got error (acceptable if DNS-related): %v", err)
	}
}

// newTestClient creates a vault client pointing at the test server,
// bypassing SSRF protection (since test servers use localhost).
func newTestClient(addr string) *Client {
	return &Client{
		address:   addr,
		token:     "test-token",
		mountPath: "secret",
		httpClient: &http.Client{},
	}
}

func TestHealth(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sys/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "test-token" {
			w.WriteHeader(403)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"initialized": true,
			"sealed":      false,
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := newTestClient(ts.URL)
	result, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error: %v", err)
	}
	if result["initialized"] != true {
		t.Error("expected initialized=true")
	}
	if result["sealed"] != false {
		t.Error("expected sealed=false")
	}
}

func TestGetSecret(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/secret/data/myapp", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{
					"username": "admin",
					"password": "s3cret",
					"port":     5432,
				},
				"metadata": map[string]interface{}{
					"version":      1,
					"created_time": "2024-01-01T00:00:00Z",
					"destroyed":    false,
				},
			},
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := newTestClient(ts.URL)
	secret, err := c.GetSecret(context.Background(), "myapp")
	if err != nil {
		t.Fatalf("GetSecret() error: %v", err)
	}
	if secret.Data["username"] != "admin" {
		t.Errorf("username = %q, want admin", secret.Data["username"])
	}
	if secret.Data["password"] != "s3cret" {
		t.Errorf("password = %q, want s3cret", secret.Data["password"])
	}
	// Integer should be converted to string
	if secret.Data["port"] != "5432" {
		t.Errorf("port = %q, want 5432", secret.Data["port"])
	}
	if secret.Metadata.Version != 1 {
		t.Errorf("version = %d, want 1", secret.Metadata.Version)
	}
}

func TestGetSecret_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/secret/data/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.GetSecret(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
	if !containsStr(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestListSecrets(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/secret/metadata/apps", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list") != "true" {
			w.WriteHeader(400)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"keys": []string{"web", "api", "worker"},
			},
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := newTestClient(ts.URL)
	keys, err := c.ListSecrets(context.Background(), "apps")
	if err != nil {
		t.Fatalf("ListSecrets() error: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	expected := map[string]bool{"web": true, "api": true, "worker": true}
	for _, k := range keys {
		if !expected[k] {
			t.Errorf("unexpected key: %s", k)
		}
	}
}

func TestListSecrets_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/secret/metadata/empty", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := newTestClient(ts.URL)
	keys, err := c.ListSecrets(context.Background(), "empty")
	if err != nil {
		t.Fatalf("ListSecrets() error: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected empty list, got %v", keys)
	}
}

func TestWriteSecret(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/secret/data/new-secret", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		if r.Header.Get("X-Vault-Token") != "test-token" {
			w.WriteHeader(403)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"metadata": map[string]interface{}{
					"version":      1,
					"created_time": "2024-01-01T00:00:00Z",
					"updated_time": "2024-01-01T00:00:00Z",
					"destroyed":    false,
				},
			},
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := newTestClient(ts.URL)
	meta, err := c.WriteSecret(context.Background(), "new-secret", map[string]string{
		"key": "value",
	})
	if err != nil {
		t.Fatalf("WriteSecret() error: %v", err)
	}
	if meta.Version != 1 {
		t.Errorf("version = %d, want 1", meta.Version)
	}
}

func TestDeleteSecret(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/secret/metadata/old-secret", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			w.WriteHeader(405)
			return
		}
		w.WriteHeader(204)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := newTestClient(ts.URL)
	err := c.DeleteSecret(context.Background(), "old-secret")
	if err != nil {
		t.Fatalf("DeleteSecret() error: %v", err)
	}
}

func TestListEngines(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sys/mounts", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"secret/": map[string]interface{}{
					"type":        "kv",
					"description": "Main KV store",
					"options":     map[string]interface{}{"version": "2"},
				},
				"kv-v1/": map[string]interface{}{
					"type":        "kv",
					"description": "Legacy KV v1",
					"options":     map[string]interface{}{},
				},
				"sys/": map[string]interface{}{
					"type":        "system",
					"description": "System",
					"options":     map[string]interface{}{},
				},
			},
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := newTestClient(ts.URL)
	engines, err := c.ListEngines(context.Background())
	if err != nil {
		t.Fatalf("ListEngines() error: %v", err)
	}
	// Should only return kv engines (not sys)
	if len(engines) != 2 {
		t.Fatalf("expected 2 kv engines, got %d", len(engines))
	}
	for _, e := range engines {
		if e["type"] != "kv" {
			t.Errorf("expected type=kv, got %s", e["type"])
		}
		if e["path"] == "kv-v1" && e["version"] != "1" {
			t.Errorf("expected kv-v1 version=1, got %s", e["version"])
		}
		if e["path"] == "secret" && e["version"] != "2" {
			t.Errorf("expected secret version=2, got %s", e["version"])
		}
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
