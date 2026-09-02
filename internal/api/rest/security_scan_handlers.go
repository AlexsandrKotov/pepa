package rest

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/security"
)

// registerSecurityScanRoutes registers all security scanning API routes.
func registerSecurityScanRoutes(v1 *gin.RouterGroup, deps Dependencies) {
	scans := v1.Group("/security")

	// Scan Targets
	scans.GET("/targets", listScanTargets(deps))
	scans.POST("/targets", createScanTarget(deps))
	scans.GET("/targets/:id", getScanTarget(deps))
	scans.PUT("/targets/:id", updateScanTarget(deps))
	scans.DELETE("/targets/:id", deleteScanTarget(deps))
	scans.POST("/targets/:id/scan", triggerScan(deps))

	// Scan Runs
	scans.GET("/scans", listScanRuns(deps))
	scans.GET("/scans/:id", getScanRun(deps))

	// Scan Schedules
	scans.GET("/schedules", listScanSchedules(deps))
	scans.POST("/schedules", createScanSchedule(deps))
	scans.GET("/schedules/:id", getScanSchedule(deps))
	scans.PUT("/schedules/:id", updateScanSchedule(deps))
	scans.DELETE("/schedules/:id", deleteScanSchedule(deps))

	// Dashboard & bulk operations
	scans.GET("/dashboard-v2", getDashboardV2(deps))
	scans.POST("/scan-all", scanAllTargets(deps))
}

// ── Scan Target Handlers ──────────────────────────────────────

func listScanTargets(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scan repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)
		targets, err := deps.Repos.SecurityScan.ListScanTargets(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if targets == nil {
			targets = []repository.ScanTarget{}
		}
		c.JSON(http.StatusOK, targets)
	}
}

func createScanTarget(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scan repository not available"})
			return
		}
		var input struct {
			Name         string         `json:"name" binding:"required"`
			ScannerType  string         `json:"scanner_type" binding:"required"`
			TargetType   string         `json:"target_type" binding:"required"`
			TargetRef    string         `json:"target_ref" binding:"required"`
			ConnectionID *uuid.UUID     `json:"connection_id,omitempty"`
			ScanConfig   map[string]any `json:"scan_config"`
			Enabled      *bool          `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validate scanner_type
		validScanners := map[string]bool{"trivy": true, "sonarqube": true, "both": true}
		if !validScanners[input.ScannerType] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "scanner_type must be 'trivy', 'sonarqube', or 'both'"})
			return
		}

		// Validate target_type
		validTargetTypes := map[string]bool{
			"image": true, "git_repo": true, "filesystem": true,
			"container": true, "service": true, "sonarqube_project": true,
		}
		if !validTargetTypes[input.TargetType] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_type"})
			return
		}

		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		if input.ScanConfig == nil {
			input.ScanConfig = map[string]any{}
		}

		tenantID := auth.GetTenantID(c)
		userID := auth.GetUserID(c)
		target := &repository.ScanTarget{
			TenantID:     tenantID,
			Name:         input.Name,
			ScannerType:  input.ScannerType,
			TargetType:   input.TargetType,
			TargetRef:    input.TargetRef,
			ConnectionID: input.ConnectionID,
			ScanConfig:   input.ScanConfig,
			Enabled:      enabled,
			CreatedBy:    userID,
		}

		if err := deps.Repos.SecurityScan.CreateScanTarget(c.Request.Context(), target); err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusCreated, target)
	}
}

func getScanTarget(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scan repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target ID"})
			return
		}
		tenantID := auth.GetTenantID(c)
		target, err := deps.Repos.SecurityScan.GetScanTarget(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "scan target not found"})
			return
		}
		c.JSON(http.StatusOK, target)
	}
}

func updateScanTarget(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scan repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target ID"})
			return
		}
		tenantID := auth.GetTenantID(c)

		existing, err := deps.Repos.SecurityScan.GetScanTarget(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "scan target not found"})
			return
		}

		var input struct {
			Name         string         `json:"name"`
			ScannerType  string         `json:"scanner_type"`
			TargetType   string         `json:"target_type"`
			TargetRef    string         `json:"target_ref"`
			ConnectionID *uuid.UUID     `json:"connection_id"`
			ScanConfig   map[string]any `json:"scan_config"`
			Enabled      *bool          `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if input.Name != "" {
			existing.Name = input.Name
		}
		if input.ScannerType != "" {
			existing.ScannerType = input.ScannerType
		}
		if input.TargetType != "" {
			existing.TargetType = input.TargetType
		}
		if input.TargetRef != "" {
			existing.TargetRef = input.TargetRef
		}
		if input.ConnectionID != nil {
			existing.ConnectionID = input.ConnectionID
		}
		if input.ScanConfig != nil {
			existing.ScanConfig = input.ScanConfig
		}
		if input.Enabled != nil {
			existing.Enabled = *input.Enabled
		}

		if err := deps.Repos.SecurityScan.UpdateScanTarget(c.Request.Context(), existing); err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, existing)
	}
}

func deleteScanTarget(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scan repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target ID"})
			return
		}
		tenantID := auth.GetTenantID(c)
		if err := deps.Repos.SecurityScan.DeleteScanTarget(c.Request.Context(), id, tenantID); err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusNoContent, nil)
	}
}

func triggerScan(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil || deps.Scanner == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scanning not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target ID"})
			return
		}
		tenantID := auth.GetTenantID(c)

		// Detach from request context — the request context is cancelled
		// when the response is sent, which would kill the scan immediately.
		bgCtx := context.Background()
		go func() {
			_, err := deps.Scanner.RunScan(bgCtx, id, tenantID, "manual")
			if err != nil {
				slog.Error("async scan failed", "target_id", id, "error", err)
			}
		}()

		c.JSON(http.StatusAccepted, gin.H{"message": "scan triggered", "target_id": id})
	}
}

