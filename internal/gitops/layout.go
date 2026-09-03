package gitops

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LayoutType represents the detected repository organization pattern.
type LayoutType string

const (
	LayoutMonorepo    LayoutType = "monorepo"
	LayoutBaseOverlay LayoutType = "base-overlay"
	LayoutTeamBased   LayoutType = "team-based"
	LayoutFlat        LayoutType = "flat"
)

// LayoutInfo describes the detected repository layout.
type LayoutInfo struct {
	Type         LayoutType `json:"type"`                    // monorepo | base-overlay | team-based | flat
	Environments []string   `json:"environments,omitempty"`  // detected environment names
	ClusterDirs  []string   `json:"cluster_dirs,omitempty"`  // detected cluster directory names
	BasePaths    []string   `json:"base_paths,omitempty"`    // relative paths to base directories
	OverlayPaths []string   `json:"overlay_paths,omitempty"` // relative paths to overlay directories
}

// knownEnvironmentNames are directory names commonly used for environments/clusters.
var knownEnvironmentNames = map[string]bool{
	"production":  true,
	"prod":        true,
	"staging":     true,
	"stage":       true,
	"dev":         true,
	"development": true,
	"test":        true,
	"testing":     true,
	"qa":          true,
	"uat":         true,
	"sandbox":     true,
	"demo":        true,
	"pre":         true,
	"preprod":     true,
}

// knownClusterDirNames are top-level directory names that contain per-cluster subdirs.
var knownClusterDirNames = map[string]bool{
	"clusters":       true,
	"environments":   true,
	"envs":           true,
	"env":            true,
	"infra":          true,
	"infrastructure": true,
}

// knownTeamDirNames are top-level directory names that contain per-team subdirs.
var knownTeamDirNames = map[string]bool{
	"teams":    true,
	"team":     true,
	"services": true,
	"apps":     true,
	"app":      true,
}

// DetectLayout analyzes the directory structure at root and returns layout information.
func DetectLayout(root string) LayoutInfo {
	info := LayoutInfo{Type: LayoutFlat}

	entries := listDir(root)
	if len(entries) == 0 {
		return info
	}

	// Score each layout type based on evidence
	monorepoScore := 0
	baseOverlayScore := 0
	teamBasedScore := 0

	var envNames []string
	var clusterDirs []string
	var basePaths []string
	var overlayPaths []string

	// Check for monorepo: clusters/<name>/ or environments/<name>/
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())

		if knownClusterDirNames[name] {
			monorepoScore += 10
			subEntries := listDir(filepath.Join(root, entry.Name()))
			for _, sub := range subEntries {
				if sub.IsDir() && !strings.HasPrefix(sub.Name(), ".") {
					clusterDirs = append(clusterDirs, sub.Name())
					envNames = append(envNames, sub.Name())
					// Check if this cluster dir has base/overlay structure
					clusterRoot := filepath.Join(root, entry.Name(), sub.Name())
					bp, op := findBaseOverlayInDir(clusterRoot, filepath.Join(entry.Name(), sub.Name()))
					basePaths = append(basePaths, bp...)
					overlayPaths = append(overlayPaths, op...)
				}
			}
		}
	}

	// Check for base-overlay: base/ + overlays/ at root or nested
	bp, op := findBaseOverlayInDir(root, ".")
	if len(bp) > 0 || len(op) > 0 {
		baseOverlayScore += 15
		basePaths = append(basePaths, bp...)
		overlayPaths = append(overlayPaths, op...)
	}

	// Also check one level deep for base/overlay patterns
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		subRoot := filepath.Join(root, entry.Name())
		sbp, sop := findBaseOverlayInDir(subRoot, entry.Name())
		if len(sbp) > 0 || len(sop) > 0 {
			baseOverlayScore += 5
			basePaths = append(basePaths, sbp...)
			overlayPaths = append(overlayPaths, sop...)
		}
	}

	// Check for team-based: teams/<name>/ with base+overlay inside
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		name := strings.ToLower(entry.Name())
		if knownTeamDirNames[name] {
			teamBasedScore += 10
			subEntries := listDir(filepath.Join(root, entry.Name()))
			for _, sub := range subEntries {
				if sub.IsDir() && !strings.HasPrefix(sub.Name(), ".") {
					teamRoot := filepath.Join(root, entry.Name(), sub.Name())
					tbp, top := findBaseOverlayInDir(teamRoot, filepath.Join(entry.Name(), sub.Name()))
					basePaths = append(basePaths, tbp...)
					overlayPaths = append(overlayPaths, top...)
					if len(top) > 0 {
						for _, o := range top {
							parts := strings.Split(o, "/")
							envName := parts[len(parts)-1]
							envNames = append(envNames, envName)
						}
					}
				}
			}
		}
	}

	// Check for kustomization.yaml files with overlay patterns at any depth
	kustOverlayEnvs := findKustomizeOverlays(root, "", 0, 3)
	if len(kustOverlayEnvs) > 0 {
		baseOverlayScore += len(kustOverlayEnvs) * 3
		for env, paths := range kustOverlayEnvs {
			found := false
			for _, e := range envNames {
				if e == env {
					found = true
					break
				}
			}
			if !found {
				envNames = append(envNames, env)
			}
			overlayPaths = append(overlayPaths, paths...)
		}
	}

	// Determine winning layout
	if teamBasedScore > monorepoScore && teamBasedScore > baseOverlayScore && teamBasedScore >= 10 {
		info.Type = LayoutTeamBased
	} else if monorepoScore > baseOverlayScore && monorepoScore >= 10 {
		info.Type = LayoutMonorepo
	} else if baseOverlayScore >= 10 {
		info.Type = LayoutBaseOverlay
	} else {
		info.Type = LayoutFlat
	}

	// Deduplicate and sort
	info.Environments = dedupSorted(envNames)
	info.ClusterDirs = dedupSorted(clusterDirs)
	info.BasePaths = dedupSorted(basePaths)
	info.OverlayPaths = dedupSorted(overlayPaths)

	return info
}

