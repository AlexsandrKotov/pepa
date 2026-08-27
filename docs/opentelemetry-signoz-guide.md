# PEPA OpenTelemetry & SigNoz Integration Guide

## Обзор

Интеграция PEPA с OpenTelemetry и SigNoz для полной observability: traces, metrics, logs.

---

## Архитектура

```
┌─────────────────────────────────────────────────────────┐
│                   PEPA Application                       │
│  - Instrumented with OpenTelemetry SDK                  │
│  - Sends traces, metrics, logs via OTLP                │
└────────────────┬────────────────────────────────────────┘
                 │
                 │ OTLP (gRPC/HTTP)
                 │
        ┌────────▼────────┐
        │  OpenTelemetry  │
        │    Collector    │
        │  - Receives     │
        │  - Processes    │
        │  - Exports      │
        └────────┬────────┘
                 │
        ┌────────┴────────┬────────────┐
        │                 │            │
┌───────▼──────┐  ┌──────▼──────┐  ┌──▼──────┐
│    SigNoz    │  │ Prometheus  │  │  Loki   │
│  (Traces +   │  │ (Metrics)   │  │ (Logs)  │
│   Metrics +  │  │             │  │         │
│   Logs)      │  │             │  │         │
└──────────────┘  └─────────────┘  └─────────┘
        │
        │
┌───────▼──────────────────────────────────────┐
│           PEPA Observability UI              │
│  - Query SigNoz API                         │
│  - Display traces, metrics, logs            │
│  - Service map visualization                │
│  - Flame graphs                             │
└──────────────────────────────────────────────┘
```

---

## OpenTelemetry Integration

### 1. Instrumentation

#### Go Application
```go
// cmd/api-server/main.go
import (
    "context"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

func initOpenTelemetry(ctx context.Context) (*sdktrace.TracerProvider, error) {
    // Create OTLP exporter
    exporter, err := otlptracegrpc.New(ctx,
        otlptracegrpc.WithEndpoint("otel-collector:4317"),
        otlptracegrpc.WithInsecure(),
    )
    if err != nil {
        return nil, err
    }

    // Create resource
    res, err := resource.New(ctx,
        resource.WithAttributes(
            semconv.ServiceNameKey.String("pepa-api"),
            semconv.ServiceVersionKey.String("1.0.0"),
            semconv.DeploymentEnvironmentKey.String("production"),
        ),
    )
    if err != nil {
        return nil, err
    }

    // Create tracer provider
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(res),
        sdktrace.WithSampler(sdktrace.AlwaysSample()),
    )

    otel.SetTracerProvider(tp)
    return tp, nil
}

func main() {
    ctx := context.Background()
    
    // Initialize OpenTelemetry
    tp, err := initOpenTelemetry(ctx)
    if err != nil {
        log.Fatal(err)
    }
    defer tp.Shutdown(ctx)
    
    // Start server
    // ...
}
```

#### Instrument HTTP Handlers
```go
// internal/api/rest/middleware.go
import (
    "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func SetupMiddleware(r *gin.Engine) {
    // OpenTelemetry middleware
    r.Use(otelgin.Middleware("pepa-api"))
    
    // Other middleware
    // ...
}
```

#### Instrument Database Calls
```go
// internal/repository/postgres.go
import (
    "go.opentelemetry.io/contrib/instrumentation/github.com/jackc/pgx/v5/otelpgx"
)

func NewPostgresRepository(connString string) (*PostgresRepository, error) {
    // Instrument pgx with OpenTelemetry
    conn, err := pgx.ConnectConfig(ctx, config,
        pgx.WithTracer(otelpgx.NewTracer()),
    )
    if err != nil {
        return nil, err
    }
    
    return &PostgresRepository{conn: conn}, nil
}
```

#### Create Custom Spans
```go
// internal/service/service.go
import (
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/trace"
)

func (s *ServiceService) Create(ctx context.Context, req CreateRequest) (*Service, error) {
    tracer := otel.Tracer("pepa-api")
    
    // Create span
    ctx, span := tracer.Start(ctx, "ServiceService.Create",
        trace.WithAttributes(
            attribute.String("service.name", req.Name),
            attribute.String("service.type", req.Type),
        ),
    )
    defer span.End()
    
    // Business logic
    service, err := s.repo.Create(ctx, req)
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return nil, err
    }
    
    span.SetStatus(codes.Ok, "")
    return service, nil
}
```

### 2. Metrics Instrumentation

