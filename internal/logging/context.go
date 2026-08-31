package logging

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	// TraceIDKey is the context key for storing trace ID.
	TraceIDKey contextKey = "trace_id"
)

// LoggerWithTrace returns a slog.Logger that includes trace_id from context.
// If OpenTelemetry is enabled and a valid span exists in the context,
// the trace_id is automatically added to all log entries.
func LoggerWithTrace(ctx context.Context) *slog.Logger {
	// Try to get trace ID from OpenTelemetry span
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		traceID := span.SpanContext().TraceID().String()
		if traceID != "00000000000000000000000000000000" {
			return slog.With("trace_id", traceID)
		}
	}

	// Try to get trace ID from context value (set by correlation middleware)
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok && traceID != "" {
		return slog.With("trace_id", traceID)
	}

	return slog.Default()
}

// LogFromContext returns a logger with trace context extracted from ctx.
// This is the recommended way to get a logger in request-scoped code.
//
// Example:
//
//	func handler(c *gin.Context) {
//	    log := logging.LogFromContext(c.Request.Context())
//	    log.Info("processing request")
//	}
func LogFromContext(ctx context.Context) *slog.Logger {
	return LoggerWithTrace(ctx)
}

// WithTraceID adds a trace_id attribute to a slog.Logger.
// Useful when you want to manually add correlation to logs.
func WithTraceID(logger *slog.Logger, traceID string) *slog.Logger {
	if traceID == "" {
		return logger
	}
	return logger.With("trace_id", traceID)
}

// ContextWithTraceID returns a new context with the trace ID set.
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// TraceIDFromContext extracts the trace ID from context.
// Returns empty string if no trace ID is found.
func TraceIDFromContext(ctx context.Context) string {
	// Try OpenTelemetry span first
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		traceID := span.SpanContext().TraceID().String()
		if traceID != "00000000000000000000000000000000" {
			return traceID
		}
	}

	// Try context value
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}

	return ""
}
