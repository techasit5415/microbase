package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/config"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/services"
	"github.com/techasit5415/microbase/internal/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIWorkflows(t *testing.T) {
	pool := testutil.Database(t)
	t.Chdir(t.TempDir())
	ctx := context.Background()
	store := cache.NewSwitchableStore(cache.NewMemoryStore())
	defer store.Close()
	e := echo.New()
	Register(e, pool, store, &config.Config{JWTSecret: "coverage-secret"})
	defer middlewares.SetLogPool(nil)
	old := middlewares.GetRateLimitRules()
	defer middlewares.SetRateLimitRules(old)
	middlewares.SetRateLimitRules([]middlewares.RateLimitRule{{Label: "/", MaxRequests: 10000, DurationSec: 60}})
	admin := ""
	seq := 0
	call := func(method, path, body, token string, want int) map[string]any {
		t.Helper()
		seq++
		var out map[string]any
		t.Run(fmt.Sprintf("%03d_%s_%s", seq, method, path), func(t *testing.T) {
			req := httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != want {
				t.Fatalf("status %d want %d: %s", rec.Code, want, rec.Body.String())
			}
			if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
				if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
			}
		})
		return out
	}
	call("GET", "/api/collections", "", "", 401)
	call("GET", "/api/superusers/status", "", "", 200)
	otp, err := services.GenerateSetupOTP(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/superusers/setup", `{"email":"root@test.local","password":"password123","otp":"bad"}`, "", 401)
	result := call("POST", "/api/superusers/setup", fmt.Sprintf(`{"email":"root@test.local","password":"password123","otp":%q}`, otp), "", 201)
	if result == nil {
		t.Fatal("setup failed")
	}
	admin, _ = result["token"].(string)
	if admin == "" {
		t.Fatal("no admin token")
	}
	call("POST", "/api/superusers/setup", `{}`, "", 403)
	call("POST", "/api/superusers/login", `{"email":"root@test.local","password":"wrong"}`, "", 401)
	call("POST", "/api/superusers/login", `{"email":"root@test.local","password":"password123"}`, "", 200)
	call("GET", "/api/collections", "", admin, 200)
	call("POST", "/api/collections", `{"name":"bad-name"}`, admin, 400)
	schema := `{"name":"articles","id_type":"text","fields":[{"name":"title","type":"text","required":true},{"name":"secret","type":"text","hidden":true},{"name":"owner","type":"text"},{"name":"meta","type":"json"},{"name":"embedding","type":"vector","dimension":2}],"api_rules":{"list":"owner = @request.auth.id","view":"owner = @request.auth.id","create":"auth","update":"owner = @request.auth.id","delete":"owner = @request.auth.id"},"indexes":[{"name":"idx_title","columns":["title"]}]}`
	call("POST", "/api/collections", schema, admin, 201)
	call("POST", "/api/collections", schema, admin, 409)
	call("GET", "/api/collections/articles", "", admin, 200)
	call("GET", "/api/collections/missing", "", admin, 404)
	call("POST", "/api/auth/register", `{"email":"user@test.local","password":"password123","name":"Tester"}`, admin, 201)
	login := call("POST", "/api/auth/login", `{"email":"user@test.local","password":"password123"}`, "", 200)
	if login == nil {
		t.Fatal("login failed")
	}
	user := login["token"].(string)
	uid := fmt.Sprint(login["user"].(map[string]any)["id"])
	call("POST", "/api/auth/login", `{"email":"user@test.local","password":"wrong"}`, "", 401)
	call("GET", "/api/auth/me", "", user, 200)
	call("POST", "/api/auth/refresh", `{}`, user, 200)
	call("GET", "/api/collections", "", user, 403)
	record := call("POST", "/api/collections/articles/records", fmt.Sprintf(`{"title":"hello world","secret":"private","owner":%q,"meta":{"key":1},"embedding":[1,0]}`, uid), user, 201)
	if record == nil {
		t.Fatal("create failed")
	}
	if _, ok := record["secret"]; ok {
		t.Error("create leaked hidden field")
	}
	id := record["id"].(string)
	call("POST", "/api/collections/articles/records", `{"owner":"other"}`, user, 400)
	call("POST", "/api/collections/articles/records", `{"title":"other","owner":"other"}`, admin, 201)
	listed := call("GET", "/api/collections/articles/records?search=hello&sort=title&filter_title=hello%20world", "", user, 200)
	if listed != nil && listed["total_items"] != float64(1) {
		t.Error(listed)
	}
	call("GET", "/api/collections/articles/records?filter=id%20%3D", "", user, 400)
	call("GET", "/api/collections/articles/records?sort=missing", "", user, 400)
	call("GET", "/api/collections/articles/records?filter_missing=x", "", user, 400)
	call("GET", "/api/collections/articles/records/"+id+"?fields=id,title", "", user, 200)
	call("GET", "/api/collections/articles/records/"+id, "", "", 403)
	call("PATCH", "/api/collections/articles/records/"+id, `{"title":"updated"}`, user, 200)
	call("PATCH", "/api/collections/articles/records/"+id, `{}`, user, 400)
	call("POST", "/api/collections/articles/search/vector", `{"field":"embedding","vector":[1,0]}`, user, 200)
	call("POST", "/api/collections/articles/search/vector", `{"field":"embedding","vector":[1]}`, user, 400)
	call("GET", "/api/openapi.json", "", admin, 200)
	call("GET", "/api/schema/export", "", admin, 200)
	call("POST", "/api/schema/import", `{"collections":[{"name":"imported","fields":[]}]}`, admin, 200)
	call("POST", "/api/schema/import", `{"collections":[{"name":"imported","fields":[]}]}`, admin, 200)
	call("POST", "/api/collections/articles/duplicate", `{"new_name":"articles_copy"}`, admin, 201)
	call("POST", "/api/collections/articles_copy/truncate", `{}`, admin, 200)
	call("DELETE", "/api/collections/articles_copy", "", admin, 200)
	call("PATCH", "/api/collections/imported", `{"fields":[{"name":"label","type":"text"}]}`, admin, 200)
	call("PATCH", "/api/collections/imported", `{"fields":[{"name":"title","original_name":"label","type":"text","required":true}],"indexes":[{"name":"idx_imported_title","columns":["title"],"unique":true}]}`, admin, 200)
	call("PATCH", "/api/collections/imported", `{"fields":[],"indexes":[]}`, admin, 200)
	call("POST", "/api/collections", `{"name":"article_view","type":"view","view_query":"SELECT id,title FROM articles"}`, admin, 201)
	call("PATCH", "/api/collections/article_view", `{"view_query":"SELECT id,title FROM articles WHERE title IS NOT NULL"}`, admin, 200)
	call("DELETE", "/api/collections/article_view", "", admin, 200)
	call("DELETE", "/api/collections/articles/records/"+id, "", user, 200)
	call("GET", "/api/collections/articles/records/"+id, "", admin, 404)
	call("DELETE", "/api/collections/articles/records/"+id, "", admin, 404)
	// Settings persist and read back; no external services configured.
	for _, p := range []string{"mail", "s3", "redis", "logs", "ratelimits", "database", "auth-options", "app"} {
		call("GET", "/api/settings/"+p, "", admin, 200)
	}
	call("PUT", "/api/settings/mail", `{"enabled":false,"password":"test-secret"}`, admin, 200)
	call("PUT", "/api/settings/mail", `{"enabled":false}`, admin, 200)
	call("POST", "/api/settings/mail/test", `{"to":"nobody@test.local"}`, admin, 400)
	call("PUT", "/api/settings/s3", `{"enabled":false}`, admin, 200)
	call("PUT", "/api/settings/logs", `{"max_days":30}`, admin, 200)
	call("PUT", "/api/settings/logs", `{"max_days":0}`, admin, 400)
	call("PUT", "/api/settings/auth-options", `{"otp_enabled":true,"mfa_enabled":false}`, admin, 200)
	call("PUT", "/api/settings/app", `{"name":"Test","batch":{"enabled":true}}`, admin, 200)
	call("POST", "/api/batch", `{"requests":[{"url":"/api/collections"},{"url":"/api/batch"}]}`, admin, 200)
	call("POST", "/api/sql", `{"query":"SELECT 42 AS answer"}`, admin, 200)
	call("POST", "/api/sql", `{"query":"SELECT missing_column"}`, admin, 400)
	call("POST", "/api/sql", `{"query":""}`, admin, 400)
	call("GET", "/api/logs", "", admin, 200)
	call("GET", "/api/system/_superusers", "", admin, 200)
	call("GET", "/api/backups", "", admin, 200)
	call("POST", "/api/crons", `{"id":"test_sql","expression":"* * * * *","kind":"sql","target":"SELECT 1","enabled":false}`, admin, 201)
	call("PATCH", "/api/crons/test_sql", `{"enabled":false,"expression":"*/5 * * * *","target":"SELECT 2"}`, admin, 200)
	call("GET", "/api/crons", "", admin, 200)
	call("POST", "/api/crons/test_sql/run", `{}`, admin, 200)
	call("POST", "/api/crons/__mbLogsPrune__/run", `{}`, admin, 200)
	call("DELETE", "/api/crons/test_sql", "", admin, 200)
	call("DELETE", "/api/crons/test_sql", "", admin, 404)
	// Loopback webhook validates real HTTP delivery without contacting other people.
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer sink.Close()
	wh := call("POST", "/api/webhooks", fmt.Sprintf(`{"collection":"articles","event":"create","url":%q,"secret":"test"}`, sink.URL), admin, 201)
	if wh != nil {
		wid := fmt.Sprint(wh["id"])
		call("GET", "/api/webhooks", "", admin, 200)
		call("POST", "/api/webhooks/"+wid+"/test", `{}`, admin, 200)
		call("PATCH", "/api/webhooks/"+wid, `{"enabled":false}`, admin, 200)
		call("DELETE", "/api/webhooks/"+wid, "", admin, 200)
	}
	call("GET", "/api/oauth/providers", "", admin, 200)
	call("GET", "/api/auth/methods", "", "", 200)
	call("GET", "/api/auth/oauth/missing", "", "", 404)
	call("POST", "/api/auth/oauth/exchange", `{"code":"bad"}`, "", 400)
	// Seed one-time tokens in this test cache, then verify consumption and DB changes.
	store.Set(ctx, "verify:test", "user@test.local", time.Minute)
	call("POST", "/api/auth/confirm-verification", `{"token":"test"}`, "", 200)
	call("POST", "/api/auth/confirm-verification", `{"token":"test"}`, "", 400)
	store.Set(ctx, "pwreset:test", "user@test.local", time.Minute)
	call("POST", "/api/auth/confirm-password-reset", `{"token":"test","password":"changed123"}`, "", 200)
	call("POST", "/api/auth/login", `{"email":"user@test.local","password":"changed123"}`, "", 200)
	store.Set(ctx, "userotp:test", fmt.Sprintf(`{"user_id":"%s","email":"user@test.local","code":"123456"}`, uid), time.Minute)
	call("POST", "/api/auth/auth-with-otp", `{"otp_id":"test","password":"000000"}`, "", 401)
	call("POST", "/api/auth/auth-with-otp", `{"otp_id":"test","password":"123456"}`, "", 200)
	call("POST", "/api/users/"+uid+"/impersonate", `{}`, admin, 200)
	call("GET", "/api/users", "", admin, 200)
	call("GET", "/api/users/"+uid, "", admin, 200)
	call("PATCH", "/api/users/"+uid, `{"name":"Updated"}`, admin, 200)
	call("DELETE", "/api/users/"+uid, "", admin, 200)
	// Superuser lifecycle and last-admin protection.
	second := call("POST", "/api/superusers", `{"email":"second@test.local","password":"password123"}`, admin, 201)
	if second != nil {
		sid := fmt.Sprint(second["id"])
		call("PATCH", "/api/superusers/"+sid, `{"email":"renamed@test.local","password":"newpassword123"}`, admin, 200)
		call("PATCH", "/api/superusers/"+sid, `{"email":"renamed2@test.local"}`, admin, 200)
		call("DELETE", "/api/superusers/"+sid, "", admin, 200)
	}
	call("DELETE", "/api/superusers/1", "", admin, 400)
	// Custom auth collection with text IDs.
	call("POST", "/api/collections", `{"name":"members","type":"auth","id_type":"text","fields":[{"name":"nickname","type":"text"}],"api_rules":{"create":"public","view":"id = @request.auth.id"}}`, admin, 201)
	member := call("POST", "/api/collections/members/register", `{"email":"member@test.local","password":"password123","nickname":"Member"}`, "", 201)
	if member != nil {
		mt := member["token"].(string)
		call("POST", "/api/collections/members/auth-refresh", `{}`, mt, 200)
	}
	call("POST", "/api/collections/members/auth-with-password", `{"identity":"member@test.local","password":"password123"}`, "", 200)
	call("POST", "/api/collections/members/auth-with-password", `{"identity":"member@test.local","password":"wrong"}`, "", 401)
	call("POST", "/api/collections/members/register", `{"email":"member@test.local","password":"password123"}`, "", 409)
	call("POST", "/api/collections/articles/register", `{}`, admin, 400)
	call("PUT", "/api/oauth/providers/google", `{"enabled":true,"client_id":"test","client_secret":"secret"}`, admin, 200)
	provider := call("PUT", "/api/oauth/providers/google", `{"enabled":false,"client_id":"test"}`, admin, 200)
	if provider != nil && (provider["has_secret"] != true || provider["client_secret"] != nil) {
		t.Error("OAuth secret handling failed")
	}
	// Serial IDs and relation FK behavior.
	call("POST", "/api/collections", `{"name":"parents","fields":[{"name":"label","type":"text"}],"api_rules":{"view":"public"}}`, admin, 201)
	parent := call("POST", "/api/collections/parents/records", `{"label":"parent"}`, admin, 201)
	call("POST", "/api/collections", `{"name":"children","fields":[{"name":"parent","type":"relation","collection_name":"parents"}],"api_rules":{"view":"public","list":"public"}}`, admin, 201)
	if parent != nil {
		call("POST", "/api/collections/children/records", fmt.Sprintf(`{"parent":%v}`, parent["id"]), admin, 201)
		expanded := call("GET", "/api/collections/children/records?expand=parent", "", "", 200)
		if expanded != nil {
			items := expanded["items"].([]any)
			if len(items) != 1 || items[0].(map[string]any)["expand"] == nil {
				t.Error("relation expansion missing")
			}
		}
	}
	call("GET", "/api/collections/parents/records/not-an-integer", "", admin, 400)

	// malformed JSON on routes must be rejected, not panic.
	for _, p := range []string{"/api/collections", "/api/auth/login", "/api/webhooks", "/api/crons", "/api/sql", "/api/schema/import"} {
		call("POST", p, "{", admin, 400)
	}
}
