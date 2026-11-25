package oauth

import (
	"context"
	"strings"
	"testing"
)

func TestGoogleAndDiscordProviders(t *testing.T) {
	g := NewGoogleProvider("gid", "gsec", "http://localhost:3000/cb")
	if u := g.AuthURL("st123"); !strings.Contains(u, "accounts.google.com") || !strings.Contains(u, "state=st123") {
		t.Fatalf("unexpected google auth URL: %s", u)
	}

	d := NewDiscordProvider("did", "dsec", "http://localhost:3000/cb")
	if u := d.AuthURL("st456"); !strings.Contains(u, "discord.com") || !strings.Contains(u, "state=st456") {
		t.Fatalf("unexpected discord auth URL: %s", u)
	}

	// BuildProvider dispatch & error paths
	if _, err := BuildProvider(context.Background(), ProviderConfig{Name: "google", Enabled: false}, "http://localhost/cb"); err == nil {
		t.Fatalf("expected disabled provider error")
	}
	if _, err := BuildProvider(context.Background(), ProviderConfig{Name: "google", Enabled: true, ClientID: "id"}, "http://localhost/cb"); err == nil {
		t.Fatalf("expected missing secret error")
	}
	if _, err := BuildProvider(context.Background(), ProviderConfig{Name: "google", Enabled: true, ClientID: "id", ClientSecret: "sec"}, "http://localhost/cb"); err != nil {
		t.Fatalf("expected google provider built, got %v", err)
	}
	if _, err := BuildProvider(context.Background(), ProviderConfig{Name: "discord", Enabled: true, ClientID: "id", ClientSecret: "sec"}, "http://localhost/cb"); err != nil {
		t.Fatalf("expected discord provider built, got %v", err)
	}
	if _, err := BuildProvider(context.Background(), ProviderConfig{Name: "unknown", Enabled: true, ClientID: "id", ClientSecret: "sec"}, "http://localhost/cb"); err == nil {
		t.Fatalf("expected unsupported provider error")
	}
}
