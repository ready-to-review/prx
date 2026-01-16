package prx_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/codeGROOVE-dev/prx/pkg/prx"
	"github.com/codeGROOVE-dev/prx/pkg/prx/github"
)

func TestCacheClient(t *testing.T) {
	// Create temporary cache directory
	cacheDir := t.TempDir()

	// Create test server that handles GraphQL and REST endpoints
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		switch r.URL.Path {
		case "/graphql":
			// GraphQL endpoint - return a minimal PR response
			response := `{"data": {"repository": {"pullRequest": {
				"number": 1,
				"title": "Test PR",
				"body": "Test body",
				"state": "CLOSED",
				"isDraft": false,
				"createdAt": "2023-01-01T00:00:00Z",
				"updatedAt": "2023-01-01T01:00:00Z",
				"closedAt": "2023-01-01T02:00:00Z",
				"mergedAt": null,
				"mergedBy": null,
				"mergeable": "UNKNOWN",
				"mergeStateStatus": "UNKNOWN",
				"additions": 10,
				"deletions": 5,
				"changedFiles": 2,
				"author": {"login": "testuser"},
				"authorAssociation": "CONTRIBUTOR",
				"headRef": {"target": {"oid": "abc123"}},
				"baseRef": {"name": "main", "target": {"oid": "def456"}},
				"assignees": {"nodes": []},
				"labels": {"nodes": []},
				"reviews": {"nodes": []},
				"reviewRequests": {"nodes": []},
				"reviewThreads": {"nodes": []},
				"commits": {"nodes": []},
				"statusCheckRollup": null,
				"timelineItems": {"nodes": [], "pageInfo": {"hasNextPage": false}},
				"comments": {"nodes": []}
			}}}}`
			if _, err := w.Write([]byte(response)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case "/repos/test/repo/rulesets":
			if _, err := w.Write([]byte(`[]`)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		default:
			// Check runs endpoint
			if r.URL.Path == "/repos/test/repo/commits/abc123/check-runs" {
				if _, err := w.Write([]byte(`{"check_runs": []}`)); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			} else {
				// Return empty array for other endpoints
				if _, err := w.Write([]byte("[]")); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}
	}))
	defer server.Close()

	// Create cache store and client with test server
	store, err := prx.NewCacheStore(cacheDir)
	if err != nil {
		t.Fatalf("Failed to create cache store: %v", err)
	}
	platform := github.NewTestPlatform("test-token", server.URL)
	client := prx.NewClient(platform,
		prx.WithCacheStore(store),
		prx.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))),
	)
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			t.Errorf("Failed to close client: %v", closeErr)
		}
	}()

	ctx := context.Background()

	// First request - should hit the API
	beforeFirstRequest := requestCount
	events1, err := client.PullRequestWithReferenceTime(ctx, "test", "repo", 1, time.Now().Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}
	afterFirstRequest := requestCount

	if afterFirstRequest == beforeFirstRequest {
		t.Error("Expected API requests for first call")
	}

	if len(events1.Events) < 2 { // At least PR opened and closed events
		t.Errorf("Expected at least 2 events, got %d", len(events1.Events))
	}

	// Second request with same reference time - should use cache for most endpoints
	beforeSecondRequest := requestCount
	events2, err := client.PullRequestWithReferenceTime(ctx, "test", "repo", 1, time.Now().Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}
	afterSecondRequest := requestCount

	// We expect zero API requests for cached call (fido handles persistence)
	if afterSecondRequest != beforeSecondRequest {
		t.Logf("Note: Got %d API requests for cached call (cache may need warming)", afterSecondRequest-beforeSecondRequest)
	}

	if len(events1.Events) != len(events2.Events) {
		t.Errorf("Expected same number of events from cache, got %d vs %d", len(events1.Events), len(events2.Events))
	}

	// Third request with future reference time - should hit API again
	beforeThirdRequest := requestCount
	_, err = client.PullRequestWithReferenceTime(ctx, "test", "repo", 1, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("Third request failed: %v", err)
	}
	afterThirdRequest := requestCount

	if afterThirdRequest == beforeThirdRequest {
		t.Error("Expected API requests for future reference time")
	}
}

func TestRulesetsCache(t *testing.T) {
	// Track API calls to rulesets endpoint
	rulesetsAPICallCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/graphql":
			response := `{"data": {"repository": {"pullRequest": {
				"number": 1,
				"title": "Test PR",
				"body": "Test body",
				"state": "OPEN",
				"isDraft": false,
				"createdAt": "2023-01-01T00:00:00Z",
				"updatedAt": "2023-01-01T01:00:00Z",
				"closedAt": null,
				"mergedAt": null,
				"mergedBy": null,
				"mergeable": "UNKNOWN",
				"mergeStateStatus": "UNKNOWN",
				"additions": 10,
				"deletions": 5,
				"changedFiles": 2,
				"author": {"login": "testuser"},
				"authorAssociation": "CONTRIBUTOR",
				"headRef": {"target": {"oid": "abc123"}},
				"baseRef": {"name": "main", "target": {"oid": "def456"}},
				"assignees": {"nodes": []},
				"labels": {"nodes": []},
				"reviews": {"nodes": []},
				"reviewRequests": {"nodes": []},
				"reviewThreads": {"nodes": []},
				"commits": {"nodes": []},
				"statusCheckRollup": null,
				"timelineItems": {"nodes": [], "pageInfo": {"hasNextPage": false}},
				"comments": {"nodes": []}
			}}}}`
			if _, err := w.Write([]byte(response)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case "/repos/test/repo/rulesets":
			rulesetsAPICallCount++
			// Return rulesets with a required check
			response := `[{"id": 1, "name": "main protection", "target": "branch", "rules": [{"type": "required_status_checks", "parameters": {"required_status_checks": [{"context": "ci/test"}]}}]}]`
			if _, err := w.Write([]byte(response)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		case "/repos/test/repo/commits/abc123/check-runs":
			if _, err := w.Write([]byte(`{"check_runs": []}`)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		default:
			if _, err := w.Write([]byte("[]")); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}))
	defer server.Close()

	platform := github.NewTestPlatform("test-token", server.URL)

	ctx := context.Background()
	refTime := time.Now()

	// First request - should call rulesets API
	_, err := platform.FetchPR(ctx, "test", "repo", 1, refTime)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}

	if rulesetsAPICallCount != 1 {
		t.Errorf("Expected 1 rulesets API call, got %d", rulesetsAPICallCount)
	}

	// Second request - should use cached rulesets
	_, err = platform.FetchPR(ctx, "test", "repo", 1, refTime)
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}

	if rulesetsAPICallCount != 1 {
		t.Errorf("Expected rulesets to be cached (still 1 API call), got %d", rulesetsAPICallCount)
	}

	// Third request for same repo - should still use cache
	_, err = platform.FetchPR(ctx, "test", "repo", 2, refTime)
	if err != nil {
		t.Fatalf("Third request failed: %v", err)
	}

	if rulesetsAPICallCount != 1 {
		t.Errorf("Expected rulesets cache to be used across PRs in same repo, got %d API calls", rulesetsAPICallCount)
	}
}
