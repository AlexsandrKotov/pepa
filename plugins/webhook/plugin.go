package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// allowedMethods is the whitelist of HTTP methods permitted for webhooks.
var allowedMethods = map[string]bool{
	"POST":  true,
	"PUT":   true,
	"PATCH": true,
}

// WebhookPlugin implements provider.Provider for generic HTTP webhook notifications.
type WebhookPlugin struct{}

var _ provider.Provider = (*WebhookPlugin)(nil)

func (p *WebhookPlugin) Name() string        { return "webhook" }
func (p *WebhookPlugin) Version() string     { return "0.1.0" }
func (p *WebhookPlugin) Description() string { return "Generic HTTP webhook notification plugin" }
func (p *WebhookPlugin) PluginType() string  { return "notification" }

func (p *WebhookPlugin) Actions() []string {
	return []string{"send_webhook"}
}

func (p *WebhookPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	switch action {
	case "send_webhook":
		return p.sendWebhook(ctx, params, config)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *WebhookPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "Webhook plugin ready — requires target URL in connection config or params",
	}, nil
}

// sendWebhook sends a JSON payload to the configured webhook URL.
func (p *WebhookPlugin) sendWebhook(ctx context.Context, params []byte, config map[string]string) ([]byte, error) {
	var req struct {
		URL     string            `json:"url"`
		Payload json.RawMessage   `json:"payload"`
		Headers map[string]string `json:"headers"`
		Method  string            `json:"method"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}

	targetURL := req.URL
	if targetURL == "" {
		targetURL = config["webhook_url"]
	}
	if targetURL == "" {
		return nil, fmt.Errorf("webhook URL is required (set in params or connection config)")
	}

	method := req.Method
	if method == "" {
		method = "POST"
	}
	// Whitelist HTTP methods
	if !allowedMethods[method] {
		return nil, fmt.Errorf("HTTP method %q not allowed (use POST, PUT, or PATCH)", method)
	}

	// SSRF protection: validate URL and resolve IP once, then pin the connection
	// to the resolved IP to prevent DNS rebinding (TOCTOU).
	safeAddr, err := validateWebhookURL(targetURL)
	if err != nil {
		return nil, err
	}

	// Build payload
	var body []byte
	if req.Payload != nil {
		body = req.Payload
	} else {
		body = []byte(`{}`)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set default headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "PEPA-Webhook/0.1.0")

	// Apply custom headers from params
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	// Apply auth headers from connection config
	if token := config["bearer_token"]; token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}
	if secret := config["hmac_secret"]; secret != "" {
		sig := computeHMAC(body, secret)
		httpReq.Header.Set("X-PEPA-Signature", "sha256="+sig)
	}
	if apiKey := config["api_key"]; apiKey != "" {
		headerName := config["api_key_header"]
		if headerName == "" {
			headerName = "X-API-Key"
		}
		httpReq.Header.Set(headerName, apiKey)
	}

	// Use a custom transport that dials the pre-validated IP address,
	// preventing DNS rebinding between validation and connection.
	u, _ := url.Parse(targetURL)
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	dialAddr := net.JoinHostPort(safeAddr, port)

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			// Always connect to the pre-validated IP, ignoring the URL's hostname
			dialer := &net.Dialer{Timeout: 10 * time.Second}
			return dialer.DialContext(ctx, "tcp", dialAddr)
		},
		TLSHandshakeTimeout: 10 * time.Second,
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
	}

	return actionOutput(map[string]any{
		"status":      "sent",
		"status_code": resp.StatusCode,
		"url":         targetURL,
		"method":      method,
	})
}

// validateWebhookURL ensures the webhook URL is safe (no SSRF).
// Returns the first resolved safe IP address for connection pinning.
func validateWebhookURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid webhook URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("webhook URL must use http or https, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("webhook URL has no host")
	}
	// Resolve and block private/internal ranges
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", fmt.Errorf("webhook URL DNS resolution failed: %w", err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return "", fmt.Errorf("webhook URL resolves to blocked address %s", ip)
		}
	}
	// Return the first safe IP for connection pinning
	return ips[0].String(), nil
}

// isBlockedIP returns true for loopback, link-local, private, and metadata IPs.
// Covers both IPv4 and IPv6 private ranges.
func isBlockedIP(ip net.IP) bool {
	// Generic checks (covers IPv6 loopback ::1, link-local fe80::/10, unspecified, and private ranges)
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	// Explicit IPv4 ranges (defense in depth)
	if ip4 := ip.To4(); ip4 != nil {
		blocked := []struct {
			name string
			n    *net.IPNet
		}{
			{"loopback", &net.IPNet{IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)}},
			{"link-local", &net.IPNet{IP: net.IPv4(169, 254, 0, 0), Mask: net.CIDRMask(16, 32)}},
			{"private-10", &net.IPNet{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)}},
			{"private-172", &net.IPNet{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)}},
			{"private-192", &net.IPNet{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)}},
		}
		for _, b := range blocked {
			if b.n.Contains(ip) {
				return true
			}
		}
		// Block cloud metadata endpoint
		if ip.String() == "169.254.169.254" {
			return true
		}
	}
	// Explicit IPv6 unique local range fc00::/7
	if ip.To4() == nil && ip.To16() != nil {
		_, fc00, _ := net.ParseCIDR("fc00::/7")
		if fc00.Contains(ip) {
			return true
		}
	}
	return false
}

// computeHMAC returns hex-encoded HMAC-SHA256 of data using the given secret.
func computeHMAC(data []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

func actionOutput(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
