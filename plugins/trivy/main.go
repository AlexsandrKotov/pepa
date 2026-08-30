// PEPA Trivy Plugin — Security vulnerability scanner.
// Implements the Provider interface for Trivy vulnerability scanning.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// TrivyPlugin implements provider.Provider for Trivy security scanning.
type TrivyPlugin struct {
	target        string
	scanType      string
	severity      string
	ignoreUnfixed bool
	format        string
	lastResult    *ScanResult
}

// ScanResult holds the output of a Trivy scan.
type ScanResult struct {
	Target          string          `json:"target"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
	Summary         VulnSummary     `json:"summary"`
	ScanTime        string          `json:"scan_time,omitempty"`
	RawOutput       string          `json:"raw_output,omitempty"`
}

// Vulnerability represents a single CVE finding.
type Vulnerability struct {
	ID          string `json:"id"`
	PkgName     string `json:"pkg_name"`
	Installed   string `json:"installed_version"`
	FixedIn     string `json:"fixed_version,omitempty"`
	Severity    string `json:"severity"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// VulnSummary holds vulnerability counts by severity.
type VulnSummary struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Unknown  int `json:"unknown"`
	Total    int `json:"total"`
}

// NewTrivyPlugin creates a new Trivy plugin instance.
func NewTrivyPlugin(config map[string]string) (*TrivyPlugin, error) {
	target := config["target"]
	if target == "" {
		return nil, fmt.Errorf("trivy plugin requires target")
	}

	scanType := config["scan_type"]
	if scanType == "" {
		scanType = "image"
	}

	severity := config["severity"]
	if severity == "" {
		severity = "HIGH,CRITICAL"
	}

	format := config["format"]
	if format == "" {
		format = "json"
	}

	return &TrivyPlugin{
		target:        target,
		scanType:      scanType,
		severity:      severity,
		ignoreUnfixed: config["ignore_unfixed"] == "true",
		format:        format,
	}, nil
}

func (p *TrivyPlugin) Name() string        { return "trivy" }
func (p *TrivyPlugin) Version() string     { return "0.1.0" }
func (p *TrivyPlugin) Description() string { return "Trivy security scanner — vulnerability detection for images, filesystem, repos, and IaC" }
func (p *TrivyPlugin) PluginType() string  { return "security_scanner" }

func (p *TrivyPlugin) Actions() []string {
	return []string{
		"scan",
		"scan_image",
		"scan_filesystem",
		"scan_repo",
		"scan_config",
		"get_results",
		"export_sarif",
	}
}

func (p *TrivyPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	// Allow per-request config override
	if config != nil && config["target"] != "" {
		plugin, err := NewTrivyPlugin(config)
		if err != nil {
			return nil, err
		}
		return plugin.Execute(ctx, action, params, nil)
	}

	var paramMap map[string]interface{}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &paramMap); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
	}

	switch action {
	case "scan":
		return p.runScan(ctx, paramMap)
	case "scan_image":
		return p.scanImage(ctx, paramMap)
	case "scan_filesystem":
		return p.scanFilesystem(ctx, paramMap)
	case "scan_repo":
		return p.scanRepo(ctx, paramMap)
	case "scan_config":
		return p.scanConfig(ctx, paramMap)
	case "get_results":
		return p.getResults()
	case "export_sarif":
		return p.exportSARIF(ctx, paramMap)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *TrivyPlugin) runScan(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	target := p.target
	if t, ok := params["target"].(string); ok && t != "" {
		target = t
	}

	scanType := p.scanType
	if st, ok := params["scan_type"].(string); ok && st != "" {
		scanType = st
	}

	severity := p.severity
	if s, ok := params["severity"].(string); ok && s != "" {
		severity = s
	}

	ignoreUnfixed := p.ignoreUnfixed
	if iu, ok := params["ignore_unfixed"].(bool); ok {
		ignoreUnfixed = iu
	}

	result, err := p.executeTrivy(ctx, target, scanType, severity, ignoreUnfixed)
	if err != nil {
		return nil, err
	}

	p.lastResult = result
	return json.Marshal(result)
}

