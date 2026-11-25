package middlewares

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
)

const contextUserKey = "auth_claims"

// RequireAuth validates the Bearer JWT token and attaches AuthClaims to the request context.
func RequireAuth(authSvc *services.AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				return response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token")
			}

			tokenStr := strings.TrimPrefix(header, "Bearer ")
			claims, err := authSvc.ParseToken(tokenStr)
			if err != nil {
				return response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
			}

			c.Set(contextUserKey, claims)
			return next(c)
		}
	}
}

// GetAuthClaims retrieves the authenticated user's claims from the Echo context.
func GetAuthClaims(c echo.Context) *services.AuthClaims {
	claims, _ := c.Get(contextUserKey).(*services.AuthClaims)
	return claims
}
