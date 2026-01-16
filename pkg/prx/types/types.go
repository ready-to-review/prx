// Package types contains the core types and interfaces used across the prx library.
// This package is imported by platform implementations to avoid circular dependencies.
package types

import (
	"context"
	"time"
)

// Platform fetches pull request data from a code hosting service.
// Each platform (GitHub, GitLab, Codeberg) implements its own fetching strategy.
type Platform interface {
	// FetchPR retrieves a pull request with all events and metadata.
	// The refTime parameter is used for cache validation decisions.
	FetchPR(ctx context.Context, owner, repo string, number int, refTime time.Time) (*PullRequestData, error)

	// Name returns the platform identifier (e.g., "github", "gitlab", "codeberg").
	Name() string
}

// TestState represents the overall testing status of a pull request.
const (
	TestStateNone    = ""        // No tests or unknown state
	TestStateQueued  = "queued"  // Tests are queued to run
	TestStateRunning = "running" // Tests are currently executing
	TestStatePassing = "passing" // All tests passed
	TestStateFailing = "failing" // Some tests failed
	TestStatePending = "pending" // Some tests are pending
)

// ReviewState represents the current state of a reviewer's review.
type ReviewState string

// Review state constants.
const (
	ReviewStatePending          ReviewState = "pending"           // Review requested but not yet submitted
	ReviewStateApproved         ReviewState = "approved"          // Approved
	ReviewStateChangesRequested ReviewState = "changes_requested" // Changes requested
	ReviewStateCommented        ReviewState = "commented"         // Reviewed with comments only
)

// PullRequest represents a GitHub pull request with its essential metadata.
//
//nolint:govet // fieldalignment: Struct fields ordered for JSON clarity and API compatibility
type PullRequest struct {
	// 16-byte fields (time.Time)
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// 8-byte pointer fields
	ClosedAt        *time.Time       `json:"closed_at,omitempty"`
	MergedAt        *time.Time       `json:"merged_at,omitempty"`
	ApprovalSummary *ApprovalSummary `json:"approval_summary,omitempty"`
	CheckSummary    *CheckSummary    `json:"check_summary,omitempty"`
	Mergeable       *bool            `json:"mergeable,omitempty"`
	// 24-byte slice/map fields
	Assignees         []string               `json:"assignees,omitempty"`
	Labels            []string               `json:"labels,omitempty"`
	Commits           []string               `json:"commits,omitempty"` // List of commit SHAs in chronological order (oldest to newest)
	Reviewers         map[string]ReviewState `json:"reviewers,omitempty"`
	ParticipantAccess map[string]int         `json:"participant_access,omitempty"` // Map of username to WriteAccess level
	// 16-byte string fields
	MergeableState            string `json:"mergeable_state"`
	MergeableStateDescription string `json:"mergeable_state_description,omitempty"`
	Author                    string `json:"author"`
	Body                      string `json:"body"`
	Title                     string `json:"title"`
	MergedBy                  string `json:"merged_by,omitempty"`
	State                     string `json:"state"`
	TestState                 string `json:"test_state,omitempty"`
	HeadSHA                   string `json:"head_sha,omitempty"`
	// 8-byte int fields
	Number            int `json:"number"`
	ChangedFiles      int `json:"changed_files"`
	Deletions         int `json:"deletions"`
	Additions         int `json:"additions"`
	AuthorWriteAccess int `json:"author_write_access,omitempty"`
	// 1-byte bool fields
	AuthorBot bool `json:"author_bot"`
	Merged    bool `json:"merged"`
	Draft     bool `json:"draft"`
}

// CheckSummary aggregates all status checks and check runs.
type CheckSummary struct {
	Success   map[string]string `json:"success"`   // Map of successful check names to their status descriptions
	Failing   map[string]string `json:"failing"`   // Map of failing check names to their status descriptions (excludes cancelled)
	Pending   map[string]string `json:"pending"`   // Map of pending check names to their status descriptions
	Cancelled map[string]string `json:"cancelled"` // Map of cancelled check names to their status descriptions
	Skipped   map[string]string `json:"skipped"`   // Map of skipped check names to their status descriptions
	Stale     map[string]string `json:"stale"`     // Map of stale check names to their status descriptions
	Neutral   map[string]string `json:"neutral"`   // Map of neutral check names to their status descriptions
}

