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
	"time"

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
	cancel     context.CancelFunc
	logBuf     *bytes.Buffer
	status     string // pending, running, success, failed, cancelled
	result     *AnsibleResult
	finishedAt time.Time // zero while still running
}

var (
	ansibleRunsMu sync.RWMutex
	ansibleRuns   = make(map[string]*ansibleRun)
)

func init() {
	// Background cleanup: remove completed ansible runs older than 5 minutes
	go func() {
		for {
			time.Sleep(60 * time.Second)
			ansibleRunsMu.Lock()
			for id, r := range ansibleRuns {
				if r.status != "running" && !r.finishedAt.IsZero() && time.Since(r.finishedAt) > 5*time.Minute {
					delete(ansibleRuns, id)
				}
			}
			ansibleRunsMu.Unlock()
		}
	}()
}

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
	data, err := os.ReadFile(absPlaybook) // #nosec //nolint:gosec // G304: absPlaybook is validated to be within workDir
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
	// NOTE: Do NOT delete from map here — keep logs accessible for polling.
	// A background cleanup will remove stale entries after 5 minutes.

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

	args := []string{cleanPlaybook}

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

	cmd := exec.CommandContext(runCtx, "ansible-playbook", args...) // #nosec //nolint:gosec // G204: ansible-playbook is an admin-configured binary
	cmd.Dir = workDir
	cmd.Stdout = logBuf
	cmd.Stderr = logBuf
	cmd.Env = append(os.Environ(),
		"ANSIBLE_NOCOLOR=false",
		"ANSIBLE_FORCE_COLOR=true",
		"ANSIBLE_HOME=/tmp/ansible",
		"ANSIBLE_LOCAL_TEMP=/tmp/ansible",
		"ANSIBLE_REMOTE_TEMP=/tmp/ansible-remote",
		"ANSIBLE_DEPRECATION_WARNINGS=false",
	)
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
	ansibleRun.finishedAt = time.Now()
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
	args := []string{cfg.Playbook, "--check", "--diff"}

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

	cmd := exec.CommandContext(ctx, "ansible-playbook", args...) // #nosec //nolint:gosec // G204: ansible-playbook is an admin-configured binary
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"ANSIBLE_NOCOLOR=false",
		"ANSIBLE_FORCE_COLOR=true",
		"ANSIBLE_HOME=/tmp/ansible",
		"ANSIBLE_LOCAL_TEMP=/tmp/ansible",
		"ANSIBLE_REMOTE_TEMP=/tmp/ansible-remote",
		"ANSIBLE_DEPRECATION_WARNINGS=false",
	)
	output, _ := cmd.CombinedOutput()

	return &PlanResult{
		HasChanges: true, // check mode always reports potential changes
		OutputText: string(output),
	}, nil
}

// State returns managed hosts discovered from Ansible inventory files.
func (a *AnsibleAdapter) State(ctx context.Context, raw json.RawMessage, _ map[string]any) (*StateResult, error) {
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

	result := &StateResult{}

	// Determine which inventory files to parse
	var inventoryFiles []string
	if cfg.Inventory != "" {
		// Use the configured inventory path
		invPath := filepath.Join(workDir, cfg.Inventory)
		if info, statErr := os.Stat(invPath); statErr == nil {
			if info.IsDir() {
				// Directory inventory: read all files inside
				entries, _ := os.ReadDir(invPath)
				for _, e := range entries {
					if !e.IsDir() {
						inventoryFiles = append(inventoryFiles, filepath.Join(invPath, e.Name()))
					}
				}
			} else {
				inventoryFiles = append(inventoryFiles, invPath)
			}
		}
	}

	// Also discover default inventory locations
	if len(inventoryFiles) == 0 {
		candidates := []string{"inventory", "hosts", "hosts.ini", "hosts.yml", "hosts.yaml"}
		for _, c := range candidates {
			p := filepath.Join(workDir, c)
			if _, statErr := os.Stat(p); statErr == nil {
				inventoryFiles = append(inventoryFiles, p)
				break
			}
		}
	}

	// Also check inventories/ directory
	invDir := filepath.Join(workDir, "inventories")
	if entries, err := os.ReadDir(invDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				inventoryFiles = append(inventoryFiles, filepath.Join(invDir, e.Name()))
			}
		}
	}

	seen := make(map[string]bool)
	for _, invFile := range inventoryFiles {
		hosts := parseInventoryFile(invFile)
		for _, h := range hosts {
			if !seen[h.Name] {
				seen[h.Name] = true
				result.Resources = append(result.Resources, h)
			}
		}
	}

	return result, nil
}

