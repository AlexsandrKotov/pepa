// Package observability provides OpenTelemetry tracing, metrics, and log export for PEPA.
package observability

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// activeShutdown holds the shutdown functions for the currently active OTel providers.
// Protected by activeMu for hot-reload safety.
var (
	activeMu      sync.Mutex
	activeShutdown []func(context.Context) error
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

// InitOTel initializes all OpenTelemetry signals (traces, metrics, logs) and
// installs an slog handler that exports application logs to the OTLP backend.
// Returns a combined shutdown function.
func InitOTel(ctx context.Context, cfg TracingConfig) (func(context.Context) error, error) {
	if !cfg.Enabled || cfg.OTLPEndpoint == "" {
		slog.Info("OpenTelemetry disabled")
		return func(context.Context) error { return nil }, nil
	}

	slog.Info("initializing OpenTelemetry (traces + metrics + logs)",
		"endpoint", cfg.OTLPEndpoint, "service", cfg.ServiceName, "sampling", cfg.SamplingRate)

	var shutdowns []func(context.Context) error

	// 1. Traces
	traceShutdown, err := InitTracing(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("init tracing: %w", err)
	}
	shutdowns = append(shutdowns, traceShutdown)

	// 2. Metrics
	metricShutdown, err := InitMetrics(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("init metrics: %w", err)
	}
	shutdowns = append(shutdowns, metricShutdown)

	// 3. Logs
	logProvider, logShutdown, err := InitLogs(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("init logs: %w", err)
	}
	shutdowns = append(shutdowns, logShutdown)

	// 4. Install slog handler that exports to OTLP alongside stdout
	installOTelSlogHandler(cfg.ServiceName, logProvider)

	// Store for hot-reload
	activeMu.Lock()
	activeShutdown = shutdowns
	activeMu.Unlock()

	slog.Info("OpenTelemetry fully initialized (traces + metrics + logs)", "service", cfg.ServiceName)

	combined := func(ctx context.Context) error {
		activeMu.Lock()
		sds := activeShutdown
		activeMu.Unlock()
		var firstErr error
		for i := len(sds) - 1; i >= 0; i-- {
			if err := sds[i](ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
	return combined, nil
}

// ReinitOTel shuts down the current OTel providers and reinitializes with new config.
// Used when observability settings are updated via the UI.
func ReinitOTel(ctx context.Context, cfg TracingConfig) (func(context.Context) error, error) {
	// Shutdown existing providers
	activeMu.Lock()
	old := activeShutdown
	activeMu.Unlock()
	for i := len(old) - 1; i >= 0; i-- {
		_ = old[i](ctx)
	}

	return InitOTel(ctx, cfg)
}

// installOTelSlogHandler adds an OTel slog handler to the default logger.
// This exports all slog output (Info, Warn, Error, Debug) to the OTLP backend
// as OpenTelemetry log records, enabling full log visibility in SigNoz.
func installOTelSlogHandler(serviceName string, logProvider *sdklog.LoggerProvider) {
	// Create an OTel slog bridge that sends logs to the LoggerProvider
	var otelHandler slog.Handler
	if logProvider != nil {
		otelHandler = otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(logProvider))
	} else {
		otelHandler = otelslog.NewHandler(serviceName)
	}

	// Wrap the default handler to fan-out to both stdout and OTLP
	existing := slog.Default().Handler()
	slog.SetDefault(slog.New(&fanoutHandler{
		handlers: []slog.Handler{existing, otelHandler},
	}))
}

// fanoutHandler writes slog records to multiple handlers (stdout + OTLP).
// The first handler is always the stdout handler; remaining handlers are OTel exporters.
type fanoutHandler struct {
	handlers []slog.Handler
}

func (h *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, hd := range h.handlers {
		if hd.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	for i, hd := range h.handlers {
		if !hd.Enabled(ctx, r.Level) {
			continue
		}
		if i == 0 {
			// stdout handler — pass through as-is
			_ = hd.Handle(ctx, r)
		} else {
			// OTel handler — clone the record and ensure severity + trace context
			// are explicitly set, because slog.With() / WithAttrs() may lose them.
			var rec slog.Record
			rec.Time = r.Time
			rec.Message = r.Message
			rec.Level = r.Level // explicit severity

			// Try to extract span from context for trace correlation
			span := trace.SpanFromContext(ctx)
			if span.SpanContext().IsValid() {
				sc := span.SpanContext()
				rec.AddAttrs(
					slog.String("trace_id", sc.TraceID().String()),
					slog.String("span_id", sc.SpanID().String()),
				)
			}

			// Copy original attributes
			r.Attrs(func(a slog.Attr) bool {
				rec.AddAttrs(a)
				return true
			})

			_ = hd.Handle(ctx, rec)
		}
	}
	return nil
}

func (h *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, hd := range h.handlers {
		handlers[i] = hd.WithAttrs(attrs)
	}
	return &fanoutHandler{handlers: handlers}
}

func (h *fanoutHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, hd := range h.handlers {
		handlers[i] = hd.WithGroup(name)
	}
	return &fanoutHandler{handlers: handlers}
}

// Ensure fanoutHandler implements slog.Handler at compile time.
var _ slog.Handler = (*fanoutHandler)(nil)
