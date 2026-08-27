package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pepa/pepa/internal/config"
)

func TestPluginKey(t *testing.T) {
	tests := []struct {
		name, version string
		want          string
	}{
		{"myplugin", "1.0.0", "myplugin/1.0.0/myplugin"},
		{"argocd", "v2.5.0", "argocd/v2.5.0/argocd"},
		{"slack", "0.1.0", "slack/0.1.0/slack"},
	}
	for _, tt := range tests {
		got := PluginKey(tt.name, tt.version)
		if got != tt.want {
			t.Errorf("PluginKey(%q, %q) = %q, want %q", tt.name, tt.version, got, tt.want)
		}
	}
}

func TestParsePluginKey(t *testing.T) {
	tests := []struct {
		key         string
		wantName    string
		wantVersion string
	}{
		{"myplugin/1.0.0/myplugin", "myplugin", "1.0.0"},
		{"argocd/v2.5.0/argocd", "argocd", "v2.5.0"},
		{"single", "single", ""},
		{"two/parts", "two/parts", ""},
	}
	for _, tt := range tests {
		name, version := ParsePluginKey(tt.key)
		if name != tt.wantName {
			t.Errorf("ParsePluginKey(%q) name = %q, want %q", tt.key, name, tt.wantName)
		}
		if version != tt.wantVersion {
			t.Errorf("ParsePluginKey(%q) version = %q, want %q", tt.key, version, tt.wantVersion)
		}
	}
}

func TestPluginKey_RoundTrip(t *testing.T) {
	name := "myplugin"
	version := "1.2.3"
	key := PluginKey(name, version)
	gotName, gotVersion := ParsePluginKey(key)
	if gotName != name {
		t.Errorf("roundtrip name: got %q, want %q", gotName, name)
	}
	if gotVersion != version {
		t.Errorf("roundtrip version: got %q, want %q", gotVersion, version)
	}
}

func TestBucketConstants(t *testing.T) {
	if BucketPlugins != "plugins" {
		t.Errorf("BucketPlugins = %q, want plugins", BucketPlugins)
	}
}

func TestNewS3Client_EmptyEndpoint(t *testing.T) {
	_, err := NewS3Client(config.S3Config{})
	if err == nil {
		t.Fatal("expected error for empty endpoint")
	}
}

func TestS3IsAvailable(t *testing.T) {
	var c *S3Client
	if c.IsAvailable() {
		t.Error("nil client should not be available")
	}

	c = &S3Client{}
	if c.IsAvailable() {
		t.Error("client with nil mc should not be available")
	}
}

// --- LocalStorage tests ---

func TestLocalStorage_UploadDownloadDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalStorage(dir)
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	ctx := context.Background()
	name := "test-plugin"
	version := "1.0.0"
	content := "fake-plugin-binary-content"

	// Upload
	if err := s.UploadPlugin(ctx, name, version, strings.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("UploadPlugin: %v", err)
	}

	// Verify file exists on disk
	expectedPath := filepath.Join(dir, name, version, name)
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected file at %s: %v", expectedPath, err)
	}

	// Stat
	meta, err := s.StatPlugin(ctx, name, version)
	if err != nil {
		t.Fatalf("StatPlugin: %v", err)
	}
	if meta.Size != int64(len(content)) {
		t.Errorf("StatPlugin size = %d, want %d", meta.Size, len(content))
	}

	// Download
	reader, err := s.DownloadPlugin(ctx, name, version)
	if err != nil {
		t.Fatalf("DownloadPlugin: %v", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read downloaded: %v", err)
	}
	if string(data) != content {
		t.Errorf("downloaded content = %q, want %q", string(data), content)
	}

	// List
	objects, err := s.ListPlugins(ctx)
	if err != nil {
		t.Fatalf("ListPlugins: %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("ListPlugins count = %d, want 1", len(objects))
	}
	if objects[0].Size != int64(len(content)) {
		t.Errorf("ListPlugins[0].Size = %d, want %d", objects[0].Size, len(content))
	}

	// Delete
	if err := s.DeletePlugin(ctx, name, version); err != nil {
		t.Fatalf("DeletePlugin: %v", err)
	}

	// Verify deleted
	if _, err := s.StatPlugin(ctx, name, version); err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestLocalStorage_IsAvailable(t *testing.T) {
	var s *LocalStorage
	if s.IsAvailable() {
		t.Error("nil LocalStorage should not be available")
	}

	s = &LocalStorage{}
	if s.IsAvailable() {
		t.Error("LocalStorage with empty rootDir should not be available")
	}

	dir := t.TempDir()
	s, _ = NewLocalStorage(dir)
	if !s.IsAvailable() {
		t.Error("valid LocalStorage should be available")
	}
}

func TestLocalStorage_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalStorage(dir)
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}

	ctx := context.Background()

	// Attempt path traversal
	maliciousNames := []struct {
		name, version string
	}{
		{"../etc", "passwd"},
		{"plugin", "../../etc"},
		{"..", ".."},
		{"plugin/../../etc", "1.0"},
	}

	for _, tc := range maliciousNames {
		err := s.UploadPlugin(ctx, tc.name, tc.version, strings.NewReader("x"), 1)
		if err == nil {
			t.Errorf("expected error for name=%q version=%q, got nil", tc.name, tc.version)
		}
	}
}

func TestLocalStorage_NotFound(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewLocalStorage(dir)
	ctx := context.Background()

	_, err := s.DownloadPlugin(ctx, "nonexistent", "0.0.0")
	if err == nil {
		t.Error("expected error for nonexistent plugin")
	}

	_, err = s.StatPlugin(ctx, "nonexistent", "0.0.0")
	if err == nil {
		t.Error("expected error for nonexistent plugin stat")
	}

	err = s.DeletePlugin(ctx, "nonexistent", "0.0.0")
	if err == nil {
		t.Error("expected error for nonexistent plugin delete")
	}
}
