package service

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestDockerConnectionTCPAndLegacy(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/_ping" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, "OK")
	}))
	defer server.Close()
	for _, config := range []map[string]any{
		{"host_type": "tcp", "host": strings.Replace(server.URL, "http://", "tcp://", 1)},
		{"host_type": "tcp", "host": server.URL + "/"},
		{"host": server.URL},
		{"host_address": server.URL},
	} {
		result := NewConnectionService().TestDockerConnection(t.Context(), config)
		if result.Status != "connected" {
			t.Fatalf("config %v: %+v", config, result)
		}
	}
	if calls.Load() != 4 {
		t.Fatalf("expected four real pings, got %d", calls.Load())
	}
	server.Close()
	if result := NewConnectionService().TestDockerConnection(t.Context(), map[string]any{"host": server.URL}); result.Status != "error" {
		t.Fatalf("unreachable legacy host reported success: %+v", result)
	}
}

func TestDockerConnectionRejectsInvalidConfig(t *testing.T) {
	for name, config := range map[string]map[string]any{
		"missing TCP host":       {"host_type": "tcp"},
		"missing SSH host":       {"host_type": "ssh"},
		"unknown mode":           {"host_type": "typo"},
		"invalid URL":            {"host_type": "tcp", "host": "http://%invalid"},
		"unsupported scheme":     {"host": "ftp://localhost:2375"},
		"userinfo":               {"host": "http://user:password@localhost:2375"},
		"path":                   {"host": "http://localhost:2375/not-docker"},
		"local remote mismatch":  {"host_type": "local", "host": "http://localhost:2375"},
		"TCP socket mismatch":    {"host_type": "tcp", "host": "unix:///var/run/docker.sock"},
		"invalid CA":             {"host": "tcp://localhost:2376", "tls_ca_cert": "invalid"},
		"missing key":            {"host": "tcp://localhost:2376", "tls_cert": "invalid"},
		"missing cert":           {"host": "tcp://localhost:2376", "tls_key": "invalid"},
		"TLS downgrade":          {"host": "http://localhost:2375", "tls_key": "invalid"},
		"SSH credentials absent": {"host": "ssh://user@localhost"},
	} {
		t.Run(name, func(t *testing.T) {
			if result := NewConnectionService().TestDockerConnection(t.Context(), config); result.Status != "error" {
				t.Fatalf("invalid config accepted: %+v", result)
			}
		})
	}
}

func TestDockerConnectionLocalSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_ping" {
			t.Errorf("unexpected socket request: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, "OK")
	}), ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	for _, mode := range []string{"local", ""} {
		result := NewConnectionService().TestDockerConnection(t.Context(), map[string]any{"host_type": mode, "host": "unix://" + path})
		if result.Status != "connected" {
			t.Fatalf("socket check failed: %+v", result)
		}
	}
	_ = server.Close()
	result := NewConnectionService().TestDockerConnection(t.Context(), map[string]any{"host_type": "local", "host": "unix://" + path})
	if result.Status != "error" {
		t.Fatalf("missing socket reported success: %+v", result)
	}
}

func testClientCertificate(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-client"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		IsCA:        true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

func TestDockerConnectionMutualTLS(t *testing.T) {
	cert, key := testClientCertificate(t)
	clientCAs := x509.NewCertPool()
	clientCAs.AppendCertsFromPEM([]byte(cert))
	var calls atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			t.Error("client certificate was not sent")
		}
		_, _ = io.WriteString(w, "OK")
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientCAs}
	server.StartTLS()
	defer server.Close()
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	for _, scheme := range []string{"tcp", "https"} {
		result := NewConnectionService().TestDockerConnection(t.Context(), map[string]any{
			"host_type": "tcp", "host": strings.Replace(server.URL, "https", scheme, 1),
			"tls_ca_cert": ca, "tls_cert": cert, "tls_key": key,
		})
		if result.Status != "connected" {
			t.Fatalf("mTLS failed: %+v", result)
		}
	}
	for name, config := range map[string]map[string]any{
		"missing client certificate": {"host": server.URL, "tls_ca_cert": ca},
		"untrusted server":           {"host": server.URL, "tls_cert": cert, "tls_key": key},
		"wrong CA":                   {"host": server.URL, "tls_ca_cert": cert, "tls_cert": cert, "tls_key": key},
	} {
		t.Run(name, func(t *testing.T) {
			if result := NewConnectionService().TestDockerConnection(t.Context(), config); result.Status != "error" {
				t.Fatalf("invalid TLS connection accepted: %+v", result)
			}
		})
	}
	if calls.Load() != 2 {
		t.Fatalf("expected two authenticated pings, got %d", calls.Load())
	}
}

