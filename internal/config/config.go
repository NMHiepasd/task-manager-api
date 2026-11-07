package config

import (
	"fmt"
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
	CORSAllowedOrigins   []string      `env:"CORS_ALLOWED_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000,http://localhost:5173"`
	RateLimitAuthRate    float64       `env:"RATE_LIMIT_AUTH_RATE" envDefault:"0.166667"` // 10 reqs per minute (~0.167 req/sec)
	RateLimitAuthBurst   int           `env:"RATE_LIMIT_AUTH_BURST" envDefault:"5"`

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

	// HTTP Server & Middleware Timeouts
	RequestTimeout          time.Duration `env:"REQUEST_TIMEOUT" envDefault:"30s"`
	ServerReadHeaderTimeout time.Duration `env:"SERVER_READ_HEADER_TIMEOUT" envDefault:"5s"`
	ServerReadTimeout       time.Duration `env:"SERVER_READ_TIMEOUT" envDefault:"10s"`
	ServerWriteTimeout      time.Duration `env:"SERVER_WRITE_TIMEOUT" envDefault:"10s"`
	ServerIdleTimeout       time.Duration `env:"SERVER_IDLE_TIMEOUT" envDefault:"60s"`
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
