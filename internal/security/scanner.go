// Package security provides scan orchestration for Trivy and SonarQube.
package security

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/plugin/engine"
	"github.com/pepa/pepa/internal/repository"
)

// maxConcurrentScans limits the number of scans running in parallel
// to prevent resource exhaustion on the host.
const maxConcurrentScans = 3

// validSeverities is the whitelist of allowed Trivy severity values.
var validSeverities = map[string]bool{
	"UNKNOWN": true, "LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true,
}

// validScanConfigKeys is the whitelist of allowed scan_config keys per scanner type.
var validScanConfigKeys = map[string]map[string]bool{
	"trivy":     {"scan_type": true, "severity": true, "ignore_unfixed": true},
	"sonarqube": {"url": true, "token": true, "project_key": true, "branch": true},
	"both":      {"scan_type": true, "severity": true, "ignore_unfixed": true, "url": true, "token": true, "project_key": true, "branch": true},
}

// Scanner orchestrates security scans via Trivy and SonarQube.
type Scanner struct {
	pluginMgr      *engine.Manager
	repo           *repository.SecurityScanRepository
	connectionRepo *repository.ConnectionRepository
	registryRepo   *repository.RegistryRepository
	sem            chan struct{} // concurrency limiter
	mu             sync.Mutex
	cancels        map[uuid.UUID]context.CancelFunc
}

// NewScanner creates a new Scanner.
func NewScanner(pluginMgr *engine.Manager, repo *repository.SecurityScanRepository, connRepo *repository.ConnectionRepository, registryRepo *repository.RegistryRepository) *Scanner {
	return &Scanner{
		pluginMgr:      pluginMgr,
		repo:           repo,
		connectionRepo: connRepo,
		registryRepo:   registryRepo,
		sem:            make(chan struct{}, maxConcurrentScans),
		cancels:        make(map[uuid.UUID]context.CancelFunc),
	}
}

// CancelScan cancels a running scan by its run ID.
func (s *Scanner) CancelScan(runID uuid.UUID) error {
	s.mu.Lock()
	cancel, ok := s.cancels[runID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("no active scan found for run %s", runID)
	}
	cancel()
	return nil
}

