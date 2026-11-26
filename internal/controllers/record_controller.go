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
	perPageStr := c.QueryParam("per_page")
	if perPageStr == "" {
		perPageStr = c.QueryParam("perPage")
	}
	perPage, _ := strconv.Atoi(perPageStr)
	skipTotalRaw := strings.ToLower(strings.TrimSpace(c.QueryParam("skipTotal")))
	if skipTotalRaw == "" {
		skipTotalRaw = strings.ToLower(strings.TrimSpace(c.QueryParam("skip_total")))
	}
	skipTotal := skipTotalRaw == "1" || skipTotalRaw == "true"

	filters := make(map[string]string)
	for key, vals := range c.QueryParams() {
		if strings.HasPrefix(key, "filter_") && len(vals) > 0 {
			filters[strings.TrimPrefix(key, "filter_")] = vals[0]
		}
	}

	opts := models.ListOptions{
		Page:       page,
		PerPage:    perPage,
		Sort:       c.QueryParam("sort"),
		Search:     c.QueryParam("search"),
		Filters:    filters,
		FilterExpr: c.QueryParam("filter"),
		SkipTotal:  skipTotal,
		AuthClaims: middlewares.GetAuthClaims(c),
	}

	result, err := h.service.List(c.Request().Context(), c.Param("name"), opts)
	if err != nil {
		return handleRecordError(c, err)
	}
	if exp := c.QueryParam("expand"); exp != "" {
		if col, colErr := h.collections.GetByName(c.Request().Context(), c.Param("name")); colErr == nil {
			h.service.ExpandRecords(c.Request().Context(), col, result.Items, exp, middlewares.GetAuthClaims(c))
		}
	}
	for i := range result.Items {
		result.Items[i] = projectFields(h.maskHidden(c, result.Items[i]), c.QueryParam("fields"))
	}
	return response.OK(c, result)
}

// Get handles GET /api/collections/:name/records/:id.
func (h *RecordController) Get(c echo.Context) error {
	id := c.Param("id")

	rec, err := h.service.GetByID(c.Request().Context(), c.Param("name"), id, middlewares.GetAuthClaims(c))
	if err != nil {
		return handleRecordError(c, err)
	}
	if exp := c.QueryParam("expand"); exp != "" {
		if col, colErr := h.collections.GetByName(c.Request().Context(), c.Param("name")); colErr == nil {
			h.service.ExpandRecords(c.Request().Context(), col, []models.Record{rec}, exp, middlewares.GetAuthClaims(c))
		}
	}
	return response.OK(c, projectFields(h.maskHidden(c, rec), c.QueryParam("fields")))
}

// Create handles POST /api/collections/:name/records.
func (h *RecordController) Create(c echo.Context) error {
	var data models.Record
	if err := json.NewDecoder(c.Request().Body).Decode(&data); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}

	rec, err := h.service.Create(c.Request().Context(), c.Param("name"), data, middlewares.GetAuthClaims(c))
	if err != nil {
		return handleRecordError(c, err)
	}
	return response.Created(c, h.maskHidden(c, rec))
}

// Update handles PATCH /api/collections/:name/records/:id.
// Update handles PATCH /api/collections/:name/records/:id.
func (h *RecordController) Update(c echo.Context) error {
	id := c.Param("id")

	var data models.Record
	if err := json.NewDecoder(c.Request().Body).Decode(&data); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}

	rec, err := h.service.Update(c.Request().Context(), c.Param("name"), id, data, middlewares.GetAuthClaims(c))
	if err != nil {
		return handleRecordError(c, err)
	}
	return response.OK(c, projectFields(h.maskHidden(c, rec), c.QueryParam("fields")))
}

