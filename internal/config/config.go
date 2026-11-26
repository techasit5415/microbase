package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Host           string
	Port           string
	DatabaseURL    string
	RedisURL       string
	JWTSecret      string
	DataDir        string
	HooksDir       string
	MigrationsDir  string
	Origins        string
	Dev            bool
	DisableAdminUI bool
	EnvFile        string
}

func Load() *Config {
	return LoadFrom("")
}

func LoadFrom(envFile string) *Config {
	if envFile != "" {
		_ = godotenv.Overload(envFile)
	} else {
		_ = godotenv.Load()
	}

	return &Config{
		Host:           getEnv("HOST", "0.0.0.0"),
		Port:           getEnv("PORT", "3000"),
		DatabaseURL:    getEnv("DATABASE_URL", ""),
		RedisURL:       getEnv("REDIS_URL", ""),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		DataDir:        getEnv("DATA_DIR", "./storage"),
		HooksDir:       getEnv("HOOKS_DIR", "./mb_hooks"),
		MigrationsDir:  getEnv("MIGRATIONS_DIR", "./mb_migrations"),
		Origins:        getEnv("CORS_ORIGINS", ""),
		Dev:            parseBoolEnv("DEV"),
		DisableAdminUI: parseBoolEnv("DISABLE_ADMIN_UI"),
		EnvFile:        envFile,
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func parseBoolEnv(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}
