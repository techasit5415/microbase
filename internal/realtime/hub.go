package realtime

import (
	"errors"
	"sync"
	"time"
)

const (
	maxTotalSubscribers = 1000
	maxPerIPSubscribers = 20
)

var ErrTooManyConnections = errors.New("too many concurrent realtime connections")

// Event represents a realtime record mutation broadcasted to SSE subscribers.
type Event struct {
	Snapshot   any       `json:"-"`          // Internal pre-delete row for permission checks.
	Action     string    `json:"action"`     // "create", "update", or "delete"
	Collection string    `json:"collection"` // Collection name
	Record     any       `json:"record"`     // Record payload or deleted ID
	Timestamp  time.Time `json:"timestamp"`
}

type subscriberMeta struct {
	collection string
	ip         string
}

// Hub manages active SSE subscribers and broadcasts events without blocking writers.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[chan Event]subscriberMeta
	ipCounts    map[string]int
}

// NewHub constructs a Hub.
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[chan Event]subscriberMeta),
		ipCounts:    make(map[string]int),
	}
}

// Subscribe registers a client channel and returns an unsubscribe cleanup function.
func (h *Hub) Subscribe(collection string) (chan Event, func()) {
	ch, unsub, _ := h.SubscribeWithIP(collection, "")
	return ch, unsub
}

// SubscribeWithIP registers a client channel enforcing global and per-IP connection ceilings.
func (h *Hub) SubscribeWithIP(collection, ip string) (chan Event, func(), error) {
	h.mu.Lock()
	if len(h.subscribers) >= maxTotalSubscribers {
		h.mu.Unlock()
		return nil, nil, ErrTooManyConnections
	}
	if ip != "" && h.ipCounts[ip] >= maxPerIPSubscribers {
		h.mu.Unlock()
		return nil, nil, ErrTooManyConnections
	}
	ch := make(chan Event, 32)
	h.subscribers[ch] = subscriberMeta{collection: collection, ip: ip}
	if ip != "" {
		h.ipCounts[ip]++
	}
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		if meta, ok := h.subscribers[ch]; ok {
			delete(h.subscribers, ch)
			if meta.ip != "" {
				h.ipCounts[meta.ip]--
				if h.ipCounts[meta.ip] <= 0 {
					delete(h.ipCounts, meta.ip)
				}
			}
			close(ch)
		}
		h.mu.Unlock()
	}
	return ch, unsubscribe, nil
}

// Publish broadcasts an event to all matching subscribers (non-blocking).
func (h *Hub) Publish(evt Event) {
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch, meta := range h.subscribers {
		if meta.collection != "" && meta.collection != evt.Collection {
			continue
		}
		// Non-blocking send: drop if a slow client's buffer is full.
		select {
		case ch <- evt:
		default:
		}
	}
}
