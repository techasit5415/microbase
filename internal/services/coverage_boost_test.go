package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/testutil"
)

func TestCoverageBoostCronsWebhooksAuthRules(t *testing.T) {
	pool := testutil.Database(t)
	sw := cache.NewSwitchableStore(cache.NewMemoryStore())
	colSvc := NewCollectionService(pool, sw)
	whSvc := NewWebhookService(pool)
	recSvc := NewRecordService(pool, colSvc, nil, whSvc)
	authSvc := NewAuthService(pool, "cov-boost-secret")

	// 1. TestWebhook DispatchAsync with HMAC + custom header
	var whHits atomic.Int32
	var gotSig, gotCustom string
	whSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Microbase-Signature")
		gotCustom = r.Header.Get("X-Custom-Header")
		whHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer whSrv.Close()

	wh, err := whSvc.Create(context.Background(), CreateWebhookInput{
		Provider:   "generic",
		Collection: "boost_posts",
		Event:      "create",
		URL:        whSrv.URL,
		Secret:     "topsecret",
		Headers:    map[string]string{"X-Custom-Header": "hello-wh"},
		MaxRetries: 2,
	})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	if err := whSvc.Toggle(context.Background(), wh.ID, true); err != nil {
		t.Fatalf("toggle webhook: %v", err)
	}

	// Create collection with row-level rules to exercise recordExists (403 vs 404)
	_, err = colSvc.Create(context.Background(), models.CreateCollectionRequest{
		Name:   "boost_posts",
		Type:   "base",
		IDType: "text",
		Fields: []models.FieldDef{
			{Name: "title", Type: "text", Required: true},
			{Name: "owner_id", Type: "text"},
			{Name: "score", Type: "number"},
		},
		APIRules: map[string]string{
			"list":   "owner_id = @request.auth.id",
			"view":   "owner_id = @request.auth.id",
			"create": "@request.auth.id != ''",
			"update": "owner_id = @request.auth.id",
			"delete": "owner_id = @request.auth.id",
		},
	})
	if err != nil {
		t.Fatalf("create boost_posts: %v", err)
	}

	owner1 := &AuthClaims{UserID: 101, Email: "o1@test.com", Role: "user"}
	owner2 := &AuthClaims{UserID: 202, Email: "o2@test.com", Role: "user"}

	created, err := recSvc.Create(context.Background(), "boost_posts", models.Record{
		"title":    "Mine",
		"owner_id": "101",
		"score":    42,
	}, owner1)
	if err != nil {
		t.Fatalf("create record: %v", err)
	}
	recID := created["id"].(string)

	// Wait briefly for DispatchAsync webhook
	time.Sleep(150 * time.Millisecond)
	if whHits.Load() < 1 || gotSig == "" || gotCustom != "hello-wh" {
		t.Fatalf("expected async webhook hit with HMAC & custom header, hits=%d sig=%q custom=%q", whHits.Load(), gotSig, gotCustom)
	}

	// Owner2 trying to view/update/delete owner1's existing record -> ErrForbidden (exercises recordExists!)
	if _, err := recSvc.GetByID(context.Background(), "boost_posts", recID, owner2); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden on view by other user, got %v", err)
	}
	if _, err := recSvc.Update(context.Background(), "boost_posts", recID, models.Record{"title": "Hijack"}, owner2); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden on update by other user, got %v", err)
	}
	if err := recSvc.Delete(context.Background(), "boost_posts", recID, owner2); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden on delete by other user, got %v", err)
	}
	// Missing record -> ErrRecordNotFound
	if _, err := recSvc.GetByID(context.Background(), "boost_posts", "nonexistent999", owner2); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}

	// 2. Test runDueCrons with a real HTTP cron hitting httptest
	var cronHits atomic.Int32
	cronSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cronHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer cronSrv.Close()

	_, err = pool.Exec(context.Background(), `
		INSERT INTO _crons (id, expression, kind, target, method, enabled)
		VALUES ('due_http_test', '* * * * *', 'http', $1, 'POST', TRUE)`, cronSrv.URL)
	if err != nil {
		t.Fatalf("insert due cron: %v", err)
	}
	runDueCrons(pool, time.Now())
	time.Sleep(150 * time.Millisecond)
	if cronHits.Load() < 1 {
		t.Fatalf("expected runDueCrons to execute due_http_test")
	}

	// 3. Test AuthService UpdateUser & userExists (owner vs other user)
	u1, _ := authSvc.Register(context.Background(), models.RegisterRequest{Email: "u1@boost.com", Password: "password123"})
	u2, _ := authSvc.Register(context.Background(), models.RegisterRequest{Email: "u2@boost.com", Password: "password123"})
	usersCol, _ := colSvc.GetByName(context.Background(), "users")
	usersCol.APIRules = map[string]string{"view": "id = @request.auth.id", "update": "id = @request.auth.id", "delete": "id = @request.auth.id"}
	c1 := &AuthClaims{UserID: u1.User.ID, Email: u1.User.Email, Role: "user"}
	c2 := &AuthClaims{UserID: u2.User.ID, Email: u2.User.Email, Role: "user"}

	// u2 cannot update u1 -> ErrForbidden (exercises userExists!)
	if _, err := authSvc.UpdateUser(context.Background(), u1.User.ID, "", "admin", "", map[string]any{"name": "Hack"}, usersCol, c2); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden when u2 updates u1, got %v", err)
	}
	// u1 can update self (role escalation ignored)
	if updated, err := authSvc.UpdateUser(context.Background(), u1.User.ID, "u1new@boost.com", "superuser", "newpass123", map[string]any{"name": "U1"}, usersCol, c1); err != nil || updated["email"] != "u1new@boost.com" {
		t.Fatalf("expected u1 self-update ok, got %v err=%v", updated, err)
	}

	// 4. Test LoadAppSettingsFromDB
	_, _ = pool.Exec(context.Background(), `INSERT INTO _settings (key, value) VALUES ('app_settings', '{"app_name":"LoadedApp","accent_color":"#1856c9"}') ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`)
	loaded := LoadAppSettingsFromDB(context.Background(), pool)
	if loaded.AppName != "LoadedApp" || loaded.AccentColor != "#1856c9" {
		t.Fatalf("unexpected loaded app settings: %+v", loaded)
	}

	// 5. Test flipOperator & toFloat64 & EvalFilterOnData edge cases
	for _, op := range []string{">", ">=", "<", "<=", "="} {
		_ = flipOperator(op)
	}
	if _, ok := toFloat64(int(5)); !ok {
		t.Fatalf("toFloat64 int failed")
	}
	if _, ok := toFloat64(int64(5)); !ok {
		t.Fatalf("toFloat64 int64 failed")
	}
	if _, ok := toFloat64("3.14"); !ok {
		t.Fatalf("toFloat64 string failed")
	}
	if _, ok := toFloat64(struct{}{}); ok {
		t.Fatalf("toFloat64 struct should fail")
	}
	ok, err := EvalFilterOnData(`score >= 10 && @request.auth.email = 'o1@test.com' && @request.auth.role = 'user' && title ~ 'mi' && missing = null`,
		map[string]any{"score": 42, "title": "Mine"}, owner1)
	if err != nil || !ok {
		t.Fatalf("expected EvalFilterOnData true, got ok=%v err=%v", ok, err)
	}

	// 6. Test all models.FieldDef.ToPostgresType() branches
	for _, ft := range []string{"text", "editor", "number", "integer", "float", "boolean", "email", "url", "datetime", "autodate", "file", "relation", "select", "json", "geopoint"} {
		if _, err := (models.FieldDef{Name: "f", Type: ft}).ToPostgresType(); err != nil {
			t.Fatalf("ToPostgresType(%s) failed: %v", ft, err)
		}
	}
	if _, err := (models.FieldDef{Name: "v", Type: "vector", Dimension: 3}).ToPostgresType(); err != nil {
		t.Fatalf("ToPostgresType(vector) failed: %v", err)
	}
	if _, err := (models.FieldDef{Name: "bad", Type: "unknown_type"}).ToPostgresType(); err == nil {
		t.Fatalf("expected error for unknown field type")
	}
}
