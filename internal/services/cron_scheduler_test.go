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
