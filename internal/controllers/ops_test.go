package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/testutil"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestBackupVectorInspectionAndStripping(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name        string
		sql         string
		wantExt     bool
		wantVecCols bool
	}{
		{"plain", "CREATE TABLE items (id INT);", false, false},
		{"commented", "-- CREATE EXTENSION IF NOT EXISTS vector;\nSELECT 1;", false, false},
		{"unused_ext", "CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;\nCOMMENT ON EXTENSION vector IS 'vector data type';\nCREATE TABLE items (id INT);", true, false},
		{"used_col", "CREATE EXTENSION \"vector\";\nCREATE TABLE docs (id INT, emb public.vector(1536));", true, true},
	}
	for _, tc := range cases {
		p := filepath.Join(dir, tc.name+".sql")
		if err := os.WriteFile(p, []byte(tc.sql), 0o644); err != nil {
			t.Fatal(err)
		}
		gotExt, gotCols := inspectBackupVectorUsage(p)
		if gotExt != tc.wantExt || gotCols != tc.wantVecCols {
			t.Fatalf("%s: got (%v,%v), want (%v,%v)", tc.name, gotExt, gotCols, tc.wantExt, tc.wantVecCols)
		}
	}

	unusedPath := filepath.Join(dir, "unused_ext.sql")
	strippedPath, err := stripVectorExtensionToTemp(unusedPath)
	if err != nil {
		t.Fatalf("stripVectorExtensionToTemp: %v", err)
	}
	defer os.Remove(strippedPath)
	raw, err := os.ReadFile(strippedPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "CREATE EXTENSION") || strings.Contains(text, "COMMENT ON EXTENSION") {
		t.Fatalf("expected vector extension statements to be stripped, got:\n%s", text)
	}
	if !strings.Contains(text, "CREATE TABLE items (id INT);") {
		t.Fatalf("expected normal SQL statements to be preserved, got:\n%s", text)
	}
}

func TestRestoreBackupRollsBackOnFailureAndFlushesOnSuccess(t *testing.T) {
	pool := testutil.Database(t)
	dsn := pool.Config().ConnString()
	t.Setenv("DATABASE_URL", dsn)

	dir := t.TempDir()
	h := NewOpsController(pool, dir)
	flushed := false
	h.SetOnRestore(func(_ context.Context) {
		flushed = true
	})

	// Seed a sentinel setting row to verify data survives a failed restore.
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO _settings (key, value, updated_at)
		VALUES ('restore_sentinel', 'keep_me', NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`)
	if err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}

	e := echo.New()
	e.POST("/backups", h.CreateBackup)
	e.POST("/backups/:name/restore", h.RestoreBackup)

	// 1. Create a valid backup first.
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("POST", "/backups", nil))
	if rec.Code != 201 {
		t.Fatalf("CreateBackup failed: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Key == "" {
		t.Fatalf("parse backup key: %v (%s)", err, rec.Body.String())
	}

	// 2. Restore a broken SQL file -> must fail AND automatically preserve sentinel data.
	badFile := filepath.Join(dir, "broken.sql")
	if err := os.WriteFile(badFile, []byte("THIS IS NOT VALID SQL;"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("POST", "/backups/broken.sql/restore", nil))
	if rec.Code != 500 {
		t.Fatalf("expected 500 on broken restore, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "original database was automatically restored") {
		t.Fatalf("expected automatic rollback confirmation in error, got: %s", rec.Body.String())
	}
	var sentinel string
	if err := pool.QueryRow(ctx, `SELECT value FROM _settings WHERE key = 'restore_sentinel'`).Scan(&sentinel); err != nil || sentinel != "keep_me" {
		t.Fatalf("expected sentinel row preserved after failed restore, got %q (err=%v)", sentinel, err)
	}
	if flushed {
		t.Fatalf("onRestore should not run when restore fails")
	}

	// 3. Restore the valid backup -> must succeed AND invoke onRestore hook.
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("POST", "/backups/"+created.Key+"/restore", nil))
	if rec.Code != 200 {
		t.Fatalf("expected 200 on valid restore, got %d: %s", rec.Code, rec.Body.String())
	}
	if !flushed {
		t.Fatalf("expected onRestore hook to be called after successful restore")
	}
}