// Delete handles DELETE /api/collections/:name/records/:id.
func (h *RecordController) Delete(c echo.Context) error {
	id := c.Param("id")

	if err := h.service.Delete(c.Request().Context(), c.Param("name"), id, middlewares.GetAuthClaims(c)); err != nil {
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

	if err := h.enforceRule(c, "list"); err != nil {
		return handleRecordError(c, err)
	}
	items, err := h.service.VectorSearch(c.Request().Context(), c.Param("name"), req, middlewares.GetAuthClaims(c))
	if err != nil {
		return handleRecordError(c, err)
	}
	for i := range items {
		items[i] = h.maskHidden(c, items[i])
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

// maskHidden removes fields marked Hidden=true unless the requester is an authenticated superuser.
func (h *RecordController) maskHidden(c echo.Context, rec models.Record) models.Record {
	col, err := h.collections.GetByName(c.Request().Context(), c.Param("name"))
	if err != nil || col == nil {
		return models.Record{}
	}
	return services.VisibleRecord(col, middlewares.GetAuthClaims(c), rec)
}

// projectFields filters a record map down to only the comma-separated ?fields= keys when specified.
type excerptSpec struct {
	maxLen       int
	withEllipsis bool
}

func splitFieldsParam(param string) []string {
	var parts []string
	var cur strings.Builder
	depth := 0
	for _, r := range param {
		switch r {
		case '(':
			depth++
			cur.WriteRune(r)
		case ')':
			if depth > 0 {
				depth--
			}
			cur.WriteRune(r)
		case ',':
			if depth == 0 {
				if s := strings.TrimSpace(cur.String()); s != "" {
					parts = append(parts, s)
				}
				cur.Reset()
			} else {
				cur.WriteRune(r)
			}
		default:
			cur.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		parts = append(parts, s)
	}
	return parts
}

func applyExcerpt(v any, spec excerptSpec) any {
	s, ok := v.(string)
	if !ok || spec.maxLen <= 0 {
		return v
	}
	// Strip simple HTML tags for clean plain-text excerpt
	var plain strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			plain.WriteRune(r)
		}
	}
	runes := []rune(strings.TrimSpace(plain.String()))
	if len(runes) <= spec.maxLen {
		return string(runes)
	}
	out := strings.TrimSpace(string(runes[:spec.maxLen]))
	if spec.withEllipsis {
		return out + "..."
	}
	return out
}

func projectFields(rec models.Record, fieldsParam string) models.Record {
	if strings.TrimSpace(fieldsParam) == "" {
		return rec
	}
	parts := splitFieldsParam(fieldsParam)
	if len(parts) == 0 {
		return rec
	}
	wantAll := false
	want := make(map[string]bool)
	excerpts := make(map[string]excerptSpec)
	expandFields := make(map[string][]string)

	for _, raw := range parts {
		if raw == "*" {
			wantAll = true
			continue
		}
		if strings.HasPrefix(raw, "expand.") {
			rest := strings.TrimPrefix(raw, "expand.")
			dot := strings.IndexByte(rest, '.')
			if dot > 0 {
				rel := rest[:dot]
				sub := rest[dot+1:]
				expandFields[rel] = append(expandFields[rel], sub)
				want["expand"] = true
				continue
			}
		}
		if idx := strings.Index(raw, ":excerpt("); idx > 0 && strings.HasSuffix(raw, ")") {
			fname := strings.TrimSpace(raw[:idx])
			argsStr := raw[idx+len(":excerpt(") : len(raw)-1]
			args := strings.Split(argsStr, ",")
			maxLen, _ := strconv.Atoi(strings.TrimSpace(args[0]))
			withEllipsis := len(args) > 1 && strings.EqualFold(strings.TrimSpace(args[1]), "true")
			want[fname] = true
			excerpts[fname] = excerptSpec{maxLen: maxLen, withEllipsis: withEllipsis}
			continue
		}
		want[raw] = true
	}

	out := make(models.Record, len(rec))
	for k, v := range rec {
		if !wantAll && !want[k] && k != "expand" {
			continue
		}
		if k == "expand" && len(expandFields) > 0 {
			if expMap, ok := v.(map[string]any); ok {
				filteredExp := make(map[string]any, len(expMap))
				for rel, relVal := range expMap {
					subList, hasSub := expandFields[rel]
					if !hasSub {
						if wantAll {
							filteredExp[rel] = relVal
						}
						continue
					}
					subParam := strings.Join(subList, ",")
					switch rv := relVal.(type) {
					case models.Record:
						filteredExp[rel] = projectFields(rv, subParam)
					case []models.Record:
						arr := make([]models.Record, len(rv))
						for i, item := range rv {
							arr[i] = projectFields(item, subParam)
						}
						filteredExp[rel] = arr
					default:
						filteredExp[rel] = relVal
					}
				}
				out[k] = filteredExp
				continue
			}
		}
		if spec, hasEx := excerpts[k]; hasEx {
			out[k] = applyExcerpt(v, spec)
		} else {
			out[k] = v
		}
	}
	return out
}
