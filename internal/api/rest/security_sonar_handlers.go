package rest

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/security"
)

// getScanReport renders a stored scan run as a server-side report. The report is
// built from data PEPA already has, so both scanners share one code path and a
// SonarQube run exports exactly like a Trivy one.
func getScanReport(deps Dependencies) gin.HandlerFunc {
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
		format := strings.ToLower(c.DefaultQuery("format", "json"))
		if format != "json" && format != "html" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "format must be json or html"})
			return
		}

		tenantID := auth.GetTenantID(c)
		ctx := c.Request.Context()
		run, err := deps.Repos.SecurityScan.GetScanRun(ctx, id, tenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "scan run not found"})
			return
		}
		// The report is titled after its target. A target deleted after the run
		// still deserves a report, so fall back to what the run remembers.
		target, err := deps.Repos.SecurityScan.GetScanTarget(ctx, run.TargetID, tenantID)
		if err != nil || target == nil {
			target = &repository.ScanTarget{
				ID:          run.TargetID,
				TenantID:    tenantID,
				Name:        run.TargetName,
				TargetRef:   run.TargetRef,
				ScannerType: run.ScannerType,
			}
		}

		if format == "html" {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"pepa-scan-report-%s.html\"", id))
			if err := security.GenerateHTMLReport(c.Writer, target, run); err != nil {
				slog.Error("failed to render scan report", "scan_id", id, "error", err)
			}
			return
		}

		report, err := security.GenerateJSONReport(target, run)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate report"})
			return
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", report)
	}
}

