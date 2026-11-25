package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/realtime"
)

// RealtimeController streams Server-Sent Events (SSE) to connected clients.
type RealtimeController struct {
	hub *realtime.Hub
}

// NewRealtimeController constructs a RealtimeController.
func NewRealtimeController(hub *realtime.Hub) *RealtimeController {
	return &RealtimeController{hub: hub}
}

// Stream handles GET /api/realtime?collection=<optional>.
func (h *RealtimeController) Stream(c echo.Context) error {
	w := c.Response()
	flusher, ok := w.Writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming unsupported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	collection := c.QueryParam("collection")
	events, unsubscribe := h.hub.Subscribe(collection)
	defer unsubscribe()

	// Send initial connection event.
	fmt.Fprintf(w, "event: connected\ndata: {\"collection\":%q}\n\n", collection)
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	ctx := c.Request().Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case evt, open := <-events:
			if !open {
				return nil
			}
			payload, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Action, payload)
			flusher.Flush()
		}
	}
}
