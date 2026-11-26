package middlewares

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

// LogSettings mirrors PocketBase's logs configuration.
type LogSettings struct {
	MaxDays   int  `json:"max_days"`    // 0 = disable logs persistence & delete all logs; 1..365 = retain N days
	MinLevel  int  `json:"min_level"`   // -4:DEBUG, 0:INFO, 4:WARN, 8:ERROR
	MaxBytes  int  `json:"max_bytes"`   // 0 = default (~16KB = 16384)
	LogIP     bool `json:"log_ip"`      // include remoteIP / userIP
	LogAuthID bool `json:"log_auth_id"` // include auth_id
}

// DefaultLogSettings returns PocketBase-aligned defaults.
func DefaultLogSettings() LogSettings {
	return LogSettings{
		MaxDays:   7,
		MinLevel:  0,
		MaxBytes:  0,
		LogIP:     true,
		LogAuthID: true,
	}
}

// RequestLogEntry represents one recorded HTTP request for the Admin Logs view.
type RequestLogEntry struct {
	ID         int64     `json:"id,omitempty"`
	Level      int       `json:"level"`
	Timestamp  time.Time `json:"timestamp"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	URL        string    `json:"url,omitempty"`
	Status     int       `json:"status"`
	Code       string    `json:"code"`
	Message    string    `json:"message"`
	LatencyMs  int64     `json:"latency_ms"`
	ExecTimeMs float64   `json:"exec_time_ms"`
	IP         string    `json:"ip"`
	RemoteIP   string    `json:"remote_ip,omitempty"`
	Referer    string    `json:"referer,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	Auth       string    `json:"auth,omitempty"` // "_superusers", "users", custom auth collection, or empty
	AuthRole   string    `json:"auth_role,omitempty"`
	AuthEmail  string    `json:"auth_email,omitempty"`
	AuthID     string    `json:"auth_id,omitempty"`
}

const (
	maxLogEntries      = 200
	defaultMaxLogBytes = 16384
)

var (
	logMu       sync.RWMutex
	logEntries  = make([]RequestLogEntry, 0, maxLogEntries)
	logPool     *pgxpool.Pool
	logSettings = DefaultLogSettings()
)

// SetLogPool configures PostgreSQL persistence for HTTP request logs.
func SetLogPool(pool *pgxpool.Pool) {
	logMu.Lock()
	logPool = pool
	logMu.Unlock()
}

// GetLogSettings returns the current runtime log settings.
func GetLogSettings() LogSettings {
	logMu.RLock()
	defer logMu.RUnlock()
	return logSettings
}

// SetLogSettings updates the runtime log settings.
func SetLogSettings(cfg LogSettings) {
	logMu.Lock()
	logSettings = cfg
	logMu.Unlock()
}

// LoadLogSettingsFromDB loads persisted log settings from _settings (falling back to legacy logs_max_days).
func LoadLogSettingsFromDB(ctx context.Context, pool *pgxpool.Pool) LogSettings {
	cfg := DefaultLogSettings()
	if pool == nil {
		return GetLogSettings()
	}

	var raw string
	if err := pool.QueryRow(ctx, `SELECT value FROM _settings WHERE key = 'logs_settings'`).Scan(&raw); err == nil && raw != "" {
		var parsed LogSettings
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			cfg = parsed
			SetLogSettings(cfg)
			return cfg
		}
	}

	// Fallback to legacy logs_max_days key.
	var rawDays string
	if err := pool.QueryRow(ctx, `SELECT value FROM _settings WHERE key = 'logs_max_days'`).Scan(&rawDays); err == nil && rawDays != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(rawDays)); err == nil && n >= 0 && n <= 365 {
			cfg.MaxDays = n
		}
	}
	SetLogSettings(cfg)
	return cfg
}

// ClearLogs removes all persisted and in-memory HTTP logs.
func ClearLogs(ctx context.Context) error {
	logMu.Lock()
	logEntries = make([]RequestLogEntry, 0, maxLogEntries)
	pool := logPool
	logMu.Unlock()

	if pool != nil {
		_, err := pool.Exec(ctx, `DELETE FROM _logs`)
		return err
	}
	return nil
}

func statusToLevel(status int) int {
	switch {
	case status >= 500:
		return 8 // ERROR
	case status >= 400:
		return 4 // WARN
	default:
		return 0 // INFO
	}
}

func truncateString(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	if maxBytes <= 3 {
		return s[:maxBytes]
	}
	return s[:maxBytes-3] + "..."
}

// RecentLogs returns recorded HTTP logs ordered newest-first from PostgreSQL (or memory fallback).
func RecentLogs(ctx context.Context) []RequestLogEntry {
	logMu.RLock()
	pool := logPool
	logMu.RUnlock()

	if pool != nil {
		rows, err := pool.Query(ctx, `
			SELECT id, level, status, method, path, full_url, code, message, latency_ms, exec_time_ms,
			       ip, remote_ip, referer, user_agent, auth_collection, auth_role, auth_email, auth_id, created_at
			FROM _logs
			ORDER BY id DESC
			LIMIT 200`)
		if err == nil {
			defer rows.Close()
			out := make([]RequestLogEntry, 0, 64)
			for rows.Next() {
				var e RequestLogEntry
				if rows.Scan(
					&e.ID, &e.Level, &e.Status, &e.Method, &e.Path, &e.URL, &e.Code, &e.Message,
					&e.LatencyMs, &e.ExecTimeMs, &e.IP, &e.RemoteIP, &e.Referer, &e.UserAgent,
					&e.Auth, &e.AuthRole, &e.AuthEmail, &e.AuthID, &e.Timestamp,
				) == nil {
					if e.Level == 0 && e.Status >= 400 {
						e.Level = statusToLevel(e.Status)
					}
					if e.URL == "" {
						e.URL = e.Path
					}
					if e.RemoteIP == "" {
						e.RemoteIP = e.IP
					}
					out = append(out, e)
				}
			}
			return out
		}
	}

	logMu.RLock()
	defer logMu.RUnlock()
	out := make([]RequestLogEntry, len(logEntries))
	for i := range logEntries {
		out[i] = logEntries[len(logEntries)-1-i]
	}
	return out
}

