package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config holds all configuration values for the application.
type Config struct {
	// Application Settings
	AppPort  int    `env:"APP_PORT" envDefault:"8080"`
	AppEnv   string `env:"APP_ENV" envDefault:"development"`
	GinMode  string `env:"GIN_MODE" envDefault:"debug"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	// Security & Auth
	JWTSecret            string        `env:"JWT_SECRET,required"`
	JWTExpiration        time.Duration `env:"JWT_EXPIRATION" envDefault:"15m"`
	JWTRefreshExpiration time.Duration `env:"JWT_REFRESH_EXPIRATION" envDefault:"168h"`
	JWTIssuer            string        `env:"JWT_ISSUER" envDefault:"task-manager-api"`
	JWTAudience          string        `env:"JWT_AUDIENCE" envDefault:"task-manager-client"`
	CORSAllowedOrigins   []string      `env:"CORS_ALLOWED_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000,http://localhost:5173"`
	TrustedProxies       []string      `env:"TRUSTED_PROXIES" envSeparator:"," envDefault:"127.0.0.1,172.28.0.2"`
	RateLimitAuthRate    float64       `env:"RATE_LIMIT_AUTH_RATE" envDefault:"0.166667"` // 10 reqs per minute (~0.167 req/sec)
	RateLimitAuthBurst   int           `env:"RATE_LIMIT_AUTH_BURST" envDefault:"5"`
	RateLimitAuthLimit   int           `env:"RATE_LIMIT_AUTH_LIMIT" envDefault:"10"`
	RateLimitAuthWindow  time.Duration `env:"RATE_LIMIT_AUTH_WINDOW" envDefault:"1m"`
	RateLimitAPIRate     float64       `env:"RATE_LIMIT_API_RATE" envDefault:"100"` // 100 reqs per minute
	RateLimitAPIBurst    int           `env:"RATE_LIMIT_API_BURST" envDefault:"50"`
	RateLimitAPILimit    int           `env:"RATE_LIMIT_API_LIMIT" envDefault:"100"`
	RateLimitAPIWindow   time.Duration `env:"RATE_LIMIT_API_WINDOW" envDefault:"1m"`

	// Database Connection & Pool
	DatabaseURL         string        `env:"DB_URL,required"`
	DBMaxConns          int32         `env:"DB_MAX_CONNS" envDefault:"25"`
	DBMinConns          int32         `env:"DB_MIN_CONNS" envDefault:"5"`
	DBMaxConnLifetime   time.Duration `env:"DB_MAX_CONN_LIFETIME" envDefault:"1h"`
	DBMaxConnIdleTime   time.Duration `env:"DB_MAX_CONN_IDLE_TIME" envDefault:"30m"`
	DBHealthCheckPeriod time.Duration `env:"DB_HEALTH_CHECK_PERIOD" envDefault:"1m"`
	DBStatementTimeout  time.Duration `env:"DB_STATEMENT_TIMEOUT" envDefault:"10s"`

	// Redis & Caching
	RedisURL        string        `env:"REDIS_URL" envDefault:"redis://localhost:6379/0"`
	RedisPassword   string        `env:"REDIS_PASSWORD" envDefault:""`
	RedisDB         int           `env:"REDIS_DB" envDefault:"0"`
	CacheDefaultTTL time.Duration `env:"CACHE_DEFAULT_TTL" envDefault:"15m"`

	// Background Worker & Asynq
	WorkerEnabled                bool   `env:"WORKER_ENABLED" envDefault:"true"`
	WorkerConcurrency            int    `env:"WORKER_CONCURRENCY" envDefault:"10"`
	CleanupCronSchedule          string `env:"CLEANUP_CRON_SCHEDULE" envDefault:"@every 6h"`
	DeadlineReminderCronSchedule string `env:"DEADLINE_REMINDER_CRON_SCHEDULE" envDefault:"@hourly"`

	// HTTP Server & Middleware Timeouts
	RequestTimeout          time.Duration `env:"REQUEST_TIMEOUT" envDefault:"30s"`
	ServerReadHeaderTimeout time.Duration `env:"SERVER_READ_HEADER_TIMEOUT" envDefault:"5s"`
	ServerReadTimeout       time.Duration `env:"SERVER_READ_TIMEOUT" envDefault:"60s"`
	ServerWriteTimeout      time.Duration `env:"SERVER_WRITE_TIMEOUT" envDefault:"60s"`
	ServerIdleTimeout       time.Duration `env:"SERVER_IDLE_TIMEOUT" envDefault:"60s"`

	// Object Storage (MinIO / S3)
	S3Endpoint               string `env:"S3_ENDPOINT" envDefault:"localhost:9000"`
	S3PublicEndpoint         string `env:"S3_PUBLIC_ENDPOINT"`
	S3AccessKey              string `env:"S3_ACCESS_KEY" envDefault:"minioadmin"`
	S3SecretKey              string `env:"S3_SECRET_KEY" envDefault:"minioadmin"`
	S3BucketName             string `env:"S3_BUCKET_NAME" envDefault:"task-attachments"`
	S3UseSSL                 bool   `env:"S3_USE_SSL" envDefault:"false"`
	S3Region                 string `env:"S3_REGION" envDefault:"us-east-1"`
	MaxUploadSizeMB          int64  `env:"MAX_UPLOAD_SIZE_MB" envDefault:"10"`
	AttachmentPresignEnabled bool   `env:"ATTACHMENT_PRESIGN_ENABLED" envDefault:"true"`
	MaxAttachmentsPerTask    int64  `env:"MAX_ATTACHMENTS_PER_TASK" envDefault:"20"`
	MaxUserStorageMB         int64  `env:"MAX_USER_STORAGE_MB" envDefault:"500"`

	// Webhooks
	AllowLocalWebhooks bool `env:"ALLOW_LOCAL_WEBHOOKS" envDefault:"false"`
	WebhookMaxRetry    int  `env:"WEBHOOK_MAX_RETRY" envDefault:"6"`
}

// Load parses environment variables into Config and validates required fields.
// It automatically attempts to load variables from a local .env file if present.
func Load() (*Config, error) {
	// Load .env file if it exists (no error if missing, e.g. in production/Docker)
	_ = godotenv.Load()

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse environment configuration: %w", err)
	}
	return cfg, nil
}

// Validate performs fail-fast sanity checks on the configuration, enforcing strict security
// constraints when running in a production environment.
func (c *Config) Validate(env string) error {
	var errs []error

	targetEnv := env
	if targetEnv == "" {
		targetEnv = c.AppEnv
	}

	// 1. APP_ENV validation: must be one of development, staging, production, test
	validEnvs := map[string]struct{}{
		"development": {},
		"staging":     {},
		"production":  {},
		"test":        {},
	}
	if _, ok := validEnvs[targetEnv]; !ok {
		errs = append(errs, fmt.Errorf("invalid APP_ENV %q: must be one of development, staging, production, test", targetEnv))
	}

	// 2. Timeout validations
	// Always verify timeout alignment: server write timeout must be >= request timeout
	// so the timeout middleware can complete and return a 504 Problem Details before the server cuts the connection.
	if c.ServerWriteTimeout > 0 && c.RequestTimeout > 0 && c.ServerWriteTimeout < c.RequestTimeout {
		errs = append(errs, fmt.Errorf("ServerWriteTimeout (%v) must be greater than or equal to RequestTimeout (%v)", c.ServerWriteTimeout, c.RequestTimeout))
	}

	// RequestTimeout must be strictly less than InProgressLockTTL (60s) to prevent lock expiration during active processing
	const inProgressLockTTL = 60 * time.Second
	if c.RequestTimeout >= inProgressLockTTL {
		errs = append(errs, fmt.Errorf("RequestTimeout (%v) must be less than InProgressLockTTL (%v)", c.RequestTimeout, inProgressLockTTL))
	}

	if targetEnv == "production" {
		// Enforce GIN_MODE=release in production
		if c.GinMode != "release" {
			errs = append(errs, fmt.Errorf("GIN_MODE must be 'release' in production, got %q", c.GinMode))
		}

		// Enforce non-empty REDIS_PASSWORD in production
		if strings.TrimSpace(c.RedisPassword) == "" {
			errs = append(errs, errors.New("REDIS_PASSWORD cannot be empty in production"))
		}

		// Insecure default JWT secret check
		insecureSecrets := map[string]struct{}{
			"change-this-to-a-secure-random-secret-key-in-production": {},
			"task-manager-dev-secret-key-32bytes-min!":                {},
			"12345678901234567890123456789012":                        {},
		}
		if _, insecure := insecureSecrets[c.JWTSecret]; insecure {
			errs = append(errs, errors.New("JWT_SECRET matches insecure example/default secret; production requires a unique, secure secret"))
		}
		if len(c.JWTSecret) < 32 {
			errs = append(errs, errors.New("JWT_SECRET must be at least 32 characters in production"))
		}

		// 2. Default S3 / MinIO credentials check
		if c.S3AccessKey == "minioadmin" {
			errs = append(errs, errors.New("S3_ACCESS_KEY cannot be default 'minioadmin' in production"))
		}
		if c.S3SecretKey == "minioadmin" {
			errs = append(errs, errors.New("S3_SECRET_KEY cannot be default 'minioadmin' in production"))
		}

		// 3. CORS allowed origins check
		for _, origin := range c.CORSAllowedOrigins {
			trimmed := strings.TrimSpace(origin)
			if trimmed == "*" {
				errs = append(errs, errors.New("wildcard '*' CORS origin is forbidden in production"))
			} else if strings.Contains(trimmed, "localhost") || strings.Contains(trimmed, "127.0.0.1") {
				errs = append(errs, fmt.Errorf("insecure CORS origin %q containing localhost/127.0.0.1 is not permitted in production", origin))
			}
		}
	}

	return errors.Join(errs...)
}
