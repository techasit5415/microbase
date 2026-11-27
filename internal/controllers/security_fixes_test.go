package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/services"
	"github.com/techasit5415/microbase/internal/storage"
)

func TestValidateSQLBackupForRestoreBlocksMetaCommands(t *testing.T) {
	dir := t.TempDir()

	validSQL := `-- pg_dump sample
CREATE TABLE items (id INT, name TEXT);
COPY public.items (id, name) FROM stdin;
1	hello
2	\not_a_metacommand_inside_copy_data
\.
INSERT INTO items VALUES (3, 'ok');
`
	validPath := filepath.Join(dir, "valid.sql")
	if err := os.WriteFile(validPath, []byte(validSQL), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateSQLBackupForRestore(validPath); err != nil {
		t.Fatalf("expected valid dump to pass, got error: %v", err)
	}

	maliciousCases := []struct {
		name string
		sql  string
	}{
		{"shell_exec", "SELECT 1;\n\\! whoami\n"},
		{"include_file", "\\i /etc/passwd\n"},
		{"copy_program_sql", "COPY items FROM PROGRAM 'id';\n"},
		{"psql_copy_meta", "\\copy items FROM PROGRAM 'id'\n"},
		{"psql_output", "\\o /tmp/pwn\nSELECT 1;\n"},
	}

	for _, tc := range maliciousCases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, tc.name+".sql")
			if err := os.WriteFile(p, []byte(tc.sql), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := validateSQLBackupForRestore(p); err == nil {
				t.Fatalf("expected %s to be rejected, but it passed", tc.name)
			}
		})
	}
}

func TestGetAppSettingsMasksSecurityConfigForPublic(t *testing.T) {
	orig := services.GetCachedAppSettings()
	defer services.SetCachedAppSettings(orig)

	cfg := services.DefaultAppSettings()
	cfg.SuperuserIPs = services.SuperuserIPSettings{Enabled: true, AllowedIPs: []string{"10.0.0.5"}}
	cfg.TrustedProxy = services.TrustedProxySettings{Enabled: true, Headers: []string{"X-Custom-IP"}}
	services.SetCachedAppSettings(cfg)

	ctrl := NewOpsController(nil, t.TempDir())
	e := echo.New()

	// Public caller must NOT receive SuperuserIPs or TrustedProxy config
	req := httptest.NewRequest(http.MethodGet, "/api/settings/app", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := ctrl.GetAppSettings(c); err != nil {
		t.Fatal(err)
	}
	var pubOut services.AppSettings
	if err := json.Unmarshal(rec.Body.Bytes(), &pubOut); err != nil {
		t.Fatal(err)
	}
	if len(pubOut.SuperuserIPs.AllowedIPs) != 0 || pubOut.SuperuserIPs.Enabled {
		t.Fatalf("expected superuser_ips hidden for public, got %+v", pubOut.SuperuserIPs)
	}
	if len(pubOut.TrustedProxy.Headers) != 0 || pubOut.TrustedProxy.Enabled {
		t.Fatalf("expected trusted_proxy hidden for public, got %+v", pubOut.TrustedProxy)
	}

	// Superuser caller receives full config
	req2 := httptest.NewRequest(http.MethodGet, "/api/settings/app", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.Set("auth_claims", &services.AuthClaims{UserID: "1", Role: "superuser"})
	if err := ctrl.GetAppSettings(c2); err != nil {
		t.Fatal(err)
	}
	var adminOut services.AppSettings
	if err := json.Unmarshal(rec2.Body.Bytes(), &adminOut); err != nil {
		t.Fatal(err)
	}
	if !adminOut.SuperuserIPs.Enabled || len(adminOut.SuperuserIPs.AllowedIPs) != 1 {
		t.Fatalf("expected superuser_ips visible for superuser, got %+v", adminOut.SuperuserIPs)
	}
}

func TestMaskRedisURLAndDBConnValidation(t *testing.T) {
	masked, hasPass := maskRedisURL("redis://default:supersecret@127.0.0.1:6379/0")
	if !hasPass || strings.Contains(masked, "supersecret") || !strings.Contains(masked, "REDACTED") {
		t.Fatalf("expected masked redis URL, got %q (hasPass=%v)", masked, hasPass)
	}

	if err := validateDBConnRequest(dbConnRequest{Host: "localhost\nJWT_SECRET=bad", Database: "db"}); err == nil {
		t.Fatalf("expected newline in host to fail validation")
	}
	if err := validateDBConnRequest(dbConnRequest{Host: "localhost", Port: "5432", User: "pg", Database: "app"}); err != nil {
		t.Fatalf("expected valid dbConnRequest, got %v", err)
	}
}

func TestFileUploadRequiresAuthAndThumbAllowlist(t *testing.T) {
	store := storage.NewSwitchableStorage(t.TempDir(), nil)
	authSvc := services.NewAuthService(nil, "test-secret-123456")
	ctrl := NewFileController(store, authSvc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/files", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := ctrl.Upload(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated upload, got %d", rec.Code)
	}

	if _, _, _, ok := parseThumbSpec("123x456"); ok {
		t.Fatalf("expected arbitrary non-preset thumb dimension 123x456 to be rejected")
	}
	if w, h, _, ok := parseThumbSpec("100x100"); !ok || w != 100 || h != 100 {
		t.Fatalf("expected preset thumb 100x100 to be accepted")
	}
}
