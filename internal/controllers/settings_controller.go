package controllers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/storage"
)

// SettingsController manages runtime settings persisted in _settings (Redis URL, S3 Storage).
type SettingsController struct {
	pool    *pgxpool.Pool
	cache   *cache.SwitchableStore
	storage *storage.SwitchableStorage
}

func NewSettingsController(pool *pgxpool.Pool, c *cache.SwitchableStore, st *storage.SwitchableStorage) *SettingsController {
	return &SettingsController{pool: pool, cache: c, storage: st}
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

// GetS3 handles GET /api/settings/s3 (masks secret_key and reports active backend).
func (h *SettingsController) GetS3(c echo.Context) error {
	var raw string
	_ = h.pool.QueryRow(c.Request().Context(), `SELECT value FROM _settings WHERE key = 's3_config'`).Scan(&raw)
	var cfg storage.S3Config
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	return response.OK(c, map[string]any{
		"enabled":          cfg.Enabled,
		"endpoint":         cfg.Endpoint,
		"bucket":           cfg.Bucket,
		"region":           cfg.Region,
		"access_key":       cfg.AccessKey,
		"has_secret":       cfg.SecretKey != "",
		"force_path_style": cfg.ForcePathStyle,
		"backend":          h.storage.Backend(),
	})
}

// UpdateS3 handles PUT /api/settings/s3 — tests S3 bucket access, hot-swaps storage, and persists.
func (h *SettingsController) UpdateS3(c echo.Context) error {
	var req storage.S3Config
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}

	// Preserve existing secret_key if omitted on update.
	if strings.TrimSpace(req.SecretKey) == "" {
		var raw string
		_ = h.pool.QueryRow(c.Request().Context(), `SELECT value FROM _settings WHERE key = 's3_config'`).Scan(&raw)
		if raw != "" {
			var prev storage.S3Config
			if json.Unmarshal([]byte(raw), &prev) == nil {
				req.SecretKey = prev.SecretKey
			}
		}
	}

	backend, err := h.storage.Switch(c.Request().Context(), req)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "S3_CONNECT_FAILED", "S3 connection failed: "+err.Error())
	}

	encoded, _ := json.Marshal(req)
	_, err = h.pool.Exec(c.Request().Context(), `
		INSERT INTO _settings (key, value, updated_at)
		VALUES ('s3_config', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		string(encoded),
	)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	return response.OK(c, map[string]any{
		"enabled":          req.Enabled,
		"endpoint":         req.Endpoint,
		"bucket":           req.Bucket,
		"region":           req.Region,
		"access_key":       req.AccessKey,
		"has_secret":       req.SecretKey != "",
		"force_path_style": req.ForcePathStyle,
		"backend":          backend,
	})
}
