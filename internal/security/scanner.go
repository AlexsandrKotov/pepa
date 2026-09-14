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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/hostpath"
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
// Credentials are deliberately absent: SonarQube url/token live in a Connection,
// and scan_config is stored as plain JSONB.
var validScanConfigKeys = map[string]map[string]bool{
	"trivy":     {"scan_type": true, "severity": true, "ignore_unfixed": true, "vex": true, "db_repository": true, "java_db_repository": true},
	"sonarqube": {"project_key": true, "branch": true, "severity": true, "stale_after_hours": true, "source_ci_url": true},
	"both":      {"scan_type": true, "severity": true, "ignore_unfixed": true, "vex": true, "db_repository": true, "java_db_repository": true, "project_key": true, "branch": true, "stale_after_hours": true, "source_ci_url": true},
}

// ValidateScanConfig reports whether a scan_config uses only keys the scanner
// understands. It is checked when the target is saved, so an unsupported key (or
// a leftover url/token credential) is rejected with a clear message instead of
// failing every later scan.
func ValidateScanConfig(scannerType string, cfg map[string]any) error {
	allowed, ok := validScanConfigKeys[scannerType]
	if !ok {
		// Unknown scanner types are rejected by the API layer.
		return nil
	}
	for key := range cfg {
		if !allowed[key] {
			if key == "url" || key == "token" {
				return fmt.Errorf("scan_config.%s is not supported: SonarQube url and token live in a Connection", key)
			}
			return fmt.Errorf("unknown scan_config key %q for scanner type %q", key, scannerType)
		}
	}
	return nil
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

	// DB manager fields — the API server acts as the DB cache warmer,
	// downloading and refreshing Trivy databases without a separate container.
	dbRepo            string        // OCI registry for trivy-db (env: TRIVY_DB_REPOSITORY)
	javaDBRepo        string        // OCI registry for trivy-java-db (env: TRIVY_JAVA_DB_REPOSITORY)
	dbRefreshInterval time.Duration // how often to refresh DBs (env: TRIVY_DB_UPDATE_INTERVAL)

	// sonarTimeout bounds a SonarQube report collection. It is a handful of REST
	// calls, not an analysis, so it is far shorter than the Trivy scan timeout
	// (env: SONAR_SCAN_TIMEOUT, default 2m).
	sonarTimeout time.Duration

	// sonarDefaultStaleHours is the freshness budget reported to the UI when the
	// target does not configure its own (env: SONAR_DEFAULT_STALE_HOURS, default 24).
	sonarDefaultStaleHours int
}

