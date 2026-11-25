package models

import (
	"strings"
	"testing"
)

func TestIdentifierValidation(t *testing.T) {
	for _, name := range []string{"_users", "foo;drop table x", "Upper", "", strings.Repeat("a", 64)} {
		if IsValidIdentifier(name) {
			t.Error(name)
		}
	}
	if !IsValidIdentifier("posts_123") {
		t.Fatal("valid name rejected")
	}
}
func TestFieldTypeBoundaries(t *testing.T) {
	for _, dim := range []int{0, -1, 4097} {
		if _, err := (FieldDef{Type: "vector", Dimension: dim}).ToPostgresType(); err == nil {
			t.Error(dim)
		}
	}
	if typ, err := (FieldDef{Type: "select", MaxSelect: 2}).ToPostgresType(); typ != "JSONB" || err != nil {
		t.Fatal(typ, err)
	}
}
func TestPaginationNormalization(t *testing.T) {
	o := ListOptions{Page: -1, PerPage: 200}
	o.Normalize()
	if o.Page != 1 || o.PerPage != 100 || o.Offset() != 0 {
		t.Fatal(o)
	}
}
