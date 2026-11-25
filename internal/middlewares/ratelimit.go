package middlewares

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/response"
)

// RateLimitRule defines a per-prefix rate limit policy configurable from Settings.
type RateLimitRule struct {
	Label       string `json:"label"`        // Path prefix e.g. "/api/auth/login" or "/api/"
	MaxRequests int    `json:"max_requests"` // Max requests per DurationSec window
	DurationSec int    `json:"duration_sec"` // Window size in seconds
}

var (
	rlMu    sync.RWMutex
	rlRules = []RateLimitRule{
		{Label: "/api/auth/login", MaxRequests: 20, DurationSec: 60},
		{Label: "/api/superusers/login", MaxRequests: 15, DurationSec: 60},
		{Label: "/api/", MaxRequests: 120, DurationSec: 60},
	}
)

// GetRateLimitRules returns the currently active per-route rate limit policies.
func GetRateLimitRules() []RateLimitRule {
	rlMu.RLock()
	defer rlMu.RUnlock()
	out := make([]RateLimitRule, len(rlRules))
	copy(out, rlRules)
	return out
}

// SetRateLimitRules hot-swaps the active per-route rate limit policies at runtime.
func SetRateLimitRules(rules []RateLimitRule) {
	rlMu.Lock()
	rlRules = rules
	rlMu.Unlock()
}

// RateLimit enforces per-IP, per-route-prefix request limits backed by CacheStore.
func RateLimit(store cache.CacheStore, defaultLimit int, defaultWindow time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			ip := c.RealIP()

			limit := defaultLimit
			window := defaultWindow
			matchedLabel := "default"

			rlMu.RLock()
			bestLen := -1
			for _, r := range rlRules {
				if r.Label != "" && strings.HasPrefix(path, r.Label) && len(r.Label) > bestLen {
					bestLen = len(r.Label)
					if r.MaxRequests > 0 {
						limit = r.MaxRequests
					}
					if r.DurationSec > 0 {
						window = time.Duration(r.DurationSec) * time.Second
					}
					matchedLabel = r.Label
				}
			}
			rlMu.RUnlock()

			key := fmt.Sprintf("rl:%s:%s", matchedLabel, ip)
			count, err := store.Incr(c.Request().Context(), key, window)
			if err == nil && int(count) > limit {
				return response.Error(c, http.StatusTooManyRequests, "RATE_LIMITED",
					fmt.Sprintf("too many requests on %s (limit %d per %ds)", matchedLabel, limit, int(window.Seconds())))
			}
			return next(c)
		}
	}
}
