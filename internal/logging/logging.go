// Package logging provides structured logging initialization for PEPA.
//
// In production, logs are emitted as JSON to stdout (suitable for log
// aggregators like Loki, ELK, CloudWatch). In development, human-readable
// text format is used.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Init configures the default slog logger based on environment and level.
// Call this once at process startup, before any other logging.
//
//	env:   "production" → JSON handler; anything else → text handler
//	level: "debug", "info" (default), "warn", "error"
func Init(env, level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if strings.EqualFold(env, "production") {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}
