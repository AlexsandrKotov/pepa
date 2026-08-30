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

	"gopkg.in/yaml.v3"
)

// AnsibleConfig is the expected shape of PipelineSource.Config for ansible sources.
type AnsibleConfig struct {
	RepoURL   string `json:"repo_url"`
	LocalPath string `json:"local_path"` // absolute path on the server for local playbooks
	Playbook  string `json:"playbook"`   // e.g. "site.yml"
	Inventory string `json:"inventory"`  // e.g. "inventory"
	Token     string `json:"token"`      // git/repo token
}

func parseAnsibleConfig(raw json.RawMessage) (*AnsibleConfig, error) {
	var cfg AnsibleConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid ansible config: %w", err)
	}
	if cfg.RepoURL == "" && cfg.LocalPath == "" {
		return nil, fmt.Errorf("ansible config: repo_url or local_path is required")
	}
	if cfg.Playbook == "" {
		cfg.Playbook = "site.yml"
	}
	return &cfg, nil
}

// isLocal returns true when the config points to a local directory instead of a git repo.
func (c *AnsibleConfig) isLocal() bool {
	return c.LocalPath != "" && c.RepoURL == ""
}

// ── Active run tracking ────────────────────────────────────────

// ansibleRun tracks an in-flight ansible-playbook execution.
type ansibleRun struct {
	cancel context.CancelFunc
	logBuf *bytes.Buffer
	status string // pending, running, success, failed, cancelled
	result *AnsibleResult
}

var (
	ansibleRunsMu sync.RWMutex
	ansibleRuns   = make(map[string]*ansibleRun)
)

// AnsibleAdapter implements Provider and EnhancedProvider for Ansible pipelines.
type AnsibleAdapter struct{}

func NewAnsibleAdapter() *AnsibleAdapter {
	return &AnsibleAdapter{}
}

func (a *AnsibleAdapter) Name() string { return "ansible" }

