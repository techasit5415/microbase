package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/services"
	"github.com/techasit5415/microbase/internal/testutil"
)

func TestOAuthControllerStartCallbackExchange(t *testing.T) {
	pool := testutil.Database(t)
	sw := cache.NewSwitchableStore(cache.NewMemoryStore())
	authSvc := services.NewAuthService(pool, "oauth-ctrl-secret")
	oauthSvc := services.NewOAuthService(pool, sw, authSvc)
	ctrl := NewOAuthController(oauthSvc)

	// Spin up mock OIDC IdP (token + userinfo)
	var idpURL string
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "mock-at", "token_type": "Bearer"})
		case "/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":        "stu-100",
				"email":      "stu100@kmitl.ac.th",
				"department": map[string]any{"name": "Computer Science"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer idp.Close()
	idpURL = idp.URL

	e := echo.New()
	e.GET("/api/auth/methods", ctrl.AuthMethods)
	e.GET("/api/oauth/providers", ctrl.ListProviders)
	e.PUT("/api/oauth/providers/:name", ctrl.UpdateProvider)
	e.GET("/api/auth/oauth/:provider", ctrl.Start)
	e.GET("/api/auth/oauth/:provider/callback", ctrl.Callback)
	e.POST("/api/auth/oauth/exchange", ctrl.Exchange)

	// Configure OIDC provider with manual endpoints + allowed_claims
	cfgBody, _ := json.Marshal(map[string]any{
		"enabled":        true,
		"client_id":      "cid",
		"client_secret":  "csec",
		"auth_url":       idpURL + "/authorize",
		"token_url":      idpURL + "/token",
		"userinfo_url":   idpURL + "/userinfo",
		"scopes":         "openid email",
		"allowed_claims": map[string][]string{"department.name": {"Computer Science"}},
	})
	req := httptest.NewRequest("PUT", "/api/oauth/providers/oidc", bytes.NewReader(cfgBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("UpdateProvider oidc: %d %s", rec.Code, rec.Body.String())
	}

	// Start auth with redirect_to
	req = httptest.NewRequest("GET", "/api/auth/oauth/oidc?redirect_to=http://app.local/cb", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusTemporaryRedirect && rec.Code != http.StatusFound {
		t.Fatalf("expected redirect from Start, got %d: %s", rec.Code, rec.Body.String())
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	state := loc.Query().Get("state")
	if state == "" {
		t.Fatalf("expected state in redirect location: %s", loc)
	}

	// Callback with bad state -> 400
	req = httptest.NewRequest("GET", "/api/auth/oauth/oidc/callback?code=xyz&state=bad", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("expected 400 for invalid state, got %d", rec.Code)
	}

	// Callback with valid state -> redirects to http://app.local/cb?code=...
	req = httptest.NewRequest("GET", "/api/auth/oauth/oidc/callback?code=validcode&state="+state, nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusTemporaryRedirect && rec.Code != http.StatusFound {
		t.Fatalf("expected redirect from Callback, got %d: %s", rec.Code, rec.Body.String())
	}
	appRedirect, _ := url.Parse(rec.Header().Get("Location"))
	oneTimeCode := appRedirect.Query().Get("code")
	if oneTimeCode == "" || !strings.HasPrefix(appRedirect.String(), "http://app.local/cb") {
		t.Fatalf("unexpected callback redirect: %s", appRedirect)
	}

	// Exchange one-time code -> 200 + JWT
	exBody, _ := json.Marshal(map[string]any{"code": oneTimeCode})
	req = httptest.NewRequest("POST", "/api/auth/oauth/exchange", bytes.NewReader(exBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200 from Exchange, got %d: %s", rec.Code, rec.Body.String())
	}

	// Reusing one-time code -> 400
	req = httptest.NewRequest("POST", "/api/auth/oauth/exchange", bytes.NewReader(exBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("expected 400 on reused code, got %d", rec.Code)
	}
}
