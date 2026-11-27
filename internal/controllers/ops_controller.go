package controllers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/middlewares"
	"github.com/techasit5415/microbase/internal/response"
	"github.com/techasit5415/microbase/internal/services"
)

type MailConfig struct {
	Enabled       bool   `json:"enabled"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	SenderName    string `json:"sender_name"`
	SenderAddress string `json:"sender_address"`
}

// OpsController handles PocketBase-style SMTP Mail, Backups, and Cron jobs.
type OpsController struct {
	pool            *pgxpool.Pool
	backupDir       string
	databaseURL     string // active pool DSN used for pg_dump / psql
	configuredDBURL string // latest DSN saved to .env (may differ until restart)
	onRestore       func(ctx context.Context)
}

func NewOpsController(pool *pgxpool.Pool, backupDir string) *OpsController {
	_ = os.MkdirAll(backupDir, 0o755)
	dsn := os.Getenv("DATABASE_URL")
	return &OpsController{
		pool:            pool,
		backupDir:       backupDir,
		databaseURL:     dsn,
		configuredDBURL: dsn,
	}
}

// SetOnRestore registers a hook executed after a successful database restore (e.g. flushing schema cache).
func (h *OpsController) SetOnRestore(fn func(ctx context.Context)) {
	h.onRestore = fn
}

func (h *OpsController) loadMailConfig(ctx context.Context) MailConfig {
	var raw string
	_ = h.pool.QueryRow(ctx, `SELECT value FROM _settings WHERE key = 'mail_config'`).Scan(&raw)
	var cfg MailConfig
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	return cfg
}

// GetMail handles GET /api/settings/mail.
func (h *OpsController) GetMail(c echo.Context) error {
	cfg := h.loadMailConfig(c.Request().Context())
	return response.OK(c, map[string]any{
		"enabled":        cfg.Enabled,
		"host":           cfg.Host,
		"port":           cfg.Port,
		"username":       cfg.Username,
		"has_password":   cfg.Password != "",
		"sender_name":    cfg.SenderName,
		"sender_address": cfg.SenderAddress,
	})
}

// UpdateMail handles PUT /api/settings/mail.
func (h *OpsController) UpdateMail(c echo.Context) error {
	var req MailConfig
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	if req.Port == 0 {
		req.Port = 587
	}
	if strings.TrimSpace(req.Password) == "" {
		prev := h.loadMailConfig(c.Request().Context())
		if strings.EqualFold(strings.TrimSpace(req.Host), strings.TrimSpace(prev.Host)) &&
			req.Port == prev.Port &&
			strings.TrimSpace(req.Username) == strings.TrimSpace(prev.Username) {
			req.Password = prev.Password
		}
	}
	encoded, _ := json.Marshal(req)
	_, err := h.pool.Exec(c.Request().Context(), `
		INSERT INTO _settings (key, value, updated_at)
		VALUES ('mail_config', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		string(encoded),
	)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{"saved": true, "has_password": req.Password != ""})
}

// TestMail handles POST /api/settings/mail/test.
func (h *OpsController) TestMail(c echo.Context) error {
	var req struct {
		To string `json:"to"`
	}
	if err := c.Bind(&req); err != nil || !isValidEmailAddress(req.To) {
		return response.Error(c, http.StatusBadRequest, "INVALID_EMAIL", "valid recipient email is required")
	}
	cfg := h.loadMailConfig(c.Request().Context())
	if !cfg.Enabled || cfg.Host == "" {
		return response.Error(c, http.StatusBadRequest, "SMTP_DISABLED", "SMTP mail is not enabled or host is empty")
	}
	from := cfg.SenderAddress
	if from == "" {
		from = cfg.Username
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	msg := fmt.Sprintf("From: %s <%s>\r\nTo: %s\r\nSubject: Microbase SMTP Test\r\n\r\nHello from Microbase! Your SMTP configuration is working.\r\n",
		sanitizeSMTPHeader(cfg.SenderName), sanitizeSMTPHeader(from), sanitizeSMTPHeader(req.To))
	if err := smtp.SendMail(addr, auth, from, []string{req.To}, []byte(msg)); err != nil {
		return response.Error(c, http.StatusBadGateway, "SMTP_FAILED", err.Error())
	}
	return response.OK(c, map[string]any{"sent_to": req.To})
}

// ListBackups handles GET /api/backups.
func (h *OpsController) ListBackups(c echo.Context) error {
	entries, err := os.ReadDir(h.backupDir)
	if err != nil {
		return response.OK(c, map[string]any{"items": []any{}})
	}
	items := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, map[string]any{
			"key":      e.Name(),
			"size":     info.Size(),
			"modified": info.ModTime().UTC(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i]["key"].(string) > items[j]["key"].(string)
	})
	return response.OK(c, map[string]any{"items": items})
}

