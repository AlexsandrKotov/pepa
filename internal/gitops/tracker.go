package gitops

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DeployTracker monitors deployment status after a Git commit.
type DeployTracker struct {
	mu       sync.RWMutex
	watches  map[string]*watchEntry
	interval time.Duration
}

type watchEntry struct {
	repoID    string
	commitSHA string
	events    []DeployEvent
	done      chan struct{}
	closed    bool
}

// DeployEvent represents a deployment status change.
type DeployEvent struct {
	Phase          string    `json:"phase"`            // pending, ci_running, syncing, healthy, degraded, failed
	Message        string    `json:"message"`
	Timestamp      time.Time `json:"timestamp"`
	FluxCondition  string    `json:"flux_condition,omitempty"`
	FluxRevision   string    `json:"flux_revision,omitempty"`
	ArgoSyncStatus string    `json:"argo_sync_status,omitempty"`
	ArgoHealth     string    `json:"argo_health,omitempty"`
}

// NewDeployTracker creates a new tracker with the given polling interval.
func NewDeployTracker(interval time.Duration) *DeployTracker {
	if interval == 0 {
		interval = 10 * time.Second
	}
	return &DeployTracker{
		watches:  make(map[string]*watchEntry),
		interval: interval,
	}
}

// WatchCommit starts monitoring a commit and returns a channel of events.
func (t *DeployTracker) WatchCommit(ctx context.Context, repo *Repo, commitSHA string) (<-chan DeployEvent, error) {
	key := repo.ID.String() + ":" + commitSHA

	t.mu.Lock()
	if entry, ok := t.watches[key]; ok {
		// Already watching this commit, return existing channel
		ch := make(chan DeployEvent, len(entry.events)+10)
		for _, e := range entry.events {
			ch <- e
		}
		t.mu.Unlock()
		return ch, nil
	}

	entry := &watchEntry{
		repoID:    repo.ID.String(),
		commitSHA: commitSHA,
		events:    make([]DeployEvent, 0),
		done:      make(chan struct{}),
	}
	t.watches[key] = entry
	t.mu.Unlock()

	// Emit initial event
	entry.events = append(entry.events, DeployEvent{
		Phase:     "pending",
		Message:   fmt.Sprintf("Watching commit %s", shortSHA(commitSHA)),
		Timestamp: time.Now(),
	})

	// Start polling in background
	go t.pollLoop(ctx, repo, entry)

	// Create a subscriber channel
	ch := make(chan DeployEvent, 20)
	go func() {
		defer close(ch)
		idx := 0
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				t.mu.RLock()
				for idx < len(entry.events) {
					select {
					case ch <- entry.events[idx]:
						idx++
					default:
						// Channel full, skip for now
						t.mu.RUnlock()
						time.Sleep(100 * time.Millisecond)
						t.mu.RLock()
						goto recheck
					}
				}
				if entry.closed {
					t.mu.RUnlock()
					return
				}
				t.mu.RUnlock()
			case <-ctx.Done():
				return
			case <-entry.done:
				// Drain remaining events
				t.mu.RLock()
				for idx < len(entry.events) {
					ch <- entry.events[idx]
					idx++
				}
				t.mu.RUnlock()
				return
			}
		recheck:
		}
	}()

	return ch, nil
}

// StopWatch stops watching a commit.
func (t *DeployTracker) StopWatch(repoID, commitSHA string) {
	key := repoID + ":" + commitSHA
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry, ok := t.watches[key]; ok && !entry.closed {
		entry.closed = true
		close(entry.done)
		delete(t.watches, key)
	}
}

// pollLoop periodically checks deployment status.
func (t *DeployTracker) pollLoop(ctx context.Context, repo *Repo, entry *watchEntry) {
	defer func() {
		t.mu.Lock()
		if !entry.closed {
			entry.closed = true
		}
		t.mu.Unlock()
	}()

	maxPolls := 60 // 10 minutes at 10s interval
	for i := 0; i < maxPolls; i++ {
		select {
		case <-ctx.Done():
			return
		case <-entry.done:
			return
		case <-time.After(t.interval):
		}

		events := t.checkStatus(ctx, repo, entry.commitSHA)
		if len(events) > 0 {
			t.mu.Lock()
			entry.events = append(entry.events, events...)
			t.mu.Unlock()

			// Check if we reached a terminal state
			last := events[len(events)-1]
			if last.Phase == "healthy" || last.Phase == "failed" || last.Phase == "degraded" {
				return
			}
		}
	}

	// Timeout
	t.mu.Lock()
	entry.events = append(entry.events, DeployEvent{
		Phase:     "timeout",
		Message:   "Deployment tracking timed out after 10 minutes",
		Timestamp: time.Now(),
	})
	t.mu.Unlock()
}

