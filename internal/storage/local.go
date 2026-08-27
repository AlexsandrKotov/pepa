package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// LocalStorage implements the Storage interface using the local filesystem.
// It stores plugin binaries in a directory structure: <rootDir>/<name>/<version>/<name>
// This is the default storage backend — no external dependencies required.
type LocalStorage struct {
	rootDir string
}

// NewLocalStorage creates a LocalStorage rooted at the given directory.
// The directory is created if it does not exist.
func NewLocalStorage(rootDir string) (*LocalStorage, error) {
	if rootDir == "" {
		rootDir = "/custom-plugins"
	}
	if err := os.MkdirAll(rootDir, 0o750); err != nil {
		return nil, fmt.Errorf("create storage dir %s: %w", rootDir, err)
	}
	log.Printf("[storage] local storage initialized at %s", rootDir)
	return &LocalStorage{rootDir: rootDir}, nil
}

// pluginPath returns the filesystem path for a plugin binary.
// It sanitizes name and version to prevent path traversal attacks.
func (s *LocalStorage) pluginPath(name, version string) (string, error) {
	// Reject empty segments
	if name == "" || version == "" {
		return "", fmt.Errorf("plugin name and version are required")
	}
	// Reject path separators and parent-directory traversal
	for _, segment := range []string{name, version} {
		if strings.ContainsAny(segment, `/\`) || strings.Contains(segment, "..") {
			return "", fmt.Errorf("invalid characters in plugin name or version: %q", segment)
		}
	}
	path := filepath.Join(s.rootDir, name, version, name)
	// Verify the resolved path stays within rootDir
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	absRoot, err := filepath.Abs(s.rootDir)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	if !strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal detected: %q escapes %q", name, s.rootDir)
	}
	return path, nil
}

// ListPlugins walks the storage directory and returns metadata for all plugin binaries.
func (s *LocalStorage) ListPlugins(_ context.Context) ([]ObjectMeta, error) {
	var objects []ObjectMeta

	err := filepath.Walk(s.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip directories — we only care about the actual binary files
		if info.IsDir() {
			return nil
		}
		// Build the key relative to rootDir: <name>/<version>/<name>
		rel, err := filepath.Rel(s.rootDir, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)

		objects = append(objects, ObjectMeta{
			Key:          key,
			Size:         info.Size(),
			ContentType:  "application/octet-stream",
			LastModified: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list plugins: %w", err)
	}
	return objects, nil
}

// UploadPlugin stores a plugin binary to the local filesystem.
func (s *LocalStorage) UploadPlugin(_ context.Context, name, version string, reader io.Reader, _ int64) error {
	dest, err := s.pluginPath(name, version)
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}

	f, err := os.Create(dest) //nolint:gosec // G304: dest is a validated plugin storage path
	if err != nil {
		return fmt.Errorf("create plugin file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("write plugin file: %w", err)
	}
	return nil
}

// DownloadPlugin opens a plugin binary for reading.
func (s *LocalStorage) DownloadPlugin(_ context.Context, name, version string) (io.ReadCloser, error) {
	path, err := s.pluginPath(name, version)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path) //nolint:gosec // G304: path is a validated plugin storage path
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("plugin %s/%s not found", name, version)
		}
		return nil, fmt.Errorf("open plugin file: %w", err)
	}
	return f, nil
}

// DeletePlugin removes a plugin binary and its parent directory if empty.
func (s *LocalStorage) DeletePlugin(_ context.Context, name, version string) error {
	path, err := s.pluginPath(name, version)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("plugin %s/%s not found", name, version)
		}
		return fmt.Errorf("delete plugin file: %w", err)
	}
	// Clean up empty parent directories
	_ = os.Remove(filepath.Dir(path))             // <root>/<name>/<version>
	_ = os.Remove(filepath.Join(s.rootDir, name)) // <root>/<name>
	return nil
}

// StatPlugin returns metadata about a specific plugin binary.
func (s *LocalStorage) StatPlugin(_ context.Context, name, version string) (*ObjectMeta, error) {
	path, err := s.pluginPath(name, version)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("plugin %s/%s not found", name, version)
		}
		return nil, fmt.Errorf("stat plugin file: %w", err)
	}
	return &ObjectMeta{
		Key:          PluginKey(name, version),
		Size:         info.Size(),
		ContentType:  "application/octet-stream",
		LastModified: info.ModTime(),
	}, nil
}

// IsAvailable always returns true for local storage (it's always available).
func (s *LocalStorage) IsAvailable() bool {
	return s != nil && s.rootDir != ""
}

// Ensure the interface is satisfied at compile time.
var _ Storage = (*LocalStorage)(nil)
var _ Storage = (*S3Client)(nil)
