package gitops

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Editor handles YAML patching and Git commit/push for GitOps manifests.
type Editor struct {
	CacheDir string
	// sem limits concurrent edit operations (full clones are expensive).
	sem chan struct{}
}

// NewEditor creates a new Editor.
func NewEditor(cacheDir string) *Editor {
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "pepa-gitops-cache")
	}
	return &Editor{
		CacheDir: cacheDir,
		sem:      make(chan struct{}, 2), // max 2 concurrent edits
	}
}

// EditRequest describes a value change to a manifest file.
type EditRequest struct {
	FilePath  string      `json:"file_path"`  // relative path in repo
	FieldPath string      `json:"field_path"` // dot-separated path e.g. "spec.values.replicas"
	NewValue  interface{} `json:"new_value"`  // new value to set
	FullYAML  string      `json:"full_yaml"`  // if set, replace entire file content
	CommitMsg string      `json:"commit_message"`
	Branch    string      `json:"branch"` // target branch (default: repo branch)
}

// CommitResult is returned after a successful edit + commit.
type CommitResult struct {
	CommitSHA string `json:"commit_sha"`
	Branch    string `json:"branch"`
	MRURL     string `json:"mr_url,omitempty"` // set if a feature branch was created instead of direct push
	MRNeeded  bool   `json:"mr_needed"`        // true if push was rejected and a branch/MR was created
	Diff      string `json:"diff,omitempty"`   // unified diff of the change
}

