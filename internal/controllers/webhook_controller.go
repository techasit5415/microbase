package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
)

type WebhookController struct {
	service *services.WebhookService
}

func NewWebhookController(svc *services.WebhookService) *WebhookController {
	return &WebhookController{service: svc}
}

func (h *WebhookController) List(c echo.Context) error {
	items, err := h.service.List(c.Request().Context())
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{"items": items})
}

func (h *WebhookController) Create(c echo.Context) error {
	var req struct {
		Provider   string `json:"provider"`
		Collection string `json:"collection"`
		Event      string `json:"event"`
		URL        string `json:"url"`
		AuthToken  string `json:"auth_token"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	w, err := h.service.Create(c.Request().Context(), req.Provider, req.Collection, req.Event, req.URL, req.AuthToken)
	if errors.Is(err, services.ErrInvalidInput) {
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.Created(c, w)
}

func (h *WebhookController) Delete(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_ID", "invalid webhook id")
	}
	if err := h.service.Delete(c.Request().Context(), id); err != nil {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "webhook not found")
	}
	return response.OK(c, map[string]any{"deleted": id})
}
