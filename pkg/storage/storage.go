package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client defines the interface for object storage interactions (MinIO / S3).
type Client interface {
	Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)
	EnsureBucket(ctx context.Context) error
}

// MinIOClient implements Client for MinIO and AWS S3 compatible endpoints.
type MinIOClient struct {
	client *minio.Client
	bucket string
	region string
}

// NewMinIOClient initializes a new MinIO/S3 storage client.
func NewMinIOClient(endpoint, accessKey, secretKey, bucket, region string, useSSL bool) (*MinIOClient, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize minio client: %w", err)
	}

	return &MinIOClient{
		client: client,
		bucket: bucket,
		region: region,
	}, nil
}

// EnsureBucket verifies whether the configured bucket exists, and creates it if not.
func (m *MinIOClient) EnsureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		err = m.client.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{
			Region: m.region,
		})
		if err != nil {
			return fmt.Errorf("failed to create bucket %s: %w", m.bucket, err)
		}
	}

	return nil
}

// Upload streams object data into the configured bucket.
func (m *MinIOClient) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	opts := minio.PutObjectOptions{
		ContentType: contentType,
	}

	_, err := m.client.PutObject(ctx, m.bucket, key, reader, size, opts)
	if err != nil {
		return fmt.Errorf("failed to upload object to %s: %w", key, err)
	}

	return nil
}

// Download retrieves the object stream from the bucket. The caller must close the returned ReadCloser.
func (m *MinIOClient) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := m.client.GetObject(ctx, m.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object %s: %w", key, err)
	}

	// Stat object to verify existence before returning reader
	_, err = obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, fmt.Errorf("object %s not found: %w", key, err)
	}

	return obj, nil
}

// Delete removes an object from the bucket.
func (m *MinIOClient) Delete(ctx context.Context, key string) error {
	err := m.client.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to remove object %s: %w", key, err)
	}
	return nil
}

// GetPresignedURL generates a temporary presigned GET URL for secure downloading.
func (m *MinIOClient) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	reqParams := make(url.Values)
	u, err := m.client.PresignedGetObject(ctx, m.bucket, key, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned url for %s: %w", key, err)
	}
	return u.String(), nil
}