// parseInventoryFile reads an Ansible inventory file (INI or YAML) and returns host resources.
func parseInventoryFile(path string) []StateResource {
	data, err := os.ReadFile(path) // #nosec //nolint:gosec // G304: path is within validated workDir
	if err != nil {
		return nil
	}

	ext := filepath.Ext(path)
	if ext == ".yml" || ext == ".yaml" {
		return parseYAMLInventory(data)
	}
	return parseINIInventory(data)
}

// parseINIInventory parses INI-style Ansible inventory.
// Format:
//
//	[group]
//	host1 ansible_host=x.x.x.x
//	host2
func parseINIInventory(data []byte) []StateResource {
	var hosts []StateResource
	currentGroup := "ungrouped"
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Group header: [groupname]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentGroup = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			// Skip children/group vars sections
			if strings.HasSuffix(currentGroup, ":children") || strings.HasSuffix(currentGroup, ":vars") {
				currentGroup = strings.Split(currentGroup, ":")[0]
			}
			continue
		}

		// Host line: hostname [key=value ...]
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		hostname := fields[0]
		status := "unknown"
		ansibleHost := ""

		for _, kv := range fields[1:] {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) == 2 {
				switch parts[0] {
				case "ansible_host":
					ansibleHost = parts[1]
				case "ansible_connection":
					if parts[1] == "local" {
						status = "local"
					}
				}
			}
		}

		if status == "unknown" {
			status = "managed"
		}

		id := hostname
		if ansibleHost != "" {
			id = hostname + " (" + ansibleHost + ")"
		}

		hosts = append(hosts, StateResource{
			Type:     "host",
			Name:     hostname,
			ID:       id,
			Provider: "ansible/" + currentGroup,
			Status:   status,
		})
	}

	return hosts
}

