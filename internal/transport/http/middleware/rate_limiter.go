package middleware

import (
	"fmt"
	"math"
	"sync"
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