// ApprovalSummary tracks PR review approvals and change requests.
type ApprovalSummary struct {
	// Approvals from users confirmed to have write access (owners, collaborators, members with confirmed access)
	ApprovalsWithWriteAccess int `json:"approvals_with_write_access"`

	// Approvals from users with unknown or likely write access (members, uncertain cases)
	ApprovalsWithUnknownAccess int `json:"approvals_with_unknown_access"`

	// Approvals from users confirmed to not have write access (contributors, outside collaborators)
	ApprovalsWithoutWriteAccess int `json:"approvals_without_write_access"`

	// Outstanding change requests from any reviewer
	ChangesRequested int `json:"changes_requested"`
}

// PullRequestData contains a pull request and all its associated events.
type PullRequestData struct {
	CachedAt    time.Time   `json:"cached_at,omitzero"` // When this data was cached
	Events      []Event     `json:"events"`
	PullRequest PullRequest `json:"pull_request"`
}

// Event kind constants for PR timeline events.
const (
	EventKindCommit        = "commit"         // EventKindCommit represents a commit event.
	EventKindComment       = "comment"        // EventKindComment represents a comment event.
	EventKindReview        = "review"         // EventKindReview represents a review event.
	EventKindReviewComment = "review_comment" // EventKindReviewComment represents a review comment event.

	EventKindLabeled   = "labeled"   // EventKindLabeled represents a label added event.
	EventKindUnlabeled = "unlabeled" // EventKindUnlabeled represents a label removed event.

	EventKindAssigned   = "assigned"   // EventKindAssigned represents an assignment event.
	EventKindUnassigned = "unassigned" // EventKindUnassigned represents an unassignment event.

	EventKindMilestoned   = "milestoned"   // EventKindMilestoned represents a milestone added event.
	EventKindDemilestoned = "demilestoned" // EventKindDemilestoned represents a milestone removed event.

	EventKindReviewRequested      = "review_requested"       // EventKindReviewRequested represents a review request event.
	EventKindReviewRequestRemoved = "review_request_removed" // EventKindReviewRequestRemoved represents a review request removed event.

	EventKindPROpened       = "pr_opened"        // EventKindPROpened represents a PR opened event.
	EventKindPRClosed       = "pr_closed"        // EventKindPRClosed represents a PR closed event.
	EventKindPRMerged       = "pr_merged"        // EventKindPRMerged represents a PR merge event.
	EventKindMerged         = "merged"           // EventKindMerged represents a merge event from timeline.
	EventKindReadyForReview = "ready_for_review" // EventKindReadyForReview represents a ready for review event.
	EventKindConvertToDraft = "convert_to_draft" // EventKindConvertToDraft represents a convert to draft event.
	EventKindClosed         = "closed"           // EventKindClosed represents a PR closed event.
	EventKindReopened       = "reopened"         // EventKindReopened represents a PR reopened event.
	EventKindRenamedTitle   = "renamed_title"    // EventKindRenamedTitle represents a title rename event.

	EventKindMentioned       = "mentioned"        // EventKindMentioned represents a mention event.
	EventKindReferenced      = "referenced"       // EventKindReferenced represents a reference event.
	EventKindCrossReferenced = "cross_referenced" // EventKindCrossReferenced represents a cross-reference event.

	EventKindPinned      = "pinned"      // EventKindPinned represents a pin event.
	EventKindUnpinned    = "unpinned"    // EventKindUnpinned represents an unpin event.
	EventKindTransferred = "transferred" // EventKindTransferred represents a transfer event.

	EventKindSubscribed   = "subscribed"   // EventKindSubscribed represents a subscription event.
	EventKindUnsubscribed = "unsubscribed" // EventKindUnsubscribed represents an unsubscription event.

	EventKindHeadRefDeleted     = "head_ref_deleted"      // EventKindHeadRefDeleted represents a head ref deletion event.
	EventKindHeadRefRestored    = "head_ref_restored"     // EventKindHeadRefRestored represents a head ref restoration event.
	EventKindHeadRefForcePushed = "head_ref_force_pushed" // EventKindHeadRefForcePushed represents a head ref force push event.

	EventKindBaseRefChanged     = "base_ref_changed"      // EventKindBaseRefChanged represents a base ref change event.
	EventKindBaseRefForcePushed = "base_ref_force_pushed" // EventKindBaseRefForcePushed represents a base ref force push event.

	EventKindReviewDismissed = "review_dismissed" // EventKindReviewDismissed represents a review dismissed event.

	EventKindLocked   = "locked"   // EventKindLocked represents a lock event.
	EventKindUnlocked = "unlocked" // EventKindUnlocked represents an unlock event.

	EventKindAutoMergeEnabled      = "auto_merge_enabled"       // EventKindAutoMergeEnabled represents an auto merge enabled event.
	EventKindAutoMergeDisabled     = "auto_merge_disabled"      // EventKindAutoMergeDisabled represents an auto merge disabled event.
	EventKindAddedToMergeQueue     = "added_to_merge_queue"     // EventKindAddedToMergeQueue represents an added to merge queue event.
	EventKindRemovedFromMergeQueue = "removed_from_merge_queue" // EventKindRemovedFromMergeQueue represents removal from merge queue.

	// EventKindAutomaticBaseChangeSucceeded represents a successful base change.
	EventKindAutomaticBaseChangeSucceeded = "automatic_base_change_succeeded"
	// EventKindAutomaticBaseChangeFailed represents a failed base change.
	EventKindAutomaticBaseChangeFailed = "automatic_base_change_failed"

	EventKindDeployed = "deployed" // EventKindDeployed represents a deployment event.
	// EventKindDeploymentEnvironmentChanged represents a deployment environment change event.
	EventKindDeploymentEnvironmentChanged = "deployment_environment_changed"

	EventKindConnected    = "connected"    // EventKindConnected represents a connected event.
	EventKindDisconnected = "disconnected" // EventKindDisconnected represents a disconnected event.
	EventKindUserBlocked  = "user_blocked" // EventKindUserBlocked represents a user blocked event.

	EventKindStatusCheck = "status_check" // EventKindStatusCheck represents a status check event (from APIs).
	EventKindCheckRun    = "check_run"    // EventKindCheckRun represents a check run event (from APIs).
)

