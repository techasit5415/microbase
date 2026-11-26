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


func TestResolveUserInfoURLAndIAMv2DataEnvelope(t *testing.T) {
	claims := map[string]any{
		"sub": "usr_66050160",
	}
	got := resolveUserInfoURL("https://api.science.kmitl.ac.th/iam/o/v2/users/{user_id}", claims)
	want := "https://api.science.kmitl.ac.th/iam/o/v2/users/usr_66050160"
	if got != want {
		t.Fatalf("resolveUserInfoURL got %q want %q", got, want)
	}

	iamResponse := map[string]any{
		"data": map[string]any{
			"id":          "usr_66050160",
			"email":       "66050160@kmitl.ac.th",
			"role":        "student",
			"status":      "active",
			"firstnameEn": "Techasit",
			"lastnameEn":  "Vanitpattarakul",
			"student_profile": []any{
				map[string]any{
					"department": "Computer Science",
				},
			},
		},
	}
	mergeUserInfoClaims(claims, iamResponse)

	if v := valueToString(extractJSONPath(claims, "email")); v != "66050160@kmitl.ac.th" {
		t.Fatalf("expected flattened email, got %q", v)
	}
	if v := valueToString(extractJSONPath(claims, "data.role")); v != "student" {
		t.Fatalf("expected data.role=student, got %q", v)
	}
	if v := valueToString(extractJSONPath(claims, "student_profile.0.department")); v != "Computer Science" {
		t.Fatalf("expected student_profile.0.department, got %q", v)
	}
	if err := CheckAllowedClaims(map[string][]string{"role": {"student", "staff"}}, claims); err != nil {
		t.Fatalf("expected student role allowed, got %v", err)
	}
}
