package controllers

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/response"
)

// SystemController provides read-only inspection of Microbase system tables (with secrets masked).
type SystemController struct {
	pool *pgxpool.Pool
}

// NewSystemController constructs a SystemController.
func NewSystemController(pool *pgxpool.Pool) *SystemController {
	return &SystemController{pool: pool}
}

var allowedSystemQueries = map[string]string{
	"_authOrigins":     `SELECT id, collection_ref, record_ref, fingerprint, ip, created_at, updated_at FROM _auth_origins ORDER BY updated_at DESC LIMIT 200`,
	"_superusers":      `SELECT id, email, created_at, updated_at FROM _superusers ORDER BY id DESC LIMIT 100`,
	"_oauth_accounts":  `SELECT id, user_id, provider, provider_user_id, created_at FROM _oauth_accounts ORDER BY id DESC LIMIT 100`,
	"_oauth_providers": `SELECT name, display_name, enabled, client_id, (client_secret <> '') AS has_secret, issuer_url, updated_at FROM _oauth_providers ORDER BY name ASC`,
	"_webhooks":        `SELECT id, collection, event, url, enabled, created_at FROM _webhooks ORDER BY id DESC LIMIT 100`,
}

// List handles GET /api/system/:table.
func (h *SystemController) List(c echo.Context) error {
	table := c.Param("table")
	query, ok := allowedSystemQueries[table]
	if !ok {
		return response.Error(c, http.StatusNotFound, "SYSTEM_TABLE_NOT_FOUND", "system table not found")
	}

	rows, err := h.pool.Query(c.Request().Context(), query)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	defer rows.Close()

	descs := rows.FieldDescriptions()
	items := make([]map[string]any, 0)
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		}
		row := make(map[string]any, len(descs))
		for i, fd := range descs {
			row[fd.Name] = vals[i]
		}
		items = append(items, row)
	}

	return response.OK(c, map[string]any{
		"page":        1,
		"per_page":    100,
		"total_items": len(items),
		"items":       items,
	})
}

var deletableSystemTables = map[string]string{
	"_authOrigins":    "_auth_origins",
	"_oauth_accounts": "_oauth_accounts",
}

// Delete handles DELETE /api/system/:table/:id for mutable system tables (_authOrigins, _oauth_accounts).
func (h *SystemController) Delete(c echo.Context) error {
	physTable, ok := deletableSystemTables[c.Param("table")]
	if !ok {
		return response.Error(c, http.StatusForbidden, "FORBIDDEN", "records in this system table cannot be deleted directly")
	}
	id := c.Param("id")
	tag, err := h.pool.Exec(c.Request().Context(), `DELETE FROM `+physTable+` WHERE id::text = $1`, id)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	if tag.RowsAffected() == 0 {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "record not found")
	}
	return response.OK(c, map[string]any{"deleted": id})
}
