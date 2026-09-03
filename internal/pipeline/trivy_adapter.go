package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// TrivyConfig is the expected shape of PipelineSource.Config for trivy sources.
type TrivyConfig struct {
	Target        string `json:"target"`         // image name, filesystem path, or repo URL
	ScanType      string `json:"scan_type"`      // image, fs, repo, config
	Severity      string `json:"severity"`       // UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL
	IgnoreUnfixed bool   `json:"ignore_unfixed"` // only show fixed vulnerabilities
	Format        string `json:"format"`         // json, table, sarif (default: json)
}

func parseTrivyConfig(raw json.RawMessage) (*TrivyConfig, error) {
	var cfg TrivyConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid trivy config: %w", err)
	}
	if cfg.Target == "" {
		return nil, fmt.Errorf("trivy config: target is required")
	}
	if cfg.ScanType == "" {
		cfg.ScanType = "image"
	}
	if cfg.Severity == "" {
		cfg.Severity = "HIGH,CRITICAL"
	}
	if cfg.Format == "" {
		cfg.Format = "json"
	}
	return &cfg, nil
}

// ── Active run tracking ────────────────────────────────────────

// trivyRun tracks an in-flight trivy scan.
type trivyRun struct {
	cancel context.CancelFunc
	logBuf *bytes.Buffer
	status string // pending, running, success, failed, cancelled
	result *TrivyScanResult
}

var (
	trivyRunsMu sync.RWMutex
	trivyRuns   = make(map[string]*trivyRun)
)

// TrivyAdapter implements Provider for Trivy vulnerability scanning.
type TrivyAdapter struct{}

// NewTrivyAdapter creates a new Trivy adapter.
func NewTrivyAdapter() *TrivyAdapter {
	return &TrivyAdapter{}
}

func (a *TrivyAdapter) Name() string { return "trivy" }

// ResolveSchema returns the parameter schema for Trivy scans.
func (a *TrivyAdapter) ResolveSchema(ctx context.Context, raw json.RawMessage) (*ParameterSchema, error) {
	cfg, err := parseTrivyConfig(raw)
	if err != nil {
		return nil, err
	}

	props := map[string]PropertyDef{
		"target": {
			Type:        "string",
			Description: "Target to scan (image name, filesystem path, or repo URL)",
			Default:     cfg.Target,
		},
		"scan_type": {
			Type:        "string",
			Description: "Type of scan to perform",
			Default:     cfg.ScanType,
			Enum:        []string{"image", "fs", "repo", "config"},
		},
		"severity": {
			Type:        "string",
			Description: "Comma-separated list of severities to report",
			Default:     cfg.Severity,
		},
		"ignore_unfixed": {
			Type:        "boolean",
			Description: "Only show vulnerabilities with available fixes",
			Default:     "false",
		},
	}

	return &ParameterSchema{
		Type:       "object",
		Properties: props,
		Required:   []string{"target"},
	}, nil
}

// Trigger runs a Trivy scan and captures the results.
func (a *TrivyAdapter) Trigger(ctx context.Context, raw json.RawMessage, params map[string]any) (*TriggerResult, error) {
	cfg, err := parseTrivyConfig(raw)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	logBuf := &bytes.Buffer{}
	runID := fmt.Sprintf("trivy-%s", randomRunID())

	// Register the run
	trivyRunsMu.Lock()
	trivyRuns[runID] = &trivyRun{cancel: cancel, logBuf: logBuf, status: "running"}
	trivyRunsMu.Unlock()
	defer func() {
		trivyRunsMu.Lock()
		delete(trivyRuns, runID)
		trivyRunsMu.Unlock()
	}()

	// Resolve target and options from params
	target := cfg.Target
	if t, ok := params["target"]; ok && t != "" {
		target = fmt.Sprintf("%v", t)
	}
	scanType := cfg.ScanType
	if st, ok := params["scan_type"]; ok && st != "" {
		scanType = fmt.Sprintf("%v", st)
	}
	// Validate scan type
	validScanTypes := map[string]bool{"image": true, "fs": true, "repo": true, "config": true}
	if !validScanTypes[scanType] {
		return nil, fmt.Errorf("invalid scan_type: %s (must be image, fs, repo, or config)", scanType)
	}
	severity := cfg.Severity
	if s, ok := params["severity"]; ok && s != "" {
		severity = fmt.Sprintf("%v", s)
	}
	ignoreUnfixed := cfg.IgnoreUnfixed
	if iu, ok := params["ignore_unfixed"]; ok && (iu == "true" || iu == true) {
		ignoreUnfixed = true
	}

	// Build trivy command
	args := []string{scanType}
	args = append(args, "--severity", severity)
	args = append(args, "--format", "json")
	args = append(args, "--no-color")
	if ignoreUnfixed {
		args = append(args, "--ignore-unfixed")
	}
	args = append(args, target)

	cmd := exec.CommandContext(runCtx, "trivy", args...) // #nosec //nolint:gosec // G204: trivy is an admin-configured binary
	cmd.Stdout = logBuf
	cmd.Stderr = logBuf
	cmdErr := cmd.Run()

	// Parse JSON output for structured results
	scanResult := parseTrivyJSON(logBuf.String(), target)

	status := "success"
	if cmdErr != nil {
		// Trivy exits with code 1 when vulnerabilities are found — that's still "success" for us
		exitErr, ok := cmdErr.(*exec.ExitError)
		if ok && exitErr.ExitCode() == 1 {
			status = "success" // vulnerabilities found is a successful scan
		} else {
			status = "failed"
		}
	}

	trivyRunsMu.Lock()
	run := trivyRuns[runID]
	run.status = status
	run.result = scanResult
	trivyRunsMu.Unlock()

	return &TriggerResult{
		ExternalRunID: runID,
		ExternalURL:   "",
		Status:        status,
	}, nil
}