// ResolveSchema clones the repo (or reads local path) and parses the playbook to extract variables.
func (a *AnsibleAdapter) ResolveSchema(ctx context.Context, raw json.RawMessage) (*ParameterSchema, error) {
	cfg, err := parseAnsibleConfig(raw)
	if err != nil {
		return nil, err
	}

	workDir, cleanup, err := resolveAnsibleDir(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	props := make(map[string]PropertyDef)

	// Parse the playbook to extract vars and vars_prompt
	// Clean the path to prevent path traversal
	cleanPlaybook := filepath.Clean(cfg.Playbook)
	if strings.Contains(cleanPlaybook, "..") {
		cleanPlaybook = filepath.Base(cleanPlaybook)
	}
	playbookPath := filepath.Join(workDir, cleanPlaybook)
	// Verify the resolved path is within workDir
	absPlaybook, _ := filepath.Abs(playbookPath)
	absWorkDir, _ := filepath.Abs(workDir)
	if !strings.HasPrefix(absPlaybook, absWorkDir) {
		return nil, fmt.Errorf("invalid playbook path")
	}
	data, err := os.ReadFile(absPlaybook) //nolint:gosec // G304: absPlaybook is validated to be within workDir
	if err != nil {
		// Fallback: just provide basic params
		props["inventory"] = PropertyDef{Type: "string", Description: "Inventory file/host list", Default: cfg.Inventory}
		props["limit"] = PropertyDef{Type: "string", Description: "Limit to specific hosts/groups"}
		props["tags"] = PropertyDef{Type: "string", Description: "Only run plays/tasks with these tags"}
		return &ParameterSchema{
			Type:       "object",
			Properties: props,
			Required:   []string{"inventory"},
		}, nil
	}

	var plays []map[string]interface{}
	if yaml.Unmarshal(data, &plays) == nil {
		for _, play := range plays {
			// Extract vars: section
			if vars, ok := play["vars"].(map[string]interface{}); ok {
				for k, v := range vars {
					if _, exists := props[k]; !exists {
						pd := PropertyDef{
							Type:        "string",
							Description: fmt.Sprintf("Ansible variable: %s", k),
						}
						if v != nil {
							pd.Default = fmt.Sprintf("%v", v)
						}
						props[k] = pd
					}
				}
			}

			// Extract vars_prompt: section (interactive prompts)
			if varsPrompt, ok := play["vars_prompt"].([]interface{}); ok {
				for _, vp := range varsPrompt {
					if vpMap, ok := vp.(map[string]interface{}); ok {
						name, _ := vpMap["name"].(string)
						if name == "" {
							continue
						}
						if _, exists := props[name]; !exists {
							pd := PropertyDef{
								Type:        "string",
								Description: fmt.Sprintf("Prompted variable: %s", name),
							}
							if def, ok := vpMap["default"]; ok && def != nil {
								pd.Default = fmt.Sprintf("%v", def)
							}
							// If prompt is private, mark as sensitive
							if priv, ok := vpMap["private"].(bool); ok && priv {
								pd.Description += " (sensitive)"
							}
							props[name] = pd
						}
					}
				}
			}
		}
	}

	// Always include standard ansible params
	if _, ok := props["inventory"]; !ok {
		props["inventory"] = PropertyDef{Type: "string", Description: "Inventory file/host list", Default: cfg.Inventory}
	}
	if _, ok := props["limit"]; !ok {
		props["limit"] = PropertyDef{Type: "string", Description: "Limit to specific hosts/groups"}
	}
	if _, ok := props["tags"]; !ok {
		props["tags"] = PropertyDef{Type: "string", Description: "Only run plays/tasks with these tags"}
	}
	if _, ok := props["dry_run"]; !ok {
		props["dry_run"] = PropertyDef{
			Type:        "boolean",
			Description: "Run in check mode (--check) without making changes",
			Default:     "false",
		}
	}

	return &ParameterSchema{
		Type:       "object",
		Properties: props,
		Required:   []string{"inventory"},
	}, nil
}

// Trigger runs an Ansible playbook with the given parameters.
// Output is captured and parsed for per-host results.
func (a *AnsibleAdapter) Trigger(ctx context.Context, raw json.RawMessage, params map[string]any) (*TriggerResult, error) {
	cfg, err := parseAnsibleConfig(raw)
	if err != nil {
		return nil, err
	}

	workDir, cleanup, err := resolveAnsibleDir(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	logBuf := &bytes.Buffer{}
	runID := fmt.Sprintf("ansible-%s", randomRunID())

	// Register the run for tracking
	ansibleRunsMu.Lock()
	ansibleRuns[runID] = &ansibleRun{cancel: cancel, logBuf: logBuf, status: "running"}
	ansibleRunsMu.Unlock()
	defer func() {
		ansibleRunsMu.Lock()
		delete(ansibleRuns, runID)
		ansibleRunsMu.Unlock()
	}()

	// Build ansible-playbook command
	// Validate playbook path to prevent path traversal
	cleanPlaybook := filepath.Clean(cfg.Playbook)
	if strings.Contains(cleanPlaybook, "..") {
		cleanPlaybook = filepath.Base(cleanPlaybook)
	}
	playbookPath := filepath.Join(workDir, cleanPlaybook)
	absPlaybook, _ := filepath.Abs(playbookPath)
	absWorkDir, _ := filepath.Abs(workDir)
	if !strings.HasPrefix(absPlaybook, absWorkDir) {
		return nil, fmt.Errorf("invalid playbook path")
	}

	args := []string{cleanPlaybook, "--no-color"}

	if inv, ok := params["inventory"]; ok && inv != "" {
		args = append(args, "-i", fmt.Sprintf("%v", inv))
	}
	if limit, ok := params["limit"]; ok && limit != "" {
		args = append(args, "--limit", fmt.Sprintf("%v", limit))
	}
	if tags, ok := params["tags"]; ok && tags != "" {
		args = append(args, "--tags", fmt.Sprintf("%v", tags))
	}
	// Dry-run / check mode
	if dryRun, ok := params["dry_run"]; ok && (dryRun == "true" || dryRun == true) {
		args = append(args, "--check")
	}

	// Add extra vars
	for k, v := range params {
		if k == "inventory" || k == "limit" || k == "tags" || k == "ref" || k == "dry_run" {
			continue
		}
		args = append(args, "-e", fmt.Sprintf("%s=%v", k, v))
	}

	cmd := exec.CommandContext(runCtx, "ansible-playbook", args...) //nolint:gosec // G204: ansible-playbook is an admin-configured binary
	cmd.Dir = workDir
	cmd.Stdout = logBuf
	cmd.Stderr = logBuf
	cmdErr := cmd.Run()

	// Parse the output for per-host results
	parsed := parseAnsibleOutput(logBuf.String(), cfg.Playbook)

	ansibleRunsMu.Lock()
	ansibleRun := ansibleRuns[runID]
	if cmdErr != nil {
		ansibleRun.status = "failed"
	} else {
		ansibleRun.status = "success"
	}
	ansibleRun.result = parsed
	ansibleRunsMu.Unlock()

	status := "success"
	if cmdErr != nil {
		status = "failed"
	}

	return &TriggerResult{
		ExternalRunID: runID,
		ExternalURL:   "",
		Status:        status,
	}, nil
}

// Plan runs the playbook in --check mode (dry-run) and returns a preview.
func (a *AnsibleAdapter) Plan(ctx context.Context, raw json.RawMessage, params map[string]any) (*PlanResult, error) {
	cfg, err := parseAnsibleConfig(raw)
	if err != nil {
		return nil, err
	}

	workDir, cleanup, err := resolveAnsibleDir(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Build ansible-playbook --check command
	args := []string{cfg.Playbook, "--check", "--no-color", "--diff"}

	if inv, ok := params["inventory"]; ok && inv != "" {
		args = append(args, "-i", fmt.Sprintf("%v", inv))
	}
	if limit, ok := params["limit"]; ok && limit != "" {
		args = append(args, "--limit", fmt.Sprintf("%v", limit))
	}
	if tags, ok := params["tags"]; ok && tags != "" {
		args = append(args, "--tags", fmt.Sprintf("%v", tags))
	}

	for k, v := range params {
		if k == "inventory" || k == "limit" || k == "tags" || k == "ref" || k == "dry_run" {
			continue
		}
		args = append(args, "-e", fmt.Sprintf("%s=%v", k, v))
	}

	cmd := exec.CommandContext(ctx, "ansible-playbook", args...) //nolint:gosec // G204: ansible-playbook is an admin-configured binary
	cmd.Dir = workDir
	output, _ := cmd.CombinedOutput()

	return &PlanResult{
		HasChanges: true, // check mode always reports potential changes
		OutputText: string(output),
	}, nil
}

// State is a no-op for Ansible (configuration management, not infrastructure state).
func (a *AnsibleAdapter) State(ctx context.Context, raw json.RawMessage) (*StateResult, error) {
	return &StateResult{
		Resources: []StateResource{},
	}, nil
}

// Status returns the status of an Ansible run.
func (a *AnsibleAdapter) Status(ctx context.Context, raw json.RawMessage, externalRunID string) (*RunStatus, error) {
	ansibleRunsMu.RLock()
	run, ok := ansibleRuns[externalRunID]
	ansibleRunsMu.RUnlock()

	status := "success"
	if ok {
		status = run.status
	}

	return &RunStatus{
		ExternalRunID: externalRunID,
		Status:        status,
	}, nil
}

// Jobs returns no job info for Ansible (single-play execution).
func (a *AnsibleAdapter) Jobs(ctx context.Context, raw json.RawMessage, externalRunID string) ([]JobInfo, error) {
	return []JobInfo{}, nil
}

// Logs returns captured output from an Ansible run.
func (a *AnsibleAdapter) Logs(ctx context.Context, raw json.RawMessage, externalRunID string, jobID string) (string, error) {
	ansibleRunsMu.RLock()
	run, ok := ansibleRuns[externalRunID]
	ansibleRunsMu.RUnlock()

	if ok && run.logBuf != nil {
		return run.logBuf.String(), nil
	}
	return "", nil
}

// Cancel cancels a running Ansible execution.
func (a *AnsibleAdapter) Cancel(ctx context.Context, raw json.RawMessage, externalRunID string) error {
	ansibleRunsMu.RLock()
	run, ok := ansibleRuns[externalRunID]
	ansibleRunsMu.RUnlock()

	if ok && run.cancel != nil {
		run.cancel()
		ansibleRunsMu.Lock()
		run.status = "cancelled"
		ansibleRunsMu.Unlock()
		return nil
	}
	return fmt.Errorf("no active run found for %s", externalRunID)
}

// ── Output parsing helpers ──────────────────────────────────────

// parseAnsibleOutput extracts per-host task summary from ansible-playbook stdout.
// It looks for the PLAY RECAP section and parses lines like:
//
//	host1 : ok=3 changed=1 unreachable=0 failed=0 skipped=1
func parseAnsibleOutput(output, playbook string) *AnsibleResult {
	result := &AnsibleResult{
		Hosts:    make(map[string]HostResult),
		Playbook: playbook,
	}

	// Find PLAY RECAP section
	recapIdx := strings.Index(output, "PLAY RECAP")
	if recapIdx < 0 {
		return result
	}
	recapSection := output[recapIdx:]

	// Parse each host line: hostname : ok=N changed=N unreachable=N failed=N skipped=N
	hostLineRe := regexp.MustCompile(`(?m)^(\S+)\s+:\s+ok=(\d+)\s+changed=(\d+)\s+unreachable=(\d+)\s+failed=(\d+)\s+skipped=(\d+)`)
	matches := hostLineRe.FindAllStringSubmatch(recapSection, -1)
	for _, m := range matches {
		if len(m) < 7 {
			continue
		}
		host := m[1]
		var hr HostResult
		_, _ = fmt.Sscanf(m[2], "%d", &hr.OK)
		_, _ = fmt.Sscanf(m[3], "%d", &hr.Changed)
		_, _ = fmt.Sscanf(m[5], "%d", &hr.Failed)
		_, _ = fmt.Sscanf(m[6], "%d", &hr.Skipped)
		result.Hosts[host] = hr
	}

	return result
}

// resolveAnsibleDir returns the working directory for ansible operations.
// For repo-based configs it clones to a temp dir and returns a cleanup func.
// For local configs it validates and returns the local path directly.
func resolveAnsibleDir(ctx context.Context, cfg *AnsibleConfig) (workDir string, cleanup func(), err error) {
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
	tmpDir, err := os.MkdirTemp("", "pepa-ansible-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	if err := gitClone(ctx, cfg.RepoURL, cfg.Token, tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("clone ansible repo: %w", err)
	}
	return tmpDir, func() { _ = os.RemoveAll(tmpDir) }, nil
}

// gitClone performs a shallow git clone, injecting the token into the URL if provided.
func gitClone(ctx context.Context, repoURL, token, destDir string) error {
	cloneURL := repoURL
	if token != "" && !strings.Contains(cloneURL, token) {
		// Inject token for HTTPS URLs
		if strings.HasPrefix(cloneURL, "https://") {
			cloneURL = strings.Replace(cloneURL, "https://", fmt.Sprintf("https://x-access-token:%s@", token), 1)
		}
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", cloneURL, destDir) //nolint:gosec // G204: git clone with validated args
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone failed: %s: %w", string(output), err)
	}
	return nil
}
