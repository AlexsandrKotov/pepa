package rest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/hostpath"
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
	scans.POST("/scans/:id/cancel", cancelScanRun(deps))
	scans.GET("/scans/:id/report", getScanReport(deps))

	// Scan Schedules
	scans.GET("/schedules", listScanSchedules(deps))
	scans.POST("/schedules", createScanSchedule(deps))
	scans.GET("/schedules/:id", getScanSchedule(deps))
	scans.PUT("/schedules/:id", updateScanSchedule(deps))
	scans.DELETE("/schedules/:id", deleteScanSchedule(deps))

	// Dashboard & bulk operations
	scans.GET("/dashboard-v2", getDashboardV2(deps))
	scans.POST("/scan-all", scanAllTargets(deps))

	// SonarQube project lookup — feeds the Project Key picker of a SonarQube target
	scans.GET("/sonar/projects", listSonarProjects(deps))
	// SonarQube issue transitions are applied upstream, PEPA just proxies the verb
	scans.POST("/sonar/issues/transition", transitionSonarIssue(deps))

	// Database status
	scans.GET("/db-status", getDatabaseStatus(deps))
	// Database management (admin only)
	scans.POST("/db-download", requireAdminRole(deps), downloadDatabase(deps))
	scans.PUT("/db-repository", requireAdminRole(deps), setDBRepository(deps))

	// Scan Ignores (CVE ignore lists)
	scans.GET("/ignores", listScanIgnores(deps))
	scans.GET("/targets/:id/ignores", listTargetIgnores(deps))
	scans.POST("/targets/:id/ignores", createScanIgnore(deps))
	scans.DELETE("/ignores/:ignoreId", deleteScanIgnore(deps))
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
			"container": true, "service": true, "sonarqube_project": true, "registry": true,
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

		// Validate filesystem target paths are within HOST_DATA_DIR.
		if input.TargetType == "filesystem" {
			hostDataDir := deps.Config.HostDataDir
			if hostDataDir == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "HOST_DATA_DIR is not configured — filesystem scan targets require the admin to set HOST_DATA_DIR"})
				return
			}
			if err := hostpath.Validate(input.TargetRef, hostDataDir); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
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

		if msg, ok := validateScanTargetShape(c.Request.Context(), deps, target); !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}

		if err := deps.Repos.SecurityScan.CreateScanTarget(c.Request.Context(), target); err != nil {
			respondInternalError(c, err)
			return
		}
		c.JSON(http.StatusCreated, target)
	}
}

// validateScanTargetShape enforces the scanner ↔ target contract. Without it the
// server accepted combinations that can never work — a SonarQube target pointing
// at a git URL, a Trivy image target pointing at a SonarQube project — and the
// breakage only surfaced as a failed scan minutes later.
func validateScanTargetShape(ctx context.Context, deps Dependencies, target *repository.ScanTarget) (string, bool) {
	if err := validateScannerTargetPair(target.ScannerType, target.TargetType); err != nil {
		return err.Error(), false
	}
	if err := security.ValidateScanConfig(target.ScannerType, target.ScanConfig); err != nil {
		return err.Error(), false
	}
	if target.ScannerType != "sonarqube" {
		// For git_repo targets, validate the connection is a git-compatible type
		// (GitLab or Git) so Trivy can clone private repositories.
		if target.TargetType == "git_repo" && target.ConnectionID != nil {
			if deps.Repos.Connection == nil {
				return "connection repository not available", false
			}
			conn, err := deps.Repos.Connection.Get(ctx, *target.ConnectionID, target.TenantID)
			if err != nil {
				return "the selected Connection was not found in this tenant", false
			}
			if conn.Type != repository.ConnectionGitLab && conn.Type != repository.ConnectionGit {
				return "the selected Connection must be of type 'gitlab' or 'git' for git repository scans (got '" + string(conn.Type) + "')", false
			}
		}
		return "", true
	}
	if err := security.ValidateSonarProjectKey(target); err != nil {
		return err.Error(), false
	}
	if target.ConnectionID == nil {
		return "SonarQube credentials come from a Connection — add one in Connections and select it here", false
	}
	if deps.Repos.Connection == nil {
		return "connection repository not available", false
	}
	conn, err := deps.Repos.Connection.Get(ctx, *target.ConnectionID, target.TenantID)
	if err != nil {
		return "the selected Connection was not found in this tenant", false
	}
	if conn.Type != repository.ConnectionSonarQube {
		return "the selected Connection must be of type 'sonarqube' (got '" + string(conn.Type) + "')", false
	}
	return "", true
}

