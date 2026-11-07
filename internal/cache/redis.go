package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

var rateLimitLuaScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local memberId = ARGV[4]

local clearBefore = now - window

-- Remove expired entries older than the sliding window
redis.call('ZREMRANGEBYSCORE', key, '-inf', clearBefore)

-- Count current items in window
local current = redis.call('ZCARD', key)

if current < limit then
    redis.call('ZADD', key, now, memberId)
    redis.call('PEXPIRE', key, window)
    local remaining = limit - current - 1
    return {1, remaining, 0, window}
else
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local retryAfterMs = 1000
    if #oldest >= 2 then
        local oldestScore = tonumber(oldest[2])
        local waitMs = (oldestScore + window) - now
        if waitMs > 0 then
            retryAfterMs = waitMs
        end
    end
    return {0, 0, retryAfterMs, window}
end
`)

// RedisCache implements the Cache interface backed by Redis and singleflight.
type RedisCache struct {
	client       *redis.Client
	sfg          singleflight.Group
	rateLimitSeq uint64
}

// NewRedisCache initializes a Redis client from configuration and verifies connectivity.
func NewRedisCache(ctx context.Context, redisURL string, password string, db int) (*RedisCache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		opts = &redis.Options{
			Addr:     redisURL,
			Password: password,
			DB:       db,
		}
	} else {
		if password != "" {
			opts.Password = password
		}
		if db > 0 {
			opts.DB = db
		}
	}

	opts.PoolSize = 20
	opts.MinIdleConns = 5
	opts.DialTimeout = 5 * time.Second
	opts.ReadTimeout = 3 * time.Second
	opts.WriteTimeout = 3 * time.Second

	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisCache{
		client: client,
	}, nil
}

// Get fetches data for key and deserializes JSON into dest.
func (r *RedisCache) Get(ctx context.Context, key string, dest any) error {
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrCacheMiss
		}
		return err
	}
	return json.Unmarshal(data, dest)
}

// Set serializes value into JSON and stores it in Redis with the given TTL.
func (r *RedisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal cache value: %w", err)
	}
	return r.client.Set(ctx, key, data, ttl).Err()
}

// Delete removes one or more keys from Redis.
func (r *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return r.client.Del(ctx, keys...).Err()
}

// DeleteByPattern deletes all keys matching pattern using non-blocking SCAN.
func (r *RedisCache) DeleteByPattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, nextCursor, err := r.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			if err := r.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

// Remember looks up key in Redis; on cache miss, executes fetch protected by singleflight,
// caches the result with ttl, and unmarshals into dest.
func (r *RedisCache) Remember(ctx context.Context, key string, ttl time.Duration, dest any, fetch func(ctx context.Context) (any, error)) error {
	// Fast path: attempt cache read
	if err := r.Get(ctx, key, dest); err == nil {
		return nil
	}

	// Cache miss: coordinate single flight execution across concurrent goroutines
	val, err, _ := r.sfg.Do(key, func() (any, error) {
		// Double-check: another goroutine may have just populated the key
		data, readErr := r.client.Get(ctx, key).Bytes()
		if readErr == nil {
			return data, nil
		}

		fetched, fetchErr := fetch(ctx)
		if fetchErr != nil {
			return nil, fetchErr
		}

		serialized, marshalErr := json.Marshal(fetched)
		if marshalErr != nil {
			return nil, fmt.Errorf("failed to serialize cache payload: %w", marshalErr)
		}

		_ = r.client.Set(ctx, key, serialized, ttl).Err()
		return serialized, nil
	})

	if err != nil {
		return err
	}

	bytesData, ok := val.([]byte)
	if !ok {
		return errors.New("unexpected cache value type from singleflight")
	}

	return json.Unmarshal(bytesData, dest)
}

// RateLimit implements distributed sliding window rate limiting using Redis ZSET.
func (r *RedisCache) RateLimit(ctx context.Context, key string, limit int, window time.Duration) (RateLimitResult, error) {
	if limit <= 0 {
		return RateLimitResult{
			Allowed:    false,
			Remaining:  0,
			RetryAfter: window,
			ResetAfter: window,
		}, nil
	}

	if window <= 0 {
		window = time.Second
	}

	now := time.Now()
	nowMs := now.UnixMilli()
	windowMs := window.Milliseconds()
	if windowMs <= 0 {
		windowMs = 1
	}
	seq := atomic.AddUint64(&r.rateLimitSeq, 1)
	memberID := fmt.Sprintf("%d-%d", now.UnixNano(), seq)

	raw, err := rateLimitLuaScript.Run(ctx, r.client, []string{key}, nowMs, windowMs, limit, memberID).Slice()
	if err != nil {
		return RateLimitResult{}, fmt.Errorf("failed to execute rate limit script: %w", err)
	}

	if len(raw) < 4 {
		return RateLimitResult{}, fmt.Errorf("unexpected lua result length: %d", len(raw))
	}

	allowed := toInt64(raw[0]) == 1
	remaining := int(toInt64(raw[1]))
	retryAfterMs := toInt64(raw[2])
	resetAfterMs := toInt64(raw[3])

	return RateLimitResult{
		Allowed:    allowed,
		Remaining:  remaining,
		RetryAfter: time.Duration(retryAfterMs) * time.Millisecond,
		ResetAfter: time.Duration(resetAfterMs) * time.Millisecond,
	}, nil
}

func toInt64(val any) int64 {
	switch v := val.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	default:
		return 0
	}
}

// Ping verifies Redis connectivity.
func (r *RedisCache) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Close terminates connection pool.
func (r *RedisCache) Close() error {
	return r.client.Close()
}
