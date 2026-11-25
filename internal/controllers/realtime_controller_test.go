package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/realtime"
	"github.com/techasit5415/microbase/internal/services"
	"github.com/techasit5415/microbase/internal/testutil"
)

func TestRealtimeStreamSSE(t *testing.T) {
	pool := testutil.Database(t)
	sw := cache.NewSwitchableStore(cache.NewMemoryStore())
	colSvc := services.NewCollectionService(pool, sw)
	hub := realtime.NewHub()
	recSvc := services.NewRecordService(pool, colSvc, hub, nil)
	ctrl := NewRealtimeController(hub, recSvc, colSvc)

	_, err := colSvc.Create(context.Background(), models.CreateCollectionRequest{
		Name:     "sse_posts",
		Type:     "base",
		IDType:   "text",
		Fields:   []models.FieldDef{{Name: "title", Type: "text"}},
		APIRules: map[string]string{"list": "", "view": "", "create": ""},
	})
	if err != nil {
		t.Fatalf("create sse_posts: %v", err)
	}

	e := echo.New()
	e.GET("/api/realtime", ctrl.Stream)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/realtime?collection=sse_posts", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		e.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(40 * time.Millisecond)
	if _, err := recSvc.Create(context.Background(), "sse_posts", models.Record{"title": "Hello SSE"}, nil); err != nil {
		t.Fatalf("create sse_posts record: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "Hello SSE") || !strings.Contains(body, "data: ") {
		t.Fatalf("expected SSE event in stream body, got: %q", body)
	}
}
