package middlewares

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
)

const contextUserKey = "auth_claims"

func extractBearerToken(c echo.Context) string {
	header := c.Request().Header.Get("Authorization")
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}
	// Only allow ?token= query parameter on SSE realtime stream (EventSource cannot set custom headers).
	if c.Request().URL.Path == "/api/realtime" {
		if q := strings.TrimSpace(c.QueryParam("token")); q != "" {
			return q
		}
	}
	return ""
}

// RequireAuth validates the Bearer JWT token and attaches AuthClaims to the request context.
func RequireAuth(authSvc *services.AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tokenStr := extractBearerToken(c)
			if tokenStr == "" {
				return response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token")
			}

			claims, err := authSvc.ParseToken(tokenStr)
			if err != nil {
				return response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
			}

			c.Set(contextUserKey, claims)
			return next(c)
		}
	}
}

// RequireSuperuser validates the Bearer JWT token and enforces that the caller has Role == "superuser".
func RequireSuperuser(authSvc *services.AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tokenStr := extractBearerToken(c)
			if tokenStr == "" {
				return response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "superuser authentication required")
			}

			claims, err := authSvc.ParseToken(tokenStr)
			if err != nil {
				return response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
			}
			if claims.Role != "superuser" {
				return response.Error(c, http.StatusForbidden, "FORBIDDEN", "only superusers can access this endpoint")
			}
			if !services.IsSuperuserIPAllowed(services.RealClientIP(c)) {
				return response.Error(c, http.StatusForbidden, "IP_NOT_ALLOWED", "superuser access is restricted to allowed IP addresses")
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

// OptionalAuth parses a Bearer JWT if present and attaches AuthClaims, allowing anonymous access otherwise.
func OptionalAuth(authSvc *services.AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if tokenStr := extractBearerToken(c); tokenStr != "" {
				if claims, err := authSvc.ParseToken(tokenStr); err == nil {
					c.Set(contextUserKey, claims)
				}
			}
			return next(c)
		}
	}
}
