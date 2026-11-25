package controllers

import (
	"time"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/config"
	"github.com/techasit5415/microbase/internal/response"
)

type HealthController struct {
	cache cache.CacheStore
}

func NewHealthController(c cache.CacheStore) *HealthController {
	return &HealthController{cache: c}
}

// GET /health
func (h *HealthController) Check(c echo.Context) error {
	hits, _ := h.cache.Incr(c.Request().Context(), "health:hits", time.Minute)

	return response.OK(c, map[string]any{
		"status":        "ok",
		"service":       "microbase",
		"version":       config.Version,
		"cache_backend": h.cache.Backend(),
		"health_hits":   hits,
	})
}