// CreateBackup handles POST /api/backups using pg_dump.
func (h *OpsController) CreateBackup(c echo.Context) error {
	if h.databaseURL == "" {
		return response.Error(c, http.StatusBadRequest, "NO_DATABASE", "DATABASE_URL is not configured")
	}
	name, size, err := services.CreateSQLBackup(c.Request().Context(), h.databaseURL, h.backupDir)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "BACKUP_FAILED", err.Error())
	}
	return response.Created(c, map[string]any{"key": name, "size": size})
}

// DownloadBackup handles GET /api/backups/:name.
func (h *OpsController) DownloadBackup(c echo.Context) error {
	safe := filepath.Base(c.Param("name"))
	full := filepath.Join(h.backupDir, safe)
	if _, err := os.Stat(full); os.IsNotExist(err) {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "backup not found")
	}
	return c.Attachment(full, safe)
}

// DeleteBackup handles DELETE /api/backups/:name.
func (h *OpsController) DeleteBackup(c echo.Context) error {
	safe := filepath.Base(c.Param("name"))
	full := filepath.Join(h.backupDir, safe)
	if err := os.Remove(full); err != nil {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "backup not found")
	}
	return response.OK(c, map[string]any{"deleted": safe})
}

// ListCrons handles GET /api/crons.
func (h *OpsController) ListCrons(c echo.Context) error {
	items := []services.CronJob{
		{
			ID:          "__mbLogsPrune__",
			Expression:  "0 3 * * *",
			Kind:        "system",
			Description: "Prune _logs older than 7 days (keep latest 5,000 rows)",
			Enabled:     true,
			IsSystem:    true,
		},
		{
			ID:          "__mbAutoBackup__",
			Expression:  "0 2 * * *",
			Kind:        "system",
			Description: "Create PostgreSQL SQL snapshot via pg_dump",
			Enabled:     true,
			IsSystem:    true,
		},
	}

	rows, err := h.pool.Query(c.Request().Context(), `
		SELECT id, expression, kind, target, method, payload, description, enabled, last_run_at, last_status
		FROM _crons ORDER BY created_at DESC`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var j services.CronJob
			if err := rows.Scan(&j.ID, &j.Expression, &j.Kind, &j.Target, &j.Method, &j.Payload, &j.Description, &j.Enabled, &j.LastRunAt, &j.LastStatus); err == nil {
				items = append(items, j)
			}
		}
	}
	return response.OK(c, map[string]any{"items": items})
}

