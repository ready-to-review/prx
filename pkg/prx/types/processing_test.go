package types

import (
	"testing"
	"time"
)

func TestFilterEvents(t *testing.T) {
	tests := []struct {
		name     string
		events   []Event
		expected int // expected number of events after filtering
	}{
		{
			name: "keeps non-status_check events",
			events: []Event{
				{Kind: EventKindCommit, Actor: "user1"},
				{Kind: EventKindComment, Actor: "user2"},
				{Kind: EventKindReview, Actor: "user3"},
			},
			expected: 3,
		},
		{
			name: "filters successful status checks",
			events: []Event{
				{Kind: EventKindStatusCheck, Outcome: "success", Body: "check1"},
				{Kind: EventKindComment, Actor: "user1"},
			},
			expected: 1,
		},
		{
			name: "keeps failed status checks",
			events: []Event{
				{Kind: EventKindStatusCheck, Outcome: "failure", Body: "check1"},
				{Kind: EventKindStatusCheck, Outcome: "success", Body: "check2"},
				{Kind: EventKindComment, Actor: "user1"},
			},
			expected: 2, // failure + comment
		},
		{
			name:     "empty events",
			events:   []Event{},
			expected: 0,
		},
		{
			name: "mixed events",
			events: []Event{
				{Kind: EventKindStatusCheck, Outcome: "success", Body: "check1"},
				{Kind: EventKindStatusCheck, Outcome: "failure", Body: "check2"},
				{Kind: EventKindCheckRun, Outcome: "success", Body: "check3"},
				{Kind: EventKindCommit, Actor: "user1"},
			},
			expected: 3, // failure status + check_run + commit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterEvents(tt.events)
			if len(result) != tt.expected {
				t.Errorf("FilterEvents() returned %d events, expected %d", len(result), tt.expected)
			}
		})
	}
}

func TestUpgradeWriteAccess(t *testing.T) {
	tests := []struct {
		name     string
		events   []Event
		validate func(t *testing.T, events []Event)
	}{
		{
			name: "upgrades actors who merge PRs",
			events: []Event{
				{Kind: EventKindReview, Actor: "reviewer1", WriteAccess: WriteAccessLikely},
				{Kind: EventKindPRMerged, Actor: "reviewer1"},
				{Kind: EventKindComment, Actor: "reviewer1", WriteAccess: WriteAccessLikely},
			},
			validate: func(t *testing.T, events []Event) {
				// First event should be upgraded
				if events[0].WriteAccess != WriteAccessDefinitely {
					t.Errorf("Event 0: WriteAccess = %d, expected %d", events[0].WriteAccess, WriteAccessDefinitely)
				}
				// Third event should be upgraded
				if events[2].WriteAccess != WriteAccessDefinitely {
					t.Errorf("Event 2: WriteAccess = %d, expected %d", events[2].WriteAccess, WriteAccessDefinitely)
				}
			},
		},
		{
			name: "upgrades actors who add labels",
			events: []Event{
				{Kind: EventKindReview, Actor: "user1", WriteAccess: WriteAccessLikely},
				{Kind: EventKindLabeled, Actor: "user1"},
			},
			validate: func(t *testing.T, events []Event) {
				if events[0].WriteAccess != WriteAccessDefinitely {
					t.Errorf("Event 0: WriteAccess = %d, expected %d", events[0].WriteAccess, WriteAccessDefinitely)
				}
			},
		},
		{
			name: "upgrades actors who remove labels",
			events: []Event{
				{Kind: EventKindComment, Actor: "user1", WriteAccess: WriteAccessLikely},
				{Kind: EventKindUnlabeled, Actor: "user1"},
			},
			validate: func(t *testing.T, events []Event) {
				if events[0].WriteAccess != WriteAccessDefinitely {
					t.Errorf("Event 0: WriteAccess = %d, expected %d", events[0].WriteAccess, WriteAccessDefinitely)
				}
			},
		},
		{
			name: "upgrades actors who assign users",
			events: []Event{
				{Kind: EventKindReview, Actor: "user1", WriteAccess: WriteAccessLikely},
				{Kind: EventKindAssigned, Actor: "user1"},
			},
			validate: func(t *testing.T, events []Event) {
				if events[0].WriteAccess != WriteAccessDefinitely {
					t.Errorf("Event 0: WriteAccess = %d, expected %d", events[0].WriteAccess, WriteAccessDefinitely)
				}
			},
		},
		{
			name: "does not upgrade non-likely write access",
			events: []Event{
				{Kind: EventKindReview, Actor: "user1", WriteAccess: WriteAccessNo},
				{Kind: EventKindLabeled, Actor: "user1"},
			},
			validate: func(t *testing.T, events []Event) {
				// Should not upgrade from No to Definitely
				if events[0].WriteAccess != WriteAccessNo {
					t.Errorf("Event 0: WriteAccess = %d, expected %d (should not upgrade)", events[0].WriteAccess, WriteAccessNo)
				}
			},
		},
		{
			name: "handles empty actor names",
			events: []Event{
				{Kind: EventKindLabeled, Actor: ""},
				{Kind: EventKindComment, Actor: "user1", WriteAccess: WriteAccessLikely},
			},
			validate: func(t *testing.T, events []Event) {
				// Should not panic and should leave events unchanged
				if events[1].WriteAccess != WriteAccessLikely {
					t.Errorf("Event 1: WriteAccess = %d, expected %d", events[1].WriteAccess, WriteAccessLikely)
				}
			},
		},
		{
			name:   "handles empty events slice",
			events: []Event{},
			validate: func(t *testing.T, events []Event) {
				// Should not panic
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			UpgradeWriteAccess(tt.events)
			tt.validate(t, tt.events)
		})
	}
}

