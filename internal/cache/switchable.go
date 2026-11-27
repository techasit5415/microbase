package cache

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// SwitchableStore wraps a CacheStore and allows hot-swapping between In-Memory and Redis at runtime.
type SwitchableStore struct {
	mu    sync.RWMutex
	inner CacheStore
}

func NewSwitchableStore(initial CacheStore) *SwitchableStore {
	return &SwitchableStore{inner: initial}
}

// Switch connects to redisURL (or switches back to In-Memory if empty) and closes the previous store.
func (s *SwitchableStore) Switch(ctx context.Context, redisURL string) (string, error) {
	var next CacheStore
	if redisURL == "" {
		next = NewMemoryStore()
	} else {
		rs, err := NewRedisStore(ctx, redisURL)
		if err != nil {
			return "", err
		}
		next = rs
	}

	s.mu.Lock()
	old := s.inner
	s.inner = next
	s.mu.Unlock()

	if old != nil {
		_ = old.Close()
	}
	slog.Info("cache backend switched", "backend", next.Backend())
	return next.Backend(), nil
}

func (s *SwitchableStore) current() CacheStore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inner
}

func (s *SwitchableStore) Get(ctx context.Context, key string) (string, error) {
	return s.current().Get(ctx, key)
}
func (s *SwitchableStore) Set(ctx context.Context, key, val string, ttl time.Duration) error {
	return s.current().Set(ctx, key, val, ttl)
}
func (s *SwitchableStore) Delete(ctx context.Context, key string) error {
	return s.current().Delete(ctx, key)
}
func (s *SwitchableStore) Flush(ctx context.Context) error {
	return s.current().Flush(ctx)
}
func (s *SwitchableStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	return s.current().Incr(ctx, key, ttl)
}
func (s *SwitchableStore) Backend() string { return s.current().Backend() }
func (s *SwitchableStore) Close() error    { return s.current().Close() }
