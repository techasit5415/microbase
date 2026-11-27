package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CronJob represents a scheduled job (built-in or user-defined).
type CronJob struct {
	ID          string     `json:"id"`
	Expression  string     `json:"expression"`
	Kind        string     `json:"kind"` // "system" | "http" | "sql"
	Target      string     `json:"target,omitempty"`
	Method      string     `json:"method,omitempty"`
	Payload     string     `json:"payload,omitempty"`
	Description string     `json:"description"`
	Enabled     bool       `json:"enabled"`
	IsSystem    bool       `json:"is_system"`
	LastRunAt   *time.Time `json:"last_run_at,omitempty"`
	LastStatus  string     `json:"last_status,omitempty"`
}

// ValidateCronExpr validates a standard 5-field cron expression (min hour dom month dow).
func ValidateCronExpr(expr string) error {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return fmt.Errorf("cron expression must have 5 fields (minute hour day month weekday)")
	}
	limits := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}
	for i, f := range fields {
		if _, err := matchCronField(f, limits[i][0], limits[i][0], limits[i][1]); err != nil {
			return fmt.Errorf("invalid cron field #%d (%q): %w", i+1, f, err)
		}
	}
	return nil
}

// CronMatches returns true if the given 5-field cron expression matches time t.
func CronMatches(expr string, t time.Time) bool {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return false
	}
	vals := []int{t.Minute(), t.Hour(), t.Day(), int(t.Month()), int(t.Weekday())}
	limits := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}
	for i := 0; i < 5; i++ {
		ok, err := matchCronField(fields[i], vals[i], limits[i][0], limits[i][1])
		if err != nil || !ok {
			// Also allow Sunday as 7
			if i == 4 && vals[i] == 0 {
				ok7, err7 := matchCronField(fields[i], 7, 0, 7)
				if err7 == nil && ok7 {
					continue
				}
			}
			return false
		}
	}
	return true
}

func matchCronField(field string, val, minVal, maxVal int) (bool, error) {
	matched := false
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return false, fmt.Errorf("empty segment")
		}
		step := 1
		hasStep := strings.Contains(part, "/")
		if strings.Contains(part, "/") {
			sp := strings.Split(part, "/")
			if len(sp) != 2 {
				return false, fmt.Errorf("invalid step %q", part)
			}
			n, err := strconv.Atoi(sp[1])
			if err != nil || n <= 0 {
				return false, fmt.Errorf("invalid step number %q", sp[1])
			}
			step = n
			part = sp[0]
		}
		if part == "*" {
			if (val-minVal)%step == 0 {
				matched = true
			}
			continue
		}
		if strings.Contains(part, "-") {
			rp := strings.Split(part, "-")
			if len(rp) != 2 {
				return false, fmt.Errorf("invalid range %q", part)
			}
			lo, err1 := strconv.Atoi(rp[0])
			hi, err2 := strconv.Atoi(rp[1])
			if err1 != nil || err2 != nil || lo < minVal || hi > maxVal || lo > hi {
				return false, fmt.Errorf("range out of bounds %q", part)
			}
			if val >= lo && val <= hi && (val-lo)%step == 0 {
				matched = true
			}
			continue
		}
		num, err := strconv.Atoi(part)
		if err != nil || num < minVal || num > maxVal {
			return false, fmt.Errorf("value out of bounds %q", part)
		}
		if val == num || (hasStep && val >= num && (val-num)%step == 0) {
			matched = true
		}
	}
	return matched, nil
}

// ExecuteCustomCron runs a custom HTTP or SQL cron job and persists last_run_at & last_status.
func ExecuteCustomCron(ctx context.Context, pool *pgxpool.Pool, job CronJob) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var statusMsg string
	var runErr error

	switch strings.ToLower(job.Kind) {
	case "sql":
		tag, err := pool.Exec(runCtx, job.Target)
		if err != nil {
			runErr = err
			statusMsg = "error: " + err.Error()
		} else {
			statusMsg = fmt.Sprintf("ok (%d rows affected)", tag.RowsAffected())
		}
	case "http":
		method := strings.ToUpper(strings.TrimSpace(job.Method))
		if method == "" {
			method = http.MethodPost
		}
		var bodyReader io.Reader
		if strings.TrimSpace(job.Payload) != "" && method != http.MethodGet && method != http.MethodHead {
			bodyReader = bytes.NewBufferString(job.Payload)
		}
		req, err := http.NewRequestWithContext(runCtx, method, job.Target, bodyReader)
		if err != nil {
			runErr = err
			statusMsg = "error: " + err.Error()
		} else {
			if bodyReader != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set("User-Agent", "Microbase-Cron/1.0")
			resp, err := NewSafeHTTPClient(15 * time.Second).Do(req)
			if err != nil {
				runErr = err
				statusMsg = "error: " + err.Error()
			} else {
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
				_ = resp.Body.Close()
				if resp.StatusCode >= 400 {
					runErr = fmt.Errorf("HTTP %d", resp.StatusCode)
					statusMsg = fmt.Sprintf("error: HTTP %d", resp.StatusCode)
				} else {
					statusMsg = fmt.Sprintf("ok (HTTP %d)", resp.StatusCode)
				}
			}
		}
	default:
		runErr = fmt.Errorf("unsupported cron kind %q", job.Kind)
		statusMsg = "error: " + runErr.Error()
	}

	if len(statusMsg) > 250 {
		statusMsg = statusMsg[:250]
	}
	_, _ = pool.Exec(context.Background(),
		`UPDATE _crons SET last_run_at = NOW(), last_status = $2 WHERE id = $1`,
		job.ID, statusMsg,
	)
	return statusMsg, runErr
}

