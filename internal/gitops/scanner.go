package gitops

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Scanner clones a Git repository and discovers GitOps manifest files.
type Scanner struct {
	// CacheDir is the base directory for cached bare clones.
	// If empty, a temp directory is used each time.
	CacheDir string
}

// NewScanner creates a new Scanner.
func NewScanner(cacheDir string) *Scanner {
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "pepa-gitops-cache")
	}
	return &Scanner{CacheDir: cacheDir}
}

// Scan clones the repository and discovers all GitOps manifest files.
func (s *Scanner) Scan(ctx context.Context, repo *Repo) (*ScanResult, error) {
	// Build authenticated URL if token is provided
	repoURL := repo.RepoURL
	token, _ := repo.Config["token"]
	if token != "" {
		repoURL = injectToken(repoURL, token)
	}

	// Clone into a temp directory (shallow clone for speed)
	tmpDir, err := os.MkdirTemp("", "pepa-gitops-scan-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cloneArgs := []string{
		"clone", "--depth", "1",
		"--branch", repo.Branch,
		"--single-branch",
		"--no-tags",
		repoURL,
		tmpDir,
	}

	cmd := exec.CommandContext(ctx, "git", cloneArgs...) //nolint:gosec // G204: git clone with validated args
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git clone: %s: %w", maskGitSecrets(string(out), token), err)
	}

	// Determine the scan root within the cloned repo
	scanRoot := tmpDir
	if repo.Path != "" && repo.Path != "." {
		candidate := filepath.Join(tmpDir, repo.Path)
		// Prevent path traversal: ensure resolved path is inside the clone
		absClone, _ := filepath.Abs(tmpDir)
		absCandidate, _ := filepath.Abs(candidate)
		if strings.HasPrefix(absCandidate, absClone+string(filepath.Separator)) || absCandidate == absClone {
			scanRoot = candidate
		}
	}

	// Walk the directory tree and discover manifest files
	result := &ScanResult{}
	fileCount := 0

	// Detect repository layout before scanning
	layout := DetectLayout(scanRoot)
	result.Layout = layout

	err = filepath.Walk(scanRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			// Skip hidden directories and common non-manifest dirs
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		fileCount++

		relPath, _ := filepath.Rel(tmpDir, path)
		if relPath == "" {
			relPath = path
		}

		// Detect cluster from file path
		cluster := detectClusterFromPath(relPath)

		// Read and classify the file
		data, err := os.ReadFile(path) //nolint:gosec // G122,G304: path comes from filepath.Walk within a controlled temp clone
		if err != nil {
			log.Printf("gitops scanner: skip %s: %v", relPath, err)
			return nil
		}

		resources := classifyManifest(data, relPath, repo.EngineType, cluster)
		// Enhance resources with layout-detected environment and hierarchy
		for i := range resources {
			if resources[i].Environment == "" && cluster != "" {
				resources[i].Environment = cluster
			}
			// Detect sub-cluster hierarchy from path
			env, subCluster := detectClusterHierarchy(relPath)
			if env != "" && resources[i].Environment == "" {
				resources[i].Environment = env
			}
			// Always refine sub-cluster from path (overrides parser-level assignment)
			if subCluster != "" {
				resources[i].Cluster = subCluster
			} else if resources[i].Cluster == "" && resources[i].Environment != "" {
				// If no sub-cluster detected, use environment as cluster name
				resources[i].Cluster = resources[i].Environment
			}
		}
		result.Resources = append(result.Resources, resources...)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk repo: %w", err)
	}
	result.FileCount = fileCount

	// Build hierarchical tree structure
	result.Tree = buildResourceTree(result.Resources)

	// Build cluster hierarchy info
	result.Clusters = buildClusterInfo(result.Resources)

	// Detect engine from discovered resources
	if repo.EngineType == "auto" {
		result.Engine = detectEngine(result.Resources)
	} else {
		result.Engine = repo.EngineType
	}

	return result, nil
}

// classifyManifest reads a YAML file and returns any GitOps resources found.
// A single file may contain multiple YAML documents (separated by ---).
func classifyManifest(data []byte, relPath, engineHint, cluster string) []Resource {
	var resources []Resource

	// Split multi-document YAML
	docs := splitYAMLDocuments(data)
	for _, doc := range docs {
		rs := parseResource(doc, relPath, engineHint)
		if rs != nil {
			// Set cluster if detected from path and not already set
			for i := range rs {
				if rs[i].Cluster == "" && cluster != "" {
					rs[i].Cluster = cluster
				}
			}
			resources = append(resources, rs...)
		}
	}

	return resources
}