// RunScan executes a scan for the given target and persists results.
// It acquires a concurrency slot and blocks if the limit is reached.
func (s *Scanner) RunScan(ctx context.Context, targetID, tenantID uuid.UUID, triggerType string) (*repository.ScanRun, error) {
	// Acquire concurrency slot (blocks if limit reached)
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Load target
	target, err := s.repo.GetScanTarget(ctx, targetID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("load target: %w", err)
	}

	// Validate scan_config keys against the whitelist for this scanner type
	if allowedKeys, ok := validScanConfigKeys[target.ScannerType]; ok {
		for key := range target.ScanConfig {
			if !allowedKeys[key] {
				return nil, fmt.Errorf("unknown scan_config key %q for scanner type %q", key, target.ScannerType)
			}
		}
	}

	// Create scan run record
	run := &repository.ScanRun{
		TenantID:    tenantID,
		TargetID:    targetID,
		ScannerType: target.ScannerType,
		Status:      "pending",
		TriggerType: triggerType,
	}
	if err := s.repo.CreateScanRun(ctx, run); err != nil {
		return nil, fmt.Errorf("create scan run: %w", err)
	}

	// Create a cancellable context for this scan
	scanCtx, scanCancel := context.WithCancel(ctx)

	// Register cancel func so it can be stopped externally
	s.mu.Lock()
	s.cancels[run.ID] = scanCancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.cancels, run.ID)
		s.mu.Unlock()
		scanCancel()
	}()

	// Mark as running
	now := time.Now()
	run.StartedAt = &now
	run.Status = "running"
	if err := s.repo.UpdateScanRun(ctx, run); err != nil {
		return nil, fmt.Errorf("update scan run to running: %w", err)
	}

	startTime := time.Now()

	// Execute scan based on scanner type
	var resultSummary map[string]any
	var resultFull map[string]any
	var scanErr error

	switch target.ScannerType {
	case "trivy":
		resultSummary, resultFull, scanErr = s.runTrivyScan(scanCtx, target)
	case "sonarqube":
		resultSummary, resultFull, scanErr = s.runSonarQubeScan(scanCtx, target)
	case "both":
		// Run both scanners and merge results
		trivySummary, trivyFull, trivyErr := s.runTrivyScan(scanCtx, target)
		sqSummary, sqFull, sqErr := s.runSonarQubeScan(scanCtx, target)

		resultSummary = mergeMaps(trivySummary, sqSummary, "trivy", "sonarqube")
		resultFull = mergeMaps(trivyFull, sqFull, "trivy", "sonarqube")

		if trivyErr != nil && sqErr != nil {
			scanErr = fmt.Errorf("trivy: %v; sonarqube: %v", trivyErr, sqErr)
		} else if trivyErr != nil {
			scanErr = fmt.Errorf("trivy: %v", trivyErr)
		} else if sqErr != nil {
			scanErr = fmt.Errorf("sonarqube: %v", sqErr)
		}
	default:
		scanErr = fmt.Errorf("unknown scanner type: %s", target.ScannerType)
	}

	// Check if the scan was cancelled
	if scanCtx.Err() != nil {
		run.Status = "cancelled"
		cancelledMsg := "scan cancelled by user"
		run.ErrorMessage = &cancelledMsg
		durationMs := int(time.Since(startTime).Milliseconds())
		completedAt := time.Now()
		run.DurationMs = &durationMs
		run.CompletedAt = &completedAt
		if err := s.repo.UpdateScanRun(ctx, run); err != nil {
			return nil, fmt.Errorf("update cancelled scan run: %w", err)
		}
		return run, nil
	}

	// Calculate duration
	durationMs := int(time.Since(startTime).Milliseconds())
	completedAt := time.Now()

	// Update run with results
	if scanErr != nil {
		run.Status = "failed"
		errMsg := scanErr.Error()
		run.ErrorMessage = &errMsg
	} else {
		run.Status = "completed"
		run.ResultSummary = resultSummary
		run.ResultFull = resultFull
	}
	run.DurationMs = &durationMs
	run.CompletedAt = &completedAt

	if err := s.repo.UpdateScanRun(ctx, run); err != nil {
		return nil, fmt.Errorf("update scan run results: %w", err)
	}

	// Update target last scan info
	scanStatus := "completed"
	if scanErr != nil {
		scanStatus = "failed"
	}
	if err := s.repo.UpdateScanTargetLastScan(ctx, targetID, tenantID, scanStatus, resultSummary); err != nil {
		slog.Warn("failed to update target last scan", "error", err)
	}

	return run, nil
}