// Status returns the status of a Trivy scan run.
func (a *TrivyAdapter) Status(ctx context.Context, raw json.RawMessage, externalRunID string) (*RunStatus, error) {
	trivyRunsMu.RLock()
	run, ok := trivyRuns[externalRunID]
	trivyRunsMu.RUnlock()

	status := "success"
	if ok {
		status = run.status
	}

	return &RunStatus{
		ExternalRunID: externalRunID,
		Status:        status,
	}, nil
}

// Jobs returns scan stages for a Trivy run.
func (a *TrivyAdapter) Jobs(ctx context.Context, raw json.RawMessage, externalRunID string) ([]JobInfo, error) {
	trivyRunsMu.RLock()
	run, ok := trivyRuns[externalRunID]
	trivyRunsMu.RUnlock()

	if !ok {
		return []JobInfo{}, nil
	}

	jobs := []JobInfo{
		{
			ExternalJobID: externalRunID + "-scan",
			Name:          "Vulnerability Scan",
			Stage:         "scan",
			Status:        run.status,
		},
	}

	if run.result != nil && run.result.Summary.Total > 0 {
		jobs = append(jobs, JobInfo{
			ExternalJobID: externalRunID + "-report",
			Name:          fmt.Sprintf("Found %d vulnerabilities", run.result.Summary.Total),
			Stage:         "report",
			Status:        run.status,
		})
	}

	return jobs, nil
}

// Logs returns captured output from a Trivy scan.
func (a *TrivyAdapter) Logs(ctx context.Context, raw json.RawMessage, externalRunID string, jobID string) (string, error) {
	trivyRunsMu.RLock()
	run, ok := trivyRuns[externalRunID]
	trivyRunsMu.RUnlock()

	if ok && run.logBuf != nil {
		return run.logBuf.String(), nil
	}
	return "", nil
}

// Cancel cancels a running Trivy scan.
func (a *TrivyAdapter) Cancel(ctx context.Context, raw json.RawMessage, externalRunID string) error {
	trivyRunsMu.RLock()
	run, ok := trivyRuns[externalRunID]
	trivyRunsMu.RUnlock()

	if ok && run.cancel != nil {
		run.cancel()
		trivyRunsMu.Lock()
		run.status = "cancelled"
		trivyRunsMu.Unlock()
		return nil
	}
	return fmt.Errorf("no active run found for %s", externalRunID)
}

// ── Output parsing ──────────────────────────────────────────────

// parseTrivyJSON parses the JSON output from trivy into a structured result.
func parseTrivyJSON(output, target string) *TrivyScanResult {
	result := &TrivyScanResult{
		Target: target,
	}

	// Trivy JSON output may have non-JSON lines before the actual JSON (e.g., progress output)
	// Find the JSON portion by looking for the Results key
	jsonStart := strings.Index(output, `{"Results"`)
	if jsonStart < 0 {
		// Fallback: try to find any JSON object with Results
		jsonStart = strings.Index(output, "{\"Results")
	}
	if jsonStart < 0 {
		// Last resort: find first opening brace
		jsonStart = strings.Index(output, "{")
	}
	if jsonStart < 0 {
		return result
	}
	jsonOutput := output[jsonStart:]

	var trivyOutput struct {
		Results []struct {
			Target          string `json:"Target"`
			Vulnerabilities []struct {
				VulnerabilityID  string `json:"VulnerabilityID"`
				PkgName          string `json:"PkgName"`
				InstalledVersion string `json:"InstalledVersion"`
				FixedVersion     string `json:"FixedVersion"`
				Severity         string `json:"Severity"`
				Title            string `json:"Title"`
				Description      string `json:"Description"`
			} `json:"Vulnerabilities"`
		} `json:"Results"`
	}

	if err := json.Unmarshal([]byte(jsonOutput), &trivyOutput); err != nil {
		return result
	}

	for _, r := range trivyOutput.Results {
		for _, v := range r.Vulnerabilities {
			vuln := Vulnerability{
				ID:          v.VulnerabilityID,
				PkgName:     v.PkgName,
				Installed:   v.InstalledVersion,
				FixedIn:     v.FixedVersion,
				Severity:    v.Severity,
				Title:       v.Title,
				Description: v.Description,
			}
			result.Vulnerabilities = append(result.Vulnerabilities, vuln)

			// Update summary counts
			switch strings.ToUpper(v.Severity) {
			case "CRITICAL":
				result.Summary.Critical++
			case "HIGH":
				result.Summary.High++
			case "MEDIUM":
				result.Summary.Medium++
			case "LOW":
				result.Summary.Low++
			default:
				result.Summary.Unknown++
			}
			result.Summary.Total++
		}
	}

	return result
}
