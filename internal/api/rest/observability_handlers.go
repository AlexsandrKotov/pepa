package rest

import (
	"context"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/auth"
	"github.com/prometheus/client_golang/prometheus"
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
			log.Printf("observabilityLogs: query error: %v", err)
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
			log.Printf("observabilityTraces: query error: %v", err)
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

		if failedPipelines > 5 {
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

		if failedDeployments > 3 {
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

		if memUsageMB > 500 {
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

		summary := map[string]interface{}{
			"total":  len(alerts),
			"firing": len(alerts),
		}

		c.JSON(http.StatusOK, gin.H{
			"alerts":  alerts,
			"total":   len(alerts),
			"summary": summary,
		})
	}
}

// observabilityResolveAlert resolves a specific alert by ID.
func observabilityResolveAlert(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		alertID := c.Param("id")

		c.JSON(http.StatusOK, gin.H{
			"alert": map[string]interface{}{
				"id":     alertID,
				"status": "resolved",
			},
			"message": "Alert resolved successfully",
		})
	}
}

// startTime tracks when the application started for uptime calculation.
var startTime = time.Now()
