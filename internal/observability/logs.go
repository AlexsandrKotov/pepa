// Package observability provides OpenTelemetry tracing, metrics, and log export.
package observability

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// InitLogs initializes OpenTelemetry log export via OTLP gRPC.
// It creates a LoggerProvider that exports structured logs to SigNoz/Jaeger/Loki.
// Returns the LoggerProvider (to pass to otelslog bridge) and a shutdown function.
func InitLogs(ctx context.Context, cfg TracingConfig) (*sdklog.LoggerProvider, func(context.Context) error, error) {
	if !cfg.Enabled || cfg.OTLPEndpoint == "" {
		slog.Info("OpenTelemetry log export disabled")
		return nil, func(context.Context) error { return nil }, nil
	}

	slog.Info("initializing OpenTelemetry log export", "endpoint", cfg.OTLPEndpoint, "service", cfg.ServiceName)

	// Create OTLP gRPC log exporter
	opts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(cfg.OTLPEndpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlploggrpc.WithInsecure())
	}

	exporter, err := otlploggrpc.New(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create OTLP log exporter: %w", err)
	}

	// Create resource with service info
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String("1.0.0"),
			semconv.DeploymentEnvironmentKey.String("production"),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create log resource: %w", err)
	}

	// Create LoggerProvider with batch processor
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		sdklog.WithResource(res),
	)

	slog.Info("OpenTelemetry log export initialized", "service", cfg.ServiceName)

	return lp, lp.Shutdown, nil
}
