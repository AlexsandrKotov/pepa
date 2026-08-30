package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// TerraformConfig is the expected shape of PipelineSource.Config for terraform sources.
type TerraformConfig struct {
	RepoURL    string `json:"repo_url"`
	LocalPath  string `json:"local_path"`  // absolute path on the server for local stacks
	WorkingDir string `json:"working_dir"` // e.g. "." or "infra/"
	Token      string `json:"token"`       // git/repo token
}

func parseTerraformConfig(raw json.RawMessage) (*TerraformConfig, error) {
	var cfg TerraformConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid terraform config: %w", err)
	}
	if cfg.RepoURL == "" && cfg.LocalPath == "" {
		return nil, fmt.Errorf("terraform config: repo_url or local_path is required")
	}
	if cfg.WorkingDir == "" {
		cfg.WorkingDir = "."
	}
	return &cfg, nil
}

// isLocal returns true when the config points to a local directory instead of a git repo.
func (c *TerraformConfig) isLocal() bool {
	return c.LocalPath != "" && c.RepoURL == ""
}

// ── Active run tracking ────────────────────────────────────────

// tfRun tracks an in-flight terraform execution.
type tfRun struct {
	cancel context.CancelFunc
	logBuf *bytes.Buffer
	status string // pending, running, success, failed, cancelled
}

var (
	tfRunsMu sync.RWMutex
	tfRuns   = make(map[string]*tfRun)
)

// TerraformAdapter implements Provider and EnhancedProvider for Terraform pipelines.
type TerraformAdapter struct{}

func NewTerraformAdapter() *TerraformAdapter {
	return &TerraformAdapter{}
}

func (a *TerraformAdapter) Name() string { return "terraform" }

