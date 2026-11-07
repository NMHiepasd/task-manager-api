package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"task-manager-api/internal/cache"
	"task-manager-api/internal/domain"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter manages per-IP token bucket limiters with automatic eviction of stale entries.
type IPRateLimiter struct {
	mu          sync.Mutex
	clients     map[string]*clientLimiter
	limit       rate.Limit
	burst       int
	lastCleanup time.Time
	cleanupFreq time.Duration
	ttl         time.Duration
}

// NewIPRateLimiter creates an IP rate limiter.
func NewIPRateLimiter(r rate.Limit, burst int) *IPRateLimiter {
	return &IPRateLimiter{
		clients:     make(map[string]*clientLimiter),
		limit:       r,
		burst:       burst,
		lastCleanup: time.Now(),
		cleanupFreq: 5 * time.Minute,
		ttl:         15 * time.Minute,
	}
}

// getLimiter retrieves or creates a rate.Limiter for the given IP, lazily pruning stale entries.
func (rl *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Lazy cleanup of inactive IPs to avoid memory leaks
	if now.Sub(rl.lastCleanup) > rl.cleanupFreq {
		for k, v := range rl.clients {
			if now.Sub(v.lastSeen) > rl.ttl {
				delete(rl.clients, k)
			}
		}
		rl.lastCleanup = now
	}

	cl, exists := rl.clients[ip]
	if !exists {
		limiter := rate.NewLimiter(rl.limit, rl.burst)
		rl.clients[ip] = &clientLimiter{
			limiter:  limiter,
			lastSeen: now,
		}
		return limiter
	}

	cl.lastSeen = now
	return cl.limiter
}

// Middleware returns a Gin HandlerFunc enforcing the rate limit.
func (rl *IPRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if ip == "" {
			ip = "unknown"
		}

		limiter := rl.getLimiter(ip)
		if !limiter.Allow() {
			// Estimate retry-after in seconds based on fill rate
			retryAfter := 60
			if rl.limit > 0 {
				retryAfter = int(math.Ceil(1.0 / float64(rl.limit)))
				if retryAfter < 1 {
					retryAfter = 1
				}
			}

			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
			_ = c.Error(domain.ErrRateLimitExceeded)
			c.Abort()
			return
		}

		c.Next()
	}
}

// RateLimiter coordinates rate limiting using distributed Redis sliding-window counters
// and provides transparent fallback to an in-memory token bucket limiter whenever Redis
// is unavailable or operating in NoopCache mode.
type RateLimiter struct {
	cache     cache.Cache
	fallback  *IPRateLimiter
	keyPrefix string
	limit     int
	window    time.Duration
	logger    *slog.Logger
}

// NewRateLimiter creates a RateLimiter instance with distributed Redis support and in-memory fallback.
func NewRateLimiter(
	c cache.Cache,
	keyPrefix string,
	limit int,
	window time.Duration,
	fallbackRate rate.Limit,
	fallbackBurst int,
	logger *slog.Logger,
) *RateLimiter {
	var fallback *IPRateLimiter
	if fallbackRate > 0 && fallbackBurst > 0 {
		fallback = NewIPRateLimiter(fallbackRate, fallbackBurst)
	}

	return &RateLimiter{
		cache:     c,
		fallback:  fallback,
		keyPrefix: keyPrefix,
		limit:     limit,
		window:    window,
		logger:    logger,
	}
}

// Middleware returns a Gin HandlerFunc enforcing distributed rate limits with in-memory fallback.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if ip == "" {
			ip = "unknown"
		}

		// 1. Attempt distributed rate limiting via Redis if available
		if rl.cache != nil && rl.limit > 0 && rl.window > 0 {
			redisKey := fmt.Sprintf("ratelimit:%s:%s", rl.keyPrefix, ip)
			res, err := rl.cache.RateLimit(c.Request.Context(), redisKey, rl.limit, rl.window)
			if err == nil {
				// Set standard RFC-compliant rate limit headers
				c.Header("X-RateLimit-Limit", strconv.Itoa(rl.limit))
				c.Header("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
				resetUnix := time.Now().Add(res.ResetAfter).Unix()
				c.Header("X-RateLimit-Reset", strconv.FormatInt(resetUnix, 10))

				if !res.Allowed {
					retryAfterSec := int(math.Ceil(res.RetryAfter.Seconds()))
					if retryAfterSec < 1 {
						retryAfterSec = 1
					}
					c.Header("Retry-After", strconv.Itoa(retryAfterSec))
					_ = c.Error(domain.ErrRateLimitExceeded)
					c.Abort()
					return
				}

				c.Next()
				return
			}

			// If error is not merely ErrNoopCache, log the Redis failure as a warning
			if rl.logger != nil && !errors.Is(err, cache.ErrNoopCache) {
				rl.logger.Warn("Redis rate limiter failed, falling back to in-memory limiter",
					slog.String("key", redisKey),
					slog.Any("error", err),
				)
			}
		}

		// 2. Fallback to in-memory token bucket limiter
		if rl.fallback != nil {
			limiter := rl.fallback.getLimiter(ip)
			if !limiter.Allow() {
				retryAfter := 60
				if rl.fallback.limit > 0 {
					retryAfter = int(math.Ceil(1.0 / float64(rl.fallback.limit)))
					if retryAfter < 1 {
						retryAfter = 1
					}
				}

				if rl.limit > 0 {
					c.Header("X-RateLimit-Limit", strconv.Itoa(rl.limit))
					c.Header("X-RateLimit-Remaining", "0")
				}
				c.Header("Retry-After", strconv.Itoa(retryAfter))
				_ = c.Error(domain.ErrRateLimitExceeded)
				c.Abort()
				return
			}

			if rl.limit > 0 {
				c.Header("X-RateLimit-Limit", strconv.Itoa(rl.limit))
			}
		}

		c.Next()
	}
}