// checkStatus polls various sources for deployment status.
func (t *DeployTracker) checkStatus(ctx context.Context, repo *Repo, commitSHA string) []DeployEvent {
	var events []DeployEvent

	// 1. Check CI pipeline status via git log (commit status)
	ciEvent := t.checkCIStatus(ctx, repo, commitSHA)
	if ciEvent != nil {
		events = append(events, *ciEvent)
	}

	// 2. Check FluxCD status if applicable
	if repo.EngineType == "fluxcd" || repo.EngineType == "auto" {
		fluxEvents := t.checkFluxStatus(ctx, repo, commitSHA)
		events = append(events, fluxEvents...)
	}

	// 3. Check ArgoCD status if applicable
	if repo.EngineType == "argocd" {
		argoEvents := t.checkArgoStatus(ctx, repo, commitSHA)
		events = append(events, argoEvents...)
	}

	return events
}

// checkCIStatus checks if CI pipeline has passed for the commit.
// Uses Git provider APIs (GitLab, GitHub, Gitea) when a token is available,
// falling back to git ls-remote for basic connectivity.
func (t *DeployTracker) checkCIStatus(ctx context.Context, repo *Repo, commitSHA string) *DeployEvent {
	token, hasToken := repo.Config["token"]

	// Try provider-specific API if token is available
	if hasToken && token != "" {
		if event := t.checkCIviaAPI(ctx, repo, commitSHA, token); event != nil {
			return event
		}
	}

	// Fallback: basic connectivity check via git ls-remote
	repoURL := repo.RepoURL
	if hasToken && token != "" {
		repoURL = injectToken(repoURL, token)
	}

	cmd := exec.CommandContext(ctx, "git", "ls-remote", repoURL, commitSHA) //nolint:gosec // #nosec // G204: git ls-remote with validated args
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Info("CI check failed", "sha", shortSHA(commitSHA), "error", err)
		return nil
	}

	if strings.Contains(string(out), commitSHA) {
		return &DeployEvent{
			Phase:     "ci_running",
			Message:   "Commit visible on remote, CI pipeline may be running",
			Timestamp: time.Now(),
		}
	}

	return nil
}

// checkCIviaAPI queries Git provider APIs for actual CI pipeline status.
func (t *DeployTracker) checkCIviaAPI(ctx context.Context, repo *Repo, commitSHA, token string) *DeployEvent {
	repoURL := repo.RepoURL

	// Detect provider from URL pattern
	switch {
	case strings.Contains(repoURL, "gitlab"):
		return t.checkGitLabCI(ctx, repoURL, commitSHA, token)
	case strings.Contains(repoURL, "github"):
		return t.checkGitHubCI(ctx, repoURL, commitSHA, token)
	case strings.Contains(repoURL, "gitea"):
		return t.checkGiteaCI(ctx, repoURL, commitSHA, token)
	}
	return nil
}

// checkGitLabCI queries GitLab API for pipeline status.
func (t *DeployTracker) checkGitLabCI(ctx context.Context, repoURL, commitSHA, token string) *DeployEvent {
	// Extract project path from URL: https://gitlab.com/org/repo.git -> org/repo
	projectPath := extractProjectPath(repoURL, "gitlab")
	if projectPath == "" {
		return nil
	}

	apiURL := fmt.Sprintf("https://%s/api/v4/projects/%s/pipelines?sha=%s&per_page=1",
		extractHost(repoURL), url.PathEscape(projectPath), commitSHA)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Info("GitLab CI check failed", "error", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil
	}

	var pipelines []struct {
		Status string `json:"status"`
		SHA    string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pipelines); err != nil || len(pipelines) == 0 {
		return nil
	}

	switch pipelines[0].Status {
	case "success":
		return &DeployEvent{Phase: "ci_running", Message: "GitLab CI pipeline passed", Timestamp: time.Now()}
	case "failed":
		return &DeployEvent{Phase: "failed", Message: "GitLab CI pipeline failed", Timestamp: time.Now()}
	case "running", "pending":
		return &DeployEvent{Phase: "ci_running", Message: fmt.Sprintf("GitLab CI pipeline %s", pipelines[0].Status), Timestamp: time.Now()}
	}
	return nil
}

