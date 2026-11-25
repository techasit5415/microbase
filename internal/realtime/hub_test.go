package realtime

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestHubFiltersAndUnsubscribe(t *testing.T) {
	h := NewHub()
	all, stop := h.Subscribe("")
	defer stop()
	posts, off := h.Subscribe("posts")
	h.Publish(Event{Action: "create", Collection: "other"})
	if e := <-all; e.Timestamp.IsZero() {
		t.Fatal("missing timestamp")
	}
	select {
	case <-posts:
		t.Fatal("wrong collection")
	default:
	}
	off()
	off()
	if _, open := <-posts; open {
		t.Fatal("channel not closed")
	}
}
func TestHubConcurrentPublishAndUnsubscribe(t *testing.T) {
	h := NewHub()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Go(func() { _, off := h.Subscribe("posts"); h.Publish(Event{Collection: "posts"}); off() })
	}
	wg.Wait()
}
func TestDeleteSnapshotNeverSerialized(t *testing.T) {
	b, err := json.Marshal(Event{Action: "delete", Record: map[string]any{"id": 1}, Snapshot: map[string]any{"secret": "sensitive"}})
	if err != nil || strings.Contains(string(b), "sensitive") {
		t.Fatal(string(b), err)
	}
}
