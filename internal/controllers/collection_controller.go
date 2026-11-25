package controllers

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
)

// CollectionController handles CRUD endpoints for user-defined collections.
type CollectionController struct {
	service *services.CollectionService
}

// NewCollectionController constructs a CollectionController.
func NewCollectionController(svc *services.CollectionService) *CollectionController {
	return &CollectionController{service: svc}
}

// List handles GET /api/collections.
func (h *CollectionController) List(c echo.Context) error {
	cols, err := h.service.List(c.Request().Context())
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{"items": cols})
}

// Get handles GET /api/collections/:name.
func (h *CollectionController) Get(c echo.Context) error {
	name := c.Param("name")
	col, err := h.service.GetByName(c.Request().Context(), name)
	if errors.Is(err, services.ErrCollectionNotFound) {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "collection not found")
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, col)
}

// Create handles POST /api/collections.
func (h *CollectionController) Create(c echo.Context) error {
	var req models.CreateCollectionRequest
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}

	col, err := h.service.Create(c.Request().Context(), req)
	if errors.Is(err, services.ErrInvalidSchema) {
		return response.Error(c, http.StatusBadRequest, "INVALID_SCHEMA", err.Error())
	}
	if errors.Is(err, services.ErrCollectionExists) {
		return response.Error(c, http.StatusConflict, "ALREADY_EXISTS", "collection already exists")
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	return response.Created(c, col)
}

// Delete handles DELETE /api/collections/:name.
func (h *CollectionController) Delete(c echo.Context) error {
	name := c.Param("name")
	err := h.service.Delete(c.Request().Context(), name)
	if errors.Is(err, services.ErrCollectionNotFound) {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "collection not found")
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{"deleted": name})
}

// Update handles PATCH /api/collections/:name to add or remove fields.
func (h *CollectionController) Update(c echo.Context) error {
	var req struct {
		Fields    []models.FieldDef `json:"fields"`
		APIRules  map[string]string `json:"api_rules"`
		Indexes   []models.IndexDef `json:"indexes"`
		ViewQuery string            `json:"view_query"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}

	col, err := h.service.Update(c.Request().Context(), c.Param("name"), req.Fields, req.APIRules, req.Indexes, req.ViewQuery)
	if errors.Is(err, services.ErrCollectionNotFound) {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "collection not found")
	}
	if errors.Is(err, services.ErrInvalidSchema) {
		return response.Error(c, http.StatusBadRequest, "INVALID_SCHEMA", err.Error())
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, col)
}

// Truncate handles POST /api/collections/:name/truncate.
func (h *CollectionController) Truncate(c echo.Context) error {
	if err := h.service.Truncate(c.Request().Context(), c.Param("name")); err != nil {
		return response.Error(c, http.StatusBadRequest, "TRUNCATE_FAILED", err.Error())
	}
	return response.OK(c, map[string]any{"truncated": c.Param("name")})
}

// Duplicate handles POST /api/collections/:name/duplicate.
func (h *CollectionController) Duplicate(c echo.Context) error {
	var req struct {
		NewName string `json:"new_name"`
	}
	_ = c.Bind(&req)
	col, err := h.service.Duplicate(c.Request().Context(), c.Param("name"), req.NewName)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "DUPLICATE_FAILED", err.Error())
	}
	return response.Created(c, col)
}
