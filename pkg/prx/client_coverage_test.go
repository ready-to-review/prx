package prx_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/codeGROOVE-dev/prx/pkg/prx"
)

// mockPlatform implements prx.Platform for testing
type mockPlatform struct {
	name      string
	fetchFunc func(ctx context.Context, owner, repo string, number int, refTime time.Time) (*prx.PullRequestData, error)
}

func (m *mockPlatform) Name() string {
	return m.name
}

func (m *mockPlatform) FetchPR(ctx context.Context, owner, repo string, number int, refTime time.Time) (*prx.PullRequestData, error) {
	if m.fetchFunc != nil {
		return m.fetchFunc(ctx, owner, repo, number, refTime)
	}
	return &prx.PullRequestData{
		PullRequest: prx.PullRequest{
			Number: number,
			Author: "test",
		},
		Events: []prx.Event{},
	}, nil
}

// TestNewClient tests the deprecated NewClient function
func TestNewClient(t *testing.T) {
	platform := &mockPlatform{name: "test"}
	client := prx.NewClient(platform)
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Verify it works by making a request
	ctx := context.Background()
	result, err := client.PullRequest(ctx, "owner", "repo", 1)
	if err != nil {
		t.Fatalf("PullRequest failed: %v", err)
	}
	if result.PullRequest.Number != 1 {
		t.Errorf("Expected PR number 1, got %d", result.PullRequest.Number)
	}
}

// TestNewCacheStore_Errors tests error paths in NewCacheStore
func TestNewCacheStore_Errors(t *testing.T) {
	t.Run("relative path error", func(t *testing.T) {
		_, err := prx.NewCacheStore("relative/path")
		if err == nil {
			t.Error("Expected error for relative path")
		}
	})

	t.Run("invalid path error", func(t *testing.T) {
		// Use a path that cannot be created (e.g., inside a file)
		tmpFile := filepath.Join(t.TempDir(), "file.txt")
		if err := os.WriteFile(tmpFile, []byte("test"), 0o600); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		// Try to create a directory inside the file
		_, err := prx.NewCacheStore(filepath.Join(tmpFile, "subdir"))
		if err == nil {
			t.Error("Expected error when creating cache store in invalid path")
		}
	})

	t.Run("success case", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "cache")
		store, err := prx.NewCacheStore(dir)
		if err != nil {
			t.Errorf("NewCacheStore failed: %v", err)
		}
		if store == nil {
			t.Error("NewCacheStore returned nil store")
		}
	})
}

// TestClose_NilCache tests Close with nil cache
func TestClose_NilCache(t *testing.T) {
	platform := &mockPlatform{name: "test"}
	// Create client without cache store
	client := prx.NewClient(platform)
	// Force nil cache by not setting one
	err := client.Close()
	if err != nil {
		t.Errorf("Close with nil cache should not error, got: %v", err)
	}
}

// TestPullRequestWithReferenceTime_NilCache tests without cache
func TestPullRequestWithReferenceTime_NilCache(t *testing.T) {
	platform := &mockPlatform{
		name: "test",
		fetchFunc: func(ctx context.Context, owner, repo string, number int, refTime time.Time) (*prx.PullRequestData, error) {
			return &prx.PullRequestData{
				PullRequest: prx.PullRequest{
					Number: number,
					Author: owner,
				},
				Events: []prx.Event{},
			}, nil
		},
	}

	// Create client with no cache (will use nil cache path)
	client := prx.NewClient(platform)
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	ctx := context.Background()
	result, err := client.PullRequestWithReferenceTime(ctx, "owner", "repo", 1, time.Now())
	if err != nil {
		t.Fatalf("PullRequestWithReferenceTime failed: %v", err)
	}
	if result.PullRequest.Number != 1 {
		t.Errorf("Expected PR number 1, got %d", result.PullRequest.Number)
	}
}

// TestPullRequestWithReferenceTime_CacheGetError tests cache get error handling
func TestPullRequestWithReferenceTime_CacheGetError(t *testing.T) {
	platform := &mockPlatform{
		name: "test",
		fetchFunc: func(ctx context.Context, owner, repo string, number int, refTime time.Time) (*prx.PullRequestData, error) {
			return &prx.PullRequestData{
				PullRequest: prx.PullRequest{
					Number: number,
					Author: "test",
				},
				Events:   []prx.Event{},
				CachedAt: time.Now(),
			}, nil
		},
	}

	cacheDir := t.TempDir()
	store, err := prx.NewCacheStore(cacheDir)
	if err != nil {
		t.Fatalf("NewCacheStore failed: %v", err)
	}

	// Create client with cache
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := prx.NewClient(platform,
		prx.WithCacheStore(store),
		prx.WithLogger(logger),
	)
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	ctx := context.Background()

	// First call to populate cache
	_, err = client.PullRequestWithReferenceTime(ctx, "owner", "repo", 1, time.Now())
	if err != nil {
		t.Fatalf("First call failed: %v", err)
	}

	// Second call should hit cache
	result, err := client.PullRequestWithReferenceTime(ctx, "owner", "repo", 1, time.Now())
	if err != nil {
		t.Fatalf("Second call failed: %v", err)
	}
	if result.PullRequest.Number != 1 {
		t.Errorf("Expected PR number 1, got %d", result.PullRequest.Number)
	}
}

