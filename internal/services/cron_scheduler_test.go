package services

import (
	"testing"
	"time"
)

func TestCronExprMatching(t *testing.T) {
	if err := ValidateCronExpr("*/5 * * * *"); err != nil {
		t.Fatalf("expected valid */5 * * * *, got %v", err)
	}
	if err := ValidateCronExpr("0 8 * * 1-5"); err != nil {
		t.Fatalf("expected valid 0 8 * * 1-5, got %v", err)
	}
	if err := ValidateCronExpr("bad expr"); err == nil {
		t.Fatalf("expected error for invalid cron expression")
	}

	tm := time.Date(2026, 11, 25, 8, 15, 0, 0, time.UTC) // Wednesday (3)
	if !CronMatches("*/5 * * * *", tm) {
		t.Fatalf("expected */5 * * * * to match minute 15")
	}
	if !CronMatches("15 8 * * 1-5", tm) {
		t.Fatalf("expected 15 8 * * 1-5 to match Wed 08:15")
	}
	if CronMatches("0 9 * * *", tm) {
		t.Fatalf("expected 0 9 * * * NOT to match 08:15")
	}
}

func TestPostgresToolPatternsWindows(t *testing.T) {
	pats := postgresToolPatterns("windows", "psql")
	want := []string{
		`C:\Program Files\PostgreSQL\*\bin\psql.exe`,
		`C:\Program Files (x86)\PostgreSQL\*\bin\psql.exe`,
	}
	if len(pats) != len(want) {
		t.Fatalf("got %d patterns, want %d", len(pats), len(want))
	}
	for i := range want {
		if pats[i] != want[i] {
			t.Fatalf("pattern[%d] = %q, want %q", i, pats[i], want[i])
		}
	}
}

func TestSystemCronSchedules(t *testing.T) {
	t2 := time.Date(2026, 11, 27, 2, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	if !CronMatches("0 2 * * *", t2) {
		t.Fatalf("expected __mbAutoBackup__ schedule to match 02:00")
	}
	if !CronMatches("0 3 * * *", t3) {
		t.Fatalf("expected __mbLogsPrune__ schedule to match 03:00")
	}
}
