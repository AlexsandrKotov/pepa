package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	pepacrypto "github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/storage"
	"golang.org/x/crypto/ssh"
)

// ConnectionService handles connection testing business logic for various protocols.
type ConnectionService struct {
	httpClient        *http.Client
	builtinVaultCheck func(context.Context) error
}

// SetBuiltinVaultCheck installs the storage readiness probe before serving requests.
func (s *ConnectionService) SetBuiltinVaultCheck(check func(context.Context) error) {
	s.builtinVaultCheck = check
}

// NewConnectionService creates a new ConnectionService.
func NewConnectionService() *ConnectionService {
	return &ConnectionService{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// TestResult represents the result of a connection test.
type TestResult struct {
	Status  string
	Message string
}

// TestGitConnection tests a Git connection.
func (s *ConnectionService) TestGitConnection(ctx context.Context, rawURL, token, provider string) TestResult {
	url := strings.TrimRight(rawURL, "/")

	switch provider {
	case "github":
		return s.TestGitHubConnection(ctx, url, token)
	case "gitea":
		return s.TestGiteaConnection(ctx, url, token)
	case "bitbucket":
		return s.TestBitbucketConnection(ctx, url, token)
	case "local":
		return s.testLocalGitConnection(url)
	case "gitlab":
		return s.TestGitLabConnection(ctx, url, token)
	default:
		// Generic git: try common API endpoints
		return s.testGenericGitConnection(ctx, url, token)
	}
}

// TestGitHubConnection tests connectivity to GitHub or GitHub Enterprise.
func (s *ConnectionService) TestGitHubConnection(ctx context.Context, rawURL, token string) TestResult {
	apiURL := rawURL
	if !strings.Contains(rawURL, "github.com") && !strings.HasSuffix(rawURL, "/api/v3") {
		apiURL = rawURL + "/api/v3"
	} else if strings.HasSuffix(rawURL, "github.com") || strings.Contains(rawURL, "github.com") {
		apiURL = "https://api.github.com"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"/user", nil)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Failed to create request: %v", err)}
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "x509") {
			return TestResult{Status: "error", Message: fmt.Sprintf("TLS certificate error: %v. Check that the GitHub server certificate is trusted.", err)}
		}
		if strings.Contains(err.Error(), "no such host") {
			return TestResult{Status: "error", Message: fmt.Sprintf("Cannot resolve GitHub host: %v", err)}
		}
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach GitHub: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 401 {
		return TestResult{Status: "error", Message: "Invalid token - authentication failed. Verify the token is a valid GitHub Personal Access Token."}
	}
	if resp.StatusCode == 403 {
		return TestResult{Status: "error", Message: "Token is valid but access is forbidden. Check token scopes."}
	}
	if resp.StatusCode != 200 {
		return TestResult{Status: "error", Message: fmt.Sprintf("GitHub returned status %d", resp.StatusCode)}
	}

	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err == nil {
		username, _ := userInfo["login"].(string)
		return TestResult{Status: "connected", Message: fmt.Sprintf("Authenticated as %s. GitHub token valid.", username)}
	}
	return TestResult{Status: "connected", Message: "Successfully authenticated with GitHub"}
}

// TestGiteaConnection tests connectivity to a Gitea instance.
func (s *ConnectionService) TestGiteaConnection(ctx context.Context, rawURL, token string) TestResult {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL+"/api/v1/user", nil)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Failed to create request: %v", err)}
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach Gitea: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 401 {
		return TestResult{Status: "error", Message: "Invalid token - authentication failed"}
	}
	if resp.StatusCode != 200 {
		return TestResult{Status: "error", Message: fmt.Sprintf("Gitea returned status %d", resp.StatusCode)}
	}

	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err == nil {
		username, _ := userInfo["login"].(string)
		return TestResult{Status: "connected", Message: fmt.Sprintf("Authenticated as %s. Gitea token valid.", username)}
	}
	return TestResult{Status: "connected", Message: "Successfully authenticated with Gitea"}
}