func appendLog(entry RequestLogEntry) {
	logMu.Lock()
	if len(logEntries) >= maxLogEntries {
		logEntries = logEntries[1:]
	}
	logEntries = append(logEntries, entry)
	pool := logPool
	logMu.Unlock()

	if pool != nil {
		go func(e RequestLogEntry) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, _ = pool.Exec(ctx, `
				INSERT INTO _logs (
					level, status, method, path, full_url, code, message, latency_ms, exec_time_ms,
					ip, remote_ip, referer, user_agent, auth_collection, auth_role, auth_email, auth_id, created_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
				e.Level, e.Status, e.Method, e.Path, e.URL, e.Code, e.Message, e.LatencyMs, e.ExecTimeMs,
				e.IP, e.RemoteIP, e.Referer, e.UserAgent, e.Auth, e.AuthRole, e.AuthEmail, e.AuthID, e.Timestamp,
			)
		}(entry)
	}
}

func extractCodeAndMessage(status int, err error) (string, string) {
	if err != nil {
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			return strconv.Itoa(httpErr.Code), fmt.Sprint(httpErr.Message)
		}
		return "ERROR", err.Error()
	}
	if status >= 400 {
		return strconv.Itoa(status), http.StatusText(status)
	}
	return "OK", http.StatusText(status)
}

func stripPort(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	return remoteAddr
}

func shouldSkipRequestLog(path string) bool {
	return path == "/_" ||
		strings.HasPrefix(path, "/_/") ||
		path == "/favicon.ico" ||
		path == "/api/logs"
}

// RequestLogger logs HTTP requests via slog and persists them to _logs for /api/logs.
func RequestLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if shouldSkipRequestLog(path) {
				return next(c)
			}

			start := time.Now()
			err := next(c)
			if err != nil {
				c.Error(err)
			}

			dur := time.Since(start)
			status := c.Response().Status
			level := statusToLevel(status)
			code, msg := extractCodeAndMessage(status, err)

			cfg := GetLogSettings()
			maxBytes := cfg.MaxBytes
			if maxBytes <= 0 {
				maxBytes = defaultMaxLogBytes
			}

			authCol := ""
			var authRole, authEmail, authID string
			if claims := GetAuthClaims(c); claims != nil {
				authRole = claims.Role
				authEmail = claims.Email
				if cfg.LogAuthID {
					authID = claims.RecordID
					if authID == "" {
						authID = claims.UserID
					}
				}
				switch {
				case claims.Role == "superuser":
					authCol = "_superusers"
				case claims.Collection != "":
					authCol = claims.Collection
				default:
					authCol = "users"
				}
			}

			fullURL := c.Request().URL.RequestURI()
			if fullURL == "" {
				fullURL = path
			}

			ip := ""
			remoteIP := ""
			if cfg.LogIP {
				ip = c.RealIP()
				remoteIP = stripPort(c.Request().RemoteAddr)
			}

			// Enforce per-field budget derived from maxBytes so a single huge header/message cannot bloat _logs.
			fieldCap := maxBytes / 2
			if fieldCap < 256 {
				fieldCap = 256
			}

			entry := RequestLogEntry{
				Level:      level,
				Timestamp:  start.UTC(),
				Method:     c.Request().Method,
				Path:       truncateString(path, fieldCap),
				URL:        truncateString(fullURL, fieldCap),
				Status:     status,
				Code:       code,
				Message:    truncateString(msg, fieldCap),
				LatencyMs:  dur.Milliseconds(),
				ExecTimeMs: float64(dur.Nanoseconds()) / 1e6,
				IP:         ip,
				RemoteIP:   remoteIP,
				Referer:    truncateString(c.Request().Referer(), fieldCap),
				UserAgent:  truncateString(c.Request().UserAgent(), fieldCap),
				Auth:       authCol,
				AuthRole:   authRole,
				AuthEmail:  authEmail,
				AuthID:     authID,
			}

			if cfg.MaxDays > 0 && level >= cfg.MinLevel {
				appendLog(entry)
			}

			attrs := []any{
				"level_num", entry.Level,
				"method", entry.Method,
				"path", entry.Path,
				"status", entry.Status,
				"code", entry.Code,
				"latency", dur.Round(time.Millisecond).String(),
			}
			if ip != "" {
				attrs = append(attrs, "ip", ip)
			}
			if authCol != "" {
				attrs = append(attrs, "auth", authCol)
			}
			if status >= 400 && entry.Message != "" {
				attrs = append(attrs, "message", entry.Message)
			}

			switch {
			case status >= 500:
				slog.Error("http request", attrs...)
			case status >= 400:
				slog.Warn("http request", attrs...)
			default:
				slog.Info("http request", attrs...)
			}
			return nil
		}
	}
}
