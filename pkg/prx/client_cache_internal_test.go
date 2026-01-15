package prx

import (
	"testing"
)

func TestCacheKeyGeneration(t *testing.T) {
	// Test that cache keys are consistent
	key1 := prCacheKey("github", "owner", "repo", 123)
	key2 := prCacheKey("github", "owner", "repo", 123)

	if key1 != key2 {
		t.Error("Cache keys should be consistent for same inputs")
	}

	// Test that different inputs produce different keys
	key3 := prCacheKey("github", "owner", "repo", 456)
	if key1 == key3 {
		t.Error("Different inputs should produce different cache keys")
	}

	// Test that different platforms produce different keys
	key4 := prCacheKey("gitlab", "owner", "repo", 123)
	if key1 == key4 {
		t.Error("Different platforms should produce different cache keys")
	}

	// Verify key format (should be 64 char hex string)
	if len(key1) != 64 {
		t.Errorf("Cache key should be 64 characters, got %d", len(key1))
	}

	if !isHexString(key1) {
		t.Error("Cache key should be a hex string")
	}
}

func TestIsHexString(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"0123456789abcdef", true},
		{"ABCDEF", true},
		{"0123456789ABCDEF", true},
		{"xyz", false},
		{"12g4", false},
		{"", true}, // Empty string is technically all hex
	}

	for _, tt := range tests {
		result := isHexString(tt.input)
		if result != tt.expected {
			t.Errorf("isHexString(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestRulesetsCacheKey(t *testing.T) {
	key1 := rulesetsCacheKey("owner", "repo")
	key2 := rulesetsCacheKey("owner", "repo")

	if key1 != key2 {
		t.Errorf("Same inputs produced different keys: %s vs %s", key1, key2)
	}

	key3 := rulesetsCacheKey("other", "repo")
	if key1 == key3 {
		t.Error("Different inputs produced same key")
	}

	// Verify format
	expected := "owner/repo"
	if key1 != expected {
		t.Errorf("Expected key %q, got %q", expected, key1)
	}
}
