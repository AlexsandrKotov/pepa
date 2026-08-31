package rest

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	// TraceIDKey is the context key for storing trace ID.
	TraceIDKey contextKey = "trace_id"
	// SpanIDKey is the context key for storing span ID.
	SpanIDKey contextKey = "span_id"
)

// correlationMiddleware extracts or generates a trace ID for request correlation.
// If OpenTelemetry is enabled, it extracts the trace ID from the current span.
// Otherwise, it generates a new UUID for correlation.
// The trace ID is set in the request context and response header.
func correlationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var traceID string

		// Try to get trace ID from OpenTelemetry span
		span := trace.SpanFromContext(c.Request.Context())
		if span.SpanContext().IsValid() {
			traceID = span.SpanContext().TraceID().String()
		}

		// If no valid trace ID, generate a correlation ID
		if traceID == "" || traceID == "00000000000000000000000000000000" {
			traceID = uuid.New().String()
		}

		// Set trace ID in request context
		ctx := context.WithValue(c.Request.Context(), TraceIDKey, traceID)
		c.Request = c.Request.WithContext(ctx)

		// Set trace ID in response header for client correlation
		c.Header("X-Trace-ID", traceID)

		// Also set in Gin context for easy access in handlers
		c.Set("trace_id", traceID)

		c.Next()
	}
}

// GetTraceID extracts the trace ID from the Gin context.
func GetTraceID(c *gin.Context) string {
	if traceID, exists := c.Get("trace_id"); exists {
		return traceID.(string)
	}
	return ""
}

// GetTraceIDFromContext extracts the trace ID from a standard context.
func GetTraceIDFromContext(ctx context.Context) string {
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}
	return ""
}
