package hooks

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestHooksBeforeCreateMutateAndAbort(t *testing.T) {
	eng := NewEngine(nil)
	err := eng.LoadScript("posts.js", `
		onRecordBeforeCreate("posts", (e) => {
			if (!e.data.title || e.data.title === "forbidden") {
				throw new Error("title is not allowed");
			}
			e.data.slug = e.data.title.toLowerCase().replace(" ", "-");
		});
	`)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	// Valid payload gets mutated
	data := map[string]any{"title": "Hello World"}
	if err := eng.RunBefore("beforeCreate", "posts", map[string]any{"collection": "posts", "data": data}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if data["slug"] != "hello-world" {
		t.Fatalf("expected mutated slug 'hello-world', got %v", data["slug"])
	}

	// Forbidden payload aborts with custom error
	bad := map[string]any{"title": "forbidden"}
	err = eng.RunBefore("beforeCreate", "posts", map[string]any{"collection": "posts", "data": bad})
	if err == nil || !strings.Contains(err.Error(), "title is not allowed") {
		t.Fatalf("expected custom abort error, got %v", err)
	}
}

func TestHooksCustomRoute(t *testing.T) {
	eng := NewEngine(nil)
	err := eng.LoadScript("routes.js", `
		routerAdd("GET", "/api/custom/ping", (c) => {
			return { pong: true, who: c.query("name") || "guest" };
		});
	`)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	ec := echo.New()
	eng.RegisterRoutes(ec)

	req := httptest.NewRequest(http.MethodGet, "/api/custom/ping?name=microbase", nil)
	rec := httptest.NewRecorder()
	ec.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"who":"microbase"`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}
