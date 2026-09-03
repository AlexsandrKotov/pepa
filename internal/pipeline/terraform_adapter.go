package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
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
	cancel     context.CancelFunc
	logBuf     *bytes.Buffer
	status     string // pending, running, success, failed, cancelled
	finishedAt time.Time
}

var (
	tfRunsMu sync.RWMutex
	tfRuns   = make(map[string]*tfRun)
)

func init() {
	// Background cleanup: remove completed terraform runs older than 5 minutes
	go func() {
		for {
			time.Sleep(60 * time.Second)
			tfRunsMu.Lock()
			for id, r := range tfRuns {
				if r.status != "running" && !r.finishedAt.IsZero() && time.Since(r.finishedAt) > 5*time.Minute {
					delete(tfRuns, id)
				}
			}
			tfRunsMu.Unlock()
		}
	}()
}

// TerraformAdapter implements Provider and EnhancedProvider for Terraform pipelines.
type TerraformAdapter struct{}

func NewTerraformAdapter() *TerraformAdapter {
	return &TerraformAdapter{}
}

func (a *TerraformAdapter) Name() string { return "terraform" }

// iacBinary resolves the IaC CLI binary name.
// Priority: IAC_BINARY env var > "tofu" in PATH > "terraform" in PATH.
// Falls back to "tofu" (will produce a clear exec error if missing).
func iacBinary() string {
	if v := os.Getenv("IAC_BINARY"); v != "" {
		return v
	}
	if p, err := exec.LookPath("tofu"); err == nil {
		return p
	}
	if p, err := exec.LookPath("terraform"); err == nil {
		return p
	}
	return "tofu" // will fail with clear "executable not found" message
}

// selectWorkspace selects or creates a terraform workspace if requested via params.
// It runs "terraform workspace select <name>" or "terraform workspace new <name>" if it doesn't exist.
func selectWorkspace(ctx context.Context, bin, workDir, workspace string) error {
	if workspace == "" || workspace == "default" {
		return nil
	}
	// Try to select existing workspace
	cmd := exec.CommandContext(ctx, bin, "workspace", "select", workspace) //nolint:gosec // #nosec // G204: bin and workspace from pipeline config
	cmd.Dir = workDir
	if err := cmd.Run(); err == nil {
		return nil
	}
	// If select failed, try to create it
	cmd = exec.CommandContext(ctx, bin, "workspace", "new", workspace) //nolint:gosec // #nosec // G204: bin and workspace from pipeline config
	cmd.Dir = workDir
	return cmd.Run()
}

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
		data, err := os.ReadFile(tfFile) //nolint:gosec // #nosec // G304: tfFile is from a validated directory listing
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

	// If the backend block references variables, expose corresponding backend_* properties
	// so users can provide concrete values via pipeline parameters.
	addBackendSchemaProps(workDir, props)

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

// writeBackendOverride extracts backend config from pipeline parameters
// (keys prefixed with "backend_") and writes a pepa_backend_override.tf file
// that Terraform/OpenTofu merges with the main configuration during init.
// This allows users to provide backend values via pipeline parameters instead
// of hardcoding them or using variables (which are not available at init time).
// Returns the override file path (empty if no override was written) and the set
// of backend config keys so the caller can exclude them from -var flags.
func writeBackendOverride(workDir string, params map[string]any) (string, map[string]bool) {
	backendConfig := make(map[string]string)
	for k, v := range params {
		if strings.HasPrefix(k, "backend_") {
			key := strings.TrimPrefix(k, "backend_")
			backendConfig[key] = fmt.Sprintf("%v", v)
		}
	}
	if len(backendConfig) == 0 {
		return "", nil
	}

	// Build HCL override file
	var buf bytes.Buffer
	buf.WriteString("# Generated by PEPA — provides backend config values at init time\n")
	buf.WriteString("terraform {\n")
	// Detect backend type from existing .tf files
	backendType := detectBackendType(workDir)
	if backendType != "" {
		buf.WriteString(fmt.Sprintf("  backend %q {\n", backendType))
	} else {
		buf.WriteString("  backend \"http\" {\n")
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(backendConfig))
	for k := range backendConfig {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		buf.WriteString(fmt.Sprintf("    %s = %q\n", k, backendConfig[k]))
	}
	buf.WriteString("  }\n")
	buf.WriteString("}\n")

	overridePath := filepath.Join(workDir, "pepa_backend_override.tf")
	if err := os.WriteFile(overridePath, buf.Bytes(), 0600); err != nil {
		slog.Error("failed to write backend override file", "path", overridePath, "error", err)
		return "", nil
	}

	backendKeys := make(map[string]bool)
	for k := range backendConfig {
		backendKeys["backend_"+k] = true
	}
	return overridePath, backendKeys
}

