package controllers

import (
	"bytes"
	"encoding/json"
	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/testutil"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBackupFileLifecycle(t *testing.T) {
	pool := testutil.Database(t)
	t.Setenv("DATABASE_URL", "")
	h := NewOpsController(pool, t.TempDir())
	e := echo.New()
	e.GET("/backups", h.ListBackups)
	e.POST("/backups", h.CreateBackup)
	e.POST("/upload", h.UploadBackup)
	e.GET("/backups/:name", h.DownloadBackup)
	e.DELETE("/backups/:name", h.DeleteBackup)
	e.POST("/backups/:name/restore", h.RestoreBackup)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	f, _ := mw.CreateFormFile("file", "test.sql")
	f.Write([]byte("SELECT 1;"))
	mw.Close()
	req := httptest.NewRequest("POST", "/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	for _, tc := range []struct {
		method, path string
		code         int
	}{{"GET", "/backups", 200}, {"GET", "/backups/test.sql", 200}, {"POST", "/backups/test.sql/restore", 400}, {"POST", "/backups", 400}, {"DELETE", "/backups/test.sql", 200}, {"GET", "/backups/test.sql", 404}, {"DELETE", "/backups/test.sql", 404}, {"POST", "/backups/test.sql/restore", 404}, {"POST", "/upload", 400}} {
		rec = httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != tc.code {
			t.Fatal(tc, rec.Code, rec.Body.String())
		}
		if tc.method == "GET" && tc.path == "/backups/test.sql" && rec.Code == 200 && rec.Body.String() != "SELECT 1;" {
			t.Fatal("backup content mismatch")
		}
	}
}
func TestDatabaseURLRoundTrip(t *testing.T) {
	raw := buildDBURL(dbConnRequest{Host: "localhost", User: "test user", Database: "test", Password: "p@ss/?#"}, "")
	host, port, user, pass, db, ssl := parseDBURL(raw)
	if host != "localhost" || port != "5432" || user != "test user" || pass != "p@ss/?#" || db != "test" || ssl != "disable" {
		t.Fatal(host, port, user, db, ssl)
	}
	if strings.Contains(raw, "p@ss") {
		t.Fatal("password not URL escaped")
	}
}
func TestProjectionAndRecordErrors(t *testing.T) {
	rec := projectFields(map[string]any{"id": 1, "title": "x", "secret": "y"}, "id,title")
	b, _ := json.Marshal(rec)
	if strings.Contains(string(b), "secret") {
		t.Fatal(string(b))
	}
}