// CreateCron handles POST /api/crons (creates a custom HTTP or SQL cron job).
func (h *OpsController) CreateCron(c echo.Context) error {
	var req services.CronJob
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid cron payload")
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Expression = strings.TrimSpace(req.Expression)
	req.Kind = strings.ToLower(strings.TrimSpace(req.Kind))
	req.Target = strings.TrimSpace(req.Target)
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	if req.ID == "" || strings.HasPrefix(req.ID, "__mb") {
		return response.Error(c, http.StatusBadRequest, "INVALID_ID", "cron id is required and cannot start with __mb")
	}
	if err := services.ValidateCronExpr(req.Expression); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_CRON", err.Error())
	}
	if req.Kind != "http" && req.Kind != "sql" {
		return response.Error(c, http.StatusBadRequest, "INVALID_KIND", "kind must be 'http' or 'sql'")
	}
	if req.Target == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_TARGET", "target URL or SQL query is required")
	}
	if req.Kind == "http" && req.Method == "" {
		req.Method = http.MethodPost
	}

	_, err := h.pool.Exec(c.Request().Context(), `
		INSERT INTO _crons (id, expression, kind, target, method, payload, description, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		req.ID, req.Expression, req.Kind, req.Target, req.Method, req.Payload, req.Description, req.Enabled,
	)
	if err != nil {
		return response.Error(c, http.StatusConflict, "CRON_EXISTS", err.Error())
	}
	return response.Created(c, req)
}

// UpdateCron handles PATCH /api/crons/:id (toggle enabled or update schedule/target).
func (h *OpsController) UpdateCron(c echo.Context) error {
	id := c.Param("id")
	var req struct {
		Enabled    *bool   `json:"enabled"`
		Expression *string `json:"expression"`
		Target     *string `json:"target"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid payload")
	}
	if req.Expression != nil {
		if err := services.ValidateCronExpr(*req.Expression); err != nil {
			return response.Error(c, http.StatusBadRequest, "INVALID_CRON", err.Error())
		}
		_, _ = h.pool.Exec(c.Request().Context(), `UPDATE _crons SET expression = $2 WHERE id = $1`, id, strings.TrimSpace(*req.Expression))
	}
	if req.Target != nil {
		_, _ = h.pool.Exec(c.Request().Context(), `UPDATE _crons SET target = $2 WHERE id = $1`, id, strings.TrimSpace(*req.Target))
	}
	if req.Enabled != nil {
		_, _ = h.pool.Exec(c.Request().Context(), `UPDATE _crons SET enabled = $2 WHERE id = $1`, id, *req.Enabled)
	}
	return response.OK(c, map[string]any{"updated": id})
}

// DeleteCron handles DELETE /api/crons/:id.
func (h *OpsController) DeleteCron(c echo.Context) error {
	id := c.Param("id")
	if strings.HasPrefix(id, "__mb") {
		return response.Error(c, http.StatusForbidden, "SYSTEM_CRON", "built-in system crons cannot be deleted")
	}
	tag, err := h.pool.Exec(c.Request().Context(), `DELETE FROM _crons WHERE id = $1`, id)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "DB_ERROR", err.Error())
	}
	if tag.RowsAffected() == 0 {
		return response.Error(c, http.StatusNotFound, "CRON_NOT_FOUND", "cron job not found")
	}
	return response.OK(c, map[string]any{"deleted": id})
}

// RunCron handles POST /api/crons/:id/run (supports both built-in and custom HTTP/SQL crons).
func (h *OpsController) RunCron(c echo.Context) error {
	id := c.Param("id")
	switch id {
	case "__mbLogsPrune__":
		pruned, err := services.RunLogsPrune(c.Request().Context(), h.pool)
		if err != nil {
			return response.Error(c, http.StatusInternalServerError, "CRON_FAILED", err.Error())
		}
		return response.OK(c, map[string]any{"id": id, "result": fmt.Sprintf("pruned %d rows", pruned)})
	case "__mbAutoBackup__":
		return h.CreateBackup(c)
	default:
		var job services.CronJob
		err := h.pool.QueryRow(c.Request().Context(), `
			SELECT id, expression, kind, target, method, payload, description, enabled
			FROM _crons WHERE id = $1`, id,
		).Scan(&job.ID, &job.Expression, &job.Kind, &job.Target, &job.Method, &job.Payload, &job.Description, &job.Enabled)
		if err != nil {
			return response.Error(c, http.StatusNotFound, "CRON_NOT_FOUND", "cron job not found")
		}
		msg, runErr := services.ExecuteCustomCron(c.Request().Context(), h.pool, job)
		if runErr != nil {
			return response.Error(c, http.StatusBadGateway, "CRON_FAILED", msg)
		}
		return response.OK(c, map[string]any{"id": id, "result": msg})
	}
}

// UploadBackup handles POST /api/backups/upload (multipart field "file").
func (h *OpsController) UploadBackup(c echo.Context) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "MISSING_FILE", "form field 'file' is required")
	}
	safe := filepath.Base(fh.Filename)
	if !strings.HasSuffix(strings.ToLower(safe), ".sql") {
		return response.Error(c, http.StatusBadRequest, "INVALID_FILE", "only .sql backup files are supported")
	}
	src, err := fh.Open()
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, 100<<20))
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	if err := os.WriteFile(filepath.Join(h.backupDir, safe), data, 0o644); err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.Created(c, map[string]any{"key": safe, "size": len(data)})
}

