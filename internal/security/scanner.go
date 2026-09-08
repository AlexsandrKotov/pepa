// Package security provides scan orchestration for Trivy and SonarQube.
package security

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
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
	"trivy":     {"scan_type": true, "severity": true, "ignore_unfixed": true, "vex": true},
	"sonarqube": {"url": true, "token": true, "project_key": true, "branch": true},
	"both":      {"scan_type": true, "severity": true, "ignore_unfixed": true, "url": true, "token": true, "project_key": true, "branch": true, "vex": true},
}

// Scanner orchestrates security scans via Trivy and SonarQube.
type Scanner struct {
	pluginMgr      *engine.Manager
	repo           *repository.SecurityScanRepository
	connectionRepo *repository.ConnectionRepository
	registryRepo   *repository.RegistryRepository
	ignoreRepo     *repository.ScanIgnoreRepository
	sem            chan struct{} // concurrency limiter
	mu             sync.Mutex
	cancels        map[uuid.UUID]context.CancelFunc
}

// NewScanner creates a new Scanner.
func NewScanner(pluginMgr *engine.Manager, repo *repository.SecurityScanRepository, connRepo *repository.ConnectionRepository, registryRepo *repository.RegistryRepository, ignoreRepo *repository.ScanIgnoreRepository) *Scanner {
	return &Scanner{
		pluginMgr:      pluginMgr,
		repo:           repo,
		connectionRepo: connRepo,
		registryRepo:   registryRepo,
		ignoreRepo:     ignoreRepo,
		sem:            make(chan struct{}, maxConcurrentScans),
		cancels:        make(map[uuid.UUID]context.CancelFunc),
	}
}