// runTrivyScan executes a Trivy scan using the trivy binary.
func (s *Scanner) runTrivyScan(ctx context.Context, target *repository.ScanTarget) (summary, full map[string]any, err error) {
	// Build trivy command args
	scanType := "image"
	if st, ok := target.ScanConfig["scan_type"].(string); ok && st != "" {
		scanType = st
	}
	// Map target_type to scan_type
	switch target.TargetType {
	case "git_repo":
		scanType = "repo"
	case "filesystem":
		scanType = "fs"
	case "container":
		scanType = "image"
	}

	// Validate scan_type to prevent command injection via untrusted config
	validScanTypes := map[string]bool{"image": true, "fs": true, "repo": true, "config": true}
	if !validScanTypes[scanType] {
		return nil, nil, fmt.Errorf("invalid trivy scan type: %s (must be image, fs, repo, or config)", scanType)
	}

	severity := "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"
	if sev, ok := target.ScanConfig["severity"].(string); ok && sev != "" {
		// Validate each severity value against the whitelist
		parts := strings.Split(sev, ",")
		for _, p := range parts {
			p = strings.TrimSpace(strings.ToUpper(p))
			if !validSeverities[p] {
				return nil, nil, fmt.Errorf("invalid severity %q (must be one of: UNKNOWN, LOW, MEDIUM, HIGH, CRITICAL)", p)
			}
		}
		severity = sev
	}

	args := []string{
		scanType,
		"--format", "json",
		"--severity", severity,
		"--no-progress",
		"--quiet", // suppress log messages that can corrupt JSON output
	}

	ignoreUnfixed, _ := target.ScanConfig["ignore_unfixed"].(bool)
	if ignoreUnfixed {
		args = append(args, "--ignore-unfixed")
	}

	// Resolve registry credentials for private image scans.
	// When the target was created from the Registry Repo selector, connection_id
	// holds the registry_repository UUID. We look up the stored credentials and
	// prepend the registry hostname to the image reference if missing.
	// Trivy uses environment variables (not CLI flags) for registry auth.
	imageRef := target.TargetRef
	var regUsername, regPassword, regToken string
	if target.ConnectionID != nil && s.registryRepo != nil && scanType == "image" {
		regRepo, regErr := s.registryRepo.GetDecrypted(ctx, *target.ConnectionID, target.TenantID)
		if regErr == nil && regRepo != nil {
			// Prepend registry hostname if the image ref doesn't already include it.
			// A registry hostname contains a '.' (e.g. ghcr.io) or ':' (e.g. localhost:5000)
			// before the first '/', OR the imageRef is just a hostname (no slash at all).
			host := strings.TrimPrefix(strings.TrimSuffix(regRepo.URL, "/"), "https://")
			host = strings.TrimPrefix(host, "http://")
			needsHost := true
			// If imageRef has no slash, check if it's already a hostname (contains '.')
			if idx := strings.Index(imageRef, "/"); idx > 0 {
				beforeSlash := imageRef[:idx]
				if strings.Contains(beforeSlash, ".") || strings.Contains(beforeSlash, ":") {
					needsHost = false
				}
			} else if strings.Contains(imageRef, ".") {
				// No slash found - imageRef might be just a hostname like "registry.example.com"
				needsHost = false
			}
			// Also check if imageRef equals the host (exact match)
			if imageRef == host {
				needsHost = false
			}
			if needsHost && host != "" {
				imageRef = host + "/" + imageRef
			}
			// Collect credentials for env vars (set on cmd below)
			if regRepo.Username != "" && regRepo.Password != "" {
				regUsername = regRepo.Username
				regPassword = regRepo.Password
			} else if regRepo.Token != "" {
				if regRepo.Username != "" {
					regUsername = regRepo.Username
					regPassword = regRepo.Token
				} else {
					regToken = regRepo.Token
				}
			}
			slog.Info("trivy scan using registry credentials", "registry", host, "image", imageRef)
		}
	}

	args = append(args, imageRef)

	cmd := exec.CommandContext(ctx, "trivy", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Pass registry credentials via environment variables.
	// Trivy reads TRIVY_USERNAME/TRIVY_PASSWORD for private registry auth.
	// Must inherit os.Environ() — setting cmd.Env replaces the entire environment.
	if regUsername != "" || regToken != "" {
		cmd.Env = os.Environ()
		if regUsername != "" {
			cmd.Env = append(cmd.Env, "TRIVY_USERNAME="+regUsername, "TRIVY_PASSWORD="+regPassword)
		}
		if regToken != "" {
			cmd.Env = append(cmd.Env, "TRIVY_REGISTRY_TOKEN="+regToken)
		}
	}

	slog.Info("running trivy scan", "target", target.TargetRef, "scan_type", scanType)

	if err := cmd.Run(); err != nil {
		// Trivy returns exit code 0 even with vulnerabilities found
		// Only fail on actual errors (non-zero exit with no JSON output)
		if stdout.Len() == 0 {
			return nil, nil, fmt.Errorf("trivy scan failed: %w: %s", err, stderr.String())
		}
	}

	// Parse trivy JSON output
	var trivyResult map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &trivyResult); err != nil {
		// Include raw output preview for debugging
		raw := stdout.String()
		preview := raw
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		return nil, nil, fmt.Errorf("parse trivy output: %w (raw output: %s)", err, preview)
	}

	// Build summary from results
	summaryCounts := map[string]int{
		"critical": 0, "high": 0, "medium": 0, "low": 0, "unknown": 0,
	}
	totalVulns := 0

	if results, ok := trivyResult["Results"].([]any); ok {
		for _, r := range results {
			if result, ok := r.(map[string]any); ok {
				if vulns, ok := result["Vulnerabilities"].([]any); ok {
					for _, v := range vulns {
						if vuln, ok := v.(map[string]any); ok {
							sev := strings.ToLower(fmt.Sprintf("%v", vuln["Severity"]))
							summaryCounts[sev]++
							totalVulns++
						}
					}
				}
			}
		}
	}
	summaryCounts["total"] = totalVulns

	return mapToAny(summaryCounts), trivyResult, nil
}

