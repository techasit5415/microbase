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

	"github.com/labstack/echo/v4"
	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/config"
	"github.com/techasit5415/microbase/internal/routes"
)

func main() {
	// 1. ตั้งค่า Structured Logger (log/slog)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// 2. โหลด Config (.env)
	cfg := config.Load()

	// 3. เตรียม Cache (In-Memory หรือ Redis อัตโนมัติ)
	ctx := context.Background()
	cacheStore := cache.NewStore(ctx, cfg.RedisURL)
	defer cacheStore.Close()

	// 4. สร้าง Echo Server และผูก Routes
	e := echo.New()
	e.HideBanner = true
	routes.Register(e, cacheStore)

	// 5. รัน Server ใน Goroutine เพื่อให้รองรับ Graceful Shutdown
	addr := ":" + cfg.Port
	go func() {
		slog.Info("microbase server started", "url", "http://localhost"+addr)
		if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// 6. รอสัญญาณปิดโปรแกรม (Ctrl+C / SIGTERM) เพื่อปิดอย่างปลอดภัย
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
