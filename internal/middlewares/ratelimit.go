package middlewares

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

// RateLimitConfig holds the runtime rate limiting toggle and per-route rules.
type RateLimitConfig struct {
	Enabled bool            `json:"enabled"`
	Rules   []RateLimitRule `json:"rules"`
}

var (
	rlMu      sync.RWMutex
	rlEnabled = true
	rlRules   = []RateLimitRule{
		{Label: "/api/auth/login", MaxRequests: 20, DurationSec: 60},
		{Label: "/api/superusers/login", MaxRequests: 15, DurationSec: 60},
		{Label: "/api/", MaxRequests: 120, DurationSec: 60},
	}
)

// GetRateLimitConfig returns the currently active rate limit toggle and rules.
func GetRateLimitConfig() RateLimitConfig {
	rlMu.RLock()
	defer rlMu.RUnlock()
	out := make([]RateLimitRule, len(rlRules))
	copy(out, rlRules)
	return RateLimitConfig{
		Enabled: rlEnabled,
		Rules:   out,
	}
}

// SetRateLimitConfig hot-swaps the active rate limit enabled flag and rules at runtime.
func SetRateLimitConfig(cfg RateLimitConfig) {
	rlMu.Lock()
	rlEnabled = cfg.Enabled
	if cfg.Rules == nil {
		rlRules = []RateLimitRule{}
	} else {
		out := make([]RateLimitRule, len(cfg.Rules))
		copy(out, cfg.Rules)
		rlRules = out
	}
	rlMu.Unlock()
}

// IsRateLimitEnabled reports whether rate limiting is currently enabled.
func IsRateLimitEnabled() bool {
	rlMu.RLock()
	defer rlMu.RUnlock()
	return rlEnabled
}

// SetRateLimitEnabled toggles rate limiting at runtime without altering rules.
func SetRateLimitEnabled(enabled bool) {
	rlMu.Lock()
	rlEnabled = enabled
	rlMu.Unlock()
}

// GetRateLimitRules returns the currently active per-route rate limit policies.
func GetRateLimitRules() []RateLimitRule {
	return GetRateLimitConfig().Rules
}

// SetRateLimitRules hot-swaps the active per-route rate limit policies at runtime.
func SetRateLimitRules(rules []RateLimitRule) {
	rlMu.Lock()
	if rules == nil {
		rlRules = []RateLimitRule{}
	} else {
		out := make([]RateLimitRule, len(rules))
		copy(out, rules)
		rlRules = out
	}
	rlMu.Unlock()
}

// LoadRateLimitConfigFromDB loads persisted rate limit settings from _settings if present.
// Supports both the current object format {"enabled":bool,"rules":[...]} and the legacy array format [...].
func LoadRateLimitConfigFromDB(ctx context.Context, pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	var raw string
	if err := pool.QueryRow(ctx, `SELECT value FROM _settings WHERE key = 'rate_limit_rules'`).Scan(&raw); err != nil {
		return
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	if strings.HasPrefix(raw, "{") {
		var cfg RateLimitConfig
		if json.Unmarshal([]byte(raw), &cfg) == nil {
			SetRateLimitConfig(cfg)
		}
		return
	}
	var legacy []RateLimitRule
	if json.Unmarshal([]byte(raw), &legacy) == nil {
		SetRateLimitRules(legacy)
	}
}

// RateLimit enforces per-IP, per-route-prefix request limits backed by CacheStore.
func RateLimit(store cache.CacheStore, defaultLimit int, defaultWindow time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			// Never lock superusers out of viewing or disabling rate limits.
			if path == "/api/settings/ratelimits" {
				return next(c)
			}

			rlMu.RLock()
			enabled := rlEnabled
			if !enabled {
				rlMu.RUnlock()
				return next(c)
			}

			limit := defaultLimit
			window := defaultWindow
			matchedLabel := ""
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

			// If no rule matches the request path, do not enforce a hidden fallback limit.
			if bestLen == -1 || limit <= 0 || window <= 0 {
				return next(c)
			}

			ip := c.RealIP()
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