// TestBitbucketConnection tests connectivity to Bitbucket.
func (s *ConnectionService) TestBitbucketConnection(ctx context.Context, rawURL, token string) TestResult {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL+"/2.0/user", nil)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Failed to create request: %v", err)}
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach Bitbucket: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 401 {
		return TestResult{Status: "error", Message: "Invalid token - authentication failed"}
	}
	if resp.StatusCode != 200 {
		return TestResult{Status: "error", Message: fmt.Sprintf("Bitbucket returned status %d", resp.StatusCode)}
	}

	return TestResult{Status: "connected", Message: "Successfully authenticated with Bitbucket"}
}

// TestGitLabConnection tests connectivity to GitLab.
func (s *ConnectionService) TestGitLabConnection(ctx context.Context, rawURL, token string) TestResult {
	apiURL := rawURL
	if !strings.HasSuffix(rawURL, "/api/v4") {
		apiURL = rawURL + "/api/v4"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"/user", nil)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Failed to create request: %v", err)}
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "x509") {
			return TestResult{Status: "error", Message: fmt.Sprintf("TLS certificate error: %v", err)}
		}
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach GitLab: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 401 {
		return TestResult{Status: "error", Message: "Invalid token - authentication failed"}
	}
	if resp.StatusCode != 200 {
		return TestResult{Status: "error", Message: fmt.Sprintf("GitLab returned status %d", resp.StatusCode)}
	}

	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err == nil {
		username, _ := userInfo["username"].(string)
		return TestResult{Status: "connected", Message: fmt.Sprintf("Authenticated as %s. GitLab token valid.", username)}
	}
	return TestResult{Status: "connected", Message: "Successfully authenticated with GitLab"}
}

// testLocalGitConnection tests a local git repository.
func (s *ConnectionService) testLocalGitConnection(path string) TestResult {
	// Check if path exists and is a git repository
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return TestResult{Status: "error", Message: fmt.Sprintf("Path does not exist: %s", path)}
	}

	gitDir := path + "/.git"
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return TestResult{Status: "error", Message: "Not a git repository (no .git directory)"}
	}

	return TestResult{Status: "connected", Message: fmt.Sprintf("Local git repository found at %s", path)}
}

// testGenericGitConnection tests a generic git server.
func (s *ConnectionService) testGenericGitConnection(ctx context.Context, rawURL, token string) TestResult {
	// Try common git server endpoints
	endpoints := []string{"/api/v1/user", "/api/v4/user", "/api/v3/user", "/user"}

	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, "GET", rawURL+endpoint, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "token "+token)

		resp, err := s.httpClient.Do(req)
		if err != nil {
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode == 200 {
			return TestResult{Status: "connected", Message: "Successfully connected to git server"}
		}
	}

	return TestResult{Status: "error", Message: "Could not connect to git server with provided credentials"}
}

// TestJiraConnection tests a Jira connection.
func (s *ConnectionService) TestJiraConnection(ctx context.Context, url, token string) TestResult {
	req, err := http.NewRequestWithContext(ctx, "GET", url+"/rest/api/2/myself", nil)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Failed to create request: %v", err)}
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach Jira: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 401 {
		return TestResult{Status: "error", Message: "Invalid token"}
	}
	if resp.StatusCode == 200 {
		return TestResult{Status: "connected", Message: "Successfully authenticated with Jira"}
	}
	return TestResult{Status: "error", Message: fmt.Sprintf("Jira returned status %d", resp.StatusCode)}
}

