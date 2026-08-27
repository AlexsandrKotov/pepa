// Package observability provides Prometheus metrics for the PEPA platform.
package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metric collectors for PEPA.
type Metrics struct {
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPActiveRequests  prometheus.Gauge
	JobProcessedTotal   *prometheus.CounterVec
	JobDuration         *prometheus.HistogramVec
	PluginActionsTotal  *prometheus.CounterVec
	DBQueryDuration     *prometheus.HistogramVec
	CacheHitsTotal      *prometheus.CounterVec
	ActiveConnections   prometheus.Gauge
	DeploymentTotal     *prometheus.CounterVec
}

// NewMetrics creates and registers all Prometheus collectors.
func NewMetrics() *Metrics {
	m := &Metrics{
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pepa_http_requests_total",
				Help: "Total number of HTTP requests.",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pepa_http_request_duration_seconds",
				Help:    "HTTP request latency in seconds.",
				Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120, 300},
			},
			[]string{"method", "path", "status"},
		),
		HTTPActiveRequests: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "pepa_http_active_requests",
				Help: "Number of HTTP requests currently in flight.",
			},
		),
		JobProcessedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pepa_job_processed_total",
				Help: "Total number of background jobs processed.",
			},
			[]string{"type", "status"},
		),
		JobDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pepa_job_duration_seconds",
				Help:    "Background job processing duration in seconds.",
				Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120},
			},
			[]string{"type"},
		),
		PluginActionsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pepa_plugin_actions_total",
				Help: "Total number of plugin actions executed.",
			},
			[]string{"plugin", "action", "status"},
		),
		DBQueryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pepa_db_query_duration_seconds",
				Help:    "Database query duration in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation"},
		),
		CacheHitsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pepa_cache_hits_total",
				Help: "Total cache hits and misses.",
			},
			[]string{"cache", "result"},
		),
		ActiveConnections: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "pepa_active_connections",
				Help: "Number of active provider connections.",
			},
		),
		DeploymentTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pepa_deployment_total",
				Help: "Total number of deployments.",
			},
			[]string{"status", "target_type"},
		),
	}
	return m
}

// GinMiddleware returns a Gin middleware that records HTTP request metrics.
func (m *Metrics) GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		m.HTTPActiveRequests.Inc()
		start := time.Now()

		c.Next()

		m.HTTPActiveRequests.Dec()
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}

		m.HTTPRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		m.HTTPRequestDuration.WithLabelValues(c.Request.Method, path, status).Observe(duration)
	}
}

// Handler returns an HTTP handler for the /metrics endpoint.
func Handler() http.Handler {
	return promhttp.Handler()
}
