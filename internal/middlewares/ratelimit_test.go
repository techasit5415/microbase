package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
)

func TestRateLimitToggleAndRules(t *testing.T) {
	orig := GetRateLimitConfig()
	defer SetRateLimitConfig(orig)

	store := cache.NewMemoryStore()
	e := echo.New()
	mw := RateLimit(store, 100, time.Minute)
	okHandler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// 1. Enabled with limit=2: 3rd request should return 429.
	SetRateLimitConfig(RateLimitConfig{
		Enabled: true,
		Rules: []RateLimitRule{
			{Label: "/api/", MaxRequests: 2, DurationSec: 60},
		},
	})
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/collections", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if err := okHandler(c); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := http.StatusOK
		if i == 3 {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("req %d: got %d want %d", i, rec.Code, want)
		}
	}

	// 2. /api/settings/ratelimits is always exempt so admin can disable rate limiting.
	req := httptest.NewRequest(http.MethodGet, "/api/settings/ratelimits", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	if err := okHandler(e.NewContext(req, rec)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/settings/ratelimits should be exempt, got %d", rec.Code)
	}

	// 3. Disabled: bypasses rate limiting completely.
	SetRateLimitEnabled(false)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/collections", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		if err := okHandler(e.NewContext(req, rec)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("disabled req %d: got %d want 200", i, rec.Code)
		}
	}

	// 4. Enabled with no matching rules: no hidden hardcoded fallback limit.
	SetRateLimitConfig(RateLimitConfig{Enabled: true, Rules: []RateLimitRule{}})
	req = httptest.NewRequest(http.MethodGet, "/api/collections", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	rec = httptest.NewRecorder()
	if err := okHandler(e.NewContext(req, rec)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("empty rules should allow request, got %d", rec.Code)
	}
}
