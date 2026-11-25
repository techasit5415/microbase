package services

import (
	"errors"
	"strings"

	"github.com/techasit5415/microbase/internal/models"
)

var ErrForbidden = errors.New("access denied by collection api rules")

// CheckAPIRule evaluates a PocketBase-style rule expression for a collection action.
// Supported rules:
//   - "" (empty): public access
//   - "auth" or "@request.auth.id != ''": requires any authenticated user
//   - "admin" or "@request.auth.role = 'admin'": requires role admin or superuser
//   - "locked": requires superuser only
func CheckAPIRule(col *models.Collection, action string, claims *AuthClaims) error {
	if claims != nil && claims.Role == "superuser" {
		return nil
	}
	if col.APIRules == nil {
		return nil
	}

	rule := strings.TrimSpace(col.APIRules[action])
	if rule == "" {
		return nil
	}

	lower := strings.ToLower(rule)
	switch {
	case lower == "locked":
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
		// Any other non-empty rule defaults to requiring authentication.
		if claims == nil {
			return ErrForbidden
		}
		return nil
	}
}
