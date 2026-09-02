package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/logging"
	"github.com/pepa/pepa/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
)

// registerObservabilityRoutes registers observability API endpoints.
func registerObservabilityRoutes(r *gin.RouterGroup, deps Dependencies) {
	obs := r.Group("/observability")
	{
		obs.GET("/overview", observabilityOverview(deps))
		obs.GET("/metrics", observabilityMetrics(deps))
		obs.GET("/logs", observabilityLogs(deps))
		obs.GET("/traces", observabilityTraces(deps))
		obs.GET("/dashboards", observabilityDashboards(deps))
		obs.GET("/alerts", observabilityAlerts(deps))
		obs.POST("/alerts/:id/resolve", observabilityResolveAlert(deps))
		obs.GET("/correlate", observabilityCorrelate(deps)) // Correlate trace/logs/metrics
		// Observability settings (log export configuration)
		obs.GET("/settings", observabilitySettingsGet(deps))
		obs.PUT("/settings", observabilitySettingsUpdate(deps))
		obs.POST("/settings/test-syslog", observabilityTestSyslog(deps))
		obs.POST("/settings/test-otlp", observabilityTestOTLP(deps))
		obs.POST("/debug/send-test", observabilitySendTestTelemetry(deps)) // Send test trace+metric+log to OTLP
	}
}

