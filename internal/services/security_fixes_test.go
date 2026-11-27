package services

import (
	"net"
	"testing"

	"github.com/techasit5415/microbase/internal/models"
)

func TestOAuthRedirectAllowlist(t *testing.T) {
	orig := GetCachedAppSettings()
	defer SetCachedAppSettings(orig)

	cfg := DefaultAppSettings()
	cfg.AppURL = "https://app.example.com"
	SetCachedAppSettings(cfg)

	if isAllowedRedirectURL("https://evil.attacker.com/cb") {
		t.Fatalf("expected untrusted redirect_to origin to be rejected")
	}
	if !isAllowedRedirectURL("https://app.example.com/auth/callback") {
		t.Fatalf("expected AppURL origin to be allowed")
	}
}

func TestFilterParserBlocksSensitiveColumnsAndSystemTables(t *testing.T) {
	allowed := map[string]models.FieldDef{
		"author": {Name: "author", Type: "relation", CollectionName: "users"},
	}
	claims := &AuthClaims{UserID: "1", Role: "user"}

	blocked := []string{
		`@request.auth.password_hash ~ "argon2"`,
		`@request.auth.token_key ~ "a"`,
		`@request.auth._superusers.email ~ "a"`,
		`author.password_hash ~ "argon2"`,
		`author.token_key ~ "a"`,
	}

	for _, expr := range blocked {
		if _, _, _, err := ParseFilterToSQLForTable("posts", expr, allowed, claims, nil, 1); err == nil {
			t.Fatalf("expected sensitive filter %q to be rejected", expr)
		}
	}
}

func TestVisibleRecordEnforcesEmailVisibility(t *testing.T) {
	col := &models.Collection{Name: "users", Type: "auth"}
	recHidden := models.Record{"id": "10", "email": "alice@example.com", "email_visibility": false, "password_hash": "secret"}
	recPublic := models.Record{"id": "10", "email": "alice@example.com", "email_visibility": true}

	// Other user cannot see email when email_visibility=false
	outOther := VisibleRecord(col, &AuthClaims{UserID: "99", Role: "user"}, recHidden)
	if _, ok := outOther["email"]; ok {
		t.Fatalf("expected private email hidden from other user")
	}
	if _, ok := outOther["password_hash"]; ok {
		t.Fatalf("expected password_hash stripped")
	}

	// Owner can see own email
	outOwner := VisibleRecord(col, &AuthClaims{UserID: "10", Role: "user"}, recHidden)
	if outOwner["email"] != "alice@example.com" {
		t.Fatalf("expected owner to see own email")
	}

	// Superuser can see email
	outSuper := VisibleRecord(col, &AuthClaims{UserID: "1", Role: "superuser"}, recHidden)
	if outSuper["email"] != "alice@example.com" {
		t.Fatalf("expected superuser to see email")
	}

	// Other user can see email when email_visibility=true
	outPub := VisibleRecord(col, &AuthClaims{UserID: "99", Role: "user"}, recPublic)
	if outPub["email"] != "alice@example.com" {
		t.Fatalf("expected public email visible when email_visibility=true")
	}
}

func TestSSRFBlocksPrivateAndMetadataIPs(t *testing.T) {
	blockedIPs := []string{"127.0.0.1", "169.254.169.254", "10.0.0.1", "172.16.0.1", "192.168.1.1", "::1", "0.0.0.0"}
	for _, s := range blockedIPs {
		if !isBlockedSSRFIP(net.ParseIP(s)) {
			t.Fatalf("expected IP %s to be blocked", s)
		}
	}
	if isBlockedSSRFIP(net.ParseIP("93.184.216.34")) {
		t.Fatalf("expected public IP 93.184.216.34 to be allowed")
	}
}
