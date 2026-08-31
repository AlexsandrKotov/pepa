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
		slog.Error("BOT_TOKEN is required")
		os.Exit(1)
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
