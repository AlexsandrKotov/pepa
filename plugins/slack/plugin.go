package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// SlackPlugin implements provider.Provider for Slack notifications.
type SlackPlugin struct{}

var _ provider.Provider = (*SlackPlugin)(nil)

func (p *SlackPlugin) Name() string        { return "slack" }
func (p *SlackPlugin) Version() string     { return "0.1.0" }
func (p *SlackPlugin) Description() string { return "Slack notification integration" }
func (p *SlackPlugin) PluginType() string  { return "notification" }

func (p *SlackPlugin) Actions() []string {
	return []string{"send_message", "list_channels"}
}

func (p *SlackPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	webhookURL := config["webhook_url"]
	botToken := config["bot_token"]

	switch action {
	case "send_message":
		return p.sendMessage(ctx, params, webhookURL, botToken)
	case "list_channels":
		return p.listChannels(ctx, botToken)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *SlackPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "Slack plugin ready — requires connection config (webhook_url or bot_token)",
	}, nil
}

// sendMessage sends a message via webhook or Bot Token API.
func (p *SlackPlugin) sendMessage(ctx context.Context, params []byte, webhookURL, botToken string) ([]byte, error) {
	var req struct {
		Channel string `json:"channel"`
		Text    string `json:"text"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Text == "" {
		return nil, fmt.Errorf("text is required")
	}

	// Prefer webhook for simple messages
	if webhookURL != "" {
		if err := validateWebhookURL(webhookURL); err != nil {
			return nil, err
		}
		payload := map[string]string{"text": req.Text}
		if req.Channel != "" {
			payload["channel"] = req.Channel
		}
		body, _ := json.Marshal(payload)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("webhook request: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
		}
		return actionOutput(map[string]string{"status": "sent", "method": "webhook"})
	}

	// Use Bot Token API
	if botToken == "" {
		return nil, fmt.Errorf("either webhook_url or bot_token is required")
	}
	apiPayload := map[string]string{"text": req.Text}
	if req.Channel != "" {
		apiPayload["channel"] = req.Channel
	}
	body, _ := json.Marshal(apiPayload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+botToken)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("slack api request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("slack api returned %d: %s", resp.StatusCode, string(respBody))
	}
	return actionOutput(map[string]string{"status": "sent", "method": "bot_token"})
}

// listChannels lists public and private channels using the Slack API.
func (p *SlackPlugin) listChannels(ctx context.Context, botToken string) ([]byte, error) {
	if botToken == "" {
		return nil, fmt.Errorf("bot_token is required for list_channels")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://slack.com/api/conversations.list?types=public_channel,private_channel&limit=200", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+botToken)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("slack api request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		OK       bool `json:"ok"`
		Channels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"channels"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("slack api error: %s", result.Error)
	}

	type channel struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	channels := make([]channel, 0, len(result.Channels))
	for _, ch := range result.Channels {
		channels = append(channels, channel{ID: ch.ID, Name: ch.Name})
	}
	return actionOutput(map[string]any{"channels": channels, "total": len(channels)})
}

func actionOutput(v interface{}) ([]byte, error) {
	return sdk.JSONMarshal(v)
}

// validateWebhookURL ensures the webhook URL points to a legitimate Slack endpoint.
func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("webhook url must use https, got %q", u.Scheme)
	}
	host := strings.ToLower(u.Host)
	if !strings.HasSuffix(host, ".slack.com") && host != "hooks.slack.com" {
		return fmt.Errorf("webhook url must point to hooks.slack.com, got %q", host)
	}
	return nil
}
