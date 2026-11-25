package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/response"
)

type MailConfig struct {
	Enabled       bool   `json:"enabled"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	SenderName    string `json:"sender_name"`
	SenderAddress string `json:"sender_address"`
}

// OpsController handles PocketBase-style SMTP Mail, Backups, and Cron jobs.
type OpsController struct {
	pool       *pgxpool.Pool
	backupDir  string
	databaseURL string
}

func NewOpsController(pool *pgxpool.Pool, backupDir string) *OpsController {
	_ = os.MkdirAll(backupDir, 0o755)
	return &OpsController{
		pool:        pool,
		backupDir:   backupDir,
		databaseURL: os.Getenv("DATABASE_URL"),
	}
}

func (h *OpsController) loadMailConfig(ctx context.Context) MailConfig {
	var raw string
	_ = h.pool.QueryRow(ctx, `SELECT value FROM _settings WHERE key = 'mail_config'`).Scan(&raw)
	var cfg MailConfig
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	return cfg
}

// GetMail handles GET /api/settings/mail.
func (h *OpsController) GetMail(c echo.Context) error {
	cfg := h.loadMailConfig(c.Request().Context())
	return response.OK(c, map[string]any{
		"enabled":        cfg.Enabled,
		"host":           cfg.Host,
		"port":           cfg.Port,
		"username":       cfg.Username,
		"has_password":   cfg.Password != "",
		"sender_name":    cfg.SenderName,
		"sender_address": cfg.SenderAddress,
	})
}

// UpdateMail handles PUT /api/settings/mail.
func (h *OpsController) UpdateMail(c echo.Context) error {
	var req MailConfig
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	if req.Port == 0 {
		req.Port = 587
	}
	if strings.TrimSpace(req.Password) == "" {
		prev := h.loadMailConfig(c.Request().Context())
		req.Password = prev.Password
	}
	encoded, _ := json.Marshal(req)
	_, err := h.pool.Exec(c.Request().Context(), `
		INSERT INTO _settings (key, value, updated_at)
		VALUES ('mail_config', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		string(encoded),
	)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{"saved": true, "has_password": req.Password != ""})
}

// TestMail handles POST /api/settings/mail/test.
func (h *OpsController) TestMail(c echo.Context) error {
	var req struct {
		To string `json:"to"`
	}
	if err := c.Bind(&req); err != nil || !strings.Contains(req.To, "@") {
		return response.Error(c, http.StatusBadRequest, "INVALID_EMAIL", "valid recipient email is required")
	}
	cfg := h.loadMailConfig(c.Request().Context())
	if !cfg.Enabled || cfg.Host == "" {
		return response.Error(c, http.StatusBadRequest, "SMTP_DISABLED", "SMTP mail is not enabled or host is empty")
	}
	from := cfg.SenderAddress
	if from == "" {
		from = cfg.Username
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	msg := fmt.Sprintf("From: %s <%s>\r\nTo: %s\r\nSubject: Microbase SMTP Test\r\n\r\nHello from Microbase! Your SMTP configuration is working.\r\n",
		cfg.SenderName, from, req.To)
	if err := smtp.SendMail(addr, auth, from, []string{req.To}, []byte(msg)); err != nil {
		return response.Error(c, http.StatusBadGateway, "SMTP_FAILED", err.Error())
	}
	return response.OK(c, map[string]any{"sent_to": req.To})
}

// ListBackups handles GET /api/backups.
func (h *OpsController) ListBackups(c echo.Context) error {
	entries, err := os.ReadDir(h.backupDir)
	if err != nil {
		return response.OK(c, map[string]any{"items": []any{}})
	}
	items := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, map[string]any{
			"key":      e.Name(),
			"size":     info.Size(),
			"modified": info.ModTime().UTC(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i]["key"].(string) > items[j]["key"].(string)
	})
	return response.OK(c, map[string]any{"items": items})
}

// CreateBackup handles POST /api/backups using pg_dump.
func (h *OpsController) CreateBackup(c echo.Context) error {
	name := fmt.Sprintf("microbase_%s.sql", time.Now().Format("20060102_150405"))
	outPath := filepath.Join(h.backupDir, name)
	dbURL := h.databaseURL
	if dbURL == "" {
		return response.Error(c, http.StatusBadRequest, "NO_DATABASE", "DATABASE_URL is not configured")
	}
	cmd := exec.CommandContext(c.Request().Context(), "pg_dump", dbURL, "-f", outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return response.Error(c, http.StatusInternalServerError, "BACKUP_FAILED", fmt.Sprintf("pg_dump failed: %v (%s)", err, strings.TrimSpace(string(out))))
	}
	info, _ := os.Stat(outPath)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}
	return response.Created(c, map[string]any{"key": name, "size": size})
}

// DownloadBackup handles GET /api/backups/:name.
func (h *OpsController) DownloadBackup(c echo.Context) error {
	safe := filepath.Base(c.Param("name"))
	full := filepath.Join(h.backupDir, safe)
	if _, err := os.Stat(full); os.IsNotExist(err) {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "backup not found")
	}
	return c.Attachment(full, safe)
}

// DeleteBackup handles DELETE /api/backups/:name.
func (h *OpsController) DeleteBackup(c echo.Context) error {
	safe := filepath.Base(c.Param("name"))
	full := filepath.Join(h.backupDir, safe)
	if err := os.Remove(full); err != nil {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "backup not found")
	}
	return response.OK(c, map[string]any{"deleted": safe})
}

