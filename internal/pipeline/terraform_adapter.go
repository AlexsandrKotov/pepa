package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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

// TerraformAdapter implements Provider for Terraform pipelines.
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
		data, err := os.ReadFile(tfFile)
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

	// Run terraform init
	initCmd := exec.CommandContext(ctx, "terraform", "init", "-input=false")
	initCmd.Dir = workDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("terraform init failed: %s: %w", string(output), err)
	}

	// Build terraform command args
	args := []string{action, "-input=false"}

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

	tfCmd := exec.CommandContext(ctx, "terraform", args...)
	tfCmd.Dir = workDir
	if output, err := tfCmd.CombinedOutput(); err != nil {
		_ = output
		return nil, fmt.Errorf("terraform %s failed: %w", action, err)
	}

	runID := fmt.Sprintf("tf-%d", os.Getpid())
	return &TriggerResult{
		ExternalRunID: runID,
		ExternalURL:   "",
		Status:        "success",
	}, nil
}

// Status returns the status of a Terraform run (not tracked externally).
func (a *TerraformAdapter) Status(ctx context.Context, raw json.RawMessage, externalRunID string) (*RunStatus, error) {
	return &RunStatus{
		ExternalRunID: externalRunID,
		Status:        "success",
	}, nil
}

// Jobs returns no job info for Terraform.
func (a *TerraformAdapter) Jobs(ctx context.Context, raw json.RawMessage, externalRunID string) ([]JobInfo, error) {
	return []JobInfo{}, nil
}

// Logs returns empty logs for Terraform.
func (a *TerraformAdapter) Logs(ctx context.Context, raw json.RawMessage, externalRunID string, jobID string) (string, error) {
	return "", nil
}

// Cancel is a no-op for Terraform.
func (a *TerraformAdapter) Cancel(ctx context.Context, raw json.RawMessage, externalRunID string) error {
	return nil
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
		os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("clone terraform repo: %w", err)
	}
	return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
}
