package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// TeamsPlugin implements notifications via Microsoft Teams incoming webhooks.
type TeamsPlugin struct{}

func (p *TeamsPlugin) Name() string        { return "teams" }
func (p *TeamsPlugin) Version() string     { return "0.1.0" }
func (p *TeamsPlugin) Description() string { return "Microsoft Teams notification integration via webhooks" }
func (p *TeamsPlugin) PluginType() string  { return "notification" }

func (p *TeamsPlugin) Actions() []string {
	return []string{"send_message", "send_card"}
}

func (p *TeamsPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	webhookURL := config["webhook_url"]
	if webhookURL == "" {
		return nil, fmt.Errorf("webhook_url is required")
	}

	switch action {
	case "send_message":
		return p.sendMessage(ctx, params, webhookURL)
	case "send_card":
		return p.sendCard(ctx, params, webhookURL)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *TeamsPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "Teams plugin ready — requires webhook_url connection config",
	}, nil
}

// sendMessage sends a plain text message to a Teams channel via webhook.
func (p *TeamsPlugin) sendMessage(ctx context.Context, params []byte, webhookURL string) ([]byte, error) {
	var req struct {
		Text string `json:"text"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Text == "" {
		return nil, fmt.Errorf("text is required")
	}

	card := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"summary":    req.Text,
		"themeColor": "0076D7",
		"text":       req.Text,
	}
	if err := postWebhook(ctx, webhookURL, card); err != nil {
		return nil, err
	}
	return sdk.JSONMarshal(map[string]string{"status": "sent"})
}

// sendCard sends a rich MessageCard with title, color, and optional sections.
func (p *TeamsPlugin) sendCard(ctx context.Context, params []byte, webhookURL string) ([]byte, error) {
	var req struct {
		Title      string `json:"title"`
		Text       string `json:"text"`
		ThemeColor string `json:"theme_color"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Title == "" {
		return nil, fmt.Errorf("title is required")
	}

	color := req.ThemeColor
	if color == "" {
		color = "0076D7"
	}

	card := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"summary":    req.Title,
		"themeColor": color,
		"title":      req.Title,
	}
	if req.Text != "" {
		card["text"] = req.Text
	}

	if err := postWebhook(ctx, webhookURL, card); err != nil {
		return nil, err
	}
	return sdk.JSONMarshal(map[string]string{"status": "sent"})
}

// postWebhook sends a JSON payload to the Teams incoming webhook URL.
func postWebhook(ctx context.Context, webhookURL string, payload interface{}) error {
	if err := validateWebhookURL(webhookURL); err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// validateWebhookURL ensures the webhook URL points to a legitimate Teams endpoint
// and blocks private, loopback, link-local, and metadata IP ranges to prevent SSRF.
func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("webhook url must use https, got %q", u.Scheme)
	}
	host := strings.ToLower(u.Host)
	if !strings.HasSuffix(host, ".webhook.office.com") &&
		!strings.HasSuffix(host, ".outlook.com") &&
		!strings.HasSuffix(host, "teams.microsoft.com") {
		return fmt.Errorf("webhook url must point to a Microsoft Teams endpoint, got %q", host)
	}
	// Resolve hostname and block private/internal IPs.
	ips, err := net.LookupHost(u.Hostname())
	if err != nil {
		return fmt.Errorf("dns lookup failed for %q: %w", u.Hostname(), err)
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("webhook host resolves to blocked IP %s", ipStr)
		}
	}
	return nil
}
