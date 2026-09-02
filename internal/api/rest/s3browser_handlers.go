package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/storage"
)

// registerS3BrowserRoutes adds S3 browser endpoints that allow users to
// browse, upload, and delete objects in their configured S3 connections.
func registerS3BrowserRoutes(r *gin.RouterGroup, deps Dependencies) {
	s3 := r.Group("/s3-browser")
	{
		s3.GET("/:connectionId/credential-status", s3CredentialStatus(deps))
		s3.GET("/:connectionId/buckets", s3ListBuckets(deps))
		s3.POST("/:connectionId/buckets/:bucket/create", s3CreateBucket(deps))
		s3.DELETE("/:connectionId/buckets/:bucket", s3DeleteBucket(deps))
		s3.GET("/:connectionId/buckets/:bucket/objects", s3ListObjects(deps))
		s3.POST("/:connectionId/buckets/:bucket/upload", s3UploadObject(deps))
		s3.POST("/:connectionId/buckets/:bucket/upload-multiple", s3UploadMultiple(deps))
		s3.GET("/:connectionId/buckets/:bucket/objects/*key", s3GetObject(deps))
		s3.DELETE("/:connectionId/buckets/:bucket/objects/*key", s3DeleteObject(deps))
	}
}

// s3ClientFromConnection builds a temporary S3 client. It first tries the
// requesting user's personal S3 credential (stored in user_credentials),
// falling back to the admin's connection credentials. Returns the client,
// the credential source ("user" or "admin"), and the active access key.
func s3ClientFromConnection(ctx context.Context, deps Dependencies, connID uuid.UUID, tenantID uuid.UUID, userID *uuid.UUID) (*storage.S3Client, string, string, error) {
	conn, err := deps.Repos.Connection.GetDecrypted(ctx, connID, tenantID)
	if err != nil {
		return nil, "", "", fmt.Errorf("load connection: %w", err)
	}
	if conn.Type != "storage" {
		return nil, "", "", fmt.Errorf("connection is not an S3 storage type")
	}

	endpoint, _ := conn.Config["endpoint"].(string)
	adminAccessKey, _ := conn.Config["access_key"].(string)
	adminSecretKey, _ := conn.Config["secret_key"].(string)
	useSSL, _ := conn.Config["use_ssl"].(bool)
	if !useSSL {
		if v, ok := conn.Config["use_ssl"].(string); ok && v == "true" {
			useSSL = true
		}
	}
	slog.Info("connection config: endpoint= type=", "arg1", endpoint, "type", conn.Type)

	if endpoint == "" {
		return nil, "", "", fmt.Errorf("S3 endpoint is not configured in this connection")
	}

	// Try user's personal S3 credential first
	if userID != nil && deps.DB != nil {
		token, accessKey, _, credErr := GetUserCredential(ctx, deps, *userID, "s3", endpoint)
		if credErr == nil && accessKey != "" && token != "" {
			client, err := storage.NewS3ClientFromCredentials(endpoint, accessKey, token, useSSL)
			if err == nil {
				return client, "user", accessKey, nil
			}
			slog.Info("user credential failed for user , falling back to admin", "id", *userID, "error", err)
		}
	}

	// Fall back to admin's connection credentials
	client, err := storage.NewS3ClientFromCredentials(endpoint, adminAccessKey, adminSecretKey, useSSL)
	if err != nil {
		return nil, "", "", err
	}
	return client, "admin", adminAccessKey, nil
}

// s3CredentialStatus returns which credentials are active for the current user
// on the given S3 connection (personal or admin fallback).
func s3CredentialStatus(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		connID, err := uuid.Parse(c.Param("connectionId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection id"})
			return
		}

		_, source, accessKey, err := s3ClientFromConnection(c.Request.Context(), deps, connID, tenantID, auth.GetUserID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Load connection to get the endpoint
		conn, err := deps.Repos.Connection.GetDecrypted(c.Request.Context(), connID, tenantID)
		endpoint := ""
		if err == nil {
			endpoint, _ = conn.Config["endpoint"].(string)
		}

		c.JSON(http.StatusOK, gin.H{
			"source":   source,
			"username": accessKey,
			"endpoint": endpoint,
		})
	}
}

// s3ListBuckets returns all buckets for the given S3 connection.
func s3ListBuckets(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		connID, err := uuid.Parse(c.Param("connectionId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection id"})
			return
		}

		client, _, _, err := s3ClientFromConnection(c.Request.Context(), deps, connID, tenantID, auth.GetUserID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		buckets, err := client.ListBuckets(c.Request.Context())
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"buckets": buckets,
			"total":   len(buckets),
		})
	}
}

