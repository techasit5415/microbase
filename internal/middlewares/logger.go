package middlewares

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// RequestLogEntry represents one recorded HTTP request for the Admin Logs view.
type RequestLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	LatencyMs int64     `json:"latency_ms"`
	IP        string    `json:"ip"`
	Error     string    `json:"error,omitempty"`
}

const maxLogEntries = 200

var (
	logMu      sync.RWMutex
	logEntries = make([]RequestLogEntry, 0, maxLogEntries)
)

// RecentLogs returns recorded HTTP logs ordered newest-first.
func RecentLogs() []RequestLogEntry {
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
	logMu.Unlock()
}

// RequestLogger logs HTTP requests via slog and records them for /api/logs.
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
			entry := RequestLogEntry{
				Timestamp: start.UTC(),
				Method:    c.Request().Method,
				Path:      path,
				Status:    status,
				LatencyMs: dur.Milliseconds(),
				IP:        c.RealIP(),
			}
			if err != nil {
				entry.Error = err.Error()
			}
			appendLog(entry)

			attrs := []any{
				"method", entry.Method,
				"path", entry.Path,
				"status", entry.Status,
				"latency", dur.Round(time.Millisecond).String(),
				"ip", entry.IP,
			}
			if entry.Error != "" {
				attrs = append(attrs, "error", entry.Error)
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
