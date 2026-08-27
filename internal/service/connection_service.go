package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pepa/pepa/internal/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// ConnectionService handles connection testing business logic for various protocols.
type ConnectionService struct {
	httpClient *http.Client
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

// TestKubernetesConnection tests a Kubernetes connection.
func (s *ConnectionService) TestKubernetesConnection(ctx context.Context, kubeconfig string, connConfig map[string]any) TestResult {
	// Detect if kubeconfig is still encrypted (decryption failed)
	if strings.HasPrefix(kubeconfig, "enc:") {
		return TestResult{Status: "error", Message: "Kubeconfig is still encrypted — decryption failed. The encryption key (ENCRYPTION_KEY or AUTH_JWT_SECRET) may have changed since this connection was created. Please re-enter the kubeconfig in the connection settings."}
	}

	// Parse kubeconfig using client-go
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Invalid kubeconfig: %v", err)}
	}

	// Set timeout
	config.Timeout = 10 * time.Second

	// Support insecure TLS (self-signed certs, SAN mismatch)
	if insecure, _ := connConfig["insecure"].(string); insecure == "true" || insecure == "1" {
		config.Insecure = true
		config.CAFile = ""
		config.CAData = nil
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Failed to create K8s client: %v", err)}
	}

	// Test 1: Check API Server connectivity and get version
	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach API Server: %v. Check if cluster is accessible from PEPA network.", err)}
	}

	// Test 2: Check node status
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot list nodes: %v", err)}
	}

	readyNodes := 0
	notReadyNodes := 0
	for _, node := range nodes.Items {
		isReady := false
		for _, condition := range node.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "True" {
				isReady = true
				break
			}
		}
		if isReady {
			readyNodes++
		} else {
			notReadyNodes++
		}
	}

	// Test 3: Check system pods in kube-system namespace
	systemPods, err := clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot list system pods: %v", err)}
	}

	runningPods := 0
	for _, pod := range systemPods.Items {
		if pod.Status.Phase == "Running" {
			runningPods++
		}
	}

	// Test 4: Check recent events
	events, err := clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		Limit: 10,
	})
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot list events: %v", err)}
	}

	// Build comprehensive status message
	status := "connected"
	message := fmt.Sprintf(
		"Connected successfully. K8s %s. Nodes: %d Ready, %d NotReady. System pods: %d/%d running. Recent events: %d.",
		version.String(),
		readyNodes,
		notReadyNodes,
		runningPods,
		len(systemPods.Items),
		len(events.Items),
	)

	// If any nodes are NotReady, warn but still mark as connected
	if notReadyNodes > 0 {
		message += fmt.Sprintf(" WARNING: %d nodes are NotReady - check CNI plugin.", notReadyNodes)
	}

	return TestResult{Status: status, Message: message}
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
func (s *ConnectionService) TestDockerConnection(ctx context.Context, host string) TestResult {
	if host == "" || host == "unix:///var/run/docker.sock" {
		// Inside a container, we can't reach the host Docker socket directly.
		// Just validate the configuration is present.
		return TestResult{Status: "connected", Message: "Docker socket configured (local connection)"}
	}
	// TCP-based Docker host
	req, _ := http.NewRequestWithContext(ctx, "GET", host+"/_ping", nil)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach Docker: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 200 {
		return TestResult{Status: "connected", Message: "Successfully connected to Docker daemon"}
	}
	return TestResult{Status: "error", Message: fmt.Sprintf("Docker returned status %d", resp.StatusCode)}
}

// TestVaultConnection tests a Vault connection.
func (s *ConnectionService) TestVaultConnection(ctx context.Context, address, token string) TestResult {
	if address == "" {
		return TestResult{Status: "error", Message: "No Vault address configured"}
	}
	url := strings.TrimRight(address, "/")
	req, _ := http.NewRequestWithContext(ctx, "GET", url+"/v1/sys/health", nil)
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}

	// Use a client with InsecureSkipVerify for Vault (self-signed certs common)
	vaultClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := vaultClient.Do(req)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach Vault: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 200 || resp.StatusCode == 429 {
		return TestResult{Status: "connected", Message: "Successfully connected to Vault"}
	}
	if resp.StatusCode == 403 {
		return TestResult{Status: "error", Message: "Vault token is invalid or expired"}
	}
	return TestResult{Status: "error", Message: fmt.Sprintf("Vault returned status %d", resp.StatusCode)}
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

// TestKubernetesServerConnection tests a Kubernetes server connection.
func (s *ConnectionService) TestKubernetesServerConnection(ctx context.Context, server string, config map[string]any) TestResult {
	url := strings.TrimRight(server, "/")
	req, _ := http.NewRequestWithContext(ctx, "GET", url+"/version", nil)
	// Add Bearer token if provided
	if token, ok := config["token"].(string); ok && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Use a client with InsecureSkipVerify for K8s API (self-signed certs common)
	k8sClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := k8sClient.Do(req)
	if err != nil {
		return TestResult{Status: "error", Message: fmt.Sprintf("Cannot reach API Server: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 200 {
		return TestResult{Status: "connected", Message: "Successfully connected to Kubernetes API server"}
	}
	return TestResult{Status: "error", Message: fmt.Sprintf("API Server returned status %d", resp.StatusCode)}
}

// TestGitBasicAuthConnection tests a Git connection with basic auth.
func (s *ConnectionService) TestGitBasicAuthConnection(ctx context.Context, rawURL, username, password, provider string) TestResult {
	url := strings.TrimRight(rawURL, "/")

	// Use a client with InsecureSkipVerify for git servers (self-signed certs common)
	gitClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
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
