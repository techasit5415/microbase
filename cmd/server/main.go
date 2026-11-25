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

	// Connect to PostgreSQL if DATABASE_URL is configured.
	var pool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		var err error
		pool, err = database.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			slog.Error("postgresql connection failed", "error", err)
			os.Exit(1)
		}
		defer pool.Close()
	} else {
		slog.Warn("DATABASE_URL is not set; running without database")
	}

	// Initialize cache store (Redis if REDIS_URL is set, otherwise In-Memory).
	cacheStore := cache.NewStore(ctx, cfg.RedisURL)
	defer cacheStore.Close()

	e := echo.New()
	e.HideBanner = true
	routes.Register(e, pool, cacheStore, cfg)

	addr := ":" + cfg.Port
	go func() {
		slog.Info("microbase server started", "url", "http://localhost"+addr)
		if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server.
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
