package middlewares

import (
	"context"
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

// RequestLogEntry represents one recorded HTTP request for the Admin Logs view.
type RequestLogEntry struct {
	ID             int64     `json:"id,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
	Method         string    `json:"method"`
	Path           string    `json:"path"`
	URL            string    `json:"url,omitempty"`
	Status         int       `json:"status"`
	Code           string    `json:"code"`
	Message        string    `json:"message"`
	LatencyMs      int64     `json:"latency_ms"`
	ExecTimeMs     float64   `json:"exec_time_ms"`
	IP             string    `json:"ip"`
	RemoteIP       string    `json:"remote_ip,omitempty"`
	Referer        string    `json:"referer,omitempty"`
	UserAgent      string    `json:"user_agent,omitempty"`
	Auth           string    `json:"auth,omitempty"` // "_superusers", "users", custom auth collection, or "_guest"
	AuthRole       string    `json:"auth_role,omitempty"`
	AuthEmail      string    `json:"auth_email,omitempty"`
	AuthID         string    `json:"auth_id,omitempty"`
}

const maxLogEntries = 200

var (
	logMu      sync.RWMutex
	logEntries = make([]RequestLogEntry, 0, maxLogEntries)
	logPool    *pgxpool.Pool
)

// SetLogPool configures PostgreSQL persistence for HTTP request logs.
func SetLogPool(pool *pgxpool.Pool) {
	logMu.Lock()
	logPool = pool
	logMu.Unlock()
}

// RecentLogs returns recorded HTTP logs ordered newest-first from PostgreSQL (or memory fallback).
func RecentLogs(ctx context.Context) []RequestLogEntry {
	logMu.RLock()
	pool := logPool
	logMu.RUnlock()

	if pool != nil {
		rows, err := pool.Query(ctx, `
			SELECT id, status, method, path, full_url, code, message, latency_ms, exec_time_ms,
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
					&e.ID, &e.Status, &e.Method, &e.Path, &e.URL, &e.Code, &e.Message,
					&e.LatencyMs, &e.ExecTimeMs, &e.IP, &e.RemoteIP, &e.Referer, &e.UserAgent,
					&e.Auth, &e.AuthRole, &e.AuthEmail, &e.AuthID, &e.Timestamp,
				) == nil {
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
					status, method, path, full_url, code, message, latency_ms, exec_time_ms,
					ip, remote_ip, referer, user_agent, auth_collection, auth_role, auth_email, auth_id, created_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
				e.Status, e.Method, e.Path, e.URL, e.Code, e.Message, e.LatencyMs, e.ExecTimeMs,
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

// RequestLogger logs HTTP requests via slog and persists them to _logs for /api/logs.
func RequestLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if strings.HasPrefix(path, "/_/_app/") || path == "/api/logs" {
				return next(c)
			}

			start := time.Now()
			err := next(c)
			if err != nil {
				c.Error(err)
			}

			dur := time.Since(start)
			status := c.Response().Status
			code, msg := extractCodeAndMessage(status, err)

			authCol := "_guest"
			var authRole, authEmail, authID string
			if claims := GetAuthClaims(c); claims != nil {
				authRole = claims.Role
				authEmail = claims.Email
				authID = claims.RecordID
				if authID == "" {
					authID = claims.UserID
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

			entry := RequestLogEntry{
				Timestamp:  start.UTC(),
				Method:     c.Request().Method,
				Path:       path,
				URL:        fullURL,
				Status:     status,
				Code:       code,
				Message:    msg,
				LatencyMs:  dur.Milliseconds(),
				ExecTimeMs: float64(dur.Nanoseconds()) / 1e6,
				IP:         c.RealIP(),
				RemoteIP:   stripPort(c.Request().RemoteAddr),
				Referer:    c.Request().Referer(),
				UserAgent:  c.Request().UserAgent(),
				Auth:       authCol,
				AuthRole:   authRole,
				AuthEmail:  authEmail,
				AuthID:     authID,
			}
			appendLog(entry)

			attrs := []any{
				"method", entry.Method,
				"path", entry.Path,
				"status", entry.Status,
				"code", entry.Code,
				"latency", dur.Round(time.Millisecond).String(),
				"ip", entry.IP,
			}
			if authCol != "_guest" {
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
