package types

import (
	"testing"
	"time"
)

func TestFinalizePullRequest(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name             string
		pr               *PullRequest
		events           []Event
		requiredChecks   []string
		testStateFromAPI string
		validate         func(t *testing.T, pr *PullRequest)
	}{
		{
			name: "sets test state from API",
			pr:   &PullRequest{},
			events: []Event{
				{Kind: EventKindCheckRun, Body: "test", Outcome: "success", Timestamp: now},
			},
			testStateFromAPI: TestStatePassing,
			validate: func(t *testing.T, pr *PullRequest) {
				if pr.TestState != TestStatePassing {
					t.Errorf("TestState = %q, expected %q", pr.TestState, TestStatePassing)
				}
				if pr.CheckSummary == nil {
					t.Fatal("CheckSummary is nil")
				}
				if pr.ApprovalSummary == nil {
					t.Fatal("ApprovalSummary is nil")
				}
				if pr.ParticipantAccess == nil {
					t.Fatal("ParticipantAccess is nil")
				}
			},
		},
		{
			name: "sets mergeable to false for blocked state",
			pr: &PullRequest{
				MergeableState: "blocked",
			},
			events: []Event{},
			validate: func(t *testing.T, pr *PullRequest) {
				if pr.Mergeable == nil {
					t.Fatal("Mergeable is nil")
				}
				if *pr.Mergeable != false {
					t.Errorf("Mergeable = %v, expected false", *pr.Mergeable)
				}
			},
		},
		{
			name: "sets mergeable to false for dirty state",
			pr: &PullRequest{
				MergeableState: "dirty",
			},
			events: []Event{},
			validate: func(t *testing.T, pr *PullRequest) {
				if pr.Mergeable == nil {
					t.Fatal("Mergeable is nil")
				}
				if *pr.Mergeable != false {
					t.Errorf("Mergeable = %v, expected false", *pr.Mergeable)
				}
			},
		},
		{
			name: "sets mergeable to false for unstable state",
			pr: &PullRequest{
				MergeableState: "unstable",
			},
			events: []Event{},
			validate: func(t *testing.T, pr *PullRequest) {
				if pr.Mergeable == nil {
					t.Fatal("Mergeable is nil")
				}
				if *pr.Mergeable != false {
					t.Errorf("Mergeable = %v, expected false", *pr.Mergeable)
				}
			},
		},
		{
			name: "sets mergeable description",
			pr: &PullRequest{
				MergeableState: "clean",
			},
			events: []Event{},
			validate: func(t *testing.T, pr *PullRequest) {
				if pr.MergeableStateDescription == "" {
					t.Error("MergeableStateDescription is empty")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			FinalizePullRequest(tt.pr, tt.events, tt.requiredChecks, tt.testStateFromAPI)
			tt.validate(t, tt.pr)
		})
	}
}

func TestFixTestState(t *testing.T) {
	tests := []struct {
		name     string
		pr       *PullRequest
		expected string
	}{
		{
			name: "sets failing when checks fail",
			pr: &PullRequest{
				CheckSummary: &CheckSummary{
					Failing: map[string]string{"test": "failed"},
				},
			},
			expected: TestStateFailing,
		},
		{
			name: "sets failing when checks cancelled",
			pr: &PullRequest{
				CheckSummary: &CheckSummary{
					Cancelled: map[string]string{"test": "cancelled"},
				},
			},
			expected: TestStateFailing,
		},
		{
			name: "sets pending when checks pending",
			pr: &PullRequest{
				CheckSummary: &CheckSummary{
					Pending: map[string]string{"test": "pending"},
				},
			},
			expected: TestStatePending,
		},
		{
			name: "sets passing when checks succeed",
			pr: &PullRequest{
				CheckSummary: &CheckSummary{
					Success: map[string]string{"test": "passed"},
				},
			},
			expected: TestStatePassing,
		},
		{
			name: "prefers failing over pending",
			pr: &PullRequest{
				CheckSummary: &CheckSummary{
					Failing: map[string]string{"test1": "failed"},
					Pending: map[string]string{"test2": "pending"},
				},
			},
			expected: TestStateFailing,
		},
		{
			name: "prefers pending over passing",
			pr: &PullRequest{
				CheckSummary: &CheckSummary{
					Success: map[string]string{"test1": "passed"},
					Pending: map[string]string{"test2": "pending"},
				},
			},
			expected: TestStatePending,
		},
		{
			name: "preserves existing test state when no checks",
			pr: &PullRequest{
				TestState:    TestStatePassing,
				CheckSummary: &CheckSummary{},
			},
			expected: TestStatePassing,
		},
		{
			name: "sets none when no checks and no existing state",
			pr: &PullRequest{
				CheckSummary: &CheckSummary{},
			},
			expected: TestStateNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			FixTestState(tt.pr)
			if tt.pr.TestState != tt.expected {
				t.Errorf("TestState = %q, expected %q", tt.pr.TestState, tt.expected)
			}
		})
	}
}

