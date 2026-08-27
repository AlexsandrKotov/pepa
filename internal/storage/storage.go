// Package storage provides a unified interface for plugin binary storage.
// Two implementations are available:
//   - LocalStorage: filesystem-backed (default, uses Docker volumes)
//   - S3Client: S3-compatible (optional, for multi-node deployments)
package storage

import (
	"context"
	"io"
	"strings"
	"time"
)

// Storage is the abstraction for plugin binary storage.
// Both LocalStorage and S3Client implement this interface.
type Storage interface {
	// ListPlugins returns metadata for all stored plugin binaries.
	ListPlugins(ctx context.Context) ([]ObjectMeta, error)

	// UploadPlugin stores a plugin binary.
	UploadPlugin(ctx context.Context, name, version string, reader io.Reader, size int64) error

	// DownloadPlugin retrieves a plugin binary. Caller must close the ReadCloser.
	DownloadPlugin(ctx context.Context, name, version string) (io.ReadCloser, error)

	// DeletePlugin removes a plugin binary.
	DeletePlugin(ctx context.Context, name, version string) error

	// Stat returns metadata about a specific plugin binary.
	StatPlugin(ctx context.Context, name, version string) (*ObjectMeta, error)

	// IsAvailable returns true if the storage backend is operational.
	IsAvailable() bool
}

// ObjectMeta holds metadata about a stored object.
type ObjectMeta struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"content_type"`
	LastModified time.Time `json:"last_modified"`
}

// PluginKey builds a standard storage key for a plugin binary.
func PluginKey(name, version string) string {
	return name + "/" + version + "/" + name
}

// ParsePluginKey extracts name and version from a plugin storage key.
func ParsePluginKey(key string) (name, version string) {
	parts := strings.SplitN(key, "/", 3)
	if len(parts) < 3 {
		return key, ""
	}
	return parts[0], parts[1]
}
