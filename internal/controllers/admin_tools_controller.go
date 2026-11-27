package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
)

// AdminToolsController provides Superuser power tools: SQL Console and Batch API.
type AdminToolsController struct {
	pool *pgxpool.Pool
}

func NewAdminToolsController(pool *pgxpool.Pool) *AdminToolsController {
	return &AdminToolsController{pool: pool}
}

// ExecuteSQL handles POST /api/sql (Superuser only) and returns columns + rows or affected count.
func (h *AdminToolsController) ExecuteSQL(c echo.Context) error {
	claims := middlewares.GetAuthClaims(c)
	if claims == nil || claims.Role != "superuser" {
		return response.Error(c, http.StatusForbidden, "FORBIDDEN", "only superusers can execute SQL")
	}

	var req struct {
		Query string `json:"query"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	q := strings.TrimSpace(req.Query)
	if q == "" {
		return response.Error(c, http.StatusBadRequest, "EMPTY_QUERY", "SQL query is required")
	}

	start := time.Now()
	rows, err := h.pool.Query(c.Request().Context(), q)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "SQL_ERROR", err.Error())
	}
	defer rows.Close()

	descs := rows.FieldDescriptions()
	cols := make([]string, len(descs))
	for i, fd := range descs {
		cols[i] = string(fd.Name)
	}

	resultRows := make([]map[string]any, 0)
	for rows.Next() {
		vals, scanErr := rows.Values()
		if scanErr != nil {
			return response.Error(c, http.StatusInternalServerError, "SCAN_ERROR", scanErr.Error())
		}
		rowMap := make(map[string]any, len(cols))
		for i, col := range cols {
			if col == "password_hash" {
				rowMap[col] = "••••••••"
			} else {
				rowMap[col] = vals[i]
			}
		}
		resultRows = append(resultRows, rowMap)
		if len(resultRows) >= 500 {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return response.Error(c, http.StatusBadRequest, "SQL_ERROR", err.Error())
	}

	return response.OK(c, map[string]any{
		"columns":     cols,
		"rows":        resultRows,
		"row_count":   len(resultRows),
		"duration_ms": time.Since(start).Milliseconds(),
	})
}

// Batch handles POST /api/batch — executes multiple sub-requests against Echo in sequence.
func (h *AdminToolsController) Batch(c echo.Context) error {
	appCfg := services.GetCachedAppSettings()
	if !appCfg.Batch.Enabled {
		return response.Error(c, http.StatusForbidden, "BATCH_DISABLED", "batch web API is disabled in application settings")
	}
	var req struct {
		Requests []struct {
			Method string         `json:"method"`
			URL    string         `json:"url"`
			Body   map[string]any `json:"body"`
		} `json:"requests"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid batch payload")
	}
	if len(req.Requests) == 0 || len(req.Requests) > 50 {
		return response.Error(c, http.StatusBadRequest, "INVALID_BATCH", "batch must contain between 1 and 50 requests")
	}

	e := c.Echo()
	authHeader := c.Request().Header.Get("Authorization")
	results := make([]map[string]any, 0, len(req.Requests))

	for _, sub := range req.Requests {
		method := strings.ToUpper(strings.TrimSpace(sub.Method))
		if method == "" {
			method = http.MethodGet
		}
		urlPath := strings.TrimSpace(sub.URL)
		parsedSubURL, parseErr := url.Parse(urlPath)
		cleanPath := ""
		if parseErr == nil {
			cleanPath = path.Clean("/" + parsedSubURL.Path)
		}
		if parseErr != nil || parsedSubURL.Scheme != "" || parsedSubURL.Host != "" || !strings.HasPrefix(cleanPath, "/api/") || cleanPath == "/api/batch" || strings.HasPrefix(cleanPath, "/api/batch/") {
			results = append(results, map[string]any{"status": 400, "body": map[string]any{"error": "invalid batch target url"}})
			continue
		}

		var bodyBytes []byte
		if sub.Body != nil {
			bodyBytes, _ = json.Marshal(sub.Body)
		}
		httpReq := httptest.NewRequest(method, urlPath, bytes.NewReader(bodyBytes))
		httpReq.RemoteAddr = c.Request().RemoteAddr
		httpReq.Header.Set("Content-Type", "application/json")
		if xff := c.Request().Header.Get("X-Forwarded-For"); xff != "" {
			httpReq.Header.Set("X-Forwarded-For", xff)
		}
		if xri := c.Request().Header.Get("X-Real-IP"); xri != "" {
			httpReq.Header.Set("X-Real-IP", xri)
		}
		if authHeader != "" {
			httpReq.Header.Set("Authorization", authHeader)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httpReq)

		var parsedBody any
		if json.Unmarshal(rec.Body.Bytes(), &parsedBody) != nil {
			parsedBody = rec.Body.String()
		}
		results = append(results, map[string]any{
			"status": rec.Code,
			"body":   parsedBody,
		})
	}

	return response.OK(c, map[string]any{"results": results})
}