// s3CreateBucket creates a new bucket.
func s3CreateBucket(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		connID, err := uuid.Parse(c.Param("connectionId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection id"})
			return
		}

		bucketName := c.Param("bucket")
		if bucketName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bucket name is required"})
			return
		}

		client, _, _, err := s3ClientFromConnection(c.Request.Context(), deps, connID, tenantID, auth.GetUserID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		if err := client.CreateBucketNamed(c.Request.Context(), bucketName); err != nil {
			logPluginActionAsync(deps, c, "s3", "create_bucket", "bucket", string(mustMarshal(map[string]string{"bucket": bucketName})), false, err.Error())
			respondInternalError(c, err)
			return
		}

		logPluginActionAsync(deps, c, "s3", "create_bucket", "bucket", string(mustMarshal(map[string]string{"bucket": bucketName})), true, "")
		c.JSON(http.StatusCreated, gin.H{
			"message": "bucket created",
			"bucket":  bucketName,
		})
	}
}

// s3DeleteBucket removes an empty bucket.
func s3DeleteBucket(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		connID, err := uuid.Parse(c.Param("connectionId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection id"})
			return
		}

		bucketName := c.Param("bucket")
		if bucketName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bucket name is required"})
			return
		}

		client, _, _, err := s3ClientFromConnection(c.Request.Context(), deps, connID, tenantID, auth.GetUserID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		if err := client.DeleteBucketNamed(c.Request.Context(), bucketName); err != nil {
			logPluginActionAsync(deps, c, "s3", "delete_bucket", "bucket", string(mustMarshal(map[string]string{"bucket": bucketName})), false, err.Error())
			respondInternalError(c, err)
			return
		}

		logPluginActionAsync(deps, c, "s3", "delete_bucket", "bucket", string(mustMarshal(map[string]string{"bucket": bucketName})), true, "")
		c.JSON(http.StatusOK, gin.H{
			"message": "bucket deleted",
			"bucket":  bucketName,
		})
	}
}

// s3ListObjects lists objects in a bucket at the current prefix level only.
// Uses non-recursive listing (S3 Delimiter "/") so only immediate children
// (files + subfolder names) are returned — no full bucket traversal.
func s3ListObjects(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		connID, err := uuid.Parse(c.Param("connectionId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection id"})
			return
		}

		bucketName := c.Param("bucket")
		prefix := c.Query("prefix")

		client, _, _, err := s3ClientFromConnection(c.Request.Context(), deps, connID, tenantID, auth.GetUserID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Non-recursive: only immediate children (files + subfolder names).
		// S3 uses Delimiter "/" which returns CommonPrefixes as virtual folders.
		allObjects, err := client.ListObjectsInBucket(c.Request.Context(), bucketName, prefix, false)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		type folderEntry struct {
			Name string `json:"name"`
		}
		var folders []folderEntry
		var topLevel []storage.ObjectMeta

		for _, obj := range allObjects {
			// minio-go returns common prefixes (virtual folders) as entries
			// whose key ends with "/".
			if strings.HasSuffix(obj.Key, "/") {
				folders = append(folders, folderEntry{Name: obj.Key})
				continue
			}
			topLevel = append(topLevel, obj)
		}

		// Log the keys returned for debugging
		if len(topLevel) > 0 {
			keys := make([]string, 0, len(topLevel))
			for _, obj := range topLevel {
				keys = append(keys, obj.Key)
			}
			slog.Info("LIST objects", "bucket", bucketName, "prefix", prefix, "count", len(topLevel), "keys", keys)
		}
		if len(folders) > 0 {
			names := make([]string, 0, len(folders))
			for _, f := range folders {
				names = append(names, f.Name)
			}
			slog.Info("LIST folders", "bucket", bucketName, "prefix", prefix, "count", len(folders), "names", names)
		}

		c.JSON(http.StatusOK, gin.H{
			"objects": topLevel,
			"folders": folders,
			"total":   len(topLevel),
			"prefix":  prefix,
		})
	}
}

// s3UploadObject uploads a file to the specified bucket.
func s3UploadObject(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		connID, err := uuid.Parse(c.Param("connectionId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection id"})
			return
		}

		bucketName := c.Param("bucket")

		client, _, _, err := s3ClientFromConnection(c.Request.Context(), deps, connID, tenantID, auth.GetUserID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Parse multipart form (max 100MB)
		if err := c.Request.ParseMultipartForm(100 << 20); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart form: " + err.Error()})
			return
		}

		key := c.Request.FormValue("key")
		if key == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "key (object path) is required"})
			return
		}

		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file field is required: " + err.Error()})
			return
		}
		defer func() { _ = file.Close() }()

		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		if err := client.UploadToBucket(c.Request.Context(), bucketName, key, file, header.Size, contentType); err != nil {
			logPluginActionAsync(deps, c, "s3", "upload_object", "object", string(mustMarshal(map[string]string{"bucket": bucketName, "key": key})), false, err.Error())
			respondInternalError(c, err)
			return
		}

		logPluginActionAsync(deps, c, "s3", "upload_object", "object", string(mustMarshal(map[string]string{"bucket": bucketName, "key": key, "size": fmt.Sprintf("%d", header.Size)})), true, "")
		c.JSON(http.StatusCreated, gin.H{
			"message": "object uploaded",
			"key":     key,
			"bucket":  bucketName,
			"size":    header.Size,
		})
	}
}

