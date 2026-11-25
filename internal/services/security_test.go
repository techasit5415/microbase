package services

import (
	"github.com/techasit5415/microbase/internal/models"
	"regexp"
	"testing"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := VerifyPassword("test-password", hash); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if ok, err := VerifyPassword("wrong", hash); ok || err != nil {
		t.Fatal(ok, err)
	}
}
func TestPasswordRejectsMalformedHashes(t *testing.T) {
	for _, hash := range []string{"", "argon2id$$", "argon2id$YQ$", "argon2id$!$!"} {
		if ok, err := VerifyPassword("anything", hash); ok || err == nil {
			t.Errorf("accepted malformed hash %q", hash)
		}
	}
}
func TestGeneratedIDs(t *testing.T) {
	for _, p := range []string{"[a-z0-9]{15}", "usr_[A-Z0-9]{8}", "uuid"} {
		pattern := p
		if p == "uuid" {
			pattern = "[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}"
		}
		re := regexp.MustCompile("^" + pattern + "$")
		for i := 0; i < 20; i++ {
			if id := GenerateFromPattern(p); !re.MatchString(id) {
				t.Fatal(p, id)
			}
		}
	}
}
func TestSchemaRejectsInvalidAndDuplicateFields(t *testing.T) {
	for _, fields := range [][]models.FieldDef{{{Name: "id", Type: "text"}}, {{Name: "title", Type: "text"}, {Name: "title", Type: "text"}}, {{Name: "bad-name", Type: "text"}}} {
		if _, err := buildCreateTableSQL(models.CreateCollectionRequest{Name: "posts", Fields: fields}); err == nil {
			t.Fatal("invalid schema accepted")
		}
	}
}
