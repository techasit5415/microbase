package oauth

import (
	"errors"
	"testing"
)

func TestCheckAllowedClaimsDepartmentLock(t *testing.T) {
	rawCS := map[string]any{
		"student_id": "66050160",
		"department": map[string]any{
			"id":   "CS",
			"name": "Computer Science",
		},
		"status": "active",
	}
	rawMath := map[string]any{
		"student_id": "66050999",
		"department": map[string]any{
			"id":   "MATH",
			"name": "Mathematics",
		},
		"status": "active",
	}

	// Empty rules allow everyone
	if err := CheckAllowedClaims(nil, rawMath); err != nil {
		t.Fatalf("expected nil for empty rules, got %v", err)
	}

	rules := map[string][]string{
		"department.name": {"Computer Science", "วิทยาการคอมพิวเตอร์"},
		"status":          {"active"},
	}

	// Matching department allowed
	if err := CheckAllowedClaims(rules, rawCS); err != nil {
		t.Fatalf("expected CS user allowed, got %v", err)
	}

	// Non-matching department blocked
	if err := CheckAllowedClaims(rules, rawMath); !errors.Is(err, ErrClaimNotAllowed) {
		t.Fatalf("expected ErrClaimNotAllowed for Math user, got %v", err)
	}
}
