//nolint:errcheck // Test handlers don't need to check w.Write errors
package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codeGROOVE-dev/prx/pkg/prx/types"
)

func TestPlatform_Name(t *testing.T) {
	p := NewPlatform("token")
	if got := p.Name(); got != types.PlatformGitLab {
		t.Errorf("Name() = %q, want %q", got, types.PlatformGitLab)
	}
}

func TestNewPlatform(t *testing.T) {
	p := NewPlatform("test-token")
	if p.token != "test-token" {
		t.Errorf("token = %q, want %q", p.token, "test-token")
	}
	if p.baseURL != "https://gitlab.com" {
		t.Errorf("baseURL = %q, want %q", p.baseURL, "https://gitlab.com")
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
		p := NewPlatform("token", WithBaseURL("https://gitlab.example.com/"))
		if p.baseURL != "https://gitlab.example.com" {
			t.Errorf("baseURL = %q, want %q", p.baseURL, "https://gitlab.example.com")
		}
	})
}

func TestPlatform_FetchPR(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/merge_requests/123") && !strings.Contains(path, "/approvals") && !strings.Contains(path, "/pipelines") && !strings.Contains(path, "/notes") && !strings.Contains(path, "/discussions") && !strings.Contains(path, "/commits"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 1,
				"iid": 123,
				"title": "Test MR",
				"description": "Test description",
				"state": "opened",
				"draft": false,
				"work_in_progress": false,
				"has_conflicts": false,
				"merge_status": "can_be_merged",
				"detailed_merge_status": "mergeable",
				"sha": "abc123def456",
				"created_at": "2024-01-01T10:00:00Z",
				"updated_at": "2024-01-02T12:00:00Z",
				"author": {
					"id": 1,
					"username": "testauthor",
					"name": "Test Author"
				},
				"diff_refs": {
					"head_sha": "abc123def456",
					"base_sha": "base123",
					"start_sha": "start123"
				},
				"head_pipeline": {
					"id": 100,
					"status": "success",
					"created_at": "2024-01-01T11:00:00Z",
					"updated_at": "2024-01-01T12:00:00Z"
				},
				"labels": ["bug", "priority::high"],
				"assignees": [
					{"id": 2, "username": "assignee1"}
				],
				"reviewers": [
					{"id": 3, "username": "reviewer1", "state": "unreviewed"}
				]
			}`))

		case strings.Contains(path, "/approvals"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"approved": true,
				"approvals_required": 1,
				"approvals_left": 0,
				"approved_by": [
					{"user": {"id": 4, "username": "approver1"}}
				]
			}`))

		case strings.Contains(path, "/pipelines"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": 100,
					"status": "success",
					"ref": "feature-branch",
					"sha": "abc123def456",
					"created_at": "2024-01-01T11:00:00Z",
					"updated_at": "2024-01-01T12:00:00Z",
					"started_at": "2024-01-01T11:05:00Z",
					"finished_at": "2024-01-01T11:30:00Z"
				}
			]`))

		case strings.Contains(path, "/notes"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": 1,
					"body": "LGTM!",
					"author": {"id": 5, "username": "commenter1"},
					"created_at": "2024-01-01T14:00:00Z",
					"updated_at": "2024-01-01T14:00:00Z",
					"system": false,
					"resolvable": false,
					"resolved": false
				},
				{
					"id": 2,
					"body": "approved this merge request",
					"author": {"id": 4, "username": "approver1"},
					"created_at": "2024-01-01T15:00:00Z",
					"updated_at": "2024-01-01T15:00:00Z",
					"system": true,
					"resolvable": false,
					"resolved": false
				}
			]`))

		case strings.Contains(path, "/discussions"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": "disc1",
					"individual_note": false,
					"notes": [
						{
							"id": 10,
							"body": "Can you explain this?",
							"author": {"id": 6, "username": "reviewer2"},
							"created_at": "2024-01-01T16:00:00Z",
							"updated_at": "2024-01-01T16:00:00Z",
							"system": false,
							"resolvable": true,
							"resolved": false
						}
					]
				}
			]`))

		case strings.Contains(path, "/commits"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": "abc123def456789full",
					"short_id": "abc123d",
					"title": "Initial commit",
					"message": "Initial commit\n\nWith details",
					"author_name": "Test Author",
					"author_email": "test@example.com",
					"authored_date": "2024-01-01T09:00:00Z",
					"committer_name": "Test Author",
					"committer_email": "test@example.com",
					"committed_date": "2024-01-01T09:00:00Z"
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
	if pr.Title != "Test MR" {
		t.Errorf("Title = %q, want %q", pr.Title, "Test MR")
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
	if pr.HeadSHA != "abc123def456" {
		t.Errorf("HeadSHA = %q, want %q", pr.HeadSHA, "abc123def456")
	}
	if pr.TestState != types.TestStatePassing {
		t.Errorf("TestState = %q, want %q", pr.TestState, types.TestStatePassing)
	}

	// Verify labels
	if len(pr.Labels) != 2 {
		t.Errorf("len(Labels) = %d, want 2", len(pr.Labels))
	}

	// Verify assignees
	if len(pr.Assignees) != 1 || pr.Assignees[0] != "assignee1" {
		t.Errorf("Assignees = %v, want [assignee1]", pr.Assignees)
	}

	// Verify reviewers
	if pr.Reviewers["approver1"] != types.ReviewStateApproved {
		t.Errorf("Reviewers[approver1] = %v, want %v", pr.Reviewers["approver1"], types.ReviewStateApproved)
	}

	// Verify commits
	if len(pr.Commits) != 1 || pr.Commits[0] != "abc123d" {
		t.Errorf("Commits = %v, want [abc123d]", pr.Commits)
	}

	// Verify events exist
	if len(data.Events) < 3 {
		t.Errorf("len(Events) = %d, want at least 3", len(data.Events))
	}

	// Check for expected event types
	eventTypes := make(map[string]bool)
	for _, e := range data.Events {
		eventTypes[e.Kind] = true
	}
	expectedTypes := []string{types.EventKindPROpened, types.EventKindCommit, types.EventKindComment}
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

		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/merge_requests/456") && !strings.Contains(path, "/approvals"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 2,
				"iid": 456,
				"title": "Merged MR",
				"description": "",
				"state": "merged",
				"draft": false,
				"has_conflicts": false,
				"merge_status": "can_be_merged",
				"sha": "merged123",
				"created_at": "2024-01-01T10:00:00Z",
				"updated_at": "2024-01-03T15:00:00Z",
				"merged_at": "` + mergedAt + `",
				"closed_at": "` + mergedAt + `",
				"author": {"id": 1, "username": "author"},
				"merged_by": {"id": 2, "username": "merger"},
				"diff_refs": {"head_sha": "merged123"},
				"labels": [],
				"assignees": [],
				"reviewers": []
			}`))
		case strings.Contains(path, "/approvals"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"approved": false, "approved_by": []}`))
		case strings.Contains(path, "/pipelines"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(path, "/notes"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(path, "/discussions"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(path, "/commits"):
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
		if e.Kind == types.EventKindPRMerged {
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

		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/merge_requests/789") && !strings.Contains(path, "/approvals"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 3,
				"iid": 789,
				"title": "Draft: WIP MR",
				"description": "",
				"state": "opened",
				"draft": true,
				"work_in_progress": true,
				"has_conflicts": false,
				"merge_status": "cannot_be_merged",
				"detailed_merge_status": "draft_status",
				"sha": "draft123",
				"created_at": "2024-01-01T10:00:00Z",
				"updated_at": "2024-01-01T10:00:00Z",
				"author": {"id": 1, "username": "author"},
				"diff_refs": {"head_sha": "draft123"},
				"labels": [],
				"assignees": [],
				"reviewers": []
			}`))
		case strings.Contains(path, "/approvals"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"approved": false, "approved_by": []}`))
		case strings.Contains(path, "/pipelines"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(path, "/notes"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(path, "/discussions"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(path, "/commits"):
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

func TestPlatform_FetchPR_FailingPipeline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/merge_requests/101") && !strings.Contains(path, "/approvals"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": 4,
				"iid": 101,
				"title": "Failing Pipeline MR",
				"state": "opened",
				"sha": "fail123",
				"created_at": "2024-01-01T10:00:00Z",
				"updated_at": "2024-01-01T10:00:00Z",
				"author": {"id": 1, "username": "author"},
				"diff_refs": {"head_sha": "fail123"},
				"head_pipeline": {
					"id": 200,
					"status": "failed"
				},
				"labels": [],
				"assignees": [],
				"reviewers": []
			}`))
		case strings.Contains(path, "/approvals"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"approved": false, "approved_by": []}`))
		case strings.Contains(path, "/pipelines"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(path, "/notes"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(path, "/discussions"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(path, "/commits"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	p := NewPlatform("test-token", WithBaseURL(server.URL))
	ctx := context.Background()

	data, err := p.FetchPR(ctx, "owner", "repo", 101, time.Now())
	if err != nil {
		t.Fatalf("FetchPR() error = %v", err)
	}

	if data.PullRequest.TestState != types.TestStateFailing {
		t.Errorf("TestState = %q, want %q", data.PullRequest.TestState, types.TestStateFailing)
	}
}

func TestPlatform_FetchPR_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "merge request not found"}`))
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

