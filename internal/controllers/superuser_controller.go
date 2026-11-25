package controllers

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
)

type SuperuserController struct {
	service *services.SuperuserService
}

func NewSuperuserController(svc *services.SuperuserService) *SuperuserController {
	return &SuperuserController{service: svc}
}

func (h *SuperuserController) Status(c echo.Context) error {
	has, err := h.service.HasSuperuser(c.Request().Context())
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{"has_superuser": has})
}

func (h *SuperuserController) Setup(c echo.Context) error {
	var req services.SuperuserSetupRequest
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	res, err := h.service.Setup(c.Request().Context(), req)
	if errors.Is(err, services.ErrSetupLocked) {
		return response.Error(c, http.StatusForbidden, "SETUP_LOCKED", err.Error())
	}
	if errors.Is(err, services.ErrInvalidSetupOTP) {
		return response.Error(c, http.StatusUnauthorized, "INVALID_SETUP_OTP", err.Error())
	}
	if errors.Is(err, services.ErrInvalidInput) {
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	}
	if errors.Is(err, services.ErrEmailExists) {
		return response.Error(c, http.StatusConflict, "EMAIL_EXISTS", "superuser already exists")
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.Created(c, res)
}

func (h *SuperuserController) Login(c echo.Context) error {
	var req models.LoginRequest
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	res, err := h.service.Login(c.Request().Context(), req)
	if errors.Is(err, services.ErrInvalidCredentials) {
		return response.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid superuser credentials")
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, res)
}