// findBaseOverlayInDir checks if a directory has base/ + overlays/ structure.
// Returns base paths and overlay paths found.
func findBaseOverlayInDir(root, relPrefix string) ([]string, []string) {
	var basePaths, overlayPaths []string

	entries := listDir(root)
	hasBase := false
	hasOverlays := false

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if name == "base" {
			hasBase = true
			p := filepath.Join(relPrefix, entry.Name())
			if relPrefix == "." {
				p = "base"
			}
			basePaths = append(basePaths, p)
		}
		if name == "overlays" || name == "overlay" {
			hasOverlays = true
			overlayDir := filepath.Join(root, entry.Name())
			overlayEntries := listDir(overlayDir)
			for _, oe := range overlayEntries {
				if oe.IsDir() && !strings.HasPrefix(oe.Name(), ".") {
					op := filepath.Join(relPrefix, entry.Name(), oe.Name())
					if relPrefix == "." {
						op = filepath.Join(entry.Name(), oe.Name())
					}
					overlayPaths = append(overlayPaths, op)
				}
			}
		}
	}

	// Only return results if both base and overlays exist (strong signal)
	if hasBase && hasOverlays {
		return basePaths, overlayPaths
	}
	return nil, nil
}

// findKustomizeOverlays walks up to maxDepth levels looking for kustomization.yaml
// files that reference external bases (indicating overlay pattern).
// Returns map of environment name -> overlay paths.
func findKustomizeOverlays(root, relPrefix string, depth, maxDepth int) map[string][]string {
	if depth > maxDepth {
		return nil
	}

	result := make(map[string][]string)
	entries := listDir(root)

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") ||
			entry.Name() == "node_modules" || entry.Name() == "vendor" {
			continue
		}

		subDir := filepath.Join(root, entry.Name())
		subRel := entry.Name()
		if relPrefix != "" && relPrefix != "." {
			subRel = filepath.Join(relPrefix, entry.Name())
		}

		// Check for kustomization.yaml in this directory
		kustPath := findKustomizationFile(subDir)
		if kustPath != "" {
			data, err := os.ReadFile(kustPath) // #nosec // G304: kustPath is from a controlled directory listing
			if err == nil {
				kInfo := ParseKustomization(data, subRel)
				if kInfo != nil && kInfo.IsOverlay {
					// The directory name is likely the environment/cluster name
					envName := strings.ToLower(entry.Name())
					if knownEnvironmentNames[envName] || isEnvLike(entry.Name()) {
						result[envName] = append(result[envName], subRel)
					}
				}
			}
		}

		// Recurse
		subResult := findKustomizeOverlays(subDir, subRel, depth+1, maxDepth)
		for k, v := range subResult {
			result[k] = append(result[k], v...)
		}
	}

	return result
}

// findKustomizationFile looks for kustomization.yaml or kustomization.yml in a directory.
func findKustomizationFile(dir string) string {
	for _, name := range []string{"kustomization.yaml", "kustomization.yml"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// isEnvLike checks if a directory name looks like an environment name.
func isEnvLike(name string) bool {
	lower := strings.ToLower(name)
	if knownEnvironmentNames[lower] {
		return true
	}
	// Check for patterns like "us-east-1", "eu-west-1", "cluster-1"
	if strings.Contains(lower, "cluster") || strings.Contains(lower, "region") {
		return true
	}
	return false
}

// listDir returns directory entries, or nil on error.
func listDir(dir string) []os.DirEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	return entries
}

// dedupSorted removes duplicates and sorts a string slice.
func dedupSorted(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != "" && !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	sort.Strings(result)
	return result
}
