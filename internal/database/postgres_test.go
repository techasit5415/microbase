package database

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"os"
	"testing"
	"time"
)

func TestFreshDatabaseMigrations(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if os.Getenv("MICROBASE_TEST_LOCAL_ENV") == "1" {
		env, err := godotenv.Read("../../.env")
		if err != nil {
			t.Fatal(err)
		}
		url = env["DATABASE_URL"]
	}
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run database integration tests")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal("invalid test database config")
	}
	schema := fmt.Sprintf("microbase_test_%d", time.Now().UnixNano())
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	if _, err = pool.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = runMigrations(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}
	var typ string
	if err = pool.QueryRow(ctx, "SELECT type FROM _collections WHERE name='users'").Scan(&typ); err != nil || typ != "auth" {
		t.Fatalf("users type=%s err=%v", typ, err)
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name='_webhooks' AND column_name IN ('secret','headers','max_retries','provider','auth_token')", schema).Scan(&count); err != nil || count != 5 {
		t.Fatalf("webhook columns=%d err=%v", count, err)
	}
}
