package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/services"
)

func TestRequireSuperuserGuard(t *testing.T) {
	authSvc := services.NewAuthService(nil, "test-secret-key-12345")
	userTok, _ := authSvc.GenerateToken(models.User{ID: "10", Email: "user@test.com", Role: "user"})
	superTok, _ := authSvc.GenerateToken(models.User{ID: "1", Email: "admin@test.com", Role: "superuser"})

	e := echo.New()
	e.GET("/admin", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}, RequireSuperuser(authSvc))

	// 1. Anonymous -> 401
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for anonymous, got %d", rec.Code)
	}

	// 2. Regular user -> 403
	req = httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for regular user, got %d", rec.Code)
	}

	// 3. Superuser via Bearer header -> 200
	req = httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+superTok)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for superuser, got %d", rec.Code)
	}

	// 4. Superuser via ?token= query param (for backup downloads) -> 200
	req = httptest.NewRequest(http.MethodGet, "/admin?token="+superTok, nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for superuser query token, got %d", rec.Code)
	}
}
