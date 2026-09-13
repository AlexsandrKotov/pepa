package engine

import (
	"testing"
)

// TestIsReservedRoleSlug verifies that reserved role slugs are correctly identified.
// This is critical for security — reserved roles grant admin bypass.
func TestIsReservedRoleSlug(t *testing.T) {
	tests := []struct {
		slug     string
		expected bool
	}{
		// Reserved slugs (should return true).
		{"admin", true},
		{"Admin", true},
		{"ADMIN", true},
		{"super_admin", true},
		{"Super_Admin", true},
		{"platform admin", true},
		{"Platform Admin", true},
		{"platform_admin", true},
		{"Platform_Admin", true},
		{"  admin  ", true}, // with whitespace

		// Non-reserved slugs (should return false).
		{"developer", false},
		{"viewer", false},
		{"custom_role", false},
		{"admin_viewer", false}, // contains "admin" but not exact match
		{"super_admin_custom", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			result := IsReservedRoleSlug(tt.slug)
			if result != tt.expected {
				t.Errorf("IsReservedRoleSlug(%q) = %v, want %v", tt.slug, result, tt.expected)
			}
		})
	}
}

// TestReservedRoleSlugsList verifies the list contains expected values.
func TestReservedRoleSlugsList(t *testing.T) {
	expected := map[string]bool{
		"admin":          true,
		"super_admin":    true,
		"platform admin": true,
		"platform_admin": true,
	}

	for _, slug := range ReservedRoleSlugs {
		if !expected[slug] {
			t.Errorf("unexpected reserved role slug: %q", slug)
		}
	}

	if len(ReservedRoleSlugs) != len(expected) {
		t.Errorf("ReservedRoleSlugs has %d items, expected %d", len(ReservedRoleSlugs), len(expected))
	}
}

// TestAllRBACResourcesNotEmpty verifies the resource list is populated.
func TestAllRBACResourcesNotEmpty(t *testing.T) {
	if len(AllRBACResources) == 0 {
		t.Fatal("AllRBACResources is empty — this will cause all non-admin users to get 403")
	}

	// Verify critical resources are present.
	critical := map[string]bool{
		"services":     false,
		"deployments":  false,
		"gitops":       false,
		"connections":  false,
		"environments": false,
	}

	for _, res := range AllRBACResources {
		if _, ok := critical[res]; ok {
			critical[res] = true
		}
	}

	for res, found := range critical {
		if !found {
			t.Errorf("critical resource %q missing from AllRBACResources", res)
		}
	}
}
