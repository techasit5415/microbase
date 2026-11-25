package middlewares

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	ID        int64     `json:"id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	LatencyMs int64     `json:"latency_ms"`
	IP        string    `json:"ip"`
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
			SELECT id, status, method, path, code, message, latency_ms, ip, created_at
			FROM _logs
			ORDER BY id DESC
			LIMIT 200`)
		if err == nil {
			defer rows.Close()
			out := make([]RequestLogEntry, 0, 64)
			for rows.Next() {
				var e RequestLogEntry
				if rows.Scan(&e.ID, &e.Status, &e.Method, &e.Path, &e.Code, &e.Message, &e.LatencyMs, &e.IP, &e.Timestamp) == nil {
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
				INSERT INTO _logs (status, method, path, code, message, latency_ms, ip, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				e.Status, e.Method, e.Path, e.Code, e.Message, e.LatencyMs, e.IP, e.Timestamp,
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
			entry := RequestLogEntry{
				Timestamp: start.UTC(),
				Method:    c.Request().Method,
				Path:      path,
				Status:    status,
				Code:      code,
				Message:   msg,
				LatencyMs: dur.Milliseconds(),
				IP:        c.RealIP(),
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