// NewScanner creates a new Scanner.
func NewScanner(pluginMgr *engine.Manager, repo *repository.SecurityScanRepository, connRepo *repository.ConnectionRepository, registryRepo *repository.RegistryRepository, ignoreRepo *repository.ScanIgnoreRepository) *Scanner {
	s := &Scanner{
		pluginMgr:      pluginMgr,
		repo:           repo,
		connectionRepo: connRepo,
		registryRepo:   registryRepo,
		ignoreRepo:     ignoreRepo,
		sem:            make(chan struct{}, maxConcurrentScans),
		cancels:        make(map[uuid.UUID]context.CancelFunc),
	}

	// Initialize DB manager from environment variables.
	s.dbRepo = os.Getenv("TRIVY_DB_REPOSITORY")
	if s.dbRepo == "" {
		s.dbRepo = "public.ecr.aws/aquasecurity/trivy-db"
	}
	s.javaDBRepo = os.Getenv("TRIVY_JAVA_DB_REPOSITORY")
	if s.javaDBRepo == "" {
		s.javaDBRepo = "public.ecr.aws/aquasecurity/trivy-java-db"
	}
	intervalSec := 21600 // default: 6 hours
	if v := os.Getenv("TRIVY_DB_UPDATE_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			intervalSec = n
		}
	}
	s.dbRefreshInterval = time.Duration(intervalSec) * time.Second

	s.sonarTimeout = 2 * time.Minute
	if v := os.Getenv("SONAR_SCAN_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			s.sonarTimeout = d
		} else if n, err := strconv.Atoi(v); err == nil && n > 0 {
			s.sonarTimeout = time.Duration(n) * time.Second
		} else {
			slog.Warn("invalid SONAR_SCAN_TIMEOUT, using default", "value", v, "default", s.sonarTimeout)
		}
	}
	s.sonarDefaultStaleHours = 24
	if v := os.Getenv("SONAR_DEFAULT_STALE_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			s.sonarDefaultStaleHours = n
		} else {
			slog.Warn("invalid SONAR_DEFAULT_STALE_HOURS, using default", "value", v, "default", s.sonarDefaultStaleHours)
		}
	}

	return s
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
	var reportURL string
	var scanErr error

	switch target.ScannerType {
	case "trivy":
		resultSummary, resultFull, scanErr = s.runTrivyScan(scanCtx, target, run.ID)
	case "sonarqube":
		resultSummary, resultFull, reportURL, scanErr = s.runSonarQubeScan(scanCtx, target)
	case "both":
		// Run both scanners and merge results
		trivySummary, trivyFull, trivyErr := s.runTrivyScan(scanCtx, target, run.ID)
		sqSummary, sqFull, sqReportURL, sqErr := s.runSonarQubeScan(scanCtx, target)
		reportURL = sqReportURL

		// Severity counters are shared keys and must add up; anything else is
		// namespaced per scanner so neither report overwrites the other.
		resultSummary = mergeScanSummaries(trivySummary, sqSummary)
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
		// SonarQube findings live in the SonarQube UI; link the project dashboard
		// so the PEPA report can always be traced back to its source.
		if reportURL != "" {
			run.ReportURL = &reportURL
		}
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

	// Per-target DB repository override (allows different scan targets to use different DB sources)
	if dbRepo, ok := target.ScanConfig["db_repository"].(string); ok && dbRepo != "" {
		s.SetDBRepository(dbRepo, "")
	}
	if javaDBRepo, ok := target.ScanConfig["java_db_repository"].(string); ok && javaDBRepo != "" {
		s.SetDBRepository("", javaDBRepo)
	}

	slog.Info("runTrivyScan starting", "target_ref", target.TargetRef, "target_type", target.TargetType, "connection_id", target.ConnectionID, "scan_type", scanType)

	// Resolve registry credentials for private image scans.
	// When the target was created from the Registry Repo selector, connection_id
	// holds the registry_repository UUID. We look up the stored credentials and
	// prepend the registry hostname to the image reference if missing.
	// Trivy uses environment variables (not CLI flags) for registry auth.
	imageRef := target.TargetRef

	// For filesystem targets, resolve the path through hostpath to handle
	// Docker container path translation and validate it's within HOST_DATA_DIR.
	if target.TargetType == "filesystem" {
		hostDataDir := os.Getenv("HOST_DATA_DIR")
		resolved, resolveErr := hostpath.Resolve(imageRef, hostDataDir)
		if resolveErr != nil {
			return nil, nil, fmt.Errorf("filesystem target path resolution failed: %w", resolveErr)
		}
		// Verify the resolved path actually exists inside the container.
		if info, statErr := os.Stat(resolved); statErr != nil || !info.IsDir() { // #nosec G703 //nolint:gosec // resolved path is validated within HOST_DATA_DIR by hostpath.Resolve
			slog.Error("filesystem scan target not accessible inside container",
				"target_ref", imageRef,
				"resolved_path", resolved,
				"host_data_dir", hostDataDir,
				"error", statErr,
			)
			return nil, nil, fmt.Errorf("scan target path does not exist inside the container: %s (resolved to %s). Ensure HOST_DATA_DIR is mounted correctly", imageRef, resolved)
		}
		imageRef = resolved
		slog.Info("filesystem target path resolved", "original", target.TargetRef, "resolved", resolved)
	}

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
				"image":   img + ":" + selectedTag,
				"summary": imgSummary,
				"details": imgFull,
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

	// Single image scan — apply a timeout so a hung image pull can't block forever.
	scanTimeout := 10 * time.Minute
	singleCtx, singleCancel := context.WithTimeout(ctx, scanTimeout)
	defer singleCancel()

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
					defer func() { _ = os.Remove(ignoreFilePath) }()
				}
			}
		}
	}

	// VEX document for false-positive filtering (OpenVEX/CycloneDX format)
	if vexPath != "" {
		if _, err := os.Stat(vexPath); err == nil {
			args = append(args, "--vex", vexPath)
			slog.Info("using VEX document for scan", "vex_path", vexPath)
		} else {
			slog.Warn("VEX document not found, skipping", "vex_path", vexPath)
		}
	}
	args = append(args, imageRef)

	cmd := exec.CommandContext(singleCtx, "trivy", args...) //nolint:gosec // #nosec G204 G702: trivy is an admin-configured binary; args are built from controlled image refs
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

	slog.Info("running trivy scan", "target", target.TargetRef, "scan_type", scanType, "timeout", scanTimeout)

	if err := cmd.Run(); err != nil {
		if singleCtx.Err() == context.DeadlineExceeded {
			return nil, nil, fmt.Errorf("trivy scan timed out after %s", scanTimeout)
		}
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

// sonarEndpoint carries everything the plugin needs to reach one SonarQube server.
type sonarEndpoint struct {
	URL      string
	Token    string
	Insecure string
}

// resolveSonarCredentials reads SonarQube credentials from the linked Connection.
// There is deliberately no scan_config fallback: scan_config is stored as plain
// JSONB, and a silent fallback used to turn a resolution failure into an empty
// report instead of an error. The tenant is part of the lookup so a connection
// belonging to another tenant can never be resolved.
func (s *Scanner) resolveSonarCredentials(ctx context.Context, target *repository.ScanTarget) (*sonarEndpoint, error) {
	if target.ConnectionID == nil {
		return nil, fmt.Errorf("SonarQube credentials come from a Connection — link a SonarQube connection to this target")
	}
	return s.resolveSonarConnection(ctx, *target.ConnectionID, target.TenantID)
}

// resolveSonarConnection loads and validates a SonarQube Connection for a tenant.
// The tenant is part of every lookup so a connection belonging to another tenant
// can never be resolved.
func (s *Scanner) resolveSonarConnection(ctx context.Context, connectionID, tenantID uuid.UUID) (*sonarEndpoint, error) {
	if s.connectionRepo == nil {
		return nil, fmt.Errorf("connection repository not available")
	}

	conn, err := s.connectionRepo.GetDecrypted(ctx, connectionID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("resolve SonarQube connection: %w", err)
	}
	if conn.Type != repository.ConnectionSonarQube {
		return nil, fmt.Errorf("connection %q has type %q, expected %q", conn.Name, conn.Type, repository.ConnectionSonarQube)
	}

	ep := &sonarEndpoint{}
	ep.URL, _ = conn.Config["url"].(string)
	ep.Token, _ = conn.Config["token"].(string)
	// The Connections UI may store the flag as a JSON boolean or a string.
	switch v := conn.Config["insecure"].(type) {
	case bool:
		ep.Insecure = strconv.FormatBool(v)
	case string:
		ep.Insecure = v
	}

	if ep.URL == "" {
		return nil, fmt.Errorf("connection %q has no SonarQube url configured", conn.Name)
	}
	if ep.Token == "" {
		return nil, fmt.Errorf("connection %q has no SonarQube token configured", conn.Name)
	}
	return ep, nil
}

// SonarProject is one analysable project inside an external SonarQube instance.
type SonarProject struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Qualifier string `json:"qualifier"`
}