// observabilityOverview returns a summary of system health and metrics.
func observabilityOverview(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Gather runtime stats
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		// Count active plugins
		pluginCount := 0
		if deps.PluginMgr != nil {
			pluginCount = len(deps.PluginMgr.ListPlugins())
		}

		// Count active connections from DB
		var connectionCount int
		if deps.Repos.Connection != nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
			defer cancel()
			_ = deps.DB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM connections WHERE status = 'active'`).Scan(&connectionCount)
		}

		// Count recent deployments (last 24h)
		var recentDeployments int
		if deps.Repos.Deployment != nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
			defer cancel()
			_ = deps.DB.Pool.QueryRow(ctx, `
				SELECT COUNT(*) FROM deployments 
				WHERE created_at > NOW() - INTERVAL '24 hours'
			`).Scan(&recentDeployments)
		}

		// Count pipeline runs (last 24h)
		var pipelineRuns24h int
		_ = deps.DB.Pool.QueryRow(c.Request.Context(), `
			SELECT COUNT(*) FROM pipeline_runs 
			WHERE created_at > NOW() - INTERVAL '24 hours'
		`).Scan(&pipelineRuns24h)

		// Count active workflows
		var activeWorkflows int
		_ = deps.DB.Pool.QueryRow(c.Request.Context(), `
			SELECT COUNT(*) FROM workflows WHERE status = 'active'
		`).Scan(&activeWorkflows)

		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"system": gin.H{
				"version":         deps.Version,
				"go_version":      runtime.Version(),
				"uptime_seconds":  time.Since(startTime).Seconds(),
				"goroutines":      runtime.NumGoroutine(),
				"memory_alloc_mb": float64(m.Alloc) / 1024 / 1024,
				"memory_sys_mb":   float64(m.Sys) / 1024 / 1024,
				"gc_pause_total":  m.PauseTotalNs,
				"num_gc":          m.NumGC,
			},
			"services": gin.H{
				"plugins":            pluginCount,
				"active_connections": connectionCount,
				"active_workflows":   activeWorkflows,
			},
			"activity": gin.H{
				"deployments_24h":   recentDeployments,
				"pipeline_runs_24h": pipelineRuns24h,
			},
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// observabilityMetrics returns Prometheus metrics in JSON format for the frontend.
func observabilityMetrics(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Gather all Prometheus metrics
		metricFamilies, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to gather metrics"})
			return
		}

		var metrics []map[string]interface{}
		for _, mf := range metricFamilies {
			metric := map[string]interface{}{
				"name": mf.GetName(),
				"help": mf.GetHelp(),
				"type": mf.GetType().String(),
			}

			var values []map[string]interface{}
			for _, m := range mf.GetMetric() {
				value := map[string]interface{}{}

				// Extract labels
				labels := map[string]string{}
				for _, l := range m.GetLabel() {
					labels[l.GetName()] = l.GetValue()
				}
				value["labels"] = labels

				// Extract metric value based on type
				if m.GetCounter() != nil {
					value["value"] = m.GetCounter().GetValue()
				} else if m.GetGauge() != nil {
					value["value"] = m.GetGauge().GetValue()
				} else if m.GetHistogram() != nil {
					h := m.GetHistogram()
					value["sample_count"] = h.GetSampleCount()
					value["sample_sum"] = h.GetSampleSum()
				} else if m.GetSummary() != nil {
					s := m.GetSummary()
					value["sample_count"] = s.GetSampleCount()
					value["sample_sum"] = s.GetSampleSum()
				}

				values = append(values, value)
			}
			metric["values"] = values
			metrics = append(metrics, metric)
		}

		c.JSON(http.StatusOK, gin.H{
			"metrics": metrics,
			"total":   len(metrics),
		})
	}
}

// observabilityLogs returns recent activity for the caller's tenant.
// Application logs are not persisted in the database, so the audit trail is the
// closest persisted signal. Results are tenant-scoped and paginated with bounded,
// parameterized LIMIT/OFFSET (the previous version interpolated limit/offset and
// queried a non-existent table).
func observabilityLogs(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
		if err != nil || limit < 1 {
			limit = 100
		}
		if limit > 1000 {
			limit = 1000
		}
		offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
		if err != nil || offset < 0 {
			offset = 0
		}

		tenantID := auth.GetTenantID(c)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		rows, err := deps.DB.Pool.Query(ctx, `
			SELECT created_at, action, COALESCE(entity_type, ''), COALESCE(plugin_name, ''), COALESCE(ip_address::text, '')
			FROM audit_log
			WHERE tenant_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`, tenantID, limit, offset)
		if err != nil {
			slog.Info("observabilityLogs: query error", "error", err)
			c.JSON(http.StatusOK, gin.H{"logs": []interface{}{}, "total": 0})
			return
		}
		defer rows.Close()

		logs := []map[string]interface{}{}
		for rows.Next() {
			var ts time.Time
			var action, entityType, plugin, ip string
			if err := rows.Scan(&ts, &action, &entityType, &plugin, &ip); err != nil {
				continue
			}
			logs = append(logs, map[string]interface{}{
				"timestamp":   ts.UTC().Format(time.RFC3339),
				"action":      action,
				"entity_type": entityType,
				"plugin":      plugin,
				"ip":          ip,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"logs":  logs,
			"total": len(logs),
		})
	}
}

// observabilityTraces returns recent trace/span data.
func observabilityTraces(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := c.DefaultQuery("limit", "50")

		// Query recent pipeline runs as traces (each run is a "trace")
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		rows, err := deps.DB.Pool.Query(ctx, `
			SELECT 
				pr.id::text as trace_id,
				'pipeline' as service,
				COALESCE(ps.name, 'unknown') as operation,
				EXTRACT(EPOCH FROM (COALESCE(pr.finished_at, NOW()) - pr.created_at)) * 1000 as duration_ms,
				COALESCE(pr.jobs_completed, 0) + COALESCE(pr.jobs_failed, 0) as spans,
				pr.status,
				pr.created_at as timestamp
			FROM pipeline_runs pr
			LEFT JOIN pipeline_sources ps ON pr.source_id = ps.id
			ORDER BY pr.created_at DESC
			LIMIT $1
		`, limit)
		if err != nil {
			slog.Info("observabilityTraces: query error", "error", err)
			c.JSON(http.StatusOK, gin.H{"traces": []interface{}{}, "total": 0})
			return
		}
		defer rows.Close()

		var traces []map[string]interface{}
		for rows.Next() {
			var traceID, service, operation, status string
			var durationMs float64
			var spans int
			var timestamp time.Time

			if err := rows.Scan(&traceID, &service, &operation, &durationMs, &spans, &status, &timestamp); err != nil {
				continue
			}

			traces = append(traces, map[string]interface{}{
				"trace_id":    traceID,
				"service":     service,
				"operation":   operation,
				"duration_ms": durationMs,
				"spans":       spans,
				"status":      status,
				"timestamp":   timestamp.UTC().Format(time.RFC3339),
			})
		}

		if traces == nil {
			traces = []map[string]interface{}{}
		}

		c.JSON(http.StatusOK, gin.H{
			"traces": traces,
			"total":  len(traces),
		})
	}
}

// observabilityDashboards returns available dashboard configurations.
func observabilityDashboards(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Return built-in dashboards
		dashboards := []map[string]interface{}{
			{
				"id":          "overview",
				"name":        "System Overview",
				"description": "High-level system health and activity",
				"type":        "overview",
			},
			{
				"id":          "pipelines",
				"name":        "Pipeline Execution",
				"description": "Pipeline run metrics and status",
				"type":        "pipelines",
			},
			{
				"id":          "deployments",
				"name":        "Deployments",
				"description": "Deployment success rate and duration",
				"type":        "deployments",
			},
			{
				"id":          "infrastructure",
				"name":        "Infrastructure",
				"description": "Resource utilization and connection health",
				"type":        "infrastructure",
			},
		}

		c.JSON(http.StatusOK, gin.H{
			"dashboards": dashboards,
			"total":      len(dashboards),
		})
	}
}

// observabilityAlerts returns active alerts based on system metrics.
func observabilityAlerts(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var alerts []map[string]interface{}

		// Check for failed pipeline runs in last hour
		var failedPipelines int
		_ = deps.DB.Pool.QueryRow(c.Request.Context(), `
			SELECT COUNT(*) FROM pipeline_runs 
			WHERE status = 'failed' AND created_at > NOW() - INTERVAL '1 hour'
		`).Scan(&failedPipelines)

		if failedPipelines > 5 && !isAlertResolved("high-pipeline-failures") {
			alerts = append(alerts, map[string]interface{}{
				"id":          "high-pipeline-failures",
				"name":        "High Pipeline Failure Rate",
				"condition":   "failed_pipelines_1h > 5",
				"severity":    "warning",
				"status":      "firing",
				"service":     "pipeline-engine",
				"cluster":     "local",
				"fired_at":    time.Now().UTC().Format(time.RFC3339),
				"description": "More than 5 pipeline failures in the last hour",
			})
		}

		// Check for failed deployments
		var failedDeployments int
		_ = deps.DB.Pool.QueryRow(c.Request.Context(), `
			SELECT COUNT(*) FROM deployments 
			WHERE status = 'failed' AND created_at > NOW() - INTERVAL '1 hour'
		`).Scan(&failedDeployments)

		if failedDeployments > 3 && !isAlertResolved("high-deployment-failures") {
			alerts = append(alerts, map[string]interface{}{
				"id":          "high-deployment-failures",
				"name":        "High Deployment Failure Rate",
				"condition":   "failed_deployments_1h > 3",
				"severity":    "critical",
				"status":      "firing",
				"service":     "deployment-engine",
				"cluster":     "local",
				"fired_at":    time.Now().UTC().Format(time.RFC3339),
				"description": "More than 3 deployment failures in the last hour",
			})
		}

		// Check memory usage
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		memUsageMB := float64(m.Alloc) / 1024 / 1024

		if memUsageMB > 500 && !isAlertResolved("high-memory-usage") {
			alerts = append(alerts, map[string]interface{}{
				"id":          "high-memory-usage",
				"name":        "High Memory Usage",
				"condition":   "memory_alloc_mb > 500",
				"severity":    "warning",
				"status":      "firing",
				"service":     "api-server",
				"cluster":     "local",
				"fired_at":    time.Now().UTC().Format(time.RFC3339),
				"description": "API server memory usage exceeds 500MB",
			})
		}

		if alerts == nil {
			alerts = []map[string]interface{}{}
		}

		resolvedCount := 0
		resolvedAlerts.Range(func(key, value interface{}) bool {
			if expiry, ok := value.(time.Time); ok && time.Now().After(expiry) {
				resolvedAlerts.Delete(key)
			} else {
				resolvedCount++
			}
			return true
		})

		summary := map[string]interface{}{
			"total":    len(alerts) + resolvedCount,
			"firing":   len(alerts),
			"resolved": resolvedCount,
		}

		c.JSON(http.StatusOK, gin.H{
			"alerts":  alerts,
			"total":   len(alerts),
			"summary": summary,
		})
	}
}

// resolvedAlerts tracks manually resolved alert IDs with their expiry time.
// Alerts auto-expire after 6 hours so they can fire again if the condition persists.
var resolvedAlerts sync.Map

func isAlertResolved(id string) bool {
	val, ok := resolvedAlerts.Load(id)
	if !ok {
		return false
	}
	expiry, ok := val.(time.Time)
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		resolvedAlerts.Delete(id)
		return false
	}
	return true
}

// observabilityResolveAlert resolves a specific alert by ID.
// The resolution persists for 6 hours or until the underlying condition clears.
func observabilityResolveAlert(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		alertID := c.Param("id")

		// Store the resolved alert with a 6-hour TTL
		resolvedAlerts.Store(alertID, time.Now().Add(6*time.Hour))

		logAudit(deps, c, "resolve", "alert", alertID, nil, nil)

		c.JSON(http.StatusOK, gin.H{
			"alert": map[string]interface{}{
				"id":     alertID,
				"status": "resolved",
			},
			"message": "Alert resolved. It will re-activate in 6 hours if the condition persists.",
		})
	}
}

// observabilitySettingsGet returns the current observability configuration.
// It first tries to load persisted settings from the database, falling back to config defaults.
func observabilitySettingsGet(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start with config defaults
		result := gin.H{
			"otel_enabled":       deps.Config.Observability.Enabled,
			"otel_endpoint":      deps.Config.Observability.OTLPEndpoint,
			"otel_service_name":  deps.Config.Observability.ServiceName,
			"otel_sampling_rate": deps.Config.Observability.SamplingRate,
			"otel_insecure":      deps.Config.Observability.Insecure,
			"syslog_enabled":     deps.Config.Observability.Syslog.Enabled,
			"syslog_network":     deps.Config.Observability.Syslog.Network,
			"syslog_address":     deps.Config.Observability.Syslog.Address,
			"syslog_tag":         deps.Config.Observability.Syslog.Tag,
			"syslog_facility":    deps.Config.Observability.Syslog.Facility,
		}

		// Try to load persisted settings from DB
		if deps.Repos.Settings != nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
			defer cancel()
			if data, err := deps.Repos.Settings.Get(ctx, "observability"); err == nil {
				var saved map[string]interface{}
				if json.Unmarshal(data, &saved) == nil {
					// Override defaults with saved values
					for k, v := range saved {
						result[k] = v
					}
				}
			}
		}

		c.JSON(http.StatusOK, result)
	}
}

// observabilitySettingsUpdate updates the observability configuration at runtime.
func observabilitySettingsUpdate(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			OtelEnabled      *bool    `json:"otel_enabled"`
			OtelEndpoint     *string  `json:"otel_endpoint"`
			OtelServiceName  *string  `json:"otel_service_name"`
			OtelSamplingRate *float64 `json:"otel_sampling_rate"`
			OtelInsecure     *bool    `json:"otel_insecure"`
			SyslogEnabled    *bool    `json:"syslog_enabled"`
			SyslogNetwork    *string  `json:"syslog_network"`
			SyslogAddress    *string  `json:"syslog_address"`
			SyslogTag        *string  `json:"syslog_tag"`
			SyslogFacility   *string  `json:"syslog_facility"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Apply OTLP/SigNoz settings
		if req.OtelEnabled != nil {
			deps.Config.Observability.Enabled = *req.OtelEnabled
		}
		if req.OtelEndpoint != nil {
			deps.Config.Observability.OTLPEndpoint = *req.OtelEndpoint
		}
		if req.OtelServiceName != nil {
			deps.Config.Observability.ServiceName = *req.OtelServiceName
		}
		if req.OtelSamplingRate != nil {
			rate := *req.OtelSamplingRate
			if rate < 0 || rate > 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "sampling_rate must be between 0.0 and 1.0"})
				return
			}
			deps.Config.Observability.SamplingRate = rate
		}
		if req.OtelInsecure != nil {
			deps.Config.Observability.Insecure = *req.OtelInsecure
		}

		// Apply Syslog settings
		if req.SyslogEnabled != nil {
			deps.Config.Observability.Syslog.Enabled = *req.SyslogEnabled
		}
		if req.SyslogNetwork != nil {
			deps.Config.Observability.Syslog.Network = *req.SyslogNetwork
		}
		if req.SyslogAddress != nil {
			deps.Config.Observability.Syslog.Address = *req.SyslogAddress
		}
		if req.SyslogTag != nil {
			deps.Config.Observability.Syslog.Tag = *req.SyslogTag
		}
		if req.SyslogFacility != nil {
			deps.Config.Observability.Syslog.Facility = *req.SyslogFacility
		}

		slog.Info("observability settings updated",
			"otel_enabled", deps.Config.Observability.Enabled,
			"otel_endpoint", deps.Config.Observability.OTLPEndpoint,
			"syslog_enabled", deps.Config.Observability.Syslog.Enabled,
			"syslog_address", deps.Config.Observability.Syslog.Address,
		)

		// Persist settings to database
		if deps.Repos.Settings != nil {
			savedData, _ := json.Marshal(map[string]interface{}{
				"otel_enabled":       deps.Config.Observability.Enabled,
				"otel_endpoint":      deps.Config.Observability.OTLPEndpoint,
				"otel_service_name":  deps.Config.Observability.ServiceName,
				"otel_sampling_rate": deps.Config.Observability.SamplingRate,
				"otel_insecure":      deps.Config.Observability.Insecure,
				"syslog_enabled":     deps.Config.Observability.Syslog.Enabled,
				"syslog_network":     deps.Config.Observability.Syslog.Network,
				"syslog_address":     deps.Config.Observability.Syslog.Address,
				"syslog_tag":         deps.Config.Observability.Syslog.Tag,
				"syslog_facility":    deps.Config.Observability.Syslog.Facility,
			})
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()
			if err := deps.Repos.Settings.Set(ctx, "observability", savedData); err != nil {
				slog.Warn("failed to persist observability settings", "error", err)
			}
		}

		logAudit(deps, c, "update", "observability_settings", "", nil, req)

		// Hot-reload: reinitialize OTel providers with the new settings.
		// This makes traces, metrics, and logs start flowing immediately
		// without requiring an application restart.
		if deps.Config.Observability.Enabled && deps.Config.Observability.OTLPEndpoint != "" {
			go func() {
				reinitCtx, reinitCancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer reinitCancel()
				_, err := observability.ReinitOTel(reinitCtx, observability.TracingConfig{
					Enabled:      deps.Config.Observability.Enabled,
					OTLPEndpoint: deps.Config.Observability.OTLPEndpoint,
					ServiceName:  deps.Config.Observability.ServiceName,
					SamplingRate: deps.Config.Observability.SamplingRate,
					Insecure:     deps.Config.Observability.Insecure,
				})
				if err != nil {
					slog.Warn("failed to reinitialize OpenTelemetry", "error", err)
				} else {
					slog.Info("OpenTelemetry reinitialized (traces + metrics + logs)",
						"endpoint", deps.Config.Observability.OTLPEndpoint,
						"service", deps.Config.Observability.ServiceName)
				}
			}()
		}

		c.JSON(http.StatusOK, gin.H{"message": "observability settings updated"})
	}
}

