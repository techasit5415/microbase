package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/config"
)

func TestHealthEndpointIncludesVersion(t *testing.T) {
	e := echo.New()
	store := cache.NewSwitchableStore(cache.NewMemoryStore())
	cfg := &config.Config{JWTSecret: "e2e-test-secret"}

	Register(e, nil, store, cfg)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /health, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json from /health: %v", err)
	}
	data := body
	if nested, ok := body["data"].(map[string]any); ok {
		data = nested
	}
	if data["status"] != "ok" || data["version"] == "" {
		t.Fatalf("expected status=ok and non-empty version, got %v", data)
	}
}