// detectClusterFromPath extracts cluster/environment name from file path.
// Uses layout-aware detection for common patterns:
//   - clusters/production/... -> "production"
//   - overlays/staging/... -> "staging"
//   - environments/dev/... -> "dev"
//   - teams/backend/overlays/prod/... -> "prod"
//   - production/... -> "production"
//   - staging/us-east-1/... -> env="staging", cluster="us-east-1"
func detectClusterFromPath(relPath string) string {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	if len(parts) < 2 {
		return ""
	}

	// Check for common cluster/environment directory patterns
	envDirs := []string{"clusters", "environments", "envs", "env", "overlays", "overlay"}
	for i, part := range parts {
		for _, dir := range envDirs {
			if strings.EqualFold(part, dir) && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}

	// Check for team-based: teams/<team>/overlays/<env>/...
	for i, part := range parts {
		if strings.EqualFold(part, "teams") && i+2 < len(parts) {
			// Look for overlays subdirectory within team
			for j := i + 2; j < len(parts)-1; j++ {
				if strings.EqualFold(parts[j], "overlays") || strings.EqualFold(parts[j], "overlay") {
					if j+1 < len(parts) {
						return parts[j+1]
					}
				}
			}
		}
	}

	// Check if first directory looks like a cluster name (common names)
	clusterNames := []string{"production", "prod", "staging", "stage", "dev", "development", "test", "testing", "qa", "uat", "sandbox", "demo"}
	firstDir := strings.ToLower(parts[0])
	for _, name := range clusterNames {
		if firstDir == name {
			return parts[0]
		}
	}

	return ""
}

// detectClusterHierarchy extracts environment and sub-cluster from path.
// Returns (environment, subCluster) where subCluster may be empty.
// Examples:
//   - overlays/staging/us-east-1/app.yaml -> ("staging", "us-east-1")
//   - clusters/production/app.yaml -> ("production", "")
//   - staging/app.yaml -> ("staging", "")
func detectClusterHierarchy(relPath string) (env, subCluster string) {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	if len(parts) < 2 {
		return "", ""
	}

	// Check for common cluster/environment directory patterns
	envDirs := []string{"clusters", "environments", "envs", "env", "overlays", "overlay"}
	for i, part := range parts {
		for _, dir := range envDirs {
			if strings.EqualFold(part, dir) && i+1 < len(parts) {
				env = parts[i+1]
				// Check if there's a sub-cluster level
				if i+2 < len(parts) {
					subDir := parts[i+2]
					// If it's not a file and looks like a cluster name
					if !strings.HasSuffix(subDir, ".yaml") && !strings.HasSuffix(subDir, ".yml") {
						subCluster = subDir
					}
				}
				return env, subCluster
			}
		}
	}

	// Check if first directory looks like an environment name
	clusterNames := []string{"production", "prod", "staging", "stage", "dev", "development", "test", "testing", "qa", "uat", "sandbox", "demo"}
	firstDir := strings.ToLower(parts[0])
	for _, name := range clusterNames {
		if firstDir == name {
			env = parts[0]
			// Check for sub-cluster
			if len(parts) > 2 && !strings.HasSuffix(parts[1], ".yaml") && !strings.HasSuffix(parts[1], ".yml") {
				subCluster = parts[1]
			}
			return env, subCluster
		}
	}

	return "", ""
}

// splitYAMLDocuments splits a YAML file into individual documents.
func splitYAMLDocuments(data []byte) [][]byte {
	var docs [][]byte
	parts := strings.Split(string(data), "\n---")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" || trimmed == "---" {
			continue
		}
		// Re-add the --- prefix for yaml.Unmarshal compatibility
		if !strings.HasPrefix(trimmed, "---") {
			trimmed = "---\n" + trimmed
		}
		docs = append(docs, []byte(trimmed))
	}
	return docs
}

// detectEngine infers the GitOps engine from discovered resources.
func detectEngine(resources []Resource) string {
	fluxCount, argoCount := 0, 0
	for _, r := range resources {
		switch r.Kind {
		case "HelmRelease", "Kustomization":
			if strings.HasPrefix(r.APIVersion, "fluxcd.io") || strings.Contains(r.APIVersion, "flux") {
				fluxCount++
			}
		case "Application":
			if strings.Contains(r.APIVersion, "argoproj.io") {
				argoCount++
			}
		}
	}
	if argoCount > fluxCount {
		return "argocd"
	}
	if fluxCount > 0 {
		return "fluxcd"
	}
	return "unknown"
}

