package prx

import (
	"testing"
	"time"

	"github.com/codeGROOVE-dev/prx/pkg/prx/types"
)

func TestFinalizePullRequest(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name             string
		pr               types.PullRequest
		events           []types.Event
		requiredChecks   []string
		testStateFromAPI string
		wantTestState    string
		wantMergeable    *bool
		wantDescContains string
	}{
		{
			name: "blocked pr without approvals",
			pr: types.PullRequest{
				Number:         1,
				MergeableState: "blocked",
			},
			events:           []types.Event{},
			requiredChecks:   []string{},
			testStateFromAPI: "",
			wantTestState:    types.TestStateNone,
			wantMergeable:    boolPtr(false),
			wantDescContains: "requires approval",
		},
		{
			name: "clean pr ready to merge",
			pr: types.PullRequest{
				Number:         1,
				MergeableState: "clean",
			},
			events: []types.Event{
				{
					Kind:        "review",
					Actor:       "reviewer",
					Timestamp:   now,
					Outcome:     "APPROVED",
					WriteAccess: types.WriteAccessDefinitely,
				},
				{
					Kind:      "status_check",
					Timestamp: now,
					Body:      "test",
					Outcome:   "success",
				},
			},
			requiredChecks:   []string{"test"},
			testStateFromAPI: "passing",
			wantTestState:    types.TestStatePassing,
			wantDescContains: "ready to merge",
		},
		{
			name: "unstable pr with failing checks",
			pr: types.PullRequest{
				Number:         1,
				MergeableState: "unstable",
			},
			events: []types.Event{
				{
					Kind:      "status_check",
					Timestamp: now,
					Body:      "test",
					Outcome:   "failure",
				},
			},
			requiredChecks:   []string{"test"},
			testStateFromAPI: "failing",
			wantTestState:    types.TestStateFailing,
			wantDescContains: "status checks are failing",
		},
		{
			name: "dirty pr with merge conflicts",
			pr: types.PullRequest{
				Number:         1,
				MergeableState: "dirty",
			},
			events:           []types.Event{},
			requiredChecks:   []string{},
			testStateFromAPI: "",
			wantTestState:    types.TestStateNone,
			wantMergeable:    boolPtr(false),
			wantDescContains: "merge conflicts",
		},
		{
			name: "draft pr",
			pr: types.PullRequest{
				Number:         1,
				MergeableState: "draft",
				Draft:          true,
			},
			events:           []types.Event{},
			requiredChecks:   []string{},
			testStateFromAPI: "",
			wantTestState:    types.TestStateNone,
			wantDescContains: "draft state",
		},
		{
			name: "unknown mergeable state",
			pr: types.PullRequest{
				Number:         1,
				MergeableState: "unknown",
			},
			events:           []types.Event{},
			requiredChecks:   []string{},
			testStateFromAPI: "",
			wantTestState:    types.TestStateNone,
			wantDescContains: "being calculated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			types.FinalizePullRequest(&tt.pr, tt.events, tt.requiredChecks, tt.testStateFromAPI)

			if tt.pr.TestState != tt.wantTestState {
				t.Errorf("TestState = %v, want %v", tt.pr.TestState, tt.wantTestState)
			}

			if tt.wantMergeable != nil {
				if tt.pr.Mergeable == nil {
					t.Errorf("Mergeable is nil, want %v", *tt.wantMergeable)
				} else if *tt.pr.Mergeable != *tt.wantMergeable {
					t.Errorf("Mergeable = %v, want %v", *tt.pr.Mergeable, *tt.wantMergeable)
				}
			}

			if tt.wantDescContains != "" {
				if tt.pr.MergeableStateDescription == "" {
					t.Errorf("MergeableStateDescription is empty, want to contain %q", tt.wantDescContains)
				} else if !contains(tt.pr.MergeableStateDescription, tt.wantDescContains) {
					t.Errorf("MergeableStateDescription = %q, want to contain %q",
						tt.pr.MergeableStateDescription, tt.wantDescContains)
				}
			}
		})
	}
}

func TestFixTestState(t *testing.T) {
	tests := []struct {
		name          string
		checkSummary  *types.CheckSummary
		wantTestState string
	}{
		{
			name: "failing checks",
			checkSummary: &types.CheckSummary{
				Failing: map[string]string{"test1": "failed"},
				Success: map[string]string{"test2": "passed"},
			},
			wantTestState: types.TestStateFailing,
		},
		{
			name: "cancelled checks",
			checkSummary: &types.CheckSummary{
				Cancelled: map[string]string{"test1": "cancelled"},
			},
			wantTestState: types.TestStateFailing,
		},
		{
			name: "pending checks",
			checkSummary: &types.CheckSummary{
				Pending: map[string]string{"test1": "pending"},
				Success: map[string]string{"test2": "passed"},
			},
			wantTestState: types.TestStatePending,
		},
		{
			name: "only success checks",
			checkSummary: &types.CheckSummary{
				Success: map[string]string{"test1": "passed", "test2": "passed"},
			},
			wantTestState: types.TestStatePassing,
		},
		{
			name: "no checks",
			checkSummary: &types.CheckSummary{
				Success: map[string]string{},
			},
			wantTestState: types.TestStateNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &types.PullRequest{
				CheckSummary: tt.checkSummary,
			}
			types.FixTestState(pr)
			if pr.TestState != tt.wantTestState {
				t.Errorf("TestState = %v, want %v", pr.TestState, tt.wantTestState)
			}
		})
	}
}