var (
	createVectorExtRe  = regexp.MustCompile(`(?i)\bCREATE\s+EXTENSION\b[^;]*\b"?vector"?\b`)
	commentVectorExtRe = regexp.MustCompile(`(?i)\bCOMMENT\s+ON\s+EXTENSION\s+"?vector"?\b`)
	vectorColumnTypeRe = regexp.MustCompile(`(?i)(?:\b(?:public\.)?vector\s*\(\s*\d+\s*\)|::(?:public\.)?vector\b)`)
	copyProgramSQLRe   = regexp.MustCompile(`(?i)\bCOPY\b[^;]*\bPROGRAM\b`)
)

// validateSQLBackupForRestore blocks dangerous psql meta-commands (e.g. \!, \i, \copy, \o, \connect)
// and SQL COPY ... PROGRAM statements to prevent OS command execution during backup restore.
func validateSQLBackupForRestore(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open backup for validation: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	inCopyData := false
	lineNum := 0
	for sc.Scan() {
		lineNum++
		line := sc.Text()
		trimmed := strings.TrimSpace(line)

		if inCopyData {
			if trimmed == `\.` {
				inCopyData = false
			}
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		if strings.HasPrefix(trimmed, `\`) {
			// Disallow all psql backslash meta-commands outside COPY data blocks (prevents \!, \i, \o, \g |cmd, \copy, \connect).
			return fmt.Errorf("backup rejected at line %d: psql meta-commands (%q) are not allowed for security", lineNum, trimmed[:min(24, len(trimmed))])
		}
		if copyProgramSQLRe.MatchString(trimmed) {
			return fmt.Errorf("backup rejected at line %d: SQL COPY ... PROGRAM is not allowed for security", lineNum)
		}
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "COPY ") && strings.Contains(upper, " FROM STDIN") {
			inCopyData = true
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("scan backup for validation: %w", err)
	}
	return nil
}

// buildPGToolCommand builds a pg_dump/psql exec.Cmd passing PostgreSQL credentials via environment variables
// instead of CLI arguments so passwords are never exposed in process listings (ps aux / /proc/cmdline).
func buildPGToolCommand(ctx context.Context, binPath, databaseURL string, args ...string) *exec.Cmd {
	host, port, user, pass, dbName, sslmode := parseDBURL(databaseURL)
	cmd := exec.CommandContext(ctx, binPath, args...)
	env := os.Environ()
	if host != "" {
		env = append(env, "PGHOST="+host)
	}
	if port != "" {
		env = append(env, "PGPORT="+port)
	}
	if user != "" {
		env = append(env, "PGUSER="+user)
	}
	if pass != "" {
		env = append(env, "PGPASSWORD="+pass)
	}
	if dbName != "" {
		env = append(env, "PGDATABASE="+dbName)
	}
	if sslmode != "" {
		env = append(env, "PGSSLMODE="+sslmode)
	}
	cmd.Env = env
	return cmd
}

// inspectBackupVectorUsage checks whether a .sql backup references the pgvector extension
// and whether any table/query actually uses a vector(N) column type.
func inspectBackupVectorUsage(path string) (hasExt bool, usesVectorCol bool) {
	f, err := os.Open(path)
	if err != nil {
		return false, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		if createVectorExtRe.MatchString(line) || commentVectorExtRe.MatchString(line) {
			hasExt = true
		}
		if vectorColumnTypeRe.MatchString(line) {
			usesVectorCol = true
		}
	}
	return hasExt, usesVectorCol
}

// stripVectorExtensionToTemp writes a copy of the .sql backup without CREATE/COMMENT EXTENSION vector
// statements so backups that do not use any vector(...) columns can be restored on PostgreSQL servers
// without pgvector installed.
func stripVectorExtensionToTemp(srcPath string) (string, error) {
	in, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer in.Close()

	out, err := os.CreateTemp("", "microbase_restore_novec_*.sql")
	if err != nil {
		return "", err
	}
	outPath := out.Name()
	bw := bufio.NewWriter(out)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "--") && (createVectorExtRe.MatchString(trimmed) || commentVectorExtRe.MatchString(trimmed)) {
			_, _ = bw.WriteString("-- [microbase restore] skipped unused pgvector statement\n")
			continue
		}
		_, _ = bw.WriteString(line)
		_ = bw.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		_ = out.Close()
		_ = os.Remove(outPath)
		return "", err
	}
	if err := bw.Flush(); err != nil {
		_ = out.Close()
		_ = os.Remove(outPath)
		return "", err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(outPath)
		return "", err
	}
	return outPath, nil
}

// RestoreBackup handles POST /api/backups/:name/restore using psql.
func (h *OpsController) RestoreBackup(c echo.Context) error {
	safe := filepath.Base(c.Param("name"))
	full := filepath.Join(h.backupDir, safe)
	if _, err := os.Stat(full); os.IsNotExist(err) {
		return response.Error(c, http.StatusNotFound, "NOT_FOUND", "backup not found")
	}
	if h.databaseURL == "" {
		return response.Error(c, http.StatusBadRequest, "NO_DATABASE", "DATABASE_URL is not configured")
	}
	psqlBin, findErr := services.FindPostgresTool("psql")
	if findErr != nil {
		return response.Error(c, http.StatusInternalServerError, "RESTORE_FAILED", findErr.Error())
	}

	ctx := c.Request().Context()
	restorePath := full

	// Validate backup file against psql meta-command injection (\!, \i, \copy, etc.) and COPY PROGRAM before running psql.
	if valErr := validateSQLBackupForRestore(full); valErr != nil {
		return response.Error(c, http.StatusBadRequest, "RESTORE_REJECTED", valErr.Error())
	}

	// Pre-flight pgvector check:
	// - If the backup uses real vector(N) columns and target Postgres lacks pgvector -> abort before touching schema.
	// - If the backup only has an unused CREATE/COMMENT EXTENSION vector -> strip it automatically so restore succeeds.
	if hasExt, usesVectorCol := inspectBackupVectorUsage(full); hasExt || usesVectorCol {
		var available bool
		_ = h.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'vector')`).Scan(&available)
		if !available {
			if usesVectorCol {
				return response.Error(c, http.StatusBadRequest, "RESTORE_FAILED",
					`backup contains vector(...) columns requiring PostgreSQL extension "vector" (pgvector), which is not installed on the target server (original database was not modified)`)
			}
			filtered, stripErr := stripVectorExtensionToTemp(full)
			if stripErr != nil {
				return response.Error(c, http.StatusInternalServerError, "RESTORE_FAILED", fmt.Sprintf("failed to strip unused pgvector statements: %v", stripErr))
			}
			defer os.Remove(filtered)
			restorePath = filtered
		}
	}

	// Take a temporary safety snapshot before resetting the public schema so a failed restore
	// automatically rolls back instead of leaving an empty database.
	var safetyPath string
	if pgDumpBin, err := services.FindPostgresTool("pg_dump"); err == nil {
		if tmp, err := os.CreateTemp("", "microbase_pre_restore_*.sql"); err == nil {
			safetyPath = tmp.Name()
			_ = tmp.Close()
			defer os.Remove(safetyPath)
			dumpCmd := buildPGToolCommand(ctx, pgDumpBin, h.databaseURL, "--schema=public", "--clean", "--if-exists", "--no-owner", "--no-privileges", "-f", safetyPath)
			if dumpErr := dumpCmd.Run(); dumpErr != nil {
				_ = os.Remove(safetyPath)
				safetyPath = ""
			}
		}
	}

	// Reset public schema before restore so pre-created tables (e.g. users BIGINT vs TEXT, _collections UNIQUE)
	// never collide with the backup dump, even for dumps created without --clean.
	if _, resetErr := h.pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO PUBLIC;`); resetErr != nil {
		return response.Error(c, http.StatusInternalServerError, "RESTORE_FAILED", fmt.Sprintf("failed to reset schema before restore: %v", resetErr))
	}

	cmd := buildPGToolCommand(ctx, psqlBin, h.databaseURL, "-X", "-v", "ON_ERROR_STOP=1", "-f", restorePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		rollbackNote := ""
		if safetyPath != "" {
			rbCtx, rbCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer rbCancel()
			_, _ = h.pool.Exec(rbCtx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO PUBLIC;`)
			rbCmd := buildPGToolCommand(rbCtx, psqlBin, h.databaseURL, "-X", "-v", "ON_ERROR_STOP=1", "-f", safetyPath)
			if rbOut, rbErr := rbCmd.CombinedOutput(); rbErr != nil {
				rollbackNote = fmt.Sprintf("; automatic rollback also failed: %v (%s)", rbErr, strings.TrimSpace(string(rbOut)))
			} else {
				rollbackNote = "; original database was automatically restored"
			}
		}
		return response.Error(c, http.StatusInternalServerError, "RESTORE_FAILED", fmt.Sprintf("psql restore failed: %v (%s)%s", err, strings.TrimSpace(string(out)), rollbackNote))
	}

	// Refresh in-memory settings and flush schema cache so restored data is served immediately.
	services.LoadAppSettingsFromDB(ctx, h.pool)
	middlewares.LoadRateLimitConfigFromDB(ctx, h.pool)
	middlewares.LoadLogSettingsFromDB(ctx, h.pool)
	if h.onRestore != nil {
		h.onRestore(ctx)
	}

	return response.OK(c, map[string]any{"restored": safe})
}

