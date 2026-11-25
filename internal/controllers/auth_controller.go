package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
)

// AuthController handles user registration, login, and profile endpoints.
type AuthController struct {
	service *services.AuthService
}

// NewAuthController constructs an AuthController.
func NewAuthController(svc *services.AuthService) *AuthController {
	return &AuthController{service: svc}
}

// Register handles POST /api/auth/register.
func (h *AuthController) Register(c echo.Context) error {
	var req models.RegisterRequest
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}

	res, err := h.service.Register(c.Request().Context(), req)
	if errors.Is(err, services.ErrInvalidInput) {
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	}
	if errors.Is(err, services.ErrEmailExists) {
		return response.Error(c, http.StatusConflict, "EMAIL_EXISTS", "email already registered")
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.Created(c, res)
}

// Login handles POST /api/auth/login.
func (h *AuthController) Login(c echo.Context) error {
	var req models.LoginRequest
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}

	res, err := h.service.Login(c.Request().Context(), req)
	if errors.Is(err, services.ErrInvalidCredentials) {
		return response.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, res)
}

// Me handles GET /api/auth/me (requires valid Bearer JWT).
func (h *AuthController) Me(c echo.Context) error {
	claims := middlewares.GetAuthClaims(c)
	if claims == nil {
		return response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
	}

	user, err := h.service.GetUserByID(c.Request().Context(), claims.UserID)
	if err != nil {
		return response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "user not found")
	}
	return response.OK(c, user)
}

// ListUsers handles GET /api/users for the Auth Collection view.
func (h *AuthController) ListUsers(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}

	users, total, err := h.service.ListUsers(c.Request().Context(), page, perPage, c.QueryParam("search"))
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{
		"page":        page,
		"per_page":    perPage,
		"total_items": total,
		"items":       users,
	})
}

// DeleteUser handles DELETE /api/users/:id.
func (h *AuthController) DeleteUser(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_ID", "user id must be an integer")
	}
	if err := h.service.DeleteUser(c.Request().Context(), id); err != nil {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "user not found")
	}
	return response.OK(c, map[string]any{"deleted": id})
}