// ListSonarProjects returns the projects of a SonarQube Connection so the UI can
// offer real project keys instead of asking the user to guess one. The returned
// flag reports truncation, which the UI must surface rather than presenting a
// partial list as complete.
func (s *Scanner) ListSonarProjects(ctx context.Context, connectionID, tenantID uuid.UUID, query string) ([]SonarProject, bool, error) {
	if s.pluginMgr == nil {
		return nil, false, fmt.Errorf("plugin manager not available")
	}
	ep, err := s.resolveSonarConnection(ctx, connectionID, tenantID)
	if err != nil {
		return nil, false, err
	}

	config := map[string]string{"url": ep.URL, "token": ep.Token}
	if ep.Insecure != "" {
		config["insecure"] = ep.Insecure
	}
	paramsJSON, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		return nil, false, fmt.Errorf("marshal list_projects params: %w", err)
	}

	listCtx, cancel := context.WithTimeout(ctx, s.sonarTimeout)
	defer cancel()

	output, err := s.pluginMgr.ExecuteAction(listCtx, "sonarqube", "list_projects", paramsJSON, config)
	if err != nil {
		return nil, false, err
	}
	var result struct {
		Projects  []SonarProject `json:"projects"`
		Truncated bool           `json:"truncated"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, false, fmt.Errorf("parse sonarqube projects: %w", err)
	}
	if result.Projects == nil {
		result.Projects = []SonarProject{}
	}
	return result.Projects, result.Truncated, nil
}

// TransitionSonarIssue asks an external SonarQube to move one of its issues —
// mark it a false positive, won't fix, or reopen it. PEPA keeps no copy of that
// state: the transition happens upstream and the next collected report reflects it.
func (s *Scanner) TransitionSonarIssue(ctx context.Context, connectionID, tenantID uuid.UUID, issueKey, transition string) error {
	if s.pluginMgr == nil {
		return fmt.Errorf("plugin manager not available")
	}
	if strings.TrimSpace(issueKey) == "" {
		return fmt.Errorf("sonarqube issue key is required")
	}
	ep, err := s.resolveSonarConnection(ctx, connectionID, tenantID)
	if err != nil {
		return err
	}

	config := map[string]string{"url": ep.URL, "token": ep.Token}
	if ep.Insecure != "" {
		config["insecure"] = ep.Insecure
	}
	paramsJSON, err := json.Marshal(map[string]any{
		"issue_key":  issueKey,
		"transition": transition,
	})
	if err != nil {
		return fmt.Errorf("marshal transition params: %w", err)
	}

	actionCtx, cancel := context.WithTimeout(ctx, s.sonarTimeout)
	defer cancel()

	// The plugin owns the list of transitions SonarQube accepts and rejects
	// anything else, so an unknown verb surfaces as a clear upstream error here.
	_, err = s.pluginMgr.ExecuteAction(actionCtx, "sonarqube", "transition_issue", paramsJSON, config)
	return err
}

// sonarConfigString reads an optional string from a target's scan_config.
func sonarConfigString(cfg map[string]any, key string) string {
	if cfg == nil {
		return ""
	}
	v, _ := cfg[key].(string)
	return strings.TrimSpace(v)
}

// sonarConfigInt reads an optional number from scan_config, tolerating the
// float64/string shapes JSONB decoding produces.
func sonarConfigInt(cfg map[string]any, key string) int {
	if cfg == nil {
		return 0
	}
	switch v := cfg[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

// sonarProjectKey resolves which SonarQube project a target refers to.
// target_ref is only a project key for sonarqube_project targets; for every
// other type it is a git URL or a filesystem path, which SonarQube would
// reject with an unrelated "resource not found" error.
func SonarProjectKey(target *repository.ScanTarget) string {
	if k := sonarConfigString(target.ScanConfig, "project_key"); k != "" {
		return k
	}
	if target.TargetType == "sonarqube_project" {
		return strings.TrimSpace(target.TargetRef)
	}
	return ""
}

// ValidateSonarProjectKey checks that a SonarQube target resolves to exactly one
// project key. Exported for the API layer so a target that can never produce a
// report is rejected while it is being saved, not on every later scan.
func ValidateSonarProjectKey(target *repository.ScanTarget) error {
	if SonarProjectKey(target) == "" {
		return fmt.Errorf("a SonarQube target needs the project key — it identifies the project inside SonarQube, PEPA does not analyze code itself")
	}
	configured := sonarConfigString(target.ScanConfig, "project_key")
	if configured != "" && target.TargetType == "sonarqube_project" {
		if ref := strings.TrimSpace(target.TargetRef); ref != "" && ref != configured {
			return fmt.Errorf("scan_config.project_key %q conflicts with target_ref %q", configured, ref)
		}
	}
	return nil
}

// sonarGateCondition is one quality gate condition as reported by SonarQube.
type sonarGateCondition struct {
	Status         string `json:"status"`
	MetricKey      string `json:"metric_key"`
	Comparator     string `json:"comparator"`
	ErrorThreshold string `json:"error_threshold"`
	ActualValue    string `json:"actual_value"`
}

// sonarQualityGate is the project quality gate verdict.
type sonarQualityGate struct {
	Status     string               `json:"status"` // OK, WARN, ERROR, NONE
	Conditions []sonarGateCondition `json:"conditions"`
}

// sonarMetric is a single project measure.
type sonarMetric struct {
	Metric string `json:"metric"`
	Value  string `json:"value"`
}

// sonarMeasures holds the project metrics fetched by the plugin.
type sonarMeasures struct {
	Metrics []sonarMetric `json:"metrics"`
}

// sonarIssueSummary counts issues per type and per SonarQube severity.
type sonarIssueSummary struct {
	Total           int            `json:"total"`
	Bugs            int            `json:"bugs"`
	Vulnerabilities int            `json:"vulnerabilities"`
	CodeSmells      int            `json:"code_smells"`
	BySeverity      map[string]int `json:"by_severity"`
}

// sonarSnapshot mirrors the get_project_summary output of the sonarqube plugin.
type sonarSnapshot struct {
	ProjectKey   string             `json:"project_key"`
	Branch       string             `json:"branch"`
	QualityGate  *sonarQualityGate  `json:"quality_gate"`
	Measures     *sonarMeasures     `json:"measures"`
	IssueSummary *sonarIssueSummary `json:"issue_summary"`
	AnalysisDate string             `json:"analysis_date"`
	FetchedAt    string             `json:"fetched_at"`
	Warnings     []string           `json:"warnings"`
}

// sonarIssue mirrors one issue from the plugin's get_issues output.
type sonarIssue struct {
	Key           string `json:"key"`
	Rule          string `json:"rule"`
	Severity      string `json:"severity"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	Component     string `json:"component"`
	ComponentPath string `json:"component_path"`
	Line          int    `json:"line"`
	Type          string `json:"type"`
	Effort        string `json:"effort"`
	Debt          string `json:"debt"`
}

