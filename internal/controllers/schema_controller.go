package controllers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
)

// SchemaController handles schema snapshot export and import endpoints.
type SchemaController struct {
	service *services.CollectionService
}

// NewSchemaController constructs a SchemaController.
func NewSchemaController(svc *services.CollectionService) *SchemaController {
	return &SchemaController{service: svc}
}

// Export handles GET /api/schema/export.
func (h *SchemaController) Export(c echo.Context) error {
	snapshot, err := h.service.ExportSchema(c.Request().Context())
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, snapshot)
}

// Import handles POST /api/schema/import.
// Accepts both Microbase SchemaExport objects ({"collections":[...]}) and PocketBase pb_schema.json arrays ([...]).
func (h *SchemaController) Import(c echo.Context) error {
	raw, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "failed to read request body")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid schema export payload")
	}

	var payload models.SchemaExport
	if trimmed[0] == '[' {
		payload, err = services.ParsePocketBaseSchema(trimmed)
		if err != nil {
			return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid PocketBase schema payload: "+err.Error())
		}
	} else {
		if err := json.Unmarshal(trimmed, &payload); err != nil {
			return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid schema export payload")
		}
	}

	created, skipped, err := h.service.ImportSchema(c.Request().Context(), payload)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "IMPORT_FAILED", err.Error())
	}

	return response.OK(c, map[string]any{
		"created": created,
		"skipped": skipped,
	})
}