// detectBackendType scans .tf files for a backend "type" declaration.
func detectBackendType(workDir string) string {
	backendRe := regexp.MustCompile(`backend\s+"([^"]+)"`)
	files, _ := filepath.Glob(filepath.Join(workDir, "*.tf"))
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // #nosec // G304: validated directory listing
		if err != nil {
			continue
		}
		if m := backendRe.FindSubmatch(data); len(m) > 1 {
			return string(m[1])
		}
	}
	return ""
}

// addBackendSchemaProps detects variable references in the backend block and adds
// corresponding backend_* properties to the schema. This allows users to provide
// concrete backend config values via pipeline parameters.
// For example, if the backend block has: address = var.gitlab_remote_state_address
// then a "backend_address" property is added to the schema.
func addBackendSchemaProps(workDir string, props map[string]PropertyDef) {
	// Match the full backend block content
	backendBlockRe := regexp.MustCompile(`(?s)backend\s+"[^"]+"\s*\{([^}]*)\}`)
	// Match attribute = var.name references within the block
	attrVarRe := regexp.MustCompile(`(\w+)\s*=\s*var\.(\w+)`)

	files, _ := filepath.Glob(filepath.Join(workDir, "*.tf"))
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // #nosec // G304: validated directory listing
		if err != nil {
			continue
		}
		backendMatch := backendBlockRe.FindSubmatch(data)
		if len(backendMatch) < 2 {
			continue
		}
		blockContent := string(backendMatch[1])
		attrMatches := attrVarRe.FindAllStringSubmatch(blockContent, -1)
		for _, attrMatch := range attrMatches {
			if len(attrMatch) < 3 {
				continue
			}
			attrName := attrMatch[1]  // e.g. "address"
			varName := attrMatch[2]   // e.g. "gitlab_remote_state_address"
			propKey := "backend_" + attrName // e.g. "backend_address"
			if _, exists := props[propKey]; !exists {
				props[propKey] = PropertyDef{
					Type:        "string",
					Description: fmt.Sprintf("Backend config: %s (replaces var.%s)", attrName, varName),
				}
			}
		}
		break // only need to process the first backend block found
	}
}