func TestPlatform_TokenAuth(t *testing.T) {
	t.Run("PAT token uses Private-Token header", func(t *testing.T) {
		var receivedHeader string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeader = r.Header.Get("Private-Token")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer server.Close()

		p := NewPlatform("glpat-test123", WithBaseURL(server.URL))
		_ = p.doRequest(context.Background(), server.URL+"/test", &struct{}{})

		if receivedHeader != "glpat-test123" {
			t.Errorf("Private-Token header = %q, want %q", receivedHeader, "glpat-test123")
		}
	})

	t.Run("OAuth2 token uses Bearer header", func(t *testing.T) {
		var receivedHeader string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeader = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer server.Close()

		p := NewPlatform("oauth2-token-here", WithBaseURL(server.URL))
		_ = p.doRequest(context.Background(), server.URL+"/test", &struct{}{})

		if receivedHeader != "Bearer oauth2-token-here" {
			t.Errorf("Authorization header = %q, want %q", receivedHeader, "Bearer oauth2-token-here")
		}
	})
}

func TestConvertState(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"opened", "open"},
		{"closed", "closed"},
		{"merged", "closed"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertState(tt.input)
			if got != tt.want {
				t.Errorf("convertState(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConvertReviewerState(t *testing.T) {
	tests := []struct {
		input string
		want  types.ReviewState
	}{
		{"reviewed", types.ReviewStateCommented},
		{"unreviewed", types.ReviewStatePending},
		{"", types.ReviewStatePending},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertReviewerState(tt.input)
			if got != tt.want {
				t.Errorf("convertReviewerState(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestConvertPipelineToTestState(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"success", types.TestStatePassing},
		{"failed", types.TestStateFailing},
		{"running", types.TestStateRunning},
		{"pending", types.TestStatePending},
		{"waiting_for_resource", types.TestStatePending},
		{"preparing", types.TestStatePending},
		{"created", types.TestStateQueued},
		{"scheduled", types.TestStateQueued},
		{"canceled", types.TestStateNone},
		{"skipped", types.TestStateNone},
		{"manual", types.TestStateNone},
		{"unknown", types.TestStateNone},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertPipelineToTestState(tt.input)
			if got != tt.want {
				t.Errorf("convertPipelineToTestState(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConvertPipelineStatus(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"success", "success"},
		{"failed", "failure"},
		{"canceled", "cancelled"},
		{"cancelled", "cancelled"},
		{"skipped", "skipped"},
		{"running", "running"},
		{"pending", "pending"},
		{"created", "pending"},
		{"waiting_for_resource", "pending"},
		{"preparing", "pending"},
		{"manual", "action_required"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertPipelineStatus(tt.input)
			if got != tt.want {
				t.Errorf("convertPipelineStatus(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConvertMergeStatus(t *testing.T) {
	tests := []struct {
		name          string
		mr            *mergeRequest
		wantMergeable string
	}{
		{
			name:          "mergeable",
			mr:            &mergeRequest{DetailedMergeStatus: "mergeable"},
			wantMergeable: "clean",
		},
		{
			name:          "ci_must_pass",
			mr:            &mergeRequest{DetailedMergeStatus: "ci_must_pass"},
			wantMergeable: "blocked",
		},
		{
			name:          "ci_still_running",
			mr:            &mergeRequest{DetailedMergeStatus: "ci_still_running"},
			wantMergeable: "blocked",
		},
		{
			name:          "discussions_not_resolved",
			mr:            &mergeRequest{DetailedMergeStatus: "discussions_not_resolved"},
			wantMergeable: "blocked",
		},
		{
			name:          "not_approved",
			mr:            &mergeRequest{DetailedMergeStatus: "not_approved"},
			wantMergeable: "blocked",
		},
		{
			name:          "conflict",
			mr:            &mergeRequest{DetailedMergeStatus: "conflict"},
			wantMergeable: "dirty",
		},
		{
			name:          "need_rebase",
			mr:            &mergeRequest{DetailedMergeStatus: "need_rebase"},
			wantMergeable: "dirty",
		},
		{
			name:          "checking",
			mr:            &mergeRequest{DetailedMergeStatus: "checking"},
			wantMergeable: "unknown",
		},
		{
			name:          "draft_status",
			mr:            &mergeRequest{DetailedMergeStatus: "draft_status"},
			wantMergeable: "draft",
		},
		{
			name:          "fallback can_be_merged",
			mr:            &mergeRequest{MergeStatus: "can_be_merged"},
			wantMergeable: "clean",
		},
		{
			name:          "fallback cannot_be_merged",
			mr:            &mergeRequest{MergeStatus: "cannot_be_merged"},
			wantMergeable: "dirty",
		},
		{
			name:          "fallback unknown",
			mr:            &mergeRequest{MergeStatus: "unchecked"},
			wantMergeable: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertMergeStatus(tt.mr)
			if got != tt.wantMergeable {
				t.Errorf("convertMergeStatus() = %q, want %q", got, tt.wantMergeable)
			}
		})
	}
}

func TestIsBot(t *testing.T) {
	tests := []struct {
		name    string
		user    *user
		wantBot bool
	}{
		{"nil user", nil, false},
		{"regular user", &user{Username: "developer"}, false},
		{"bot suffix", &user{Username: "ci[bot]"}, true},
		{"bot suffix 2", &user{Username: "dependabot-bot"}, true},
		{"ghost user", &user{Username: "ghost"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBot(tt.user)
			if got != tt.wantBot {
				t.Errorf("isBot() = %v, want %v", got, tt.wantBot)
			}
		})
	}
}

func TestConvertSystemNote(t *testing.T) {
	now := time.Now()
	author := user{Username: "testuser"}

	tests := []struct {
		name     string
		n        *note
		wantKind string
		wantNil  bool
	}{
		{
			name:     "approved",
			n:        &note{Body: "approved this merge request", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindReview,
		},
		{
			name:     "unapproved",
			n:        &note{Body: "unapproved this merge request", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindReview,
		},
		{
			name:     "requested review",
			n:        &note{Body: "requested review from @reviewer", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindReviewRequested,
		},
		{
			name:     "assigned",
			n:        &note{Body: "assigned to @assignee", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindAssigned,
		},
		{
			name:     "unassigned",
			n:        &note{Body: "unassigned @user", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindUnassigned,
		},
		{
			name:     "added label",
			n:        &note{Body: "added ~bug label", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindLabeled,
		},
		{
			name:     "removed label",
			n:        &note{Body: "removed ~bug label", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindUnlabeled,
		},
		{
			name:     "marked as draft",
			n:        &note{Body: "marked as a draft", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindConvertToDraft,
		},
		{
			name:     "marked ready",
			n:        &note{Body: "marked this merge request as ready", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindReadyForReview,
		},
		{
			name:     "changed target branch",
			n:        &note{Body: "changed target branch from main to develop", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindBaseRefChanged,
		},
		{
			name:     "mentioned in",
			n:        &note{Body: "mentioned in issue #123", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindCrossReferenced,
		},
		{
			name:     "closed",
			n:        &note{Body: "closed", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindClosed,
		},
		{
			name:     "reopened",
			n:        &note{Body: "reopened", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindReopened,
		},
		{
			name:     "changed title",
			n:        &note{Body: "changed title from old to new", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindRenamedTitle,
		},
		{
			name:     "unknown system note",
			n:        &note{Body: "some unknown system action", Author: author, CreatedAt: now, System: true},
			wantKind: types.EventKindComment,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertSystemNote(tt.n)
			if tt.wantNil {
				if got != nil {
					t.Errorf("convertSystemNote() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("convertSystemNote() = nil, want non-nil")
			}
			if got.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.wantKind)
			}
		})
	}
}

func TestConvertNote(t *testing.T) {
	now := time.Now()
	author := user{Username: "commenter"}

	t.Run("regular comment", func(t *testing.T) {
		n := &note{
			Body:      "This looks good!",
			Author:    author,
			CreatedAt: now,
			System:    false,
		}
		got := convertNote(n)
		if got.Kind != types.EventKindComment {
			t.Errorf("Kind = %q, want %q", got.Kind, types.EventKindComment)
		}
		if got.Actor != "commenter" {
			t.Errorf("Actor = %q, want %q", got.Actor, "commenter")
		}
	})

	t.Run("question comment", func(t *testing.T) {
		n := &note{
			Body:      "Can you explain this?",
			Author:    author,
			CreatedAt: now,
			System:    false,
		}
		got := convertNote(n)
		if !got.Question {
			t.Error("Question = false, want true")
		}
	})

	t.Run("resolved comment", func(t *testing.T) {
		n := &note{
			Body:      "Fixed",
			Author:    author,
			CreatedAt: now,
			System:    false,
			Resolved:  true,
		}
		got := convertNote(n)
		if !got.Outdated {
			t.Error("Outdated = false, want true")
		}
	})

	t.Run("system note delegates", func(t *testing.T) {
		n := &note{
			Body:      "approved this merge request",
			Author:    author,
			CreatedAt: now,
			System:    true,
		}
		got := convertNote(n)
		if got.Kind != types.EventKindReview {
			t.Errorf("Kind = %q, want %q", got.Kind, types.EventKindReview)
		}
	})
}

func TestConvertPipeline(t *testing.T) {
	now := time.Now()
	started := now.Add(-30 * time.Minute)
	finished := now.Add(-5 * time.Minute)

	t.Run("completed pipeline", func(t *testing.T) {
		p := &pipeline{
			ID:         123,
			Status:     "success",
			StartedAt:  &started,
			FinishedAt: &finished,
		}
		events := convertPipeline(p)
		if len(events) != 2 {
			t.Fatalf("len(events) = %d, want 2", len(events))
		}
		// Check started event
		if events[0].Outcome != "running" && events[0].Outcome != "pending" {
			t.Errorf("Started event outcome = %q, want running or pending", events[0].Outcome)
		}
		// Check finished event
		if events[1].Outcome != "success" {
			t.Errorf("Finished event outcome = %q, want success", events[1].Outcome)
		}
	})

	t.Run("pipeline not started", func(t *testing.T) {
		p := &pipeline{
			ID:     124,
			Status: "pending",
		}
		events := convertPipeline(p)
		if len(events) != 0 {
			t.Errorf("len(events) = %d, want 0 for pending pipeline without timestamps", len(events))
		}
	})

	t.Run("running pipeline", func(t *testing.T) {
		p := &pipeline{
			ID:        125,
			Status:    "running",
			StartedAt: &started,
		}
		events := convertPipeline(p)
		if len(events) != 1 {
			t.Fatalf("len(events) = %d, want 1", len(events))
		}
		if events[0].Outcome != "running" {
			t.Errorf("Outcome = %q, want running", events[0].Outcome)
		}
	})
}

func TestExtractMentionFromNote(t *testing.T) {
	tests := []struct {
		body string
		want string
	}{
		{"requested review from @reviewer", "reviewer"},
		{"assigned to @assignee", "assignee"},
		{"no mention here", ""},
		{"@first @second", "first"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			got := extractMentionFromNote(tt.body)
			if got != tt.want {
				t.Errorf("extractMentionFromNote(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}

func TestSafeUsername(t *testing.T) {
	tests := []struct {
		name string
		u    *user
		want string
	}{
		{"nil user", nil, ""},
		{"valid user", &user{Username: "testuser"}, "testuser"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeUsername(tt.u)
			if got != tt.want {
				t.Errorf("safeUsername() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetDetailedStatusText(t *testing.T) {
	tests := []struct {
		name   string
		status *detailedStatus
		want   string
	}{
		{"nil status", nil, ""},
		{"with text", &detailedStatus{Text: "Pipeline passed"}, "Pipeline passed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDetailedStatusText(tt.status)
			if got != tt.want {
				t.Errorf("getDetailedStatusText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestURLEncode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"owner/repo", "owner%2Frepo"},
		{"group/subgroup/project", "group%2Fsubgroup%2Fproject"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := urlEncode(tt.input)
			if got != tt.want {
				t.Errorf("urlEncode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConvertMergeRequest(t *testing.T) {
	now := time.Now()
	mr := &mergeRequest{
		IID:          42,
		Title:        "Test MR",
		Description:  "Description",
		State:        "opened",
		Draft:        false,
		CreatedAt:    now,
		UpdatedAt:    now,
		Author:       user{Username: "author"},
		DiffRefs:     &diffRefs{HeadSHA: "abc123"},
		Labels:       []string{"enhancement"},
		Assignees:    []user{{Username: "dev1"}},
		Reviewers:    []reviewerState{{user: user{Username: "reviewer1"}, State: "unreviewed"}},
		HeadPipeline: &pipeline{Status: "success"},
		MergeStatus:  "can_be_merged",
	}

	a := &approvals{
		ApprovedBy: []approvalUser{{User: user{Username: "approver1"}}},
	}

	commits := []commit{
		{ShortID: "abc1234"},
		{ShortID: "def5678"},
	}

	result := convertMergeRequest(mr, a, commits)

	if result.Number != 42 {
		t.Errorf("Number = %d, want 42", result.Number)
	}
	if result.Title != "Test MR" {
		t.Errorf("Title = %q, want %q", result.Title, "Test MR")
	}
	if result.Author != "author" {
		t.Errorf("Author = %q, want %q", result.Author, "author")
	}
	if result.State != "open" {
		t.Errorf("State = %q, want open", result.State)
	}
	if len(result.Commits) != 2 {
		t.Errorf("len(Commits) = %d, want 2", len(result.Commits))
	}
	if result.Reviewers["approver1"] != types.ReviewStateApproved {
		t.Errorf("Reviewers[approver1] = %v, want approved", result.Reviewers["approver1"])
	}
	if result.TestState != types.TestStatePassing {
		t.Errorf("TestState = %q, want %q", result.TestState, types.TestStatePassing)
	}
}

func TestConvertMergeRequest_NoHeadSHA(t *testing.T) {
	now := time.Now()
	mr := &mergeRequest{
		IID:       1,
		State:     "opened",
		CreatedAt: now,
		UpdatedAt: now,
		Author:    user{Username: "author"},
		SHA:       "fallback-sha",
	}

	result := convertMergeRequest(mr, nil, nil)

	if result.HeadSHA != "fallback-sha" {
		t.Errorf("HeadSHA = %q, want fallback-sha", result.HeadSHA)
	}
}

func TestConvertMergeRequest_Merged(t *testing.T) {
	now := time.Now()
	mergedAt := now.Add(-time.Hour)
	mr := &mergeRequest{
		IID:       1,
		State:     "merged",
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now,
		MergedAt:  &mergedAt,
		MergedBy:  &user{Username: "merger"},
		Author:    user{Username: "author"},
	}

	result := convertMergeRequest(mr, nil, nil)

	if !result.Merged {
		t.Error("Merged = false, want true")
	}
	if result.MergedBy != "merger" {
		t.Errorf("MergedBy = %q, want merger", result.MergedBy)
	}
}

func TestConvertMergeRequest_BotAuthor(t *testing.T) {
	now := time.Now()
	mr := &mergeRequest{
		IID:       1,
		State:     "opened",
		CreatedAt: now,
		UpdatedAt: now,
		Author:    user{Username: "dependabot-bot"},
	}

	result := convertMergeRequest(mr, nil, nil)

	if !result.AuthorBot {
		t.Error("AuthorBot = false, want true")
	}
}
