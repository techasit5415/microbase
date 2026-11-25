package controllers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
)

// AuthExtraController implements PocketBase's extended auth flows:
// Refresh, Email Verification, Password Reset, Email Change, User OTP, MFA, and Impersonation.
type AuthExtraController struct {
	pool  *pgxpool.Pool
	auth  *services.AuthService
	cache cache.CacheStore
}

func NewAuthExtraController(pool *pgxpool.Pool, authSvc *services.AuthService, c cache.CacheStore) *AuthExtraController {
	return &AuthExtraController{pool: pool, auth: authSvc, cache: c}
}

func (h *AuthExtraController) sendEmailOrLog(ctx context.Context, to, subject, body string) {
	var raw string
	_ = h.pool.QueryRow(ctx, `SELECT value FROM _settings WHERE key = 'mail_config'`).Scan(&raw)
	var cfg MailConfig
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	if !cfg.Enabled || cfg.Host == "" {
		slog.Info("auth email (SMTP disabled; logged for dev)", "to", to, "subject", subject, "body", body)
		return
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	from := cfg.SenderAddress
	if from == "" {
		from = cfg.Username
	}
	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	msg := fmt.Sprintf("From: %s <%s>\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n", cfg.SenderName, from, to, subject, body)
	if err := smtp.SendMail(fmt.Sprintf("%s:%d", cfg.Host, cfg.Port), auth, from, []string{to}, []byte(msg)); err != nil {
		slog.Warn("smtp send failed", "to", to, "error", err)
	}
}

func genAuthToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func gen6DigitOTP() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	return fmt.Sprintf("%06d", n.Int64())
}

// Refresh handles POST /api/auth/refresh (renews JWT for authenticated user).
func (h *AuthExtraController) Refresh(c echo.Context) error {
	claims := middlewares.GetAuthClaims(c)
	if claims == nil {
		return response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	}
	u := models.User{ID: claims.UserID, Email: claims.Email, Role: claims.Role}
	tok, err := h.auth.GenerateToken(u)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, models.AuthResponse{Token: tok, User: u})
}

// RequestVerification handles POST /api/auth/request-verification.
func (h *AuthExtraController) RequestVerification(c echo.Context) error {
	var req struct{ Email string `json:"email"` }
	if err := c.Bind(&req); err != nil || !strings.Contains(req.Email, "@") {
		return response.Error(c, http.StatusBadRequest, "INVALID_EMAIL", "valid email required")
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	tok := genAuthToken()
	_ = h.cache.Set(c.Request().Context(), "verify:"+tok, email, 15*time.Minute)
	go h.sendEmailOrLog(context.Background(), email, "Verify your Microbase email", "Verification token (valid 15m): "+tok)
	return response.OK(c, map[string]any{"sent": true})
}

// ConfirmVerification handles POST /api/auth/confirm-verification.
func (h *AuthExtraController) ConfirmVerification(c echo.Context) error {
	var req struct{ Token string `json:"token"` }
	if err := c.Bind(&req); err != nil || req.Token == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_TOKEN", "token required")
	}
	email, err := h.cache.Get(c.Request().Context(), "verify:"+req.Token)
	if err != nil || email == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_TOKEN", "invalid or expired verification token")
	}
	_, err = h.pool.Exec(c.Request().Context(), `UPDATE users SET verified = TRUE, updated_at = NOW() WHERE email = $1`, email)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	_ = h.cache.Delete(c.Request().Context(), "verify:"+req.Token)
	return response.OK(c, map[string]any{"verified": true, "email": email})
}

// RequestPasswordReset handles POST /api/auth/request-password-reset.
func (h *AuthExtraController) RequestPasswordReset(c echo.Context) error {
	var req struct{ Email string `json:"email"` }
	if err := c.Bind(&req); err != nil || !strings.Contains(req.Email, "@") {
		return response.Error(c, http.StatusBadRequest, "INVALID_EMAIL", "valid email required")
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	tok := genAuthToken()
	_ = h.cache.Set(c.Request().Context(), "pwreset:"+tok, email, 15*time.Minute)
	go h.sendEmailOrLog(context.Background(), email, "Reset your Microbase password", "Password reset token (valid 15m): "+tok)
	return response.OK(c, map[string]any{"sent": true})
}

// ConfirmPasswordReset handles POST /api/auth/confirm-password-reset.
func (h *AuthExtraController) ConfirmPasswordReset(c echo.Context) error {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil || req.Token == "" || len(req.Password) < 8 {
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", "token and password (min 8 chars) required")
	}
	email, err := h.cache.Get(c.Request().Context(), "pwreset:"+req.Token)
	if err != nil || email == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_TOKEN", "invalid or expired reset token")
	}
	hash, err := services.HashPassword(req.Password)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	_, err = h.pool.Exec(c.Request().Context(), `UPDATE users SET password_hash = $2, updated_at = NOW() WHERE email = $1`, email, hash)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	_ = h.cache.Delete(c.Request().Context(), "pwreset:"+req.Token)
	return response.OK(c, map[string]any{"reset": true})
}

// RequestEmailChange handles POST /api/auth/request-email-change (requires Auth).
func (h *AuthExtraController) RequestEmailChange(c echo.Context) error {
	claims := middlewares.GetAuthClaims(c)
	if claims == nil {
		return response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	}
	var req struct{ NewEmail string `json:"new_email"` }
	if err := c.Bind(&req); err != nil || !strings.Contains(req.NewEmail, "@") {
		return response.Error(c, http.StatusBadRequest, "INVALID_EMAIL", "valid new_email required")
	}
	newEmail := strings.ToLower(strings.TrimSpace(req.NewEmail))
	tok := genAuthToken()
	payload, _ := json.Marshal(map[string]any{"user_id": claims.UserID, "new_email": newEmail})
	_ = h.cache.Set(c.Request().Context(), "emailchg:"+tok, string(payload), 15*time.Minute)
	go h.sendEmailOrLog(context.Background(), newEmail, "Confirm Microbase email change", "Email change token (valid 15m): "+tok)
	return response.OK(c, map[string]any{"sent": true})
}