// CronSchedulerOption configures optional built-in system cron behaviors.
type CronSchedulerOption func(*cronSchedulerConfig)

type cronSchedulerConfig struct {
	backupDir   string
	databaseURL string
}

// WithAutoBackup enables the built-in __mbAutoBackup__ scheduled job (0 2 * * *).
func WithAutoBackup(backupDir, databaseURL string) CronSchedulerOption {
	return func(c *cronSchedulerConfig) {
		c.backupDir = backupDir
		c.databaseURL = databaseURL
	}
}

// RunLogsPrune deletes _logs older than 7 days (built-in __mbLogsPrune__ job).
func RunLogsPrune(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM _logs WHERE created_at < NOW() - INTERVAL '7 days'`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// CreateSQLBackup creates a timestamped PostgreSQL .sql snapshot via pg_dump.
func CreateSQLBackup(ctx context.Context, databaseURL, backupDir string) (string, int64, error) {
	if databaseURL == "" {
		return "", 0, fmt.Errorf("DATABASE_URL is not configured")
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("failed to create backup dir: %w", err)
	}
	pgDumpBin, err := FindPostgresTool("pg_dump")
	if err != nil {
		return "", 0, err
	}
	name := fmt.Sprintf("microbase_%s.sql", time.Now().Format("20060102_150405"))
	outPath := filepath.Join(backupDir, name)
	cmd := exec.CommandContext(ctx, pgDumpBin, "--schema=public", "--clean", "--if-exists", "--no-owner", "--no-privileges", databaseURL, "-f", outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(outPath)
		return "", 0, fmt.Errorf("pg_dump failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	info, _ := os.Stat(outPath)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}
	return name, size, nil
}

func postgresToolPatterns(goos, name string) []string {
	if goos == "windows" {
		exe := name + ".exe"
		return []string{
			`C:\Program Files\PostgreSQL\*\bin\` + exe,
			`C:\Program Files (x86)\PostgreSQL\*\bin\` + exe,
		}
	}
	return []string{
		"/opt/homebrew/opt/postgresql@*/bin/" + name,
		"/usr/local/opt/postgresql@*/bin/" + name,
		"/usr/lib/postgresql/*/bin/" + name,
	}
}

// FindPostgresTool locates pg_dump or psql on PATH, with automatic fallback to standard
// Windows ("C:\Program Files\PostgreSQL\*\bin"), macOS Homebrew, and Linux locations.
func FindPostgresTool(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	var matches []string
	for _, pat := range postgresToolPatterns(runtime.GOOS, name) {
		if found, err := filepath.Glob(pat); err == nil {
			matches = append(matches, found...)
		}
	}
	if len(matches) > 0 {
		sort.Sort(sort.Reverse(sort.StringSlice(matches)))
		return matches[0], nil
	}
	return "", fmt.Errorf("%q executable not found in PATH (install PostgreSQL client tools or add PostgreSQL bin folder to PATH)", name)
}

// StartCronScheduler launches a background goroutine that ticks every minute and runs matching built-in & custom crons.
func StartCronScheduler(pool *pgxpool.Pool, opts ...CronSchedulerOption) {
	if pool == nil {
		return
	}
	cfg := cronSchedulerConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	go func() {
		// Align to next minute boundary.
		now := time.Now()
		next := now.Truncate(time.Minute).Add(time.Minute)
		time.Sleep(time.Until(next))
		runDueCrons(pool, next, cfg)

		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for tick := range ticker.C {
			runDueCrons(pool, tick, cfg)
		}
	}()
	slog.Info("cron scheduler started")
}

func runDueCrons(pool *pgxpool.Pool, now time.Time, cfgs ...cronSchedulerConfig) {
	var cfg cronSchedulerConfig
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	// Built-in system crons matching ListCrons schedules.
	if CronMatches("0 3 * * *", now) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			pruned, err := RunLogsPrune(ctx, pool)
			if err != nil {
				slog.Warn("system cron failed", "id", "__mbLogsPrune__", "error", err)
			} else {
				slog.Info("system cron executed", "id", "__mbLogsPrune__", "pruned_rows", pruned)
			}
		}()
	}
	if cfg.databaseURL != "" && cfg.backupDir != "" && CronMatches("0 2 * * *", now) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			name, size, err := CreateSQLBackup(ctx, cfg.databaseURL, cfg.backupDir)
			if err != nil {
				slog.Warn("system cron failed", "id", "__mbAutoBackup__", "error", err)
			} else {
				slog.Info("system cron executed", "id", "__mbAutoBackup__", "file", name, "size", size)
			}
		}()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `SELECT id, expression, kind, target, method, payload, description, enabled FROM _crons WHERE enabled = TRUE`)
	if err != nil {
		return
	}
	defer rows.Close()

	var due []CronJob
	for rows.Next() {
		var j CronJob
		if err := rows.Scan(&j.ID, &j.Expression, &j.Kind, &j.Target, &j.Method, &j.Payload, &j.Description, &j.Enabled); err == nil {
			if CronMatches(j.Expression, now) {
				due = append(due, j)
			}
		}
	}

	for _, job := range due {
		go func(j CronJob) {
			msg, err := ExecuteCustomCron(context.Background(), pool, j)
			if err != nil {
				slog.Warn("cron job failed", "id", j.ID, "kind", j.Kind, "error", err)
			} else {
				slog.Info("cron job executed", "id", j.ID, "kind", j.Kind, "result", msg)
			}
		}(job)
	}
}
