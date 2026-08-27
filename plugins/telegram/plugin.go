package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// TelegramPlugin implements provider.Provider for Telegram notifications.
type TelegramPlugin struct{}

var _ provider.Provider = (*TelegramPlugin)(nil)

func (p *TelegramPlugin) Name() string        { return "telegram" }
func (p *TelegramPlugin) Version() string     { return "0.1.0" }
func (p *TelegramPlugin) Description() string { return "Telegram notification integration" }
func (p *TelegramPlugin) PluginType() string  { return "notification" }

func (p *TelegramPlugin) Actions() []string {
	return []string{"send_message", "get_updates"}
}

func (p *TelegramPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	botToken := config["bot_token"]
	chatID := config["chat_id"]

	switch action {
	case "send_message":
		return p.sendMessage(ctx, params, botToken, chatID)
	case "get_updates":
		return p.getUpdates(ctx, botToken)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *TelegramPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "Telegram plugin ready — requires connection config (bot_token and chat_id)",
	}, nil
}

// sendMessage sends a message via the Telegram Bot API.
func (p *TelegramPlugin) sendMessage(ctx context.Context, params []byte, botToken, defaultChatID string) ([]byte, error) {
	var req struct {
		ChatID    string `json:"chat_id"`
		Text      string `json:"text"`
		ParseMode string `json:"parse_mode"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Text == "" {
		return nil, fmt.Errorf("text is required")
	}
	if req.ChatID == "" {
		req.ChatID = defaultChatID
	}
	if req.ChatID == "" {
		return nil, fmt.Errorf("chat_id is required (provide in params or connection config)")
	}
	if botToken == "" {
		return nil, fmt.Errorf("bot_token is required")
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)

	payload := map[string]string{
		"chat_id": req.ChatID,
		"text":    req.Text,
	}
	if req.ParseMode != "" {
		payload["parse_mode"] = req.ParseMode
	}

	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("telegram api request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("telegram api returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram api returned ok=false")
	}

	return actionOutput(map[string]any{"status": "sent", "message_id": result.Result.MessageID})
}

// getUpdates retrieves recent updates (used for diagnostics).
func (p *TelegramPlugin) getUpdates(ctx context.Context, botToken string) ([]byte, error) {
	if botToken == "" {
		return nil, fmt.Errorf("bot_token is required")
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?limit=5", botToken)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("telegram api request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("telegram api returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return actionOutput(map[string]any{"updates": result})
}

func actionOutput(v interface{}) ([]byte, error) {
	return sdk.JSONMarshal(v)
}
