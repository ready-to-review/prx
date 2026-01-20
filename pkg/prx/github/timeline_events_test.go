package github

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/codeGROOVE-dev/prx/pkg/prx"
)

// TestAutoMergeEventIntegration tests that we properly parse auto_merge_enabled
// events from a real PR (gitMDM PR #15).
func TestAutoMergeEventIntegration(t *testing.T) {
	// Load the saved PR data from testdata
	data, err := os.ReadFile("testdata/gitmdm_pr15.json")
	if err != nil {
		t.Fatalf("Failed to read test data: %v", err)
	}

	var prData prx.PullRequestData
	if err := json.Unmarshal(data, &prData); err != nil {
		t.Fatalf("Failed to unmarshal test data: %v", err)
	}

	// Verify we have the expected number of events
	if len(prData.Events) < 20 {
		t.Errorf("Expected at least 20 events, got %d", len(prData.Events))
	}

	// Find the auto_merge_enabled event
	var autoMergeEvent *prx.Event
	for i := range prData.Events {
		if prData.Events[i].Kind == "auto_merge_enabled" {
			autoMergeEvent = &prData.Events[i]
			break
		}
	}

	if autoMergeEvent == nil {
		t.Fatal("auto_merge_enabled event not found in events list")
	}

	// Verify the event details
	expectedTime, err := time.Parse(time.RFC3339, "2025-10-07T17:29:24Z")
	if err != nil {
		t.Fatalf("Failed to parse expected time: %v", err)
	}
	if !autoMergeEvent.Timestamp.Equal(expectedTime) {
		t.Errorf("Expected timestamp %v, got %v", expectedTime, autoMergeEvent.Timestamp)
	}

	if autoMergeEvent.Actor != "tstromberg" {
		t.Errorf("Expected actor 'tstromberg', got '%s'", autoMergeEvent.Actor)
	}

	if autoMergeEvent.Kind != "auto_merge_enabled" {
		t.Errorf("Expected kind 'auto_merge_enabled', got '%s'", autoMergeEvent.Kind)
	}

	// Verify the auto_merge event appears after review_requested but before review
	reviewRequestedIdx, autoMergeIdx, reviewIdx := -1, -1, -1
	for i := range prData.Events {
		switch prData.Events[i].Kind {
		case "review_requested":
			if prData.Events[i].Target == "tstromberg" {
				reviewRequestedIdx = i
			}
		case "auto_merge_enabled":
			autoMergeIdx = i
		case "review":
			if prData.Events[i].Actor == "tstromberg" && prData.Events[i].Outcome == "approved" {
				reviewIdx = i
			}
		}
	}

	if reviewRequestedIdx == -1 {
		t.Error("review_requested event not found")
	}
	if autoMergeIdx == -1 {
		t.Error("auto_merge_enabled event not found")
	}
	if reviewIdx == -1 {
		t.Error("review event not found")
	}

	// Verify chronological order
	if reviewRequestedIdx != -1 && autoMergeIdx != -1 && autoMergeIdx < reviewRequestedIdx {
		t.Error("auto_merge_enabled should come after review_requested")
	}
	if autoMergeIdx != -1 && reviewIdx != -1 && reviewIdx < autoMergeIdx {
		t.Error("review should come after auto_merge_enabled")
	}
}

// TestParseGraphQLTimelineEventAutoMerge tests parsing of auto-merge events
func TestParseGraphQLTimelineEventAutoMerge(t *testing.T) {
	p := &Platform{}

	tests := []struct {
		name     string
		item     map[string]any
		expected string
	}{
		{
			name: "AutoMergeEnabledEvent",
			item: map[string]any{
				"__typename": "AutoMergeEnabledEvent",
				"id":         "AME_123",
				"createdAt":  "2025-10-07T17:29:24Z",
				"actor": map[string]any{
					"login": "testuser",
				},
			},
			expected: "auto_merge_enabled",
		},
		{
			name: "AutoMergeDisabledEvent",
			item: map[string]any{
				"__typename": "AutoMergeDisabledEvent",
				"id":         "AMD_123",
				"createdAt":  "2025-10-07T18:00:00Z",
				"actor": map[string]any{
					"login": "testuser",
				},
			},
			expected: "auto_merge_disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := p.parseGraphQLTimelineEvent(context.TODO(), tt.item, "owner", "repo")
			if event == nil {
				t.Fatal("Expected event, got nil")
			}
			if event.Kind != tt.expected {
				t.Errorf("Expected kind '%s', got '%s'", tt.expected, event.Kind)
			}
			if event.Actor != "testuser" {
				t.Errorf("Expected actor 'testuser', got '%s'", event.Actor)
			}
		})
	}
}

