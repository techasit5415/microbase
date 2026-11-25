package cache

import (
	"context"
	"strconv"
	"sync"
	"time"
)

type memoryItem struct {
	value     string
	expiresAt time.Time
}

type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]memoryItem
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		items: make(map[string]memoryItem),
	}
}

func (m *MemoryStore) Get(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[key]
	if !ok {
		return "", ErrCacheMiss
	}
	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		delete(m.items, key)
		return "", ErrCacheMiss
	}
	return item.value, nil
}

func (m *MemoryStore) Set(_ context.Context, key string, value string, ttl time.Duration) error {
	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}

	m.mu.Lock()
	m.items[key] = memoryItem{value: value, expiresAt: exp}
	m.mu.Unlock()
	return nil
}

func (m *MemoryStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.items, key)
	m.mu.Unlock()
	return nil
}

func (m *MemoryStore) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int64
	var exp time.Time
	active := false
	if item, ok := m.items[key]; ok && (item.expiresAt.IsZero() || time.Now().Before(item.expiresAt)) {
		active = true
		exp = item.expiresAt
		if parsed, err := strconv.ParseInt(item.value, 10, 64); err == nil {
			count = parsed
		}
	}
	count++

	if !active && ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	m.items[key] = memoryItem{value: strconv.FormatInt(count, 10), expiresAt: exp}
	return count, nil
}

func (m *MemoryStore) Backend() string { return "memory" }
func (m *MemoryStore) Close() error    { return nil }
