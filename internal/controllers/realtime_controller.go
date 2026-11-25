package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/realtime"
	"github.com/techasit5415/microbase/internal/services"
)

// RealtimeController streams Server-Sent Events (SSE) to connected clients.
type RealtimeController struct {
	hub         *realtime.Hub
	records     *services.RecordService
	collections *services.CollectionService
}

// NewRealtimeController constructs a RealtimeController.
func NewRealtimeController(hub *realtime.Hub, records *services.RecordService, collections *services.CollectionService) *RealtimeController {
	return &RealtimeController{hub: hub, records: records, collections: collections}
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
			col, err := h.collections.GetByName(ctx, evt.Collection)
			if err != nil {
				continue
			}
			claims := middlewares.GetAuthClaims(c)
			var rec models.Record
			switch v := evt.Record.(type) {
			case models.Record:
				rec = v
			case map[string]any:
				rec = models.Record(v)
			}
			if rec == nil || rec["id"] == nil {
				continue
			}
			if evt.Action == "delete" {
				if !h.records.CanViewSnapshot(ctx, col, claims, evt.Snapshot) {
					continue
				}
			} else {
				rec, err = h.records.GetByID(ctx, evt.Collection, fmt.Sprint(rec["id"]), claims)
				if err != nil {
					continue
				}
			}
			evt.Record = services.VisibleRecord(col, claims, rec)
			payload, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Action, payload)
			flusher.Flush()
		}
	}
}
