package prx_test

import (
	"testing"

	"github.com/codeGROOVE-dev/prx/pkg/prx"
	"github.com/codeGROOVE-dev/prx/pkg/prx/github"
)

func TestClientClose(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := prx.NewCacheStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cache store: %v", err)
	}
	client := prx.NewClient(github.NewTestPlatform("test-token", ""), prx.WithCacheStore(store))

	// Close should not error
	if err := client.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Closing again should be safe
	if err := client.Close(); err != nil {
		t.Errorf("Second close returned error: %v", err)
	}
}
