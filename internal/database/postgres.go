package database

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a PostgreSQL connection pool and executes system table migrations.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is empty")
	}

	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url failed: %w", err)
	}
	// Use unnamed parameterized statements so dynamic ALTER TABLE DDL never hits
	// PostgreSQL SQLSTATE 0A000 ("cached plan must not change result type").
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeDescribeExec

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
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
		`DO $$
		BEGIN
			IF to_regclass('public._users') IS NOT NULL AND to_regclass('public.users') IS NULL THEN
				ALTER TABLE "users" RENAME TO "users";
			END IF;
		END $$;`,
		`CREATE TABLE IF NOT EXISTS "users" (
			id            BIGSERIAL PRIMARY KEY,
			email         TEXT UNIQUE NOT NULL,
			password_hash TEXT,
			role          TEXT NOT NULL DEFAULT 'user',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`ALTER TABLE "users" ALTER COLUMN password_hash DROP NOT NULL;`,
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
		`ALTER TABLE _collections ADD COLUMN IF NOT EXISTS id_type TEXT NOT NULL DEFAULT 'serial';`,
		`ALTER TABLE _collections ADD COLUMN IF NOT EXISTS id_autogenerate_pattern TEXT NOT NULL DEFAULT '';`,
		`CREATE TABLE IF NOT EXISTS _oauth_providers (
			name          TEXT PRIMARY KEY,
			display_name  TEXT NOT NULL,
			enabled       BOOLEAN NOT NULL DEFAULT FALSE,
			client_id     TEXT NOT NULL DEFAULT '',
			client_secret TEXT NOT NULL DEFAULT '',
			issuer_url    TEXT NOT NULL DEFAULT '',
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`ALTER TABLE _oauth_providers ADD COLUMN IF NOT EXISTS auth_url TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE _oauth_providers ADD COLUMN IF NOT EXISTS token_url TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE _oauth_providers ADD COLUMN IF NOT EXISTS userinfo_url TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE _oauth_providers ADD COLUMN IF NOT EXISTS scopes TEXT NOT NULL DEFAULT 'openid email profile';`,
		`ALTER TABLE _oauth_providers ADD COLUMN IF NOT EXISTS field_mapping JSONB NOT NULL DEFAULT '{}'::jsonb;`,
		`ALTER TABLE _oauth_providers ADD COLUMN IF NOT EXISTS allowed_claims JSONB NOT NULL DEFAULT '{}'::jsonb;`,
		`CREATE TABLE IF NOT EXISTS _webhooks (
			id         BIGSERIAL PRIMARY KEY,
			collection TEXT NOT NULL,
			event      TEXT NOT NULL,
			url        TEXT NOT NULL,
			enabled    BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`ALTER TABLE _webhooks ADD COLUMN IF NOT EXISTS secret TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE _webhooks ADD COLUMN IF NOT EXISTS headers JSONB NOT NULL DEFAULT '{}'::jsonb;`,
		`ALTER TABLE _webhooks ADD COLUMN IF NOT EXISTS max_retries INT NOT NULL DEFAULT 3;`,
		`ALTER TABLE _webhooks ADD COLUMN IF NOT EXISTS last_run_at TIMESTAMPTZ;`,
		`ALTER TABLE _webhooks ADD COLUMN IF NOT EXISTS last_status TEXT NOT NULL DEFAULT '';`,
		`UPDATE _collections SET type = 'auth' WHERE name = 'users';`,
		`ALTER TABLE "users" ADD COLUMN IF NOT EXISTS verified BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE "users" ADD COLUMN IF NOT EXISTS email_visibility BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE "users" ADD COLUMN IF NOT EXISTS token_key TEXT NOT NULL DEFAULT '';`,
		`UPDATE "users" SET token_key = md5(random()::text || clock_timestamp()::text || id::text) WHERE token_key = '';`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_token_key ON "users" (token_key) WHERE token_key <> '';`,
		`ALTER TABLE "users" ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE "users" ADD COLUMN IF NOT EXISTS avatar TEXT NOT NULL DEFAULT '';`,
		`INSERT INTO _collections (name, type, fields, api_rules, indexes)
		 VALUES (
			'users', 'auth',
			'[{"name":"name","type":"text"},{"name":"avatar","type":"file","max_select":1}]'::jsonb,
			'{}'::jsonb,
			'[]'::jsonb
		 )
		 ON CONFLICT (name) DO NOTHING;`,
		`INSERT INTO _oauth_providers (name, display_name) VALUES
			('google', 'Google'),
			('discord', 'Discord'),
			('oidc', 'OpenID Connect (OIDC)')
		ON CONFLICT (name) DO NOTHING;`,
		`CREATE TABLE IF NOT EXISTS _crons (
			id          TEXT PRIMARY KEY,
			expression  TEXT NOT NULL,
			kind        TEXT NOT NULL DEFAULT 'http',
			target      TEXT NOT NULL DEFAULT '',
			method      TEXT NOT NULL DEFAULT 'POST',
			payload     TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			enabled     BOOLEAN NOT NULL DEFAULT TRUE,
			last_run_at TIMESTAMPTZ,
			last_status TEXT NOT NULL DEFAULT '',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS _oauth_accounts (
			id               BIGSERIAL PRIMARY KEY,
			user_id          BIGINT NOT NULL REFERENCES "users"(id) ON DELETE CASCADE,
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