```go
// internal/observability/metrics.go
import (
    "go.opentelemetry.io/otel/metric"
)

type Metrics struct {
    RequestCounter metric.Int64Counter
    RequestLatency metric.Float64Histogram
    ActiveRequests metric.Int64UpDownCounter
}

func InitMetrics() (*Metrics, error) {
    meter := otel.Meter("pepa-api")
    
    requestCounter, err := meter.Int64Counter(
        "http.requests.total",
        metric.WithDescription("Total number of HTTP requests"),
    )
    if err != nil {
        return nil, err
    }
    
    requestLatency, err := meter.Float64Histogram(
        "http.request.duration",
        metric.WithDescription("HTTP request duration in seconds"),
        metric.WithUnit("s"),
    )
    if err != nil {
        return nil, err
    }
    
    activeRequests, err := meter.Int64UpDownCounter(
        "http.requests.active",
        metric.WithDescription("Number of active HTTP requests"),
    )
    if err != nil {
        return nil, err
    }
    
    return &Metrics{
        RequestCounter: requestCounter,
        RequestLatency: requestLatency,
        ActiveRequests: activeRequests,
    }, nil
}

func (m *Metrics) RecordRequest(ctx context.Context, method, path string, status int, duration time.Duration) {
    attrs := metric.WithAttributes(
        attribute.String("http.method", method),
        attribute.String("http.path", path),
        attribute.Int("http.status", status),
    )
    
    m.RequestCounter.Add(ctx, 1, attrs)
    m.RequestLatency.Record(ctx, duration.Seconds(), attrs)
}
```

### 3. Logging with OpenTelemetry

```go
// internal/logging/logger.go
import (
    "go.opentelemetry.io/otel/trace"
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

type OTelLogger struct {
    logger *zap.Logger
}

func (l *OTelLogger) Info(ctx context.Context, msg string, fields ...zap.Field) {
    // Extract trace context
    span := trace.SpanFromContext(ctx)
    traceID := span.SpanContext().TraceID().String()
    spanID := span.SpanContext().SpanID().String()
    
    // Add trace context to log
    fields = append(fields,
        zap.String("trace_id", traceID),
        zap.String("span_id", spanID),
    )
    
    l.logger.Info(msg, fields...)
}
```

---

## OpenTelemetry Collector Configuration

### Basic Configuration
```yaml
# otel-collector-config.yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    timeout: 1s
    send_batch_size: 1024
  
  memory_limiter:
    check_interval: 1s
    limit_mib: 512
    spike_limit_mib: 128
  
  attributes:
    actions:
      - key: deployment.environment
        value: production
        action: upsert

exporters:
  otlp/sigNoz:
    endpoint: signoz-otel:4317
    tls:
      insecure: true
  
  prometheus:
    endpoint: 0.0.0.0:8889
    namespace: pepa
  
  loki:
    endpoint: http://loki:3100/loki/api/v1/push
    labels:
      attributes:
        service.name: "service"
        deployment.environment: "env"

extensions:
  health_check:
    endpoint: 0.0.0.0:13133
  
  zpages:
    endpoint: 0.0.0.0:55679

service:
  extensions: [health_check, zpages]
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch, attributes]
      exporters: [otlp/sigNoz]
    metrics:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [prometheus]
    logs:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [loki]
```

---

## SigNoz Integration

### 1. SigNoz API Client

```go
// internal/observability/signoz.go
package observability

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type SigNozClient struct {
    BaseURL    string
    AuthToken  string
    HTTPClient *http.Client
}

func NewSigNozClient(baseURL, authToken string) *SigNozClient {
    return &SigNozClient{
        BaseURL:   baseURL,
        AuthToken: authToken,
        HTTPClient: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
}

// Query traces
func (c *SigNozClient) QueryTraces(ctx context.Context, query TraceQuery) ([]Trace, error) {
    url := fmt.Sprintf("%s/api/v3/traces", c.BaseURL)
    
    reqBody := map[string]interface{}{
        "start": query.Start.UnixNano(),
        "end":   query.End.UnixNano(),
        "filters": map[string]interface{}{
            "items": query.Filters,
        },
        "limit": query.Limit,
    }
    
    body, _ := json.Marshal(reqBody)
    req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("SIGNOZ-API-KEY", c.AuthToken)
    
    resp, err := c.HTTPClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("SigNoz API error: %d", resp.StatusCode)
    }
    
    var result SigNozTraceResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    
    return convertSigNozTraces(result), nil
}

// Query metrics
func (c *SigNozClient) QueryMetrics(ctx context.Context, query MetricQuery) ([]Metric, error) {
    url := fmt.Sprintf("%s/api/v4/metrics", c.BaseURL)
    
    reqBody := map[string]interface{}{
        "compositeQuery": map[string]interface{}{
            "queryType": "builder",
            "builderQueries": map[string]interface{}{
                "A": map[string]interface{}{
                    "dataSource":          "metrics",
                    "aggregateOperator":   query.Aggregate,
                    "aggregateAttribute":  query.Metric,
                    "filters":             query.Filters,
                },
            },
        },
        "start": query.Start.UnixMilli(),
        "end":   query.End.UnixMilli(),
        "step":  query.Step,
    }
    
    body, _ := json.Marshal(reqBody)
    req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("SIGNOZ-API-KEY", c.AuthToken)
    
    resp, err := c.HTTPClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result SigNozMetricResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    
    return convertSigNozMetrics(result), nil
}

// Get service map
func (c *SigNozClient) GetServiceMap(ctx context.Context, start, end time.Time) (*ServiceMap, error) {
    url := fmt.Sprintf("%s/api/v1/services", c.BaseURL)
    
    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
    req.Header.Set("SIGNOZ-API-KEY", c.AuthToken)
    
    q := req.URL.Query()
    q.Add("start", fmt.Sprintf("%d", start.UnixNano()))
    q.Add("end", fmt.Sprintf("%d", end.UnixNano()))
    req.URL.RawQuery = q.Encode()
    
    resp, err := c.HTTPClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result SigNozServiceMapResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    
    return convertSigNozServiceMap(result), nil
}
```

