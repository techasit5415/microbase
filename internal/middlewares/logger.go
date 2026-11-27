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

// RecentLogs returns up to 200 newest recorded HTTP logs.
func RecentLogs(ctx context.Context) []RequestLogEntry {
	items, _ := QueryLogs(ctx, 1, 200, "")
	return items
}

func levelKeywordMatches(level int, q string) bool {
	switch q {
	case "debug":
		return level < 0
	case "info":
		return level >= 0 && level < 4
	case "warn", "warning":
		return level >= 4 && level < 8
	case "error", "err":
		return level >= 8
	default:
		return false
	}
}

// QueryLogs returns paginated HTTP logs ordered newest-first along with total matching row count.
func QueryLogs(ctx context.Context, page, perPage int, search string) ([]RequestLogEntry, int64) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 30
	}
	if perPage > 200 {
		perPage = 200
	}
	search = strings.TrimSpace(search)
	offset := (page - 1) * perPage

	logMu.RLock()
	pool := logPool
	logMu.RUnlock()

	if pool != nil {
		whereClause := ""
		args := []any{}
		if search != "" {
			pattern := "%" + search + "%"
			lower := strings.ToLower(search)
			levelCond := ""
			switch lower {
			case "debug":
				levelCond = " OR level < 0"
			case "info":
				levelCond = " OR (level >= 0 AND level < 4 AND status < 400)"
			case "warn", "warning":
				levelCond = " OR (level >= 4 AND level < 8) OR (level = 0 AND status >= 400 AND status < 500)"
			case "error", "err":
				levelCond = " OR level >= 8 OR (level = 0 AND status >= 500)"
			}
			whereClause = ` WHERE (
				method ILIKE $1 OR path ILIKE $1 OR full_url ILIKE $1 OR
				auth_collection ILIKE $1 OR auth_role ILIKE $1 OR auth_email ILIKE $1 OR auth_id ILIKE $1 OR
				ip ILIKE $1 OR user_agent ILIKE $1 OR code ILIKE $1 OR message ILIKE $1 OR status::text ILIKE $1` + levelCond + `
			)`
			args = append(args, pattern)
		}

		var total int64
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM _logs`+whereClause, args...).Scan(&total); err == nil {
			limitIdx := len(args) + 1
			offsetIdx := len(args) + 2
			queryArgs := append(append([]any{}, args...), perPage, offset)
			query := fmt.Sprintf(`
				SELECT id, level, status, method, path, full_url, code, message, latency_ms, exec_time_ms,
				       ip, remote_ip, referer, user_agent, auth_collection, auth_role, auth_email, auth_id, created_at
				FROM _logs%s
				ORDER BY id DESC
				LIMIT $%d OFFSET $%d`, whereClause, limitIdx, offsetIdx)

			rows, err := pool.Query(ctx, query, queryArgs...)
			if err == nil {
				defer rows.Close()
				out := make([]RequestLogEntry, 0, perPage)
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
				return out, total
			}
		}
	}

	logMu.RLock()
	defer logMu.RUnlock()
	lower := strings.ToLower(search)
	matched := make([]RequestLogEntry, 0, len(logEntries))
	for i := len(logEntries) - 1; i >= 0; i-- {
		e := logEntries[i]
		if lower != "" {
			hay := strings.ToLower(fmt.Sprintf("%s %s %s %d %s %s %s %s %s %s %s",
				e.Method, e.Path, e.URL, e.Status, e.Code, e.Message, e.Auth, e.AuthEmail, e.AuthID, e.IP, e.UserAgent))
			if !strings.Contains(hay, lower) && !levelKeywordMatches(e.Level, lower) {
				continue
			}
		}
		matched = append(matched, e)
	}
	total := int64(len(matched))
	if offset >= len(matched) {
		return []RequestLogEntry{}, total
	}
	end := offset + perPage
	if end > len(matched) {
		end = len(matched)
	}
	return matched[offset:end], total
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

			logURL := *c.Request().URL
			query := logURL.Query()
			for _, key := range []string{"token", "code", "state"} {
				if query.Has(key) {
					query.Set(key, "[REDACTED]")
				}
			}
			logURL.RawQuery = query.Encode()
			fullURL := logURL.RequestURI()
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