func TestSetMergeableDescription(t *testing.T) {
	tests := []struct {
		name             string
		mergeableState   string
		checkSummary     *types.CheckSummary
		approvalSummary  *types.ApprovalSummary
		wantDescContains string
	}{
		{
			name:           "blocked state without approvals",
			mergeableState: "blocked",
			checkSummary: &types.CheckSummary{
				Failing: map[string]string{},
				Pending: map[string]string{},
			},
			approvalSummary: &types.ApprovalSummary{
				ApprovalsWithWriteAccess: 0,
			},
			wantDescContains: "requires approval",
		},
		{
			name:             "dirty state",
			mergeableState:   "dirty",
			wantDescContains: "merge conflicts",
		},
		{
			name:             "unstable state",
			mergeableState:   "unstable",
			wantDescContains: "failing",
		},
		{
			name:             "clean state",
			mergeableState:   "clean",
			wantDescContains: "ready to merge",
		},
		{
			name:             "unknown state",
			mergeableState:   "unknown",
			wantDescContains: "being calculated",
		},
		{
			name:             "draft state",
			mergeableState:   "draft",
			wantDescContains: "draft",
		},
		{
			name:             "unknown mergeable state value",
			mergeableState:   "some_other_value",
			wantDescContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &types.PullRequest{
				MergeableState:  tt.mergeableState,
				CheckSummary:    tt.checkSummary,
				ApprovalSummary: tt.approvalSummary,
			}
			if pr.CheckSummary == nil {
				pr.CheckSummary = &types.CheckSummary{}
			}
			if pr.ApprovalSummary == nil {
				pr.ApprovalSummary = &types.ApprovalSummary{}
			}
			types.SetMergeableDescription(pr)
			if tt.wantDescContains == "" {
				if pr.MergeableStateDescription != "" {
					t.Errorf("Expected empty description for unknown state, got %q", pr.MergeableStateDescription)
				}
			} else if !contains(pr.MergeableStateDescription, tt.wantDescContains) {
				t.Errorf("MergeableStateDescription = %q, want to contain %q",
					pr.MergeableStateDescription, tt.wantDescContains)
			}
		})
	}
}

func TestSetBlockedDescription(t *testing.T) {
	tests := []struct {
		name             string
		approvalSummary  *types.ApprovalSummary
		checkSummary     *types.CheckSummary
		wantDescContains string
	}{
		{
			name: "no approvals, no checks",
			approvalSummary: &types.ApprovalSummary{
				ApprovalsWithWriteAccess: 0,
			},
			checkSummary: &types.CheckSummary{
				Failing: map[string]string{},
				Pending: map[string]string{},
			},
			wantDescContains: "requires approval",
		},
		{
			name: "no approvals with pending checks",
			approvalSummary: &types.ApprovalSummary{
				ApprovalsWithWriteAccess: 0,
			},
			checkSummary: &types.CheckSummary{
				Failing: map[string]string{},
				Pending: map[string]string{"test": "pending"},
			},
			wantDescContains: "requires approval and has pending",
		},
		{
			name: "failing checks without approval",
			approvalSummary: &types.ApprovalSummary{
				ApprovalsWithWriteAccess: 0,
			},
			checkSummary: &types.CheckSummary{
				Failing: map[string]string{"test": "failed"},
			},
			wantDescContains: "failing status checks and requires approval",
		},
		{
			name: "failing checks with approval",
			approvalSummary: &types.ApprovalSummary{
				ApprovalsWithWriteAccess: 1,
			},
			checkSummary: &types.CheckSummary{
				Failing: map[string]string{"test": "failed"},
			},
			wantDescContains: "blocked by failing status checks",
		},
		{
			name: "pending checks only",
			approvalSummary: &types.ApprovalSummary{
				ApprovalsWithWriteAccess: 1,
			},
			checkSummary: &types.CheckSummary{
				Failing: map[string]string{},
				Pending: map[string]string{"test": "pending"},
			},
			wantDescContains: "blocked by pending",
		},
		{
			name: "has approvals but still blocked",
			approvalSummary: &types.ApprovalSummary{
				ApprovalsWithWriteAccess: 1,
			},
			checkSummary: &types.CheckSummary{
				Failing: map[string]string{},
				Pending: map[string]string{},
			},
			wantDescContains: "blocked by required status checks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &types.PullRequest{
				MergeableState:  "blocked",
				ApprovalSummary: tt.approvalSummary,
				CheckSummary:    tt.checkSummary,
			}
			types.SetBlockedDescription(pr)
			if !contains(pr.MergeableStateDescription, tt.wantDescContains) {
				t.Errorf("MergeableStateDescription = %q, want to contain %q",
					pr.MergeableStateDescription, tt.wantDescContains)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func contains(s, substr string) bool {
	return s != "" && substr != "" && (s == substr || len(s) >= len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