// ListCrons handles GET /api/crons.
func (h *OpsController) ListCrons(c echo.Context) error {
	return response.OK(c, map[string]any{
		"items": []map[string]string{
			{"id": "__mbLogsPrune__", "expression": "0 3 * * *", "description": "Prune _logs older than 7 days (keep latest 5,000 rows)"},
			{"id": "__mbAutoBackup__", "expression": "0 2 * * *", "description": "Create PostgreSQL SQL snapshot via pg_dump"},
		},
	})
}

// RunCron handles POST /api/crons/:id/run.
func (h *OpsController) RunCron(c echo.Context) error {
	id := c.Param("id")
	switch id {
	case "__mbLogsPrune__":
		tag, err := h.pool.Exec(c.Request().Context(), `DELETE FROM _logs WHERE created_at < NOW() - INTERVAL '7 days'`)
		if err != nil {
			return response.Error(c, http.StatusInternalServerError, "CRON_FAILED", err.Error())
		}
		return response.OK(c, map[string]any{"id": id, "pruned": tag.RowsAffected()})
	case "__mbAutoBackup__":
		return h.CreateBackup(c)
	default:
		return response.Error(c, http.StatusNotFound, "CRON_NOT_FOUND", "cron job not found")
	}
}

// UploadBackup handles POST /api/backups/upload (multipart field "file").
func (h *OpsController) UploadBackup(c echo.Context) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "MISSING_FILE", "form field 'file' is required")
	}
	safe := filepath.Base(fh.Filename)
	if !strings.HasSuffix(strings.ToLower(safe), ".sql") {
		return response.Error(c, http.StatusBadRequest, "INVALID_FILE", "only .sql backup files are supported")
	}
	src, err := fh.Open()
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, 100<<20))
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	if err := os.WriteFile(filepath.Join(h.backupDir, safe), data, 0o644); err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.Created(c, map[string]any{"key": safe, "size": len(data)})
}

// RestoreBackup handles POST /api/backups/:name/restore using psql.
func (h *OpsController) RestoreBackup(c echo.Context) error {
	safe := filepath.Base(c.Param("name"))
	full := filepath.Join(h.backupDir, safe)
	if _, err := os.Stat(full); os.IsNotExist(err) {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "backup not found")
	}
	if h.databaseURL == "" {
		return response.Error(c, http.StatusBadRequest, "NO_DATABASE", "DATABASE_URL is not configured")
	}
	cmd := exec.CommandContext(c.Request().Context(), "psql", h.databaseURL, "-f", full)
	if out, err := cmd.CombinedOutput(); err != nil {
		return response.Error(c, http.StatusInternalServerError, "RESTORE_FAILED", fmt.Sprintf("psql restore failed: %v (%s)", err, strings.TrimSpace(string(out))))
	}
	return response.OK(c, map[string]any{"restored": safe})
}

// GetRateLimits handles GET /api/settings/ratelimits.
func (h *OpsController) GetRateLimits(c echo.Context) error {
	var raw string
	_ = h.pool.QueryRow(c.Request().Context(), `SELECT value FROM _settings WHERE key = 'rate_limit_rules'`).Scan(&raw)
	if raw != "" {
		var saved []middlewares.RateLimitRule
		if json.Unmarshal([]byte(raw), &saved) == nil {
			middlewares.SetRateLimitRules(saved)
		}
	}
	return response.OK(c, map[string]any{"rules": middlewares.GetRateLimitRules()})
}

// UpdateRateLimits handles PUT /api/settings/ratelimits and hot-swaps rules live.
func (h *OpsController) UpdateRateLimits(c echo.Context) error {
	var req struct {
		Rules []middlewares.RateLimitRule `json:"rules"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	middlewares.SetRateLimitRules(req.Rules)
	encoded, _ := json.Marshal(req.Rules)
	_, err := h.pool.Exec(c.Request().Context(), `
		INSERT INTO _settings (key, value, updated_at)
		VALUES ('rate_limit_rules', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		string(encoded),
	)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{"rules": req.Rules})
}

// GetLogSettings handles GET /api/settings/logs.
func (h *OpsController) GetLogSettings(c echo.Context) error {
	var raw string
	_ = h.pool.QueryRow(c.Request().Context(), `SELECT value FROM _settings WHERE key = 'logs_max_days'`).Scan(&raw)
	maxDays := 7
	if raw != "" {
		var n int
		if _, err := fmt.Sscanf(raw, "%d", &n); err == nil && n >= 1 {
			maxDays = n
		}
	}
	return response.OK(c, map[string]any{"max_days": maxDays})
}

// UpdateLogSettings handles PUT /api/settings/logs.
func (h *OpsController) UpdateLogSettings(c echo.Context) error {
	var req struct {
		MaxDays int `json:"max_days"`
	}
	if err := c.Bind(&req); err != nil || req.MaxDays < 1 || req.MaxDays > 365 {
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", "max_days must be between 1 and 365")
	}
	_, err := h.pool.Exec(c.Request().Context(), `
		INSERT INTO _settings (key, value, updated_at)
		VALUES ('logs_max_days', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		fmt.Sprintf("%d", req.MaxDays),
	)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{"max_days": req.MaxDays})
}
