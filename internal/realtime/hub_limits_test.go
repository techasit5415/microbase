package realtime

import (
	"errors"
	"testing"
)

func TestHubEnforcesPerIPConnectionLimit(t *testing.T) {
	hub := NewHub()
	unsubs := make([]func(), 0, maxPerIPSubscribers)
	for i := 0; i < maxPerIPSubscribers; i++ {
		_, unsub, err := hub.SubscribeWithIP("posts", "198.51.100.10")
		if err != nil {
			t.Fatalf("subscription %d unexpectedly failed: %v", i+1, err)
		}
		unsubs = append(unsubs, unsub)
	}

	if _, _, err := hub.SubscribeWithIP("posts", "198.51.100.10"); !errors.Is(err, ErrTooManyConnections) {
		t.Fatalf("expected ErrTooManyConnections on limit+1, got %v", err)
	}

	// Different IP is still allowed.
	_, otherUnsub, err := hub.SubscribeWithIP("posts", "198.51.100.11")
	if err != nil {
		t.Fatalf("expected different IP to succeed, got %v", err)
	}
	otherUnsub()

	// Unsubscribing frees slot for the original IP.
	unsubs[0]()
	_, retryUnsub, err := hub.SubscribeWithIP("posts", "198.51.100.10")
	if err != nil {
		t.Fatalf("expected slot freed after unsubscribe, got %v", err)
	}
	retryUnsub()
	for _, u := range unsubs[1:] {
		u()
	}
}
