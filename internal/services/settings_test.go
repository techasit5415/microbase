package services

import (
	"github.com/labstack/echo/v4"
	"net/http/httptest"
	"testing"
)

func TestIPRestrictionsAndProxyHeaders(t *testing.T) {
	old := GetCachedAppSettings()
	defer SetCachedAppSettings(old)
	s := DefaultAppSettings()
	s.SuperuserIPs = SuperuserIPSettings{Enabled: true, AllowedIPs: []string{"192.168.1.0/24", "::1", "bad/cidr"}}
	SetCachedAppSettings(s)
	for _, ip := range []string{"192.168.1.2", "[::1]:1234"} {
		if !IsSuperuserIPAllowed(ip) {
			t.Error(ip)
		}
	}
	if IsSuperuserIPAllowed("10.0.0.1") {
		t.Fatal("unlisted IP allowed")
	}
	s.TrustedProxy = TrustedProxySettings{Enabled: true, Headers: []string{"", "X-Real-IP"}}
	SetCachedAppSettings(s)
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Real-IP", "192.168.1.2, 10.0.0.1")
	if got := RealClientIP(e.NewContext(req, httptest.NewRecorder())); got != "192.168.1.2" {
		t.Fatal(got)
	}
}
