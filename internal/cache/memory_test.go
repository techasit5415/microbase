package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMemoryLifecycle(t *testing.T) {
	c := context.Background()
	s := NewMemoryStore()
	defer s.Close()
	if _, err := s.Get(c, "missing"); !errors.Is(err, ErrCacheMiss) {
		t.Fatal(err)
	}
	s.Set(c, "key", "value", 0)
	if v, err := s.Get(c, "key"); err != nil || v != "value" {
		t.Fatal(v, err)
	}
	s.Delete(c, "key")
	if _, err := s.Get(c, "key"); !errors.Is(err, ErrCacheMiss) {
		t.Fatal(err)
	}
	s.items["old"] = memoryItem{value: "x", expiresAt: time.Now().Add(-time.Second)}
	if _, err := s.Get(c, "old"); !errors.Is(err, ErrCacheMiss) {
		t.Fatal("expired item returned")
	}
}
func TestCounterPreservesFixedWindow(t *testing.T) {
	c := context.Background()
	s := NewMemoryStore()
	s.Incr(c, "key", time.Minute)
	deadline := s.items["key"].expiresAt
	if n, err := s.Incr(c, "key", time.Minute); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if !s.items["key"].expiresAt.Equal(deadline) {
		t.Fatal("counter moved its expiry; continuous requests never reset")
	}
}
func TestConcurrentCounter(t *testing.T) {
	s := NewMemoryStore()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Go(func() { s.Incr(context.Background(), "key", time.Minute) })
	}
	wg.Wait()
	if n, _ := s.Get(context.Background(), "key"); n != "50" {
		t.Fatal(n)
	}
}
func TestSwitchToMemory(t *testing.T) {
	s := NewSwitchableStore(NewMemoryStore())
	c := context.Background()
	s.Set(c, "old", "value", 0)
	if b, err := s.Switch(c, ""); err != nil || b != "memory" {
		t.Fatal(b, err)
	}
	if _, err := s.Get(c, "old"); !errors.Is(err, ErrCacheMiss) {
		t.Fatal(err)
	}
}