// sonarIssuePage mirrors the paginated get_issues envelope.
type sonarIssuePage struct {
	Issues     []sonarIssue   `json:"issues"`
	Total      int            `json:"total"`
	Fetched    int            `json:"fetched"`
	Truncated  bool           `json:"truncated"`
	BySeverity map[string]int `json:"by_severity"`
	ByType     map[string]int `json:"by_type"`
}

// sonarMetricValue returns the raw value of a project measure.
func (s *sonarSnapshot) sonarMetricValue(name string) string {
	if s == nil || s.Measures == nil {
		return ""
	}
	for _, m := range s.Measures.Metrics {
		if m.Metric == name {
			return m.Value
		}
	}
	return ""
}

// sonarNumericMetric reads a counter measure, returning 0 when absent or unparsable.
func (s *sonarSnapshot) sonarNumericMetric(name string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s.sonarMetricValue(name)))
	return n
}

// sonarFloatMetric reads a percentage-style measure, returning 0 when absent.
func (s *sonarSnapshot) sonarFloatMetric(name string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s.sonarMetricValue(name)), 64)
	return f
}

// runSonarQubeScan collects a report snapshot from an external SonarQube.
// PEPA never analyses code itself — no subprocess, no clone, no container: the
// analysis is produced by SonarQube (normally in CI) and the latest one is read
// here over REST, then normalised into the same report shape Trivy produces so
// findings, ignores, exports and dashboards stay scanner-agnostic.
func (s *Scanner) runSonarQubeScan(ctx context.Context, target *repository.ScanTarget) (summary, full map[string]any, reportURL string, err error) {
	if s.pluginMgr == nil {
		return nil, nil, "", fmt.Errorf("plugin manager not available")
	}

	ep, err := s.resolveSonarCredentials(ctx, target)
	if err != nil {
		return nil, nil, "", err
	}

	projectKey := SonarProjectKey(target)
	if projectKey == "" {
		return nil, nil, "", fmt.Errorf("SonarQube project key is empty — set the Project Key on the target (the identifier of the project inside SonarQube)")
	}
	branch := sonarConfigString(target.ScanConfig, "branch")

	// A snapshot is a handful of API calls, so it gets its own short deadline
	// instead of the 10-minute budget a Trivy scan is allowed to take.
	collectCtx, cancel := context.WithTimeout(ctx, s.sonarTimeout)
	defer cancel()

	// project_key travels with the config: the plugin rebuilds itself from this
	// map per request, and an incomplete config used to be executed by a
	// zero-value plugin instance with no HTTP client at all.
	config := map[string]string{
		"url":         ep.URL,
		"token":       ep.Token,
		"project_key": projectKey,
	}
	if ep.Insecure != "" {
		config["insecure"] = ep.Insecure
	}
	if branch != "" {
		config["branch"] = branch
	}

	params := map[string]any{"project_key": projectKey}
	if branch != "" {
		params["branch"] = branch
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, nil, "", fmt.Errorf("marshal sonarqube params: %w", err)
	}

	slog.Info("collecting sonarqube report", "project_key", projectKey, "branch", branch, "target", target.Name)

	snapshotRaw, err := s.pluginMgr.ExecuteAction(collectCtx, "sonarqube", "get_project_summary", paramsJSON, config)
	if err != nil {
		return nil, nil, "", fmt.Errorf("sonarqube report collection failed: %w", err)
	}
	var snapshot sonarSnapshot
	if err := json.Unmarshal(snapshotRaw, &snapshot); err != nil {
		return nil, nil, "", fmt.Errorf("parse sonarqube summary: %w", err)
	}

	page, err := s.fetchSonarIssues(collectCtx, paramsJSON, config)
	if err != nil {
		// The quality gate and metrics are still a valid report; say so instead
		// of failing the whole run for one unavailable endpoint.
		snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("issues unavailable: %v", err))
	}

	summary, full = buildSonarReport(snapshot, page, ep.URL, projectKey, s.sonarIgnoreSet(collectCtx, target))
	// Staleness is a property of how the target is polled, not of SonarQube itself,
	// so the report carries the budget for the UI to flag an outdated snapshot.
	hours := sonarConfigInt(target.ScanConfig, "stale_after_hours")
	if hours <= 0 {
		hours = s.sonarDefaultStaleHours
	}
	summary["stale_after_hours"] = hours
	if ciURL := sanitizeHTTPURL(sonarConfigString(target.ScanConfig, "source_ci_url")); ciURL != "" {
		summary["source_ci_url"] = ciURL
	}
	// Issue transitions are addressed by connection, and the report viewer only
	// has this run to go by.
	if target.ConnectionID != nil {
		summary["connection_id"] = target.ConnectionID.String()
	}
	return summary, full, sonarDashboardURL(ep.URL, projectKey), nil
}

