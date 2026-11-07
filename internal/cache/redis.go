package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// RedisCache implements the Cache interface backed by Redis and singleflight.
type RedisCache struct {
	client *redis.Client
	sfg    singleflight.Group
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

// Ping verifies Redis connectivity.
func (r *RedisCache) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Close terminates connection pool.
func (r *RedisCache) Close() error {
	return r.client.Close()
}