// sanitizeBackendVars replaces var.* references in backend blocks with empty strings.
// Terraform backend config is resolved at init time before variables are available,
// so var.* references in backend blocks always fail. The actual values are provided
// by the pepa_backend_override.tf file written by writeBackendOverride.
func sanitizeBackendVars(workDir string) {
	backendBlockRe := regexp.MustCompile(`(?s)(backend\s+"[^"]+"\s*\{)([^}]*)(\})`)
	varRefRe := regexp.MustCompile(`var\.\w+`)

	files, _ := filepath.Glob(filepath.Join(workDir, "*.tf"))
	for _, f := range files {
		// Skip the override file we generated
		if strings.HasSuffix(f, "pepa_backend_override.tf") {
			continue
		}
		data, err := os.ReadFile(f) //nolint:gosec // #nosec // G304: validated directory listing
		if err != nil {
			continue
		}
		if !backendBlockRe.Match(data) {
			continue
		}
		modified := backendBlockRe.ReplaceAllFunc(data, func(match []byte) []byte {
			return varRefRe.ReplaceAll(match, []byte(`""`))
		})
		if !bytes.Equal(data, modified) {
			_ = os.WriteFile(f, modified, 0600) //nolint:gosec // #nosec // G703: f from validated directory listing
		}
	}
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
	// NOTE: Do NOT delete from map here — keep logs accessible for polling.
	// A background cleanup will remove stale entries after 5 minutes.

	// Inject backend config override if backend_* parameters are provided.
	// This solves the Terraform limitation where backend blocks cannot use variables.
	overridePath, backendKeys := writeBackendOverride(workDir, params)
	if overridePath != "" {
		defer func() { _ = os.Remove(overridePath) }()
	}

	// Replace var.* references in backend blocks with empty strings.
	// Backend config is resolved at init time before variables are available,
	// so var.* references always fail. The override file provides real values.
	sanitizeBackendVars(workDir)

	// Run IaC init
	bin := iacBinary()
	initCmd := exec.CommandContext(runCtx, bin, "init", "-input=false") //nolint:gosec // #nosec // G204: bin from pipeline config
	initCmd.Dir = workDir
	initCmd.Stdout = logBuf
	initCmd.Stderr = logBuf
	if err := initCmd.Run(); err != nil {
		tfRunsMu.Lock()
		tfRuns[runID].status = "failed"
		tfRuns[runID].finishedAt = time.Now()
		tfRunsMu.Unlock()
		return nil, fmt.Errorf("iac init failed: %s: %w", logBuf.String(), err)
	}

	// Select workspace if requested
	if ws, ok := params["workspace"].(string); ok {
		if wsErr := selectWorkspace(runCtx, bin, workDir, ws); wsErr != nil {
			fmt.Fprintf(logBuf, "Warning: workspace select failed: %s\n", wsErr)
		}
	}

	// Build IaC command args
	args := []string{action, "-input=false", "-no-color"}

	if autoApprove, ok := params["auto_approve"]; ok && (autoApprove == "true" || autoApprove == true) {
		args = append(args, "-auto-approve")
	}

	// Add -var flags for extra params (skip backend_* keys and internal params)
	for k, v := range params {
		if k == "tf_action" || k == "auto_approve" || k == "ref" || k == "workspace" {
			continue
		}
		if backendKeys != nil && backendKeys[k] {
			continue
		}
		args = append(args, "-var", fmt.Sprintf("%s=%v", k, v))
	}

	tfCmd := exec.CommandContext(runCtx, bin, args...) //nolint:gosec // #nosec // G204: bin and args from pipeline config
	tfCmd.Dir = workDir
	tfCmd.Stdout = logBuf
	tfCmd.Stderr = logBuf
	if err := tfCmd.Run(); err != nil {
		tfRunsMu.Lock()
		tfRuns[runID].status = "failed"
		tfRuns[runID].finishedAt = time.Now()
		tfRunsMu.Unlock()
		return &TriggerResult{
			ExternalRunID: runID,
			ExternalURL:   "",
			Status:        "failed",
		}, fmt.Errorf("terraform %s failed: %w", action, err)
	}

	tfRunsMu.Lock()
	tfRuns[runID].status = "success"
	tfRuns[runID].finishedAt = time.Now()
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

	// Inject backend config override if backend_* parameters are provided.
	overridePath, backendKeys := writeBackendOverride(workDir, params)
	if overridePath != "" {
		defer func() { _ = os.Remove(overridePath) }()
	}

	// Replace var.* references in backend blocks with empty strings.
	sanitizeBackendVars(workDir)

	// IaC init
	bin := iacBinary()
	initCmd := exec.CommandContext(ctx, bin, "init", "-input=false", "-no-color") //nolint:gosec // #nosec // G204: bin from pipeline config
	initCmd.Dir = workDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("iac init failed: %s: %w", string(output), err)
	}

	// Select workspace if requested
	if ws, ok := params["workspace"].(string); ok {
		if wsErr := selectWorkspace(ctx, bin, workDir, ws); wsErr != nil {
			// Workspace selection failure will surface in plan output
			_ = wsErr
		}
	}

	// terraform plan with JSON output
	// planFile is written to a validated workDir (already checked above), so the path is safe.
	planFile := filepath.Join(workDir, "tfplan")
	planArgs := []string{"plan", "-input=false", "-no-color", "-out=" + planFile}
	for k, v := range params {
		if k == "tf_action" || k == "auto_approve" || k == "ref" || k == "workspace" {
			continue
		}
		if backendKeys != nil && backendKeys[k] {
			continue
		}
		planArgs = append(planArgs, "-var", fmt.Sprintf("%s=%v", k, v))
	}

	planCmd := exec.CommandContext(ctx, bin, planArgs...) //nolint:gosec // #nosec // G204: IaC CLI is an expected subprocess
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
	showCmd := exec.CommandContext(ctx, bin, "show", "-json", planFile) //nolint:gosec // #nosec // G204: IaC CLI is an expected subprocess
	showCmd.Dir = workDir
	if jsonOutput, err := showCmd.Output(); err == nil {
		result.OutputJSON = string(jsonOutput)
	}

	// Clean up plan file
	_ = os.Remove(planFile)

	return result, nil
}

