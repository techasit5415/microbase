package database

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a PostgreSQL connection pool and executes system table migrations.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is empty")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pool failed: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres failed: %w", err)
	}

	if err := runMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}

	slog.Info("postgresql connected")
	return pool, nil
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		slog.Warn("pgvector extension not available", "error", err)
	} else {
		slog.Info("pgvector extension enabled")
	}

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS _collections (
			id         BIGSERIAL PRIMARY KEY,
			name       TEXT UNIQUE NOT NULL,
			fields     JSONB NOT NULL DEFAULT '[]'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS _users (
			id            BIGSERIAL PRIMARY KEY,
			email         TEXT UNIQUE NOT NULL,
			password_hash TEXT,
			role          TEXT NOT NULL DEFAULT 'user',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`ALTER TABLE _users ALTER COLUMN password_hash DROP NOT NULL;`,
		`CREATE TABLE IF NOT EXISTS _superusers (
			id            BIGSERIAL PRIMARY KEY,
			email         TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`ALTER TABLE _collections ADD COLUMN IF NOT EXISTS api_rules JSONB NOT NULL DEFAULT '{}'::jsonb;
		ALTER TABLE _collections ADD COLUMN IF NOT EXISTS indexes JSONB NOT NULL DEFAULT '[]'::jsonb;`,
		`ALTER TABLE _collections ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'base';`,
		`ALTER TABLE _collections ADD COLUMN IF NOT EXISTS view_query TEXT NOT NULL DEFAULT '';`,
		`UPDATE _collections SET type = 'auth' WHERE name = 'users';`,
		`ALTER TABLE _users ADD COLUMN IF NOT EXISTS verified BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE _users ADD COLUMN IF NOT EXISTS email_visibility BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE _users ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE _users ADD COLUMN IF NOT EXISTS avatar TEXT NOT NULL DEFAULT '';`,
		`INSERT INTO _collections (name, fields, api_rules, indexes)
		 VALUES (
			'users',
			'[{"name":"name","type":"text"},{"name":"avatar","type":"file","max_select":1}]'::jsonb,
			'{}'::jsonb,
			'[]'::jsonb
		 )
		 ON CONFLICT (name) DO NOTHING;`,
		`CREATE TABLE IF NOT EXISTS _oauth_providers (
			name          TEXT PRIMARY KEY,
			display_name  TEXT NOT NULL,
			enabled       BOOLEAN NOT NULL DEFAULT FALSE,
			client_id     TEXT NOT NULL DEFAULT '',
			client_secret TEXT NOT NULL DEFAULT '',
			issuer_url    TEXT NOT NULL DEFAULT '',
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`INSERT INTO _oauth_providers (name, display_name) VALUES
			('google', 'Google'),
			('discord', 'Discord'),
			('oidc', 'OpenID Connect (OIDC)')
		ON CONFLICT (name) DO NOTHING;`,
		`CREATE TABLE IF NOT EXISTS _webhooks (
			id         BIGSERIAL PRIMARY KEY,
			collection TEXT NOT NULL,
			event      TEXT NOT NULL,
			url        TEXT NOT NULL,
			enabled    BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS _oauth_accounts (
			id               BIGSERIAL PRIMARY KEY,
			user_id          BIGINT NOT NULL REFERENCES _users(id) ON DELETE CASCADE,
			provider         TEXT NOT NULL,
			provider_user_id TEXT NOT NULL,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (provider, provider_user_id)
		);`,
	}

	migrations = append(migrations,
		`CREATE TABLE IF NOT EXISTS _logs (
			id BIGSERIAL PRIMARY KEY,
			status INTEGER NOT NULL,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			code TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL DEFAULT '',
			latency_ms BIGINT NOT NULL DEFAULT 0,
			ip TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_logs_created_at ON _logs (created_at DESC);

		CREATE TABLE IF NOT EXISTS _settings (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`ALTER TABLE _webhooks ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT 'generic';`,
		`ALTER TABLE _webhooks ADD COLUMN IF NOT EXISTS auth_token TEXT NOT NULL DEFAULT '';`,
	)

	for _, sql := range migrations {
		if _, err := pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	return nil
}