// fetchSonarIssues pulls the finding list for a project (already paginated by
// the plugin).
func (s *Scanner) fetchSonarIssues(ctx context.Context, paramsJSON []byte, config map[string]string) (sonarIssuePage, error) {
	var page sonarIssuePage
	output, err := s.pluginMgr.ExecuteAction(ctx, "sonarqube", "get_issues", paramsJSON, config)
	if err != nil {
		return page, err
	}
	if err := json.Unmarshal(output, &page); err != nil {
		return page, fmt.Errorf("parse sonarqube issues: %w", err)
	}
	return page, nil
}

// sonarIgnoreSet collects the suppression keys of a target. Trivy ignores are
// keyed by CVE, SonarQube ignores by issue key; a "rule:<rule>" entry suppresses
// every finding of one rule.
func (s *Scanner) sonarIgnoreSet(ctx context.Context, target *repository.ScanTarget) map[string]bool {
	set := map[string]bool{}
	if s.ignoreRepo == nil {
		return set
	}
	ignores, err := s.ignoreRepo.ListByTarget(ctx, target.ID, target.TenantID)
	if err != nil {
		// An unreadable ignore list must not silently hide findings.
		slog.Warn("failed to load ignores for sonarqube scan", "target_id", target.ID, "error", err)
		return set
	}
	for _, ig := range ignores {
		if ig == nil {
			continue
		}
		if ig.IssueKey != nil && *ig.IssueKey != "" {
			set[*ig.IssueKey] = true
		}
		// cve_id also counts: SonarQube ignores created before issue_key existed
		// stored the finding key there, and those must keep working.
		if ig.CveID != "" {
			set[ig.CveID] = true
		}
	}
	return set
}