// transitionSonarIssue forwards an issue workflow change to the external
// SonarQube that owns the issue. PEPA stores nothing: the next collected report
// simply reflects the new upstream state.
func transitionSonarIssue(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Scanner == nil || deps.Repos.Connection == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scanner not available"})
			return
		}
		var req struct {
			ConnectionID string `json:"connection_id"`
			IssueKey     string `json:"issue_key"`
			Transition   string `json:"transition"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		connectionID, err := uuid.Parse(req.ConnectionID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "connection_id must reference the SonarQube connection of the issue"})
			return
		}
		if strings.TrimSpace(req.IssueKey) == "" || strings.TrimSpace(req.Transition) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "issue_key and transition are required"})
			return
		}

		if err := deps.Scanner.TransitionSonarIssue(c.Request.Context(), connectionID, auth.GetTenantID(c), req.IssueKey, req.Transition); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message":    "issue transitioned in SonarQube",
			"issue_key":  req.IssueKey,
			"transition": req.Transition,
		})
	}
}

// sonarWebhookPayload is the subset of the SonarQube Compute Operator webhook
// report PEPA needs: the project identity to match targets against, plus the
// gate status for the log line and the response echo.
type sonarWebhookPayload struct {
	Type    string `json:"type"`
	Project struct {
		Name string `json:"name"`
		Key  string `json:"key"`
		URL  string `json:"url"`
	} `json:"project"`
	QualityGate struct {
		Status string `json:"status"`
	} `json:"qualityGate"`
	Date string `json:"date"`
}

// checkSonarWebhookSecret verifies the shared secret SonarQube sends in
// X-PEPA-Signature. An unconfigured token answers 503 instead of accepting
// anonymous triggers, because forgetting the configuration is the likely mistake.
func checkSonarWebhookSecret(provided string) (ok bool, status int) {
	token := os.Getenv("SONAR_WEBHOOK_TOKEN")
	if token == "" {
		return false, http.StatusServiceUnavailable
	}
	if provided == "" {
		return false, http.StatusUnauthorized
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(provided)) != 1 {
		return false, http.StatusUnauthorized
	}
	return true, http.StatusOK
}

// sonarPayloadURLMatches reports whether the dashboard URL of a webhook payload
// belongs to a connection's base URL. SonarQube builds that URL from its own
// configured base, so this is the only link between "which instance sent this"
// and "which Connection may be used to collect the report" — therefore it
// compares scheme, host and path boundary instead of a raw string prefix, so a
// host named like another one can never borrow its credentials.
func sonarPayloadURLMatches(payloadURL, connectionURL string) bool {
	payload, err := url.Parse(strings.TrimSpace(payloadURL))
	if err != nil || payload.Host == "" {
		return false
	}
	base, err := url.Parse(strings.TrimSpace(connectionURL))
	if err != nil || base.Host == "" {
		return false
	}
	if !strings.EqualFold(payload.Scheme, base.Scheme) || !strings.EqualFold(payload.Host, base.Host) {
		return false
	}
	basePath := strings.TrimRight(base.Path, "/")
	if basePath == "" {
		return true
	}
	return payload.Path == basePath || strings.HasPrefix(payload.Path, basePath+"/")
}

// sonarWebhookTargetMatches finds the scan targets that own a webhook report:
// same SonarQube instance, same project key.
func sonarWebhookTargetMatches(target *repository.ScanTarget, connectionID uuid.UUID, projectKey string) bool {
	if target.TargetType != "sonarqube_project" {
		return false
	}
	if target.ConnectionID == nil || *target.ConnectionID != connectionID {
		return false
	}
	return security.SonarProjectKey(target) == projectKey
}

// sonarWebhook receives "analysis report ready" notifications from SonarQube and
// collects a fresh PEPA report for every target owning the analyzed project.
// It is a public route: SonarQube cannot hold a PEPA token, so the shared secret
// in X-PEPA-Signature is the authentication.
func sonarWebhook(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Scanner == nil || deps.Repos.Connection == nil || deps.Repos.SecurityScan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security scanning not available"})
			return
		}
		if ok, status := checkSonarWebhookSecret(c.GetHeader("X-PEPA-Signature")); !ok {
			if status == http.StatusServiceUnavailable {
				c.JSON(status, gin.H{"error": "sonarqube webhook is disabled: SONAR_WEBHOOK_TOKEN is not configured"})
				return
			}
			slog.Warn("sonarqube webhook: rejected signature", "remote", c.ClientIP())
			c.JSON(status, gin.H{"error": "invalid webhook signature"})
			return
		}

		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
			return
		}
		var payload sonarWebhookPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
		if payload.Type != "" && payload.Type != "REPORT" {
			c.JSON(http.StatusOK, gin.H{"message": "ignored", "type": payload.Type})
			return
		}
		projectKey := strings.TrimSpace(payload.Project.Key)
		if projectKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "payload carries no project key"})
			return
		}

		// Webhooks arrive without an auth context, so they resolve inside the
		// default tenant — the convention the GitLab deploy webhook already uses.
		tenantID, err := uuid.Parse(database.DefaultTenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "default tenant id is malformed"})
			return
		}
		ctx := c.Request.Context()

		conns, err := deps.Repos.Connection.FindByType(ctx, string(repository.ConnectionSonarQube), tenantID)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to look up sonarqube connections"})
			return
		}
		targets, err := deps.Repos.SecurityScan.ListScanTargets(ctx, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list scan targets"})
			return
		}

		// Detach from the request context: the collection must outlive this response.
		collectCtx := context.WithoutCancel(ctx)
		triggered := make([]string, 0, len(targets))
		for i := range conns {
			conn := &conns[i]
			baseURL, _ := conn.Config["url"].(string)
			if !sonarPayloadURLMatches(payload.Project.URL, baseURL) {
				continue
			}
			for j := range targets {
				target := &targets[j]
				if !sonarWebhookTargetMatches(target, conn.ID, projectKey) {
					continue
				}
				targetID := target.ID
				triggered = append(triggered, targetID.String())
				go func(id uuid.UUID) {
					if _, err := deps.Scanner.RunScan(collectCtx, id, tenantID, "webhook"); err != nil {
						slog.Error("sonarqube webhook scan failed", "target_id", id, "error", err)
					}
				}(targetID)
			}
		}

		slog.Info("sonarqube webhook received",
			"project_key", projectKey,
			"quality_gate", payload.QualityGate.Status,
			"targets_triggered", len(triggered),
		)
		c.JSON(http.StatusAccepted, gin.H{
			"message":           "webhook accepted",
			"project_key":       projectKey,
			"quality_gate":      payload.QualityGate.Status,
			"targets_triggered": len(triggered),
			"target_ids":        triggered,
		})
	}
}
