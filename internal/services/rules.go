package services

import (
	"errors"
	"strings"

	"github.com/techasit5415/microbase/internal/models"
)

var ErrForbidden = errors.New("only superusers can perform this action")

// CheckAPIRule evaluates a PocketBase-style rule expression for a collection action.
// Rules follow PocketBase's secure-by-default model:
//   - Key absent (nil) or "locked"/"superuser": Superusers only (403 for everyone else)
//   - Key present with "" (empty) or "public": Public access (anyone)
//   - "auth" or "@request.auth.id != ''": Any authenticated user
//   - "admin" or "@request.auth.role = 'admin'": Admin or Superuser
func CheckAPIRule(col *models.Collection, action string, claims *AuthClaims) error {
	// Superusers always bypass API rules.
	if claims != nil && claims.Role == "superuser" {
		return nil
	}
	if col.APIRules == nil {
		return ErrForbidden
	}

	raw, exists := col.APIRules[action]
	if !exists {
		return ErrForbidden
	}

	rule := strings.TrimSpace(raw)
	if rule == "" || strings.EqualFold(rule, "public") {
		return nil
	}

	lower := strings.ToLower(rule)
	switch {
	case lower == "locked" || lower == "superuser":
		return ErrForbidden
	case lower == "admin" || strings.Contains(lower, "role = 'admin'") || strings.Contains(lower, `role = "admin"`):
		if claims == nil || (claims.Role != "admin" && claims.Role != "superuser") {
			return ErrForbidden
		}
		return nil
	case lower == "auth" || strings.Contains(lower, "@request.auth.id"):
		if claims == nil {
			return ErrForbidden
		}
		return nil
	default:
		if claims == nil {
			return ErrForbidden
		}
		return nil
	}
}
