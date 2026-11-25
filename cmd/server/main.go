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
	"github.com/techasit5415/microbase/internal/services"
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

		var savedRedis string
		if err := pool.QueryRow(ctx, `SELECT value FROM _settings WHERE key = 'redis_url'`).Scan(&savedRedis); err == nil && savedRedis != "" {
			redisURL = savedRedis
		}
	} else {
		slog.Warn("DATABASE_URL is not set; running without database")
	}

	cacheStore := cache.NewSwitchableStore(cache.NewStore(ctx, redisURL))
	defer cacheStore.Close()

	// If no superuser exists yet, generate a 30-minute 8-digit Setup OTP and print it to the terminal.
	if pool != nil {
		var superCount int64
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM _superusers`).Scan(&superCount); err == nil && superCount == 0 {
			if otp, err := services.GenerateSetupOTP(ctx, cacheStore); err == nil {
				slog.Info("FIRST-TIME SUPERUSER SETUP",
					"otp", otp,
					"expires_in", "5m",
					"url", "http://localhost:"+cfg.Port+"/_/",
				)
			}
		}
	}

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
