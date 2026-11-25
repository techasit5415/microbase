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