### 2. Frontend Integration

```typescript
// frontend/lib/signoz.ts
export interface SigNozTrace {
  traceID: string;
  serviceName: string;
  operationName: string;
  duration: number;
  startTime: number;
  spans: SigNozSpan[];
}

export interface SigNozSpan {
  spanID: string;
  parentSpanID: string;
  operationName: string;
  serviceName: string;
  duration: number;
  startTime: number;
  tags: Record<string, string>;
}

export async function queryTraces(query: TraceQuery): Promise<SigNozTrace[]> {
  const response = await fetch('/api/v1/observability/traces', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(query),
  });
  
  if (!response.ok) {
    throw new Error('Failed to query traces');
  }
  
  return response.json();
}

export async function getServiceMap(start: Date, end: Date): Promise<ServiceMap> {
  const response = await fetch('/api/v1/observability/service-map', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ start, end }),
  });
  
  if (!response.ok) {
    throw new Error('Failed to get service map');
  }
  
  return response.json();
}
```

### 3. Trace Viewer Component

```typescript
// frontend/components/SigNozTraceViewer.tsx
import { useState } from 'react';
import { SigNozTrace, SigNozSpan } from '@/lib/signoz';

interface TraceViewerProps {
  trace: SigNozTrace;
}

export function SigNozTraceViewer({ trace }: TraceViewerProps) {
  const [selectedSpan, setSelectedSpan] = useState<SigNozSpan | null>(null);
  
  return (
    <div className="trace-viewer">
      <div className="trace-header">
        <h3>Trace: {trace.traceID}</h3>
        <div className="trace-info">
          <span>Service: {trace.serviceName}</span>
          <span>Operation: {trace.operationName}</span>
          <span>Duration: {trace.duration}ms</span>
        </div>
      </div>
      
      <div className="trace-timeline">
        {trace.spans.map(span => (
          <div
            key={span.spanID}
            className={`span ${selectedSpan?.spanID === span.spanID ? 'selected' : ''}`}
            onClick={() => setSelectedSpan(span)}
          >
            <div className="span-info">
              <span className="service-name">{span.serviceName}</span>
              <span className="operation-name">{span.operationName}</span>
            </div>
            <div
              className="span-bar"
              style={{
                width: `${(span.duration / trace.duration) * 100}%`,
                marginLeft: `${((span.startTime - trace.startTime) / trace.duration) * 100}%`,
              }}
            />
          </div>
        ))}
      </div>
      
      {selectedSpan && (
        <div className="span-details">
          <h4>Span Details</h4>
          <div className="span-tags">
            {Object.entries(selectedSpan.tags).map(([key, value]) => (
              <div key={key} className="tag">
                <span className="key">{key}:</span>
                <span className="value">{value}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
```

### 4. Service Map Component

```typescript
// frontend/components/SigNozServiceMap.tsx
import { useEffect, useRef } from 'react';
import { ServiceMap } from '@/lib/signoz';
import * as d3 from 'd3';

interface ServiceMapViewerProps {
  serviceMap: ServiceMap;
}

export function SigNozServiceMap({ serviceMap }: ServiceMapViewerProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  
  useEffect(() => {
    if (!svgRef.current) return;
    
    const svg = d3.select(svgRef.current);
    svg.selectAll('*').remove();
    
    const width = 800;
    const height = 600;
    
    // Create force simulation
    const simulation = d3.forceSimulation(serviceMap.nodes)
      .force('link', d3.forceLink(serviceMap.links).id((d: any) => d.id))
      .force('charge', d3.forceManyBody().strength(-300))
      .force('center', d3.forceCenter(width / 2, height / 2));
    
    // Create links
    const link = svg.append('g')
      .selectAll('line')
      .data(serviceMap.links)
      .enter().append('line')
      .attr('stroke', '#999')
      .attr('stroke-opacity', 0.6);
    
    // Create nodes
    const node = svg.append('g')
      .selectAll('circle')
      .data(serviceMap.nodes)
      .enter().append('circle')
      .attr('r', 10)
      .attr('fill', (d: any) => d.errorRate > 0.1 ? '#f00' : '#0f0');
    
    // Add labels
    const label = svg.append('g')
      .selectAll('text')
      .data(serviceMap.nodes)
      .enter().append('text')
      .text((d: any) => d.name)
      .attr('font-size', '12px');
    
    // Update positions on tick
    simulation.on('tick', () => {
      link
        .attr('x1', (d: any) => d.source.x)
        .attr('y1', (d: any) => d.source.y)
        .attr('x2', (d: any) => d.target.x)
        .attr('y2', (d: any) => d.target.y);
      
      node
        .attr('cx', (d: any) => d.x)
        .attr('cy', (d: any) => d.y);
      
      label
        .attr('x', (d: any) => d.x)
        .attr('y', (d: any) => d.y);
    });
  }, [serviceMap]);
  
  return <svg ref={svgRef} width={800} height={600} />;
}
```