// GetRateLimits handles GET /api/settings/ratelimits.
func (h *OpsController) GetRateLimits(c echo.Context) error {
	middlewares.LoadRateLimitConfigFromDB(c.Request().Context(), h.pool)
	return response.OK(c, middlewares.GetRateLimitConfig())
}

// UpdateRateLimits handles PUT /api/settings/ratelimits and hot-swaps enabled/rules live.
func (h *OpsController) UpdateRateLimits(c echo.Context) error {
	var req struct {
		Enabled *bool                       `json:"enabled"`
		Rules   []middlewares.RateLimitRule `json:"rules"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
	}
	current := middlewares.GetRateLimitConfig()
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	rules := req.Rules
	if rules == nil {
		rules = current.Rules
	}
	cfg := middlewares.RateLimitConfig{
		Enabled: enabled,
		Rules:   rules,
	}
	middlewares.SetRateLimitConfig(cfg)
	encoded, _ := json.Marshal(cfg)
	_, err := h.pool.Exec(c.Request().Context(), `
		INSERT INTO _settings (key, value, updated_at)
		VALUES ('rate_limit_rules', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		string(encoded),
	)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, cfg)
}

// GetLogSettings handles GET /api/settings/logs.
func (h *OpsController) GetLogSettings(c echo.Context) error {
	cfg := middlewares.LoadLogSettingsFromDB(c.Request().Context(), h.pool)
	return response.OK(c, cfg)
}

