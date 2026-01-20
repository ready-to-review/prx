//nolint:errcheck // Test handlers don't need to check w.Write errors
package gitea

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codeGROOVE-dev/prx/pkg/prx"
)

// Test helper function
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func TestPlatform_Name(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{
			name:    "codeberg",
			baseURL: "https://codeberg.org",
			want:    prx.PlatformCodeberg,
		},
		{
			name:    "self-hosted gitea",
			baseURL: "https://gitea.example.com",
			want:    "gitea",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPlatform("token", WithBaseURL(tt.baseURL))
			if got := p.Name(); got != tt.want {
				t.Errorf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewCodebergPlatform(t *testing.T) {
	p := NewCodebergPlatform("test-token")
	if p.Name() != prx.PlatformCodeberg {
		t.Errorf("NewCodebergPlatform().Name() = %q, want %q", p.Name(), prx.PlatformCodeberg)
	}
}

func TestPlatform_FetchPR(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, "/pulls/123") && !strings.Contains(r.URL.Path, "/reviews") && !strings.Contains(r.URL.Path, "/commits"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 1,
				"number": 123,
				"title": "Test PR",
				"body": "Test description",
				"state": "open",
				"draft": false,
				"mergeable": true,
				"merged": false,
				"additions": 100,
				"deletions": 50,
				"changed_files": 5,
				"created_at": "2024-01-01T10:00:00Z",
				"updated_at": "2024-01-02T12:00:00Z",
				"user": {
					"id": 1,
					"login": "testauthor",
					"full_name": "Test Author"
				},
				"head": {
					"ref": "feature-branch",
					"sha": "abc123def456789"
				},
				"base": {
					"ref": "main",
					"sha": "base123456789"
				},
				"labels": [
					{"id": 1, "name": "bug", "color": "ff0000"}
				],
				"assignees": [
					{"id": 2, "login": "assignee1"}
				],
				"requested_reviewers": [
					{"id": 3, "login": "reviewer1"}
				]
			}`))

		case strings.Contains(r.URL.Path, "/reviews"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": 1,
					"user": {"id": 4, "login": "reviewer2"},
					"state": "APPROVED",
					"body": "LGTM!",
					"submitted_at": "2024-01-02T10:00:00Z",
					"official": true,
					"stale": false,
					"dismissed": false
				}
			]`))

		case strings.Contains(r.URL.Path, "/comments"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": 1,
					"user": {"id": 5, "login": "commenter1"},
					"body": "Can you clarify this?",
					"created_at": "2024-01-01T14:00:00Z",
					"updated_at": "2024-01-01T14:00:00Z"
				}
			]`))

		case strings.Contains(r.URL.Path, "/commits"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"sha": "abc123def456789",
					"commit": {
						"message": "Initial commit\n\nWith more details",
						"author": {
							"name": "Test Author",
							"email": "test@example.com",
							"date": "2024-01-01T09:00:00Z"
						},
						"committer": {
							"name": "Test Author",
							"email": "test@example.com",
							"date": "2024-01-01T09:00:00Z"
						}
					},
					"author": {"id": 1, "login": "testauthor"}
				}
			]`))

		case strings.Contains(r.URL.Path, "/timeline"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": 1,
					"type": "label",
					"created_at": "2024-01-01T11:00:00Z",
					"user": {"id": 1, "login": "testauthor"},
					"label": {"id": 1, "name": "bug"}
				}
			]`))

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message": "not found"}`))
		}
	}))
	defer server.Close()

	p := NewPlatform("test-token", WithBaseURL(server.URL))
	ctx := context.Background()

	data, err := p.FetchPR(ctx, "owner", "repo", 123, time.Now())
	if err != nil {
		t.Fatalf("FetchPR() error = %v", err)
	}

	pr := data.PullRequest

	// Verify basic PR fields
	if pr.Number != 123 {
		t.Errorf("Number = %d, want 123", pr.Number)
	}
	if pr.Title != "Test PR" {
		t.Errorf("Title = %q, want %q", pr.Title, "Test PR")
	}
	if pr.Author != "testauthor" {
		t.Errorf("Author = %q, want %q", pr.Author, "testauthor")
	}
	if pr.State != "open" {
		t.Errorf("State = %q, want %q", pr.State, "open")
	}
	if pr.Draft {
		t.Error("Draft = true, want false")
	}
	if !*pr.Mergeable {
		t.Error("Mergeable = false, want true")
	}
	if pr.MergeableState != "clean" {
		t.Errorf("MergeableState = %q, want %q", pr.MergeableState, "clean")
	}
	if pr.Additions != 100 {
		t.Errorf("Additions = %d, want 100", pr.Additions)
	}
	if pr.Deletions != 50 {
		t.Errorf("Deletions = %d, want 50", pr.Deletions)
	}
	if pr.ChangedFiles != 5 {
		t.Errorf("ChangedFiles = %d, want 5", pr.ChangedFiles)
	}
	if pr.HeadSHA != "abc123def456789" {
		t.Errorf("HeadSHA = %q, want %q", pr.HeadSHA, "abc123def456789")
	}

	// Verify labels
	if len(pr.Labels) != 1 || pr.Labels[0] != "bug" {
		t.Errorf("Labels = %v, want [bug]", pr.Labels)
	}

	// Verify assignees
	if len(pr.Assignees) != 1 || pr.Assignees[0] != "assignee1" {
		t.Errorf("Assignees = %v, want [assignee1]", pr.Assignees)
	}

	// Verify reviewers (requested + approved)
	if len(pr.Reviewers) != 2 {
		t.Errorf("len(Reviewers) = %d, want 2", len(pr.Reviewers))
	}
	if pr.Reviewers["reviewer1"] != prx.ReviewStatePending {
		t.Errorf("Reviewers[reviewer1] = %v, want %v", pr.Reviewers["reviewer1"], prx.ReviewStatePending)
	}
	if pr.Reviewers["reviewer2"] != prx.ReviewStateApproved {
		t.Errorf("Reviewers[reviewer2] = %v, want %v", pr.Reviewers["reviewer2"], prx.ReviewStateApproved)
	}

	// Verify events
	if len(data.Events) < 4 {
		t.Errorf("len(Events) = %d, want at least 4 (pr_opened, commit, review, comment)", len(data.Events))
	}

	// Check for expected event types
	eventTypes := make(map[string]bool)
	for _, e := range data.Events {
		eventTypes[e.Kind] = true
	}
	expectedTypes := []string{prx.EventKindPROpened, prx.EventKindCommit, prx.EventKindReview, prx.EventKindComment, prx.EventKindLabeled}
	for _, et := range expectedTypes {
		if !eventTypes[et] {
			t.Errorf("Missing event type %q in events", et)
		}
	}
}