// WriteAccess constants for the Event.WriteAccess field.
const (
	WriteAccessNo         = -2 // User confirmed to not have write access
	WriteAccessUnlikely   = -1 // User unlikely to have write access (CONTRIBUTOR, NONE, etc.)
	WriteAccessNA         = 0  // Not applicable/not set (omitted from JSON)
	WriteAccessLikely     = 1  // User likely has write access but unable to confirm (MEMBER with 403 API response)
	WriteAccessDefinitely = 2  // User definitely has write access (OWNER, COLLABORATOR, or confirmed via API)
)

// Event represents a single event that occurred on a pull request.
// Each event captures who did what and when, with additional context depending on the event type.
type Event struct {
	Timestamp   time.Time `json:"timestamp"`
	Kind        string    `json:"kind"`
	Actor       string    `json:"actor"`
	Target      string    `json:"target,omitempty"`
	Outcome     string    `json:"outcome,omitempty"`
	Body        string    `json:"body,omitempty"`
	Description string    `json:"description,omitempty"`
	WriteAccess int       `json:"write_access,omitempty"`
	Bot         bool      `json:"bot,omitempty"`
	TargetIsBot bool      `json:"target_is_bot,omitempty"`
	Question    bool      `json:"question,omitempty"`
	Required    bool      `json:"required,omitempty"`
	Outdated    bool      `json:"outdated,omitempty"` // For review comments: indicates comment is on outdated code
}

