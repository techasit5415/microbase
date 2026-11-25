package config

import "testing"

func TestLoadEnvironment(t *testing.T) {
	t.Setenv("PORT", "4321")
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("REDIS_URL", "")
	cfg := Load()
	if cfg.Port != "4321" || cfg.DatabaseURL != "postgres://test" || cfg.JWTSecret != "test-secret" || cfg.RedisURL != "" {
		t.Fatal("config mismatch")
	}
	t.Setenv("PORT", "")
	if Load().Port != "3000" {
		t.Fatal("missing default port")
	}
}
