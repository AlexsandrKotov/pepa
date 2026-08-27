package main

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// S3Plugin implements provider.Provider for S3-compatible object storage.
type S3Plugin struct{}

var _ provider.Provider = (*S3Plugin)(nil)

func (p *S3Plugin) Name() string        { return "s3" }
func (p *S3Plugin) Version() string     { return "0.1.0" }
func (p *S3Plugin) Description() string { return "S3-compatible object storage — browse buckets, upload and manage files" }
func (p *S3Plugin) PluginType() string  { return "storage" }

func (p *S3Plugin) Actions() []string {
	return []string{
		"test_connection",
		"list_buckets",
		"list_objects",
		"create_bucket",
		"delete_bucket",
		"upload_object",
		"upload_objects",
		"delete_object",
		"get_object_meta",
	}
}

func (p *S3Plugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	client, err := p.buildClient(config)
	if err != nil {
		return nil, err
	}

	switch action {
	case "test_connection":
		return p.testConnection(ctx, client)
	case "list_buckets":
		return p.listBuckets(ctx, client)
	case "list_objects":
		return p.listObjects(ctx, client, params)
	case "create_bucket":
		return p.createBucket(ctx, client, params)
	case "delete_bucket":
		return p.deleteBucket(ctx, client, params)
	case "upload_object":
		return p.uploadObject(ctx, client, params)
	case "upload_objects":
		return p.uploadObjects(ctx, client, params)
	case "delete_object":
		return p.deleteObject(ctx, client, params)
	case "get_object_meta":
		return p.getObjectMeta(ctx, client, params)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *S3Plugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "S3 plugin ready — configure an S3 connection with endpoint and credentials",
	}, nil
}

// buildClient creates a minio client from the connection config.
func (p *S3Plugin) buildClient(config map[string]string) (*minio.Client, error) {
	endpoint := config["endpoint"]
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}

	useSSL := config["use_ssl"] == "true"

	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(config["access_key"], config["secret_key"], ""),
		Secure: useSSL,
		Region: "",
	}

	mc, err := minio.New(u.Host, opts)
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	return mc, nil
}

// testConnection verifies connectivity by listing buckets.
func (p *S3Plugin) testConnection(ctx context.Context, client *minio.Client) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := client.ListBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot reach S3: %w", err)
	}

	return sdk.JSONMarshal(map[string]string{
		"status":  "connected",
		"message": "Successfully connected to S3 storage",
	})
}

// listBuckets returns all visible buckets.
func (p *S3Plugin) listBuckets(ctx context.Context, client *minio.Client) ([]byte, error) {
	buckets, err := client.ListBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}

	type bucketInfo struct {
		Name         string `json:"name"`
		CreationDate string `json:"creation_date"`
	}

	result := make([]bucketInfo, 0, len(buckets))
	for _, b := range buckets {
		result = append(result, bucketInfo{
			Name:         b.Name,
			CreationDate: b.CreationDate.Format("2006-01-02T15:04:05Z"),
		})
	}

	return sdk.JSONMarshal(map[string]any{
		"buckets": result,
		"total":   len(result),
	})
}

