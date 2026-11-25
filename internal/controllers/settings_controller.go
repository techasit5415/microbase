package controllers

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/response"
)

// SettingsController manages runtime settings persisted in _settings (such as Redis URL).
type SettingsController struct {
	pool  *pgxpool.Pool
	cache *cache.SwitchableStore
}

func NewSettingsController(pool *pgxpool.Pool, c *cache.SwitchableStore) *SettingsController {
	return &SettingsController{pool: pool, cache: c}
}

// GetRedis handles GET /api/settings/redis.
func (h *SettingsController) GetRedis(c echo.Context) error {
	var redisURL string
	_ = h.pool.QueryRow(c.Request().Context(), `SELECT value FROM _settings WHERE key = 'redis_url'`).Scan(&redisURL)
	return response.OK(c, map[string]any{
		"redis_url": redisURL,
		"backend":   h.cache.Backend(),
	})
}

// UpdateRedis handles PUT /api/settings/redis — tests connection, hot-swaps CacheStore, and persists.
func (h *SettingsController) UpdateRedis(c echo.Context) error {
	var req struct {
		RedisURL string `json:"redis_url"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	url := strings.TrimSpace(req.RedisURL)

	backend, err := h.cache.Switch(c.Request().Context(), url)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "REDIS_CONNECT_FAILED", "failed to connect to redis: "+err.Error())
	}

	_, err = h.pool.Exec(c.Request().Context(), `
		INSERT INTO _settings (key, value, updated_at)
		VALUES ('redis_url', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		url,
	)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	return response.OK(c, map[string]any{
		"redis_url": url,
		"backend":   backend,
	})
}
