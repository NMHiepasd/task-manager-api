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
		GinMode:            "release",
		RedisPassword:      "super-secure-production-redis-password-12345",
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

func TestConfig_Validate_AppEnvAllowedValues(t *testing.T) {
	testCases := []struct {
		name        string
		env         string
		expectError bool
	}{
		{"DevelopmentValid", "development", false},
		{"StagingValid", "staging", false},
		{"ProductionValid", "production", false},
		{"TestValid", "test", false},
		{"LocalInvalid", "local", true},
		{"DevInvalid", "dev", true},
		{"ProdInvalid", "prod", true},
		{"CustomInvalid", "qa-environment", true},
		{"EmptyInvalid", "unknown", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validProductionConfig()
			cfg.AppEnv = tc.env

			err := cfg.Validate(tc.env)
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid APP_ENV")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Validate_Production_GinMode(t *testing.T) {
	testCases := []struct {
		name        string
		ginMode     string
		expectError bool
	}{
		{"ReleaseModeValid", "release", false},
		{"DebugModeForbidden", "debug", true},
		{"TestModeForbidden", "test", true},
		{"EmptyModeForbidden", "", true},
		{"CustomModeForbidden", "custom", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validProductionConfig()
			cfg.GinMode = tc.ginMode

			err := cfg.Validate("production")
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "GIN_MODE")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Validate_Production_RedisPassword(t *testing.T) {
	testCases := []struct {
		name          string
		redisPassword string
		expectError   bool
	}{
		{"StrongPasswordValid", "super-secret-redis-password", false},
		{"SimplePasswordValid", "redis123", false},
		{"EmptyPasswordForbidden", "", true},
		{"WhitespaceOnlyPasswordForbidden", "   ", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validProductionConfig()
			cfg.RedisPassword = tc.redisPassword

			err := cfg.Validate("production")
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "REDIS_PASSWORD")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Validate_RequestTimeout_LessThan_InProgressLockTTL(t *testing.T) {
	testCases := []struct {
		name           string
		requestTimeout time.Duration
		expectError    bool
	}{
		{"NormalTimeoutPass", 30 * time.Second, false},
		{"FastTimeoutPass", 5 * time.Second, false},
		{"NearLimitTimeoutPass", 59 * time.Second, false},
		{"Exact60sLimitFails", 60 * time.Second, true},
		{"Over60sLimitFails", 61 * time.Second, true},
		{"HighTimeoutFails", 120 * time.Second, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validProductionConfig()
			cfg.RequestTimeout = tc.requestTimeout
			// Ensure ServerWriteTimeout >= RequestTimeout so write timeout misalignment is not triggered
			cfg.ServerWriteTimeout = tc.requestTimeout + 10*time.Second

			err := cfg.Validate("production")
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "RequestTimeout")
				assert.Contains(t, err.Error(), "InProgressLockTTL")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Validate_ProductionDisallowsLocalWebhooks(t *testing.T) {
	t.Run("DisallowAllowLocalWebhooksInProd", func(t *testing.T) {
		cfg := validProductionConfig()
		cfg.AllowLocalWebhooks = true

		err := cfg.Validate("production")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ALLOW_LOCAL_WEBHOOKS")
	})

	t.Run("DisallowWebhookAllowPrivateHostsInProd", func(t *testing.T) {
		cfg := validProductionConfig()
		cfg.WebhookAllowPrivateHosts = true

		err := cfg.Validate("production")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "WEBHOOK_ALLOW_PRIVATE_HOSTS")
	})

	t.Run("AllowLocalWebhooksInNonProdEnvs", func(t *testing.T) {
		for _, envName := range []string{"development", "test", "staging"} {
			cfg := validProductionConfig()
			cfg.AllowLocalWebhooks = true
			cfg.WebhookAllowPrivateHosts = true

			err := cfg.Validate(envName)
			require.NoError(t, err, "env %s should allow local webhooks flag", envName)
		}
	})
}
