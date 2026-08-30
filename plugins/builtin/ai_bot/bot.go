// Package ai_bot provides a Slack/Telegram bot plugin that routes messages
// to the PEPA AI Manager and returns responses back to the chat channel.
package ai_bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// BotConfig holds configuration for the AI bot.
type BotConfig struct {
	Platform   string `json:"platform"`    // slack, telegram
	Token      string `json:"token"`       // Bot token
	APIURL     string `json:"api_url"`     // PEPA API URL
	PEPAToken  string `json:"pepa_token"`  // PEPA auth token
	ChannelID  string `json:"channel_id"`  // Default channel (optional)
}

// Bot handles chat messages from Slack/Telegram and routes them to PEPA AI.
type Bot struct {
	config  BotConfig
	client  *http.Client
	stopCh  chan struct{}
}

// NewBot creates a new AI bot.
func NewBot(cfg BotConfig) *Bot {
	return &Bot{
		config: cfg,
		client: &http.Client{Timeout: 120 * time.Second},
		stopCh: make(chan struct{}),
	}
}

// Start begins polling for messages (for Telegram) or starts the webhook server (for Slack).
func (b *Bot) Start(ctx context.Context) error {
	switch b.config.Platform {
	case "slack":
		return b.startSlack(ctx)
	case "telegram":
		return b.startTelegram(ctx)
	default:
		return fmt.Errorf("unsupported platform: %s", b.config.Platform)
	}
}

// Stop shuts down the bot.
func (b *Bot) Stop() {
	close(b.stopCh)
}

// startSlack starts the Slack bot using Events API.
// TODO: Implement Slack RTM/Events API integration.
// Currently a placeholder — Slack requires a public webhook URL
// which is different from Telegram's polling approach.
func (b *Bot) startSlack(ctx context.Context) error {
	slog.Info("AI bot started (Slack mode — stub, not yet implemented)")

	// Slack uses Events API with webhooks. A production implementation
	// would register a handler at /api/v1/bot/slack/events and verify
	// Slack's URL challenge. For now, just block until context is done.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.stopCh:
		return nil
	}
}

// startTelegram starts the Telegram bot using long polling.
func (b *Bot) startTelegram(ctx context.Context) error {
	slog.Info("AI bot started (Telegram mode)")

	var offset int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-b.stopCh:
			return nil
		default:
		}

		// Poll for updates
		updates, err := b.telegramGetUpdates(ctx, offset)
		if err != nil {
			slog.Warn("telegram poll failed", "error", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range updates {
			offset = update.UpdateID + 1
			if update.Message.Text == "" {
				continue
			}

			// Handle commands
			response := b.handleMessage(update.Message.Text, update.Message.From.Username)
			if response != "" {
				_ = b.telegramSendMessage(ctx, update.Message.Chat.ID, response)
			}
		}
	}
}

// handleMessage processes a chat message and returns an AI response.
func (b *Bot) handleMessage(text string, from string) string {
	text = strings.TrimSpace(text)

	// Check for commands
	if strings.HasPrefix(text, "/ai ") || strings.HasPrefix(text, "/ai@") {
		query := strings.TrimPrefix(text, "/ai")
		// Remove bot mention (e.g. @botname) if present
		if strings.HasPrefix(query, "@") {
			if idx := strings.Index(query, " "); idx >= 0 {
				query = query[idx:]
			} else {
				query = ""
			}
		}
		query = strings.TrimSpace(query)
		if query == "" {
			return "Usage: /ai <your question>"
		}
		return b.queryPEPA(query)
	}

	if strings.HasPrefix(text, "/deploy ") {
		return b.handleDeploy(text)
	}

	if strings.HasPrefix(text, "/status") {
		return b.handleStatus()
	}

	if strings.HasPrefix(text, "/incident") {
		return b.handleIncident(text)
	}

	// Default: treat as AI query if it starts with @bot or is a reply
	if strings.HasPrefix(text, "@pepa") || strings.HasPrefix(text, "/pepa") {
		query := strings.TrimPrefix(text, "@pepa")
		query = strings.TrimPrefix(query, "/pepa")
		query = strings.TrimSpace(query)
		if query != "" {
			return b.queryPEPA(query)
		}
	}

	return ""
}

// queryPEPA sends a query to the PEPA AI API and returns the response.
func (b *Bot) queryPEPA(query string) string {
	body := map[string]interface{}{
		"message":      query,
		"enable_tools": true,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", b.config.APIURL+"/api/v1/ai/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Sprintf("Error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.config.PEPAToken)

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Sprintf("AI request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("AI error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Sprintf("Parse error: %v", err)
	}

	// Truncate for chat (Telegram limit is 4096 chars)
	if len(result.Response) > 4000 {
		// Find the last valid UTF-8 boundary before 4000 bytes
		cut := 4000
		for cut > 0 && !utf8.RuneStart(result.Response[cut]) {
			cut--
		}
		result.Response = result.Response[:cut] + "\n\n...(truncated)"
	}

	return result.Response
}

// handleDeploy handles the /deploy command.
func (b *Bot) handleDeploy(text string) string {
	parts := strings.Fields(text)
	if len(parts) < 3 {
		return "Usage: /deploy <service> <version>\nExample: /deploy payment-api v1.2.3"
	}
	service := parts[1]
	version := parts[2]

	// Trigger deployment via PEPA API
	body := map[string]interface{}{
		"service_name": service,
		"version":      version,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", b.config.APIURL+"/api/v1/deployments", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.config.PEPAToken)

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Sprintf("Deploy request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return fmt.Sprintf("Deployment initiated: %s@%s", service, version)
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Sprintf("Deploy failed (status %d): %s", resp.StatusCode, string(respBody))
}

// handleStatus handles the /status command.
func (b *Bot) handleStatus() string {
	req, err := http.NewRequest("GET", b.config.APIURL+"/healthz", nil)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Sprintf("PEPA status: unreachable (%v)", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	return fmt.Sprintf("PEPA Status: %s (version: %s)", result.Status, result.Version)
}

// handleIncident handles the /incident command.
func (b *Bot) handleIncident(text string) string {
	query := strings.TrimPrefix(text, "/incident")
	query = strings.TrimSpace(query)
	if query == "" {
		query = "What incidents or issues are currently active on the platform?"
	}
	return b.queryPEPA(query)
}

// ── Telegram API helpers ──────────────────────────────────────────

type telegramUpdate struct {
	UpdateID int64          `json:"update_id"`
	Message  telegramMessage `json:"message"`
}

type telegramMessage struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	From      struct {
		Username string `json:"username"`
	} `json:"from"`
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
}

func (b *Bot) telegramGetUpdates(ctx context.Context, offset int64) ([]telegramUpdate, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", b.config.Token, offset)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		OK     bool             `json:"ok"`
		Result []telegramUpdate `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram API error")
	}

	return result.Result, nil
}

func (b *Bot) telegramSendMessage(ctx context.Context, chatID int64, text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.config.Token)

	body := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	return nil
}