// checkGitHubCI queries GitHub API for commit status.
func (t *DeployTracker) checkGitHubCI(ctx context.Context, repoURL, commitSHA, token string) *DeployEvent {
	ownerRepo := extractProjectPath(repoURL, "github")
	if ownerRepo == "" {
		return nil
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/commits/%s/status", ownerRepo, commitSHA)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Info("GitHub CI check failed", "error", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil
	}

	var status struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil
	}

	switch status.State {
	case "success":
		return &DeployEvent{Phase: "ci_running", Message: "GitHub CI checks passed", Timestamp: time.Now()}
	case "failure", "error":
		return &DeployEvent{Phase: "failed", Message: fmt.Sprintf("GitHub CI checks: %s", status.State), Timestamp: time.Now()}
	case "pending":
		return &DeployEvent{Phase: "ci_running", Message: "GitHub CI checks pending", Timestamp: time.Now()}
	}
	return nil
}

// checkGiteaCI queries Gitea API for commit status.
func (t *DeployTracker) checkGiteaCI(ctx context.Context, repoURL, commitSHA, token string) *DeployEvent {
	ownerRepo := extractProjectPath(repoURL, "gitea")
	if ownerRepo == "" {
		return nil
	}
	host := extractHost(repoURL)

	apiURL := fmt.Sprintf("https://%s/api/v1/repos/%s/commits/%s/status", host, ownerRepo, commitSHA)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Info("Gitea CI check failed", "error", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil
	}

	var status struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil
	}

	switch status.State {
	case "success":
		return &DeployEvent{Phase: "ci_running", Message: "Gitea CI checks passed", Timestamp: time.Now()}
	case "error", "failure":
		return &DeployEvent{Phase: "failed", Message: fmt.Sprintf("Gitea CI checks: %s", status.State), Timestamp: time.Now()}
	case "pending":
		return &DeployEvent{Phase: "ci_running", Message: "Gitea CI checks pending", Timestamp: time.Now()}
	}
	return nil
}

// extractProjectPath extracts "org/repo" from a git URL for a given provider.
func extractProjectPath(repoURL, provider string) string {
	// Remove .git suffix
	repoURL = strings.TrimSuffix(repoURL, ".git")
	// Find the provider host and extract the path after it
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(repoURL, prefix) {
			path := strings.TrimPrefix(repoURL, prefix)
			parts := strings.SplitN(path, "/", 2)
			if len(parts) == 2 {
				return parts[1]
			}
		}
	}
	return ""
}

// extractHost extracts the hostname from a git URL.
func extractHost(repoURL string) string {
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(repoURL, prefix) {
			path := strings.TrimPrefix(repoURL, prefix)
			parts := strings.SplitN(path, "/", 2)
			return parts[0]
		}
	}
	return ""
}

// checkFluxStatus checks FluxCD resource status via kubectl (if available).
func (t *DeployTracker) checkFluxStatus(ctx context.Context, repo *Repo, commitSHA string) []DeployEvent {
	var events []DeployEvent

	// Try to get FluxCD HelmRelease status via kubectl
	cmd := exec.CommandContext(ctx, "kubectl", "get", "helmreleases", "--all-namespaces", //nolint:gosec // #nosec // G204: kubectl with static args
		"-o", "json", "-l", fmt.Sprintf("meta.helm.sh/release-name"))
	out, err := cmd.Output()
	if err != nil {
		// kubectl not available or not configured - skip
		return nil
	}

	var result struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
					Reason string `json:"reason"`
				} `json:"conditions"`
				LastAppliedRevision string `json:"lastAppliedRevision"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.Unmarshal(out, &result); err != nil {
		return nil
	}

	for _, item := range result.Items {
		for _, cond := range item.Status.Conditions {
			if cond.Type == "Ready" {
				phase := "syncing"
				msg := fmt.Sprintf("FluxCD HelmRelease %s/%s: %s", item.Metadata.Namespace, item.Metadata.Name, cond.Reason)
				if cond.Status == "True" {
					phase = "healthy"
					msg = fmt.Sprintf("FluxCD HelmRelease %s/%s is healthy", item.Metadata.Namespace, item.Metadata.Name)
				} else if cond.Status == "False" {
					phase = "degraded"
					msg = fmt.Sprintf("FluxCD HelmRelease %s/%s is degraded: %s", item.Metadata.Namespace, item.Metadata.Name, cond.Reason)
				}
				events = append(events, DeployEvent{
					Phase:         phase,
					Message:       msg,
					Timestamp:     time.Now(),
					FluxCondition: cond.Reason,
					FluxRevision:  item.Status.LastAppliedRevision,
				})
			}
		}
	}

	return events
}

// checkArgoStatus checks ArgoCD Application status via its REST API.
func (t *DeployTracker) checkArgoStatus(ctx context.Context, repo *Repo, commitSHA string) []DeployEvent {
	// ArgoCD status checking requires the ArgoCD connection details
	// For now, return nil - this will be implemented when ArgoCD connection is configured
	return nil
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