// ApplyEdit clones the repo, applies the edit, commits, and pushes.
func (e *Editor) ApplyEdit(ctx context.Context, repo *Repo, req *EditRequest) (*CommitResult, error) {
	// Acquire semaphore to limit concurrent edits
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Build authenticated URL
	repoURL := repo.RepoURL
	token, _ := repo.Config["token"]
	if token != "" {
		repoURL = injectToken(repoURL, token)
	}

	// Validate client-supplied target branch to block argument injection.
	if req.Branch != "" && !ValidBranchName(req.Branch) {
		return nil, fmt.Errorf("invalid branch name: %q", req.Branch)
	}

	// Full clone (needed for commit/push)
	tmpDir, err := os.MkdirTemp("", "pepa-gitops-edit-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Shallow clone with fetch depth 50 (enough history for push, but much faster)
	cloneArgs := []string{
		"clone",
		"--depth", "50",
		"--branch", repo.Branch,
		"--single-branch",
		"--no-tags",
		repoURL,
		tmpDir,
	}

	cmd := exec.CommandContext(ctx, "git", cloneArgs...) // #nosec // G204: git clone with validated args
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git clone: %s: %w", maskGitSecrets(string(out), token), err)
	}

	// Configure git user for commit — use the authenticated user's identity if available,
	// so commits are authored under their account in the remote service.
	gitName := "PEPA GitOps"
	gitEmail := "pepa@gitops.local"
	if n, ok := repo.Config["git_user_name"]; ok && n != "" {
		gitName = n
	}
	if e, ok := repo.Config["git_user_email"]; ok && e != "" {
		gitEmail = e
	}
	for _, cfg := range []string{"user.email=" + gitEmail, "user.name=" + gitName} {
		cmd = exec.CommandContext(ctx, "git", "-C", tmpDir, "config", strings.SplitN(cfg, "=", 2)[0], strings.SplitN(cfg, "=", 2)[1]) // #nosec // G204: git config with validated args
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("git config: %s: %w", string(out), err)
		}
	}

	// Clean and resolve file path
	cleanPath := filepath.Clean(req.FilePath)
	if strings.HasPrefix(cleanPath, "..") || strings.HasPrefix(cleanPath, "/") {
		return nil, fmt.Errorf("invalid file path: must be relative within repo")
	}
	targetPath := filepath.Join(tmpDir, cleanPath)
	// Prevent path traversal
	absClone, _ := filepath.Abs(tmpDir)
	absTarget, _ := filepath.Abs(targetPath)
	if !strings.HasPrefix(absTarget, absClone+string(filepath.Separator)) && absTarget != absClone {
		return nil, fmt.Errorf("file path escapes repository root")
	}

	// Read original content for diff
	originalContent, err := os.ReadFile(targetPath) // #nosec // G304: targetPath is validated against path traversal
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Apply the edit
	var newContent []byte
	if req.FullYAML != "" {
		newContent = []byte(req.FullYAML)
	} else if req.FieldPath != "" {
		newContent, err = applyFieldPatch(originalContent, req.FieldPath, req.NewValue)
		if err != nil {
			return nil, fmt.Errorf("apply field patch: %w", err)
		}
	} else {
		return nil, fmt.Errorf("either field_path or full_yaml must be provided")
	}

	// Write the modified file
	if err := os.WriteFile(targetPath, newContent, 0600); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	// Generate diff
	diff := generateDiff(cleanPath, originalContent, newContent)

	// Determine target branch
	branch := repo.Branch
	if req.Branch != "" {
		branch = req.Branch
	}

	// Stage the change
	cmd = exec.CommandContext(ctx, "git", "-C", tmpDir, "add", cleanPath) // #nosec // G204: git add with validated path
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git add: %s: %w", string(out), err)
	}

	// Commit
	commitMsg := req.CommitMsg
	if commitMsg == "" {
		commitMsg = fmt.Sprintf("gitops: update %s in %s", req.FieldPath, req.FilePath)
	}
	cmd = exec.CommandContext(ctx, "git", "-C", tmpDir, "commit", "-m", commitMsg) // #nosec // G204: git commit in controlled temp dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git commit: %s: %w", string(out), err)
	}

	// Try to push directly
	pushCmd := exec.CommandContext(ctx, "git", "-C", tmpDir, "push", "origin", branch) // #nosec // G204: git push in controlled temp dir
	pushCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	_, pushErr := pushCmd.CombinedOutput()

	result := &CommitResult{
		Branch: branch,
		Diff:   diff,
	}

	if pushErr == nil {
		// Direct push succeeded
		sha := getHeadSHA(ctx, tmpDir)
		result.CommitSHA = sha
		return result, nil
	}

	// Push rejected — create a feature branch and try again
	featureBranch := fmt.Sprintf("pepa/edit-%d", time.Now().Unix())
	cmd = exec.CommandContext(ctx, "git", "-C", tmpDir, "branch", "-m", featureBranch) // #nosec // G204: git branch in controlled temp dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git branch rename: %s: %w", string(out), err)
	}

	pushCmd = exec.CommandContext(ctx, "git", "-C", tmpDir, "push", "origin", featureBranch) // #nosec // G204: git push in controlled temp dir
	pushCmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	pushOut, pushErr := pushCmd.CombinedOutput()
	if pushErr != nil {
		return nil, fmt.Errorf("git push feature branch: %s: %w", maskGitSecrets(string(pushOut), token), pushErr)
	}

	sha := getHeadSHA(ctx, tmpDir)
	result.CommitSHA = sha
	result.Branch = featureBranch
	result.MRNeeded = true
	result.MRURL = fmt.Sprintf("(push to %s rejected, created branch %s — create MR manually)", repo.Branch, featureBranch)

	return result, nil
}

// PreviewDiff returns a unified diff without committing.
func (e *Editor) PreviewDiff(ctx context.Context, repo *Repo, req *EditRequest) (string, error) {
	repoURL := repo.RepoURL
	token, _ := repo.Config["token"]
	if token != "" {
		repoURL = injectToken(repoURL, token)
	}

	tmpDir, err := os.MkdirTemp("", "pepa-gitops-preview-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cloneArgs := []string{"clone", "--depth", "1", "--branch", repo.Branch, "--single-branch", "--no-tags", repoURL, tmpDir}
	cmd := exec.CommandContext(ctx, "git", cloneArgs...) // #nosec // G204: git clone with validated args
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone: %s: %w", maskGitSecrets(string(out), token), err)
	}

	// Clean and resolve file path
	cleanPath := filepath.Clean(req.FilePath)
	if strings.HasPrefix(cleanPath, "..") || strings.HasPrefix(cleanPath, "/") {
		return "", fmt.Errorf("invalid file path: must be relative within repo")
	}
	targetPath := filepath.Join(tmpDir, cleanPath)
	absClone, _ := filepath.Abs(tmpDir)
	absTarget, _ := filepath.Abs(targetPath)
	if !strings.HasPrefix(absTarget, absClone+string(filepath.Separator)) && absTarget != absClone {
		return "", fmt.Errorf("file path escapes repository root")
	}

	originalContent, err := os.ReadFile(targetPath) // #nosec // G304: targetPath is validated against path traversal
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	var newContent []byte
	if req.FullYAML != "" {
		newContent = []byte(req.FullYAML)
	} else if req.FieldPath != "" {
		newContent, err = applyFieldPatch(originalContent, req.FieldPath, req.NewValue)
		if err != nil {
			return "", fmt.Errorf("apply field patch: %w", err)
		}
	} else {
		return "", fmt.Errorf("either field_path or full_yaml must be provided")
	}

	return generateDiff(cleanPath, originalContent, newContent), nil
}

