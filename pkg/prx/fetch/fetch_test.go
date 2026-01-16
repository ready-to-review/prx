package fetch_test

import (
	"context"
	"testing"

	"github.com/codeGROOVE-dev/prx/pkg/prx/fetch"
)

func TestFetch_InvalidURL(t *testing.T) {
	ctx := context.Background()

	_, err := fetch.Fetch(ctx, "not-a-valid-url")
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

func TestFetch_EmptyURL(t *testing.T) {
	ctx := context.Background()

	_, err := fetch.Fetch(ctx, "")
	if err == nil {
		t.Error("expected error for empty URL, got nil")
	}
}

func TestWithOptions_Compatibility(t *testing.T) {
	ctx := context.Background()

	// Test the deprecated WithOptions function still works
	_, err := fetch.WithOptions(ctx, "invalid-url")
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

// Note: This package is a thin convenience wrapper that combines:
// - URL parsing (tested in pkg/prx/url_test.go)
// - Authentication resolution (tested in pkg/prx/auth/auth_test.go)
// - Platform client creation (tested in each platform package)
// - Pull request fetching (tested in each platform package)
//
// Full integration tests would require real API tokens and network access.
// The coverage here reflects that this is glue code - the real logic
// is thoroughly tested in the underlying packages.
