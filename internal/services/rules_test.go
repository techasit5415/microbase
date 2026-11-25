package services

import (
	"errors"
	"testing"

	"github.com/techasit5415/microbase/internal/models"
)

func TestCompileAPIRuleRowLevel(t *testing.T) {
	col := &models.Collection{
		Name: "posts",
		Fields: []models.FieldDef{
			{Name: "owner_id", Type: "text"},
			{Name: "status", Type: "text"},
		},
		APIRules: map[string]string{
			"list":   "status = 'published' || owner_id = @request.auth.id",
			"view":   "owner_id = @request.auth.id",
			"update": "@request.auth.id = owner_id && @request.body.status != 'archived'",
			"delete": "@request.auth.role = 'admin'",
		},
	}

	userClaims := &AuthClaims{UserID: "42", Email: "u@test.com", Role: "user"}
	adminClaims := &AuthClaims{UserID: "1", Email: "a@test.com", Role: "admin"}
	superClaims := &AuthClaims{UserID: "9", Email: "s@test.com", Role: "superuser"}

	// 1. Superuser always bypasses
	sql, args, _, err := CompileAPIRule(col, "view", superClaims, nil, 2)
	if err != nil || sql != "" || len(args) != 0 {
		t.Fatalf("expected superuser bypass, got sql=%q args=%v err=%v", sql, args, err)
	}

	// 2. Locked (absent key) denies regular user
	colLocked := &models.Collection{Name: "secret", APIRules: map[string]string{}}
	if _, _, _, err := CompileAPIRule(colLocked, "list", userClaims, nil, 1); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for locked rule, got %v", err)
	}

	// 3. Row-level OR expression for anonymous user -> ("status" = $1 OR FALSE)
	sql, args, nextIdx, err := CompileAPIRule(col, "list", nil, nil, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != `("status" = $1 OR FALSE)` || len(args) != 1 || args[0] != "published" || nextIdx != 2 {
		t.Fatalf("unexpected anon list SQL: %q args=%v nextIdx=%d", sql, args, nextIdx)
	}

	// 4. Row-level OR expression for logged-in user -> ("status" = $1 OR CAST("owner_id" AS TEXT) = $2)
	sql, args, nextIdx, err = CompileAPIRule(col, "list", userClaims, nil, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != `("status" = $1 OR CAST("owner_id" AS TEXT) = $2)` || len(args) != 2 || args[0] != "published" || args[1] != "42" || nextIdx != 3 {
		t.Fatalf("unexpected user list SQL: %q args=%v nextIdx=%d", sql, args, nextIdx)
	}

	// 5. Reversed @request.auth.id = owner_id + @request.body check on update
	sql, args, _, err = CompileAPIRule(col, "update", userClaims, map[string]any{"status": "draft"}, 3)
	if err != nil {
		t.Fatalf("unexpected update error: %v", err)
	}
	if sql != `(CAST("owner_id" AS TEXT) = $3 AND TRUE)` || len(args) != 1 || args[0] != "42" {
		t.Fatalf("unexpected update SQL: %q args=%v", sql, args)
	}

	// 6. Static role expression on delete
	if _, _, _, err := CompileAPIRule(col, "delete", userClaims, nil, 2); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-admin delete, got %v", err)
	}
	if sql, _, _, err := CompileAPIRule(col, "delete", adminClaims, nil, 2); err != nil || sql != "" {
		t.Fatalf("expected admin delete allowed, got sql=%q err=%v", sql, err)
	}
}

func TestUsersCollectionRowLevelRule(t *testing.T) {
	usersCol := &models.Collection{
		Name: "users",
		Type: "auth",
		APIRules: map[string]string{
			"list":   "id = @request.auth.id",
			"view":   "id = @request.auth.id",
			"create": "@request.body.role != 'admin'",
		},
	}
	userClaims := &AuthClaims{UserID: "7", Email: "me@kmitl.ac.th", Role: "user"}

	sql, args, _, err := CompileAPIRule(usersCol, "view", userClaims, nil, 2)
	if err != nil {
		t.Fatalf("unexpected users view error: %v", err)
	}
	if sql != `CAST("id" AS TEXT) = $2` || len(args) != 1 || args[0] != "7" {
		t.Fatalf("unexpected users view SQL: %q args=%v", sql, args)
	}

	// Create rule in-memory evaluation
	if err := EvalCreateRule(usersCol, nil, map[string]any{"email": "a@b.com", "role": "admin"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden when creating admin role, got %v", err)
	}
	if err := EvalCreateRule(usersCol, nil, map[string]any{"email": "a@b.com", "role": "user"}); err != nil {
		t.Fatalf("expected user create allowed, got %v", err)
	}
}
