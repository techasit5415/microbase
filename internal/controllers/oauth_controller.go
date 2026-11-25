package controllers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
)

// OAuthController handles OAuth2/OIDC provider configuration and login flows.
type OAuthController struct {
	service *services.OAuthService
}

// NewOAuthController constructs an OAuthController.
func NewOAuthController(svc *services.OAuthService) *OAuthController {
	return &OAuthController{service: svc}
}

// AuthMethods handles GET /api/auth/methods.
func (h *OAuthController) AuthMethods(c echo.Context) error {
	methods, err := h.service.AuthMethods(c.Request().Context())
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, methods)
}

// ListProviders handles GET /api/oauth/providers.
func (h *OAuthController) ListProviders(c echo.Context) error {
	providers, err := h.service.ListProviders(c.Request().Context())
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{"items": providers})
}

// UpdateProvider handles PUT /api/oauth/providers/:name.
func (h *OAuthController) UpdateProvider(c echo.Context) error {
	var req services.UpdateProviderRequest
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}

	updated, err := h.service.UpdateProvider(c.Request().Context(), c.Param("name"), req)
	if errors.Is(err, services.ErrProviderNotFound) {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "oauth provider not found")
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, updated)
}

// Start handles GET /api/auth/oauth/:provider?redirect_to=<optional>.
func (h *OAuthController) Start(c echo.Context) error {
	provider := c.Param("provider")
	callbackURL := buildCallbackURL(c, provider)
	redirectTo := c.QueryParam("redirect_to")

	authURL, err := h.service.StartAuth(c.Request().Context(), provider, callbackURL, redirectTo)
	if errors.Is(err, services.ErrProviderNotFound) {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "oauth provider not found")
	}
	if errors.Is(err, services.ErrInvalidInput) {
		return response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// Callback handles GET /api/auth/oauth/:provider/callback.
func (h *OAuthController) Callback(c echo.Context) error {
	provider := c.Param("provider")
	code := c.QueryParam("code")
	state := c.QueryParam("state")
	callbackURL := buildCallbackURL(c, provider)

	result, err := h.service.HandleCallback(c.Request().Context(), provider, code, state, callbackURL)
	if errors.Is(err, services.ErrInvalidState) {
		return response.Error(c, http.StatusBadRequest, "INVALID_STATE", "invalid or expired oauth state")
	}
	if errors.Is(err, services.ErrProviderNotFound) {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "oauth provider not found")
	}
	if err != nil {
		return response.Error(c, http.StatusBadGateway, "OAUTH_FAILED", err.Error())
	}

	if result.RedirectURL != "" {
		return c.Redirect(http.StatusTemporaryRedirect, result.RedirectURL)
	}
	return response.OK(c, result.Auth)
}

// Exchange handles POST /api/auth/oauth/exchange with {"code": "..."}.
func (h *OAuthController) Exchange(c echo.Context) error {
	var req struct {
		Code string `json:"code"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}

	authRes, err := h.service.ExchangeCode(c.Request().Context(), req.Code)
	if errors.Is(err, services.ErrInvalidCode) {
		return response.Error(c, http.StatusBadRequest, "INVALID_CODE", "invalid or expired exchange code")
	}
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, authRes)
}

func buildCallbackURL(c echo.Context, provider string) string {
	scheme := "http"
	if c.IsTLS() || c.Request().Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/auth/oauth/%s/callback", scheme, c.Request().Host, provider)
}
