package prx

import (
	"testing"
	"time"

	"github.com/codeGROOVE-dev/prx/pkg/prx/types"
)

// TestCalculateParticipantAccess tests participant access map building
func TestCalculateParticipantAccess(t *testing.T) {
	pr := &types.PullRequest{
		Author:            "author1",
		AuthorWriteAccess: types.WriteAccessUnlikely,
		Assignees:         []string{"assignee1", "assignee2"},
		Reviewers: map[string]types.ReviewState{
			"reviewer1": types.ReviewStateApproved,
			"reviewer2": types.ReviewStatePending,
		},
	}

	events := []types.Event{
		{
			Kind:        "pr_opened",
			Actor:       "author1",
			WriteAccess: types.WriteAccessUnlikely,
			Timestamp:   time.Now(),
		},
		{
			Kind:        "review",
			Actor:       "reviewer1",
			WriteAccess: types.WriteAccessDefinitely,
			Timestamp:   time.Now(),
		},
		{
			Kind:        "comment",
			Actor:       "commenter1",
			WriteAccess: types.WriteAccessLikely,
			Timestamp:   time.Now(),
		},
		{
			Kind:        "labeled",
			Actor:       "labeler1",
			WriteAccess: types.WriteAccessDefinitely,
			Timestamp:   time.Now(),
		},
	}

	participants := types.CalculateParticipantAccess(events, pr)

	// Verify author is included
	if access, ok := participants["author1"]; !ok || access != types.WriteAccessUnlikely {
		t.Errorf("Expected author1 with WriteAccessUnlikely, got %v", access)
	}

	// Verify assignees are included with WriteAccessNA
	if access, ok := participants["assignee1"]; !ok || access != types.WriteAccessNA {
		t.Errorf("Expected assignee1 with WriteAccessNA, got %v", access)
	}
	if access, ok := participants["assignee2"]; !ok || access != types.WriteAccessNA {
		t.Errorf("Expected assignee2 with WriteAccessNA, got %v", access)
	}

	// Verify reviewers are included (reviewer1 gets upgraded from NA to Definitely by event)
	if access, ok := participants["reviewer1"]; !ok || access != types.WriteAccessDefinitely {
		t.Errorf("Expected reviewer1 with WriteAccessDefinitely, got %v", access)
	}
	if access, ok := participants["reviewer2"]; !ok || access != types.WriteAccessNA {
		t.Errorf("Expected reviewer2 with WriteAccessNA, got %v", access)
	}

	// Verify event actors are included
	if access, ok := participants["commenter1"]; !ok || access != types.WriteAccessLikely {
		t.Errorf("Expected commenter1 with WriteAccessLikely, got %v", access)
	}
	if access, ok := participants["labeler1"]; !ok || access != types.WriteAccessDefinitely {
		t.Errorf("Expected labeler1 with WriteAccessDefinitely, got %v", access)
	}

	// Verify total number of participants
	expected := 7 // author1, assignee1, assignee2, reviewer1, reviewer2, commenter1, labeler1
	if len(participants) != expected {
		t.Errorf("Expected %d participants, got %d: %v", expected, len(participants), participants)
	}
}

// TestCalculateParticipantAccessUpgrade tests that write access gets upgraded
func TestCalculateParticipantAccessUpgrade(t *testing.T) {
	pr := &types.PullRequest{
		Author:            "user1",
		AuthorWriteAccess: types.WriteAccessLikely,
	}

	events := []types.Event{
		{
			Kind:        "pr_opened",
			Actor:       "user1",
			WriteAccess: types.WriteAccessLikely,
			Timestamp:   time.Now(),
		},
		{
			Kind:        "labeled",
			Actor:       "user1",
			WriteAccess: types.WriteAccessDefinitely, // Upgraded after label action
			Timestamp:   time.Now().Add(time.Minute),
		},
	}

	participants := types.CalculateParticipantAccess(events, pr)

	// Verify user1 got upgraded to WriteAccessDefinitely
	if access, ok := participants["user1"]; !ok || access != types.WriteAccessDefinitely {
		t.Errorf("Expected user1 to be upgraded to WriteAccessDefinitely, got %v", access)
	}
}

// TestCalculateParticipantAccessEmpty tests empty PR
func TestCalculateParticipantAccessEmpty(t *testing.T) {
	pr := &types.PullRequest{
		Author:            "author1",
		AuthorWriteAccess: types.WriteAccessUnlikely,
	}

	events := []types.Event{}

	participants := types.CalculateParticipantAccess(events, pr)

	// Should only have the author
	if len(participants) != 1 {
		t.Errorf("Expected 1 participant, got %d", len(participants))
	}

	if access, ok := participants["author1"]; !ok || access != types.WriteAccessUnlikely {
		t.Errorf("Expected author1 with WriteAccessUnlikely, got %v", access)
	}
}
