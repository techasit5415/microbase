package realtime

import (
	"sync"
	"time"
)

// Event represents a realtime record mutation broadcasted to SSE subscribers.
type Event struct {
	Snapshot   any       `json:"-"`          // Internal pre-delete row for permission checks.
	Action     string    `json:"action"`     // "create", "update", or "delete"
	Collection string    `json:"collection"` // Collection name
	Record     any       `json:"record"`     // Record payload or deleted ID
	Timestamp  time.Time `json:"timestamp"`
}

// Hub manages active SSE subscribers and broadcasts events without blocking writers.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[chan Event]string // channel -> optional collection filter ("" = all)
}

// NewHub constructs a Hub.
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[chan Event]string),
	}
}

// Subscribe registers a client channel and returns an unsubscribe cleanup function.
func (h *Hub) Subscribe(collection string) (chan Event, func()) {
	ch := make(chan Event, 32)

	h.mu.Lock()
	h.subscribers[ch] = collection
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		if _, ok := h.subscribers[ch]; ok {
			delete(h.subscribers, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
	return ch, unsubscribe
}

// Publish broadcasts an event to all matching subscribers (non-blocking).
func (h *Hub) Publish(evt Event) {
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch, filter := range h.subscribers {
		if filter != "" && filter != evt.Collection {
			continue
		}
		// Non-blocking send: drop if a slow client's buffer is full.
		select {
		case ch <- evt:
		default:
		}
	}
}