// s3UploadMultiple handles uploading multiple files at once, preserving
// directory structure when uploading folders. Each file can carry its own
// relative path via the "relative_path" form field (matching the file by
// index), or falls back to the prefix + filename.
func s3UploadMultiple(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		connID, err := uuid.Parse(c.Param("connectionId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection id"})
			return
		}

		bucketName := c.Param("bucket")

		client, _, _, err := s3ClientFromConnection(c.Request.Context(), deps, connID, tenantID, auth.GetUserID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// Parse multipart form (max 500MB for bulk uploads)
		if err := c.Request.ParseMultipartForm(500 << 20); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart form: " + err.Error()})
			return
		}

		prefix := c.Request.FormValue("prefix")
		files := c.Request.MultipartForm.File["files"]
		relativePaths := c.Request.Form["relative_path"]

		if len(files) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "files field is required"})
			return
		}

		type uploadResult struct {
			Key  string `json:"key"`
			Size int64  `json:"size"`
			OK   bool   `json:"ok"`
			Err  string `json:"error,omitempty"`
		}

		results := make([]uploadResult, 0, len(files))
		var failed int

		for i, fh := range files {
			// Determine the S3 key for this file
			var key string
			if i < len(relativePaths) && relativePaths[i] != "" {
				key = prefix + relativePaths[i]
			} else {
				key = prefix + fh.Filename
			}

			file, err := fh.Open()
			if err != nil {
				failed++
				results = append(results, uploadResult{Key: key, OK: false, Err: err.Error()})
				continue
			}

			contentType := fh.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}

			if err := client.UploadToBucket(c.Request.Context(), bucketName, key, file, fh.Size, contentType); err != nil {
				_ = file.Close()
				failed++
				results = append(results, uploadResult{Key: key, OK: false, Err: err.Error()})
				continue
			}
			_ = file.Close()
			results = append(results, uploadResult{Key: key, Size: fh.Size, OK: true})
		}

		status := http.StatusCreated
		if failed > 0 && failed == len(files) {
			status = http.StatusInternalServerError
		}

		c.JSON(status, gin.H{
			"message":  fmt.Sprintf("uploaded %d of %d file(s)", len(files)-failed, len(files)),
			"bucket":   bucketName,
			"total":    len(files),
			"uploaded": len(files) - failed,
			"failed":   failed,
			"results":  results,
		})
	}
}