// sonarIssueIgnored reports whether a finding is suppressed for the target.
func sonarIssueIgnored(ignores map[string]bool, issue sonarIssue) bool {
	if len(ignores) == 0 {
		return false
	}
	if ignores[issue.Key] {
		return true
	}
	return ignores["rule:"+issue.Rule]
}

// sonarDashboardURL links to the project overview inside SonarQube.
func sonarDashboardURL(baseURL, projectKey string) string {
	return strings.TrimRight(baseURL, "/") + "/dashboard?id=" + url.QueryEscape(projectKey)
}

// sonarIssueURL links straight to one finding in the project's issue list.
func sonarIssueURL(baseURL, projectKey, issueKey string) string {
	return fmt.Sprintf("%s/project/issues?id=%s&open=%s",
		strings.TrimRight(baseURL, "/"), url.QueryEscape(projectKey), url.QueryEscape(issueKey))
}

// sanitizeHTTPURL returns raw unchanged when it is a safe http or https URL,
// or empty string otherwise. User-supplied URLs stored in scan_config (such as
// source_ci_url) are passed through this helper before being surfaced to the
// frontend, so a javascript: or data: value cannot become a stored-XSS vector
// via an <a href> rendered in the report viewer.
func sanitizeHTTPURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return raw
}

// sonarSeverityToTrivy maps a SonarQube severity onto the Trivy severity scale.
// The two products name severities differently; the report keeps one scale so
// severity bars, filters and exports stay shared. The original value is preserved
// on every finding as SonarSeverity.
func sonarSeverityToTrivy(severity string) string {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "BLOCKER":
		return "critical"
	case "CRITICAL":
		return "high"
	case "MAJOR":
		return "medium"
	case "MINOR":
		return "low"
	default: // INFO and anything unrecognised
		return "unknown"
	}
}

