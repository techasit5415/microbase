package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/techasit5415/microbase/internal/models"
)

var ErrForbidden = errors.New("access denied by collection api rules")

// ruleAllowedFields returns the map of valid columns for a collection's API rule expressions.
func ruleAllowedFields(col *models.Collection) map[string]models.FieldDef {
	allowed := make(map[string]models.FieldDef, len(col.Fields)+8)
	for _, f := range col.Fields {
		allowed[f.Name] = f
	}
	if col.Name == "users" || col.Type == "auth" {
		allowed["email"] = models.FieldDef{Name: "email", Type: "email"}
		allowed["role"] = models.FieldDef{Name: "role", Type: "text"}
		allowed["verified"] = models.FieldDef{Name: "verified", Type: "bool"}
		allowed["email_visibility"] = models.FieldDef{Name: "email_visibility", Type: "bool"}
	}
	return allowed
}

// CompileAPIRule evaluates a collection's PocketBase-style API rule for list/view/update/delete.
// It returns:
//   - err == ErrForbidden if access is statically denied (e.g., locked rule, unauthenticated "auth", or "FALSE")
//   - whereSQL == "" if access is unconditionally granted (e.g., superuser, public "", or statically "TRUE")
//   - whereSQL != "" (with parameterized args) for row-level SQL filtering (e.g., owner_id = @request.auth.id)
func CompileAPIRule(col *models.Collection, action string, claims *AuthClaims, body map[string]any, startArgIdx int) (string, []any, int, error) {
	// Superusers always bypass API rules.
	if claims != nil && claims.Role == "superuser" {
		return "", nil, startArgIdx, nil
	}
	if col == nil || col.APIRules == nil {
		return "", nil, startArgIdx, ErrForbidden
	}

	raw, exists := col.APIRules[action]
	if !exists {
		return "", nil, startArgIdx, ErrForbidden
	}

	rule := strings.TrimSpace(raw)
	if rule == "" || strings.EqualFold(rule, "public") {
		return "", nil, startArgIdx, nil
	}

	lower := strings.ToLower(rule)
	switch lower {
	case "locked", "superuser":
		return "", nil, startArgIdx, ErrForbidden
	case "admin":
		if claims == nil || (claims.Role != "admin" && claims.Role != "superuser") {
			return "", nil, startArgIdx, ErrForbidden
		}
		return "", nil, startArgIdx, nil
	case "auth":
		if claims == nil {
			return "", nil, startArgIdx, ErrForbidden
		}
		return "", nil, startArgIdx, nil
	}

	sqlExpr, args, nextIdx, err := ParseFilterToSQLWithBody(rule, ruleAllowedFields(col), claims, body, startArgIdx)
	if err != nil {
		return "", nil, startArgIdx, fmt.Errorf("%w: invalid api rule %q: %v", ErrForbidden, rule, err)
	}
	if sqlExpr == "FALSE" {
		return "", nil, startArgIdx, ErrForbidden
	}
	if sqlExpr == "TRUE" {
		return "", nil, startArgIdx, nil
	}
	return sqlExpr, args, nextIdx, nil
}

// EvalCreateRule evaluates a collection's "create" API rule against the incoming payload and auth claims.
func EvalCreateRule(col *models.Collection, claims *AuthClaims, data map[string]any) error {
	if claims != nil && claims.Role == "superuser" {
		return nil
	}
	if col == nil || col.APIRules == nil {
		return ErrForbidden
	}
	raw, exists := col.APIRules["create"]
	if !exists {
		return ErrForbidden
	}
	rule := strings.TrimSpace(raw)
	if rule == "" || strings.EqualFold(rule, "public") {
		return nil
	}
	lower := strings.ToLower(rule)
	switch lower {
	case "locked", "superuser":
		return ErrForbidden
	case "admin":
		if claims == nil || (claims.Role != "admin" && claims.Role != "superuser") {
			return ErrForbidden
		}
		return nil
	case "auth":
		if claims == nil {
			return ErrForbidden
		}
		return nil
	}

	ok, err := EvalFilterOnData(rule, data, claims)
	if err != nil || !ok {
		return ErrForbidden
	}
	return nil
}

// CheckAPIRule evaluates a static rule check (retained for backwards compatibility).
func CheckAPIRule(col *models.Collection, action string, claims *AuthClaims) error {
	_, _, _, err := CompileAPIRule(col, action, claims, nil, 1)
	return err
}