// TestParseGraphQLTimelineEvent_AllTypes tests parsing of all timeline event types
func TestParseGraphQLTimelineEvent_AdditionalTypes(t *testing.T) {
	p := &Platform{}
	ctx := context.TODO()

	tests := []struct {
		name     string
		item     map[string]any
		wantKind string
		wantNil  bool
	}{
		{
			name: "LabeledEvent",
			item: map[string]any{
				"__typename": "LabeledEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "user1"},
				"label": map[string]any{"name": "bug"},
			},
			wantKind: prx.EventKindLabeled,
		},
		{
			name: "UnlabeledEvent",
			item: map[string]any{
				"__typename": "UnlabeledEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "user1"},
				"label": map[string]any{"name": "wontfix"},
			},
			wantKind: prx.EventKindUnlabeled,
		},
		{
			name: "AssignedEvent",
			item: map[string]any{
				"__typename": "AssignedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "assigner"},
				"assignee": map[string]any{"login": "assignee1"},
			},
			wantKind: prx.EventKindAssigned,
		},
		{
			name: "UnassignedEvent",
			item: map[string]any{
				"__typename": "UnassignedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "unassigner"},
				"assignee": map[string]any{"login": "removed"},
			},
			wantKind: prx.EventKindUnassigned,
		},
		{
			name: "MilestonedEvent",
			item: map[string]any{
				"__typename": "MilestonedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "pm"},
				"milestoneTitle": "v1.0",
			},
			wantKind: prx.EventKindMilestoned,
		},
		{
			name: "DemilestonedEvent",
			item: map[string]any{
				"__typename": "DemilestonedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "pm"},
				"milestoneTitle": "v0.9",
			},
			wantKind: prx.EventKindDemilestoned,
		},
		{
			name: "RenamedTitleEvent",
			item: map[string]any{
				"__typename": "RenamedTitleEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "editor"},
				"previousTitle": "Old",
				"currentTitle": "New",
			},
			wantKind: prx.EventKindRenamedTitle,
		},
		{
			name: "LockedEvent",
			item: map[string]any{
				"__typename": "LockedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "mod"},
			},
			wantKind: prx.EventKindLocked,
		},
		{
			name: "UnlockedEvent",
			item: map[string]any{
				"__typename": "UnlockedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "mod"},
			},
			wantKind: prx.EventKindUnlocked,
		},
		{
			name: "HeadRefDeletedEvent",
			item: map[string]any{
				"__typename": "HeadRefDeletedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "maint"},
			},
			wantKind: prx.EventKindHeadRefDeleted,
		},
		{
			name: "HeadRefForcePushedEvent",
			item: map[string]any{
				"__typename": "HeadRefForcePushedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "dev"},
			},
			wantKind: prx.EventKindHeadRefForcePushed,
		},
		{
			name: "BaseRefForcePushedEvent",
			item: map[string]any{
				"__typename": "BaseRefForcePushedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "maint"},
			},
			wantKind: prx.EventKindBaseRefForcePushed,
		},
		{
			name: "ReviewRequestedEvent",
			item: map[string]any{
				"__typename": "ReviewRequestedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "requester"},
				"requestedReviewer": map[string]any{"login": "reviewer"},
			},
			wantKind: prx.EventKindReviewRequested,
		},
		{
			name: "ReviewRequestRemovedEvent",
			item: map[string]any{
				"__typename": "ReviewRequestRemovedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "requester"},
				"requestedReviewer": map[string]any{"login": "removed"},
			},
			wantKind: prx.EventKindReviewRequestRemoved,
		},
		{
			name: "ReviewDismissedEvent",
			item: map[string]any{
				"__typename": "ReviewDismissedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "maint"},
				"dismissalMessage": "No longer relevant",
			},
			wantKind: prx.EventKindReviewDismissed,
		},
		{
			name: "ReadyForReviewEvent",
			item: map[string]any{
				"__typename": "ReadyForReviewEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "author"},
			},
			wantKind: prx.EventKindReadyForReview,
		},
		{
			name: "ConvertToDraftEvent",
			item: map[string]any{
				"__typename": "ConvertToDraftEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "author"},
			},
			wantKind: prx.EventKindConvertToDraft,
		},
		{
			name: "MergedEvent",
			item: map[string]any{
				"__typename": "MergedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "merger"},
			},
			wantKind: prx.EventKindMerged,
		},
		{
			name: "ClosedEvent",
			item: map[string]any{
				"__typename": "ClosedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "closer"},
			},
			wantKind: prx.EventKindClosed,
		},
		{
			name: "ReopenedEvent",
			item: map[string]any{
				"__typename": "ReopenedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "reopener"},
			},
			wantKind: prx.EventKindReopened,
		},
		{
			name: "CrossReferencedEvent",
			item: map[string]any{
				"__typename": "CrossReferencedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "dev"},
				"source": map[string]any{"__typename": "Issue"},
			},
			wantKind: prx.EventKindCrossReferenced,
		},
		{
			name: "ReferencedEvent",
			item: map[string]any{
				"__typename": "ReferencedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "dev"},
			},
			wantKind: prx.EventKindReferenced,
		},
		{
			name: "SubscribedEvent",
			item: map[string]any{
				"__typename": "SubscribedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "subscriber"},
			},
			wantKind: prx.EventKindSubscribed,
		},
		{
			name: "UnsubscribedEvent",
			item: map[string]any{
				"__typename": "UnsubscribedEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
				"actor": map[string]any{"login": "unsubscriber"},
			},
			wantKind: prx.EventKindUnsubscribed,
		},
		{
			name: "UnknownEventType",
			item: map[string]any{
				"__typename": "UnknownFutureEvent",
				"createdAt":  "2024-01-01T10:00:00Z",
			},
			wantNil: true,
		},
		{
			name: "MissingTypename",
			item: map[string]any{
				"createdAt": "2024-01-01T10:00:00Z",
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := p.parseGraphQLTimelineEvent(ctx, tt.item, "owner", "repo")
			if tt.wantNil {
				if event != nil {
					t.Errorf("Expected nil event, got %+v", event)
				}
				return
			}
			if event == nil {
				t.Fatal("Expected event, got nil")
			}
			if event.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", event.Kind, tt.wantKind)
			}
		})
	}
}

