package migrations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techasit5415/microbase/internal/models"
	"github.com/techasit5415/microbase/internal/services"
)

const createMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS _migrations (
	name TEXT PRIMARY KEY,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`

var nonIdentRe = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

type StatusRow struct {
	Name      string
	Applied   bool
	AppliedAt *time.Time
}

func EnsureTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, createMigrationsTableSQL)
	return err
}

func ListFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".sql") || strings.HasSuffix(lower, ".json") {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	return files, nil
}

func AppliedMap(ctx context.Context, pool *pgxpool.Pool) (map[string]time.Time, error) {
	if err := EnsureTable(ctx, pool); err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `SELECT name, applied_at FROM _migrations ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]time.Time)
	for rows.Next() {
		var name string
		var ts time.Time
		if err := rows.Scan(&name, &ts); err == nil {
			out[name] = ts
		}
	}
	return out, rows.Err()
}

// SplitSQLSections extracts the "-- +migrate Up" and "-- +migrate Down" blocks from a .sql migration.
// If no markers are present, the entire file is treated as the Up migration.
func SplitSQLSections(content string) (upSQL string, downSQL string) {
	var upBuf, downBuf strings.Builder
	mode := "up"
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.ToLower(line))
		if trimmed == "-- +migrate up" {
			mode = "up"
			continue
		}
		if trimmed == "-- +migrate down" {
			mode = "down"
			continue
		}
		if mode == "down" {
			downBuf.WriteString(line)
			downBuf.WriteByte('\n')
		} else {
			upBuf.WriteString(line)
			upBuf.WriteByte('\n')
		}
	}
	return strings.TrimSpace(upBuf.String()), strings.TrimSpace(downBuf.String())
}

func CreateSQL(dir, name string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	slug := strings.Trim(nonIdentRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "_"), "_")
	if slug == "" {
		slug = "migration"
	}
	filename := fmt.Sprintf("%d_%s.sql", time.Now().Unix(), slug)
	fullPath := filepath.Join(dir, filename)
	tmpl := fmt.Sprintf("-- Migration: %s\n-- Created: %s\n\n-- +migrate Up\n-- Write your UP SQL statements here\n\n-- +migrate Down\n-- Write your DOWN rollback SQL statements here\n",
		slug, time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(fullPath, []byte(tmpl), 0o644); err != nil {
		return "", err
	}
	return fullPath, nil
}

func CreateSnapshot(ctx context.Context, colSvc *services.CollectionService, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	export, err := colSvc.ExportSchema(ctx)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%d_snapshot.json", time.Now().Unix())
	fullPath := filepath.Join(dir, filename)
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return "", err
	}
	return fullPath, nil
}

func Up(ctx context.Context, pool *pgxpool.Pool, colSvc *services.CollectionService, dir string) ([]string, error) {
	applied, err := AppliedMap(ctx, pool)
	if err != nil {
		return nil, err
	}
	files, err := ListFiles(dir)
	if err != nil {
		return nil, err
	}

	var ran []string
	for _, fname := range files {
		if _, ok := applied[fname]; ok {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, fname))
		if err != nil {
			return ran, fmt.Errorf("read %s: %w", fname, err)
		}

		if strings.HasSuffix(strings.ToLower(fname), ".json") {
			if colSvc == nil {
				return ran, fmt.Errorf("collection service required for json snapshot %s", fname)
			}
			var bundle models.SchemaExport
			if err := json.Unmarshal(raw, &bundle); err != nil {
				return ran, fmt.Errorf("parse snapshot %s: %w", fname, err)
			}
			if _, _, err := colSvc.ImportSchema(ctx, bundle); err != nil {
				return ran, fmt.Errorf("import snapshot %s: %w", fname, err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO _migrations (name) VALUES ($1)`, fname); err != nil {
				return ran, fmt.Errorf("record migration %s: %w", fname, err)
			}
		} else {
			upSQL, _ := SplitSQLSections(string(raw))
			tx, err := pool.Begin(ctx)
			if err != nil {
				return ran, err
			}
			if upSQL != "" {
				if _, err := tx.Exec(ctx, upSQL); err != nil {
					_ = tx.Rollback(ctx)
					return ran, fmt.Errorf("apply %s: %w", fname, err)
				}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO _migrations (name) VALUES ($1)`, fname); err != nil {
				_ = tx.Rollback(ctx)
				return ran, fmt.Errorf("record %s: %w", fname, err)
			}
			if err := tx.Commit(ctx); err != nil {
				return ran, err
			}
		}
		ran = append(ran, fname)
	}
	return ran, nil
}

func Down(ctx context.Context, pool *pgxpool.Pool, dir string) (string, error) {
	if err := EnsureTable(ctx, pool); err != nil {
		return "", err
	}
	var lastName string
	err := pool.QueryRow(ctx, `SELECT name FROM _migrations ORDER BY name DESC LIMIT 1`).Scan(&lastName)
	if err != nil {
		return "", fmt.Errorf("no applied migrations to roll back")
	}

	raw, err := os.ReadFile(filepath.Join(dir, lastName))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", lastName, err)
	}
	if strings.HasSuffix(strings.ToLower(lastName), ".json") {
		return "", fmt.Errorf("cannot automatically roll back JSON snapshot %s", lastName)
	}
	_, downSQL := SplitSQLSections(string(raw))
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	if downSQL != "" {
		if _, err := tx.Exec(ctx, downSQL); err != nil {
			_ = tx.Rollback(ctx)
			return "", fmt.Errorf("rollback %s: %w", lastName, err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM _migrations WHERE name = $1`, lastName); err != nil {
		_ = tx.Rollback(ctx)
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return lastName, nil
}

func Status(ctx context.Context, pool *pgxpool.Pool, dir string) ([]StatusRow, error) {
	applied, err := AppliedMap(ctx, pool)
	if err != nil {
		return nil, err
	}
	files, err := ListFiles(dir)
	if err != nil {
		return nil, err
	}
	rows := make([]StatusRow, 0, len(files))
	for _, f := range files {
		ts, ok := applied[f]
		row := StatusRow{Name: f, Applied: ok}
		if ok {
			t := ts
			row.AppliedAt = &t
		}
		rows = append(rows, row)
	}
	return rows, nil
}