// State returns the current terraform state as structured data.
// It initializes the backend before running `terraform show -json` so that
// remote state backends (e.g. GitLab HTTP) work correctly.
func (a *TerraformAdapter) State(ctx context.Context, raw json.RawMessage, params map[string]any) (*StateResult, error) {
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

	// Inject backend config override if backend_* parameters are provided.
	overridePath, _ := writeBackendOverride(workDir, params)
	if overridePath != "" {
		defer func() { _ = os.Remove(overridePath) }()
	}

	// Replace var.* references in backend blocks with empty strings.
	sanitizeBackendVars(workDir)

	// Initialize the backend before reading state.
	bin := iacBinary()
	initCmd := exec.CommandContext(ctx, bin, "init", "-input=false", "-no-color") //nolint:gosec // #nosec // G204: bin from pipeline config
	initCmd.Dir = workDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("iac init failed during state fetch: %s: %w", string(output), err)
	}

	// IaC show -json
	showCmd := exec.CommandContext(ctx, bin, "show", "-json") //nolint:gosec // #nosec // G204: bin from pipeline config
	showCmd.Dir = workDir
	jsonOutput, err := showCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("iac show failed: %w", err)
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
		data, readErr := os.ReadFile(tfFile) //nolint:gosec // #nosec // G304: validated path
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
	bin := iacBinary()
	wsCmd := exec.CommandContext(ctx, bin, "workspace", "list") //nolint:gosec // #nosec // G204: bin from pipeline config
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
// For local configs it copies to a temp dir (since host mounts are read-only)
// and returns a cleanup func.
func resolveTerraformDir(ctx context.Context, cfg *TerraformConfig) (baseDir string, cleanup func(), err error) {
	if cfg.isLocal() {
		resolved, resolveErr := resolveContainerPath(cfg.LocalPath)
		if resolveErr != nil {
			return "", nil, resolveErr
		}
		// Copy to temp dir because host mounts are read-only and Terraform
		// needs write access for .terraform/, state files, and plan files.
		tmpDir, mkdirErr := os.MkdirTemp("", "pepa-terraform-*")
		if mkdirErr != nil {
			return "", nil, fmt.Errorf("create temp dir: %w", mkdirErr)
		}
		if copyErr := copyDir(resolved, tmpDir); copyErr != nil {
			_ = os.RemoveAll(tmpDir)
			return "", nil, fmt.Errorf("copy terraform project: %w", copyErr)
		}
		return tmpDir, func() { _ = os.RemoveAll(tmpDir) }, nil
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

// copyDir recursivelyally copies a directory from src to dst.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0750); err != nil { //nolint:gosec // #nosec // G301: standard directory copy
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath) //nolint:gosec // #nosec // G304: srcPath from copyDir input
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0600); err != nil { //nolint:gosec // #nosec // G306: standard file copy
				return err
			}
		}
	}
	return nil
}
