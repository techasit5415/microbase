package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
)

// RecordController handles dynamic CRUD and search endpoints for collection records.
type RecordController struct {
	service     *services.RecordService
	collections *services.CollectionService
}

// NewRecordController constructs a RecordController.
func NewRecordController(svc *services.RecordService, colSvc *services.CollectionService) *RecordController {
	return &RecordController{service: svc, collections: colSvc}
}

// List handles GET /api/collections/:name/records.
func (h *RecordController) List(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))

	filters := make(map[string]string)
	for key, vals := range c.QueryParams() {
		if strings.HasPrefix(key, "filter_") && len(vals) > 0 {
			filters[strings.TrimPrefix(key, "filter_")] = vals[0]
		}
	}

	opts := models.ListOptions{
		Page:    page,
		PerPage: perPage,
		Sort:    c.QueryParam("sort"),
		Search:  c.QueryParam("search"),
		Filters: filters,
	}

	if err := h.enforceRule(c, "list"); err != nil {
		return handleRecordError(c, err)
	}
	result, err := h.service.List(c.Request().Context(), c.Param("name"), opts)
	if err != nil {
		return handleRecordError(c, err)
	}
	return response.OK(c, result)
}

// Get handles GET /api/collections/:name/records/:id.
func (h *RecordController) Get(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_ID", "record id must be an integer")
	}

	if err := h.enforceRule(c, "view"); err != nil {
		return handleRecordError(c, err)
	}
	rec, err := h.service.GetByID(c.Request().Context(), c.Param("name"), id)
	if err != nil {
		return handleRecordError(c, err)
	}
	return response.OK(c, rec)
}

// Create handles POST /api/collections/:name/records.
func (h *RecordController) Create(c echo.Context) error {
	var data models.Record
	if err := json.NewDecoder(c.Request().Body).Decode(&data); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}

	if err := h.enforceRule(c, "create"); err != nil {
		return handleRecordError(c, err)
	}
	rec, err := h.service.Create(c.Request().Context(), c.Param("name"), data)
	if err != nil {
		return handleRecordError(c, err)
	}
	return response.Created(c, rec)
}

// Update handles PATCH /api/collections/:name/records/:id.
// Update handles PATCH /api/collections/:name/records/:id.
func (h *RecordController) Update(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_ID", "record id must be an integer")
	}

	var data models.Record
	if err := json.NewDecoder(c.Request().Body).Decode(&data); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}

	if err := h.enforceRule(c, "update"); err != nil {
		return handleRecordError(c, err)
	}
	rec, err := h.service.Update(c.Request().Context(), c.Param("name"), id, data)
	if err != nil {
		return handleRecordError(c, err)
	}
	return response.OK(c, rec)
}

// Delete handles DELETE /api/collections/:name/records/:id.
func (h *RecordController) Delete(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_ID", "record id must be an integer")
	}

	if err := h.enforceRule(c, "delete"); err != nil {
		return handleRecordError(c, err)
	}
	if err := h.service.Delete(c.Request().Context(), c.Param("name"), id); err != nil {
		return handleRecordError(c, err)
	}
	return response.OK(c, map[string]any{"deleted": id})
}

// VectorSearch handles POST /api/collections/:name/search/vector.
func (h *RecordController) VectorSearch(c echo.Context) error {
	var req models.VectorSearchRequest
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid vector search payload")
	}

	items, err := h.service.VectorSearch(c.Request().Context(), c.Param("name"), req)
	if err != nil {
		return handleRecordError(c, err)
	}
	return response.OK(c, map[string]any{"items": items})
}

func handleRecordError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, services.ErrCollectionNotFound):
		return response.Error(c, http.StatusNotFound, "COLLECTION_NOT_FOUND", "collection not found")
	case errors.Is(err, services.ErrRecordNotFound):
		return response.Error(c, http.StatusNotFound, "RECORD_NOT_FOUND", "record not found")
	case errors.Is(err, services.ErrForbidden):
		return response.Error(c, http.StatusForbidden, "FORBIDDEN", "access denied by collection api rules")
	case errors.Is(err, services.ErrInvalidInput):
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	default:
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
}

func (h *RecordController) enforceRule(c echo.Context, action string) error {
	if h.collections == nil {
		return nil
	}
	col, err := h.collections.GetByName(c.Request().Context(), c.Param("name"))
	if err != nil {
		return err
	}
	return services.CheckAPIRule(col, action, middlewares.GetAuthClaims(c))
}
