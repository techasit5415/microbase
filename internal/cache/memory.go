package cache

import (
	"context"
	"strconv"
	"sync"
	"time"
)

const maxMemoryKeys = 100000

type memoryItem struct {
	value     string
	expiresAt time.Time
}

type MemoryStore struct {
	mu        sync.RWMutex
	items     map[string]memoryItem
	stopCh    chan struct{}
	closeOnce sync.Once
}

func NewMemoryStore() *MemoryStore {
	m := &MemoryStore{
		items:  make(map[string]memoryItem),
		stopCh: make(chan struct{}),
	}
	go m.evictLoop()
	return m
}

func (m *MemoryStore) evictLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case now := <-ticker.C:
			m.mu.Lock()
			for k, it := range m.items {
				if !it.expiresAt.IsZero() && now.After(it.expiresAt) {
					delete(m.items, k)
				}
			}
			m.mu.Unlock()
		}
	}
}

// evictIfFullLocked sweeps expired entries and, if still at capacity, evicts oldest-expiring entries to prevent OOM.
func (m *MemoryStore) evictIfFullLocked(now time.Time) {
	if len(m.items) < maxMemoryKeys {
		return
	}
	for k, it := range m.items {
		if !it.expiresAt.IsZero() && now.After(it.expiresAt) {
			delete(m.items, k)
		}
	}
	// If still at capacity, shed ~10% of entries so memory remains bounded under flood.
	if len(m.items) >= maxMemoryKeys {
		drop := maxMemoryKeys / 10
		for k := range m.items {
			delete(m.items, k)
			drop--
			if drop <= 0 {
				break
			}
		}
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
	now := time.Now()
	var exp time.Time
	if ttl > 0 {
		exp = now.Add(ttl)
	}

	m.mu.Lock()
	if _, exists := m.items[key]; !exists {
		m.evictIfFullLocked(now)
	}
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

func (m *MemoryStore) Flush(_ context.Context) error {
	m.mu.Lock()
	m.items = make(map[string]memoryItem)
	m.mu.Unlock()
	return nil
}

func (m *MemoryStore) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	var count int64
	var exp time.Time
	active := false
	if item, ok := m.items[key]; ok && (item.expiresAt.IsZero() || now.Before(item.expiresAt)) {
		active = true
		exp = item.expiresAt
		if parsed, err := strconv.ParseInt(item.value, 10, 64); err == nil {
			count = parsed
		}
	} else if !ok {
		m.evictIfFullLocked(now)
	}
	count++

	if !active && ttl > 0 {
		exp = now.Add(ttl)
	}
	m.items[key] = memoryItem{value: strconv.FormatInt(count, 10), expiresAt: exp}
	return count, nil
}

func (m *MemoryStore) Backend() string { return "memory" }
func (m *MemoryStore) Close() error {
	m.closeOnce.Do(func() {
		if m.stopCh != nil {
			close(m.stopCh)
		}
	})
	return nil
}
