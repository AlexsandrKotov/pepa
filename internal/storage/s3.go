// Package storage provides an S3-compatible object storage client for PEPA.
// It wraps minio-go to offer upload, download, list, and delete operations.
// S3 is optional — by default, LocalStorage is used (filesystem-backed).
package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pepa/pepa/internal/config"
)

// Bucket represents one of the PEPA storage buckets (S3 only).
type Bucket string

const (
	BucketPlugins Bucket = "plugins"
)

// S3Client wraps a minio-go client and implements the Storage interface.
type S3Client struct {
	mc            *minio.Client
	bucketPlugins string
}

// NewS3Client creates and verifies connectivity to the S3-compatible store.
// It also ensures the three PEPA buckets exist.
func NewS3Client(cfg config.S3Config) (*S3Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("s3.endpoint is required")
	}

	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid s3.endpoint: %w", err)
	}

	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: "",
	}

	mc, err := minio.New(endpoint.Host, opts)
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	c := &S3Client{
		mc:            mc,
		bucketPlugins: cfg.BucketPlugins,
	}

	if c.bucketPlugins == "" {
		c.bucketPlugins = "pepa-plugins"
	}

	return c, nil
}

// EnsureBuckets creates the PEPA plugins bucket if it does not exist.
func (c *S3Client) EnsureBuckets(ctx context.Context) error {
	exists, err := c.mc.BucketExists(ctx, c.bucketPlugins)
	if err != nil {
		return fmt.Errorf("check bucket %s: %w", c.bucketPlugins, err)
	}
	if !exists {
		if err := c.mc.MakeBucket(ctx, c.bucketPlugins, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("create bucket %s: %w", c.bucketPlugins, err)
		}
		slog.Info("created S3 bucket", "arg1", c.bucketPlugins)
	}
	slog.Info("S3 connected, bucket verified: plugins=", "arg1", c.bucketPlugins)
	return nil
}

// bucketName resolves the Bucket enum to the configured bucket name.
func (c *S3Client) bucketName(b Bucket) string {
	if b == BucketPlugins {
		return c.bucketPlugins
	}
	return string(b)
}

// Upload stores an object in the specified bucket.
// contentType is optional — pass "" for auto-detection.
func (c *S3Client) Upload(ctx context.Context, bucket Bucket, key string, reader io.Reader, objectSize int64, contentType string) error {
	opts := minio.PutObjectOptions{}
	if contentType != "" {
		opts.ContentType = contentType
	}

	bucketName := c.bucketName(bucket)
	_, err := c.mc.PutObject(ctx, bucketName, key, reader, objectSize, opts)
	if err != nil {
		return fmt.Errorf("upload %s/%s: %w", bucketName, key, err)
	}
	return nil
}

// Download retrieves an object from the specified bucket.
// The caller is responsible for closing the returned ReadCloser.
func (c *S3Client) Download(ctx context.Context, bucket Bucket, key string) (io.ReadCloser, error) {
	bucketName := c.bucketName(bucket)
	obj, err := c.mc.GetObject(ctx, bucketName, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("download %s/%s: %w", bucketName, key, err)
	}
	return obj, nil
}

// Delete removes an object from the specified bucket.
func (c *S3Client) Delete(ctx context.Context, bucket Bucket, key string) error {
	bucketName := c.bucketName(bucket)
	if err := c.mc.RemoveObject(ctx, bucketName, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete %s/%s: %w", bucketName, key, err)
	}
	return nil
}

// List returns all objects in the specified bucket, optionally filtered by prefix.
func (c *S3Client) List(ctx context.Context, bucket Bucket, prefix string) ([]ObjectMeta, error) {
	bucketName := c.bucketName(bucket)
	var objects []ObjectMeta

	for obj := range c.mc.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list %s: %w", bucketName, obj.Err)
		}
		objects = append(objects, ObjectMeta{
			Key:          obj.Key,
			Size:         obj.Size,
			ContentType:  obj.ContentType,
			LastModified: obj.LastModified,
		})
	}

	return objects, nil
}

// Stat returns metadata about a single object.
func (c *S3Client) Stat(ctx context.Context, bucket Bucket, key string) (*ObjectMeta, error) {
	bucketName := c.bucketName(bucket)
	info, err := c.mc.StatObject(ctx, bucketName, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("stat %s/%s: %w", bucketName, key, err)
	}
	return &ObjectMeta{
		Key:          info.Key,
		Size:         info.Size,
		ContentType:  info.ContentType,
		LastModified: info.LastModified,
	}, nil
}

// PresignedGetURL generates a time-limited download URL for an object.
func (c *S3Client) PresignedGetURL(ctx context.Context, bucket Bucket, key string, expiry time.Duration) (string, error) {
	bucketName := c.bucketName(bucket)
	u, err := c.mc.PresignedGetObject(ctx, bucketName, key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("presigned URL %s/%s: %w", bucketName, key, err)
	}
	return u.String(), nil
}

// ListPlugins lists all plugin binaries stored in S3.
func (c *S3Client) ListPlugins(ctx context.Context) ([]ObjectMeta, error) {
	return c.List(ctx, BucketPlugins, "")
}

// UploadPlugin stores a plugin binary in the plugins bucket.
func (c *S3Client) UploadPlugin(ctx context.Context, name, version string, reader io.Reader, size int64) error {
	key := PluginKey(name, version)
	return c.Upload(ctx, BucketPlugins, key, reader, size, "application/octet-stream")
}

