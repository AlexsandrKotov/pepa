# PEPA Observability Guide

This guide explains how to configure and use PEPA's observability features for monitoring, tracing, and logging.

## Overview

PEPA provides comprehensive observability through:

- **Distributed Tracing** — OpenTelemetry integration for request tracing across services
- **Metrics** — Prometheus metrics for HTTP, jobs, plugins, database, and deployments
- **Logging** — Structured logging with syslog forwarding support
- **Correlation** — Trace ID correlation across logs, traces, and metrics

## Configuration

### Environment Variables

#### OpenTelemetry (Tracing & Metrics)

```bash
# Enable OpenTelemetry
OTEL_ENABLED=true

# OTLP endpoint (OpenTelemetry Collector)
OTEL_ENDPOINT=otel-collector:4317

# Service name for identification
OTEL_SERVICE_NAME=pepa-api

# Sampling rate (0.0 to 1.0, where 1.0 = 100%)
OTEL_SAMPLING_RATE=1.0

# Use insecure connection (no TLS)
OTEL_INSECURE=true
```

#### Syslog Forwarding

```bash
# Enable syslog forwarding
SYSLOG_ENABLED=true

# Syslog server address
SYSLOG_ADDRESS=syslog:514

# Transport protocol (udp or tcp)
SYSLOG_NETWORK=udp

# Message tag
SYSLOG_TAG=pepa

# Syslog facility (local0-local7, user, daemon)
SYSLOG_FACILITY=local0
```

### Docker Compose

Enable the observability stack with the `observability` profile:

```bash
docker compose --profile observability up -d
```

This starts:
- **OpenTelemetry Collector** — Receives and exports traces, metrics, and logs
- **Jaeger** — Distributed tracing backend with UI at http://localhost:16686
- **Loki** — Log aggregation
- **Grafana** — Visualization at http://localhost:3001

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   PEPA Application                       │
│  - Instrumented with OpenTelemetry SDK                  │
│  - Structured logging with trace_id correlation         │
│  - Prometheus metrics endpoint (/metrics)               │
└────────────────┬────────────────────────────────────────┘
                 │
                 │ OTLP (gRPC/HTTP)
                 │
        ┌────────▼────────┐
        │  OpenTelemetry  │
        │    Collector    │
        └────────┬────────┘
                 │
        ┌────────┴────────┬────────────┐
        │                 │            │
┌───────▼──────┐  ┌──────▼──────┐  ┌──▼──────┐
│    Jaeger    │  │ Prometheus  │  │  Loki   │
│  (Traces)    │  │ (Metrics)   │  │ (Logs)  │
└──────────────┘  └─────────────┘  └─────────┘
        │                 │            │
        └────────┬────────┴────────────┘
                 │
        ┌────────▼────────┐
        │    Grafana      │
        │ (Visualization) │
        └─────────────────┘
```

## API Endpoints

### Observability API

PEPA exposes observability data through REST API:

```
GET /api/v1/observability/overview     — System health summary
GET /api/v1/observability/metrics      — Prometheus metrics in JSON
GET /api/v1/observability/logs         — Recent audit logs
GET /api/v1/observability/traces       — Recent pipeline runs as traces
GET /api/v1/observability/alerts       — Active system alerts
GET /api/v1/observability/correlate    — Correlate trace/logs/metrics by trace_id
```

### Correlation Endpoint

The `/correlate` endpoint enables full-stack observability by finding all related data for a given trace ID:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/observability/correlate?trace_id=abc123"
```

Response includes:
- Related traces (pipeline runs)
- Related logs (audit entries with trace_id)
- Current metrics snapshot

## Plugins

### Prometheus Plugin

The Prometheus plugin can now:
- Query PromQL metrics
- List alerts, rules, and targets
- **Remote write** — Push metrics to Prometheus
- **Push metrics** — Send internal PEPA metrics

Configuration:
```yaml
url: http://prometheus:9090
token: optional-bearer-token
remote_write_url: http://prometheus:9090/api/v1/write  # optional
```

### Syslog Plugin

Forward logs, events, and audit trail to syslog server:

```yaml
server: syslog:514
protocol: udp
facility: local0
tag: pepa
format: json  # json, text, or rfc5424
```

Actions:
- `send_log` — Send log message
- `send_audit` — Send audit event
- `send_event` — Send system event
- `test_connection` — Test syslog connectivity

## Trace Correlation

All requests include a `trace_id` for correlation:

1. **Request arrives** → Correlation middleware extracts/generates trace_id
2. **Trace_id set in context** → Available to all handlers
3. **Response header** → `X-Trace-ID` header sent to client
4. **Logs include trace_id** → All log entries have trace_id field
5. **OpenTelemetry span** → If enabled, trace_id from OTel span is used

Example log entry:
```json
{
  "time": "2026-08-31T10:00:00Z",
  "level": "INFO",
  "msg": "processing request",
  "trace_id": "abc123def456",
  "method": "GET",
  "path": "/api/v1/services"
}
```

## Grafana Dashboards

Access Grafana at http://localhost:3001 (default credentials: admin/admin)

Pre-configured data sources:
- **Jaeger** — Trace visualization
- **Loki** — Log querying
- **Prometheus** — Metrics (if using OTel Collector Prometheus exporter)

## Troubleshooting

### Traces not appearing

1. Check OTel Collector is running:
   ```bash
   docker compose ps otel-collector
   ```

2. Check OTel Collector logs:
   ```bash
   docker compose logs otel-collector
   ```

3. Verify endpoint connectivity:
   ```bash
   docker compose exec api-server wget -qO- http://otel-collector:13133/health
   ```

### Logs not forwarded to syslog

1. Check syslog server is reachable:
   ```bash
   docker compose exec api-server nc -zv syslog 514
   ```

2. Check syslog plugin status in PEPA UI (Settings → Plugins)

3. Test connection:
   ```bash
   curl -X POST -H "Authorization: Bearer $TOKEN" \
     -d '{"action":"test_connection"}' \
     "http://localhost:8080/api/v1/plugins/syslog/execute"
   ```

### High memory usage

Adjust OTel Collector memory limiter in `otel-collector-config.yaml`:
```yaml
processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: 400
    spike_limit_mib: 100
```

## Production Recommendations

1. **Enable TLS** for OTLP endpoint:
   ```bash
   OTEL_INSECURE=false
   ```

2. **Reduce sampling rate** for high-traffic deployments:
   ```bash
   OTEL_SAMPLING_RATE=0.1  # 10% of traces
   ```

3. **Use TCP for syslog** for reliability:
   ```bash
   SYSLOG_NETWORK=tcp
   ```

4. **Configure persistent storage** for Jaeger and Loki

5. **Set up alerts** for:
   - OTel Collector health
   - Jaeger storage usage
   - Loki ingestion rate

## Resources

- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [Jaeger Documentation](https://www.jaegertracing.io/docs/)
- [Loki Documentation](https://grafana.com/docs/loki/)
- [PEPA OpenTelemetry Guide](../docs/opentelemetry-signoz-guide.md)
