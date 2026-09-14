package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg := BotConfig{
		Platform:  getEnv("BOT_PLATFORM", "telegram"),
		Token:     os.Getenv("BOT_TOKEN"),
		APIURL:    getEnv("PEPA_API_URL", "http://localhost:8080"),
		PEPAToken: os.Getenv("PEPA_TOKEN"),
		ChannelID: os.Getenv("BOT_CHANNEL_ID"),
	}

	if cfg.Token == "" {
		// Degrade gracefully: the bot is an optional notification channel, not a
		// core platform component.  Exiting with code 0 keeps container orchestrators
		// from restart-loops when the bot is deployed without credentials (e.g. a
		// dev stack that does not need Telegram notifications).
		slog.Warn("BOT_TOKEN is not set — ai_bot will exit without starting. " +
			"Set BOT_TOKEN to enable the notification bot.")
		os.Exit(0)
	}

	bot := NewBot(cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	slog.Info("Starting AI bot", "platform", cfg.Platform)
	if err := bot.Start(ctx); err != nil && err != context.Canceled {
		slog.Error("Bot failed", "error", err)
		os.Exit(1)
	}
	slog.Info("AI bot stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