// validateScannerTargetPair documents which target types each scanner can act
// on. SonarQube analyses a project it already owns, addressed by its key — never
// by a git URL or a filesystem path. Trivy scans images and registries, and
// fetches code itself for git_repo/filesystem targets.
func validateScannerTargetPair(scannerType, targetType string) error {
	switch scannerType {
	case "sonarqube":
		if targetType != "sonarqube_project" {
			return fmt.Errorf("scanner_type 'sonarqube' requires target_type 'sonarqube_project' — " +
				"SonarQube analyses a project by its key; git URLs and filesystem paths are scanned by Trivy")
		}
	case "trivy":
		// "service" is kept for legacy targets: it refers to an image name.
		switch targetType {
		case "image", "registry", "container", "git_repo", "filesystem", "service":
		default:
			return fmt.Errorf("scanner_type 'trivy' does not support target_type %q", targetType)
		}
	case "both":
		switch targetType {
		case "git_repo", "filesystem":
		default:
			return fmt.Errorf("scanner_type 'both' requires target_type 'git_repo' or 'filesystem'")
		}
	}
	return nil
}

// ── SonarQube Helpers ─────────────────────────────────────────

// listSonarProjects returns the projects of a SonarQube Connection so the target
// form can offer real project keys. SonarQube is an external service, so its own
// error is passed through — a silently empty list would look like "no projects".
func listSonarProjects(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Scanner == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scanner not available"})
			return
		}
		connectionID, err := uuid.Parse(c.Query("connection_id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "connection_id query parameter is required"})
			return
		}
		projects, truncated, err := deps.Scanner.ListSonarProjects(
			c.Request.Context(), connectionID, auth.GetTenantID(c), c.Query("q"))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"projects": projects, "truncated": truncated})
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
			// Validate scanner_type (same validation as create)
			validScanners := map[string]bool{"trivy": true, "sonarqube": true, "both": true}
			if !validScanners[input.ScannerType] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "scanner_type must be 'trivy', 'sonarqube', or 'both'"})
				return
			}
			existing.ScannerType = input.ScannerType
		}
		if input.TargetType != "" {
			// Validate target_type (same validation as create)
			validTargetTypes := map[string]bool{
				"image": true, "git_repo": true, "filesystem": true,
				"container": true, "service": true, "sonarqube_project": true, "registry": true,
			}
			if !validTargetTypes[input.TargetType] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_type"})
				return
			}
			existing.TargetType = input.TargetType
		}
		if input.TargetRef != "" {
			existing.TargetRef = input.TargetRef
		}

		// Validate filesystem target paths are within HOST_DATA_DIR.
		if existing.TargetType == "filesystem" {
			hostDataDir := deps.Config.HostDataDir
			if hostDataDir == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "HOST_DATA_DIR is not configured — filesystem scan targets require the admin to set HOST_DATA_DIR"})
				return
			}
			if err := hostpath.Validate(existing.TargetRef, hostDataDir); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
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

		// Re-check the whole shape after applying the patch: a target may become
		// invalid by changing only one side of the scanner ↔ target pair.
		if msg, ok := validateScanTargetShape(c.Request.Context(), deps, existing); !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
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
		// Use WithoutCancel (not Background) to preserve tracing/tenant metadata.
		bgCtx := context.WithoutCancel(c.Request.Context())
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

