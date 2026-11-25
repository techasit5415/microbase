package services

import (
	"github.com/techasit5415/microbase/internal/models"
	"strings"
	"testing"
)

func TestFilterComparisonMatrix(t *testing.T) {
	data := map[string]any{"x": int64(10), "y": int64(20), "name": "Hello", "empty": nil}
	cases := map[string]bool{"x = 10": true, "x != 10": false, "x < y": true, "x <= 10": true, "y > x": true, "x >= 11": false, "name ~ 'ell'": true, "name !~ 'ell'": false, "empty = null": true, "empty != null": false, "(x = 10 && y = 20) || x = 1": true, "@request.auth.email = 'a@test.local'": true, "@request.body.x = 10": true, "name = 'Other'": false}
	claims := &AuthClaims{UserID: 1, Email: "a@test.local", Role: "user"}
	allowed := map[string]models.FieldDef{"x": {Type: "integer"}, "y": {Type: "integer"}, "name": {Type: "text"}, "empty": {Type: "text"}}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			got, err := EvalFilterOnData(expr, data, claims)
			if err != nil || got != want {
				t.Fatalf("got %v want %v err %v", got, want, err)
			}
			sql, _, _, err := ParseFilterToSQLWithBody(expr, allowed, claims, data, 1)
			if err != nil || sql == "" {
				t.Fatal(sql, err)
			}
		})
	}
}
func TestQueryValidationAndParameters(t *testing.T) {
	col := &models.Collection{Name: "posts", Fields: []models.FieldDef{{Name: "title", Type: "text"}}, APIRules: map[string]string{"list": "public"}}
	allowed := allowedFieldMap(col)
	sql, args, next, err := buildWhereClause(col, models.ListOptions{Search: "hello", Filters: map[string]string{"title": "x' OR TRUE"}, FilterExpr: "title != 'hidden'"}, allowed)
	if err != nil || next != 5 || len(args) != 4 || strings.Contains(sql, "x' OR TRUE") {
		t.Fatal(sql, args, next, err)
	}
	for _, sort := range []string{"", "title", "+title", "-title", "id"} {
		if _, err := buildOrderBy(sort, allowed); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := buildOrderBy("missing", allowed); err == nil {
		t.Fatal("unknown sort allowed")
	}
}
func TestCoercion(t *testing.T) {
	if _, err := coerceInputValue(models.FieldDef{Type: "vector", Dimension: 2}, []any{1.0, "bad"}); err == nil {
		t.Fatal("string vector accepted")
	}
	if _, err := coerceInputValue(models.FieldDef{Type: "vector", Dimension: 2}, []any{1.0}); err == nil {
		t.Fatal("wrong dimension accepted")
	}
	if _, err := coerceInputValue(models.FieldDef{Type: "vector", Dimension: 2}, "bad"); err == nil {
		t.Fatal("non-array accepted")
	}
	if _, err := coerceInputValue(models.FieldDef{Type: "json"}, map[string]any{"x": 1}); err != nil {
		t.Fatal(err)
	}
}
