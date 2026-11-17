package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ErrObjectNotFound indicates that the requested object does not exist in storage.
var ErrObjectNotFound = errors.New("storage object not found")

// IsNotFoundError checks whether an error signifies that the object does not exist.
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrObjectNotFound) {
		return true
	}
	var errResp minio.ErrorResponse
	if errors.As(err, &errResp) {
		if errResp.Code == "NoSuchKey" || errResp.Code == "ResourceNotFound" {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "nosuchkey") || strings.Contains(msg, "not found") || strings.Contains(msg, "does not exist")
}

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
	client       *minio.Client
	signerClient *minio.Client
	bucket       string
	region       string
}

// parseEndpoint extracts host and determines SSL for an endpoint string.
func parseEndpoint(raw string, defaultSSL bool) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", defaultSSL, nil
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", false, fmt.Errorf("invalid endpoint url: %w", err)
		}
		return u.Host, u.Scheme == "https", nil
	}
	return raw, defaultSSL, nil
}

// NewMinIOClient initializes a new MinIO/S3 storage client.
func NewMinIOClient(endpoint, accessKey, secretKey, bucket, region string, useSSL bool) (*MinIOClient, error) {
	return NewMinIOClientWithPublic(endpoint, "", accessKey, secretKey, bucket, region, useSSL)
}

// NewMinIOClientWithPublic initializes a new MinIO/S3 storage client with an optional public endpoint.
// When publicEndpoint is provided, it uses a dedicated signing client solely to compute presigned URLs
// with the public host, avoiding any network connectivity requirements on the signing path.
func NewMinIOClientWithPublic(endpoint, publicEndpoint, accessKey, secretKey, bucket, region string, useSSL bool) (*MinIOClient, error) {
	internalHost, internalSSL, err := parseEndpoint(endpoint, useSSL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse internal s3 endpoint: %w", err)
	}

	client, err := minio.New(internalHost, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: internalSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize minio client: %w", err)
	}

	signerClient := client
	if strings.TrimSpace(publicEndpoint) != "" {
		pubHost, pubSSL, pErr := parseEndpoint(publicEndpoint, useSSL)
		if pErr != nil {
			return nil, fmt.Errorf("failed to parse public s3 endpoint: %w", pErr)
		}
		sClient, sErr := minio.New(pubHost, &minio.Options{
			Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
			Secure: pubSSL,
			Region: region,
		})
		if sErr != nil {
			return nil, fmt.Errorf("failed to initialize minio signer client: %w", sErr)
		}
		signerClient = sClient
	}

	return &MinIOClient{
		client:       client,
		signerClient: signerClient,
		bucket:       bucket,
		region:       region,
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
	u, err := m.signerClient.PresignedGetObject(ctx, m.bucket, key, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned url for %s: %w", key, err)
	}
	return u.String(), nil
}
