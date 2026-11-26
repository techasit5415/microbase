package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/techasit5415/microbase/internal/cache"
	"github.com/techasit5415/microbase/internal/config"
	"github.com/techasit5415/microbase/internal/database"
	"github.com/techasit5415/microbase/internal/migrations"
	"github.com/techasit5415/microbase/internal/services"
)

func runMigrateCLI(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	envPath := fs.String("env", "", "Path to .env file")
	dbURL := fs.String("db", "", "PostgreSQL DATABASE_URL")
	migDir := fs.String("migrationsDir", "", "Migrations directory (default ./mb_migrations)")
	_ = fs.Parse(args)
	pos := fs.Args()

	if len(pos) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: microbase migrate [--migrationsDir ./mb_migrations] <up|down|status|create <name>|snapshot>")
		os.Exit(1)
	}

	cfg := config.LoadFrom(*envPath)
	if *dbURL != "" {
		cfg.DatabaseURL = *dbURL
	}
	dir := cfg.MigrationsDir
	if *migDir != "" {
		dir = *migDir
	}

	sub := pos[0]
	if sub == "create" {
		if len(pos) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: microbase migrate create <name>")
			os.Exit(1)
		}
		created, err := migrations.CreateSQL(dir, pos[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "create migration failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Created migration: %s\n", created)
		return
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

	cacheStore := cache.NewSwitchableStore(cache.NewStore(ctx, ""))
	defer cacheStore.Close()
	colSvc := services.NewCollectionService(pool, cacheStore)

	switch sub {
	case "up":
		ran, err := migrations.Up(ctx, pool, colSvc, dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "migrate up failed: %v\n", err)
			os.Exit(1)
		}
		if len(ran) == 0 {
			fmt.Println("No pending migrations.")
		} else {
			for _, f := range ran {
				fmt.Printf("  applied: %s\n", f)
			}
			fmt.Printf("Applied %d migration(s).\n", len(ran))
		}
	case "down":
		rolled, err := migrations.Down(ctx, pool, dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "migrate down failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Rolled back: %s\n", rolled)
	case "status":
		rows, err := migrations.Status(ctx, pool, dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "migrate status failed: %v\n", err)
			os.Exit(1)
		}
		if len(rows) == 0 {
			fmt.Printf("No migration files found in %s\n", dir)
			return
		}
		for _, r := range rows {
			if r.Applied {
				fmt.Printf("[APPLIED] %s (%s)\n", r.Name, r.AppliedAt.Format("2006-01-02 15:04:05"))
			} else {
				fmt.Printf("[PENDING] %s\n", r.Name)
			}
		}
	case "snapshot":
		snap, err := migrations.CreateSnapshot(ctx, colSvc, dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "migrate snapshot failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Created schema snapshot: %s\n", snap)
	default:
		fmt.Fprintf(os.Stderr, "unknown migrate subcommand %q\n", sub)
		os.Exit(1)
	}
}
