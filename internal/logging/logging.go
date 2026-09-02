// Package logging provides structured logging initialization for PEPA.
//
// In production, logs are emitted as JSON to stdout (suitable for log
// aggregators like Loki, ELK, CloudWatch). In development, human-readable
// text format is used.
//
// Supports multiple output destinations:
// - stdout (always enabled)
// - syslog (optional, for centralized logging)
// - OTLP (optional, for OpenTelemetry log export)
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"log/syslog"
	"os"
	"strings"
	"sync"
)

// activeCfg stores the current logging configuration for runtime updates.
var (
	activeCfg Config
	activeMu  sync.RWMutex
)

// Config holds logging configuration.
type Config struct {
	Environment string
	Level       string
	Syslog      SyslogConfig
}

// SyslogConfig holds syslog forwarding settings.
type SyslogConfig struct {
	Enabled  bool
	Network  string // "udp", "tcp"
	Address  string // "syslog-server:514"
	Tag      string // "pepa"
	Facility string // "local0"-"local7"
}

// Init configures the default slog logger based on environment and level.
// Call this once at process startup, before any other logging.
//
//	env:   "production" → JSON handler; anything else → text handler
//	level: "debug", "info" (default), "warn", "error"
func Init(env, level string) {
	InitWithConfig(Config{
		Environment: env,
		Level:       level,
	})
}

// InitWithConfig configures logging with full configuration including syslog.
func InitWithConfig(cfg Config) {
	activeMu.Lock()
	activeCfg = cfg
	activeMu.Unlock()
	applyConfig(cfg)
}

// SetLevel updates the log level at runtime without affecting syslog configuration.
func SetLevel(level string) {
	activeMu.Lock()
	activeCfg.Level = level
	cfg := activeCfg
	activeMu.Unlock()
	applyConfig(cfg)
}

// applyConfig is the internal implementation that configures the logger.
func applyConfig(cfg Config) {
	var lvl slog.Level
	switch strings.ToLower(cfg.Level) {
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

	// Build list of output writers
	var writers []io.Writer
	writers = append(writers, os.Stdout)

	// Add syslog if enabled
	if cfg.Syslog.Enabled && cfg.Syslog.Address != "" {
		if syslogWriter, err := setupSyslog(cfg.Syslog); err != nil {
			slog.Warn("failed to setup syslog, falling back to stdout only", "error", err)
		} else {
			writers = append(writers, syslogWriter)
		}
	}

	// Create multi-writer if needed
	var output io.Writer
	if len(writers) == 1 {
		output = writers[0]
	} else {
		output = io.MultiWriter(writers...)
	}

	var handler slog.Handler
	if strings.EqualFold(cfg.Environment, "production") {
		handler = slog.NewJSONHandler(output, opts)
	} else {
		handler = slog.NewTextHandler(output, opts)
	}

	slog.SetDefault(slog.New(handler))
}

// setupSyslog creates a syslog writer.
func setupSyslog(cfg SyslogConfig) (io.Writer, error) {
	// Parse facility
	facility := parseFacility(cfg.Facility)

	// Connect to syslog server
	writer, err := syslog.Dial(cfg.Network, cfg.Address, facility|syslog.LOG_INFO, cfg.Tag)
	if err != nil {
		return nil, fmt.Errorf("dial syslog server: %w", err)
	}

	return writer, nil
}

// parseFacility converts facility string to syslog.Priority.
func parseFacility(facility string) syslog.Priority {
	switch strings.ToLower(facility) {
	case "local0":
		return syslog.LOG_LOCAL0
	case "local1":
		return syslog.LOG_LOCAL1
	case "local2":
		return syslog.LOG_LOCAL2
	case "local3":
		return syslog.LOG_LOCAL3
	case "local4":
		return syslog.LOG_LOCAL4
	case "local5":
		return syslog.LOG_LOCAL5
	case "local6":
		return syslog.LOG_LOCAL6
	case "local7":
		return syslog.LOG_LOCAL7
	case "user":
		return syslog.LOG_USER
	case "daemon":
		return syslog.LOG_DAEMON
	default:
		return syslog.LOG_LOCAL0
	}
}