func (p *TrivyPlugin) scanImage(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	image, _ := params["image"].(string)
	if image == "" {
		return nil, fmt.Errorf("image is required")
	}

	severity := p.severity
	if s, ok := params["severity"].(string); ok && s != "" {
		severity = s
	} else {
		severity = "HIGH,CRITICAL"
	}

	result, err := p.executeTrivy(ctx, image, "image", severity, false)
	if err != nil {
		return nil, err
	}

	p.lastResult = result
	return json.Marshal(result)
}

func (p *TrivyPlugin) scanFilesystem(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	path, _ := params["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	severity := p.severity
	if s, ok := params["severity"].(string); ok && s != "" {
		severity = s
	}

	result, err := p.executeTrivy(ctx, path, "fs", severity, false)
	if err != nil {
		return nil, err
	}

	p.lastResult = result
	return json.Marshal(result)
}

func (p *TrivyPlugin) scanRepo(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	repoURL, _ := params["url"].(string)
	if repoURL == "" {
		return nil, fmt.Errorf("url is required")
	}

	severity := p.severity
	if s, ok := params["severity"].(string); ok && s != "" {
		severity = s
	}

	result, err := p.executeTrivy(ctx, repoURL, "repo", severity, false)
	if err != nil {
		return nil, err
	}

	p.lastResult = result
	return json.Marshal(result)
}

func (p *TrivyPlugin) scanConfig(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	path, _ := params["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	result, err := p.executeTrivy(ctx, path, "config", "HIGH,CRITICAL", false)
	if err != nil {
		return nil, err
	}

	p.lastResult = result
	return json.Marshal(result)
}

func (p *TrivyPlugin) getResults() ([]byte, error) {
	if p.lastResult == nil {
		return json.Marshal(map[string]string{"status": "no_scan_results"})
	}
	return json.Marshal(p.lastResult)
}

func (p *TrivyPlugin) exportSARIF(ctx context.Context, params map[string]interface{}) ([]byte, error) {
	target := p.target
	if t, ok := params["target"].(string); ok && t != "" {
		target = t
	}

	// Run trivy with SARIF output
	args := []string{p.scanType, "--format", "sarif", "--severity", p.severity}
	if p.ignoreUnfixed {
		args = append(args, "--ignore-unfixed")
	}
	args = append(args, target)

	cmd := exec.CommandContext(ctx, "trivy", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Trivy exits with code 1 when vulnerabilities are found
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Still return the SARIF output
		} else {
			return nil, fmt.Errorf("trivy scan failed: %w", err)
		}
	}

	return stdout.Bytes(), nil
}

func (p *TrivyPlugin) executeTrivy(ctx context.Context, target, scanType, severity string, ignoreUnfixed bool) (*ScanResult, error) {
	startTime := time.Now()

	args := []string{scanType, "--format", "json", "--severity", severity, "--no-color"}
	if ignoreUnfixed {
		args = append(args, "--ignore-unfixed")
	}
	args = append(args, target)

	cmd := exec.CommandContext(ctx, "trivy", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	cmdErr := cmd.Run()

	// Parse JSON output
	result := parseTrivyOutput(stdout.String(), target)
	result.ScanTime = time.Since(startTime).String()

	// Trivy exits with code 1 when vulnerabilities are found — that's still a successful scan
	if cmdErr != nil {
		if exitErr, ok := cmdErr.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Vulnerabilities found — scan was successful
		} else {
			return result, fmt.Errorf("trivy scan failed: %w", cmdErr)
		}
	}

	return result, nil
}

func parseTrivyOutput(output, target string) *ScanResult {
	result := &ScanResult{
		Target: target,
	}

	// Find JSON portion of output
	jsonStart := strings.Index(output, `{"Results"`)
	if jsonStart < 0 {
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

func (p *TrivyPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	// Check if trivy binary is available
	cmd := exec.CommandContext(ctx, "trivy", "--version")
	if err := cmd.Run(); err != nil {
		return &provider.HealthStatus{
			Status:  "unhealthy",
			Message: "trivy binary not found or not executable",
		}, nil
	}
	return &provider.HealthStatus{
		Status: "healthy",
	}, nil
}

func main() {
	sdk.Serve(&TrivyPlugin{})
}
