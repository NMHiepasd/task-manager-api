package storage_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"task-manager-api/pkg/storage"
)

func TestMinIOClient_GetPresignedURL_DefaultInternalEndpoint(t *testing.T) {
	client, err := storage.NewMinIOClient(
		"localhost:9000",
		"minioadmin",
		"minioadmin",
		"task-attachments",
		"us-east-1",
		false,
	)
	require.NoError(t, err)

	ctx := context.Background()
	presigned, err := client.GetPresignedURL(ctx, "tasks/123/file.pdf", 15*time.Minute)
	require.NoError(t, err)

	u, err := url.Parse(presigned)
	require.NoError(t, err)
	assert.Equal(t, "http", u.Scheme)
	assert.Equal(t, "localhost:9000", u.Host)
	assert.Contains(t, u.Path, "/task-attachments/tasks/123/file.pdf")
	assert.NotEmpty(t, u.Query().Get("X-Amz-Signature"))
}

func TestMinIOClient_GetPresignedURL_PublicEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		internal       string
		public         string
		defaultSSL     bool
		expectedScheme string
		expectedHost   string
	}{
		{
			name:           "PublicEndpointWithHostOnly",
			internal:       "minio:9000",
			public:         "s3.example.com",
			defaultSSL:     false,
			expectedScheme: "http",
			expectedHost:   "s3.example.com",
		},
		{
			name:           "PublicEndpointWithHTTPScheme",
			internal:       "minio:9000",
			public:         "http://cdn.taskmanager.com:8080",
			defaultSSL:     false,
			expectedScheme: "http",
			expectedHost:   "cdn.taskmanager.com:8080",
		},
		{
			name:           "PublicEndpointWithHTTPSScheme",
			internal:       "minio:9000",
			public:         "https://s3.taskmanager.com",
			defaultSSL:     false,
			expectedScheme: "https",
			expectedHost:   "s3.taskmanager.com",
		},
		{
			name:           "EmptyPublicFallsBackToInternal",
			internal:       "storage.internal:9000",
			public:         "",
			defaultSSL:     false,
			expectedScheme: "http",
			expectedHost:   "storage.internal:9000",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, err := storage.NewMinIOClientWithPublic(
				tc.internal,
				tc.public,
				"test-access-key",
				"test-secret-key-123",
				"task-attachments",
				"us-east-1",
				tc.defaultSSL,
			)
			require.NoError(t, err)

			presigned, err := client.GetPresignedURL(context.Background(), "tasks/test-key", 10*time.Minute)
			require.NoError(t, err)

			u, err := url.Parse(presigned)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedScheme, u.Scheme)
			assert.Equal(t, tc.expectedHost, u.Host)
			assert.Contains(t, u.Path, "/task-attachments/tasks/test-key")
			assert.NotEmpty(t, u.Query().Get("X-Amz-Signature"))
		})
	}
}

func TestMinIOClient_InvalidEndpoint(t *testing.T) {
	_, err := storage.NewMinIOClientWithPublic(
		"http://invalid :::: endpoint",
		"",
		"key",
		"sec",
		"bucket",
		"us-east-1",
		false,
	)
	require.Error(t, err)
}