// CancelScan cancels a running scan by its run ID.
// It always force-updates the database status to "cancelled" to ensure the scan
// doesn't remain stuck in "running" if the goroutine is blocked in cmd.Run().
func (s *Scanner) CancelScan(ctx context.Context, runID uuid.UUID, tenantID uuid.UUID) error {
	s.mu.Lock()
	cancel, ok := s.cancels[runID]
	s.mu.Unlock()
	if ok {
		cancel()
	}

	// Force-update DB status immediately — the goroutine may be stuck in cmd.Run()
	// (e.g. trivy child processes holding pipes open) and never reach the DB update.
	run, err := s.repo.GetScanRun(ctx, runID, tenantID)
	if err != nil {
		return fmt.Errorf("scan run not found: %w", err)
	}
	if run.Status != "running" && run.Status != "pending" {
		return fmt.Errorf("scan is not active (status: %s)", run.Status)
	}

	now := time.Now()
	cancelledMsg := "scan cancelled by user"
	run.Status = "cancelled"
	run.ErrorMessage = &cancelledMsg
	run.CompletedAt = &now
	if run.StartedAt != nil {
		durationMs := int(time.Since(*run.StartedAt).Milliseconds())
		run.DurationMs = &durationMs
	}
	if err := s.repo.UpdateScanRun(ctx, run); err != nil {
		return fmt.Errorf("update cancelled scan run: %w", err)
	}
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
		resultSummary, resultFull, scanErr = s.runTrivyScan(scanCtx, target, run.ID)
	case "sonarqube":
		resultSummary, resultFull, scanErr = s.runSonarQubeScan(scanCtx, target)
	case "both":
		// Run both scanners and merge results
		trivySummary, trivyFull, trivyErr := s.runTrivyScan(scanCtx, target, run.ID)
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
func (s *Scanner) runTrivyScan(ctx context.Context, target *repository.ScanTarget, runID uuid.UUID) (summary, full map[string]any, err error) {
	slog.Info("=== runTrivyScan ENTERED ===", "target_ref", target.TargetRef)
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

	ignoreUnfixed, _ := target.ScanConfig["ignore_unfixed"].(bool)

	// VEX document path for false-positive filtering
	vexPath, _ := target.ScanConfig["vex"].(string)

	slog.Info("runTrivyScan starting", "target_ref", target.TargetRef, "target_type", target.TargetType, "connection_id", target.ConnectionID, "scan_type", scanType)

	// Resolve registry credentials for private image scans.
	// When the target was created from the Registry Repo selector, connection_id
	// holds the registry_repository UUID. We look up the stored credentials and
	// prepend the registry hostname to the image reference if missing.
	// Trivy uses environment variables (not CLI flags) for registry auth.
	imageRef := target.TargetRef
	var regUsername, regPassword, regToken string
	var regHost string
	var regRepoData *repository.RegistryRepo
	if target.ConnectionID != nil && s.registryRepo != nil && scanType == "image" {
		regRepo, regErr := s.registryRepo.GetDecrypted(ctx, *target.ConnectionID, target.TenantID)
		if regErr == nil && regRepo != nil {
			regRepoData = regRepo
			// Prepend registry hostname if the image ref doesn't already include it.
			// A registry hostname contains a '.' (e.g. ghcr.io) or ':' (e.g. localhost:5000)
			// before the first '/', OR the imageRef is just a hostname (no slash at all).
			host := strings.TrimPrefix(strings.TrimSuffix(regRepo.URL, "/"), "https://")
			host = strings.TrimPrefix(host, "http://")
			regHost = host
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

	slog.Info("AFTER registry credential resolution", "regHost", regHost, "imageRef", imageRef, "regRepoData", regRepoData != nil)

	// Detect "scan entire registry" case: target_ref is just the registry hostname
	// with no specific image path. List all images and scan each one.
	slog.Info("checking scan entire registry condition", "regHost", regHost, "imageRef", imageRef, "equal", imageRef == regHost)
	if regHost != "" && imageRef == regHost {
		slog.Info("scanning entire registry, listing images", "registry", regHost)
		images, listErr := s.listRegistryImages(ctx, regRepoData, regHost)
		if listErr != nil {
			return nil, nil, fmt.Errorf("list registry images: %w", listErr)
		}
		if len(images) == 0 {
			return nil, nil, fmt.Errorf("no images found in registry %s", regHost)
		}
		slog.Info("registry image listing complete", "registry", regHost, "count", len(images))

		// Scan each image and aggregate results
		aggregateSummary := map[string]int{
			"critical": 0, "high": 0, "medium": 0, "low": 0, "unknown": 0, "total": 0,
		}
		aggregateFull := map[string]any{
			"registry": regHost,
			"images":   []any{},
		}
		var scanErrors []string
		totalImages := len(images)
		scanStartTime := time.Now()
		for i, img := range images {
			// Check for cancellation before processing each image
			if ctx.Err() != nil {
				slog.Info("scan cancelled, stopping registry scan loop", "processed", i, "total", totalImages)
				break
			}
			// Update progress in database (showing current image being scanned)
			elapsed := time.Since(scanStartTime).Milliseconds()
			// Use max(1, i+1) to avoid division by zero and show progress starting from 1
			imagesProcessed := i + 1
			avgTimePerImage := float64(elapsed) / float64(imagesProcessed)
			remainingImages := totalImages - imagesProcessed
			estimatedRemainingMs := int(avgTimePerImage * float64(remainingImages))
			progressInfo := map[string]any{
				"scanned":                imagesProcessed,
				"total":                  totalImages,
				"current":                img,
				"elapsed_ms":             elapsed,
				"estimated_remaining_ms": estimatedRemainingMs,
			}
			// Update scan run with progress (non-blocking, ignore errors)
			_ = s.repo.UpdateScanRunProgress(ctx, runID, map[string]any{"progress": progressInfo})

			// Get tags for this image to find the correct one to scan
			tags, tagsErr := s.getRegistryImageTags(ctx, regRepoData, regHost, img)
			if tagsErr != nil {
				slog.Warn("failed to get tags for registry image", "image", img, "error", tagsErr)
				scanErrors = append(scanErrors, fmt.Sprintf("%s: failed to get tags: %v", img, tagsErr))
				continue
			}
			if len(tags) == 0 {
				slog.Warn("no tags found for registry image", "image", img)
				scanErrors = append(scanErrors, fmt.Sprintf("%s: no tags found", img))
				continue
			}
			// Use "latest" if available, otherwise use the first tag
			selectedTag := tags[0]
			for _, t := range tags {
				if t == "latest" {
					selectedTag = t
					break
				}
			}
			fullImageRef := regHost + "/" + img + ":" + selectedTag
			slog.Info("scanning registry image", "image", fullImageRef, "progress", fmt.Sprintf("%d/%d", i+1, totalImages))
			// Per-image timeout prevents a single image from hanging the entire registry scan
			imgCtx, imgCancel := context.WithTimeout(ctx, 5*time.Minute)
			imgSummary, imgFull, imgErr := s.scanSingleImage(imgCtx, fullImageRef, scanType, severity, ignoreUnfixed, regUsername, regPassword, regToken)
			imgCancel()
			if imgErr != nil {
				slog.Warn("scan failed for registry image", "image", fullImageRef, "error", imgErr)
				scanErrors = append(scanErrors, fmt.Sprintf("%s:%s: %v", img, selectedTag, imgErr))
				continue
			}
			// Aggregate counts
			for _, key := range []string{"critical", "high", "medium", "low", "unknown", "total"} {
				if v, ok := imgSummary[key].(int); ok {
					aggregateSummary[key] += v
				}
			}
			// Add to full results
			imgResult := map[string]any{
				"image":    img + ":" + selectedTag,
				"summary":  imgSummary,
				"details":  imgFull,
			}
			if existingImages, ok := aggregateFull["images"].([]any); ok {
				aggregateFull["images"] = append(existingImages, imgResult)
			}
		}
		if len(scanErrors) > 0 {
			aggregateFull["errors"] = scanErrors
		}
		return mapToAny(aggregateSummary), aggregateFull, nil
	}

	// Single image scan
	args := []string{
		scanType,
		"--format", "json",
		"--severity", severity,
		"--no-progress",
		"--quiet", // suppress log messages that can corrupt JSON output
	}
	if ignoreUnfixed {
		args = append(args, "--ignore-unfixed")
	}

	// Generate .trivyignore file if there are ignored CVEs for this target
	var ignoreFilePath string
	if s.ignoreRepo != nil {
		ignoreContent, err := s.ignoreRepo.GetIgnoreFileContent(ctx, target.ID, target.TenantID)
		if err != nil {
			slog.Warn("failed to get ignore file content", "target_id", target.ID, "error", err)
		} else if ignoreContent != "" {
			// Write to temp file
			tmpFile, err := os.CreateTemp("", "trivyignore-*.txt")
			if err != nil {
				slog.Warn("failed to create temp ignore file", "error", err)
			} else {
				if _, err := tmpFile.WriteString(ignoreContent); err != nil {
					slog.Warn("failed to write ignore file", "error", err)
					_ = tmpFile.Close()
					_ = os.Remove(tmpFile.Name())
				} else {
					_ = tmpFile.Close()
					ignoreFilePath = tmpFile.Name()
					args = append(args, "--ignorefile", ignoreFilePath)
					slog.Info("using ignore file for scan", "ignore_file", ignoreFilePath, "target_id", target.ID)
					defer func() { _ = os.Remove(ignoreFilePath) }() // Clean up after scan
				}
			}
		}
	}

	// VEX document for false-positive filtering (OpenVEX/CycloneDX format)
	if vexPath != "" {
		// Check if VEX file exists
		if _, err := os.Stat(vexPath); err == nil {
			args = append(args, "--vex", vexPath)
			slog.Info("using VEX document for scan", "vex_path", vexPath)
		} else {
			slog.Warn("VEX document not found, skipping", "vex_path", vexPath)
		}
	}
	args = append(args, imageRef)

	cmd := exec.CommandContext(ctx, "trivy", args...) //nolint:gosec // #nosec // G204: trivy is an admin-configured binary
	// Kill the entire process group on cancellation to prevent child processes
	// (e.g. image pull helpers) from keeping pipes open and blocking cmd.Run() forever.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // #nosec G104 //nolint:errcheck // negative PID kills process group
		}
		return nil
	}
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

// scanSingleImage runs a Trivy scan on a single image and returns the parsed results.
func (s *Scanner) scanSingleImage(ctx context.Context, imageRef, scanType, severity string, ignoreUnfixed bool, regUsername, regPassword, regToken string) (summary, full map[string]any, err error) {
	args := []string{
		scanType,
		"--format", "json",
		"--severity", severity,
		"--no-progress",
		"--quiet",
	}
	if ignoreUnfixed {
		args = append(args, "--ignore-unfixed")
	}
	args = append(args, imageRef)

	cmd := exec.CommandContext(ctx, "trivy", args...) //nolint:gosec // #nosec // G204: trivy is an admin-configured binary
	// Kill the entire process group on cancellation to prevent child processes
	// from keeping pipes open and blocking cmd.Run() forever.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // #nosec G104 //nolint:errcheck // negative PID kills process group
		}
		return nil
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if regUsername != "" || regToken != "" {
		cmd.Env = os.Environ()
		if regUsername != "" {
			cmd.Env = append(cmd.Env, "TRIVY_USERNAME="+regUsername, "TRIVY_PASSWORD="+regPassword)
		}
		if regToken != "" {
			cmd.Env = append(cmd.Env, "TRIVY_REGISTRY_TOKEN="+regToken)
		}
	}

	if cmdErr := cmd.Run(); cmdErr != nil {
		if stdout.Len() == 0 {
			return nil, nil, fmt.Errorf("trivy scan failed: %w: %s", cmdErr, stderr.String())
		}
	}

	var trivyResult map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &trivyResult); err != nil {
		return nil, nil, fmt.Errorf("parse trivy output: %w", err)
	}

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

// listRegistryImages discovers all images in a registry using the Docker Registry V2 API
// with a GitLab API fallback (GitLab restricts /v2/_catalog to admin users).
func (s *Scanner) listRegistryImages(ctx context.Context, repo *repository.RegistryRepo, host string) ([]string, error) {
	baseURL := strings.TrimSuffix(repo.URL, "/")
	client := &http.Client{Timeout: 30 * time.Second}

	// Step 1: Get auth challenge
	challenge, challengeErr := s.getRegistryAuthChallenge(ctx, client, repo)
	if challengeErr != nil {
		slog.Warn("registry auth challenge failed", "host", host, "error", challengeErr.Error())
	}

	// Step 2: Try /v2/_catalog with a catalog-scoped token
	var repositories []string
	catalogOK := false
	if challenge != nil {
		token, tokenErr := s.getRegistryScopedToken(ctx, client, repo, challenge, "registry:catalog:*")
		if tokenErr == nil && token != "" {
			req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/v2/_catalog", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				var catalog struct {
					Repositories []string `json:"repositories"`
				}
				if json.NewDecoder(resp.Body).Decode(&catalog) == nil {
					repositories = catalog.Repositories
					catalogOK = true
				}
				_ = resp.Body.Close()
			}
		}
	} else if challengeErr == nil {
		req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/v2/_catalog", nil)
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			var catalog struct {
				Repositories []string `json:"repositories"`
			}
			if json.NewDecoder(resp.Body).Decode(&catalog) == nil {
				repositories = catalog.Repositories
				catalogOK = true
			}
			_ = resp.Body.Close()
		}
	}

	// Step 3: GitLab API fallback
	if !catalogOK && challenge != nil {
		slog.Debug("catalog listing failed, trying GitLab API fallback", "host", host)
		gitlabRepos, err := s.listGitLabContainerRepos(ctx, client, challenge.realm, repo)
		if err != nil {
			return nil, fmt.Errorf("GitLab API fallback failed: %w", err)
		}
		repositories = gitlabRepos
	}

	sort.Strings(repositories)
	return repositories, nil
}