// parseYAMLInventory parses YAML-style Ansible inventory.
// Format:
//
//	all:
//	  hosts:
//	    host1:
//	      ansible_host: x.x.x.x
//	  children:
//	    webservers:
//	      hosts:
//	        host1:
func parseYAMLInventory(data []byte) []StateResource {
	var inv map[string]interface{}
	if err := yaml.Unmarshal(data, &inv); err != nil {
		return nil
	}

	var hosts []StateResource
	seen := make(map[string]bool)

	var extractHosts func(node map[string]interface{}, group string)
	extractHosts = func(node map[string]interface{}, group string) {
		// Direct hosts
		if hostsRaw, ok := node["hosts"]; ok {
			if hostsMap, ok := hostsRaw.(map[string]interface{}); ok {
				for name, vars := range hostsMap {
					if seen[name] {
						continue
					}
					seen[name] = true
					status := "managed"
					id := name
					if varsMap, ok := vars.(map[string]interface{}); ok {
						if ah, ok := varsMap["ansible_host"].(string); ok && ah != "" {
							id = name + " (" + ah + ")"
						}
						if ac, ok := varsMap["ansible_connection"].(string); ok && ac == "local" {
							status = "local"
						}
					}
					hosts = append(hosts, StateResource{
						Type:     "host",
						Name:     name,
						ID:       id,
						Provider: "ansible/" + group,
						Status:   status,
					})
				}
			}
		}

		// Children groups
		if childrenRaw, ok := node["children"]; ok {
			if children, ok := childrenRaw.(map[string]interface{}); ok {
				for childName, childNode := range children {
					if childMap, ok := childNode.(map[string]interface{}); ok {
						extractHosts(childMap, childName)
					}
				}
			}
		}
	}

	// Start from "all" group or root
	if allGroup, ok := inv["all"].(map[string]interface{}); ok {
		extractHosts(allGroup, "all")
	} else {
		extractHosts(inv, "all")
	}

	return hosts
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

// Inspect parses the Ansible project directory to discover playbooks, roles, and inventories.
func (a *AnsibleAdapter) Inspect(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
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

	inspection := &AnsibleInspection{}

	// Discover playbooks: find all .yml/.yaml files and try to parse as playbooks
	_ = filepath.Walk(workDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			// Skip hidden dirs and common non-playbook dirs
			if info != nil && info.IsDir() {
				base := filepath.Base(path)
				if strings.HasPrefix(base, ".") || base == "roles" || base == "inventories" || base == "inventory" || base == "collection" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".yml" && ext != ".yaml" {
			return nil
		}
		// Skip files inside roles/ or inventories/
		rel, _ := filepath.Rel(workDir, path)
		if strings.HasPrefix(rel, "roles/") || strings.HasPrefix(rel, "inventories/") || strings.HasPrefix(rel, "inventory/") {
			return nil
		}

		data, readErr := os.ReadFile(path) // #nosec //nolint:gosec // G304: path is within validated workDir
		if readErr != nil {
			return nil
		}
		var plays []map[string]interface{}
		if yaml.Unmarshal(data, &plays) != nil {
			return nil // not a valid playbook
		}
		if len(plays) == 0 {
			return nil
		}
		// Check if it looks like a playbook (has hosts key in at least one play)
		hasHosts := false
		for _, p := range plays {
			if _, ok := p["hosts"]; ok {
				hasHosts = true
				break
			}
		}
		if !hasHosts {
			return nil
		}

		pb := AnsiblePlaybook{
			Name: strings.TrimSuffix(filepath.Base(path), ext),
			File: rel,
		}
		for _, p := range plays {
			play := AnsiblePlay{}
			if n, ok := p["name"].(string); ok {
				play.Name = n
			}
			if h, ok := p["hosts"].(string); ok {
				play.Hosts = h
			}
			// Extract roles
			if roles, ok := p["roles"].([]interface{}); ok {
				for _, r := range roles {
					switch rv := r.(type) {
					case string:
						play.Roles = append(play.Roles, rv)
					case map[string]interface{}:
						if rn, ok := rv["role"].(string); ok {
							play.Roles = append(play.Roles, rn)
						}
					}
				}
			}
			// Count tasks
			if tasks, ok := p["tasks"].([]interface{}); ok {
				play.Tasks = len(tasks)
			}
			if preTasks, ok := p["pre_tasks"].([]interface{}); ok {
				play.Tasks += len(preTasks)
			}
			if postTasks, ok := p["post_tasks"].([]interface{}); ok {
				play.Tasks += len(postTasks)
			}
			// Extract tags
			if tags, ok := p["tags"].([]interface{}); ok {
				for _, t := range tags {
					play.Tags = append(play.Tags, fmt.Sprintf("%v", t))
				}
			}
			pb.Plays = append(pb.Plays, play)
		}
		inspection.Playbooks = append(inspection.Playbooks, pb)
		return nil
	})

	// Discover roles from roles/ directory
	rolesDir := filepath.Join(workDir, "roles")
	if entries, err := os.ReadDir(rolesDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			role := AnsibleRole{
				Name: entry.Name(),
				Path: "roles/" + entry.Name(),
			}
			// Count tasks in the role
			tasksDir := filepath.Join(rolesDir, entry.Name(), "tasks")
			if taskEntries, err := os.ReadDir(tasksDir); err == nil {
				for _, te := range taskEntries {
					if !te.IsDir() && (filepath.Ext(te.Name()) == ".yml" || filepath.Ext(te.Name()) == ".yaml") {
						data, readErr := os.ReadFile(filepath.Join(tasksDir, te.Name())) // #nosec //nolint:gosec // G304: validated path
						if readErr == nil {
							var tasks []interface{}
							if yaml.Unmarshal(data, &tasks) == nil {
								role.Tasks += len(tasks)
							}
						}
					}
				}
			}
			// Try to read meta/main.yml for description
			metaPath := filepath.Join(rolesDir, entry.Name(), "meta", "main.yml")
			if metaData, err := os.ReadFile(metaPath); err == nil { // #nosec //nolint:gosec // G304: validated path
				var meta map[string]interface{}
				if yaml.Unmarshal(metaData, &meta) == nil {
					if galaxyInfo, ok := meta["galaxy_info"].(map[string]interface{}); ok {
						if desc, ok := galaxyInfo["description"].(string); ok {
							role.Description = desc
						}
					}
				}
			}
			inspection.Roles = append(inspection.Roles, role)
		}
	}

	// Also collect role names referenced in plays that aren't in roles/ dir
	roleNames := make(map[string]bool)
	for _, r := range inspection.Roles {
		roleNames[r.Name] = true
	}
	for _, pb := range inspection.Playbooks {
		for _, play := range pb.Plays {
			for _, rn := range play.Roles {
				if !roleNames[rn] {
					inspection.Roles = append(inspection.Roles, AnsibleRole{
						Name: rn,
						Path: "(external)",
					})
					roleNames[rn] = true
				}
			}
		}
	}

	// Discover inventories
	inventoryCandidates := []string{"inventory", "hosts", "hosts.ini", "hosts.yml", "hosts.yaml"}
	for _, candidate := range inventoryCandidates {
		p := filepath.Join(workDir, candidate)
		if _, err := os.Stat(p); err == nil {
			inspection.Inventories = append(inspection.Inventories, candidate)
		}
	}
	// Check inventories/ directory
	invDir := filepath.Join(workDir, "inventories")
	if entries, err := os.ReadDir(invDir); err == nil {
		for _, entry := range entries {
			inspection.Inventories = append(inspection.Inventories, "inventories/"+entry.Name())
		}
	}

	return json.Marshal(inspection)
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
		resolved, resolveErr := resolveContainerPath(cfg.LocalPath)
		if resolveErr != nil {
			return "", nil, resolveErr
		}
		return resolved, nil, nil
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

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", cloneURL, destDir) // #nosec //nolint:gosec // G204: git clone with validated args
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone failed: %s: %w", string(output), err)
	}
	return nil
}