// FinalizePullRequest applies final calculations and consistency fixes.
func FinalizePullRequest(pullRequest *PullRequest, events []Event, requiredChecks []string, testStateFromAPI string) {
	pullRequest.TestState = testStateFromAPI
	pullRequest.CheckSummary = CalculateCheckSummary(events, requiredChecks)
	pullRequest.ApprovalSummary = CalculateApprovalSummary(events)
	pullRequest.ParticipantAccess = CalculateParticipantAccess(events, pullRequest)

	FixTestState(pullRequest)

	// Ensure mergeable is consistent with mergeable_state
	if pullRequest.MergeableState == "blocked" || pullRequest.MergeableState == "dirty" || pullRequest.MergeableState == "unstable" {
		falseVal := false
		pullRequest.Mergeable = &falseVal
	}

	SetMergeableDescription(pullRequest)
}

// FixTestState ensures test_state is consistent with check_summary.
// If CheckSummary has data, it takes precedence. Otherwise, preserve
// the existing TestState (which may have been set from platform-specific
// data like GitLab pipelines).
func FixTestState(pullRequest *PullRequest) {
	switch {
	case len(pullRequest.CheckSummary.Failing) > 0 || len(pullRequest.CheckSummary.Cancelled) > 0:
		pullRequest.TestState = TestStateFailing
	case len(pullRequest.CheckSummary.Pending) > 0:
		pullRequest.TestState = TestStatePending
	case len(pullRequest.CheckSummary.Success) > 0:
		pullRequest.TestState = TestStatePassing
	default:
		// Preserve existing TestState if CheckSummary is empty.
		// This allows platform-specific test state (e.g., GitLab pipelines)
		// to be retained when there are no check_run events.
		if pullRequest.TestState == "" {
			pullRequest.TestState = TestStateNone
		}
	}
}

// SetMergeableDescription adds human-readable description for mergeable state.
func SetMergeableDescription(pullRequest *PullRequest) {
	switch pullRequest.MergeableState {
	case "blocked":
		SetBlockedDescription(pullRequest)
	case "dirty":
		pullRequest.MergeableStateDescription = "PR has merge conflicts that need to be resolved"
	case "unstable":
		pullRequest.MergeableStateDescription = "PR is mergeable but status checks are failing"
	case "clean":
		pullRequest.MergeableStateDescription = "PR is ready to merge"
	case "unknown":
		pullRequest.MergeableStateDescription = "Merge status is being calculated"
	case "draft":
		pullRequest.MergeableStateDescription = "PR is in draft state"
	default:
		pullRequest.MergeableStateDescription = ""
	}
}

// SetBlockedDescription determines what's blocking the PR and sets appropriate description.
func SetBlockedDescription(pullRequest *PullRequest) {
	hasApprovals := pullRequest.ApprovalSummary.ApprovalsWithWriteAccess > 0
	hasFailingChecks := len(pullRequest.CheckSummary.Failing) > 0 || len(pullRequest.CheckSummary.Cancelled) > 0
	hasPendingChecks := len(pullRequest.CheckSummary.Pending) > 0

	switch {
	case !hasApprovals && !hasFailingChecks:
		if hasPendingChecks {
			pullRequest.MergeableStateDescription = "PR requires approval and has pending status checks"
		} else {
			pullRequest.MergeableStateDescription = "PR requires approval"
		}
	case hasFailingChecks:
		if !hasApprovals {
			pullRequest.MergeableStateDescription = "PR has failing status checks and requires approval"
		} else {
			pullRequest.MergeableStateDescription = "PR is blocked by failing status checks"
		}
	case hasPendingChecks:
		pullRequest.MergeableStateDescription = "PR is blocked by pending status checks"
	default:
		pullRequest.MergeableStateDescription = "PR is blocked by required status checks, reviews, or branch protection rules"
	}
}