// injectToken embeds a token into an HTTPS or HTTP git URL for authentication.
func injectToken(repoURL, token string) string {
	if strings.HasPrefix(repoURL, "https://") {
		// https://token@gitlab.com/org/repo.git
		return strings.Replace(repoURL, "https://", "https://oauth2:"+token+"@", 1)
	}
	if strings.HasPrefix(repoURL, "http://") {
		// http://token@gitea:3000/org/repo.git
		return strings.Replace(repoURL, "http://", "http://oauth2:"+token+"@", 1)
	}
	return repoURL
}

// maskGitSecrets strips the repository token from git command output so
// credentials never leak into error messages, logs or API responses.
func maskGitSecrets(output, token string) string {
	if token == "" {
		return output
	}
	out := strings.ReplaceAll(output, "oauth2:"+token, "oauth2:***")
	return strings.ReplaceAll(out, token, "***")
}

// branchNameRe allows only safe git branch name characters; the leading
// character must be alphanumeric, which also blocks "-" option injection.
var branchNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,254}$`)

// ValidBranchName validates a git branch name supplied by API clients.
func ValidBranchName(name string) bool {
	if !branchNameRe.MatchString(name) {
		return false
	}
	if strings.Contains(name, "..") || strings.Contains(name, "//") ||
		strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".") ||
		strings.HasSuffix(name, ".lock") {
		return false
	}
	return true
}

// buildResourceTree creates a hierarchical tree from flat resources.
// Groups by environment -> cluster/subcluster -> directory -> files.
func buildResourceTree(resources []Resource) *FileNode {
	root := &FileNode{
		Name: "root",
		Path: ".",
		Type: "dir",
	}

	// Group resources by environment first
	envMap := make(map[string][]Resource)
	var noEnv []Resource

	for _, r := range resources {
		if r.Environment != "" {
			envMap[r.Environment] = append(envMap[r.Environment], r)
		} else {
			noEnv = append(noEnv, r)
		}
	}

	// Build tree for each environment (sorted for deterministic output)
	envNames := make([]string, 0, len(envMap))
	for env := range envMap {
		envNames = append(envNames, env)
	}
	sort.Strings(envNames)

	for _, env := range envNames {
		envResources := envMap[env]
		envNode := &FileNode{
			Name: env,
			Path: env,
			Type: "environment",
		}

		// Group by cluster/subcluster within environment
		clusterMap := make(map[string][]Resource)
		var noCluster []Resource

		for _, r := range envResources {
			if r.Cluster != "" {
				clusterMap[r.Cluster] = append(clusterMap[r.Cluster], r)
			} else {
				noCluster = append(noCluster, r)
			}
		}

		// Add cluster nodes (sorted for deterministic output)
		clusterNames := make([]string, 0, len(clusterMap))
		for cluster := range clusterMap {
			clusterNames = append(clusterNames, cluster)
		}
		sort.Strings(clusterNames)

		for _, cluster := range clusterNames {
			clusterResources := clusterMap[cluster]
			clusterNode := &FileNode{
				Name: cluster,
				Path: env + "/" + cluster,
				Type: "subcluster",
			}
			addResourcesToTree(clusterNode, clusterResources)
			envNode.Children = append(envNode.Children, clusterNode)
			envNode.Count += clusterNode.Count
		}

		// Add resources without cluster
		if len(noCluster) > 0 {
			addResourcesToTree(envNode, noCluster)
		}

		root.Children = append(root.Children, envNode)
		root.Count += envNode.Count
	}

	// Add resources without environment
	if len(noEnv) > 0 {
		addResourcesToTree(root, noEnv)
	}

	return root
}

// addResourcesToTree adds resources to a parent node, building proper nested
// directory hierarchies that mirror the actual repository folder structure.
func addResourcesToTree(parent *FileNode, resources []Resource) {
	// Compute relative paths from parent and group by first directory segment
	type pathEntry struct {
		relPath  string
		resource Resource
	}

	var entries []pathEntry
	for _, r := range resources {
		rel := stripPathSuffix(parent.Path, r.FilePath)
		entries = append(entries, pathEntry{relPath: rel, resource: r})
	}

	// Group by first path segment to build nested directories
	groups := make(map[string][]pathEntry)
	var directResources []Resource

	for _, entry := range entries {
		parts := strings.SplitN(entry.relPath, "/", 2)
		if len(parts) == 1 {
			// File is directly in this directory
			directResources = append(directResources, entry.resource)
		} else {
			groups[parts[0]] = append(groups[parts[0]], pathEntry{
				relPath:  parts[1],
				resource: entry.resource,
			})
		}
	}

	// Build child directory nodes and recurse (sorted for deterministic output)
	dirNames := make([]string, 0, len(groups))
	for dirName := range groups {
		dirNames = append(dirNames, dirName)
	}
	sort.Strings(dirNames)

	for _, dirName := range dirNames {
		groupEntries := groups[dirName]
		childPath := dirName
		if parent.Path != "" && parent.Path != "." {
			childPath = parent.Path + "/" + dirName
		}

		childNode := &FileNode{
			Name: dirName,
			Path: childPath,
			Type: "dir",
		}

		// Convert back to resources and recurse
		var childResources []Resource
		for _, e := range groupEntries {
			childResources = append(childResources, e.resource)
		}
		addResourcesToTree(childNode, childResources)

		parent.Children = append(parent.Children, childNode)
		parent.Count += childNode.Count
	}

	// Add resources directly in this directory
	if len(directResources) > 0 {
		parent.Resources = append(parent.Resources, directResources...)
		parent.Count += len(directResources)
	}
}

// stripPathSuffix removes the parent path segments from the resource path,
// returning the relative path after the parent. It first tries a prefix match
// (parent path is a prefix of resource path), then falls back to a suffix match
// for cases where the resource FilePath includes prefix directories that are
// not part of the parent node's logical path.
func stripPathSuffix(parentPath, resourcePath string) string {
	if parentPath == "" || parentPath == "." {
		return resourcePath
	}

	parentParts := strings.Split(parentPath, "/")
	resourceParts := strings.Split(resourcePath, "/")

	// Try PREFIX match first: parent path is a prefix of resource path.
	// e.g. parent="dev", resource="dev/kustomization.yaml" -> "kustomization.yaml"
	if len(resourceParts) > len(parentParts) {
		prefixMatch := true
		for i, pp := range parentParts {
			if resourceParts[i] != pp {
				prefixMatch = false
				break
			}
		}
		if prefixMatch {
			return strings.Join(resourceParts[len(parentParts):], "/")
		}
	}

	// Exact match: parent and resource are the same path
	if len(resourceParts) == len(parentParts) {
		exactMatch := true
		for i, pp := range parentParts {
			if resourceParts[i] != pp {
				exactMatch = false
				break
			}
		}
		if exactMatch {
			return filepath.Base(resourcePath)
		}
	}

	// Fallback: suffix match — find parent path segments anywhere in resource path.
	// This handles prefix dirs like "overlays/" that aren't part of the parent path.
	if len(resourceParts) >= len(parentParts) {
		for endPos := len(resourceParts) - 1; endPos >= len(parentParts); endPos-- {
			startPos := endPos - len(parentParts) + 1
			match := true
			for i, pp := range parentParts {
				if resourceParts[startPos+i] != pp {
					match = false
					break
				}
			}
			if match {
				remaining := resourceParts[endPos+1:]
				if len(remaining) == 0 {
					return filepath.Base(resourcePath)
				}
				return strings.Join(remaining, "/")
			}
		}
	}

	// Last resort: return just the filename
	return filepath.Base(resourcePath)
}

// buildClusterInfo extracts cluster hierarchy information from resources.
func buildClusterInfo(resources []Resource) []ClusterInfo {
	// Group by environment
	envMap := make(map[string][]Resource)
	for _, r := range resources {
		if r.Environment != "" {
			envMap[r.Environment] = append(envMap[r.Environment], r)
		}
	}

	var clusters []ClusterInfo
	for env, envResources := range envMap {
		cluster := ClusterInfo{
			Name:          env,
			Environment:   env,
			ResourceCount: len(envResources),
		}

		// Group by sub-cluster
		subClusterMap := make(map[string]int)
		for _, r := range envResources {
			if r.Cluster != "" {
				subClusterMap[r.Cluster]++
			}
		}

		for subCluster, count := range subClusterMap {
			cluster.SubClusters = append(cluster.SubClusters, SubClusterInfo{
				Name:          subCluster,
				ResourceCount: count,
			})
		}

		clusters = append(clusters, cluster)
	}

	return clusters
}
