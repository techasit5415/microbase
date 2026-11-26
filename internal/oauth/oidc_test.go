package oauth

import (
	"errors"
	"testing"
)

func TestCheckAllowedClaimsDepartmentLock(t *testing.T) {
	rawCS := map[string]any{
		"student_id": "66050001",
		"department": map[string]any{
			"id":   "CS",
			"name": "Computer Science",
		},
		"status": "active",
	}
	rawMath := map[string]any{
		"student_id": "66050002",
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
		"sub": "usr_0001",
	}
	got := resolveUserInfoURL("https://iam.example.edu/o/v2/users/{user_id}", claims)
	want := "https://iam.example.edu/o/v2/users/usr_0001"
	if got != want {
		t.Fatalf("resolveUserInfoURL got %q want %q", got, want)
	}

	iamResponse := map[string]any{
		"data": map[string]any{
			"id":          "usr_0001",
			"email":       "student01@example.edu",
			"role":        "student",
			"status":      "active",
			"firstnameEn": "Alice",
			"lastnameEn":  "Example",
			"student_profile": []any{
				map[string]any{
					"department": "Computer Science",
				},
			},
		},
	}
	mergeUserInfoClaims(claims, iamResponse)

	if v := valueToString(extractJSONPath(claims, "email")); v != "student01@example.edu" {
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


func TestCheckAllowedClaimsArrayAndFallbackPaths(t *testing.T) {
	studentCS := map[string]any{
		"role": "student",
		"profile": map[string]any{
			"departmentId": "dept-cs-01",
			"firstNameTh":  "สมชาย",
		},
	}
	staffCS := map[string]any{
		"role": "staff",
		"profile": map[string]any{
			"firstname_th": "อาจารย์สมหญิง",
			"profiles": []any{
				map[string]any{
					"is_default_profile": false,
					"department": map[string]any{
						"id": "dept-math-01",
					},
				},
				map[string]any{
					"is_default_profile": true,
					"department": map[string]any{
						"id": "dept-cs-01",
					},
				},
			},
		},
	}
	staffOther := map[string]any{
		"role": "staff",
		"profile": map[string]any{
			"profiles": []any{
				map[string]any{
					"department": map[string]any{
						"id": "dept-phys-01",
					},
				},
			},
		},
	}

	rules := map[string][]string{
		"role": {"student", "staff"},
		"profile.departmentId | profile.profiles.department.id": {"dept-cs-01"},
	}

	if err := CheckAllowedClaims(rules, studentCS); err != nil {
		t.Fatalf("expected CS student allowed, got %v", err)
	}
	if err := CheckAllowedClaims(rules, staffCS); err != nil {
		t.Fatalf("expected CS staff allowed via profiles array, got %v", err)
	}
	if err := CheckAllowedClaims(rules, staffOther); !errors.Is(err, ErrClaimNotAllowed) {
		t.Fatalf("expected non-CS staff blocked, got %v", err)
	}

	// Wildcard syntax and fallback field mapping
	if v := valueToString(extractJSONPath(studentCS, "profile.firstNameTh | profile.firstname_th")); v != "สมชาย" {
		t.Fatalf("expected student fallback firstname, got %q", v)
	}
	if v := valueToString(extractJSONPath(staffCS, "profile.firstNameTh | profile.firstname_th")); v != "อาจารย์สมหญิง" {
		t.Fatalf("expected staff fallback firstname, got %q", v)
	}
	if err := CheckAllowedClaims(map[string][]string{"profile.profiles.*.department.id": {"dept-cs-01"}}, staffCS); err != nil {
		t.Fatalf("expected wildcard array syntax allowed, got %v", err)
	}
}
