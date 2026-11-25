package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/services"
	"github.com/techasit5415/microbase/internal/testutil"
)

func TestAuthExtraFlowsFull(t *testing.T) {
	pool := testutil.Database(t)
	mem := cache.NewMemoryStore()
	sw := cache.NewSwitchableStore(mem)
	authSvc := services.NewAuthService(pool, "auth-extra-test-secret-key")
	ctrl := NewAuthExtraController(pool, authSvc, sw)

	e := echo.New()
	e.POST("/auth/refresh", ctrl.Refresh, middlewares.RequireAuth(authSvc))
	e.POST("/auth/request-verification", ctrl.RequestVerification)
	e.POST("/auth/confirm-verification", ctrl.ConfirmVerification)
	e.POST("/auth/request-password-reset", ctrl.RequestPasswordReset)
	e.POST("/auth/confirm-password-reset", ctrl.ConfirmPasswordReset)
	e.POST("/auth/request-email-change", ctrl.RequestEmailChange, middlewares.RequireAuth(authSvc))
	e.POST("/auth/confirm-email-change", ctrl.ConfirmEmailChange)
	e.POST("/auth/request-otp", ctrl.RequestOTP)
	e.POST("/auth/auth-with-otp", ctrl.AuthWithOTP)
	e.POST("/users/:id/impersonate", ctrl.Impersonate, middlewares.RequireSuperuser(authSvc))
	e.GET("/settings/auth-options", ctrl.GetAuthOptions)
	e.PUT("/settings/auth-options", ctrl.UpdateAuthOptions)

	// Register a real user
	regRes, err := authSvc.Register(context.Background(), models.RegisterRequest{
		Email:    "extra@example.com",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	superTok, _ := authSvc.GenerateToken(models.User{ID: "999", Email: "super@example.com", Role: "superuser"})

	doJSON := func(method, path string, body any, tok string) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	// 1. RequestVerification (bad email -> 400, valid -> 200)
	if rec := doJSON("POST", "/auth/request-verification", map[string]any{"email": "nope"}, ""); rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if rec := doJSON("POST", "/auth/request-verification", map[string]any{"email": "extra@example.com"}, ""); rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. RequestPasswordReset (bad -> 400, valid -> 200)
	if rec := doJSON("POST", "/auth/request-password-reset", map[string]any{"email": "bad"}, ""); rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if rec := doJSON("POST", "/auth/request-password-reset", map[string]any{"email": "extra@example.com"}, ""); rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// 3. RequestEmailChange + ConfirmEmailChange
	if rec := doJSON("POST", "/auth/request-email-change", map[string]any{"new_email": "bad"}, regRes.Token); rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if rec := doJSON("POST", "/auth/request-email-change", map[string]any{"new_email": "changed@example.com"}, regRes.Token); rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	// Put a known emailchg token in cache to test ConfirmEmailChange end-to-end
	chgPayload, _ := json.Marshal(map[string]any{"user_id": regRes.User.ID, "new_email": "changed@example.com"})
	_ = sw.Set(context.Background(), "emailchg:known-tok", string(chgPayload), time.Minute)
	if rec := doJSON("POST", "/auth/confirm-email-change", map[string]any{"token": "wrong"}, ""); rec.Code != 400 {
		t.Fatalf("expected 400 for bad emailchg token, got %d", rec.Code)
	}
	if rec := doJSON("POST", "/auth/confirm-email-change", map[string]any{"token": "known-tok"}, ""); rec.Code != 200 {
		t.Fatalf("expected 200 for confirm email change, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. RequestOTP (unknown email -> 200 no leak, real email -> 200 + otp_id, AuthWithOTP wrong & right)
	if rec := doJSON("POST", "/auth/request-otp", map[string]any{"email": "invalid"}, ""); rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if rec := doJSON("POST", "/auth/request-otp", map[string]any{"email": "ghost@example.com"}, ""); rec.Code != 200 {
		t.Fatalf("expected 200 for unknown email, got %d", rec.Code)
	}
	otpReqRec := doJSON("POST", "/auth/request-otp", map[string]any{"email": "changed@example.com"}, "")
	if otpReqRec.Code != 200 {
		t.Fatalf("expected 200 for RequestOTP, got %d", otpReqRec.Code)
	}
	// Also test known OTP flow
	otpPayload, _ := json.Marshal(map[string]any{"user_id": regRes.User.ID, "email": "changed@example.com", "code": "654321"})
	_ = sw.Set(context.Background(), "userotp:known-otp", string(otpPayload), time.Minute)
	if rec := doJSON("POST", "/auth/auth-with-otp", map[string]any{"otp_id": "known-otp", "password": "000000"}, ""); rec.Code != 401 {
		t.Fatalf("expected 401 for wrong OTP, got %d", rec.Code)
	}
	if rec := doJSON("POST", "/auth/auth-with-otp", map[string]any{"otp_id": "known-otp", "password": "654321"}, ""); rec.Code != 200 {
		t.Fatalf("expected 200 for valid OTP, got %d: %s", rec.Code, rec.Body.String())
	}

	// 5. Refresh, Impersonate, AuthOptions
	if rec := doJSON("POST", "/auth/refresh", nil, regRes.Token); rec.Code != 200 {
		t.Fatalf("expected 200 for refresh, got %d", rec.Code)
	}
	if rec := doJSON("POST", "/users/invalid/impersonate", nil, superTok); rec.Code != 404 {
		t.Fatalf("expected 404 for unknown impersonate id, got %d", rec.Code)
	}
	if rec := doJSON("GET", "/settings/auth-options", nil, superTok); rec.Code != 200 {
		t.Fatalf("expected 200 for GetAuthOptions, got %d", rec.Code)
	}
	if rec := doJSON("PUT", "/settings/auth-options", map[string]any{"otp_enabled": true, "mfa_enabled": true}, superTok); rec.Code != 200 {
		t.Fatalf("expected 200 for UpdateAuthOptions, got %d", rec.Code)
	}

	// Wait briefly for background sendEmailOrLog goroutines to exercise logging branch cleanly
	time.Sleep(50 * time.Millisecond)
}
