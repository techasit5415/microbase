package controllers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/services"
	"github.com/techasit5415/microbase/internal/storage"
	"github.com/techasit5415/microbase/internal/testutil"
)

func TestOpsAndSettingsControllersFull(t *testing.T) {
	pool := testutil.Database(t)
	sw := cache.NewSwitchableStore(cache.NewMemoryStore())
	authSvc := services.NewAuthService(pool, "ops-extra-secret")
	superTok, _ := authSvc.GenerateToken(models.User{ID: "1", Email: "su@test.com", Role: "superuser"})

	backupDir := t.TempDir()
	opsCtrl := NewOpsController(pool, backupDir)
	fs := storage.NewSwitchableStorage(t.TempDir(), nil)
	settingsCtrl := NewSettingsController(pool, sw, fs)

	e := echo.New()
	admin := middlewares.RequireSuperuser(authSvc)
	e.GET("/settings/ratelimits", opsCtrl.GetRateLimits, admin)
	e.PUT("/settings/ratelimits", opsCtrl.UpdateRateLimits, admin)
	e.GET("/settings/logs", opsCtrl.GetLogSettings, admin)
	e.PUT("/settings/logs", opsCtrl.UpdateLogSettings, admin)
	e.GET("/settings/mail", opsCtrl.GetMail, admin)
	e.PUT("/settings/mail", opsCtrl.UpdateMail, admin)
	e.POST("/settings/mail/test", opsCtrl.TestMail, admin)
	e.GET("/settings/database", opsCtrl.GetDatabase, admin)
	e.POST("/settings/database/test", opsCtrl.TestDatabase, admin)
	e.PUT("/settings/database", opsCtrl.UpdateDatabase, admin)
	e.GET("/settings/redis", settingsCtrl.GetRedis, admin)
	e.PUT("/settings/redis", settingsCtrl.UpdateRedis, admin)
	e.GET("/settings/s3", settingsCtrl.GetS3, admin)
	e.PUT("/settings/s3", settingsCtrl.UpdateS3, admin)

	doJSON := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+superTok)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	// Rate limits
	if rec := doJSON("GET", "/settings/ratelimits", nil); rec.Code != 200 {
		t.Fatalf("GetRateLimits: %d", rec.Code)
	}
	if rec := doJSON("PUT", "/settings/ratelimits", map[string]any{
		"enabled": true,
		"rules":   []map[string]any{{"label": "/api/auth/login", "max_requests": 20, "duration": 60}},
	}); rec.Code != 200 {
		t.Fatalf("UpdateRateLimits: %d %s", rec.Code, rec.Body.String())
	}

	// Log settings
	if rec := doJSON("GET", "/settings/logs", nil); rec.Code != 200 {
		t.Fatalf("GetLogSettings: %d", rec.Code)
	}
	if rec := doJSON("PUT", "/settings/logs", map[string]any{"max_days": 14}); rec.Code != 200 {
		t.Fatalf("UpdateLogSettings: %d", rec.Code)
	}

	// Mail settings & TestMail error paths
	if rec := doJSON("GET", "/settings/mail", nil); rec.Code != 200 {
		t.Fatalf("GetMail: %d", rec.Code)
	}
	if rec := doJSON("PUT", "/settings/mail", map[string]any{"enabled": false, "host": "smtp.test", "port": 587}); rec.Code != 200 {
		t.Fatalf("UpdateMail: %d", rec.Code)
	}
	if rec := doJSON("POST", "/settings/mail/test", map[string]any{"to": "bad-email"}); rec.Code != 400 {
		t.Fatalf("expected 400 for bad TestMail email, got %d", rec.Code)
	}
	if rec := doJSON("POST", "/settings/mail/test", map[string]any{"to": "a@b.com"}); rec.Code != 400 {
		t.Fatalf("expected 400 when SMTP disabled, got %d", rec.Code)
	}

	// Redis hot-swap (blank -> memory, invalid -> 400)
	if rec := doJSON("GET", "/settings/redis", nil); rec.Code != 200 {
		t.Fatalf("GetRedis: %d", rec.Code)
	}
	if rec := doJSON("PUT", "/settings/redis", map[string]any{"redis_url": ""}); rec.Code != 200 {
		t.Fatalf("UpdateRedis memory: %d", rec.Code)
	}
	if rec := doJSON("PUT", "/settings/redis", map[string]any{"redis_url": "redis://127.0.0.1:1/0"}); rec.Code != 400 {
		t.Fatalf("expected 400 for unreachable Redis, got %d", rec.Code)
	}

	// S3 settings (disabled -> local)
	if rec := doJSON("GET", "/settings/s3", nil); rec.Code != 200 {
		t.Fatalf("GetS3: %d", rec.Code)
	}
	if rec := doJSON("PUT", "/settings/s3", map[string]any{"enabled": false}); rec.Code != 200 {
		t.Fatalf("UpdateS3 local: %d", rec.Code)
	}

	// Database settings Get / Test / Update (using temp dir so real .env is untouched)
	if rec := doJSON("GET", "/settings/database", nil); rec.Code != 200 {
		t.Fatalf("GetDatabase: %d", rec.Code)
	}
	if rec := doJSON("POST", "/settings/database/test", map[string]any{"host": "", "database": ""}); rec.Code != 400 {
		t.Fatalf("expected 400 for empty db test, got %d", rec.Code)
	}
	if rec := doJSON("POST", "/settings/database/test", map[string]any{"host": "127.0.0.1", "port": "1", "database": "nope", "user": "x"}); rec.Code != 400 {
		t.Fatalf("expected 400 for unreachable db test, got %d", rec.Code)
	}

	// Parse current pool DSN for live TestDatabase + UpdateDatabase into temp dir .env
	dsn := pool.Config().ConnString()
	u, err := url.Parse(dsn)
	if err == nil && u.Hostname() != "" {
		port, _ := strconv.Atoi(u.Port())
		pass, _ := u.User.Password()
		dbName := u.Path
		if len(dbName) > 0 && dbName[0] == '/' {
			dbName = dbName[1:]
		}
		validDB := map[string]any{
			"host":     u.Hostname(),
			"port":     strconv.Itoa(port),
			"database": dbName,
			"user":     u.User.Username(),
			"password": pass,
			"sslmode":  "disable",
		}
		if rec := doJSON("POST", "/settings/database/test", validDB); rec.Code != 200 {
			t.Fatalf("expected 200 for live TestDatabase, got %d: %s", rec.Code, rec.Body.String())
		}
		origWD, _ := os.Getwd()
		tmpDir := t.TempDir()
		_ = os.Chdir(tmpDir)
		defer os.Chdir(origWD)
		if rec := doJSON("PUT", "/settings/database", validDB); rec.Code != 200 {
			t.Fatalf("expected 200 for UpdateDatabase, got %d: %s", rec.Code, rec.Body.String())
		}
	}
}