func TestPlatform_FetchPR_Merged(t *testing.T) {
	mergedAt := "2024-01-03T15:00:00Z"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, "/pulls/456") && !strings.Contains(r.URL.Path, "/reviews") && !strings.Contains(r.URL.Path, "/commits"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 2,
				"number": 456,
				"title": "Merged PR",
				"body": "",
				"state": "closed",
				"draft": false,
				"mergeable": false,
				"merged": true,
				"merged_at": "` + mergedAt + `",
				"closed_at": "` + mergedAt + `",
				"created_at": "2024-01-01T10:00:00Z",
				"updated_at": "2024-01-03T15:00:00Z",
				"user": {"id": 1, "login": "author"},
				"merged_by": {"id": 2, "login": "merger"},
				"head": {"ref": "feature", "sha": "merged123"},
				"base": {"ref": "main", "sha": "base456"},
				"labels": [],
				"assignees": [],
				"requested_reviewers": []
			}`))
		case strings.Contains(r.URL.Path, "/reviews"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(r.URL.Path, "/comments"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(r.URL.Path, "/commits"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(r.URL.Path, "/timeline"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	p := NewPlatform("test-token", WithBaseURL(server.URL))
	ctx := context.Background()

	data, err := p.FetchPR(ctx, "owner", "repo", 456, time.Now())
	if err != nil {
		t.Fatalf("FetchPR() error = %v", err)
	}

	pr := data.PullRequest

	if !pr.Merged {
		t.Error("Merged = false, want true")
	}
	if pr.MergedBy != "merger" {
		t.Errorf("MergedBy = %q, want %q", pr.MergedBy, "merger")
	}
	if pr.MergedAt == nil {
		t.Error("MergedAt = nil, want non-nil")
	}

	// Check for merged event
	hasMergedEvent := false
	for _, e := range data.Events {
		if e.Kind == prx.EventKindPRMerged {
			hasMergedEvent = true
			if e.Actor != "merger" {
				t.Errorf("Merged event actor = %q, want %q", e.Actor, "merger")
			}
		}
	}
	if !hasMergedEvent {
		t.Error("Missing pr_merged event")
	}
}

func TestPlatform_FetchPR_Draft(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, "/pulls/789") && !strings.Contains(r.URL.Path, "/reviews") && !strings.Contains(r.URL.Path, "/commits"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 3,
				"number": 789,
				"title": "WIP: Draft PR",
				"body": "",
				"state": "open",
				"draft": true,
				"mergeable": false,
				"merged": false,
				"created_at": "2024-01-01T10:00:00Z",
				"updated_at": "2024-01-01T10:00:00Z",
				"user": {"id": 1, "login": "author"},
				"head": {"ref": "wip", "sha": "draft123"},
				"base": {"ref": "main", "sha": "base789"},
				"labels": [],
				"assignees": [],
				"requested_reviewers": []
			}`))
		case strings.Contains(r.URL.Path, "/reviews"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(r.URL.Path, "/comments"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(r.URL.Path, "/commits"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(r.URL.Path, "/timeline"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	p := NewPlatform("test-token", WithBaseURL(server.URL))
	ctx := context.Background()

	data, err := p.FetchPR(ctx, "owner", "repo", 789, time.Now())
	if err != nil {
		t.Fatalf("FetchPR() error = %v", err)
	}

	pr := data.PullRequest

	if !pr.Draft {
		t.Error("Draft = false, want true")
	}
	if pr.MergeableState != "draft" {
		t.Errorf("MergeableState = %q, want %q", pr.MergeableState, "draft")
	}
}

func TestPlatform_FetchPR_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "pull request not found"}`))
	}))
	defer server.Close()

	p := NewPlatform("test-token", WithBaseURL(server.URL))
	ctx := context.Background()

	_, err := p.FetchPR(ctx, "owner", "repo", 999, time.Now())
	if err == nil {
		t.Fatal("FetchPR() expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Error should contain 404, got: %v", err)
	}
}

func TestConvertReviewState(t *testing.T) {
	tests := []struct {
		input string
		want  prx.ReviewState
	}{
		{"APPROVED", prx.ReviewStateApproved},
		{"approved", prx.ReviewStateApproved},
		{"REQUEST_CHANGES", prx.ReviewStateChangesRequested},
		{"request_changes", prx.ReviewStateChangesRequested},
		{"COMMENT", prx.ReviewStateCommented},
		{"comment", prx.ReviewStateCommented},
		{"PENDING", prx.ReviewStatePending},
		{"pending", prx.ReviewStatePending},
		{"unknown", prx.ReviewStatePending},
		{"", prx.ReviewStatePending},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertReviewState(tt.input)
			if got != tt.want {
				t.Errorf("convertReviewState(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestConvertReviewOutcome(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"APPROVED", "approved"},
		{"approved", "approved"},
		{"REQUEST_CHANGES", "changes_requested"},
		{"COMMENT", "commented"},
		{"PENDING", "pending"},
		{"unknown", "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertReviewOutcome(tt.input)
			if got != tt.want {
				t.Errorf("convertReviewOutcome(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConvertTimelineEvent(t *testing.T) {
	now := time.Now()
	testUser := &user{Login: "testuser"}
	testAssignee := &user{Login: "assignee"}
	testLabel := &label{Name: "bug"}

	tests := []struct {
		name      string
		event     timelineEvent
		wantKind  string
		wantActor string
		wantNil   bool
	}{
		{
			name:      "label event",
			event:     timelineEvent{Type: "label", CreatedAt: now, User: testUser, Label: testLabel},
			wantKind:  prx.EventKindLabeled,
			wantActor: "testuser",
		},
		{
			name:      "unlabel event",
			event:     timelineEvent{Type: "unlabel", CreatedAt: now, User: testUser, Label: testLabel},
			wantKind:  prx.EventKindUnlabeled,
			wantActor: "testuser",
		},
		{
			name:    "label event without label",
			event:   timelineEvent{Type: "label", CreatedAt: now, User: testUser},
			wantNil: true,
		},
		{
			name:      "assignees event",
			event:     timelineEvent{Type: "assignees", CreatedAt: now, User: testUser, Assignee: testAssignee},
			wantKind:  prx.EventKindAssigned,
			wantActor: "testuser",
		},
		{
			name:      "unassignees event",
			event:     timelineEvent{Type: "unassignees", CreatedAt: now, User: testUser, Assignee: testAssignee},
			wantKind:  prx.EventKindUnassigned,
			wantActor: "testuser",
		},
		{
			name:      "review_requested event",
			event:     timelineEvent{Type: "review_requested", CreatedAt: now, User: testUser, Assignee: testAssignee},
			wantKind:  prx.EventKindReviewRequested,
			wantActor: "testuser",
		},
		{
			name:      "close event",
			event:     timelineEvent{Type: "close", CreatedAt: now, User: testUser},
			wantKind:  prx.EventKindClosed,
			wantActor: "testuser",
		},
		{
			name:      "reopen event",
			event:     timelineEvent{Type: "reopen", CreatedAt: now, User: testUser},
			wantKind:  prx.EventKindReopened,
			wantActor: "testuser",
		},
		{
			name:      "change_title event",
			event:     timelineEvent{Type: "change_title", CreatedAt: now, User: testUser, Body: "old -> new"},
			wantKind:  prx.EventKindRenamedTitle,
			wantActor: "testuser",
		},
		{
			name:      "change_ref event",
			event:     timelineEvent{Type: "change_ref", CreatedAt: now, User: testUser, OldRef: "old", NewRef: "new"},
			wantKind:  prx.EventKindBaseRefChanged,
			wantActor: "testuser",
		},
		{
			name:      "merge event",
			event:     timelineEvent{Type: "merge", CreatedAt: now, User: testUser},
			wantKind:  prx.EventKindMerged,
			wantActor: "testuser",
		},
		{
			name:      "comment_ref event",
			event:     timelineEvent{Type: "comment_ref", CreatedAt: now, User: testUser},
			wantKind:  prx.EventKindCrossReferenced,
			wantActor: "testuser",
		},
		{
			name:    "unknown event",
			event:   timelineEvent{Type: "unknown_type", CreatedAt: now, User: testUser},
			wantNil: true,
		},
		{
			name:      "event without user",
			event:     timelineEvent{Type: "close", CreatedAt: now},
			wantKind:  prx.EventKindClosed,
			wantActor: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertTimelineEvent(&tt.event)

			if tt.wantNil {
				if got != nil {
					t.Errorf("convertTimelineEvent() = %+v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatal("convertTimelineEvent() = nil, want non-nil")
			}
			if got.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.wantKind)
			}
			if got.Actor != tt.wantActor {
				t.Errorf("Actor = %q, want %q", got.Actor, tt.wantActor)
			}
		})
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"single line", "single line"},
		{"first\nsecond", "first"},
		{"first\nsecond\nthird", "first"},
		{"", ""},
		{"\nstarting with newline", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := firstLine(tt.input)
			if got != tt.want {
				t.Errorf("firstLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPlatform_WithOptions(t *testing.T) {
	t.Run("WithLogger", func(t *testing.T) {
		p := NewPlatform("token", WithLogger(nil))
		if p == nil {
			t.Error("NewPlatform returned nil")
		}
	})

	t.Run("WithHTTPClient", func(t *testing.T) {
		customClient := &http.Client{Timeout: 60 * time.Second}
		p := NewPlatform("token", WithHTTPClient(customClient))
		if p.httpClient != customClient {
			t.Error("Custom HTTP client not set")
		}
	})

	t.Run("WithBaseURL", func(t *testing.T) {
		p := NewPlatform("token", WithBaseURL("https://gitea.example.com/"))
		if p.baseURL != "https://gitea.example.com" {
			t.Errorf("baseURL = %q, want %q", p.baseURL, "https://gitea.example.com")
		}
	})
}

func TestConvertPullRequest(t *testing.T) {
	now := time.Now()
	pr := &pullRequest{
		Number:             42,
		Title:              "Test PR",
		Body:               "Description",
		State:              "open",
		Draft:              false,
		Mergeable:          true,
		Merged:             false,
		Additions:          10,
		Deletions:          5,
		ChangedFiles:       2,
		CreatedAt:          now,
		UpdatedAt:          now,
		User:               user{Login: "author"},
		Head:               branch{SHA: "abc123"},
		Labels:             []label{{Name: "enhancement"}},
		Assignees:          []user{{Login: "dev1"}, {Login: "dev2"}},
		RequestedReviewers: []user{{Login: "reviewer"}},
	}

	reviews := []review{
		{User: user{Login: "approver"}, State: "APPROVED", SubmittedAt: now},
	}

	result := convertPullRequest(pr, reviews)

	if result.Number != 42 {
		t.Errorf("Number = %d, want 42", result.Number)
	}
	if result.Title != "Test PR" {
		t.Errorf("Title = %q, want %q", result.Title, "Test PR")
	}
	if result.Author != "author" {
		t.Errorf("Author = %q, want %q", result.Author, "author")
	}
	if len(result.Labels) != 1 || result.Labels[0] != "enhancement" {
		t.Errorf("Labels = %v, want [enhancement]", result.Labels)
	}
	if len(result.Assignees) != 2 {
		t.Errorf("len(Assignees) = %d, want 2", len(result.Assignees))
	}
	if result.Reviewers["reviewer"] != prx.ReviewStatePending {
		t.Errorf("Reviewers[reviewer] = %v, want pending", result.Reviewers["reviewer"])
	}
	if result.Reviewers["approver"] != prx.ReviewStateApproved {
		t.Errorf("Reviewers[approver] = %v, want approved", result.Reviewers["approver"])
	}
	if result.MergeableState != "clean" {
		t.Errorf("MergeableState = %q, want clean", result.MergeableState)
	}
}

func TestConvertPullRequest_StaleReview(t *testing.T) {
	now := time.Now()
	pr := &pullRequest{
		Number:    1,
		State:     "open",
		User:      user{Login: "author"},
		Head:      branch{SHA: "abc"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	reviews := []review{
		{User: user{Login: "reviewer1"}, State: "APPROVED", Stale: true},
		{User: user{Login: "reviewer2"}, State: "APPROVED", Dismissed: true},
		{User: user{Login: "reviewer3"}, State: "APPROVED", Stale: false, Dismissed: false},
	}

	result := convertPullRequest(pr, reviews)

	// Stale and dismissed reviews should not update reviewer state
	if _, exists := result.Reviewers["reviewer1"]; exists {
		t.Error("Stale review should not update reviewer state")
	}
	if _, exists := result.Reviewers["reviewer2"]; exists {
		t.Error("Dismissed review should not update reviewer state")
	}
	if result.Reviewers["reviewer3"] != prx.ReviewStateApproved {
		t.Errorf("Reviewers[reviewer3] = %v, want approved", result.Reviewers["reviewer3"])
	}
}

func TestPlatform_FetchPR_CacheHit(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, "/pulls/100"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 100,
				"number": 100,
				"title": "Cached PR",
				"state": "open",
				"draft": false,
				"created_at": "2024-01-01T10:00:00Z",
				"updated_at": "2024-01-01T12:00:00Z",
				"user": {"id": 1, "login": "author"},
				"head": {"ref": "feature", "sha": "cache123"},
				"base": {"ref": "main", "sha": "base123"},
				"labels": [],
				"assignee": null,
				"assignees": []
			}`))
		case strings.Contains(r.URL.Path, "/reviews"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(r.URL.Path, "/comments"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(r.URL.Path, "/commits"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"sha": "cache123", "commit": {"message": "test", "author": {"name": "author", "date": "2024-01-01T10:00:00Z"}}}]`))
		case strings.Contains(r.URL.Path, "/timeline"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()

	p := NewPlatform("test-token", WithBaseURL(server.URL))
	ctx := context.Background()

	// First request - cache miss
	refTime := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)
	_, err := p.FetchPR(ctx, "owner", "repo", 100, refTime)
	if err != nil {
		t.Fatalf("First FetchPR() error = %v", err)
	}
	firstRequestCount := requestCount

	// Second request with same refTime - should hit cache
	_, err = p.FetchPR(ctx, "owner", "repo", 100, refTime)
	if err != nil {
		t.Fatalf("Second FetchPR() error = %v", err)
	}

	// Verify cache was used (request count should not increase significantly)
	// Note: PR fetch will still happen, but supplemental data should be cached
	if requestCount <= firstRequestCount {
		t.Errorf("Expected cache hit for supplemental data, requestCount = %d after first = %d", requestCount, firstRequestCount)
	}
	// After cache hit, we expect only 1 new request (for the PR itself), not 5
	if requestCount > firstRequestCount+1 {
		t.Errorf("Too many requests after cache hit: %d new requests, expected 1", requestCount-firstRequestCount)
	}
}