func TestCalculateCheckSummary(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)

	tests := []struct {
		name           string
		events         []Event
		requiredChecks []string
		validate       func(t *testing.T, summary *CheckSummary)
	}{
		{
			name: "categorizes successful checks",
			events: []Event{
				{Kind: EventKindCheckRun, Body: "test", Outcome: "success", Timestamp: now},
			},
			validate: func(t *testing.T, summary *CheckSummary) {
				if len(summary.Success) != 1 {
					t.Errorf("Success count = %d, expected 1", len(summary.Success))
				}
				if _, ok := summary.Success["test"]; !ok {
					t.Error("Expected 'test' in Success")
				}
			},
		},
		{
			name: "categorizes failing checks",
			events: []Event{
				{Kind: EventKindCheckRun, Body: "test", Outcome: "failure", Timestamp: now},
			},
			validate: func(t *testing.T, summary *CheckSummary) {
				if len(summary.Failing) != 1 {
					t.Errorf("Failing count = %d, expected 1", len(summary.Failing))
				}
			},
		},
		{
			name: "categorizes error as failing",
			events: []Event{
				{Kind: EventKindCheckRun, Body: "test", Outcome: "error", Timestamp: now},
			},
			validate: func(t *testing.T, summary *CheckSummary) {
				if len(summary.Failing) != 1 {
					t.Errorf("Failing count = %d, expected 1", len(summary.Failing))
				}
			},
		},
		{
			name: "categorizes cancelled checks",
			events: []Event{
				{Kind: EventKindCheckRun, Body: "test", Outcome: "cancelled", Timestamp: now},
			},
			validate: func(t *testing.T, summary *CheckSummary) {
				if len(summary.Cancelled) != 1 {
					t.Errorf("Cancelled count = %d, expected 1", len(summary.Cancelled))
				}
			},
		},
		{
			name: "categorizes pending checks",
			events: []Event{
				{Kind: EventKindCheckRun, Body: "test1", Outcome: "pending", Timestamp: now},
				{Kind: EventKindCheckRun, Body: "test2", Outcome: "queued", Timestamp: now},
				{Kind: EventKindCheckRun, Body: "test3", Outcome: "in_progress", Timestamp: now},
			},
			validate: func(t *testing.T, summary *CheckSummary) {
				if len(summary.Pending) != 3 {
					t.Errorf("Pending count = %d, expected 3", len(summary.Pending))
				}
			},
		},
		{
			name: "uses latest status for duplicate checks",
			events: []Event{
				{Kind: EventKindCheckRun, Body: "test", Outcome: "pending", Timestamp: earlier},
				{Kind: EventKindCheckRun, Body: "test", Outcome: "success", Timestamp: now},
			},
			validate: func(t *testing.T, summary *CheckSummary) {
				if len(summary.Success) != 1 {
					t.Errorf("Success count = %d, expected 1", len(summary.Success))
				}
				if len(summary.Pending) != 0 {
					t.Errorf("Pending count = %d, expected 0", len(summary.Pending))
				}
			},
		},
		{
			name: "adds missing required checks as pending",
			events: []Event{
				{Kind: EventKindCheckRun, Body: "test1", Outcome: "success", Timestamp: now},
			},
			requiredChecks: []string{"test1", "test2"},
			validate: func(t *testing.T, summary *CheckSummary) {
				if len(summary.Pending) != 1 {
					t.Errorf("Pending count = %d, expected 1", len(summary.Pending))
				}
				if _, ok := summary.Pending["test2"]; !ok {
					t.Error("Expected 'test2' in Pending")
				}
			},
		},
		{
			name: "ignores checks with no name",
			events: []Event{
				{Kind: EventKindCheckRun, Body: "", Outcome: "success", Timestamp: now},
				{Kind: EventKindCheckRun, Body: "test", Outcome: "success", Timestamp: now},
			},
			validate: func(t *testing.T, summary *CheckSummary) {
				if len(summary.Success) != 1 {
					t.Errorf("Success count = %d, expected 1", len(summary.Success))
				}
			},
		},
		{
			name: "categorizes skipped checks",
			events: []Event{
				{Kind: EventKindCheckRun, Body: "test", Outcome: "skipped", Timestamp: now},
			},
			validate: func(t *testing.T, summary *CheckSummary) {
				if len(summary.Skipped) != 1 {
					t.Errorf("Skipped count = %d, expected 1", len(summary.Skipped))
				}
			},
		},
		{
			name: "categorizes stale checks",
			events: []Event{
				{Kind: EventKindCheckRun, Body: "test", Outcome: "stale", Timestamp: now},
			},
			validate: func(t *testing.T, summary *CheckSummary) {
				if len(summary.Stale) != 1 {
					t.Errorf("Stale count = %d, expected 1", len(summary.Stale))
				}
			},
		},
		{
			name: "categorizes neutral checks",
			events: []Event{
				{Kind: EventKindCheckRun, Body: "test", Outcome: "neutral", Timestamp: now},
			},
			validate: func(t *testing.T, summary *CheckSummary) {
				if len(summary.Neutral) != 1 {
					t.Errorf("Neutral count = %d, expected 1", len(summary.Neutral))
				}
			},
		},
		{
			name: "handles both status_check and check_run events",
			events: []Event{
				{Kind: EventKindStatusCheck, Body: "check1", Outcome: "success", Timestamp: now},
				{Kind: EventKindCheckRun, Body: "check2", Outcome: "failure", Timestamp: now},
			},
			validate: func(t *testing.T, summary *CheckSummary) {
				if len(summary.Success) != 1 {
					t.Errorf("Success count = %d, expected 1", len(summary.Success))
				}
				if len(summary.Failing) != 1 {
					t.Errorf("Failing count = %d, expected 1", len(summary.Failing))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateCheckSummary(tt.events, tt.requiredChecks)
			tt.validate(t, result)
		})
	}
}

