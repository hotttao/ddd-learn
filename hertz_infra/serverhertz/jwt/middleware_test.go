package jwt

import "testing"

func TestProtectedPrefixes(t *testing.T) {
	prefixes, err := protectedPrefixes([]string{"/v1/xhs/"})
	if err != nil {
		t.Fatalf("protectedPrefixes() error = %v", err)
	}
	if !matchesProtectedPrefix("/v1/xhs/organizations/1", prefixes) {
		t.Fatal("expected xhs API path to be protected")
	}
	if !matchesProtectedPrefix("/v1/xhs", prefixes) {
		t.Fatal("expected exact xhs API root to be protected")
	}
	if matchesProtectedPrefix("/v1/xhs-admin", prefixes) {
		t.Fatal("expected path segment boundary to be enforced")
	}
	if matchesProtectedPrefix("/health", prefixes) {
		t.Fatal("expected health path to remain public")
	}
}

func TestProtectedPrefixesRejectsUnsafeConfiguration(t *testing.T) {
	for _, values := range [][]string{nil, {""}, {"v1/xhs/"}} {
		if _, err := protectedPrefixes(values); err == nil {
			t.Fatalf("protectedPrefixes(%q) expected error", values)
		}
	}
}