func cancelScanRun(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Scanner == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "scanner not available"})
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan ID"})
			return
		}
		tenantID := auth.GetTenantID(c)
		if err := deps.Scanner.CancelScan(c.Request.Context(), id, tenantID); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "scan cancelled"})
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
		if deps.ScanScheduler != nil {
			deps.ScanScheduler.Reload()
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
		if deps.ScanScheduler != nil {
			deps.ScanScheduler.Reload()
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
		if deps.ScanScheduler != nil {
			deps.ScanScheduler.Reload()
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
		// Use WithoutCancel (not Background) to preserve tracing/tenant metadata.
		bgCtx := context.WithoutCancel(c.Request.Context())
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

// ── Database Status Handler ───────────────────────────────────

func getDatabaseStatus(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Scanner == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "scanner not available"})
			return
		}
		status := deps.Scanner.GetDatabaseStatus(c.Request.Context())
		c.JSON(http.StatusOK, status)
	}
}

// ── Database Management Handlers ───────────────────────────────

// downloadDatabase triggers an immediate download of the Trivy vulnerability
// and Java databases. This is called automatically when the Trivy plugin is
// installed or enabled, but can also be triggered manually from the UI.
func downloadDatabase(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Scanner == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "scanner not available"})
			return
		}

		// Optional: override repositories for this download
		var req struct {
			DBRepository     string `json:"db_repository,omitempty"`
			JavaDBRepository string `json:"java_db_repository,omitempty"`
		}
		_ = c.ShouldBindJSON(&req)
		if req.DBRepository != "" || req.JavaDBRepository != "" {
			deps.Scanner.SetDBRepository(req.DBRepository, req.JavaDBRepository)
		}

		// Run download with a timeout (this can take several minutes for large DBs)
		dlCtx, dlCancel := context.WithTimeout(c.Request.Context(), 15*time.Minute)
		defer dlCancel()

		slog.Info("manual Trivy DB download triggered", "db_repo", req.DBRepository, "java_db_repo", req.JavaDBRepository)
		if err := deps.Scanner.DownloadDB(dlCtx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Trivy DB download failed",
				"details": err.Error(),
			})
			return
		}

		// Return updated status
		status := deps.Scanner.GetDatabaseStatus(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{
			"message": "Trivy databases downloaded successfully",
			"status":  status,
		})
	}
}