func TestDockerConnectionRejectsNonDockerAndRedirects(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusFound} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", "/redirect")
				w.WriteHeader(code)
				_, _ = io.WriteString(w, "not a Docker daemon")
			}))
			defer server.Close()
			result := NewConnectionService().TestDockerConnection(t.Context(), map[string]any{"host": server.URL})
			if result.Status != "error" || calls.Load() != 1 {
				t.Fatalf("unexpected result/calls: %+v / %d", result, calls.Load())
			}
		})
	}
}

func testSSHKey(t *testing.T) (ssh.Signer, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return signer, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestDockerConnectionSSH(t *testing.T) {
	for _, scenario := range []string{"success", "wrong host key", "wrong client key", "docker failure", "empty version"} {
		t.Run(scenario, func(t *testing.T) {
			hostSigner, _ := testSSHKey(t)
			clientSigner, privateKey := testSSHKey(t)
			otherSigner, otherPrivateKey := testSSHKey(t)
			serverConfig := &ssh.ServerConfig{PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
				if meta.User() != "tester" || !bytes.Equal(key.Marshal(), clientSigner.PublicKey().Marshal()) {
					return nil, errors.New("rejected")
				}
				return nil, nil
			}}
			serverConfig.AddHostKey(hostSigner)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				raw, err := listener.Accept()
				if err != nil {
					return
				}
				defer func() { _ = raw.Close() }()
				_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
				conn, channels, requests, err := ssh.NewServerConn(raw, serverConfig)
				if err != nil {
					return
				}
				defer func() { _ = conn.Close() }()
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					channel, requests, err := incoming.Accept()
					if err != nil {
						return
					}
					for request := range requests {
						if request.Type != "exec" {
							_ = request.Reply(false, nil)
							continue
						}
						var command struct{ Command string }
						if err := ssh.Unmarshal(request.Payload, &command); err != nil || command.Command != "docker version --format '{{json .Server}}'" {
							t.Error("unexpected SSH command")
						}
						_ = request.Reply(true, nil)
						body, exit := `{"Version":"28.0.0"}`, uint32(0)
						if scenario == "docker failure" {
							exit = 1
						}
						if scenario == "empty version" {
							body = `{}`
						}
						_, _ = io.WriteString(channel, body)
						_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{exit}))
						_ = channel.Close()
						return
					}
				}
			}()
			t.Cleanup(func() { _ = listener.Close(); <-done })
			hostKey := string(ssh.MarshalAuthorizedKey(hostSigner.PublicKey()))
			if scenario == "wrong host key" {
				hostKey = string(ssh.MarshalAuthorizedKey(otherSigner.PublicKey()))
			}
			if scenario == "wrong client key" {
				privateKey = otherPrivateKey
			}
			result := NewConnectionService().TestDockerConnection(t.Context(), map[string]any{
				"host": "ssh://tester@" + listener.Addr().String(), "ssh_key": privateKey, "ssh_host_key": hostKey,
			})
			want := "error"
			if scenario == "success" {
				want = "connected"
			}
			if result.Status != want {
				t.Fatalf("got %+v, want %s", result, want)
			}
		})
	}
}

func TestConnectionChecksRespectCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	for _, kind := range []string{"docker", "vault"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			svc := NewConnectionService()
			var result TestResult
			if kind == "docker" {
				result = svc.TestDockerConnection(ctx, map[string]any{"host": server.URL})
			} else {
				result = svc.TestVaultConnection(ctx, map[string]any{"address": server.URL, "token": "test"})
			}
			if result.Status != "error" {
				t.Fatalf("canceled check accepted: %+v", result)
			}
		})
	}
}

