package config_test

import (
	"task-manager-api/internal/config"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:gosec // Test mock configuration
func validProductionConfig() *config.Config {
	return &config.Config{
		AppPort:            8080,
		AppEnv:             "production",
		JWTSecret:          "prod-super-secure-random-secret-key-64bytes-very-long!",
		JWTExpiration:      15 * time.Minute,
		S3AccessKey:        "actual-prod-access-key-id",
		S3SecretKey:        "actual-prod-secret-access-key",
		CORSAllowedOrigins: []string{"https://app.taskmanager.com", "https://api.taskmanager.com"},
		RequestTimeout:     30 * time.Second,
		ServerWriteTimeout: 60 * time.Second,
	}
}

func TestConfig_Validate_ProductionSuccess(t *testing.T) {
	cfg := validProductionConfig()
	err := cfg.Validate("production")
	require.NoError(t, err)
}

func TestConfig_Validate_DevelopmentAllowsDefaults(t *testing.T) {
	cfg := &config.Config{
		AppEnv:             "development",
		JWTSecret:          "change-this-to-a-secure-random-secret-key-in-production",
		S3AccessKey:        "minioadmin",
		S3SecretKey:        "minioadmin",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
		RequestTimeout:     30 * time.Second,
		ServerWriteTimeout: 60 * time.Second,
	}

	err := cfg.Validate("development")
	require.NoError(t, err, "Development environment should allow default dev credentials and localhost CORS")
}

func TestConfig_Validate_ProductionInsecureJWTSecret(t *testing.T) {
	testCases := []struct {
		name      string
		jwtSecret string
	}{
		{"ExampleSecretFromDotenv", "change-this-to-a-secure-random-secret-key-in-production"},
		{"DevSecretFromReadme", "task-manager-dev-secret-key-32bytes-min!"},
		{"SequentialTestSecret", "12345678901234567890123456789012"},
		{"TooShortSecret", "short-secret"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validProductionConfig()
			cfg.JWTSecret = tc.jwtSecret

			err := cfg.Validate("production")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "JWT_SECRET")
		})
	}
}

func TestConfig_Validate_ProductionDefaultS3Credentials(t *testing.T) {
	t.Run("DefaultS3AccessKey", func(t *testing.T) {
		cfg := validProductionConfig()
		cfg.S3AccessKey = "minioadmin"

		err := cfg.Validate("production")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "S3_ACCESS_KEY")
	})

	t.Run("DefaultS3SecretKey", func(t *testing.T) {
		cfg := validProductionConfig()
		cfg.S3SecretKey = "minioadmin"

		err := cfg.Validate("production")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "S3_SECRET_KEY")
	})
}

func TestConfig_Validate_ProductionInsecureCORS(t *testing.T) {
	testCases := []struct {
		name    string
		origins []string
	}{
		{"WildcardOrigin", []string{"*"}},
		{"LocalhostOrigin", []string{"http://localhost:3000"}},
		{"LoopbackIPOrigin", []string{"http://127.0.0.1:5173"}},
		{"MixedWithLocalhost", []string{"https://app.taskmanager.com", "http://localhost:8080"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validProductionConfig()
			cfg.CORSAllowedOrigins = tc.origins

			err := cfg.Validate("production")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "CORS")
		})
	}
}

func TestConfig_Validate_TimeoutMisalignment(t *testing.T) {
	cfg := validProductionConfig()
	// ServerWriteTimeout (10s) < RequestTimeout (30s) -> invalid!
	cfg.ServerWriteTimeout = 10 * time.Second
	cfg.RequestTimeout = 30 * time.Second

	err := cfg.Validate("production")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ServerWriteTimeout")

	// Also invalid in development
	errDev := cfg.Validate("development")
	require.Error(t, errDev)
	assert.Contains(t, errDev.Error(), "ServerWriteTimeout")
}

func TestConfig_Load_S3PublicEndpointAndPresign(t *testing.T) {
	t.Setenv("APP_PORT", "8080")
	t.Setenv("DB_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("JWT_SECRET", "test-secret-at-least-32-bytes-long!")
	t.Setenv("S3_PUBLIC_ENDPOINT", "https://cdn.example.com")
	t.Setenv("ATTACHMENT_PRESIGN_ENABLED", "false")
	t.Setenv("MAX_ATTACHMENTS_PER_TASK", "50")
	t.Setenv("MAX_USER_STORAGE_MB", "1000")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example.com", cfg.S3PublicEndpoint)
	assert.False(t, cfg.AttachmentPresignEnabled)
	assert.Equal(t, int64(50), cfg.MaxAttachmentsPerTask)
	assert.Equal(t, int64(1000), cfg.MaxUserStorageMB)
}