// TestAIConnection tests an AI provider connection.
func (s *ConnectionService) TestAIConnection(ctx context.Context, config map[string]any) TestResult {
	provider, _ := config["provider"].(string)
	apiKey, _ := config["api_key"].(string)

	switch provider {
	case "openai":
		if apiKey == "" {
			return TestResult{Status: "error", Message: "API key required for OpenAI"}
		}
		return TestResult{Status: "connected", Message: "OpenAI configuration valid"}
	case "ollama":
		baseURL, _ := config["base_url"].(string)
		if baseURL == "" {
			return TestResult{Status: "error", Message: "Base URL required for Ollama"}
		}
		// Try to reach Ollama
		req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/tags", nil)
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach Ollama: %v", err)}
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == 200 {
			return TestResult{Status: "connected", Message: "Successfully connected to Ollama"}
		}
		return TestResult{Status: "error", Message: fmt.Sprintf("Ollama returned status %d", resp.StatusCode)}
	case "anthropic":
		if apiKey == "" {
			return TestResult{Status: "error", Message: "API key required for Anthropic"}
		}
		return TestResult{Status: "connected", Message: "Anthropic configuration valid"}
	case "groq":
		if apiKey == "" {
			return TestResult{Status: "error", Message: "API key required for Groq"}
		}
		return TestResult{Status: "connected", Message: "Groq configuration valid"}
	case "qoder":
		if apiKey == "" {
			return TestResult{Status: "error", Message: "API key required for Qoder"}
		}
		baseURL, _ := config["base_url"].(string)
		if baseURL == "" {
			baseURL = "https://api.qoder.com/v1"
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach Qoder: %v", err)}
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == 200 {
			return TestResult{Status: "connected", Message: "Successfully connected to Qoder"}
		} else if resp.StatusCode == 401 {
			return TestResult{Status: "error", Message: "Invalid API key"}
		}
		return TestResult{Status: "error", Message: fmt.Sprintf("Qoder returned status %d", resp.StatusCode)}
	case "lmstudio":
		baseURL, _ := config["base_url"].(string)
		if baseURL == "" {
			baseURL, _ = config["url"].(string)
		}
		if baseURL == "" {
			baseURL = "http://host.docker.internal:1234/v1"
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach LM Studio: %v", err)}
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == 200 {
			return TestResult{Status: "connected", Message: "Successfully connected to LM Studio"}
		}
		return TestResult{Status: "error", Message: fmt.Sprintf("LM Studio returned status %d", resp.StatusCode)}
	default:
		return TestResult{Status: "error", Message: fmt.Sprintf("Unknown provider: %s", provider)}
	}
}

// TestStorageConnection tests a storage connection.
func (s *ConnectionService) TestStorageConnection(ctx context.Context, endpoint string) TestResult {
	req, err := http.NewRequestWithContext(ctx, "HEAD", endpoint, nil)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Invalid endpoint: %v", err)}
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach storage: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	// Any response means endpoint is reachable
	return TestResult{Status: "connected", Message: "Storage endpoint is reachable"}
}

// TestCIConnection tests a CI system connection.
func (s *ConnectionService) TestCIConnection(ctx context.Context, url string, config map[string]any) TestResult {
	provider, _ := config["provider"].(string)
	token, _ := config["token"].(string)

	if url == "" {
		return TestResult{Status: "error", Message: "CI URL is required"}
	}

	switch provider {
	case "jenkins":
		req, _ := http.NewRequestWithContext(ctx, "GET", url+"/api/json", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach Jenkins: %v", err)}
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == 200 {
			return TestResult{Status: "connected", Message: "Successfully connected to Jenkins"}
		}
		return TestResult{Status: "error", Message: fmt.Sprintf("Jenkins returned status %d", resp.StatusCode)}

	case "circleci":
		req, _ := http.NewRequestWithContext(ctx, "GET", url+"/api/v1.1/me", nil)
		if token != "" {
			req.Header.Set("Circle-Token", token)
		}
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach CircleCI: %v", err)}
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == 200 {
			return TestResult{Status: "connected", Message: "Successfully connected to CircleCI"}
		}
		return TestResult{Status: "error", Message: fmt.Sprintf("CircleCI returned status %d", resp.StatusCode)}

	default:
		return TestResult{Status: "error", Message: fmt.Sprintf("Unknown CI provider: %s", provider)}
	}
}

// TestDockerConnection tests a Docker connection.
func (s *ConnectionService) TestDockerConnection(ctx context.Context, config map[string]any) TestResult {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	hostType, _ := config["host_type"].(string)
	host, _ := config["host"].(string)
	if host == "" {
		host, _ = config["host_address"].(string)
	}
	host = strings.TrimSpace(host)
	if hostType == "" {
		switch {
		case host == "", strings.HasPrefix(host, "unix://"):
			hostType = "local"
		case strings.HasPrefix(host, "ssh://"), strings.Contains(host, "@") && !strings.Contains(host, "://"):
			hostType = "ssh"
		default:
			hostType = "tcp"
		}
	}
	if hostType == "ssh" {
		return testDockerSSH(ctx, host, config)
	}

	transport := &http.Transport{TLSHandshakeTimeout: 5 * time.Second}
	defer transport.CloseIdleConnections()
	endpoint := ""
	switch hostType {
	case "local":
		if host == "" {
			host = "unix:///var/run/docker.sock"
		}
		u, err := url.Parse(host)
		if err != nil || u.Scheme != "unix" || u.Host != "" || u.Path == "" || u.RawQuery != "" || u.Fragment != "" {
			return TestResult{Status: "error", Message: "Local Docker requires a unix:///absolute/socket/path address"}
		}
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", u.Path)
		}
		endpoint = "http://docker/_ping"
	case "tcp":
		u, err := url.Parse(host)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return TestResult{Status: "error", Message: "Docker host must be a tcp://, http://, or https:// address"}
		}
		if u.Scheme != "tcp" && u.Scheme != "http" && u.Scheme != "https" {
			return TestResult{Status: "error", Message: "Unsupported Docker host scheme"}
		}
		ca, _ := config["tls_ca_cert"].(string)
		cert, _ := config["tls_cert"].(string)
		key, _ := config["tls_key"].(string)
		hasTLS := ca != "" || cert != "" || key != ""
		if u.Scheme == "http" && hasTLS {
			return TestResult{Status: "error", Message: "TLS credentials require a tcp:// or https:// Docker address"}
		}
		if u.Scheme == "tcp" {
			u.Scheme = "http"
			if hasTLS {
				u.Scheme = "https"
			}
		}
		if u.Scheme == "https" {
			tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
			if ca != "" {
				pool := x509.NewCertPool()
				if !pool.AppendCertsFromPEM([]byte(ca)) {
					return TestResult{Status: "error", Message: "Invalid Docker CA certificate"}
				}
				tlsConfig.RootCAs = pool
			}
			if cert != "" || key != "" {
				pair, err := tls.X509KeyPair([]byte(cert), []byte(key))
				if err != nil {
					return TestResult{Status: "error", Message: "Invalid Docker client certificate/key pair"}
				}
				tlsConfig.Certificates = []tls.Certificate{pair}
			}
			transport.TLSClientConfig = tlsConfig
		}
		u.Path = "/_ping"
		endpoint = u.String()
	default:
		return TestResult{Status: "error", Message: fmt.Sprintf("Unknown Docker host type: %s", hostType)}
	}

	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: noConnectionRedirect}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return TestResult{Status: "error", Message: "Invalid Docker ping request"}
	}
	resp, err := client.Do(req)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach Docker: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil || resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "OK" {
		return TestResult{Status: "error", Message: fmt.Sprintf("Docker ping failed (HTTP %d or invalid response)", resp.StatusCode)}
	}
	return TestResult{Status: "connected", Message: "Successfully connected to Docker daemon"}
}

