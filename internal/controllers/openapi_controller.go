package controllers

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/config"
	"github.com/techasit5415/microbase/internal/services"
)

// OpenAPIController dynamically generates an OpenAPI 3.0 spec from registered collections.
type OpenAPIController struct {
	collections *services.CollectionService
}

func NewOpenAPIController(colSvc *services.CollectionService) *OpenAPIController {
	return &OpenAPIController{collections: colSvc}
}

// Spec handles GET /api/openapi.json.
func (h *OpenAPIController) Spec(c echo.Context) error {
	cols, err := h.collections.List(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	paths := map[string]any{
		"/api/auth/methods":  map[string]any{"get": map[string]any{"summary": "List enabled auth methods", "tags": []string{"Auth"}}},
		"/api/auth/register": map[string]any{"post": map[string]any{"summary": "Register user", "tags": []string{"Auth"}}},
		"/api/auth/login":    map[string]any{"post": map[string]any{"summary": "Login user", "tags": []string{"Auth"}}},
		"/api/auth/me":       map[string]any{"get": map[string]any{"summary": "Get current user", "tags": []string{"Auth"}}},
		"/api/files":         map[string]any{"post": map[string]any{"summary": "Upload file", "tags": []string{"Storage"}}},
	}

	for _, col := range cols {
		listPath := fmt.Sprintf("/api/collections/%s/records", col.Name)
		itemPath := fmt.Sprintf("/api/collections/%s/records/{id}", col.Name)
		paths[listPath] = map[string]any{
			"get":  map[string]any{"summary": "List " + col.Name, "tags": []string{col.Name}},
			"post": map[string]any{"summary": "Create " + col.Name, "tags": []string{col.Name}},
		}
		paths[itemPath] = map[string]any{
			"get":    map[string]any{"summary": "Get " + col.Name + " by ID", "tags": []string{col.Name}},
			"patch":  map[string]any{"summary": "Update " + col.Name, "tags": []string{col.Name}},
			"delete": map[string]any{"summary": "Delete " + col.Name, "tags": []string{col.Name}},
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   "Microbase API",
			"version": config.Version,
		},
		"paths": paths,
	})
}
