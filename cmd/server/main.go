package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/config"
	"github.com/techasit5415/microbase/internal/database"
	"github.com/techasit5415/microbase/internal/routes"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()
	ctx := context.Background()

	var pool *pgxpool.Pool
	redisURL := cfg.RedisURL

	if cfg.DatabaseURL != "" {
		var err error
		pool, err = database.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			slog.Error("postgresql connection failed", "error", err)
			os.Exit(1)
		}
		defer pool.Close()

		// Load persisted Redis URL from _settings if configured via Admin UI.
		var savedRedis string
		if err := pool.QueryRow(ctx, `SELECT value FROM _settings WHERE key = 'redis_url'`).Scan(&savedRedis); err == nil && savedRedis != "" {
			redisURL = savedRedis
		}

		// Print PocketBase-style first-run superuser setup banner if _superusers is empty.
		var superCount int64
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM _superusers`).Scan(&superCount); err == nil && superCount == 0 {
			slog.Info("FIRST-TIME SETUP: open http://localhost:" + cfg.Port + "/_/ to create your first Superuser account")
		}
	} else {
		slog.Warn("DATABASE_URL is not set; running without database")
	}

	cacheStore := cache.NewSwitchableStore(cache.NewStore(ctx, redisURL))
	defer cacheStore.Close()

	e := echo.New()
	e.HideBanner = true
	routes.Register(e, pool, cacheStore, cfg)

	addr := ":" + cfg.Port
	go func() {
		slog.Info("microbase server started", "url", "http://localhost"+addr, "admin", "http://localhost"+addr+"/_/")
		if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down microbase...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
	slog.Info("microbase stopped cleanly")
}