// noConnectionRedirect keeps connection checks and credentials on the configured endpoint.
func noConnectionRedirect(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

func testDockerSSH(ctx context.Context, host string, config map[string]any) TestResult {
	if !strings.Contains(host, "://") {
		host = "ssh://" + host
	}
	u, err := url.Parse(host)
	if err != nil || u.Scheme != "ssh" || u.Hostname() == "" || u.User == nil || u.User.Username() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return TestResult{Status: "error", Message: "SSH Docker requires ssh://user@host[:port]"}
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		return TestResult{Status: "error", Message: "Use the SSH private key field, not a password in the host address"}
	}
	privateKey, _ := config["ssh_key"].(string)
	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return TestResult{Status: "error", Message: "A valid, unencrypted SSH private key is required"}
	}
	hostKey, _ := config["ssh_host_key"].(string)
	publicKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hostKey))
	if err != nil {
		return TestResult{Status: "error", Message: "A verified SSH host public key is required in ssh_host_key"}
	}
	port := u.Port()
	if port == "" {
		port = "22"
	}
	addr := net.JoinHostPort(u.Hostname(), port)
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach SSH Docker host: %v", err)}
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return TestResult{Status: "error", Message: "Cannot set SSH connection deadline"}
		}
	}
	sshConn, channels, requests, err := ssh.NewClientConn(conn, addr, &ssh.ClientConfig{
		User: u.User.Username(), Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.FixedHostKey(publicKey),
	})
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("SSH authentication or host verification failed: %v", err)}
	}
	client := ssh.NewClient(sshConn, channels, requests)
	defer func() { _ = client.Close() }()
	session, err := client.NewSession()
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot open Docker SSH session: %v", err)}
	}
	defer func() { _ = session.Close() }()
	output, err := session.StdoutPipe()
	if err != nil {
		return TestResult{Status: "error", Message: "Cannot read Docker SSH output"}
	}
	if err := session.Start("docker version --format '{{json .Server}}'"); err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot execute Docker check: %v", err)}
	}
	body, err := io.ReadAll(io.LimitReader(output, 64*1024))
	if err != nil || len(body) >= 64*1024 {
		return TestResult{Status: "error", Message: "Invalid Docker SSH response"}
	}
	if err := session.Wait(); err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Remote Docker daemon check failed: %v", err)}
	}
	var server struct{ Version string }
	if err := json.Unmarshal(body, &server); err != nil || server.Version == "" {
		return TestResult{Status: "error", Message: "SSH connected, but no Docker server version was returned"}
	}
	return TestResult{Status: "connected", Message: "Successfully verified the remote Docker daemon over SSH"}
}