// UpdateLogSettings handles PUT /api/settings/logs.
func (h *OpsController) UpdateLogSettings(c echo.Context) error {
	var req struct {
		MaxDays   *int  `json:"max_days"`
		MinLevel  *int  `json:"min_level"`
		MaxBytes  *int  `json:"max_bytes"`
		LogIP     *bool `json:"log_ip"`
		LogAuthID *bool `json:"log_auth_id"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", "invalid log settings payload")
	}

	cfg := middlewares.GetLogSettings()
	if req.MaxDays != nil {
		if *req.MaxDays < 0 || *req.MaxDays > 365 {
			return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", "max_days must be between 0 and 365")
		}
		cfg.MaxDays = *req.MaxDays
	}
	if req.MinLevel != nil {
		if *req.MinLevel < -8 || *req.MinLevel > 12 {
			return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", "min_level must be between -8 and 12")
		}
		cfg.MinLevel = *req.MinLevel
	}
	if req.MaxBytes != nil {
		if *req.MaxBytes < 0 || *req.MaxBytes > 1048576 {
			return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", "max_bytes must be between 0 and 1048576")
		}
		cfg.MaxBytes = *req.MaxBytes
	}
	if req.LogIP != nil {
		cfg.LogIP = *req.LogIP
	}
	if req.LogAuthID != nil {
		cfg.LogAuthID = *req.LogAuthID
	}

	middlewares.SetLogSettings(cfg)
	encoded, _ := json.Marshal(cfg)
	_, err := h.pool.Exec(c.Request().Context(), `
		INSERT INTO _settings (key, value, updated_at)
		VALUES ('logs_settings', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		string(encoded),
	)
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	_, _ = h.pool.Exec(c.Request().Context(), `
		INSERT INTO _settings (key, value, updated_at)
		VALUES ('logs_max_days', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		fmt.Sprintf("%d", cfg.MaxDays),
	)
	if cfg.MaxDays == 0 {
		_ = middlewares.ClearLogs(c.Request().Context())
	}
	return response.OK(c, cfg)
}

// ClearLogs handles DELETE /api/logs.
func (h *OpsController) ClearLogs(c echo.Context) error {
	if err := middlewares.ClearLogs(c.Request().Context()); err != nil {
		return response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
	return response.OK(c, map[string]any{"cleared": true})
}

type dbConnRequest struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	SSLMode  string `json:"sslmode"`
}

func parseDBURL(raw string) (host, port, user, password, dbName, sslmode string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "localhost", "5432", "microbase", "", "microbase", "disable"
	}
	host = u.Hostname()
	port = u.Port()
	if port == "" {
		port = "5432"
	}
	if u.User != nil {
		user = u.User.Username()
		password, _ = u.User.Password()
	}
	dbName = strings.TrimLeft(u.Path, "/")
	sslmode = u.Query().Get("sslmode")
	if sslmode == "" {
		sslmode = "disable"
	}
	return
}

func buildDBURL(req dbConnRequest, fallbackPass string) string {
	pass := req.Password
	if pass == "" {
		pass = fallbackPass
	}
	port := strings.TrimSpace(req.Port)
	if port == "" {
		port = "5432"
	}
	ssl := strings.TrimSpace(req.SSLMode)
	if ssl == "" {
		ssl = "disable"
	}
	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(strings.TrimSpace(req.User), pass),
		Host:     fmt.Sprintf("%s:%s", strings.TrimSpace(req.Host), port),
		Path:     "/" + strings.TrimSpace(req.Database),
		RawQuery: "sslmode=" + url.QueryEscape(ssl),
	}
	return u.String()
}

// GetDatabase handles GET /api/settings/database (Superuser only, masks password).
func (h *OpsController) GetDatabase(c echo.Context) error {
	claims := middlewares.GetAuthClaims(c)
	if claims == nil || claims.Role != "superuser" {
		return response.Error(c, http.StatusForbidden, "FORBIDDEN", "only superusers can view database settings")
	}
	host, port, user, pass, dbName, ssl := parseDBURL(h.configuredDBURL)
	var pgVersion string
	_ = h.pool.QueryRow(c.Request().Context(), `SELECT version()`).Scan(&pgVersion)
	return response.OK(c, map[string]any{
		"host":         host,
		"port":         port,
		"user":         user,
		"has_password": pass != "",
		"database":     dbName,
		"sslmode":      ssl,
		"version":      pgVersion,
	})
}

func validateDBConnRequest(req dbConnRequest) error {
	for _, v := range []string{req.Host, req.Port, req.User, req.Database, req.SSLMode} {
		if strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("database connection parameters must not contain newlines")
		}
	}
	return nil
}

// TestDatabase handles POST /api/settings/database/test (tests connectivity via pgxpool.Ping).
func (h *OpsController) TestDatabase(c echo.Context) error {
	claims := middlewares.GetAuthClaims(c)
	if claims == nil || claims.Role != "superuser" {
		return response.Error(c, http.StatusForbidden, "FORBIDDEN", "only superusers can test database connections")
	}
	var req dbConnRequest
	if err := c.Bind(&req); err != nil || strings.TrimSpace(req.Host) == "" || strings.TrimSpace(req.Database) == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", "host and database name are required")
	}
	if err := validateDBConnRequest(req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	}
	prevHost, prevPort, prevUser, prevPass, _, _ := parseDBURL(h.configuredDBURL)
	reqPort := strings.TrimSpace(req.Port)
	if reqPort == "" {
		reqPort = "5432"
	}
	fallbackPass := ""
	if strings.EqualFold(strings.TrimSpace(req.Host), prevHost) && reqPort == prevPort && strings.TrimSpace(req.User) == prevUser {
		fallbackPass = prevPass
	}
	dsn := buildDBURL(req, fallbackPass)

	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()
	testPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "DB_CONNECT_FAILED", err.Error())
	}
	defer testPool.Close()
	if err := testPool.Ping(ctx); err != nil {
		return response.Error(c, http.StatusBadRequest, "DB_PING_FAILED", err.Error())
	}
	var ver string
	_ = testPool.QueryRow(ctx, `SELECT version()`).Scan(&ver)
	return response.OK(c, map[string]any{"connected": true, "version": ver})
}

// UpdateDatabase handles PUT /api/settings/database — tests connection and updates DATABASE_URL in .env.
func (h *OpsController) UpdateDatabase(c echo.Context) error {
	claims := middlewares.GetAuthClaims(c)
	if claims == nil || claims.Role != "superuser" {
		return response.Error(c, http.StatusForbidden, "FORBIDDEN", "only superusers can update database settings")
	}
	var req dbConnRequest
	if err := c.Bind(&req); err != nil || strings.TrimSpace(req.Host) == "" || strings.TrimSpace(req.Database) == "" {
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", "host and database name are required")
	}
	if err := validateDBConnRequest(req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	}
	prevHost, prevPort, prevUser, prevPass, _, _ := parseDBURL(h.configuredDBURL)
	reqPort := strings.TrimSpace(req.Port)
	if reqPort == "" {
		reqPort = "5432"
	}
	fallbackPass := ""
	if strings.EqualFold(strings.TrimSpace(req.Host), prevHost) && reqPort == prevPort && strings.TrimSpace(req.User) == prevUser {
		fallbackPass = prevPass
	}
	dsn := buildDBURL(req, fallbackPass)

	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()
	testPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return response.Error(c, http.StatusBadRequest, "DB_CONNECT_FAILED", err.Error())
	}
	if err := testPool.Ping(ctx); err != nil {
		testPool.Close()
		return response.Error(c, http.StatusBadRequest, "DB_PING_FAILED", "cannot connect to target PostgreSQL: "+err.Error())
	}
	testPool.Close()

	// Update DATABASE_URL in .env cleanly.
	envPath := ".env"
	content, _ := os.ReadFile(envPath)
	lines := strings.Split(string(content), "\n")
	replaced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "DATABASE_URL=") {
			lines[i] = "DATABASE_URL=" + dsn
			replaced = true
		}
	}
	if !replaced {
		lines = append(lines, "DATABASE_URL="+dsn)
	}
	if err := os.WriteFile(envPath, []byte(strings.TrimSpace(strings.Join(lines, "\n"))+"\n"), 0o600); err != nil {
		return response.Error(c, http.StatusInternalServerError, "ENV_WRITE_FAILED", err.Error())
	}
	// Keep h.databaseURL pointing to the active pool DSN until restart so pg_dump/psql
	// always target the same database as h.pool.
	h.configuredDBURL = dsn

	return response.OK(c, map[string]any{
		"saved":            true,
		"requires_restart": true,
		"host":             req.Host,
		"port":             req.Port,
		"database":         req.Database,
	})
}


// GetAppSettings handles GET /api/settings/app (public-safe UI branding for guests, full config for superusers).
func (h *OpsController) GetAppSettings(c echo.Context) error {
	cfg := services.GetCachedAppSettings()
	claims := middlewares.GetAuthClaims(c)
	if claims == nil || claims.Role != "superuser" {
		cfg.TrustedProxy = services.TrustedProxySettings{}
		cfg.SuperuserIPs = services.SuperuserIPSettings{}
	}
	return response.OK(c, cfg)
}

// UpdateAppSettings handles PUT /api/settings/app (Superuser only — persisted to _settings and hot-reloaded).
func (h *OpsController) UpdateAppSettings(c echo.Context) error {
	var req services.AppSettings
	if err := c.Bind(&req); err != nil {
		return response.Error(c, http.StatusBadRequest, "INVALID_JSON", "invalid application settings payload")
	}
	services.SetCachedAppSettings(req)
	saved := services.GetCachedAppSettings()
	raw, _ := json.Marshal(saved)
	_, err := h.pool.Exec(c.Request().Context(), `
		INSERT INTO _settings (key, value, updated_at) VALUES ('app_settings', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`, string(raw))
	if err != nil {
		return response.Error(c, http.StatusInternalServerError, "DB_ERROR", err.Error())
	}
	return response.OK(c, saved)
}