// ResolveSchema clones the repo (or reads local path) and parses .tf files to extract variable blocks.
func (a *TerraformAdapter) ResolveSchema(ctx context.Context, raw json.RawMessage) (*ParameterSchema, error) {
	cfg, err := parseTerraformConfig(raw)
	if err != nil {
		return nil, err
	}

	baseDir, cleanup, err := resolveTerraformDir(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	workDir := filepath.Join(baseDir, filepath.Clean(cfg.WorkingDir))
	// For local mode, working_dir can be absolute or relative to local_path
	if cfg.isLocal() && filepath.IsAbs(cfg.WorkingDir) {
		workDir = filepath.Clean(cfg.WorkingDir)
	}
	// Verify the resolved path is within baseDir (skip for local mode with absolute working_dir)
	if !cfg.isLocal() {
		absWorkDir, _ := filepath.Abs(workDir)
		absBaseDir, _ := filepath.Abs(baseDir)
		if !strings.HasPrefix(absWorkDir, absBaseDir) {
			return nil, fmt.Errorf("invalid working_dir path")
		}
	}

	props := make(map[string]PropertyDef)

	// Find all .tf files in the working directory
	tfFiles, _ := filepath.Glob(filepath.Join(workDir, "*.tf"))
	// Also check for .tf.json files
	tfJSONFiles, _ := filepath.Glob(filepath.Join(workDir, "*.tf.json"))
	tfFiles = append(tfFiles, tfJSONFiles...)

	// Regex to match variable blocks in HCL
	// Matches: variable "name" { ... }
	varBlockRe := regexp.MustCompile(`(?s)variable\s+"([^"]+)"\s*\{([^}]*)\}`)
	// Extract attributes from within the block
	defaultRe := regexp.MustCompile(`(?s)default\s*=\s*"([^"]*)"`)
	descRe := regexp.MustCompile(`(?s)description\s*=\s*"([^"]*)"`)
	typeRe := regexp.MustCompile(`(?s)type\s*=\s*(\S+)`)

	for _, tfFile := range tfFiles {
		data, err := os.ReadFile(tfFile) //nolint:gosec // G304: tfFile is from a validated directory listing
		if err != nil {
			continue
		}

		if strings.HasSuffix(tfFile, ".tf.json") {
			// Parse JSON-format terraform
			var tfJSON map[string]interface{}
			if json.Unmarshal(data, &tfJSON) != nil {
				continue
			}
			if vars, ok := tfJSON["variable"].(map[string]interface{}); ok {
				for name, v := range vars {
					if vMap, ok := v.(map[string]interface{}); ok {
						pd := PropertyDef{Type: "string"}
						if desc, ok := vMap["description"].(string); ok {
							pd.Description = desc
						}
						if def, ok := vMap["default"]; ok && def != nil {
							pd.Default = fmt.Sprintf("%v", def)
						}
						props[name] = pd
					}
				}
			}
			continue
		}

		// Parse HCL-format terraform
		matches := varBlockRe.FindAllSubmatch(data, -1)
		for _, match := range matches {
			if len(match) < 3 {
				continue
			}
			name := string(match[1])
			block := string(match[2])

			pd := PropertyDef{Type: "string"}

			if descMatches := descRe.FindSubmatch([]byte(block)); len(descMatches) > 1 {
				pd.Description = string(descMatches[1])
			} else {
				pd.Description = fmt.Sprintf("Terraform variable: %s", name)
			}

			if defMatches := defaultRe.FindSubmatch([]byte(block)); len(defMatches) > 1 {
				pd.Default = string(defMatches[1])
			}

			if typeMatches := typeRe.FindSubmatch([]byte(block)); len(typeMatches) > 1 {
				tfType := string(typeMatches[1])
				switch {
				case strings.Contains(tfType, "number"):
					pd.Type = "number"
				case strings.Contains(tfType, "bool"):
					pd.Type = "boolean"
				default:
					pd.Type = "string"
				}
			}

			props[name] = pd
		}
	}

	// Always include standard terraform params
	if _, ok := props["tf_action"]; !ok {
		props["tf_action"] = PropertyDef{
			Type:        "string",
			Description: "Terraform action to run",
			Default:     "apply",
			Enum:        []string{"plan", "apply", "destroy", "refresh"},
		}
	}
	if _, ok := props["auto_approve"]; !ok {
		props["auto_approve"] = PropertyDef{
			Type:        "boolean",
			Description: "Skip interactive approval",
			Default:     "false",
		}
	}

	return &ParameterSchema{
		Type:       "object",
		Properties: props,
	}, nil
}

// Trigger runs terraform commands with the given parameters.
// Output is captured and stored for later retrieval via Logs().
func (a *TerraformAdapter) Trigger(ctx context.Context, raw json.RawMessage, params map[string]any) (*TriggerResult, error) {
	cfg, err := parseTerraformConfig(raw)
	if err != nil {
		return nil, err
	}

	baseDir, cleanup, err := resolveTerraformDir(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	workDir := filepath.Join(baseDir, filepath.Clean(cfg.WorkingDir))
	if cfg.isLocal() && filepath.IsAbs(cfg.WorkingDir) {
		workDir = filepath.Clean(cfg.WorkingDir)
	}
	if !cfg.isLocal() {
		absWorkDir, _ := filepath.Abs(workDir)
		absBaseDir, _ := filepath.Abs(baseDir)
		if !strings.HasPrefix(absWorkDir, absBaseDir) {
			return nil, fmt.Errorf("invalid working_dir path")
		}
	}

	action := "apply"
	if a, ok := params["tf_action"]; ok && a != "" {
		action = fmt.Sprintf("%v", a)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	logBuf := &bytes.Buffer{}
	runID := fmt.Sprintf("tf-%s", randomRunID())

	// Register the run for tracking
	tfRunsMu.Lock()
	tfRuns[runID] = &tfRun{cancel: cancel, logBuf: logBuf, status: "running"}
	tfRunsMu.Unlock()
	defer func() {
		tfRunsMu.Lock()
		delete(tfRuns, runID)
		tfRunsMu.Unlock()
	}()

	// Run terraform init
	initCmd := exec.CommandContext(runCtx, "terraform", "init", "-input=false")
	initCmd.Dir = workDir
	initCmd.Stdout = logBuf
	initCmd.Stderr = logBuf
	if err := initCmd.Run(); err != nil {
		tfRunsMu.Lock()
		tfRuns[runID].status = "failed"
		tfRunsMu.Unlock()
		return nil, fmt.Errorf("terraform init failed: %s: %w", logBuf.String(), err)
	}

	// Build terraform command args
	args := []string{action, "-input=false", "-no-color"}

	if autoApprove, ok := params["auto_approve"]; ok && (autoApprove == "true" || autoApprove == true) {
		args = append(args, "-auto-approve")
	}

	// Add -var flags for extra params
	for k, v := range params {
		if k == "tf_action" || k == "auto_approve" || k == "ref" {
			continue
		}
		args = append(args, "-var", fmt.Sprintf("%s=%v", k, v))
	}

	tfCmd := exec.CommandContext(runCtx, "terraform", args...)
	tfCmd.Dir = workDir
	tfCmd.Stdout = logBuf
	tfCmd.Stderr = logBuf
	if err := tfCmd.Run(); err != nil {
		tfRunsMu.Lock()
		tfRuns[runID].status = "failed"
		tfRunsMu.Unlock()
		return &TriggerResult{
			ExternalRunID: runID,
			ExternalURL:   "",
			Status:        "failed",
		}, fmt.Errorf("terraform %s failed: %w", action, err)
	}

	tfRunsMu.Lock()
	tfRuns[runID].status = "success"
	tfRunsMu.Unlock()

	return &TriggerResult{
		ExternalRunID: runID,
		ExternalURL:   "",
		Status:        "success",
	}, nil
}

// Plan runs terraform plan and returns a structured preview of changes.
func (a *TerraformAdapter) Plan(ctx context.Context, raw json.RawMessage, params map[string]any) (*PlanResult, error) {
	cfg, err := parseTerraformConfig(raw)
	if err != nil {
		return nil, err
	}

	baseDir, cleanup, err := resolveTerraformDir(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	workDir := filepath.Join(baseDir, filepath.Clean(cfg.WorkingDir))
	if cfg.isLocal() && filepath.IsAbs(cfg.WorkingDir) {
		workDir = filepath.Clean(cfg.WorkingDir)
	}
	if !cfg.isLocal() {
		absWorkDir, _ := filepath.Abs(workDir)
		absBaseDir, _ := filepath.Abs(baseDir)
		if !strings.HasPrefix(absWorkDir, absBaseDir) {
			return nil, fmt.Errorf("invalid working_dir path")
		}
	}

	// terraform init
	initCmd := exec.CommandContext(ctx, "terraform", "init", "-input=false", "-no-color")
	initCmd.Dir = workDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("terraform init failed: %s: %w", string(output), err)
	}

	// terraform plan with JSON output
	// planFile is written to a validated workDir (already checked above), so the path is safe.
	planFile := filepath.Join(workDir, "tfplan")
	planArgs := []string{"plan", "-input=false", "-no-color", "-out=" + planFile}
	for k, v := range params {
		if k == "tf_action" || k == "auto_approve" || k == "ref" {
			continue
		}
		planArgs = append(planArgs, "-var", fmt.Sprintf("%s=%v", k, v))
	}

	planCmd := exec.CommandContext(ctx, "terraform", planArgs...) //nolint:gosec // G204: terraform CLI is an expected subprocess
	planCmd.Dir = workDir
	planOutput, planErr := planCmd.CombinedOutput()

	// Try to parse the plan file for structured output
	result := &PlanResult{
		OutputText: string(planOutput),
	}

	// Parse summary from plan output text
	if planErr != nil {
		// exit code 2 means changes detected (not an error)
		exitErr, ok := planErr.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 2 {
			return result, nil
		}
	}

	// Parse add/change/destroy counts from plan output
	// Terraform v1.x format: "Plan: 1 to add, 2 to change, 3 to destroy."
	addRe := regexp.MustCompile(`(\d+)\s+to add`)
	changeRe := regexp.MustCompile(`(\d+)\s+to change`)
	destroyRe := regexp.MustCompile(`(\d+)\s+to destroy`)
	// Fallback for newer Terraform formats
	createRe := regexp.MustCompile(`Create:\s*(\d+)`)
	updateRe := regexp.MustCompile(`Update:\s*(\d+)`)
	deleteRe := regexp.MustCompile(`Delete:\s*(\d+)`)

	if m := addRe.FindSubmatch(planOutput); len(m) > 1 {
		_, _ = fmt.Sscanf(string(m[1]), "%d", &result.AddCount)
	} else if m := createRe.FindSubmatch(planOutput); len(m) > 1 {
		_, _ = fmt.Sscanf(string(m[1]), "%d", &result.AddCount)
	}
	if m := changeRe.FindSubmatch(planOutput); len(m) > 1 {
		_, _ = fmt.Sscanf(string(m[1]), "%d", &result.ChangeCount)
	} else if m := updateRe.FindSubmatch(planOutput); len(m) > 1 {
		_, _ = fmt.Sscanf(string(m[1]), "%d", &result.ChangeCount)
	}
	if m := destroyRe.FindSubmatch(planOutput); len(m) > 1 {
		_, _ = fmt.Sscanf(string(m[1]), "%d", &result.DestroyCount)
	} else if m := deleteRe.FindSubmatch(planOutput); len(m) > 1 {
		_, _ = fmt.Sscanf(string(m[1]), "%d", &result.DestroyCount)
	}

	result.HasChanges = result.AddCount > 0 || result.ChangeCount > 0 || result.DestroyCount > 0

	// Try to get JSON representation via show
	showCmd := exec.CommandContext(ctx, "terraform", "show", "-json", planFile) //nolint:gosec // G204: terraform CLI is an expected subprocess
	showCmd.Dir = workDir
	if jsonOutput, err := showCmd.Output(); err == nil {
		result.OutputJSON = string(jsonOutput)
	}

	// Clean up plan file
	_ = os.Remove(planFile)

	return result, nil
}

// State returns the current terraform state as structured data.
func (a *TerraformAdapter) State(ctx context.Context, raw json.RawMessage) (*StateResult, error) {
	cfg, err := parseTerraformConfig(raw)
	if err != nil {
		return nil, err
	}

	baseDir, cleanup, err := resolveTerraformDir(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	workDir := filepath.Join(baseDir, filepath.Clean(cfg.WorkingDir))
	if cfg.isLocal() && filepath.IsAbs(cfg.WorkingDir) {
		workDir = filepath.Clean(cfg.WorkingDir)
	}
	// Validate path is within baseDir
	absWorkDir, _ := filepath.Abs(workDir)
	absBaseDir, _ := filepath.Abs(baseDir)
	if !strings.HasPrefix(absWorkDir, absBaseDir) {
		return nil, fmt.Errorf("invalid working_dir path")
	}

	// terraform show -json
	showCmd := exec.CommandContext(ctx, "terraform", "show", "-json")
	showCmd.Dir = workDir
	jsonOutput, err := showCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("terraform show failed: %w", err)
	}

	result := &StateResult{
		RawJSON: string(jsonOutput),
	}

	// Parse the JSON to extract resources
	var stateDoc struct {
		Values struct {
			RootModule struct {
				Resources []struct {
					Type    string `json:"type"`
					Name    string `json:"name"`
					Value   map[string]interface{} `json:"values"`
				} `json:"resources"`
			} `json:"root_module"`
		} `json:"values"`
	}
	if json.Unmarshal(jsonOutput, &stateDoc) == nil {
		for _, r := range stateDoc.Values.RootModule.Resources {
			id := ""
			if v, ok := r.Value["id"].(string); ok {
				id = v
			}
			result.Resources = append(result.Resources, StateResource{
				Type:   r.Type,
				Name:   r.Name,
				ID:     id,
				Status: "created",
			})
		}
	}

	return result, nil
}

// Status returns the status of a Terraform run.
func (a *TerraformAdapter) Status(ctx context.Context, raw json.RawMessage, externalRunID string) (*RunStatus, error) {
	tfRunsMu.RLock()
	run, ok := tfRuns[externalRunID]
	tfRunsMu.RUnlock()

	status := "success"
	if ok {
		status = run.status
	}

	return &RunStatus{
		ExternalRunID: externalRunID,
		Status:        status,
	}, nil
}

// Jobs returns no job info for Terraform (single-command execution).
func (a *TerraformAdapter) Jobs(ctx context.Context, raw json.RawMessage, externalRunID string) ([]JobInfo, error) {
	return []JobInfo{}, nil
}

// Logs returns captured output from a Terraform run.
func (a *TerraformAdapter) Logs(ctx context.Context, raw json.RawMessage, externalRunID string, jobID string) (string, error) {
	tfRunsMu.RLock()
	run, ok := tfRuns[externalRunID]
	tfRunsMu.RUnlock()

	if ok && run.logBuf != nil {
		return run.logBuf.String(), nil
	}
	return "", nil
}

// Cancel cancels a running Terraform execution.
func (a *TerraformAdapter) Cancel(ctx context.Context, raw json.RawMessage, externalRunID string) error {
	tfRunsMu.RLock()
	run, ok := tfRuns[externalRunID]
	tfRunsMu.RUnlock()

	if ok && run.cancel != nil {
		run.cancel()
		tfRunsMu.Lock()
		run.status = "cancelled"
		tfRunsMu.Unlock()
		return nil
	}
	return fmt.Errorf("no active run found for %s", externalRunID)
}

// Inspect parses .tf files to discover modules, resources, data sources, and outputs.
func (a *TerraformAdapter) Inspect(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	cfg, err := parseTerraformConfig(raw)
	if err != nil {
		return nil, err
	}

	baseDir, cleanup, err := resolveTerraformDir(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	workDir := filepath.Join(baseDir, filepath.Clean(cfg.WorkingDir))
	if cfg.isLocal() && filepath.IsAbs(cfg.WorkingDir) {
		workDir = filepath.Clean(cfg.WorkingDir)
	}

	inspection := &TerraformInspection{}

	// Regex patterns for HCL blocks
	moduleRe := regexp.MustCompile(`(?s)module\s+"([^"]+)"\s*\{([^}]*(?:\{[^}]*\}[^}]*)*)\}`)
	resourceRe := regexp.MustCompile(`(?s)resource\s+"([^"]+)"\s+"([^"]+)"\s*\{`)
	dataRe := regexp.MustCompile(`(?s)data\s+"([^"]+)"\s+"([^"]+)"\s*\{`)
	outputRe := regexp.MustCompile(`(?s)output\s+"([^"]+)"\s*\{([^}]*(?:\{[^}]*\}[^}]*)*)\}`)
	backendRe := regexp.MustCompile(`(?s)backend\s+"([^"]+)"`)
	sourceRe := regexp.MustCompile(`(?s)source\s*=\s*"([^"]*)"`)
	versionRe := regexp.MustCompile(`(?s)version\s*=\s*"([^"]*)"`)
	descRe := regexp.MustCompile(`(?s)description\s*=\s*"([^"]*)"`)
	sensitiveRe := regexp.MustCompile(`(?s)sensitive\s*=\s*(true|false)`)

	// Find all .tf files in the working directory (including subdirectories for modules)
	tfFiles, _ := filepath.Glob(filepath.Join(workDir, "*.tf"))
	// Also check subdirectories one level deep (for local modules)
	subTfFiles, _ := filepath.Glob(filepath.Join(workDir, "*", "*.tf"))
	tfFiles = append(tfFiles, subTfFiles...)

	for _, tfFile := range tfFiles {
		data, readErr := os.ReadFile(tfFile) //nolint:gosec // G304: validated path
		if readErr != nil {
			continue
		}

		// Modules
		for _, m := range moduleRe.FindAllSubmatch(data, -1) {
			if len(m) < 3 {
				continue
			}
			mod := TerraformModule{
				Name: string(m[1]),
			}
			block := m[2]
			if srcM := sourceRe.FindSubmatch(block); len(srcM) > 1 {
				mod.Source = string(srcM[1])
			}
			if verM := versionRe.FindSubmatch(block); len(verM) > 1 {
				mod.Version = string(verM[1])
			}
			inspection.Modules = append(inspection.Modules, mod)
		}

		// Resources
		for _, m := range resourceRe.FindAllSubmatch(data, -1) {
			if len(m) < 3 {
				continue
			}
			inspection.Resources = append(inspection.Resources, TerraformResourceDef{
				Type: string(m[1]),
				Name: string(m[2]),
			})
		}

		// Data sources
		for _, m := range dataRe.FindAllSubmatch(data, -1) {
			if len(m) < 3 {
				continue
			}
			inspection.DataSources = append(inspection.DataSources, TerraformResourceDef{
				Type: string(m[1]),
				Name: string(m[2]),
			})
		}

		// Outputs
		for _, m := range outputRe.FindAllSubmatch(data, -1) {
			if len(m) < 3 {
				continue
			}
			out := TerraformOutputDef{
				Name: string(m[1]),
			}
			block := m[2]
			if descM := descRe.FindSubmatch(block); len(descM) > 1 {
				out.Description = string(descM[1])
			}
			if sensM := sensitiveRe.FindSubmatch(block); len(sensM) > 1 {
				out.Sensitive = string(sensM[1]) == "true"
			}
			inspection.Outputs = append(inspection.Outputs, out)
		}

		// Backend (from terraform block)
		if backendM := backendRe.FindSubmatch(data); len(backendM) > 1 {
			inspection.Backend = string(backendM[1])
		}
	}

	// Try to list workspaces
	wsCmd := exec.CommandContext(ctx, "terraform", "workspace", "list")
	wsCmd.Dir = workDir
	if wsOut, wsErr := wsCmd.Output(); wsErr == nil {
		for _, line := range strings.Split(string(wsOut), "\n") {
			ws := strings.TrimSpace(line)
			ws = strings.TrimPrefix(ws, "* ") // current workspace has a * prefix
			if ws != "" {
				inspection.Workspaces = append(inspection.Workspaces, ws)
			}
		}
	}

	return json.Marshal(inspection)
}

// resolveTerraformDir returns the base directory for terraform operations.
// For repo-based configs it clones to a temp dir and returns a cleanup func.
// For local configs it validates and returns the local path directly.
func resolveTerraformDir(ctx context.Context, cfg *TerraformConfig) (baseDir string, cleanup func(), err error) {
	if cfg.isLocal() {
		absPath, err := filepath.Abs(cfg.LocalPath)
		if err != nil {
			return "", nil, fmt.Errorf("invalid local_path: %w", err)
		}
		info, statErr := os.Stat(absPath)
		if statErr != nil || !info.IsDir() {
			return "", nil, fmt.Errorf("local_path does not exist or is not a directory: %s", absPath)
		}
		return absPath, nil, nil
	}
	tmpDir, err := os.MkdirTemp("", "pepa-terraform-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	if err := gitClone(ctx, cfg.RepoURL, cfg.Token, tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("clone terraform repo: %w", err)
	}
	return tmpDir, func() { _ = os.RemoveAll(tmpDir) }, nil
}
