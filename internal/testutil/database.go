// Package testutil provides isolated PostgreSQL fixtures for integration tests.
package testutil

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/techasit5415/microbase/internal/database"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func Database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if os.Getenv("MICROBASE_TEST_LOCAL_ENV") == "1" {
		_, file, _, _ := runtime.Caller(0)
		env, err := godotenv.Read(filepath.Join(filepath.Dir(file), "../../.env"))
		if err != nil {
			t.Fatal(err)
		}
		dsn = env["DATABASE_URL"]
	}
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL or MICROBASE_TEST_LOCAL_ENV=1 is required; integration tests must not skip")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("test database config invalid")
	}
	schema := fmt.Sprintf("coverage_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid config")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("invalid test URL")
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	migrated, err := database.Connect(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	migrated.Close()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	return pool
}
