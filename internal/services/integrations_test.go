package services

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/testutil"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOAuthStateExchangeAndAccountLink(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	store := cache.NewMemoryStore()
	svc := NewOAuthService(pool, store, NewAuthService(pool, "test-secret"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			io.WriteString(w, `{"access_token":"access","token_type":"Bearer"}`)
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer access" {
				t.Error("missing bearer")
			}
			io.WriteString(w, `{"sub":"student123","email":"student@test.local","profile":{"name":"Student"},"department":"CS"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg := UpdateProviderRequest{Enabled: true, ClientID: "client", ClientSecret: "secret", AuthURL: server.URL + "/authorize", TokenURL: server.URL + "/token", UserInfoURL: server.URL + "/userinfo", FieldMapping: map[string]string{"name": "profile.name"}, AllowedClaims: map[string][]string{"department": {"CS"}}}
	if _, err := svc.UpdateProvider(ctx, "oidc", cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartAuth(ctx, "oidc", "http://localhost/callback", "javascript:bad"); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
	if _, err := svc.HandleCallback(ctx, "oidc", "code", "missing", "http://localhost/callback"); !errors.Is(err, ErrInvalidState) {
		t.Fatal(err)
	}
	for _, redirect := range []string{"", "http://localhost/app"} {
		authURL, err := svc.StartAuth(ctx, "oidc", "http://localhost/callback", redirect)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(authURL)
		state := u.Query().Get("state")
		result, err := svc.HandleCallback(ctx, "oidc", "code", state, "http://localhost/callback")
		if err != nil {
			t.Fatal(err)
		}
		if redirect == "" {
			if result.Auth == nil || result.Auth.User.Email != "student@test.local" {
				t.Fatal(result)
			}
		} else {
			u, _ = url.Parse(result.RedirectURL)
			code := u.Query().Get("code")
			auth, err := svc.ExchangeCode(ctx, code)
			if err != nil || auth.Token == "" {
				t.Fatal(err)
			}
			if _, err = svc.ExchangeCode(ctx, code); !errors.Is(err, ErrInvalidCode) {
				t.Fatal("exchange code replay accepted")
			}
		}
		if _, err = svc.HandleCallback(ctx, "oidc", "code", state, "http://localhost/callback"); !errors.Is(err, ErrInvalidState) {
			t.Fatal("state replay accepted")
		}
	}
	var name string
	var count int
	pool.QueryRow(ctx, "SELECT name FROM users WHERE email='student@test.local'").Scan(&name)
	pool.QueryRow(ctx, "SELECT count(*) FROM _oauth_accounts").Scan(&count)
	if name != "Student" || count != 1 {
		t.Fatal(name, count)
	}
	cfg.AllowedClaims = map[string][]string{"department": {"OTHER"}}
	svc.UpdateProvider(ctx, "oidc", cfg)
	authURL, _ := svc.StartAuth(ctx, "oidc", "http://localhost/callback", "http://localhost/app")
	u, _ := url.Parse(authURL)
	res, err := svc.HandleCallback(ctx, "oidc", "code", u.Query().Get("state"), "http://localhost/callback")
	if err != nil || !strings.Contains(res.RedirectURL, "access_denied") {
		t.Fatal(res, err)
	}
}
func TestWebhookRetrySignatureAndClientErrors(t *testing.T) {
	pool := testutil.Database(t)
	svc := NewWebhookService(pool)
	ctx := context.Background()
	var calls atomic.Int32
	var status atomic.Int32
	status.Store(503)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-Microbase-Signature") != ComputeHMACSignature(body, "secret") {
			t.Error("wrong signature")
		}
		if r.Header.Get("X-Test") != "value" || r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing custom/auth headers")
		}
		if status.Load() == 503 && n > 1 {
			w.WriteHeader(204)
		} else {
			w.WriteHeader(int(status.Load()))
		}
	}))
	defer server.Close()
	wh, err := svc.Create(ctx, CreateWebhookInput{Collection: "posts", Event: "create", URL: server.URL, Secret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.deliverWithRetry(ctx, wh.ID, "generic", "posts", "create", server.URL, "token", "secret", map[string]string{"X-Test": "value"}, 2, map[string]any{"id": 1})
	if err != nil || calls.Load() != 2 {
		t.Fatal(err, calls.Load())
	}
	calls.Store(0)
	status.Store(400)
	_, err = svc.deliverWithRetry(ctx, wh.ID, "generic", "posts", "create", server.URL, "token", "secret", map[string]string{"X-Test": "value"}, 3, nil)
	if err == nil || calls.Load() != 1 {
		t.Fatal("4xx should not retry", err, calls.Load())
	}
	for _, provider := range []string{"generic", "discord", "fcm"} {
		for _, event := range []string{"create", "update", "delete"} {
			if !json.Valid(buildWebhookPayload(provider, "posts", event, map[string]any{"text": strings.Repeat("x", 1800)})) {
				t.Fatal(provider, event)
			}
		}
	}
	if _, err = svc.Create(ctx, CreateWebhookInput{Provider: "unsupported"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
	if _, err = svc.Create(ctx, CreateWebhookInput{Collection: "posts", Event: "create", URL: "bad"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
}
func TestHTTPAndSQLCronExecution(t *testing.T) {
	pool := testutil.Database(t)
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			w.WriteHeader(500)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	for _, tc := range []struct {
		job  CronJob
		fail bool
	}{{CronJob{Kind: "http", Target: server.URL, Payload: `{}`}, false}, {CronJob{Kind: "http", Target: server.URL + "/fail", Method: "GET"}, true}, {CronJob{Kind: "sql", Target: "SELECT 1"}, false}, {CronJob{Kind: "sql", Target: "SELECT nonexistent"}, true}, {CronJob{Kind: "unknown"}, true}, {CronJob{Kind: "http", Target: "://invalid"}, true}} {
		msg, err := ExecuteCustomCron(ctx, pool, tc.job)
		if (err != nil) != tc.fail || msg == "" {
			t.Fatal(tc.job, err, msg)
		}
	}
}