// registryAuthHeader sets authentication on an HTTP request for a Docker registry.
func registryAuthHeader(req *http.Request, repo *repository.RegistryRepo) {
	if repo.Token != "" {
		if repo.Username != "" {
			req.SetBasicAuth(repo.Username, repo.Token)
		} else {
			req.Header.Set("Authorization", "Bearer "+repo.Token)
		}
	} else if repo.Username != "" && repo.Password != "" {
		req.SetBasicAuth(repo.Username, repo.Password)
	}
}

type registryAuthChallenge struct {
	realm   string
	service string
}

// getRegistryAuthChallenge pings /v2/ and parses the Www-Authenticate header.
func (s *Scanner) getRegistryAuthChallenge(ctx context.Context, client *http.Client, repo *repository.RegistryRepo) (*registryAuthChallenge, error) {
	baseURL := strings.TrimSuffix(repo.URL, "/")
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/v2/", nil)
	if err != nil {
		return nil, fmt.Errorf("create ping request: %w", err)
	}
	registryAuthHeader(req, repo)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registry ping failed: %w", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil, nil
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	wwwAuth := resp.Header.Get("Www-Authenticate")
	if wwwAuth == "" {
		return nil, fmt.Errorf("no Www-Authenticate header in 401 response")
	}

	realm := extractRegistryAuthParam(wwwAuth, "realm")
	if realm == "" {
		return nil, fmt.Errorf("no realm in Www-Authenticate header")
	}

	return &registryAuthChallenge{
		realm:   realm,
		service: extractRegistryAuthParam(wwwAuth, "service"),
	}, nil
}