func TestVaultConnectionModesAndAuthentication(t *testing.T) {
	for _, scenario := range []string{"legacy", "explicit", "standby", "invalid token", "sealed", "invalid response", "redirect", "missing token"} {
		t.Run(scenario, func(t *testing.T) {
			var healthCalls, authCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/sys/health":
					healthCalls.Add(1)
					if scenario == "redirect" {
						w.Header().Set("Location", "/elsewhere")
						w.WriteHeader(302)
						return
					}
					if scenario == "sealed" {
						w.WriteHeader(503)
						return
					}
					if scenario == "invalid response" {
						_, _ = io.WriteString(w, "not vault")
						return
					}
					if scenario == "standby" {
						w.WriteHeader(429)
					}
					_, _ = io.WriteString(w, `{"initialized":true,"sealed":false}`)
				case "/v1/auth/token/lookup-self":
					authCalls.Add(1)
					if r.Header.Get("X-Vault-Token") != "test-token" {
						t.Error("token not forwarded")
					}
					if scenario == "invalid token" {
						w.WriteHeader(403)
						return
					}
					_, _ = io.WriteString(w, `{"data":{"id":"test-token"}}`)
				default:
					t.Errorf("unexpected Vault request: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			config := map[string]any{"address": server.URL, "token": "test-token"}
			if scenario == "explicit" {
				config["backend_mode"] = "vault"
			}
			if scenario == "missing token" {
				delete(config, "token")
			}
			result := NewConnectionService().TestVaultConnection(t.Context(), config)
			want, wantAuth := "error", int32(0)
			switch scenario {
			case "legacy", "explicit", "standby":
				want, wantAuth = "connected", 1
			case "invalid token":
				wantAuth = 1
			case "missing token":
				want = "disconnected"
			}
			if result.Status != want || healthCalls.Load() != 1 || authCalls.Load() != wantAuth {
				t.Fatalf("got %+v, health=%d auth=%d; want %s auth=%d", result, healthCalls.Load(), authCalls.Load(), want, wantAuth)
			}
		})
	}
	for _, config := range []map[string]any{
		{"backend_mode": "vault"}, {"backend_mode": "typo"}, {"token": "test"},
		{"address": "http://%invalid"}, {"address": "ftp://localhost"},
	} {
		if result := NewConnectionService().TestVaultConnection(t.Context(), config); result.Status != "error" {
			t.Fatalf("invalid Vault config accepted: %+v", result)
		}
	}
}

func TestVaultConnectionBuiltinReadiness(t *testing.T) {
	svc := NewConnectionService()
	config := map[string]any{"backend_mode": "builtin", "address": "http://unused.invalid"}
	if result := svc.TestVaultConnection(t.Context(), config); result.Status != "disconnected" {
		t.Fatalf("unverified builtin reported success: %+v", result)
	}
	svc.SetBuiltinVaultCheck(func(context.Context) error { return errors.New("storage offline") })
	if result := svc.TestVaultConnection(t.Context(), config); result.Status != "error" {
		t.Fatal(result)
	}
	svc.SetBuiltinVaultCheck(func(context.Context) error { return nil })
	for _, key := range []string{"ENCRYPTION_KEY", "AUTH_JWT_SECRET", "JWT_SECRET"} {
		t.Setenv(key, "")
	}
	if result := svc.TestVaultConnection(t.Context(), config); result.Status != "error" {
		t.Fatal(result)
	}
	t.Setenv("ENCRYPTION_KEY", "connection-readiness-test-key-32-characters")
	for _, cfg := range []map[string]any{config, {}} {
		if result := svc.TestVaultConnection(t.Context(), cfg); result.Status != "connected" {
			t.Fatal(result)
		}
	}
}

func TestVaultConnectionTLSOptIn(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"initialized":true,"sealed":false,"data":{}}`)
	}))
	defer server.Close()
	config := map[string]any{"address": server.URL, "token": "test-token"}
	if result := NewConnectionService().TestVaultConnection(t.Context(), config); result.Status != "error" {
		t.Fatalf("untrusted Vault TLS accepted without opt-in: %+v", result)
	}
	for _, insecure := range []any{true, "true"} {
		config["insecure_tls"] = insecure
		if result := NewConnectionService().TestVaultConnection(t.Context(), config); result.Status != "connected" {
			t.Fatal(result)
		}
	}
}