// ── Scan Run Handlers ─────────────────────────────────────────

func listScanRuns(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scan repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)

		var targetID *uuid.UUID
		if tid := c.Query("target_id"); tid != "" {
			if parsed, err := uuid.Parse(tid); err == nil {
				targetID = &parsed
			}
		}
		status := c.Query("status")
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

		runs, err := deps.Repos.SecurityScan.ListScanRuns(c.Request.Context(), tenantID, targetID, status, limit, offset)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if runs == nil {
			runs = []repository.ScanRun{}
		}
		c.JSON(http.StatusOK, runs)
	}
}

func getScanRun(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scan repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID"})
			return
		}
		tenantID := auth.GetTenantID(c)
		run, err := deps.Repos.SecurityScan.GetScanRun(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "scan run not found"})
			return
		}
		c.JSON(http.StatusOK, run)
	}
}

// ── Scan Schedule Handlers ────────────────────────────────────

func listScanSchedules(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scan repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)
		schedules, err := deps.Repos.SecurityScan.ListScanSchedules(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if schedules == nil {
			schedules = []repository.ScanSchedule{}
		}
		c.JSON(http.StatusOK, schedules)
	}
}

func createScanSchedule(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scan repository not available"})
			return
		}
		var input struct {
			TargetID       uuid.UUID `json:"target_id" binding:"required"`
			CronExpression string    `json:"cron_expression" binding:"required"`
			Enabled        *bool     `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}

		tenantID := auth.GetTenantID(c)
		userID := auth.GetUserID(c)
		schedule := &repository.ScanSchedule{
			TenantID:       tenantID,
			TargetID:       input.TargetID,
			CronExpression: input.CronExpression,
			Enabled:        enabled,
			CreatedBy:      userID,
		}

		// Calculate next run time from cron expression
		nextRun := security.NextCronRun(input.CronExpression)
		schedule.NextRunAt = &nextRun

		if err := deps.Repos.SecurityScan.CreateScanSchedule(c.Request.Context(), schedule); err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusCreated, schedule)
	}
}

func getScanSchedule(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scan repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule ID"})
			return
		}
		tenantID := auth.GetTenantID(c)
		schedule, err := deps.Repos.SecurityScan.GetScanSchedule(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "scan schedule not found"})
			return
		}
		c.JSON(http.StatusOK, schedule)
	}
}

func updateScanSchedule(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scan repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule ID"})
			return
		}
		tenantID := auth.GetTenantID(c)

		existing, err := deps.Repos.SecurityScan.GetScanSchedule(c.Request.Context(), id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "scan schedule not found"})
			return
		}

		var input struct {
			CronExpression string `json:"cron_expression"`
			Enabled        *bool  `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if input.CronExpression != "" {
			existing.CronExpression = input.CronExpression
			nextRun := security.NextCronRun(input.CronExpression)
			existing.NextRunAt = &nextRun
		}
		if input.Enabled != nil {
			existing.Enabled = *input.Enabled
		}

		if err := deps.Repos.SecurityScan.UpdateScanSchedule(c.Request.Context(), existing); err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, existing)
	}
}

func deleteScanSchedule(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scan repository not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule ID"})
			return
		}
		tenantID := auth.GetTenantID(c)
		if err := deps.Repos.SecurityScan.DeleteScanSchedule(c.Request.Context(), id, tenantID); err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusNoContent, nil)
	}
}

// ── Dashboard & Bulk Operations ───────────────────────────────

func getDashboardV2(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scan repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)
		ctx := c.Request.Context()

		// Get all targets
		targets, err := deps.Repos.SecurityScan.ListScanTargets(ctx, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		// Get recent scan runs
		runs, err := deps.Repos.SecurityScan.ListScanRuns(ctx, tenantID, nil, "", 20, 0)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		// Get schedules
		schedules, err := deps.Repos.SecurityScan.ListScanSchedules(ctx, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		// Build dashboard summary
		dashboard := gin.H{
			"targets": gin.H{
				"total":   len(targets),
				"enabled": countEnabled(targets),
			},
			"recent_scans": runs,
			"scan_summary": gin.H{
				"completed": countByStatus(runs, "completed"),
				"failed":    countByStatus(runs, "failed"),
				"running":   countByStatus(runs, "running"),
				"pending":   countByStatus(runs, "pending"),
			},
			"schedules": gin.H{
				"total":   len(schedules),
				"enabled": countEnabledSchedules(schedules),
			},
		}

		c.JSON(http.StatusOK, dashboard)
	}
}

func scanAllTargets(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.SecurityScan == nil || deps.Scanner == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scanning not available"})
			return
		}
		tenantID := auth.GetTenantID(c)

		// Detach from request context to avoid cancellation when response is sent.
		bgCtx := context.Background()
		go func() {
			_, err := deps.Scanner.ScanAllEnabled(bgCtx, tenantID, "manual")
			if err != nil {
				slog.Error("scan-all failed", "error", err)
			}
		}()

		c.JSON(http.StatusAccepted, gin.H{"message": "scanning all enabled targets"})
	}
}

// ── Helpers ────────────────────────────────────────────────────

func countEnabled(targets []repository.ScanTarget) int {
	count := 0
	for _, t := range targets {
		if t.Enabled {
			count++
		}
	}
	return count
}

func countByStatus(runs []repository.ScanRun, status string) int {
	count := 0
	for _, r := range runs {
		if r.Status == status {
			count++
		}
	}
	return count
}

func countEnabledSchedules(schedules []repository.ScanSchedule) int {
	count := 0
	for _, s := range schedules {
		if s.Enabled {
			count++
		}
	}
	return count
}
