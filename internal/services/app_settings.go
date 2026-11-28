package services

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

type BatchSettings struct {
	Enabled     bool `json:"enabled"`
	MaxRequests int  `json:"max_requests"`
	Timeout     int  `json:"timeout"`
}

type TrustedProxySettings struct {
	Enabled bool     `json:"enabled"`
	Headers []string `json:"headers"`
}

type SuperuserIPSettings struct {
	Enabled    bool     `json:"enabled"`
	AllowedIPs []string `json:"allowed_ips"`
}

// AppSettings stores PocketBase-style Application settings persisted in _settings('app_settings').
type AppSettings struct {
	AppName      string               `json:"app_name"`
	AppURL       string               `json:"app_url"`
	AccentColor  string               `json:"accent_color"`
	HideControls bool                 `json:"hide_controls"`
	Batch        BatchSettings        `json:"batch"`
	TrustedProxy TrustedProxySettings `json:"trusted_proxy"`
	SuperuserIPs SuperuserIPSettings  `json:"superuser_ips"`
}

func DefaultAppSettings() AppSettings {
	return AppSettings{
		AppName:      "Microbase",
		AppURL:       "http://localhost:3000",
		AccentColor:  "#25272d",
		HideControls: false,
		Batch: BatchSettings{
			Enabled:     true,
			MaxRequests: 50,
			Timeout:     10,
		},
		TrustedProxy: TrustedProxySettings{
			Enabled: false,
			Headers: []string{"X-Forwarded-For", "X-Real-IP"},
		},
		SuperuserIPs: SuperuserIPSettings{
			Enabled:    false,
			AllowedIPs: []string{"127.0.0.1", "::1"},
		},
	}
}

var (
	appSettingsMu    sync.RWMutex
	cachedAppSetting = DefaultAppSettings()
)

// GetCachedAppSettings returns the current in-memory AppSettings snapshot.
func GetCachedAppSettings() AppSettings {
	appSettingsMu.RLock()
	defer appSettingsMu.RUnlock()
	return cachedAppSetting
}

// SetCachedAppSettings updates the in-memory AppSettings snapshot.
func SetCachedAppSettings(s AppSettings) {
	if strings.TrimSpace(s.AppName) == "" {
		s.AppName = "Microbase"
	}
	if strings.TrimSpace(s.AccentColor) == "" {
		s.AccentColor = "#25272d"
	}
	if s.Batch.MaxRequests <= 0 {
		s.Batch.MaxRequests = 50
	}
	if s.Batch.Timeout <= 0 {
		s.Batch.Timeout = 10
	}
	appSettingsMu.Lock()
	cachedAppSetting = s
	appSettingsMu.Unlock()
}

// LoadAppSettingsFromDB loads persisted app_settings from PostgreSQL _settings table on startup.
func LoadAppSettingsFromDB(ctx context.Context, pool *pgxpool.Pool) AppSettings {
	def := DefaultAppSettings()
	if pool == nil {
		SetCachedAppSettings(def)
		return def
	}
	var raw string
	if err := pool.QueryRow(ctx, `SELECT value FROM _settings WHERE key = 'app_settings'`).Scan(&raw); err == nil && raw != "" {
		var loaded AppSettings
		if json.Unmarshal([]byte(raw), &loaded) == nil {
			SetCachedAppSettings(loaded)
			return GetCachedAppSettings()
		}
	}
	SetCachedAppSettings(def)
	return def
}

// RealClientIP extracts the client IP respecting TrustedProxy headers only when TrustedProxy.Enabled is true.
// When TrustedProxy is disabled, it ignores X-Forwarded-For / X-Real-IP and uses the direct peer RemoteAddr.
func RealClientIP(c echo.Context) string {
	s := GetCachedAppSettings()
	if s.TrustedProxy.Enabled {
		for _, hdr := range s.TrustedProxy.Headers {
			hdr = strings.TrimSpace(hdr)
			if hdr == "" {
				continue
			}
			if val := strings.TrimSpace(c.Request().Header.Get(hdr)); val != "" {
				parts := strings.Split(val, ",")
				first := strings.TrimSpace(parts[0])
				if first != "" {
					return first
				}
			}
		}
	}
	remote := strings.TrimSpace(c.Request().RemoteAddr)
	if host, _, err := net.SplitHostPort(remote); err == nil && host != "" {
		return host
	}
	if remote != "" {
		return remote
	}
	return c.RealIP()
}

// IsSuperuserIPAllowed checks whether clientIP is permitted when SuperuserIPs restriction is enabled.
func IsSuperuserIPAllowed(clientIP string) bool {
	s := GetCachedAppSettings()
	if !s.SuperuserIPs.Enabled || len(s.SuperuserIPs.AllowedIPs) == 0 {
		return true
	}
	cleanIP := strings.TrimSpace(clientIP)
	if host, _, err := net.SplitHostPort(cleanIP); err == nil {
		cleanIP = host
	}
	parsedClient := net.ParseIP(cleanIP)

	for _, rule := range s.SuperuserIPs.AllowedIPs {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		if rule == cleanIP || rule == "*" {
			return true
		}
		if strings.Contains(rule, "/") && parsedClient != nil {
			if _, cidr, err := net.ParseCIDR(rule); err == nil && cidr.Contains(parsedClient) {
				return true
			}
		}
	}
	return false
}