// ConfirmEmailChange handles POST /api/auth/confirm-email-change.
func (h *AuthExtraController) ConfirmEmailChange(c echo.Context) error {
	var req struct{ Token string `json:"token"` }
	if err := c.Bind(&req); err != nil || req.Token == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_TOKEN", "token required")
	}
	raw, err := h.cache.Get(c.Request().Context(), "emailchg:"+req.Token)
	if err != nil || raw == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_TOKEN", "invalid or expired token")
	}
	var data struct {
		UserID   string `json:"user_id"`
		NewEmail string `json:"new_email"`
	}
	_ = json.Unmarshal([]byte(raw), &data)
	_, err = h.pool.Exec(c.Request().Context(), `UPDATE users SET email = $2, updated_at = NOW() WHERE id::text = $1`, data.UserID, data.NewEmail)
	if err != nil {
		return response.Error(c, http.StatusConflict, "EMAIL_CHANGE_FAILED", err.Error())
	}
	_ = h.cache.Delete(c.Request().Context(), "emailchg:"+req.Token)
	return response.OK(c, map[string]any{"email": data.NewEmail})
}

// RequestOTP handles POST /api/auth/request-otp (6-digit email OTP valid 5m).
func (h *AuthExtraController) RequestOTP(c echo.Context) error {
	var req struct{ Email string `json:"email"` }
	if err := c.Bind(&req); err != nil || !strings.Contains(req.Email, "@") {
		return response.Error(c, http.StatusBadRequest, "INVALID_EMAIL", "valid email required")
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	var userID string
	if err := h.pool.QueryRow(c.Request().Context(), `SELECT id::text FROM users WHERE email = $1`, email).Scan(&userID); err != nil {
		return response.OK(c, map[string]any{"otp_id": genAuthToken()}) // Do not leak account existence
	}
	otpID := genAuthToken()
	code := gen6DigitOTP()
	payload, _ := json.Marshal(map[string]any{"user_id": userID, "email": email, "code": code})
	_ = h.cache.Set(c.Request().Context(), "userotp:"+otpID, string(payload), 5*time.Minute)
	go h.sendEmailOrLog(context.Background(), email, "Your Microbase Login OTP", "Your 6-digit login code (valid 5 minutes): "+code)
	return response.OK(c, map[string]any{"otp_id": otpID})
}

// AuthWithOTP handles POST /api/auth/auth-with-otp.
func (h *AuthExtraController) AuthWithOTP(c echo.Context) error {
	var req struct {
		OTPID    string `json:"otp_id"`
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil || req.OTPID == "" || req.Password == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", "otp_id and password (6-digit code) required")
	}
	raw, err := h.cache.Get(c.Request().Context(), "userotp:"+req.OTPID)
	if err != nil || raw == "" {
		return response.Error(c, http.StatusUnauthorized, "INVALID_OTP", "invalid or expired OTP")
	}
	var data struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
		Code   string `json:"code"`
	}
	_ = json.Unmarshal([]byte(raw), &data)
	if strings.TrimSpace(req.Password) != data.Code {
		return response.Error(c, http.StatusUnauthorized, "INVALID_OTP", "incorrect OTP code")
	}
	_ = h.cache.Delete(c.Request().Context(), "userotp:"+req.OTPID)

	var u models.User
	err = h.pool.QueryRow(c.Request().Context(), `SELECT id::text, email, role, created_at, updated_at FROM users WHERE id::text = $1`, data.UserID).
		Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return response.Error(c, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
	}
	tok, err := h.auth.GenerateToken(u)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, models.AuthResponse{Token: tok, User: u})
}

// Impersonate handles POST /api/users/:id/impersonate (Superuser-only: mints a JWT as that user).
func (h *AuthExtraController) Impersonate(c echo.Context) error {
	claims := middlewares.GetAuthClaims(c)
	if claims == nil || claims.Role != "superuser" {
		return response.Error(c, http.StatusForbidden, "FORBIDDEN", "only superusers can impersonate users")
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_ID", "invalid user id")
	}
	var u models.User
	err := h.pool.QueryRow(c.Request().Context(), `SELECT id::text, email, role, created_at, updated_at FROM users WHERE id::text = $1`, id).
		Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return response.Error(c, http.StatusNotFound, "USER_NOT_FOUND", "user not found")
	}
	tok, err := h.auth.GenerateToken(u)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, models.AuthResponse{Token: tok, User: u})
}

// GetAuthOptions handles GET /api/settings/auth-options (OTP & MFA toggles).
func (h *AuthExtraController) GetAuthOptions(c echo.Context) error {
	var raw string
	_ = h.pool.QueryRow(c.Request().Context(), `SELECT value FROM _settings WHERE key = 'auth_options'`).Scan(&raw)
	out := map[string]bool{"otp_enabled": true, "mfa_enabled": false}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return response.OK(c, out)
}

// UpdateAuthOptions handles PUT /api/settings/auth-options.
func (h *AuthExtraController) UpdateAuthOptions(c echo.Context) error {
	var req map[string]bool
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	encoded, _ := json.Marshal(req)
	_, err := h.pool.Exec(c.Request().Context(), `
		INSERT INTO _settings (key, value, updated_at)
		VALUES ('auth_options', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		string(encoded),
	)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, req)
}