// getRegistryScopedToken requests a JWT token from the registry auth endpoint.
func (s *Scanner) getRegistryScopedToken(ctx context.Context, client *http.Client, repo *repository.RegistryRepo, challenge *registryAuthChallenge, scope string) (string, error) {
	u, err := url.Parse(challenge.realm)
	if err != nil {
		return "", fmt.Errorf("invalid realm URL: %w", err)
	}
	q := u.Query()
	if challenge.service != "" {
		q.Set("service", challenge.service)
	}
	if scope != "" {
		q.Set("scope", scope)
	}
	u.RawQuery = q.Encode()

	tokenReq, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	registryAuthHeader(tokenReq, repo)

	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer func() { _ = tokenResp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(tokenResp.Body, 1<<20))
	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d: %s", tokenResp.StatusCode, string(body)[:min(len(body), 200)])
	}

	var tokenData struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenData); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if tokenData.Token != "" {
		return tokenData.Token, nil
	}
	return tokenData.AccessToken, nil
}

// listGitLabContainerRepos uses the GitLab API to discover container repositories.
func (s *Scanner) listGitLabContainerRepos(ctx context.Context, client *http.Client, realm string, repo *repository.RegistryRepo) ([]string, error) {
	u, err := url.Parse(realm)
	if err != nil {
		return nil, fmt.Errorf("invalid realm: %w", err)
	}
	gitlabBase := fmt.Sprintf("%s://%s", u.Scheme, u.Host)

	var allRepos []string
	page := 1
	for {
		projectsURL := fmt.Sprintf("%s/api/v4/projects?per_page=100&page=%d&simple=true&order_by=name", gitlabBase, page)
		req, _ := http.NewRequestWithContext(ctx, "GET", projectsURL, nil)
		if repo.Token != "" {
			req.Header.Set("PRIVATE-TOKEN", repo.Token)
		} else if repo.Username != "" && repo.Password != "" {
			req.SetBasicAuth(repo.Username, repo.Password)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("gitlab projects request failed: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			return nil, fmt.Errorf("gitlab projects returned %d: %s", resp.StatusCode, string(body)[:min(len(body), 200)])
		}

		var projects []struct {
			ID int `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("parse projects: %w", err)
		}
		_ = resp.Body.Close()

		if len(projects) == 0 {
			break
		}

		for _, proj := range projects {
			reposURL := fmt.Sprintf("%s/api/v4/projects/%d/registry/repositories", gitlabBase, proj.ID)
			req, _ := http.NewRequestWithContext(ctx, "GET", reposURL, nil)
			if repo.Token != "" {
				req.Header.Set("PRIVATE-TOKEN", repo.Token)
			} else if repo.Username != "" && repo.Password != "" {
				req.SetBasicAuth(repo.Username, repo.Password)
			}

			resp, err := client.Do(req)
			if err != nil {
				slog.Warn("gitlab registry repos request failed", "project_id", proj.ID, "error", err)
				continue
			}
			if resp.StatusCode != http.StatusOK {
				_ = resp.Body.Close()
				continue
			}

			var repos []struct {
				Path string `json:"path"`
			}
			if json.NewDecoder(resp.Body).Decode(&repos) == nil {
				for _, r := range repos {
					if r.Path != "" {
						allRepos = append(allRepos, r.Path)
					}
				}
			}
			_ = resp.Body.Close()
		}

		if len(projects) < 100 {
			break
		}
		page++
	}

	return allRepos, nil
}

// getRegistryImageTags fetches the list of tags for a specific image from the registry.
func (s *Scanner) getRegistryImageTags(ctx context.Context, repo *repository.RegistryRepo, host, imageName string) ([]string, error) {
	baseURL := strings.TrimSuffix(repo.URL, "/")
	client := &http.Client{Timeout: 30 * time.Second}

	// Get auth challenge and token
	challenge, err := s.getRegistryAuthChallenge(ctx, client, repo)
	if err != nil {
		return nil, fmt.Errorf("auth challenge: %w", err)
	}

	var token string
	if challenge != nil {
		scope := fmt.Sprintf("repository:%s:pull", imageName)
		token, err = s.getRegistryScopedToken(ctx, client, repo, challenge, scope)
		if err != nil {
			return nil, fmt.Errorf("get token: %w", err)
		}
	}

	tagsURL := fmt.Sprintf("%s/v2/%s/tags/list", baseURL, imageName)
	req, _ := http.NewRequestWithContext(ctx, "GET", tagsURL, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		registryAuthHeader(req, repo)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tags request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("tags endpoint returned %d: %s", resp.StatusCode, string(body)[:min(len(body), 200)])
	}

	var tagsData struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tagsData); err != nil {
		return nil, fmt.Errorf("parse tags response: %w", err)
	}

	return tagsData.Tags, nil
}

// extractRegistryAuthParam extracts a parameter value from a Www-Authenticate header.
func extractRegistryAuthParam(header, param string) string {
	search := param + `="`
	idx := strings.Index(header, search)
	if idx < 0 {
		return ""
	}
	start := idx + len(search)
	end := strings.Index(header[start:], `"`)
	if end < 0 {
		return ""
	}
	return header[start : start+end]
}

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

// GetDatabaseStatus returns the status of Trivy databases.
func (s *Scanner) GetDatabaseStatus(ctx context.Context) map[string]any {
	status := map[string]any{
		"trivy_db":      map[string]any{"available": false},
		"java_db":       map[string]any{"available": false},
		"trivy_version": "unknown",
	}

	// Get trivy version
	cmd := exec.CommandContext(ctx, "trivy", "--version")
	var versionOut bytes.Buffer
	cmd.Stdout = &versionOut
	if err := cmd.Run(); err == nil {
		status["trivy_version"] = strings.TrimSpace(versionOut.String())
	}

	// Check DB cache directory
	cacheDir := os.Getenv("TRIVY_CACHE_DIR")
	if cacheDir == "" {
		cacheDir = "/tmp/trivy-cache"
	}
	// Sanitize cache directory path to prevent path traversal
	cacheDir = filepath.Clean(cacheDir)

	// Check trivy-db metadata
	dbMetadataPath := filepath.Join(cacheDir, "db", "metadata.json")
	if info, err := os.Stat(dbMetadataPath); err == nil {
		dbInfo := map[string]any{
			"available":  true,
			"updated_at": info.ModTime(),
		}
		// Report the actual trivy.db size, not the tiny metadata.json
		trivyDBPath := filepath.Join(cacheDir, "db", "trivy.db")
		if dbFile, err := os.Stat(trivyDBPath); err == nil {
			dbInfo["size_bytes"] = dbFile.Size()
		} else {
			dbInfo["size_bytes"] = info.Size()
		}
		status["trivy_db"] = dbInfo
	}

	// Check java-db metadata
	javaDBPath := filepath.Join(cacheDir, "java-db", "trivy-java.db")
	if info, err := os.Stat(javaDBPath); err == nil {
		status["java_db"] = map[string]any{
			"available":  true,
			"updated_at": info.ModTime(),
			"size_bytes": info.Size(),
		}
	}

	return status
}