// DownloadPlugin retrieves a plugin binary from the plugins bucket.
func (c *S3Client) DownloadPlugin(ctx context.Context, name, version string) (io.ReadCloser, error) {
	key := PluginKey(name, version)
	return c.Download(ctx, BucketPlugins, key)
}

// DeletePlugin removes a plugin binary from the plugins bucket.
func (c *S3Client) DeletePlugin(ctx context.Context, name, version string) error {
	key := PluginKey(name, version)
	return c.Delete(ctx, BucketPlugins, key)
}

// StatPlugin returns metadata about a specific plugin binary.
func (c *S3Client) StatPlugin(ctx context.Context, name, version string) (*ObjectMeta, error) {
	key := PluginKey(name, version)
	return c.Stat(ctx, BucketPlugins, key)
}

// IsAvailable returns true if the S3 client was initialized.
func (c *S3Client) IsAvailable() bool {
	return c != nil && c.mc != nil
}

// ---------------------------------------------------------------------------
// Generic operations — work with arbitrary bucket names (for the S3 browser).
// ---------------------------------------------------------------------------

// BucketInfo holds metadata about a single S3 bucket.
type BucketInfo struct {
	Name         string    `json:"name"`
	CreationDate time.Time `json:"creation_date"`
}

// NewS3ClientFromCredentials creates a lightweight S3 client from raw
// connection credentials (endpoint, access key, secret key). This is used
// by the S3 browser to dynamically connect to user-configured storages.
func NewS3ClientFromCredentials(endpoint, accessKey, secretKey string, useSSL bool) (*S3Client, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}

	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
		Region: "",
	}

	mc, err := minio.New(u.Host, opts)
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	return &S3Client{mc: mc}, nil
}

// ListBuckets returns all buckets visible to the configured credentials.
func (c *S3Client) ListBuckets(ctx context.Context) ([]BucketInfo, error) {
	buckets, err := c.mc.ListBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	result := make([]BucketInfo, 0, len(buckets))
	for _, b := range buckets {
		result = append(result, BucketInfo{
			Name:         b.Name,
			CreationDate: b.CreationDate,
		})
	}
	return result, nil
}

// ListObjectsInBucket lists objects in an arbitrary bucket, optionally
// filtered by prefix. If recursive is false, only the top-level "directory"
// entries are returned (using delimiter "/").
func (c *S3Client) ListObjectsInBucket(ctx context.Context, bucketName, prefix string, recursive bool) ([]ObjectMeta, error) {
	var objects []ObjectMeta

	for obj := range c.mc.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: recursive,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list %s: %w", bucketName, obj.Err)
		}
		objects = append(objects, ObjectMeta{
			Key:          obj.Key,
			Size:         obj.Size,
			ContentType:  obj.ContentType,
			LastModified: obj.LastModified,
		})
	}

	return objects, nil
}

// CreateBucketNamed creates a new bucket with the given name.
func (c *S3Client) CreateBucketNamed(ctx context.Context, bucketName string) error {
	if err := c.mc.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create bucket %s: %w", bucketName, err)
	}
	return nil
}

// DeleteBucketNamed removes an empty bucket.
func (c *S3Client) DeleteBucketNamed(ctx context.Context, bucketName string) error {
	if err := c.mc.RemoveBucket(ctx, bucketName); err != nil {
		return fmt.Errorf("delete bucket %s: %w", bucketName, err)
	}
	return nil
}

// UploadToBucket stores an object in the named bucket.
func (c *S3Client) UploadToBucket(ctx context.Context, bucketName, key string, reader io.Reader, objectSize int64, contentType string) error {
	opts := minio.PutObjectOptions{}
	if contentType != "" {
		opts.ContentType = contentType
	}
	if _, err := c.mc.PutObject(ctx, bucketName, key, reader, objectSize, opts); err != nil {
		return fmt.Errorf("upload %s/%s: %w", bucketName, key, err)
	}
	return nil
}

// DownloadFromBucket retrieves an object from the named bucket.
func (c *S3Client) DownloadFromBucket(ctx context.Context, bucketName, key string) (io.ReadCloser, error) {
	obj, err := c.mc.GetObject(ctx, bucketName, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("download %s/%s: %w", bucketName, key, err)
	}
	return obj, nil
}

// DeleteFromBucket removes an object from the named bucket.
func (c *S3Client) DeleteFromBucket(ctx context.Context, bucketName, key string) error {
	if err := c.mc.RemoveObject(ctx, bucketName, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete %s/%s: %w", bucketName, key, err)
	}
	return nil
}

// StatObjectInBucket returns metadata about an object in the named bucket.
func (c *S3Client) StatObjectInBucket(ctx context.Context, bucketName, key string) (*ObjectMeta, error) {
	info, err := c.mc.StatObject(ctx, bucketName, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("stat %s/%s: %w", bucketName, key, err)
	}
	return &ObjectMeta{
		Key:          info.Key,
		Size:         info.Size,
		ContentType:  info.ContentType,
		LastModified: info.LastModified,
	}, nil
}

// PresignedURL generates a time-limited download URL for an object in the named bucket.
func (c *S3Client) PresignedURL(ctx context.Context, bucketName, key string, expiry time.Duration) (string, error) {
	u, err := c.mc.PresignedGetObject(ctx, bucketName, key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("presigned URL %s/%s: %w", bucketName, key, err)
	}
	return u.String(), nil
}

// MinioClient returns the underlying minio client (used by handlers that
// need direct access, e.g. for streaming downloads).
func (c *S3Client) MinioClient() *minio.Client {
	return c.mc
}
