package controllers

import (
	"encoding/json"
	"net/http"
	"net/url"
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

func maskRedisURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.User == nil {
		return raw, false
	}
	if _, hasPass := u.User.Password(); !hasPass {
		return raw, false
	}
	u.User = url.UserPassword(u.User.Username(), "REDACTED")
	return u.String(), true
}

// GetRedis handles GET /api/settings/redis (masks Redis password).
func (h *SettingsController) GetRedis(c echo.Context) error {
	var redisURL string
	_ = h.pool.QueryRow(c.Request().Context(), `SELECT value FROM _settings WHERE key = 'redis_url'`).Scan(&redisURL)
	masked, hasPass := maskRedisURL(redisURL)
	return response.OK(c, map[string]any{
		"redis_url":    masked,
		"has_password": hasPass,
		"backend":      h.cache.Backend(),
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
	rawURL := strings.TrimSpace(req.RedisURL)
	if parsed, err := url.Parse(rawURL); err == nil && parsed.User != nil {
		if pw, ok := parsed.User.Password(); ok && (pw == "REDACTED" || pw == "********") {
			var prevRaw string
			_ = h.pool.QueryRow(c.Request().Context(), `SELECT value FROM _settings WHERE key = 'redis_url'`).Scan(&prevRaw)
			if prevU, pErr := url.Parse(strings.TrimSpace(prevRaw)); pErr == nil && prevU.User != nil && strings.EqualFold(parsed.Host, prevU.Host) {
				if prevPW, ok2 := prevU.User.Password(); ok2 {
					parsed.User = url.UserPassword(parsed.User.Username(), prevPW)
					rawURL = parsed.String()
				}
			}
		}
	}

	backend, err := h.cache.Switch(c.Request().Context(), rawURL)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "REDIS_CONNECT_FAILED", "failed to connect to redis: "+err.Error())
	}

	_, err = h.pool.Exec(c.Request().Context(), `
		INSERT INTO _settings (key, value, updated_at)
		VALUES ('redis_url', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		rawURL,
	)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	masked, hasPass := maskRedisURL(rawURL)
	return response.OK(c, map[string]any{
		"redis_url":    masked,
		"has_password": hasPass,
		"backend":      backend,
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

	// Preserve existing secret_key only if endpoint and access_key did not change (prevents secret exfiltration to a new endpoint).
	if strings.TrimSpace(req.SecretKey) == "" {
		var raw string
		_ = h.pool.QueryRow(c.Request().Context(), `SELECT value FROM _settings WHERE key = 's3_config'`).Scan(&raw)
		if raw != "" {
			var prev storage.S3Config
			if json.Unmarshal([]byte(raw), &prev) == nil &&
				strings.EqualFold(strings.TrimRight(strings.TrimSpace(req.Endpoint), "/"), strings.TrimRight(strings.TrimSpace(prev.Endpoint), "/")) &&
				strings.TrimSpace(req.AccessKey) == strings.TrimSpace(prev.AccessKey) {
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