// observabilityTestSyslog tests connectivity to the syslog server.
func observabilityTestSyslog(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Network string `json:"network"`
			Address string `json:"address"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Network == "" {
			req.Network = "udp"
		}
		// Validate network protocol to prevent SSRF via arbitrary protocols
		if req.Network != "udp" && req.Network != "tcp" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "network must be udp or tcp"})
			return
		}
		if req.Address == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "address is required"})
			return
		}

		conn, err := net.DialTimeout(req.Network, req.Address, 5*time.Second)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Connection failed: %v", err),
			})
			return
		}
		defer func() { _ = conn.Close() }()

		testMsg := "<14>PEPA syslog connection test"
		if _, err := conn.Write([]byte(testMsg)); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Write failed: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "Syslog connection test successful",
		})
	}
}

// observabilityTestOTLP tests connectivity to the OTLP endpoint (SigNoz, etc).
func observabilityTestOTLP(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Endpoint string `json:"endpoint"`
			Insecure bool   `json:"insecure"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Endpoint == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "endpoint is required"})
			return
		}

		conn, err := net.DialTimeout("tcp", req.Endpoint, 5*time.Second)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Connection failed: %v", err),
			})
			return
		}
		defer func() { _ = conn.Close() }()

		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "OTLP endpoint reachable",
		})
	}
}