// buildSonarReport normalises a SonarQube snapshot into the Trivy report contract:
// flat severity counters in the summary, and a Results[] list grouped per file so
// the collapsible finding list works without a scanner-specific branch.
func buildSonarReport(snapshot sonarSnapshot, page sonarIssuePage, baseURL, projectKey string, ignores map[string]bool) (map[string]any, map[string]any) {
	counts := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0, "unknown": 0, "total": 0}

	type findingGroup struct{ path, kind string }
	grouped := map[findingGroup][]map[string]any{}
	var order []findingGroup

	for _, issue := range page.Issues {
		if sonarIssueIgnored(ignores, issue) {
			continue
		}
		path := issue.ComponentPath
		if path == "" {
			path = issue.Component
		}
		if path == "" {
			path = projectKey
		}
		kind := strings.ToUpper(strings.TrimSpace(issue.Type))
		if kind == "" {
			kind = "CODE_SMELL"
		}
		location := path
		if issue.Line > 0 {
			location = fmt.Sprintf("%s:%d", path, issue.Line)
		}
		bucket := sonarSeverityToTrivy(issue.Severity)
		counts[bucket]++
		counts["total"]++

		group := findingGroup{path: path, kind: kind}
		if _, seen := grouped[group]; !seen {
			order = append(order, group)
		}
		grouped[group] = append(grouped[group], map[string]any{
			"VulnerabilityID":  issue.Key,
			"Severity":         strings.ToUpper(bucket),
			"SonarSeverity":    strings.ToUpper(strings.TrimSpace(issue.Severity)),
			"PkgName":          issue.Rule,
			"Title":            issue.Message,
			"Description":      issue.Rule,
			"InstalledVersion": location,
			"PrimaryURL":       sonarIssueURL(baseURL, projectKey, issue.Key),
			"Status":           issue.Status,
			"Line":             issue.Line,
			"Effort":           issue.Effort,
			"TechnicalDebt":    issue.Debt,
		})
	}

	sort.Slice(order, func(i, j int) bool {
		if order[i].path != order[j].path {
			return order[i].path < order[j].path
		}
		return order[i].kind < order[j].kind
	})

	results := make([]map[string]any, 0, len(order))
	for _, g := range order {
		vulns := grouped[g]
		sort.Slice(vulns, func(i, j int) bool {
			a, _ := vulns[i]["VulnerabilityID"].(string)
			b, _ := vulns[j]["VulnerabilityID"].(string)
			return a < b
		})
		results = append(results, map[string]any{
			"Target":          g.path,
			"Class":           "sonar",
			"Type":            g.kind,
			"Vulnerabilities": vulns,
		})
	}

	summary := map[string]any{}
	for k, v := range counts {
		summary[k] = v
	}
	key := snapshot.ProjectKey
	if key == "" {
		key = projectKey
	}
	summary["project_key"] = key
	summary["branch"] = snapshot.Branch
	summary["last_analysis_at"] = snapshot.AnalysisDate
	summary["truncated"] = page.Truncated
	summary["issue_total"] = page.Total
	if snapshot.QualityGate != nil {
		summary["quality_gate_status"] = snapshot.QualityGate.Status
	}
	bugs, vulnCount, smells := snapshot.sonarNumericMetric("bugs"), snapshot.sonarNumericMetric("vulnerabilities"), snapshot.sonarNumericMetric("code_smells")
	if snapshot.IssueSummary != nil {
		bugs, vulnCount, smells = snapshot.IssueSummary.Bugs, snapshot.IssueSummary.Vulnerabilities, snapshot.IssueSummary.CodeSmells
	}
	summary["bugs"] = bugs
	summary["vulnerabilities"] = vulnCount
	summary["code_smells"] = smells
	summary["coverage"] = snapshot.sonarFloatMetric("coverage")
	summary["duplicated_lines_density"] = snapshot.sonarFloatMetric("duplicated_lines_density")
	summary["technical_debt"] = snapshot.sonarMetricValue("sq_debt")

	var rawSnapshot any
	if encoded, err := json.Marshal(snapshot); err == nil {
		if err := json.Unmarshal(encoded, &rawSnapshot); err != nil {
			slog.Warn("failed to re-encode sonarqube snapshot", "error", err)
		}
	}

	full := map[string]any{
		"Results": results,
		"sonar":   rawSnapshot,
	}
	if len(snapshot.Warnings) > 0 {
		full["sonar_warnings"] = snapshot.Warnings
	}

	return summary, full
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

// mergeScanSummaries combines two scanner summaries for a "both" target. Shared
// counters (critical/high/…) must add up — overwriting them would make a
// combined report claim fewer findings than the two scans found. Anything else
// is scanner-specific and only copied over when the first summary lacks it.
func mergeScanSummaries(a, b map[string]any) map[string]any {
	result := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		result[k] = v
	}
	for k, v := range b {
		if existing, ok := result[k]; ok {
			if summed, isNumber := sumNumeric(existing, v); isNumber {
				result[k] = summed
				continue
			}
			continue
		}
		result[k] = v
	}
	return result
}