// s3GetObject returns object metadata and a presigned download URL, or
// streams the object content directly if ?download=true.
func s3GetObject(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		connID, err := uuid.Parse(c.Param("connectionId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection id"})
			return
		}

		bucketName := c.Param("bucket")
		// The key is everything after /objects/ — Gin's *key includes the leading /
		key := strings.TrimPrefix(c.Param("key"), "/")
		if key == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "object key is required"})
			return
		}
		slog.Info("GET object", "bucket", bucketName, "key", key, "preview", c.Query("preview"), "download", c.Query("download"))

		client, _, _, err := s3ClientFromConnection(c.Request.Context(), deps, connID, tenantID, auth.GetUserID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		// If ?preview=true, stream the file inline for browser preview.
		// Text files are limited to 1 MB; images and PDFs are streamed fully.
		if c.Query("preview") == "true" {
			meta, err := client.StatObjectInBucket(c.Request.Context(), bucketName, key)
			if err != nil {
				slog.Info("StatObject error: bucket= key= err=", "name", bucketName, "arg2", key, "error", err)
				c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("cannot stat object: %v", err)})
				return
			}

			ct := meta.ContentType
			if ct == "" || ct == "application/octet-stream" {
				ct = detectContentType(key)
			}

			// Cap text previews at 1 MB to avoid huge payloads.
			if isTextContentType(ct) && meta.Size > 1<<20 {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{
					"error":    "file too large for text preview",
					"max_size": 1 << 20,
					"size":     meta.Size,
				})
				return
			}

			reader, err := client.DownloadFromBucket(c.Request.Context(), bucketName, key)
			if err != nil {
				slog.Info("Download error (preview): bucket= key= err=", "name", bucketName, "arg2", key, "error", err)
				c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("cannot download object: %v", err)})
				return
			}
			defer func() { _ = reader.Close() }()

			c.Header("Content-Type", ct)
			c.Header("Content-Disposition", "inline")
			// Neutralize active content in user-uploaded files (stored XSS):
			// sandbox disables scripts/forms/same-origin; nosniff stops MIME sniffing.
			c.Header("Content-Security-Policy", "sandbox")
			c.Header("X-Content-Type-Options", "nosniff")
			c.Status(http.StatusOK)
			if _, err := io.Copy(c.Writer, reader); err != nil {
				slog.Info("s3browser preview stream error", "error", err)
			}
			return
		}

		// If ?download=true, stream the file directly
		if c.Query("download") == "true" {
			reader, err := client.DownloadFromBucket(c.Request.Context(), bucketName, key)
			if err != nil {
				slog.Info("Download error: bucket= key= err=", "name", bucketName, "arg2", key, "error", err)
				c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("cannot download object: %v", err)})
				return
			}
			defer func() { _ = reader.Close() }()

			// Extract filename from key for Content-Disposition
			parts := strings.Split(key, "/")
			filename := parts[len(parts)-1]
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
			c.Header("Content-Type", "application/octet-stream")
			c.Status(http.StatusOK)
			if _, err := io.Copy(c.Writer, reader); err != nil {
				slog.Info("s3browser download stream error", "error", err)
			}
			return
		}

		// Otherwise return metadata + presigned URL
		meta, err := client.StatObjectInBucket(c.Request.Context(), bucketName, key)
		if err != nil {
			slog.Info("StatObject error (meta): bucket= key= err=", "name", bucketName, "arg2", key, "error", err)
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("cannot stat object: %v", err)})
			return
		}

		presignedURL, err := client.PresignedURL(c.Request.Context(), bucketName, key, 15*time.Minute)
		if err != nil {
			slog.Info("s3browser presigned URL error", "error", err)
			// Non-fatal — return metadata without URL
		}

		c.JSON(http.StatusOK, gin.H{
			"key":           meta.Key,
			"size":          meta.Size,
			"content_type":  meta.ContentType,
			"last_modified": meta.LastModified.Format("2006-01-02T15:04:05Z"),
			"presigned_url": presignedURL,
			"bucket":        bucketName,
		})
	}
}

// s3DeleteObject removes an object from the bucket.
func s3DeleteObject(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := auth.GetTenantID(c)
		connID, err := uuid.Parse(c.Param("connectionId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection id"})
			return
		}

		bucketName := c.Param("bucket")
		key := strings.TrimPrefix(c.Param("key"), "/")
		if key == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "object key is required"})
			return
		}

		client, _, _, err := s3ClientFromConnection(c.Request.Context(), deps, connID, tenantID, auth.GetUserID(c))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		if err := client.DeleteFromBucket(c.Request.Context(), bucketName, key); err != nil {
			logPluginActionAsync(deps, c, "s3", "delete_object", "object", string(mustMarshal(map[string]string{"bucket": bucketName, "key": key})), false, err.Error())
			respondInternalError(c, err)
			return
		}

		logPluginActionAsync(deps, c, "s3", "delete_object", "object", string(mustMarshal(map[string]string{"bucket": bucketName, "key": key})), true, "")
		c.JSON(http.StatusOK, gin.H{
			"message": "object deleted",
			"key":     key,
			"bucket":  bucketName,
		})
	}
}

// ── Content-type helpers for preview ─────────────────────────────────

// detectContentType guesses a MIME type from the file extension.
func detectContentType(key string) string {
	ext := strings.ToLower(filepath.Ext(key))
	switch ext {
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "text/yaml"
	case ".toml":
		return "application/toml"
	case ".xml":
		return "application/xml"
	case ".html", ".htm":
		return "text/html"
	case ".txt", ".log", ".cfg", ".conf", ".ini", ".properties":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".csv":
		return "text/csv"
	case ".go":
		return "text/x-go"
	case ".py":
		return "text/x-python"
	case ".js":
		return "application/javascript"
	case ".ts":
		return "text/typescript"
	case ".sh", ".bash":
		return "text/x-shellscript"
	case ".sql":
		return "text/x-sql"
	case ".env":
		return "text/plain"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

// mustMarshal marshals v to JSON, returning nil on error (for logging only).
func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

// isTextContentType returns true for MIME types that can be shown as text.
func isTextContentType(ct string) bool {
	if strings.HasPrefix(ct, "text/") {
		return true
	}
	switch ct {
	case "application/json", "application/xml", "application/javascript",
		"application/toml", "application/yaml",
		"text/x-go", "text/x-python", "text/x-shellscript",
		"text/x-sql", "text/typescript":
		return true
	}
	return false
}