// resolveConnectionCredentials fetches URL and token from a linked connection.
// Falls back to scan_config values if no connection is linked or resolution fails.
func (s *Scanner) resolveConnectionCredentials(ctx context.Context, target *repository.ScanTarget, url, token string) (string, string) {
	if target.ConnectionID == nil || s.connectionRepo == nil {
		return url, token
	}

	conn, err := s.connectionRepo.GetDecrypted(ctx, *target.ConnectionID, target.TenantID)
	if err != nil {
		slog.Warn("failed to resolve connection credentials, falling back to scan_config",
			"connection_id", target.ConnectionID, "error", err)
		return url, token
	}

	// Override with connection values if present
	if connURL, ok := conn.Config["url"].(string); ok && connURL != "" {
		url = connURL
	}
	if connToken, ok := conn.Config["token"].(string); ok && connToken != "" {
		token = connToken
	}

	return url, token
}

// runSonarQubeScan executes a SonarQube scan via the plugin engine.
func (s *Scanner) runSonarQubeScan(ctx context.Context, target *repository.ScanTarget) (summary, full map[string]any, err error) {
	if s.pluginMgr == nil {
		return nil, nil, fmt.Errorf("plugin manager not available")
	}

	// Build params from target config
	url, _ := target.ScanConfig["url"].(string)
	token, _ := target.ScanConfig["token"].(string)

	// Resolve credentials from linked connection (preferred over scan_config)
	url, token = s.resolveConnectionCredentials(ctx, target, url, token)

	if url == "" {
		return nil, nil, fmt.Errorf("sonarqube URL not configured (set url in scan_config or link a SonarQube connection)")
	}
	if token == "" {
		return nil, nil, fmt.Errorf("sonarqube token not configured (set token in scan_config or link a SonarQube connection)")
	}

	projectKey, _ := target.ScanConfig["project_key"].(string)
	if projectKey == "" {
		projectKey = target.TargetRef
	}
	branch, _ := target.ScanConfig["branch"].(string)

	// Call get_project_summary action
	params := map[string]any{
		"project_key": projectKey,
	}
	if url != "" {
		params["url"] = url
	}
	if token != "" {
		params["token"] = token
	}
	if branch != "" {
		params["branch"] = branch
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal sonarqube params: %w", err)
	}

	// Build config from connection if available
	config := map[string]string{}
	if url != "" {
		config["url"] = url
	}
	if token != "" {
		config["token"] = token
	}

	slog.Info("running sonarqube scan", "project_key", projectKey)

	output, err := s.pluginMgr.ExecuteAction(ctx, "sonarqube", "get_project_summary", paramsJSON, config)
	if err != nil {
		return nil, nil, fmt.Errorf("sonarqube scan failed: %w", err)
	}

	// Parse result
	var sqResult map[string]any
	if err := json.Unmarshal(output, &sqResult); err != nil {
		return nil, nil, fmt.Errorf("parse sonarqube output: %w", err)
	}

	// Build summary
	summaryMap := map[string]any{}
	if qg, ok := sqResult["quality_gate"].(map[string]any); ok {
		summaryMap["quality_gate_status"] = qg["status"]
	}
	if issues, ok := sqResult["issue_summary"].(map[string]any); ok {
		summaryMap["issues"] = issues
	}
	if measures, ok := sqResult["measures"].(map[string]any); ok {
		summaryMap["coverage"] = measures["coverage"]
		summaryMap["duplication"] = measures["duplicated_lines_density"]
	}

	return summaryMap, sqResult, nil
}

// ScanAllEnabled runs scans for all enabled targets.
func (s *Scanner) ScanAllEnabled(ctx context.Context, tenantID uuid.UUID, triggerType string) ([]*repository.ScanRun, error) {
	targets, err := s.repo.GetEnabledTargets(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	var runs []*repository.ScanRun
	for _, t := range targets {
		run, err := s.RunScan(ctx, t.ID, tenantID, triggerType)
		if err != nil {
			slog.Warn("scan failed for target", "target_id", t.ID, "error", err)
			continue
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// ── Helpers ────────────────────────────────────────────────────

func mapToAny(m map[string]int) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func mergeMaps(a, b map[string]any, keyA, keyB string) map[string]any {
	result := make(map[string]any)
	if a != nil {
		result[keyA] = a
	}
	if b != nil {
		result[keyB] = b
	}
	return result
}
