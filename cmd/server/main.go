package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
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

func printUsage() {
	fmt.Printf(`Microbase %s — Single-binary PostgreSQL + pgvector BaaS

Usage:
  microbase [command] [flags]

Commands:
  serve                Start the Microbase HTTP server (default)
  update               Check or self-update binary from GitHub Releases
  superuser upsert     Create or reset a superuser account from CLI
  migrate              Run file-based database migrations (up|down|status|create|snapshot)
  version              Print version and exit

Global / Serve Flags:
  -v, --version            Print version and exit
  -h, --help               Show this help message
  -e, --env <path>         Path to .env file (default ".env")
      --db <url>           Override PostgreSQL DATABASE_URL
      --dir <path>         Data/storage directory (default "./storage")
      --dev                Enable verbose debug logging
      --http <addr>        Listen address host:port (e.g. "0.0.0.0:3000" or ":8090")
      --host <host>        Listen host (default "0.0.0.0")
      --port <port>        Listen port (default "3000")
      --origins <list>     Comma-separated CORS allowed origins
      --no-ui              Disable embedded Admin UI (/_/) for headless API mode
      --hooksDir <path>    Directory for JS hooks (default "./mb_hooks")
      --migrationsDir <p>  Directory for SQL/JSON migrations (default "./mb_migrations")

Update Flags:
      --check              Only check if a newer release is available
      --to <tag>           Update or rollback to a specific release tag (e.g. v0.11.0)
`, config.Version)
}

func main() {
	// Clean up leftover Windows self-update .old binary if present.
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
	args := os.Args[1:]

	// Quick top-level shorthands: -v / --version / version / --update / -h / --help
	if len(args) > 0 {
		switch args[0] {
		case "-v", "--version", "version":
			fmt.Printf("microbase %s\n", config.Version)
			return
		case "-h", "--help", "help":
			printUsage()
			return
		case "--update":
			args = append([]string{"update"}, args[1:]...)
		}
	}

	cmd := "serve"
	cmdArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		cmdArgs = args[1:]
	}

	switch cmd {
	case "serve":
		runServe(cmdArgs)
	case "update":
		fs := flag.NewFlagSet("update", flag.ExitOnError)
		checkOnly := fs.Bool("check", false, "Only check for updates")
		targetTag := fs.String("to", "", "Target release tag")
		_ = fs.Parse(cmdArgs)
		if err := runUpdateCommand(*checkOnly, *targetTag); err != nil {
			fmt.Fprintf(os.Stderr, "update error: %v\n", err)
			os.Exit(1)
		}
	case "superuser":
		runSuperuserCLI(cmdArgs)
	case "migrate":
		runMigrateCLI(cmdArgs)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func runSuperuserCLI(args []string) {
	fs := flag.NewFlagSet("superuser", flag.ExitOnError)
	envPath := fs.String("env", "", "Path to .env file")
	dbURL := fs.String("db", "", "PostgreSQL DATABASE_URL")
	_ = fs.Parse(args)
	pos := fs.Args()

	if len(pos) < 3 || pos[0] != "upsert" {
		fmt.Fprintln(os.Stderr, "Usage: microbase superuser [--env .env] [--db url] upsert <email> <password>")
		os.Exit(1)
	}
	email := strings.ToLower(strings.TrimSpace(pos[1]))
	password := pos[2]
	if !strings.Contains(email, "@") || len(password) < 8 {
		fmt.Fprintln(os.Stderr, "error: valid email and password (min 8 chars) required")
		os.Exit(1)
	}

	cfg := config.LoadFrom(*envPath)
	if *dbURL != "" {
		cfg.DatabaseURL = *dbURL
	}
	if cfg.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "error: DATABASE_URL is required")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgresql connection failed: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	hash, err := services.HashPassword(password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash password failed: %v\n", err)
		os.Exit(1)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO _superusers (email, password_hash)
		VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash, updated_at = NOW()`,
		email, hash,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "superuser upsert failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Superuser %q upserted successfully.\n", email)
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	showVersion := fs.Bool("version", false, "Print version and exit")
	showVersionShort := fs.Bool("v", false, "Print version and exit")
	envFile := fs.String("env", "", "Path to .env file")
	envFileShort := fs.String("e", "", "Path to .env file")
	dbURL := fs.String("db", "", "Override DATABASE_URL")
	dataDir := fs.String("dir", "", "Data/storage directory")
	httpAddr := fs.String("http", "", "Listen address host:port")
	host := fs.String("host", "", "Listen host")
	port := fs.String("port", "", "Listen port")
	origins := fs.String("origins", "", "Comma-separated CORS allowed origins")
	noUI := fs.Bool("no-ui", false, "Disable embedded Admin UI (/_/)")
	dev := fs.Bool("dev", false, "Enable verbose debug logging")
	hooksDir := fs.String("hooksDir", "", "Hooks directory")
	migrationsDir := fs.String("migrationsDir", "", "Migrations directory")
	fs.Usage = printUsage
	_ = fs.Parse(args)

	if *showVersion || *showVersionShort {
		fmt.Printf("microbase %s\n", config.Version)
		return
	}

	envPath := *envFile
	if envPath == "" {
		envPath = *envFileShort
	}
	cfg := config.LoadFrom(envPath)
	if *dbURL != "" {
		cfg.DatabaseURL = *dbURL
	}
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	}
	if *host != "" {
		cfg.Host = *host
	}
	if *port != "" {
		cfg.Port = *port
	}
	if *httpAddr != "" {
		h, p, err := net.SplitHostPort(*httpAddr)
		if err == nil {
			if h != "" {
				cfg.Host = h
			}
			if p != "" {
				cfg.Port = p
			}
		} else {
			cfg.Port = strings.TrimPrefix(*httpAddr, ":")
		}
	}
	if *origins != "" {
		cfg.Origins = *origins
	}
	if *noUI {
		cfg.DisableAdminUI = true
	}
	if *dev {
		cfg.Dev = true
	}
	if *hooksDir != "" {
		cfg.HooksDir = *hooksDir
	}
	if *migrationsDir != "" {
		cfg.MigrationsDir = *migrationsDir
	}

	logLevel := slog.LevelInfo
	if cfg.Dev {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

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

	if pool != nil && !cfg.DisableAdminUI {
		var superCount int64
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM _superusers`).Scan(&superCount); err == nil && superCount == 0 {
			if otp, err := services.GenerateSetupOTP(ctx, cacheStore); err == nil {
				slog.Info("FIRST-TIME SUPERUSER SETUP",
					"otp", otp,
					"expires_in", "5m",
					"url", fmt.Sprintf("http://localhost:%s/_/", cfg.Port),
				)
			}
		}
	}

	e := echo.New()
	e.HideBanner = true
	routes.Register(e, pool, cacheStore, cfg)

	listenHost := cfg.Host
	if listenHost == "0.0.0.0" {
		listenHost = ""
	}
	addr := listenHost + ":" + cfg.Port
	displayHost := cfg.Host
	if displayHost == "" || displayHost == "0.0.0.0" {
		displayHost = "localhost"
	}
	adminDisplay := fmt.Sprintf("http://%s:%s/_/", displayHost, cfg.Port)
	if cfg.DisableAdminUI {
		adminDisplay = "disabled"
	}

	go func() {
		slog.Info("microbase server started",
			"version", config.Version,
			"url", fmt.Sprintf("http://%s:%s", displayHost, cfg.Port),
			"admin", adminDisplay,
		)
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
