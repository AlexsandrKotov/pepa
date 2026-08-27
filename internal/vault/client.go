// Package vault provides a client for HashiCorp Vault's HTTP API.
// Supports KV v2 secret engine operations.
package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client communicates with a HashiCorp Vault server.
type Client struct {
	address    string
	token      string
	mountPath  string // KV engine mount path, default "secret"
	httpClient *http.Client
}

// Config holds Vault connection configuration.
type Config struct {
	Address   string `json:"address"`
	Token     string `json:"token"`
	MountPath string `json:"mount_path"` // default "secret"
}

// SecretData represents a KV v2 secret response.
type SecretData struct {
	Data     map[string]string `json:"data"`
	Metadata SecretMetadata    `json:"metadata"`
}

// SecretMetadata holds version info for a KV v2 secret.
type SecretMetadata struct {
	Version      int    `json:"version"`
	CreatedTime  string `json:"created_time"`
	UpdatedTime  string `json:"updated_time"`
	Destroyed    bool   `json:"destroyed"`
	DeletionTime string `json:"deletion_time"`
}

// NewClient creates a new Vault client.
func NewClient(cfg Config) (*Client, error) {
	mount := cfg.MountPath
	if mount == "" {
		mount = "secret"
	}
	if err := validateAddress(cfg.Address); err != nil {
		return nil, err
	}
	return &Client{
		address:   strings.TrimRight(cfg.Address, "/"),
		token:     cfg.Token,
		mountPath: mount,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("too many redirects")
				}
				return validateRedirect(req.URL)
			},
		},
	}, nil
}

// validateAddress ensures the Vault address uses an allowed scheme and does not
// point to loopback, link-local, or private network ranges.
func validateAddress(addr string) error {
	u, err := url.Parse(addr)
	if err != nil {
		return fmt.Errorf("invalid vault address: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("vault address must use http or https, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("vault address must include a hostname")
	}
	// Resolve and check IP ranges
	ips, err := net.LookupIP(host)
	if err != nil {
		// If DNS fails, still allow it (might be reachable at request time)
		return nil
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("vault address resolves to blocked IP range: %s", ip)
		}
	}
	return nil
}

// validateRedirect checks redirect targets are not pointing to blocked IPs.
func validateRedirect(u *url.URL) error {
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("redirect to blocked IP range: %s", ip)
		}
	}
	return nil
}

// isBlockedIP returns true if the IP is in a private/loopback/link-local range.
// Set VAULT_ALLOW_PRIVATE_IPS=true to bypass entirely (dev/docker).
// Set VAULT_ALLOWED_CIDRS=10.0.0.0/8,172.16.0.0/12 to allow specific ranges only.
func isBlockedIP(ip net.IP) bool {
	// Full bypass — dev/docker mode
	if os.Getenv("VAULT_ALLOW_PRIVATE_IPS") == "true" {
		return false
	}
	// Granular allowlist — production-safe
	if allowedCIDRs := os.Getenv("VAULT_ALLOWED_CIDRS"); allowedCIDRs != "" {
		for _, cidr := range strings.Split(allowedCIDRs, ",") {
			cidr = strings.TrimSpace(cidr)
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			if network.Contains(ip) {
				return false
			}
		}
	}
	blocked := []string{
		"127.0.0.0/8",    // loopback
		"::1/128",        // loopback v6
		"10.0.0.0/8",     // private
		"172.16.0.0/12",  // private
		"192.168.0.0/16", // private
		"169.254.0.0/16", // link-local
		"fe80::/10",      // link-local v6
		"100.64.0.0/10",  // CGNAT
	}
	for _, cidr := range blocked {
		_, network, _ := net.ParseCIDR(cidr)
		if network != nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

// Health checks if the Vault server is reachable and healthy.
func (c *Client) Health(ctx context.Context) (map[string]interface{}, error) {
	resp, err := c.doRequest(ctx, "GET", "/v1/sys/health", nil)
	if err != nil {
		return nil, fmt.Errorf("vault health check failed: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode health response: %w", err)
	}
	return result, nil
}

// ListSecrets lists secret keys at the given path.
func (c *Client) ListSecrets(ctx context.Context, path string) ([]string, error) {
	apiPath := fmt.Sprintf("/v1/%s/metadata/%s", c.mountPath, strings.TrimRight(path, "/"))
	if path == "" {
		apiPath = fmt.Sprintf("/v1/%s/metadata", c.mountPath)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.address+apiPath+"?list=true", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault list failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return []string{}, nil
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			Keys []string `json:"keys"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode list response: %w", err)
	}
	return result.Data.Keys, nil
}

// GetSecret reads a secret at the given path.
func (c *Client) GetSecret(ctx context.Context, path string) (*SecretData, error) {
	apiPath := fmt.Sprintf("/v1/%s/data/%s", c.mountPath, path)

	resp, err := c.doRequest(ctx, "GET", apiPath, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("secret not found: %s", path)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			Data     map[string]interface{} `json:"data"`
			Metadata SecretMetadata         `json:"metadata"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode secret response: %w", err)
	}

	// Convert all values to strings
	data := make(map[string]string)
	for k, v := range result.Data.Data {
		data[k] = fmt.Sprintf("%v", v)
	}

	return &SecretData{
		Data:     data,
		Metadata: result.Data.Metadata,
	}, nil
}

// WriteSecret writes a secret at the given path.
func (c *Client) WriteSecret(ctx context.Context, path string, data map[string]string) (*SecretMetadata, error) {
	apiPath := fmt.Sprintf("/v1/%s/data/%s", c.mountPath, path)

	payload := map[string]interface{}{
		"data": data,
	}

	resp, err := c.doRequest(ctx, "POST", apiPath, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			Metadata SecretMetadata `json:"metadata"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode write response: %w", err)
	}

	return &result.Data.Metadata, nil
}

// DeleteSecret deletes a secret at the given path.
func (c *Client) DeleteSecret(ctx context.Context, path string) error {
	apiPath := fmt.Sprintf("/v1/%s/metadata/%s", c.mountPath, path)

	resp, err := c.doRequest(ctx, "DELETE", apiPath, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault delete returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ListEngines returns available secret engines from the mount table.
func (c *Client) ListEngines(ctx context.Context) ([]map[string]string, error) {
	resp, err := c.doRequest(ctx, "GET", "/v1/sys/mounts", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
			Options     struct {
				Version string `json:"version"`
			} `json:"options"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode engines response: %w", err)
	}

	var engines []map[string]string
	for path, mount := range result.Data {
		if mount.Type == "kv" {
			version := mount.Options.Version
			if version == "" {
				version = "1"
			}
			engines = append(engines, map[string]string{
				"path":        strings.TrimSuffix(path, "/"),
				"type":        "kv",
				"description": mount.Description,
				"version":     version,
			})
		}
	}
	return engines, nil
}

// doRequest performs an authenticated HTTP request to Vault.
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.address + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Vault-Token", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault request failed: %w", err)
	}
	return resp, nil
}
