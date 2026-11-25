package middlewares

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/response"
)

// RateLimit enforces a per-IP request limit backed by CacheStore (In-Memory or Redis).
func RateLimit(store cache.CacheStore, maxRequests int64, window time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := fmt.Sprintf("rl:%s", c.RealIP())
			count, err := store.Incr(c.Request().Context(), key, window)
			if err != nil {
				// Fail open on transient cache errors so legitimate traffic is not blocked.
				return next(c)
			}

			remaining := maxRequests - count
			if remaining < 0 {
				remaining = 0
			}
			c.Response().Header().Set("X-RateLimit-Limit", strconv.FormatInt(maxRequests, 10))
			c.Response().Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

			if count > maxRequests {
				return response.Error(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests, please try again later")
			}
			return next(c)
		}
	}
}