func TestSetMergeableDescription(t *testing.T) {
	tests := []struct {
		name                 string
		mergeableState       string
		checkSummary         *CheckSummary
		approvalSummary      *ApprovalSummary
		expectedDescription  string
		descriptionShouldBe  string // for more flexible matching
		shouldNotBeEmpty     bool
	}{
		{
			name:                "clean state",
			mergeableState:      "clean",
			expectedDescription: "PR is ready to merge",
		},
		{
			name:                "dirty state",
			mergeableState:      "dirty",
			expectedDescription: "PR has merge conflicts that need to be resolved",
		},
		{
			name:                "unstable state",
			mergeableState:      "unstable",
			expectedDescription: "PR is mergeable but status checks are failing",
		},
		{
			name:                "unknown state",
			mergeableState:      "unknown",
			expectedDescription: "Merge status is being calculated",
		},
		{
			name:                "draft state",
			mergeableState:      "draft",
			expectedDescription: "PR is in draft state",
		},
		{
			name:             "blocked state triggers SetBlockedDescription",
			mergeableState:   "blocked",
			checkSummary:     &CheckSummary{},
			approvalSummary:  &ApprovalSummary{},
			shouldNotBeEmpty: true,
		},
		{
			name:                 "unknown/unhandled state",
			mergeableState:       "some_other_state",
			expectedDescription:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &PullRequest{
				MergeableState:  tt.mergeableState,
				CheckSummary:    tt.checkSummary,
				ApprovalSummary: tt.approvalSummary,
			}
			if pr.CheckSummary == nil {
				pr.CheckSummary = &CheckSummary{}
			}
			if pr.ApprovalSummary == nil {
				pr.ApprovalSummary = &ApprovalSummary{}
			}

			SetMergeableDescription(pr)

			if tt.shouldNotBeEmpty {
				if pr.MergeableStateDescription == "" {
					t.Error("MergeableStateDescription should not be empty")
				}
			} else if tt.expectedDescription != "" {
				if pr.MergeableStateDescription != tt.expectedDescription {
					t.Errorf("MergeableStateDescription = %q, expected %q", pr.MergeableStateDescription, tt.expectedDescription)
				}
			}
		})
	}
}

