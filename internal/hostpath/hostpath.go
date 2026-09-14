// Package hostpath provides unified host filesystem path resolution for PEPA.
//
// When PEPA runs inside Docker the host data directory (HOST_DATA_DIR) is
// bind-mounted into the container at the same absolute path. User-supplied
// paths like /Users/alice/projects/ansible need to be translated so that
// they resolve to the mounted location.
//
// When PEPA runs natively (HOST_DATA_DIR is empty) paths are returned as-is.
package hostpath

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// hostHomePrefixes are common absolute-path prefixes found on host machines.
var hostHomePrefixes = []string{"/Users/", "/home/"}

// Resolve translates a user-supplied host path to a path accessible inside
// the current process. The resolution order is:
//
//  1. hostDataDir empty → return path as-is (native mode, no translation).
//  2. Path exists directly → return as-is (already inside container).
//  3. Path starts with a known host prefix (/Users/, /home/) → translate
//     by stripping the prefix + username and prepending hostDataDir.
//  4. Path is already within hostDataDir → return as-is.
//  5. Otherwise → error (path is outside the allowed host data directory).
func Resolve(p string, hostDataDir string) (string, error) {
	absPath, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Native mode — no translation needed.
	if hostDataDir == "" {
		if info, statErr := os.Stat(absPath); statErr == nil && info.IsDir() {
			return absPath, nil
		}
		return "", fmt.Errorf("path does not exist or is not a directory: %s", p)
	}

	// 1. Direct access (path already inside container or native mode).
	if info, statErr := os.Stat(absPath); statErr == nil && info.IsDir() {
		return absPath, nil
	}

	// 2. Translate host home path → HOST_DATA_DIR/...
	//    /Users/alice/projects/ansible → <hostDataDir>/projects/ansible
	//    /home/alice/projects/ansible  → <hostDataDir>/projects/ansible
	if filepath.IsAbs(p) {
		for _, prefix := range hostHomePrefixes {
			if strings.HasPrefix(p, prefix) {
				rest := strings.TrimPrefix(p, prefix)
				if idx := strings.Index(rest, "/"); idx >= 0 {
					rest = rest[idx+1:]
				} else {
					continue // no sub-path after username
				}
				containerPath := filepath.Join(hostDataDir, filepath.FromSlash(rest))
				if info, statErr := os.Stat(containerPath); statErr == nil && info.IsDir() {
					return containerPath, nil
				}
			}
		}
	}

	// 3. Path is already within hostDataDir.
	cleanBase := filepath.Clean(hostDataDir)
	cleanPath := filepath.Clean(absPath)
	if isSubPath(cleanPath, cleanBase) {
		return cleanPath, nil
	}

	return "", fmt.Errorf("path is outside the allowed host data directory (%s): %s", hostDataDir, p)
}

// Validate checks that a path is within the allowed hostDataDir.
// Returns nil if hostDataDir is empty (native mode) or if the path is contained.
func Validate(p string, hostDataDir string) error {
	if hostDataDir == "" {
		return nil
	}
	cleanBase := filepath.Clean(hostDataDir)
	cleanPath := filepath.Clean(p)
	if isSubPath(cleanPath, cleanBase) {
		return nil
	}
	return fmt.Errorf("path must be within the configured host data directory (%s)", hostDataDir)
}

// isSubPath reports whether child is equal to or a descendant of parent.
// Both paths must already be cleaned with filepath.Clean.
func isSubPath(child, parent string) bool {
	if child == parent {
		return true
	}
	return strings.HasPrefix(child, parent+string(filepath.Separator))
}

// DirEntry represents a single directory entry returned by ListDirectories.
type DirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ListDirectories returns the subdirectories of the given path, which must be
// within hostDataDir. If path is empty, hostDataDir itself is listed.
// The path parameter is resolved and validated to prevent directory traversal.
func ListDirectories(path string, hostDataDir string) ([]DirEntry, error) {
	if hostDataDir == "" {
		return nil, fmt.Errorf("HOST_DATA_DIR is not configured — filesystem browsing is disabled")
	}

	// Determine the directory to list.
	dir := filepath.Clean(hostDataDir)
	if path != "" {
		resolved, err := Resolve(path, hostDataDir)
		if err != nil {
			return nil, err
		}
		dir = resolved
	}

	// Security: double-check resolved directory is within hostDataDir.
	cleanBase := filepath.Clean(hostDataDir)
	cleanDir := filepath.Clean(dir)
	if !isSubPath(cleanDir, cleanBase) {
		return nil, fmt.Errorf("access denied: path is outside the allowed host data directory")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read directory %s: %w", dir, err)
	}

	var dirs []DirEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip hidden directories (e.g. .git).
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		dirs = append(dirs, DirEntry{
			Name: name,
			Path: filepath.Join(dir, name),
		})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Name < dirs[j].Name
	})
	return dirs, nil
}