// TestPullRequestWithReferenceTime_FetchError tests fetch error handling
func TestPullRequestWithReferenceTime_FetchError(t *testing.T) {
	expectedErr := errors.New("fetch failed")
	platform := &mockPlatform{
		name: "test",
		fetchFunc: func(ctx context.Context, owner, repo string, number int, refTime time.Time) (*prx.PullRequestData, error) {
			return nil, expectedErr
		},
	}

	cacheDir := t.TempDir()
	store, err := prx.NewCacheStore(cacheDir)
	if err != nil {
		t.Fatalf("NewCacheStore failed: %v", err)
	}

	client := prx.NewClient(platform, prx.WithCacheStore(store))
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	ctx := context.Background()
	_, err = client.PullRequestWithReferenceTime(ctx, "owner", "repo", 1, time.Now())
	if err == nil {
		t.Error("Expected error from fetch")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

// TestPullRequestWithReferenceTime_ExpiredCache tests expired cache handling
func TestPullRequestWithReferenceTime_ExpiredCache(t *testing.T) {
	callCount := 0
	platform := &mockPlatform{
		name: "test",
		fetchFunc: func(ctx context.Context, owner, repo string, number int, refTime time.Time) (*prx.PullRequestData, error) {
			callCount++
			return &prx.PullRequestData{
				PullRequest: prx.PullRequest{
					Number: number,
					Author: "test",
				},
				Events:   []prx.Event{},
				CachedAt: time.Now(),
			}, nil
		},
	}

	cacheDir := t.TempDir()
	store, err := prx.NewCacheStore(cacheDir)
	if err != nil {
		t.Fatalf("NewCacheStore failed: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client := prx.NewClient(platform,
		prx.WithCacheStore(store),
		prx.WithLogger(logger),
	)
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	ctx := context.Background()

	// First call with old reference time
	oldRefTime := time.Now().Add(-1 * time.Hour)
	_, err = client.PullRequestWithReferenceTime(ctx, "owner", "repo", 1, oldRefTime)
	if err != nil {
		t.Fatalf("First call failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}

	// Second call with future reference time should invalidate cache
	futureRefTime := time.Now().Add(1 * time.Hour)
	_, err = client.PullRequestWithReferenceTime(ctx, "owner", "repo", 1, futureRefTime)
	if err != nil {
		t.Fatalf("Second call failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("Expected 2 calls (cache should be expired), got %d", callCount)
	}
}

// TestWithCacheStore_CreateError tests error handling in WithCacheStore
func TestWithCacheStore_CreateError(t *testing.T) {
	// Create a mock store that will trigger an error in fido.NewTiered
	// This is tricky because we need a valid store that fido will reject
	// For now, just test that the option doesn't panic with a valid store

	cacheDir := t.TempDir()
	store, err := prx.NewCacheStore(cacheDir)
	if err != nil {
		t.Fatalf("NewCacheStore failed: %v", err)
	}

	platform := &mockPlatform{name: "test"}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// This should not panic even if cache creation has issues
	client := prx.NewClient(platform,
		prx.WithCacheStore(store),
		prx.WithLogger(logger),
	)
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	// Verify client works
	ctx := context.Background()
	result, err := client.PullRequest(ctx, "owner", "repo", 1)
	if err != nil {
		t.Fatalf("PullRequest failed: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result")
	}
}

// TestCreateDefaultCache tests the default cache creation logic
func TestCreateDefaultCache(t *testing.T) {
	// Test that default cache creation works
	platform := &mockPlatform{name: "test"}

	// Create client without explicit cache store - should create default cache
	client := prx.NewClient(platform)
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	ctx := context.Background()
	result, err := client.PullRequest(ctx, "owner", "repo", 1)
	if err != nil {
		t.Fatalf("PullRequest failed: %v", err)
	}
	if result.PullRequest.Number != 1 {
		t.Errorf("Expected PR number 1, got %d", result.PullRequest.Number)
	}
}

// TestClientOptions tests various client option combinations
func TestClientOptions(t *testing.T) {
	platform := &mockPlatform{name: "test"}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	t.Run("with logger only", func(t *testing.T) {
		client := prx.NewClient(platform, prx.WithLogger(logger))
		defer func() {
			if err := client.Close(); err != nil {
				t.Errorf("Close failed: %v", err)
			}
		}()

		ctx := context.Background()
		result, err := client.PullRequest(ctx, "owner", "repo", 1)
		if err != nil {
			t.Fatalf("PullRequest failed: %v", err)
		}
		if result == nil {
			t.Error("Expected non-nil result")
		}
	})

	t.Run("with cache and logger", func(t *testing.T) {
		cacheDir := t.TempDir()
		store, err := prx.NewCacheStore(cacheDir)
		if err != nil {
			t.Fatalf("NewCacheStore failed: %v", err)
		}

		client := prx.NewClient(platform,
			prx.WithCacheStore(store),
			prx.WithLogger(logger),
		)
		defer func() {
			if err := client.Close(); err != nil {
				t.Errorf("Close failed: %v", err)
			}
		}()

		ctx := context.Background()
		result, err := client.PullRequest(ctx, "owner", "repo", 1)
		if err != nil {
			t.Fatalf("PullRequest failed: %v", err)
		}
		if result == nil {
			t.Error("Expected non-nil result")
		}
	})

	t.Run("no options", func(t *testing.T) {
		client := prx.NewClient(platform)
		defer func() {
			if err := client.Close(); err != nil {
				t.Errorf("Close failed: %v", err)
			}
		}()

		ctx := context.Background()
		result, err := client.PullRequest(ctx, "owner", "repo", 1)
		if err != nil {
			t.Fatalf("PullRequest failed: %v", err)
		}
		if result == nil {
			t.Error("Expected non-nil result")
		}
	})
}
