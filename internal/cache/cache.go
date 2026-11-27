package cache

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

var ErrCacheMiss = errors.New("cache miss")

// CacheStore คือ Interface กลางที่รองรับทั้ง In-Memory และ Redis
type CacheStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Flush(ctx context.Context) error
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Backend() string
	Close() error
}

// NewStore เลือกใช้ Redis อัตโนมัติถ้ามี redisURL มิฉะนั้นใช้ In-Memory
func NewStore(ctx context.Context, redisURL string) CacheStore {
	if redisURL != "" {
		store, err := NewRedisStore(ctx, redisURL)
		if err == nil {
			slog.Info("cache initialized", "backend", "redis")
			return store
		}
		slog.Warn("redis unavailable, falling back to in-memory cache", "error", err)
	}

	slog.Info("cache initialized", "backend", "memory")
	return NewMemoryStore()
}