// startTime tracks when the application started for uptime calculation.
var startTime = time.Now()

// observabilityCorrelate correlates traces, logs, and metrics by trace_id.
// This endpoint enables full-stack observability by finding all related data
// for a given trace ID across the system.
func observabilityCorrelate(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.Query("trace_id")
		if traceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "trace_id query parameter is required"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		result := map[string]interface{}{
			"trace_id": traceID,
		}

		// 1. Find related pipeline runs (traces)
		var traces []map[string]interface{}
		if deps.DB != nil && deps.DB.Pool != nil {
			rows, err := deps.DB.Pool.Query(ctx, `
				SELECT 
					pr.id::text as trace_id,
					'pipeline' as service,
					COALESCE(ps.name, 'unknown') as operation,
					EXTRACT(EPOCH FROM (COALESCE(pr.finished_at, NOW()) - pr.created_at)) * 1000 as duration_ms,
					COALESCE(pr.jobs_completed, 0) + COALESCE(pr.jobs_failed, 0) as spans,
					pr.status,
					pr.created_at as timestamp
				FROM pipeline_runs pr
				LEFT JOIN pipeline_sources ps ON pr.source_id = ps.id
				WHERE pr.id::text = $1
				ORDER BY pr.created_at DESC
			`, traceID)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var tID, service, operation, status string
					var durationMs float64
					var spans int
					var timestamp time.Time
					if err := rows.Scan(&tID, &service, &operation, &durationMs, &spans, &status, &timestamp); err == nil {
						traces = append(traces, map[string]interface{}{
							"trace_id":    tID,
							"service":     service,
							"operation":   operation,
							"duration_ms": durationMs,
							"spans":       spans,
							"status":      status,
							"timestamp":   timestamp.UTC().Format(time.RFC3339),
						})
					}
				}
			}
		}
		result["traces"] = traces

		// 2. Find related audit logs (logs with trace_id in metadata)
		var logs []map[string]interface{}
		if deps.DB != nil && deps.DB.Pool != nil {
			rows, err := deps.DB.Pool.Query(ctx, `
				SELECT created_at, action, COALESCE(entity_type, ''), COALESCE(plugin_name, ''), COALESCE(ip_address::text, '')
				FROM audit_log
				WHERE new_values::text LIKE $1
				ORDER BY created_at DESC
				LIMIT 100
			`, "%"+traceID+"%")
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var ts time.Time
					var action, entityType, plugin, ip string
					if err := rows.Scan(&ts, &action, &entityType, &plugin, &ip); err == nil {
						logs = append(logs, map[string]interface{}{
							"timestamp":   ts.UTC().Format(time.RFC3339),
							"action":      action,
							"entity_type": entityType,
							"plugin":      plugin,
							"ip":          ip,
							"trace_id":    traceID,
						})
					}
				}
			}
		}
		result["logs"] = logs

		// 3. Get metrics during the trace timeframe (last hour as fallback)
		var metrics []map[string]interface{}
		metricFamilies, err := prometheus.DefaultGatherer.Gather()
		if err == nil {
			for _, mf := range metricFamilies {
				metrics = append(metrics, map[string]interface{}{
					"name": mf.GetName(),
					"type": mf.GetType().String(),
					"help": mf.GetHelp(),
				})
			}
		}
		result["metrics"] = metrics

		// 4. Add correlation metadata
		result["correlated_at"] = time.Now().UTC().Format(time.RFC3339)
		result["total_traces"] = len(traces)
		result["total_logs"] = len(logs)
		result["total_metrics"] = len(metrics)

		c.JSON(http.StatusOK, result)
	}
}