// listObjects lists objects in a bucket with optional prefix.
func (p *S3Plugin) listObjects(ctx context.Context, client *minio.Client, params []byte) ([]byte, error) {
	var req struct {
		Bucket    string `json:"bucket"`
		Prefix    string `json:"prefix"`
		Recursive bool   `json:"recursive"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}

	type objectInfo struct {
		Key          string `json:"key"`
		Size         int64  `json:"size"`
		ContentType  string `json:"content_type"`
		LastModified string `json:"last_modified"`
	}

	var objects []objectInfo
	for obj := range client.ListObjects(ctx, req.Bucket, minio.ListObjectsOptions{
		Prefix:    req.Prefix,
		Recursive: req.Recursive,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list objects: %w", obj.Err)
		}
		objects = append(objects, objectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			ContentType:  obj.ContentType,
			LastModified: obj.LastModified.Format("2006-01-02T15:04:05Z"),
		})
	}

	if objects == nil {
		objects = []objectInfo{}
	}

	return sdk.JSONMarshal(map[string]any{
		"objects": objects,
		"total":   len(objects),
		"prefix":  req.Prefix,
	})
}

// createBucket creates a new bucket.
func (p *S3Plugin) createBucket(ctx context.Context, client *minio.Client, params []byte) ([]byte, error) {
	var req struct {
		Bucket string `json:"bucket"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Bucket == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	if err := client.MakeBucket(ctx, req.Bucket, minio.MakeBucketOptions{}); err != nil {
		return nil, fmt.Errorf("create bucket: %w", err)
	}

	return sdk.JSONMarshal(map[string]string{
		"status": "created",
		"bucket": req.Bucket,
	})
}

// deleteBucket removes an empty bucket.
func (p *S3Plugin) deleteBucket(ctx context.Context, client *minio.Client, params []byte) ([]byte, error) {
	var req struct {
		Bucket string `json:"bucket"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Bucket == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	if err := client.RemoveBucket(ctx, req.Bucket); err != nil {
		return nil, fmt.Errorf("delete bucket: %w", err)
	}

	return sdk.JSONMarshal(map[string]string{
		"status": "deleted",
		"bucket": req.Bucket,
	})
}

// uploadObject uploads data to an object. The data is provided as base64 in the params.
func (p *S3Plugin) uploadObject(ctx context.Context, client *minio.Client, params []byte) ([]byte, error) {
	var req struct {
		Bucket      string `json:"bucket"`
		Key         string `json:"key"`
		ContentType string `json:"content_type"`
		// Data is not included here — in practice, uploads go through
		// the REST API (s3browser_handlers.go) which handles multipart.
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Bucket == "" || req.Key == "" {
		return nil, fmt.Errorf("bucket and key are required")
	}

	// This action is a placeholder — actual file uploads go through the
	// dedicated REST endpoint for multipart streaming.
	return sdk.JSONMarshal(map[string]string{
		"status":  "ok",
		"message": "Use the REST upload endpoint for file uploads",
		"bucket":  req.Bucket,
		"key":     req.Key,
	})
}

// uploadObjects uploads multiple files to a bucket, preserving directory structure.
// Each entry in the "keys" array maps to a corresponding entry in "data_b64" (base64).
// In practice, bulk uploads go through the REST endpoint; this is for plugin API completeness.
func (p *S3Plugin) uploadObjects(ctx context.Context, client *minio.Client, params []byte) ([]byte, error) {
	var req struct {
		Bucket string   `json:"bucket"`
		Prefix string   `json:"prefix"`
		Keys   []string `json:"keys"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}
	if len(req.Keys) == 0 {
		return nil, fmt.Errorf("at least one key is required")
	}

	// This action is a metadata placeholder — actual multi-file uploads go
	// through the dedicated REST endpoint POST /upload-multiple which handles
	// multipart streaming of real file data.
	return sdk.JSONMarshal(map[string]any{
		"status":  "ok",
		"message": "Use the REST upload-multiple endpoint for bulk file uploads",
		"bucket":  req.Bucket,
		"prefix":  req.Prefix,
		"count":   len(req.Keys),
	})
}

// deleteObject removes an object.
func (p *S3Plugin) deleteObject(ctx context.Context, client *minio.Client, params []byte) ([]byte, error) {
	var req struct {
		Bucket string `json:"bucket"`
		Key    string `json:"key"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Bucket == "" || req.Key == "" {
		return nil, fmt.Errorf("bucket and key are required")
	}

	if err := client.RemoveObject(ctx, req.Bucket, req.Key, minio.RemoveObjectOptions{}); err != nil {
		return nil, fmt.Errorf("delete object: %w", err)
	}

	return sdk.JSONMarshal(map[string]string{
		"status": "deleted",
		"key":    req.Key,
		"bucket": req.Bucket,
	})
}

// getObjectMeta returns metadata about an object.
func (p *S3Plugin) getObjectMeta(ctx context.Context, client *minio.Client, params []byte) ([]byte, error) {
	var req struct {
		Bucket string `json:"bucket"`
		Key    string `json:"key"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Bucket == "" || req.Key == "" {
		return nil, fmt.Errorf("bucket and key are required")
	}

	info, err := client.StatObject(ctx, req.Bucket, req.Key, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("stat object: %w", err)
	}

	return sdk.JSONMarshal(map[string]any{
		"key":           info.Key,
		"size":          info.Size,
		"content_type":  info.ContentType,
		"last_modified": info.LastModified.Format("2006-01-02T15:04:05Z"),
		"bucket":        req.Bucket,
	})
}