// setDBRepository updates the OCI registry endpoints used for Trivy DB downloads.
// This allows administrators to switch between different DB sources (e.g., ECR,
// GHCR, or a private mirror) without restarting the API server.
func setDBRepository(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Scanner == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "scanner not available"})
			return
		}

		var req struct {
			DBRepository     string `json:"db_repository" binding:"required"`
			JavaDBRepository string `json:"java_db_repository"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "db_repository is required"})
			return
		}

		deps.Scanner.SetDBRepository(req.DBRepository, req.JavaDBRepository)
		logAudit(deps, c, "update", "trivy_db_repository", "", nil, gin.H{
			"db_repository":      req.DBRepository,
			"java_db_repository": req.JavaDBRepository,
		})

		c.JSON(http.StatusOK, gin.H{
			"message":            "Trivy DB repository updated",
			"db_repository":      req.DBRepository,
			"java_db_repository": req.JavaDBRepository,
		})
	}
}

// ── Scan Ignore Handlers ──────────────────────────────────────

func listScanIgnores(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.ScanIgnore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "scan ignore repository not available"})
			return
		}
		tenantID := auth.GetTenantID(c)
		ignores, err := deps.Repos.ScanIgnore.List(c.Request.Context(), tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if ignores == nil {
			ignores = []*repository.ScanIgnore{}
		}
		c.JSON(http.StatusOK, ignores)
	}
}

func listTargetIgnores(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.ScanIgnore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "scan ignore repository not available"})
			return
		}
		targetID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target ID"})
			return
		}
		tenantID := auth.GetTenantID(c)
		ignores, err := deps.Repos.ScanIgnore.ListByTarget(c.Request.Context(), targetID, tenantID)
		if err != nil {
			respondInternalError(c, err)
			return
		}
		if ignores == nil {
			ignores = []*repository.ScanIgnore{}
		}
		c.JSON(http.StatusOK, ignores)
	}
}

func createScanIgnore(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.ScanIgnore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "scan ignore repository not available"})
			return
		}
		targetID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target ID"})
			return
		}
		tenantID := auth.GetTenantID(c)

		var input struct {
			CveID    string  `json:"cve_id"`
			IssueKey *string `json:"issue_key"`
			Reason   *string `json:"reason"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Trivy findings are suppressed by CVE, SonarQube findings by issue key
		// (or "rule:<rule>" for a whole rule). Exactly one identifier per ignore.
		cveID := strings.TrimSpace(input.CveID)
		issueKey := ""
		if input.IssueKey != nil {
			issueKey = strings.TrimSpace(*input.IssueKey)
		}
		if cveID == "" && issueKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "either cve_id (Trivy) or issue_key (SonarQube) is required"})
			return
		}
		if len(cveID) > 50 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cve_id is too long (max 50 characters)"})
			return
		}
		if len(issueKey) > 255 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "issue_key is too long (max 255 characters)"})
			return
		}

		userID := auth.GetUserID(c)
		ignore := &repository.ScanIgnore{
			ID:        uuid.New(),
			TenantID:  tenantID,
			TargetID:  targetID,
			CveID:     cveID,
			Reason:    input.Reason,
			CreatedBy: userID,
		}
		if issueKey != "" {
			ignore.IssueKey = &issueKey
		}

		if err := deps.Repos.ScanIgnore.Create(c.Request.Context(), ignore); err != nil {
			if strings.Contains(err.Error(), "duplicate key") {
				c.JSON(http.StatusConflict, gin.H{"error": "this finding is already ignored for the target"})
				return
			}
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusCreated, ignore)
	}
}

func deleteScanIgnore(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Repos.ScanIgnore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "scan ignore repository not available"})
			return
		}
		ignoreID, err := uuid.Parse(c.Param("ignoreId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ignore ID"})
			return
		}
		tenantID := auth.GetTenantID(c)

		if err := deps.Repos.ScanIgnore.Delete(c.Request.Context(), ignoreID, tenantID); err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "ignore deleted"})
	}
}

// ── Host Data Directory Browser ────────────────────────────────

// registerHostDataRoutes registers the directory listing endpoint used by the
// frontend directory browser. The endpoint lists subdirectories within
// HOST_DATA_DIR so users can pick scan targets, Terraform stacks, etc.
func registerHostDataRoutes(v1 *gin.RouterGroup, deps Dependencies) {
	host := v1.Group("/host")
	host.GET("/directories", listHostDirectories(deps))
	host.GET("/config", getHostDataConfig(deps))
}

// getHostDataConfig returns the HOST_DATA_DIR setting so the frontend knows
// the root directory for the directory browser.
func getHostDataConfig(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		hostDataDir := deps.Config.HostDataDir
		c.JSON(http.StatusOK, gin.H{
			"host_data_dir": hostDataDir,
			"configured":    hostDataDir != "",
		})
	}
}

// listHostDirectories lists subdirectories within HOST_DATA_DIR.
// Query parameter "path" optionally specifies a subdirectory to list.
func listHostDirectories(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		hostDataDir := deps.Config.HostDataDir
		if hostDataDir == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "HOST_DATA_DIR is not configured — ask your admin to set it in .env",
			})
			return
		}

		subPath := c.Query("path")
		dirs, err := hostpath.ListDirectories(subPath, hostDataDir)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		currentPath := hostDataDir
		if subPath != "" {
			resolved, resolveErr := hostpath.Resolve(subPath, hostDataDir)
			if resolveErr == nil {
				currentPath = resolved
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"base_dir":     hostDataDir,
			"current_path": currentPath,
			"directories":  dirs,
		})
	}
}