// applyFieldPatch applies a dot-separated field path update to a YAML document.
func applyFieldPatch(original []byte, fieldPath string, newValue interface{}) ([]byte, error) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal(original, &doc); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	if doc == nil {
		doc = make(map[string]interface{})
	}

	// Navigate the dot-separated path
	parts := strings.Split(fieldPath, ".")
	if err := setNestedValue(doc, parts, newValue); err != nil {
		return nil, err
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal yaml: %w", err)
	}
	return out, nil
}

// setNestedValue sets a value at a nested path in a map.
func setNestedValue(m map[string]interface{}, path []string, value interface{}) error {
	if len(path) == 0 {
		return fmt.Errorf("empty field path")
	}

	current := m
	for i := 0; i < len(path)-1; i++ {
		key := path[i]
		next, ok := current[key]
		if !ok {
			// Create intermediate maps
			newMap := make(map[string]interface{})
			current[key] = newMap
			current = newMap
			continue
		}
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			// Overwrite non-map intermediate
			newMap := make(map[string]interface{})
			current[key] = newMap
			current = newMap
			continue
		}
		current = nextMap
	}

	current[path[len(path)-1]] = value
	return nil
}

// generateDiff produces a simple unified diff between two byte slices.
func generateDiff(filePath string, original, modified []byte) string {
	origLines := strings.Split(string(original), "\n")
	modLines := strings.Split(string(modified), "\n")

	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- a/%s\n", filePath))
	b.WriteString(fmt.Sprintf("+++ b/%s\n", filePath))

	// Simple line-by-line diff (good enough for preview)
	maxLen := len(origLines)
	if len(modLines) > maxLen {
		maxLen = len(modLines)
	}

	inHunk := false
	for i := 0; i < maxLen; i++ {
		var origLine, modLine string
		if i < len(origLines) {
			origLine = origLines[i]
		}
		if i < len(modLines) {
			modLine = modLines[i]
		}

		if origLine != modLine {
			if !inHunk {
				start := i - 2
				if start < 0 {
					start = 0
				}
				b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", start+1, len(origLines), start+1, len(modLines)))
				// Print context before
				for j := start; j < i; j++ {
					if j < len(origLines) {
						b.WriteString(fmt.Sprintf(" %s\n", origLines[j]))
					}
				}
				inHunk = true
			}
			if i < len(origLines) {
				b.WriteString(fmt.Sprintf("-%s\n", origLine))
			}
			if i < len(modLines) {
				b.WriteString(fmt.Sprintf("+%s\n", modLine))
			}
		} else if inHunk {
			b.WriteString(fmt.Sprintf(" %s\n", origLine))
			// End hunk after 3 lines of context
			contextEnd := true
			for j := i + 1; j < i+4 && j < maxLen; j++ {
				var ol, ml string
				if j < len(origLines) {
					ol = origLines[j]
				}
				if j < len(modLines) {
					ml = modLines[j]
				}
				if ol != ml {
					contextEnd = false
					break
				}
			}
			if contextEnd {
				inHunk = false
			}
		}
	}

	return b.String()
}

// getHeadSHA returns the current HEAD commit SHA.
func getHeadSHA(ctx context.Context, dir string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD") // #nosec // G204: git rev-parse in controlled dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