---

## Deployment

### Docker Compose

```yaml
# docker-compose.yaml
services:
  otel-collector:
    image: otel/opentelemetry-collector-contrib:latest
    ports:
      - "4317:4317"   # OTLP gRPC
      - "4318:4318"   # OTLP HTTP
      - "13133:13133" # Health check
    volumes:
      - ./otel-collector-config.yaml:/etc/otel-collector-config.yaml
    command: ["--config", "/etc/otel-collector-config.yaml"]
    depends_on:
      - signoz-otel
  
  signoz-otel:
    image: signoz/signoz:latest
    ports:
      - "8080:8080"   # SigNoz UI
      - "4317:4317"   # OTLP gRPC
    environment:
      - SIGNOZ_ALERTS_DEFAULT_ENABLED=true
    volumes:
      - signoz-data:/var/lib/signoz
  
  signoz-clickhouse:
    image: clickhouse/clickhouse-server:latest
    volumes:
      - clickhouse-data:/var/lib/clickhouse

volumes:
  signoz-data:
  clickhouse-data:
```

---

## Configuration

### PEPA Configuration

```yaml
# config/observability.yaml
observability:
  opentelemetry:
    enabled: true
    endpoint: "otel-collector:4317"
    insecure: true
    service_name: "pepa-api"
    sampling_rate: 1.0
  
  signoz:
    enabled: true
    base_url: "http://signoz-otel:8080"
    auth_token: "${SIGNOZ_API_TOKEN}"
  
  prometheus:
    enabled: true
    url: "http://prometheus:9090"
  
  loki:
    enabled: true
    url: "http://loki:3100"
```

---

## Use Cases

### 1. Distributed Tracing
```yaml
# Сценарий:
1. Пользователь делает запрос к API
2. PEPA создает span для HTTP request
3. API вызывает database
4. Создается span для database query
5. API вызывает внешний сервис
6. Создается span для external call
7. Все spans отправляются в OpenTelemetry Collector
8. Collector экспортирует в SigNoz
9. PEPA UI отображает trace с flame graph

# Результат:
- Полная visibility в request flow
- Identification bottlenecks
- Error tracking
- Performance analysis
```

### 2. Service Map
```yaml
# Сценарий:
1. SigNoz анализирует все traces
2. Строит dependency graph между сервисами
3. PEPA UI отображает service map
4. Показывает latency, error rate, throughput

# Результат:
- Visual representation архитектуры
- Quick identification проблем
- Impact analysis
- Dependency tracking
```

### 3. Metrics & Alerts
```yaml
# Сценарий:
1. PEPA отправляет метрики через OpenTelemetry
2. Метрики сохраняются в SigNoz/Prometheus
3. Настраиваются alerts
4. При нарушении threshold - notification

# Результат:
- Real-time monitoring
- Proactive issue detection
- Performance tracking
- SLA compliance
```

---

## Security

### API Key Management
```yaml
# Храните SigNoz API key в Vault:
vault:
  path: secret/data/signoz
  key: api_token

# PEPA получает key из Vault при старте
```

### Network Security
```yaml
# TLS для OpenTelemetry:
otel:
  tls:
    enabled: true
    cert_file: /path/to/cert.pem
    key_file: /path/to/key.pem
    ca_file: /path/to/ca.pem
```

---

## Troubleshooting

### Traces not appearing
```bash
# Проверьте:
1. OpenTelemetry Collector запущен
2. PEPA отправляет traces
3. SigNoz принимает traces
4. Network connectivity

# Логи:
$ docker logs otel-collector
$ docker logs signoz-otel
```

### High memory usage
```yaml
# Настройте memory limiter:
processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: 512
    spike_limit_mib: 128
```

---

**Создано**: 2026-08-11
**Версия**: 1.0
**Статус**: ✅ Готово к реализации