// observabilitySendTestTelemetry sends a test trace, metric, and log to the OTLP backend.
// This helps verify that SigNoz/Jaeger/Tempo is receiving data from PEPA.
func observabilitySendTestTelemetry(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !deps.Config.Observability.Enabled || deps.Config.Observability.OTLPEndpoint == "" {
			c.JSON(http.StatusOK, gin.H{
				"status":  "skipped",
				"message": "OTLP is not enabled. Enable it in Settings → Observability.",
			})
			return
		}

		results := map[string]interface{}{}

		// 1. Send a test trace span
		tracer := observability.GetTracer("pepa-test")
		ctx, span := tracer.Start(c.Request.Context(), "pepa-otel-diagnostic-test")
		span.AddEvent("diagnostic test span created")
		traceID := span.SpanContext().TraceID().String()
		spanID := span.SpanContext().SpanID().String()
		span.End()
		results["trace"] = gin.H{
			"sent":     true,
			"trace_id": traceID,
			"span_id":  spanID,
			"service":  deps.Config.Observability.ServiceName,
			"endpoint": deps.Config.Observability.OTLPEndpoint,
		}

		// 2. Send a test log via OTLP (using the OTel slog bridge)
		log := logging.LogFromContext(ctx)
		log.Info("pepa-otel-diagnostic-test",
			"trace_id", traceID,
			"test", true,
			"message", "If you see this in SigNoz, OTLP log export is working",
		)
		results["log"] = gin.H{
			"sent":     true,
			"trace_id": traceID,
			"body":     "pepa-otel-diagnostic-test",
		}

		// 3. Record a test metric
		meter := otel.GetMeterProvider().Meter("pepa-test")
		counter, err := meter.Int64Counter("pepa_otel_diagnostic_test")
		if err == nil {
			counter.Add(c.Request.Context(), 1)
			results["metric"] = gin.H{
				"sent":  true,
				"name":  "pepa_otel_diagnostic_test",
				"value": 1,
			}
		} else {
			results["metric"] = gin.H{"sent": false, "error": err.Error()}
		}

		slog.Info("OTel diagnostic test telemetry sent",
			"trace_id", traceID,
			"endpoint", deps.Config.Observability.OTLPEndpoint,
		)

		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": fmt.Sprintf("Test telemetry sent to %s. Check SigNoz in 5-10 seconds.", deps.Config.Observability.OTLPEndpoint),
			"trace_id": traceID,
			"results": results,
		})
	}
}