// TestParseGraphQLTimelineEventNewTypes tests parsing of all newly added event types
func TestParseGraphQLTimelineEventNewTypes(t *testing.T) {
	p := &Platform{}

	tests := []struct {
		typename string
		expected string
	}{
		{"ReviewDismissedEvent", "review_dismissed"},
		{"BaseRefChangedEvent", "base_ref_changed"},
		{"BaseRefForcePushedEvent", "base_ref_force_pushed"},
		{"HeadRefForcePushedEvent", "head_ref_force_pushed"},
		{"HeadRefDeletedEvent", "head_ref_deleted"},
		{"HeadRefRestoredEvent", "head_ref_restored"},
		{"RenamedTitleEvent", "renamed_title"},
		{"LockedEvent", "locked"},
		{"UnlockedEvent", "unlocked"},
		{"AddedToMergeQueueEvent", "added_to_merge_queue"},
		{"RemovedFromMergeQueueEvent", "removed_from_merge_queue"},
		{"AutomaticBaseChangeSucceededEvent", "automatic_base_change_succeeded"},
		{"AutomaticBaseChangeFailedEvent", "automatic_base_change_failed"},
		{"ConnectedEvent", "connected"},
		{"DisconnectedEvent", "disconnected"},
		{"CrossReferencedEvent", "cross_referenced"},
		{"ReferencedEvent", "referenced"},
		{"SubscribedEvent", "subscribed"},
		{"UnsubscribedEvent", "unsubscribed"},
		{"DeployedEvent", "deployed"},
		{"DeploymentEnvironmentChangedEvent", "deployment_environment_changed"},
		{"PinnedEvent", "pinned"},
		{"UnpinnedEvent", "unpinned"},
		{"TransferredEvent", "transferred"},
		{"UserBlockedEvent", "user_blocked"},
	}

	for _, tt := range tests {
		t.Run(tt.typename, func(t *testing.T) {
			item := map[string]any{
				"__typename": tt.typename,
				"id":         "TEST_123",
				"createdAt":  "2025-10-07T12:00:00Z",
				"actor": map[string]any{
					"login": "testuser",
				},
			}

			event := p.parseGraphQLTimelineEvent(context.TODO(), item, "owner", "repo")
			if event == nil {
				t.Fatalf("Expected event for %s, got nil", tt.typename)
			}
			if event.Kind != tt.expected {
				t.Errorf("For %s: expected kind '%s', got '%s'", tt.typename, tt.expected, event.Kind)
			}
		})
	}
}

// TestParseGraphQLTimelineEventRenamedTitle tests that renamed title events include title info
func TestParseGraphQLTimelineEventRenamedTitle(t *testing.T) {
	p := &Platform{}

	item := map[string]any{
		"__typename":    "RenamedTitleEvent",
		"id":            "RTE_123",
		"createdAt":     "2025-10-07T12:00:00Z",
		"previousTitle": "Old Title",
		"currentTitle":  "New Title",
		"actor": map[string]any{
			"login": "testuser",
		},
	}

	event := p.parseGraphQLTimelineEvent(context.TODO(), item, "owner", "repo")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}

	if event.Kind != "renamed_title" {
		t.Errorf("Expected kind 'renamed_title', got '%s'", event.Kind)
	}

	expectedBody := "Renamed from \"Old Title\" to \"New Title\""
	if event.Body != expectedBody {
		t.Errorf("Expected body '%s', got '%s'", expectedBody, event.Body)
	}
}

// TestParseGraphQLTimelineEventReviewDismissed tests that review dismissed events include message
func TestParseGraphQLTimelineEventReviewDismissed(t *testing.T) {
	p := &Platform{}

	item := map[string]any{
		"__typename":       "ReviewDismissedEvent",
		"id":               "RDE_123",
		"createdAt":        "2025-10-07T12:00:00Z",
		"dismissalMessage": "Not relevant anymore",
		"actor": map[string]any{
			"login": "testuser",
		},
	}

	event := p.parseGraphQLTimelineEvent(context.TODO(), item, "owner", "repo")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}

	if event.Kind != "review_dismissed" {
		t.Errorf("Expected kind 'review_dismissed', got '%s'", event.Kind)
	}

	if event.Body != "Not relevant anymore" {
		t.Errorf("Expected body 'Not relevant anymore', got '%s'", event.Body)
	}
}
