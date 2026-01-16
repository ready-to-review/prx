package prx

import (
	"testing"
	"time"

	"github.com/codeGROOVE-dev/prx/pkg/prx/types"
)

func TestUpgradeWriteAccess(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		events   []types.Event
		expected map[string]int // actor -> expected write access
	}{
		{
			name: "upgrade write access for user who merged PR",
			events: []types.Event{
				{
					Kind:        "comment",
					Timestamp:   now.Add(-2 * time.Hour),
					Actor:       "reviewer1",
					WriteAccess: types.WriteAccessLikely, // 1
				},
				{
					Kind:      "pr_merged",
					Timestamp: now.Add(-1 * time.Hour),
					Actor:     "reviewer1",
				},
			},
			expected: map[string]int{
				"reviewer1": types.WriteAccessDefinitely, // Should be upgraded to 2
			},
		},
		{
			name: "upgrade write access for user who labeled issue",
			events: []types.Event{
				{
					Kind:        "review",
					Timestamp:   now.Add(-3 * time.Hour),
					Actor:       "maintainer",
					WriteAccess: types.WriteAccessLikely, // 1
					Outcome:     "approved",
				},
				{
					Kind:      "labeled",
					Timestamp: now.Add(-2 * time.Hour),
					Actor:     "maintainer",
					Target:    "bug",
				},
			},
			expected: map[string]int{
				"maintainer": types.WriteAccessDefinitely, // Should be upgraded to 2
			},
		},
		{
			name: "don't upgrade if already definitely has write access",
			events: []types.Event{
				{
					Kind:        "comment",
					Timestamp:   now.Add(-2 * time.Hour),
					Actor:       "owner",
					WriteAccess: types.WriteAccessDefinitely, // Already 2
				},
				{
					Kind:      "pr_merged",
					Timestamp: now.Add(-1 * time.Hour),
					Actor:     "owner",
				},
			},
			expected: map[string]int{
				"owner": types.WriteAccessDefinitely, // Should remain 2
			},
		},
		{
			name: "don't upgrade if user has no write access",
			events: []types.Event{
				{
					Kind:        "comment",
					Timestamp:   now.Add(-2 * time.Hour),
					Actor:       "contributor",
					WriteAccess: types.WriteAccessUnlikely, // -1
				},
				{
					Kind:      "comment",
					Timestamp: now.Add(-1 * time.Hour),
					Actor:     "contributor",
					Body:      "Please merge this",
				},
			},
			expected: map[string]int{
				"contributor": types.WriteAccessUnlikely, // Should remain 0
			},
		},
		{
			name: "upgrade multiple users based on different actions",
			events: []types.Event{
				{
					Kind:        "comment",
					Timestamp:   now.Add(-4 * time.Hour),
					Actor:       "user1",
					WriteAccess: types.WriteAccessLikely, // 1
				},
				{
					Kind:        "review",
					Timestamp:   now.Add(-3 * time.Hour),
					Actor:       "user2",
					WriteAccess: types.WriteAccessLikely, // 1
					Outcome:     "approved",
				},
				{
					Kind:      "assigned",
					Timestamp: now.Add(-2 * time.Hour),
					Actor:     "user1",
					Target:    "assignee1",
				},
				{
					Kind:      "milestoned",
					Timestamp: now.Add(-1 * time.Hour),
					Actor:     "user2",
				},
			},
			expected: map[string]int{
				"user1": types.WriteAccessDefinitely, // Should be upgraded to 2
				"user2": types.WriteAccessDefinitely, // Should be upgraded to 2
			},
		},
		{
			name: "handle events with nil write access",
			events: []types.Event{
				{
					Kind:        "comment",
					Timestamp:   now.Add(-2 * time.Hour),
					Actor:       "user1",
					WriteAccess: types.WriteAccessNA, // no write access info
				},
				{
					Kind:      "labeled",
					Timestamp: now.Add(-1 * time.Hour),
					Actor:     "user1",
				},
			},
			expected: map[string]int{
				// user1 should not have write access modified since it was nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of events to avoid modifying test data
			events := make([]types.Event, len(tt.events))
			copy(events, tt.events)

			// Apply the upgrade function
			types.UpgradeWriteAccess(events)

			// Check results - look for events with WriteAccess field
			for _, event := range events {
				if event.WriteAccess != types.WriteAccessNA {
					if expectedAccess, ok := tt.expected[event.Actor]; ok {
						if event.WriteAccess != expectedAccess {
							t.Errorf("%s: Actor %s has write access %d, expected %d",
								tt.name, event.Actor, event.WriteAccess, expectedAccess)
						}
					}
				}
			}
		})
	}
}