// sumNumeric adds two JSON numbers, reporting whether both operands were numeric.
func sumNumeric(a, b any) (any, bool) {
	switch av := a.(type) {
	case int:
		bv, ok := toFloat(b)
		if !ok {
			return nil, false
		}
		return av + int(bv), true
	case float64:
		bv, ok := toFloat(b)
		if !ok {
			return nil, false
		}
		return av + bv, true
	default:
		return nil, false
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
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

// DownloadDB downloads the Trivy vulnerability and Java databases into the
// shared cache directory. It uses the configured OCI registries (dbRepo,
// javaDBRepo) which can be overridden at runtime via SetDBRepository.
// This eliminates the need for a separate db-cache container.
func (s *Scanner) DownloadDB(ctx context.Context) error {
	cacheDir := os.Getenv("TRIVY_CACHE_DIR")
	if cacheDir == "" {
		cacheDir = "/tmp/trivy-cache"
	}
	cacheDir = filepath.Clean(cacheDir)

	// Ensure cache directories exist
	if err := os.MkdirAll(filepath.Join(cacheDir, "db"), 0o750); err != nil {
		return fmt.Errorf("create db cache dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(cacheDir, "java-db"), 0o750); err != nil {
		return fmt.Errorf("create java-db cache dir: %w", err)
	}

	// Download trivy-db (Vulnerability DB)
	slog.Info("downloading trivy-db", "repository", s.dbRepo)
	if err := s.runTrivyDBDownload(ctx, cacheDir, "--db-repository", s.dbRepo); err != nil {
		slog.Warn("trivy-db download failed from primary", "repository", s.dbRepo, "error", err)
		// Try fallback registries
		fallbacks := []string{
			"ghcr.io/aquasecurity/trivy-db",
		}
		downloaded := false
		for _, fb := range fallbacks {
			if fb == s.dbRepo {
				continue // skip if it's the same as primary
			}
			slog.Info("trying trivy-db fallback", "repository", fb)
			if fbErr := s.runTrivyDBDownload(ctx, cacheDir, "--db-repository", fb); fbErr == nil {
				slog.Info("trivy-db downloaded from fallback", "repository", fb)
				downloaded = true
				break
			}
		}
		if !downloaded {
			return fmt.Errorf("trivy-db download failed from all sources: %w", err)
		}
	} else {
		slog.Info("trivy-db downloaded successfully", "repository", s.dbRepo)
	}

	// Download trivy-java-db (Java DB)
	slog.Info("downloading trivy-java-db", "repository", s.javaDBRepo)
	if err := s.runTrivyDBDownload(ctx, cacheDir, "--java-db-repository", s.javaDBRepo); err != nil {
		slog.Warn("trivy-java-db download failed from primary", "repository", s.javaDBRepo, "error", err)
		fallbacks := []string{
			"ghcr.io/aquasecurity/trivy-java-db",
		}
		downloaded := false
		for _, fb := range fallbacks {
			if fb == s.javaDBRepo {
				continue
			}
			slog.Info("trying trivy-java-db fallback", "repository", fb)
			if fbErr := s.runTrivyDBDownload(ctx, cacheDir, "--java-db-repository", fb); fbErr == nil {
				slog.Info("trivy-java-db downloaded from fallback", "repository", fb)
				downloaded = true
				break
			}
		}
		if !downloaded {
			return fmt.Errorf("trivy-java-db download failed from all sources: %w", err)
		}
	} else {
		slog.Info("trivy-java-db downloaded successfully", "repository", s.javaDBRepo)
	}

	return nil
}

// runTrivyDBDownload executes a single trivy DB download command:
// --download-db-only for trivy-db or --download-java-db-only for trivy-java-db.
func (s *Scanner) runTrivyDBDownload(ctx context.Context, cacheDir string, repoFlag, repoValue string) error {
	// Use a timeout to prevent hanging indefinitely
	dlCtx, dlCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer dlCancel()

	// The Java DB requires its own download flag: --download-db-only silently
	// ignores --java-db-repository and exits 0 without downloading anything.
	downloadFlag := "--download-db-only"
	if repoFlag == "--java-db-repository" {
		downloadFlag = "--download-java-db-only"
	}

	args := []string{
		"--cache-dir", cacheDir,
		"image",
		downloadFlag,
		"--no-progress",
		repoFlag, repoValue,
		"alpine:latest",
	}

	cmd := exec.CommandContext(dlCtx, "trivy", args...) // #nosec G204,G702 //nolint:gosec // trivy is an admin-configured binary
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // #nosec G104 //nolint:errcheck // negative PID kills process group
		}
		return nil
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}

// StartDBManager launches a background goroutine that periodically refreshes
// the Trivy databases. Call this when the Trivy plugin is activated.
func (s *Scanner) StartDBManager(ctx context.Context) {
	slog.Info("Trivy DB manager started", "refresh_interval", s.dbRefreshInterval,
		"db_repo", s.dbRepo, "java_db_repo", s.javaDBRepo)

	go func() {
		// Initial download on startup
		s.initialDBDownload(ctx)

		// Periodic refresh
		ticker := time.NewTicker(s.dbRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Info("Trivy DB manager stopped")
				return
			case <-ticker.C:
				refreshCtx, refreshCancel := context.WithTimeout(ctx, 15*time.Minute)
				if err := s.DownloadDB(refreshCtx); err != nil {
					slog.Warn("Trivy DB refresh failed", "error", err)
				} else {
					slog.Info("Trivy DB refresh complete")
				}
				refreshCancel()
			}
		}
	}()
}

// initialDBDownload performs the first Trivy DB download with a bounded timeout.
func (s *Scanner) initialDBDownload(ctx context.Context) {
	dlCtx, dlCancel := context.WithTimeout(ctx, 15*time.Minute)
	defer dlCancel()
	if err := s.DownloadDB(dlCtx); err != nil {
		slog.Warn("initial Trivy DB download failed", "error", err)
	} else {
		slog.Info("initial Trivy DB download complete")
	}
}

// SetDBRepository overrides the DB registry endpoints at runtime.
// Call this when the Trivy plugin config specifies custom repositories.
func (s *Scanner) SetDBRepository(dbRepo, javaDBRepo string) {
	if dbRepo != "" {
		s.dbRepo = dbRepo
	}
	if javaDBRepo != "" {
		s.javaDBRepo = javaDBRepo
	}
	slog.Info("Trivy DB repository updated", "db_repo", s.dbRepo, "java_db_repo", s.javaDBRepo)
}