func TestCalculateApprovalSummary(t *testing.T) {
	tests := []struct {
		name     string
		events   []Event
		validate func(t *testing.T, summary *ApprovalSummary)
	}{
		{
			name: "counts approvals with write access",
			events: []Event{
				{Kind: EventKindReview, Actor: "user1", Outcome: "approved", WriteAccess: WriteAccessDefinitely},
			},
			validate: func(t *testing.T, summary *ApprovalSummary) {
				if summary.ApprovalsWithWriteAccess != 1 {
					t.Errorf("ApprovalsWithWriteAccess = %d, expected 1", summary.ApprovalsWithWriteAccess)
				}
			},
		},
		{
			name: "counts approvals without write access",
			events: []Event{
				{Kind: EventKindReview, Actor: "user1", Outcome: "approved", WriteAccess: WriteAccessNo},
			},
			validate: func(t *testing.T, summary *ApprovalSummary) {
				if summary.ApprovalsWithoutWriteAccess != 1 {
					t.Errorf("ApprovalsWithoutWriteAccess = %d, expected 1", summary.ApprovalsWithoutWriteAccess)
				}
			},
		},
		{
			name: "counts approvals with unknown write access",
			events: []Event{
				{Kind: EventKindReview, Actor: "user1", Outcome: "approved", WriteAccess: WriteAccessLikely},
				{Kind: EventKindReview, Actor: "user2", Outcome: "approved", WriteAccess: WriteAccessUnlikely},
				{Kind: EventKindReview, Actor: "user3", Outcome: "approved", WriteAccess: WriteAccessNA},
			},
			validate: func(t *testing.T, summary *ApprovalSummary) {
				if summary.ApprovalsWithUnknownAccess != 3 {
					t.Errorf("ApprovalsWithUnknownAccess = %d, expected 3", summary.ApprovalsWithUnknownAccess)
				}
			},
		},
		{
			name: "counts change requests",
			events: []Event{
				{Kind: EventKindReview, Actor: "user1", Outcome: "changes_requested", WriteAccess: WriteAccessDefinitely},
			},
			validate: func(t *testing.T, summary *ApprovalSummary) {
				if summary.ChangesRequested != 1 {
					t.Errorf("ChangesRequested = %d, expected 1", summary.ChangesRequested)
				}
			},
		},
		{
			name: "uses latest review from each user",
			events: []Event{
				{Kind: EventKindReview, Actor: "user1", Outcome: "approved", WriteAccess: WriteAccessDefinitely},
				{Kind: EventKindReview, Actor: "user1", Outcome: "changes_requested", WriteAccess: WriteAccessDefinitely},
			},
			validate: func(t *testing.T, summary *ApprovalSummary) {
				if summary.ChangesRequested != 1 {
					t.Errorf("ChangesRequested = %d, expected 1", summary.ChangesRequested)
				}
				if summary.ApprovalsWithWriteAccess != 0 {
					t.Errorf("ApprovalsWithWriteAccess = %d, expected 0 (latest review should override)", summary.ApprovalsWithWriteAccess)
				}
			},
		},
		{
			name: "ignores commented reviews",
			events: []Event{
				{Kind: EventKindReview, Actor: "user1", Outcome: "commented", WriteAccess: WriteAccessDefinitely},
			},
			validate: func(t *testing.T, summary *ApprovalSummary) {
				if summary.ApprovalsWithWriteAccess != 0 {
					t.Errorf("ApprovalsWithWriteAccess = %d, expected 0", summary.ApprovalsWithWriteAccess)
				}
				if summary.ChangesRequested != 0 {
					t.Errorf("ChangesRequested = %d, expected 0", summary.ChangesRequested)
				}
			},
		},
		{
			name: "ignores non-review events",
			events: []Event{
				{Kind: EventKindComment, Actor: "user1"},
				{Kind: EventKindCommit, Actor: "user2"},
			},
			validate: func(t *testing.T, summary *ApprovalSummary) {
				if summary.ApprovalsWithWriteAccess != 0 {
					t.Errorf("ApprovalsWithWriteAccess = %d, expected 0", summary.ApprovalsWithWriteAccess)
				}
			},
		},
		{
			name: "handles multiple reviewers",
			events: []Event{
				{Kind: EventKindReview, Actor: "user1", Outcome: "approved", WriteAccess: WriteAccessDefinitely},
				{Kind: EventKindReview, Actor: "user2", Outcome: "approved", WriteAccess: WriteAccessDefinitely},
				{Kind: EventKindReview, Actor: "user3", Outcome: "changes_requested", WriteAccess: WriteAccessNo},
			},
			validate: func(t *testing.T, summary *ApprovalSummary) {
				if summary.ApprovalsWithWriteAccess != 2 {
					t.Errorf("ApprovalsWithWriteAccess = %d, expected 2", summary.ApprovalsWithWriteAccess)
				}
				if summary.ChangesRequested != 1 {
					t.Errorf("ChangesRequested = %d, expected 1", summary.ChangesRequested)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateApprovalSummary(tt.events)
			tt.validate(t, result)
		})
	}
}

func TestCalculateParticipantAccess(t *testing.T) {
	tests := []struct {
		name     string
		events   []Event
		pr       *PullRequest
		validate func(t *testing.T, participants map[string]int)
	}{
		{
			name:   "includes PR author",
			events: []Event{},
			pr: &PullRequest{
				Author:            "author1",
				AuthorWriteAccess: WriteAccessDefinitely,
			},
			validate: func(t *testing.T, participants map[string]int) {
				if participants["author1"] != WriteAccessDefinitely {
					t.Errorf("author1 access = %d, expected %d", participants["author1"], WriteAccessDefinitely)
				}
			},
		},
		{
			name:   "includes assignees",
			events: []Event{},
			pr: &PullRequest{
				Author:    "author1",
				Assignees: []string{"assignee1", "assignee2"},
			},
			validate: func(t *testing.T, participants map[string]int) {
				if participants["assignee1"] != WriteAccessNA {
					t.Errorf("assignee1 access = %d, expected %d", participants["assignee1"], WriteAccessNA)
				}
				if participants["assignee2"] != WriteAccessNA {
					t.Errorf("assignee2 access = %d, expected %d", participants["assignee2"], WriteAccessNA)
				}
			},
		},
		{
			name:   "includes reviewers",
			events: []Event{},
			pr: &PullRequest{
				Author: "author1",
				Reviewers: map[string]ReviewState{
					"reviewer1": ReviewStateApproved,
					"reviewer2": ReviewStatePending,
				},
			},
			validate: func(t *testing.T, participants map[string]int) {
				if participants["reviewer1"] != WriteAccessNA {
					t.Errorf("reviewer1 access = %d, expected %d", participants["reviewer1"], WriteAccessNA)
				}
				if participants["reviewer2"] != WriteAccessNA {
					t.Errorf("reviewer2 access = %d, expected %d", participants["reviewer2"], WriteAccessNA)
				}
			},
		},
		{
			name: "includes event actors",
			events: []Event{
				{Actor: "actor1", WriteAccess: WriteAccessDefinitely},
				{Actor: "actor2", WriteAccess: WriteAccessNo},
			},
			pr: &PullRequest{Author: "author1"},
			validate: func(t *testing.T, participants map[string]int) {
				if participants["actor1"] != WriteAccessDefinitely {
					t.Errorf("actor1 access = %d, expected %d", participants["actor1"], WriteAccessDefinitely)
				}
				if participants["actor2"] != WriteAccessNo {
					t.Errorf("actor2 access = %d, expected %d", participants["actor2"], WriteAccessNo)
				}
			},
		},
		{
			name: "upgrades write access to highest level",
			events: []Event{
				{Actor: "user1", WriteAccess: WriteAccessLikely},
				{Actor: "user1", WriteAccess: WriteAccessDefinitely},
			},
			pr: &PullRequest{Author: "author1"},
			validate: func(t *testing.T, participants map[string]int) {
				if participants["user1"] != WriteAccessDefinitely {
					t.Errorf("user1 access = %d, expected %d (should upgrade to highest)", participants["user1"], WriteAccessDefinitely)
				}
			},
		},
		{
			name: "handles empty actor names",
			events: []Event{
				{Actor: "", WriteAccess: WriteAccessDefinitely},
				{Actor: "actor1", WriteAccess: WriteAccessNo},
			},
			pr: &PullRequest{Author: "author1"},
			validate: func(t *testing.T, participants map[string]int) {
				if _, ok := participants[""]; ok {
					t.Error("Empty actor should not be in participants")
				}
				if participants["actor1"] != WriteAccessNo {
					t.Errorf("actor1 access = %d, expected %d", participants["actor1"], WriteAccessNo)
				}
			},
		},
		{
			name: "handles empty assignee names",
			events: []Event{},
			pr: &PullRequest{
				Author:    "author1",
				Assignees: []string{"", "assignee1"},
			},
			validate: func(t *testing.T, participants map[string]int) {
				if _, ok := participants[""]; ok {
					t.Error("Empty assignee should not be in participants")
				}
				if participants["assignee1"] != WriteAccessNA {
					t.Errorf("assignee1 access = %d, expected %d", participants["assignee1"], WriteAccessNA)
				}
			},
		},
		{
			name: "does not downgrade author's write access",
			events: []Event{
				{Actor: "author1", WriteAccess: WriteAccessNo},
			},
			pr: &PullRequest{
				Author:            "author1",
				AuthorWriteAccess: WriteAccessDefinitely,
			},
			validate: func(t *testing.T, participants map[string]int) {
				if participants["author1"] != WriteAccessDefinitely {
					t.Errorf("author1 access = %d, expected %d (should keep author's higher access)", participants["author1"], WriteAccessDefinitely)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateParticipantAccess(tt.events, tt.pr)
			tt.validate(t, result)
		})
	}
}
