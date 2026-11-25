package database

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is empty")
	}
	//create  Connection Pool
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pool failed: %w", err)
	}
	//ping pool
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres failed: %w", err)
	}
	// run auto migrate create table
	if err := runMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	slog.Info("postgresql connected")
	return pool, nil
}
func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// พยายามเปิด pgvector (ถ้า Postgres เครื่องนั้นยังไม่ได้ลง pgvector จะแค่เตือน ไม่ให้เซิร์ฟเวอร์พัง)
	if _, err := pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		slog.Warn("pgvector extension not available", "error", err)
	} else {
		slog.Info("pgvector extension enabled")
	}
	query := `
	CREATE TABLE IF NOT EXISTS _collections (
		id         BIGSERIAL PRIMARY KEY,
		name       TEXT UNIQUE NOT NULL,
		fields     JSONB NOT NULL DEFAULT '[]'::jsonb,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`
	if _, err := pool.Exec(ctx, query); err != nil {
		return fmt.Errorf("create _collections table failed: %w", err)
	}
	return nil
}
