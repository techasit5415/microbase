package services

import (
	"github.com/techasit5415/microbase/internal/models"
	"testing"
)

func TestIncompleteFiltersDoNotPanic(t *testing.T) {
	for _, expr := range []string{"", "(", "id", "id =", "id = 1 &&", "id = 1 || (", "id = )", "id = 1.2.3"} {
		t.Run(expr, func(t *testing.T) {
			if _, _, _, err := ParseFilterToSQL(expr, nil, nil, 1); err == nil {
				t.Fatalf("accepted malformed filter %q", expr)
			}
			if _, err := EvalFilterOnData(expr, nil, nil); err == nil {
				t.Fatalf("accepted malformed rule %q", expr)
			}
		})
	}
}
func TestCronRejectsInvalidTrailingSegments(t *testing.T) {
	for _, expr := range []string{"0,bad * * * *", "*,99 * * * *", "0, * * * *"} {
		if ValidateCronExpr(expr) == nil {
			t.Errorf("accepted %q", expr)
		}
	}
	ok, err := matchCronField("5/1", 25, 0, 59)
	if err != nil || !ok {
		t.Fatalf("numeric step failed: %v %v", ok, err)
	}
}
func TestVisibleRecordDoesNotMutatePublishedRecord(t *testing.T) {
	col := &models.Collection{Fields: []models.FieldDef{{Name: "secret", Hidden: true}}}
	rec := models.Record{"id": "abc", "secret": "hidden", "password_hash": "hash"}
	visible := VisibleRecord(col, nil, rec)
	if _, ok := visible["secret"]; ok {
		t.Fatal("hidden field exposed")
	}
	if _, ok := visible["password_hash"]; ok {
		t.Fatal("password exposed")
	}
	if rec["secret"] != "hidden" {
		t.Fatal("source record mutated")
	}
}
func TestNullOrderingDoesNotBecomeNotNull(t *testing.T) {
	sql, _, _, err := ParseFilterToSQL("id > null", nil, nil, 1)
	if err != nil || sql != "FALSE" {
		t.Fatalf("got %s %v", sql, err)
	}
}