func TestSetBlockedDescription(t *testing.T) {
	tests := []struct {
		name                string
		approvalSummary     *ApprovalSummary
		checkSummary        *CheckSummary
		expectedDescription string
	}{
		{
			name: "requires approval only",
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 0,
			},
			checkSummary: &CheckSummary{
				Failing: map[string]string{},
				Pending: map[string]string{},
			},
			expectedDescription: "PR requires approval",
		},
		{
			name: "requires approval with pending checks",
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 0,
			},
			checkSummary: &CheckSummary{
				Failing: map[string]string{},
				Pending: map[string]string{"test": "pending"},
			},
			expectedDescription: "PR requires approval and has pending status checks",
		},
		{
			name: "has failing checks without approval",
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 0,
			},
			checkSummary: &CheckSummary{
				Failing: map[string]string{"test": "failed"},
			},
			expectedDescription: "PR has failing status checks and requires approval",
		},
		{
			name: "has failing checks with approval",
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 1,
			},
			checkSummary: &CheckSummary{
				Failing: map[string]string{"test": "failed"},
			},
			expectedDescription: "PR is blocked by failing status checks",
		},
		{
			name: "has pending checks with approval",
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 1,
			},
			checkSummary: &CheckSummary{
				Pending: map[string]string{"test": "pending"},
				Failing: map[string]string{},
			},
			expectedDescription: "PR is blocked by pending status checks",
		},
		{
			name: "has cancelled checks (treated as failing)",
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 0,
			},
			checkSummary: &CheckSummary{
				Cancelled: map[string]string{"test": "cancelled"},
			},
			expectedDescription: "PR has failing status checks and requires approval",
		},
		{
			name: "generic blocked message",
			approvalSummary: &ApprovalSummary{
				ApprovalsWithWriteAccess: 1,
			},
			checkSummary: &CheckSummary{
				Failing: map[string]string{},
				Pending: map[string]string{},
			},
			expectedDescription: "PR is blocked by required status checks, reviews, or branch protection rules",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &PullRequest{
				ApprovalSummary: tt.approvalSummary,
				CheckSummary:    tt.checkSummary,
			}

			SetBlockedDescription(pr)

			if pr.MergeableStateDescription != tt.expectedDescription {
				t.Errorf("MergeableStateDescription = %q, expected %q", pr.MergeableStateDescription, tt.expectedDescription)
			}
		})
	}
}

func TestReviewStateConstants(t *testing.T) {
	// Test that constants are properly defined
	tests := []struct {
		name  string
		state ReviewState
		value string
	}{
		{"pending", ReviewStatePending, "pending"},
		{"approved", ReviewStateApproved, "approved"},
		{"changes_requested", ReviewStateChangesRequested, "changes_requested"},
		{"commented", ReviewStateCommented, "commented"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.state) != tt.value {
				t.Errorf("ReviewState %q = %q, expected %q", tt.name, tt.state, tt.value)
			}
		})
	}
}

func TestTestStateConstants(t *testing.T) {
	// Test that constants are properly defined
	tests := []struct {
		name  string
		state string
		value string
	}{
		{"none", TestStateNone, ""},
		{"queued", TestStateQueued, "queued"},
		{"running", TestStateRunning, "running"},
		{"passing", TestStatePassing, "passing"},
		{"failing", TestStateFailing, "failing"},
		{"pending", TestStatePending, "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.state != tt.value {
				t.Errorf("TestState %q = %q, expected %q", tt.name, tt.state, tt.value)
			}
		})
	}
}

func TestWriteAccessConstants(t *testing.T) {
	// Test that constants have expected values
	tests := []struct {
		name  string
		value int
		want  int
	}{
		{"WriteAccessNo", WriteAccessNo, -2},
		{"WriteAccessUnlikely", WriteAccessUnlikely, -1},
		{"WriteAccessNA", WriteAccessNA, 0},
		{"WriteAccessLikely", WriteAccessLikely, 1},
		{"WriteAccessDefinitely", WriteAccessDefinitely, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want {
				t.Errorf("%s = %d, expected %d", tt.name, tt.value, tt.want)
			}
		})
	}
}

func TestEventKindConstants(t *testing.T) {
	// Just verify some key event kinds are defined correctly
	tests := []struct {
		name  string
		kind  string
		value string
	}{
		{"commit", EventKindCommit, "commit"},
		{"comment", EventKindComment, "comment"},
		{"review", EventKindReview, "review"},
		{"status_check", EventKindStatusCheck, "status_check"},
		{"check_run", EventKindCheckRun, "check_run"},
		{"labeled", EventKindLabeled, "labeled"},
		{"pr_merged", EventKindPRMerged, "pr_merged"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.kind != tt.value {
				t.Errorf("EventKind %q = %q, expected %q", tt.name, tt.kind, tt.value)
			}
		})
	}
}

func TestPlatformConstants(t *testing.T) {
	// Test platform constants from url.go (but they're used by types)
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"GitHub", PlatformGitHub, "github"},
		{"GitLab", PlatformGitLab, "gitlab"},
		{"Codeberg", PlatformCodeberg, "codeberg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want {
				t.Errorf("%s = %q, expected %q", tt.name, tt.value, tt.want)
			}
		})
	}
}
