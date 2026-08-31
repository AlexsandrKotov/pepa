// Package observability provides OpenTelemetry tracing and metrics for PEPA.
package observability

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// TracingConfig holds OpenTelemetry tracing configuration.
type TracingConfig struct {
	Enabled      bool
	OTLPEndpoint string
	ServiceName  string
	SamplingRate float64
	Insecure     bool
}

// InitTracing initializes OpenTelemetry tracing with OTLP exporter.
// Returns a shutdown function that must be called on application exit.
func InitTracing(ctx context.Context, cfg TracingConfig) (func(context.Context) error, error) {
	if !cfg.Enabled || cfg.OTLPEndpoint == "" {
		slog.Info("OpenTelemetry tracing disabled")
		return func(context.Context) error { return nil }, nil
	}

	slog.Info("initializing OpenTelemetry tracing", "endpoint", cfg.OTLPEndpoint, "service", cfg.ServiceName)

	// Create OTLP gRPC exporter
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP exporter: %w", err)
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
		return nil, fmt.Errorf("create resource: %w", err)
	}

	// Configure sampler
	var sampler sdktrace.Sampler
	if cfg.SamplingRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SamplingRate <= 0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SamplingRate)
	}

	// Create TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global TracerProvider
	otel.SetTracerProvider(tp)

	// Set global propagator (for cross-service context propagation)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("OpenTelemetry tracing initialized", "service", cfg.ServiceName, "sampling_rate", cfg.SamplingRate)

	return tp.Shutdown, nil
}

// InitMetrics initializes OpenTelemetry metrics with OTLP exporter.
// Returns a shutdown function that must be called on application exit.
func InitMetrics(ctx context.Context, cfg TracingConfig) (func(context.Context) error, error) {
	if !cfg.Enabled || cfg.OTLPEndpoint == "" {
		slog.Info("OpenTelemetry metrics disabled")
		return func(context.Context) error { return nil }, nil
	}

	slog.Info("initializing OpenTelemetry metrics", "endpoint", cfg.OTLPEndpoint, "service", cfg.ServiceName)

	// Create OTLP gRPC metric exporter
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}

	// Create resource with service info
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	// Create MeterProvider
	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exporter)),
		metric.WithResource(res),
	)

	// Set global MeterProvider
	otel.SetMeterProvider(mp)

	slog.Info("OpenTelemetry metrics initialized", "service", cfg.ServiceName)

	return mp.Shutdown, nil
}

// GetTracer returns a tracer from the global TracerProvider.
func GetTracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
