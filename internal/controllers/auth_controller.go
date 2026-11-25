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

// AuthController handles user registration, login, and profile endpoints.
type AuthController struct {
	service     *services.AuthService
	collections *services.CollectionService
}

// NewAuthController constructs an AuthController.
func NewAuthController(svc *services.AuthService, colSvc *services.CollectionService) *AuthController {
	return &AuthController{service: svc, collections: colSvc}
}

func (h *AuthController) usersCollection(c echo.Context) *models.Collection {
	if h.collections == nil {
		return nil
	}
	col, err := h.collections.GetByName(c.Request().Context(), "users")
	if err != nil {
		return nil
	}
	return col
}

func (h *AuthController) maskHiddenUserFields(col *models.Collection, claims *services.AuthClaims, rec models.Record) models.Record {
	if col == nil || (claims != nil && claims.Role == "superuser") {
		return rec
	}
	for _, f := range col.Fields {
		if f.Hidden {
			delete(rec, f.Name)
		}
	}
	return rec
}

// Register handles POST /api/auth/register (enforces users "create" API rule).
func (h *AuthController) Register(c echo.Context) error {
	var raw map[string]any
	if err := json.NewDecoder(c.Request().Body).Decode(&raw); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	claims := middlewares.GetAuthClaims(c)
	if usersCol := h.usersCollection(c); usersCol != nil {
		if err := services.EvalCreateRule(usersCol, claims, raw); err != nil {
			return response.Error(c, http.StatusForbidden, "FORBIDDEN", "access denied by users create api rule")
		}
	}

	email, _ := raw["email"].(string)
	password, _ := raw["password"].(string)

	res, err := h.service.RegisterWithExtras(c.Request().Context(), email, password, raw, claims)
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

// ListUsers handles GET /api/users (enforces users "list" API rule including row-level SQL).
func (h *AuthController) ListUsers(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}

	claims := middlewares.GetAuthClaims(c)
	usersCol := h.usersCollection(c)
	ruleSQL, ruleArgs, _, ruleErr := services.CompileAPIRule(usersCol, "list", claims, nil, 1)
	if ruleErr != nil {
		return response.Error(c, http.StatusForbidden, "FORBIDDEN", "access denied by users list api rule")
	}

	users, total, err := h.service.ListUsers(c.Request().Context(), page, perPage, c.QueryParam("search"), ruleSQL, ruleArgs, c.QueryParam("sort"))
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	for i := range users {
		users[i] = h.maskHiddenUserFields(usersCol, claims, users[i])
	}
	return response.OK(c, map[string]any{
		"page":        page,
		"per_page":    perPage,
		"total_items": total,
		"items":       users,
	})
}

// GetUser handles GET /api/users/:id (enforces users "view" API rule including row-level SQL).
func (h *AuthController) GetUser(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_ID", "invalid user id")
	}
	claims := middlewares.GetAuthClaims(c)
	usersCol := h.usersCollection(c)
	ruleSQL, ruleArgs, _, ruleErr := services.CompileAPIRule(usersCol, "view", claims, nil, 2)
	if ruleErr != nil {
		return response.Error(c, http.StatusForbidden, "FORBIDDEN", "access denied by users view api rule")
	}

	u, err := h.service.GetUserRecord(c.Request().Context(), id, ruleSQL, ruleArgs)
	if errors.Is(err, services.ErrForbidden) {
		return response.Error(c, http.StatusForbidden, "FORBIDDEN", "access denied by users view api rule")
	}
	if errors.Is(err, services.ErrRecordNotFound) {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "user not found")
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, h.maskHiddenUserFields(usersCol, claims, u))
}

// DeleteUser handles DELETE /api/users/:id (enforces users "delete" API rule including row-level SQL).
func (h *AuthController) DeleteUser(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_ID", "invalid user id")
	}
	claims := middlewares.GetAuthClaims(c)
	usersCol := h.usersCollection(c)
	ruleSQL, ruleArgs, _, ruleErr := services.CompileAPIRule(usersCol, "delete", claims, nil, 2)
	if ruleErr != nil {
		return response.Error(c, http.StatusForbidden, "FORBIDDEN", "access denied by users delete api rule")
	}

	if err := h.service.DeleteUser(c.Request().Context(), id, ruleSQL, ruleArgs); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return response.Error(c, http.StatusForbidden, "FORBIDDEN", "access denied by users delete api rule")
		}
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "user not found")
	}
	return response.OK(c, map[string]any{"deleted": id})
}

// UpdateUser handles PATCH /api/users/:id (enforces users "update" API rule including row-level SQL).
func (h *AuthController) UpdateUser(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_ID", "invalid user id")
	}
	var raw map[string]any
	if err := json.NewDecoder(c.Request().Body).Decode(&raw); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	claims := middlewares.GetAuthClaims(c)
	usersCol := h.usersCollection(c)
	email, _ := raw["email"].(string)
	role, _ := raw["role"].(string)
	password, _ := raw["password"].(string)

	u, err := h.service.UpdateUser(c.Request().Context(), id, email, role, password, raw, usersCol, claims)
	if errors.Is(err, services.ErrForbidden) {
		return response.Error(c, http.StatusForbidden, "FORBIDDEN", "access denied by users update api rule")
	}
	if errors.Is(err, services.ErrInvalidInput) {
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	}
	if errors.Is(err, services.ErrEmailExists) {
		return response.Error(c, http.StatusConflict, "EMAIL_EXISTS", "email already registered")
	}
	if errors.Is(err, services.ErrRecordNotFound) {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "user not found")
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, h.maskHiddenUserFields(usersCol, claims, u))
}
