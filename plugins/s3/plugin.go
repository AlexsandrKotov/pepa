package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
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
		DataB64     string `json:"data_b64"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if req.Bucket == "" || req.Key == "" {
		return nil, fmt.Errorf("bucket and key are required")
	}
	if req.DataB64 == "" {
		return nil, fmt.Errorf("data_b64 is required (provide file content as base64)")
	}

	data, err := base64.StdEncoding.DecodeString(req.DataB64)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}

	contentType := req.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	info, err := client.PutObject(ctx, req.Bucket, req.Key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return nil, fmt.Errorf("put object: %w", err)
	}

	return sdk.JSONMarshal(map[string]interface{}{
		"status":  "ok",
		"bucket":  req.Bucket,
		"key":     req.Key,
		"size":    info.Size,
		"etag":    info.ETag,
	})
}

// uploadObjects uploads multiple files to a bucket from base64-encoded data.
func (p *S3Plugin) uploadObjects(ctx context.Context, client *minio.Client, params []byte) ([]byte, error) {
	var req struct {
		Bucket  string   `json:"bucket"`
		Prefix  string   `json:"prefix"`
		Keys    []string `json:"keys"`
		DataB64 []string `json:"data_b64"`
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
	if len(req.DataB64) != len(req.Keys) {
		return nil, fmt.Errorf("keys and data_b64 must have the same length")
	}

	prefix := strings.TrimRight(req.Prefix, "/")
	uploaded := 0
	var errors []string
	for i, key := range req.Keys {
		data, err := base64.StdEncoding.DecodeString(req.DataB64[i])
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: decode error: %v", key, err))
			continue
		}
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "/" + key
		}
		_, err = client.PutObject(ctx, req.Bucket, fullKey, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", key, err))
			continue
		}
		uploaded++
	}

	result := map[string]interface{}{
		"status":   "ok",
		"bucket":   req.Bucket,
		"prefix":   req.Prefix,
		"uploaded": uploaded,
		"total":    len(req.Keys),
	}
	if len(errors) > 0 {
		result["errors"] = errors
	}
	return sdk.JSONMarshal(result)
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