// TestVaultConnection tests a Vault connection.
func (s *ConnectionService) TestVaultConnection(ctx context.Context, config map[string]any) TestResult {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	backendMode, _ := config["backend_mode"].(string)
	address, _ := config["address"].(string)
	token, _ := config["token"].(string)
	if backendMode == "" {
		backendMode = "builtin"
		if address != "" || token != "" {
			backendMode = "vault"
		}
	}
	if backendMode == "builtin" {
		if s.builtinVaultCheck == nil {
			return TestResult{Status: "disconnected", Message: "Built-in KV is configured, but no storage readiness probe is available"}
		}
		if err := s.builtinVaultCheck(ctx); err != nil {
			return TestResult{Status: "error", Message: "Built-in KV storage is unavailable"}
		}
		const probe = "pepa-vault-readiness"
		encrypted, err := pepacrypto.Encrypt(probe)
		if err != nil {
			return TestResult{Status: "error", Message: "Built-in KV encryption is unavailable; check ENCRYPTION_KEY"}
		}
		decrypted, err := pepacrypto.Decrypt(encrypted)
		if err != nil || decrypted != probe {
			return TestResult{Status: "error", Message: "Built-in KV encryption self-test failed"}
		}
		return TestResult{Status: "connected", Message: "Built-in KV storage is reachable and encryption self-test passed"}
	}
	if backendMode != "vault" {
		return TestResult{Status: "error", Message: fmt.Sprintf("Unknown secret backend mode: %s", backendMode)}
	}
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return TestResult{Status: "error", Message: "Vault address must be an http:// or https:// URL"}
	}
	insecure := config["insecure_tls"] == true || config["insecure_tls"] == "true"
	transport := &http.Transport{
		TLSHandshakeTimeout: 5 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: insecure, //nolint:gosec // #nosec G402: explicit per-connection administrator opt-in
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport, CheckRedirect: noConnectionRedirect}
	baseURL := strings.TrimRight(u.String(), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/sys/health", nil)
	if err != nil {
		return TestResult{Status: "error", Message: "Invalid Vault health request"}
	}
	resp, err := client.Do(req)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach Vault: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusTooManyRequests {
		return TestResult{Status: "error", Message: fmt.Sprintf("Vault returned status %d", resp.StatusCode)}
	}
	var health struct {
		Initialized bool `json:"initialized"`
		Sealed      bool `json:"sealed"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&health); err != nil || !health.Initialized || health.Sealed {
		return TestResult{Status: "error", Message: "Vault is not initialized, is sealed, or returned an invalid health response"}
	}
	if token == "" {
		return TestResult{Status: "disconnected", Message: "Vault is reachable, but no authentication token is configured"}
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/auth/token/lookup-self", nil)
	if err != nil {
		return TestResult{Status: "error", Message: "Invalid Vault token verification request"}
	}
	req.Header.Set("X-Vault-Token", token)
	authResp, err := client.Do(req)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot verify Vault token: %v", err)}
	}
	defer func() { _ = authResp.Body.Close() }()
	if authResp.StatusCode != http.StatusOK {
		return TestResult{Status: "error", Message: fmt.Sprintf("Vault token verification failed (HTTP %d)", authResp.StatusCode)}
	}
	return TestResult{Status: "connected", Message: "Successfully connected to Vault and verified the token"}
}

// TestNotificationConnection tests a notification service connection.
func (s *ConnectionService) TestNotificationConnection(ctx context.Context, config map[string]any) TestResult {
	provider, _ := config["provider"].(string)
	if provider == "" {
		return TestResult{Status: "error", Message: "No notification provider configured"}
	}

	switch provider {
	case "slack":
		webhookURL, _ := config["webhook_url"].(string)
		botToken, _ := config["bot_token"].(string)
		if webhookURL == "" && botToken == "" {
			return TestResult{Status: "error", Message: "Either webhook_url or bot_token is required for Slack"}
		}
		if webhookURL != "" {
			// Send a test ping to the webhook
			payload := []byte(`{"text":"PEPA connection test"}`)
			req, _ := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			resp, err := s.httpClient.Do(req)
			if err != nil {
				return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach Slack webhook: %v", err)}
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode == 200 {
				return TestResult{Status: "connected", Message: "Slack webhook is reachable"}
			}
			return TestResult{Status: "error", Message: fmt.Sprintf("Slack webhook returned status %d", resp.StatusCode)}
		}
		return TestResult{Status: "connected", Message: "Slack bot_token configured"}

	case "telegram":
		botToken, _ := config["bot_token"].(string)
		chatID, _ := config["chat_id"].(string)
		if botToken == "" {
			return TestResult{Status: "error", Message: "bot_token is required for Telegram"}
		}
		if chatID == "" {
			return TestResult{Status: "error", Message: "chat_id is required for Telegram"}
		}
		// Test by calling getMe
		apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", botToken)
		req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach Telegram API: %v", err)}
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == 200 {
			return TestResult{Status: "connected", Message: "Telegram bot is reachable"}
		}
		return TestResult{Status: "error", Message: fmt.Sprintf("Telegram API returned status %d", resp.StatusCode)}

	default:
		return TestResult{Status: "error", Message: fmt.Sprintf("Unknown notification provider: %s", provider)}
	}
}

// TestGitBasicAuthConnection tests a Git connection with basic auth.
func (s *ConnectionService) TestGitBasicAuthConnection(ctx context.Context, rawURL, username, password, provider string) TestResult {
	url := strings.TrimRight(rawURL, "/")

	// Use a client with InsecureSkipVerify for git servers (self-signed certs common)
	gitClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // #nosec // G402: self-signed certs common in git servers
		},
	}

	switch provider {
	case "gitea":
		req, _ := http.NewRequestWithContext(ctx, "GET", url+"/api/v1/user", nil)
		req.SetBasicAuth(username, password)
		resp, err := gitClient.Do(req)
		if err != nil {
			return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach Gitea: %v", err)}
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == 200 {
			var info map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&info); err == nil {
				login, _ := info["login"].(string)
				return TestResult{Status: "connected", Message: fmt.Sprintf("Authenticated as %s. Gitea credentials valid.", login)}
			}
			return TestResult{Status: "connected", Message: "Successfully authenticated with Gitea"}
		}
		return TestResult{Status: "error", Message: fmt.Sprintf("Gitea returned status %d — check credentials", resp.StatusCode)}

	case "gitlab":
		req, _ := http.NewRequestWithContext(ctx, "GET", url+"/api/v4/user", nil)
		req.SetBasicAuth(username, password)
		resp, err := gitClient.Do(req)
		if err != nil {
			return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach GitLab: %v", err)}
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == 200 {
			var info map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&info); err == nil {
				login, _ := info["username"].(string)
				return TestResult{Status: "connected", Message: fmt.Sprintf("Authenticated as %s. GitLab credentials valid.", login)}
			}
			return TestResult{Status: "connected", Message: "Successfully authenticated with GitLab"}
		}
		return TestResult{Status: "error", Message: fmt.Sprintf("GitLab returned status %d — check credentials", resp.StatusCode)}

	default:
		// Generic git: just try to reach the server
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		resp, err := gitClient.Do(req)
		if err != nil {
			return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach git server: %v", err)}
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode < 500 {
			return TestResult{Status: "connected", Message: fmt.Sprintf("Git server reachable (status %d)", resp.StatusCode)}
		}
		return TestResult{Status: "error", Message: fmt.Sprintf("Git server returned status %d", resp.StatusCode)}
	}
}

// TestS3Credential tests S3 credentials.
func (s *ConnectionService) TestS3Credential(ctx context.Context, endpoint, accessKey, secretKey string) TestResult {
	// Try with SSL first, then without
	for _, useSSL := range []bool{true, false} {
		s3Client, err := storage.NewS3ClientFromCredentials(endpoint, accessKey, secretKey, useSSL)
		if err != nil {
			return TestResult{Status: "error", Message: err.Error()}
		}
		if _, err := s3Client.ListBuckets(ctx); err == nil {
			return TestResult{Status: "connected", Message: fmt.Sprintf("S3 credentials valid (ssl=%v)", useSSL)}
		}
	}
	return TestResult{Status: "error", Message: "S3 credentials rejected by server"}
}

// TestSonarQubeConnection tests a SonarQube connection. Internal instances commonly
// serve self-signed certificates, so the connection's `insecure` flag is honoured here
// exactly as it is by the sonarqube plugin during a scan.
func (s *ConnectionService) TestSonarQubeConnection(ctx context.Context, url, token string, insecure bool) TestResult {
	if url == "" {
		return TestResult{Status: "error", Message: "SonarQube URL is required"}
	}
	if token == "" {
		return TestResult{Status: "error", Message: "SonarQube token is required"}
	}

	client := s.httpClient
	if insecure {
		client = &http.Client{
			Timeout: s.httpClient.Timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // #nosec // G402: admin-provided endpoint, opt-in per connection
			},
		}
	}

	// Call /api/system/status endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(url, "/")+"/api/system/status", nil)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Failed to create request: %v", err)}
	}
	// SonarQube uses Basic Auth with token as username and empty password
	req.SetBasicAuth(token, "")

	resp, err := client.Do(req)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach SonarQube: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 401 {
		return TestResult{Status: "error", Message: "Invalid token - authentication failed"}
	}
	if resp.StatusCode != 200 {
		return TestResult{Status: "error", Message: fmt.Sprintf("SonarQube returned status %d", resp.StatusCode)}
	}

	var statusResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err == nil {
		if status, ok := statusResp["status"].(string); ok {
			return TestResult{Status: "connected", Message: fmt.Sprintf("Successfully connected to SonarQube. System status: %s", status)}
		}
	}
	return TestResult{Status: "connected", Message: "Successfully connected to SonarQube"}
}
