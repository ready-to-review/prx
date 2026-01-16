package github

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/fido"
	"github.com/codeGROOVE-dev/prx/pkg/prx/types"
)

const (
	// HTTP client configuration constants.
	maxIdleConns        = 100
	maxIdleConnsPerHost = 10
	idleConnTimeoutSec  = 90

	// Cache TTL constants.
	checkRunsCacheTTL     = 20 * 24 * time.Hour // 20 days - validity checked against reference time
	collaboratorsCacheTTL = 3 * time.Hour       // 3 hours - repo-level, simple TTL
	rulesetsCacheTTL      = 3 * time.Hour       // 3 hours - repo-level, simple TTL
)

// cachedCheckRuns stores check run events with a timestamp for cache validation.
type cachedCheckRuns struct {
	CachedAt time.Time
	Events   []types.Event
}

// Platform implements the prx.Platform interface for GitHub.
type Platform struct {
	client             *Client
	logger             *slog.Logger
	collaboratorsCache *fido.Cache[string, map[string]string]
	rulesetsCache      *fido.Cache[string, []string]
	checkRunsCache     *fido.Cache[string, cachedCheckRuns]
}

// Option configures a Platform.
type Option func(*Platform)

// WithLogger sets a custom logger for the GitHub platform.
func WithLogger(logger *slog.Logger) Option {
	return func(p *Platform) {
		p.logger = logger
	}
}

// WithHTTPClient sets a custom HTTP client for the GitHub platform.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(p *Platform) {
		if httpClient.Transport == nil {
			httpClient.Transport = &Transport{Base: http.DefaultTransport}
		} else if _, ok := httpClient.Transport.(*Transport); !ok {
			httpClient.Transport = &Transport{Base: httpClient.Transport}
		}
		p.client = &Client{
			HTTPClient: httpClient,
			Token:      p.client.Token,
			BaseURL:    p.client.BaseURL,
		}
	}
}

// WithBaseURL sets a custom base URL for the GitHub API.
func WithBaseURL(baseURL string) Option {
	return func(p *Platform) {
		p.client.BaseURL = baseURL
	}
}

// NewTestPlatform creates a Platform for testing with a custom base URL.
// Exported for use in prx package tests.
func NewTestPlatform(token, baseURL string) *Platform {
	return NewPlatform(token, WithBaseURL(baseURL))
}

// NewPlatform creates a new GitHub platform client.
func NewPlatform(token string, opts ...Option) *Platform {
	transport := &http.Transport{
		MaxIdleConns:        maxIdleConns,
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
		IdleConnTimeout:     idleConnTimeoutSec * time.Second,
		DisableCompression:  false,
		DisableKeepAlives:   false,
	}

	p := &Platform{
		client: &Client{
			HTTPClient: &http.Client{
				Transport: &Transport{Base: transport},
				Timeout:   30 * time.Second,
			},
			Token:   token,
			BaseURL: API,
		},
		logger:             slog.Default(),
		collaboratorsCache: fido.New[string, map[string]string](fido.TTL(collaboratorsCacheTTL)),
		rulesetsCache:      fido.New[string, []string](fido.TTL(rulesetsCacheTTL)),
		checkRunsCache:     fido.New[string, cachedCheckRuns](fido.TTL(checkRunsCacheTTL)),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Name returns the platform identifier.
func (*Platform) Name() string {
	return "github"
}

// FetchPR retrieves a pull request with all events and metadata.
func (p *Platform) FetchPR(ctx context.Context, owner, repo string, number int, refTime time.Time) (*types.PullRequestData, error) {
	p.logger.InfoContext(ctx, "fetching pull request via GraphQL", "owner", owner, "repo", repo, "pr", number)

	prData, err := p.fetchPullRequestCompleteViaGraphQL(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("GraphQL query failed: %w", err)
	}

	additionalRequired, err := p.fetchRulesetsREST(ctx, owner, repo)
	if err != nil {
		p.logger.WarnContext(ctx, "failed to fetch rulesets", "error", err)
	} else if prData.PullRequest.CheckSummary != nil && len(additionalRequired) > 0 {
		p.logger.InfoContext(ctx, "added required checks from rulesets", "count", len(additionalRequired))
	}

	existingRequired := p.existingRequiredChecks(prData)
	existingRequired = append(existingRequired, additionalRequired...)

	checkRunEvents := p.fetchAllCheckRunsREST(ctx, owner, repo, prData, refTime)

	for i := range checkRunEvents {
		if slices.Contains(existingRequired, checkRunEvents[i].Body) {
			checkRunEvents[i].Required = true
		}
	}

	prData.Events = append(prData.Events, checkRunEvents...)

	if len(checkRunEvents) > 0 {
		p.recalculateCheckSummaryWithCheckRuns(ctx, prData, checkRunEvents)
	}

	p.logger.InfoContext(ctx, "fetched check runs via REST", "count", len(checkRunEvents))

	sort.Slice(prData.Events, func(i, j int) bool {
		return prData.Events[i].Timestamp.Before(prData.Events[j].Timestamp)
	})

	apiCallsUsed := 2
	if len(checkRunEvents) > 0 {
		apiCallsUsed++
	}

	p.logger.InfoContext(ctx, "successfully fetched pull request via hybrid GraphQL+REST",
		"owner", owner, "repo", repo, "pr", number,
		"event_count", len(prData.Events),
		"api_calls_made", fmt.Sprintf("%d (vs 13+ with REST)", apiCallsUsed))

	return prData, nil
}

// fetchPullRequestCompleteViaGraphQL fetches all PR data in a single GraphQL query.
func (p *Platform) fetchPullRequestCompleteViaGraphQL(ctx context.Context, owner, repo string, prNumber int) (*types.PullRequestData, error) {
	data, err := p.executeGraphQL(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	pr := p.convertGraphQLToPullRequest(ctx, data, owner, repo)
	events := p.convertGraphQLToEventsComplete(ctx, data, owner, repo)
	requiredChecks := p.extractRequiredChecksFromGraphQL(data)

	events = types.FilterEvents(events)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	types.UpgradeWriteAccess(events)

	testState := p.calculateTestStateFromGraphQL(data)
	types.FinalizePullRequest(&pr, events, requiredChecks, testState)

	return &types.PullRequestData{
		PullRequest: pr,
		Events:      events,
	}, nil
}

// executeGraphQL executes the GraphQL query and handles errors.
func (p *Platform) executeGraphQL(ctx context.Context, owner, repo string, prNumber int) (*graphQLPullRequestComplete, error) {
	variables := map[string]any{
		"owner":  owner,
		"repo":   repo,
		"number": prNumber,
	}

	var result graphQLCompleteResponse
	if err := p.client.GraphQL(ctx, completeGraphQLQuery, variables, &result); err != nil {
		return nil, err
	}

	if len(result.Errors) > 0 {
		var errMsgs []string
		var hasPermissionError bool
		for _, e := range result.Errors {
			errMsgs = append(errMsgs, e.Message)
			msg := strings.ToLower(e.Message)
			if strings.Contains(msg, "not accessible by integration") ||
				strings.Contains(msg, "resource not accessible") ||
				strings.Contains(msg, "forbidden") ||
				strings.Contains(msg, "insufficient permissions") ||
				strings.Contains(msg, "requires authentication") {
				hasPermissionError = true
			}
		}

		errStr := strings.Join(errMsgs, "; ")
		if result.Data.Repository.PullRequest.Number == 0 {
			if hasPermissionError {
				return nil, fmt.Errorf(
					"fetching PR %s/%s#%d via GraphQL failed due to insufficient permissions: %s "+
						"(note: some fields like branchProtectionRule or refUpdateRule may require push access "+
						"even on public repositories; check token scopes or try using a token with 'repo' or 'public_repo' scope)",
					owner, repo, prNumber, errStr)
			}
			return nil, fmt.Errorf("fetching PR %s/%s#%d via GraphQL: %s", owner, repo, prNumber, errStr)
		}

		if hasPermissionError {
			p.logger.WarnContext(ctx, "GraphQL query returned permission errors but PR data was retrieved - some fields may be missing",
				"owner", owner,
				"repo", repo,
				"pr", prNumber,
				"errors", errStr,
				"note", "fields like branchProtectionRule or refUpdateRule require push access")
		} else {
			p.logger.WarnContext(ctx, "GraphQL query returned errors but PR data was retrieved",
				"owner", owner,
				"repo", repo,
				"pr", prNumber,
				"errors", errStr)
		}
	}

	return &result.Data.Repository.PullRequest, nil
}

// convertGraphQLToPullRequest converts GraphQL data to PullRequest.
func (p *Platform) convertGraphQLToPullRequest(ctx context.Context, data *graphQLPullRequestComplete, owner, repo string) types.PullRequest {
	pr := types.PullRequest{
		Number:       data.Number,
		Title:        data.Title,
		Body:         types.Truncate(data.Body),
		Author:       data.Author.Login,
		State:        strings.ToLower(data.State),
		CreatedAt:    data.CreatedAt,
		UpdatedAt:    data.UpdatedAt,
		Draft:        data.IsDraft,
		Additions:    data.Additions,
		Deletions:    data.Deletions,
		ChangedFiles: data.ChangedFiles,
		HeadSHA:      data.HeadRef.Target.OID,
	}

	if data.ClosedAt != nil {
		pr.ClosedAt = data.ClosedAt
	}
	if data.MergedAt != nil {
		pr.MergedAt = data.MergedAt
		pr.Merged = true
	}
	if data.MergedBy != nil {
		pr.MergedBy = data.MergedBy.Login
	}

	switch data.MergeStateStatus {
	case "CLEAN":
		pr.MergeableState = "clean"
	case "UNSTABLE":
		pr.MergeableState = "unstable"
	case "BLOCKED":
		pr.MergeableState = "blocked"
	case "BEHIND":
		pr.MergeableState = "behind"
	case "DIRTY":
		pr.MergeableState = "dirty"
	default:
		pr.MergeableState = strings.ToLower(data.MergeStateStatus)
	}

	if data.Author.Login != "" {
		pr.AuthorWriteAccess = p.writeAccessFromAssociation(ctx, owner, repo, data.Author.Login, data.AuthorAssociation)
		pr.AuthorBot = isBot(data.Author)
	}

	pr.Assignees = make([]string, 0)
	for _, assignee := range data.Assignees.Nodes {
		pr.Assignees = append(pr.Assignees, assignee.Login)
	}

	for _, label := range data.Labels.Nodes {
		pr.Labels = append(pr.Labels, label.Name)
	}

	for _, node := range data.Commits.Nodes {
		pr.Commits = append(pr.Commits, node.Commit.OID)
	}

	pr.Reviewers = buildReviewersMap(data)

	return pr
}

// convertGraphQLToEventsComplete converts GraphQL data to Events.
func (p *Platform) convertGraphQLToEventsComplete(ctx context.Context, data *graphQLPullRequestComplete, owner, repo string) []types.Event {
	var events []types.Event

	events = append(events, types.Event{
		Kind:        types.EventKindPROpened,
		Timestamp:   data.CreatedAt,
		Actor:       data.Author.Login,
		Body:        types.Truncate(data.Body),
		Bot:         isBot(data.Author),
		WriteAccess: p.writeAccessFromAssociation(ctx, owner, repo, data.Author.Login, data.AuthorAssociation),
	})

	for _, node := range data.Commits.Nodes {
		event := types.Event{
			Kind:        types.EventKindCommit,
			Timestamp:   node.Commit.CommittedDate,
			Body:        node.Commit.OID,
			Description: types.Truncate(node.Commit.Message),
		}
		if node.Commit.Author.User != nil {
			event.Actor = node.Commit.Author.User.Login
			event.Bot = isBot(*node.Commit.Author.User)
		} else {
			event.Actor = node.Commit.Author.Name
		}
		events = append(events, event)
	}

	for i := range data.Reviews.Nodes {
		review := &data.Reviews.Nodes[i]
		if review.State == "" {
			continue
		}
		timestamp := review.CreatedAt
		if review.SubmittedAt != nil {
			timestamp = *review.SubmittedAt
		}
		event := types.Event{
			Kind:        types.EventKindReview,
			Timestamp:   timestamp,
			Actor:       review.Author.Login,
			Body:        types.Truncate(review.Body),
			Outcome:     strings.ToLower(review.State),
			Question:    types.ContainsQuestion(review.Body),
			Bot:         isBot(review.Author),
			WriteAccess: p.writeAccessFromAssociation(ctx, owner, repo, review.Author.Login, review.AuthorAssociation),
		}
		events = append(events, event)
	}

	for i := range data.ReviewThreads.Nodes {
		thread := &data.ReviewThreads.Nodes[i]
		for j := range thread.Comments.Nodes {
			comment := &thread.Comments.Nodes[j]
			event := types.Event{
				Kind:        types.EventKindReviewComment,
				Timestamp:   comment.CreatedAt,
				Actor:       comment.Author.Login,
				Body:        types.Truncate(comment.Body),
				Question:    types.ContainsQuestion(comment.Body),
				Bot:         isBot(comment.Author),
				WriteAccess: p.writeAccessFromAssociation(ctx, owner, repo, comment.Author.Login, comment.AuthorAssociation),
				Outdated:    comment.Outdated,
			}
			events = append(events, event)
		}
	}

	for _, comment := range data.Comments.Nodes {
		event := types.Event{
			Kind:        types.EventKindComment,
			Timestamp:   comment.CreatedAt,
			Actor:       comment.Author.Login,
			Body:        types.Truncate(comment.Body),
			Question:    types.ContainsQuestion(comment.Body),
			Bot:         isBot(comment.Author),
			WriteAccess: p.writeAccessFromAssociation(ctx, owner, repo, comment.Author.Login, comment.AuthorAssociation),
		}
		events = append(events, event)
	}

	if data.HeadRef.Target.StatusCheckRollup != nil {
		for i := range data.HeadRef.Target.StatusCheckRollup.Contexts.Nodes {
			node := &data.HeadRef.Target.StatusCheckRollup.Contexts.Nodes[i]
			switch node.TypeName {
			case "CheckRun":
				var description string
				switch {
				case node.Title != "" && node.Summary != "":
					description = fmt.Sprintf("%s: %s", node.Title, node.Summary)
				case node.Title != "":
					description = node.Title
				case node.Summary != "":
					description = node.Summary
				default:
					// No description available
				}

				if node.StartedAt != nil {
					events = append(events, types.Event{
						Kind:        types.EventKindCheckRun,
						Timestamp:   *node.StartedAt,
						Body:        node.Name,
						Outcome:     strings.ToLower(node.Status),
						Bot:         true,
						Description: description,
					})
				}

				if node.CompletedAt != nil {
					events = append(events, types.Event{
						Kind:        types.EventKindCheckRun,
						Timestamp:   *node.CompletedAt,
						Body:        node.Name,
						Outcome:     strings.ToLower(node.Conclusion),
						Bot:         true,
						Description: description,
					})
				}

			case "StatusContext":
				if node.CreatedAt == nil {
					continue
				}
				event := types.Event{
					Kind:        types.EventKindStatusCheck,
					Timestamp:   *node.CreatedAt,
					Outcome:     strings.ToLower(node.State),
					Body:        node.Context,
					Description: node.Description,
				}
				if node.Creator != nil {
					event.Actor = node.Creator.Login
					event.Bot = isBot(*node.Creator)
				}
				events = append(events, event)
			default:
				// Skip unknown status check types
			}
		}
	}

	for _, item := range data.TimelineItems.Nodes {
		event := p.parseGraphQLTimelineEvent(ctx, item, owner, repo)
		if event != nil {
			events = append(events, *event)
		}
	}

	if data.ClosedAt != nil && !data.IsDraft {
		event := types.Event{
			Kind:      types.EventKindPRClosed,
			Timestamp: *data.ClosedAt,
		}
		if data.MergedBy != nil {
			event.Actor = data.MergedBy.Login
			event.Kind = types.EventKindPRMerged
			event.Bot = isBot(*data.MergedBy)
		}
		events = append(events, event)
	}

	return events
}

// parseGraphQLTimelineEvent parses a single timeline event.
//
//nolint:gocognit,maintidx,revive // High complexity justified - must handle all GitHub timeline event types
func (*Platform) parseGraphQLTimelineEvent(_ context.Context, item map[string]any, _, _ string) *types.Event {
	typename, ok := item["__typename"].(string)
	if !ok {
		return nil
	}

	getTime := func(key string) *time.Time {
		if str, ok := item[key].(string); ok {
			if t, err := time.Parse(time.RFC3339, str); err == nil {
				return &t
			}
		}
		return nil
	}

	getActor := func() string {
		if actor, ok := item["actor"].(map[string]any); ok {
			if login, ok := actor["login"].(string); ok {
				return login
			}
		}
		return "unknown"
	}

	isActorBot := func() bool {
		if actor, ok := item["actor"].(map[string]any); ok {
			var actorObj graphQLActor
			if login, ok := actor["login"].(string); ok {
				actorObj.Login = login
			}
			if id, ok := actor["id"].(string); ok {
				actorObj.ID = id
			}
			if typ, ok := actor["__typename"].(string); ok {
				actorObj.Type = typ
			}
			return isBot(actorObj)
		}
		return false
	}

	createdAt := getTime("createdAt")
	if createdAt == nil {
		return nil
	}

	event := &types.Event{
		Timestamp: *createdAt,
		Actor:     getActor(),
		Bot:       isActorBot(),
	}

	switch typename {
	case "AssignedEvent":
		event.Kind = types.EventKindAssigned
		if assignee, ok := item["assignee"].(map[string]any); ok {
			if login, ok := assignee["login"].(string); ok {
				event.Target = login
			}
		}

	case "UnassignedEvent":
		event.Kind = types.EventKindUnassigned
		if assignee, ok := item["assignee"].(map[string]any); ok {
			if login, ok := assignee["login"].(string); ok {
				event.Target = login
			}
		}

	case "LabeledEvent":
		event.Kind = types.EventKindLabeled
		if label, ok := item["label"].(map[string]any); ok {
			if name, ok := label["name"].(string); ok {
				event.Target = name
			}
		}

	case "UnlabeledEvent":
		event.Kind = types.EventKindUnlabeled
		if label, ok := item["label"].(map[string]any); ok {
			if name, ok := label["name"].(string); ok {
				event.Target = name
			}
		}

	case "MilestonedEvent":
		event.Kind = types.EventKindMilestoned
		if title, ok := item["milestoneTitle"].(string); ok {
			event.Target = title
		}

	case "DemilestonedEvent":
		event.Kind = types.EventKindDemilestoned
		if title, ok := item["milestoneTitle"].(string); ok {
			event.Target = title
		}

	case "ReviewRequestedEvent":
		event.Kind = types.EventKindReviewRequested
		if reviewer, ok := item["requestedReviewer"].(map[string]any); ok {
			if login, ok := reviewer["login"].(string); ok {
				event.Target = login
			} else if name, ok := reviewer["name"].(string); ok {
				event.Target = name
			}
		}

	case "ReviewRequestRemovedEvent":
		event.Kind = types.EventKindReviewRequestRemoved
		if reviewer, ok := item["requestedReviewer"].(map[string]any); ok {
			if login, ok := reviewer["login"].(string); ok {
				event.Target = login
			} else if name, ok := reviewer["name"].(string); ok {
				event.Target = name
			}
		}

	case "MentionedEvent":
		event.Kind = types.EventKindMentioned
		event.Body = "User was mentioned"

	case "ReadyForReviewEvent":
		event.Kind = types.EventKindReadyForReview

	case "ConvertToDraftEvent":
		event.Kind = types.EventKindConvertToDraft

	case "ClosedEvent":
		event.Kind = types.EventKindClosed

	case "ReopenedEvent":
		event.Kind = types.EventKindReopened

	case "MergedEvent":
		event.Kind = types.EventKindMerged

	case "AutoMergeEnabledEvent":
		event.Kind = types.EventKindAutoMergeEnabled

	case "AutoMergeDisabledEvent":
		event.Kind = types.EventKindAutoMergeDisabled

	case "ReviewDismissedEvent":
		event.Kind = types.EventKindReviewDismissed
		if msg, ok := item["dismissalMessage"].(string); ok {
			event.Body = msg
		}

	case "BaseRefChangedEvent":
		event.Kind = types.EventKindBaseRefChanged

	case "BaseRefForcePushedEvent":
		event.Kind = types.EventKindBaseRefForcePushed

	case "HeadRefForcePushedEvent":
		event.Kind = types.EventKindHeadRefForcePushed

	case "HeadRefDeletedEvent":
		event.Kind = types.EventKindHeadRefDeleted

	case "HeadRefRestoredEvent":
		event.Kind = types.EventKindHeadRefRestored

	case "RenamedTitleEvent":
		event.Kind = types.EventKindRenamedTitle
		if prev, ok := item["previousTitle"].(string); ok {
			if curr, ok := item["currentTitle"].(string); ok {
				event.Body = fmt.Sprintf("Renamed from %q to %q", prev, curr)
			}
		}

	case "LockedEvent":
		event.Kind = types.EventKindLocked

	case "UnlockedEvent":
		event.Kind = types.EventKindUnlocked

	case "AddedToMergeQueueEvent":
		event.Kind = types.EventKindAddedToMergeQueue

	case "RemovedFromMergeQueueEvent":
		event.Kind = types.EventKindRemovedFromMergeQueue

	case "AutomaticBaseChangeSucceededEvent":
		event.Kind = types.EventKindAutomaticBaseChangeSucceeded

	case "AutomaticBaseChangeFailedEvent":
		event.Kind = types.EventKindAutomaticBaseChangeFailed

	case "ConnectedEvent":
		event.Kind = types.EventKindConnected

	case "DisconnectedEvent":
		event.Kind = types.EventKindDisconnected

	case "CrossReferencedEvent":
		event.Kind = types.EventKindCrossReferenced

	case "ReferencedEvent":
		event.Kind = types.EventKindReferenced

	case "SubscribedEvent":
		event.Kind = types.EventKindSubscribed

	case "UnsubscribedEvent":
		event.Kind = types.EventKindUnsubscribed

	case "DeployedEvent":
		event.Kind = types.EventKindDeployed

	case "DeploymentEnvironmentChangedEvent":
		event.Kind = types.EventKindDeploymentEnvironmentChanged

	case "PinnedEvent":
		event.Kind = types.EventKindPinned

	case "UnpinnedEvent":
		event.Kind = types.EventKindUnpinned

	case "TransferredEvent":
		event.Kind = types.EventKindTransferred

	case "UserBlockedEvent":
		event.Kind = types.EventKindUserBlocked

	default:
		return nil
	}

	return event
}

// writeAccessFromAssociation calculates write access from association.
func (p *Platform) writeAccessFromAssociation(ctx context.Context, owner, repo, user, association string) int {
	if user == "" {
		return types.WriteAccessNA
	}

	switch association {
	case "OWNER", "COLLABORATOR":
		return types.WriteAccessDefinitely
	case "MEMBER":
		return p.checkCollaboratorPermission(ctx, owner, repo, user)
	case "CONTRIBUTOR", "NONE", "FIRST_TIME_CONTRIBUTOR", "FIRST_TIMER":
		return types.WriteAccessUnlikely
	default:
		return types.WriteAccessNA
	}
}

// checkCollaboratorPermission checks if a user has write access.
func (p *Platform) checkCollaboratorPermission(ctx context.Context, owner, repo, user string) int {
	collabs, err := p.collaboratorsCache.Fetch(fmt.Sprintf("%s/%s", owner, repo), func() (map[string]string, error) {
		result, fetchErr := p.client.Collaborators(ctx, owner, repo)
		if fetchErr != nil {
			p.logger.WarnContext(ctx, "failed to fetch collaborators for write access check",
				"owner", owner,
				"repo", repo,
				"user", user,
				"error", fetchErr)
			return nil, fetchErr
		}
		return result, nil
	})
	if err != nil {
		return types.WriteAccessLikely
	}

	switch collabs[user] {
	case "admin", "maintain", "write":
		return types.WriteAccessDefinitely
	case "read", "triage", "none":
		return types.WriteAccessNo
	default:
		return types.WriteAccessUnlikely
	}
}

// extractRequiredChecksFromGraphQL gets required checks from GraphQL response.
func (*Platform) extractRequiredChecksFromGraphQL(data *graphQLPullRequestComplete) []string {
	seen := make(map[string]bool)

	if data.BaseRef.RefUpdateRule != nil {
		for _, c := range data.BaseRef.RefUpdateRule.RequiredStatusCheckContexts {
			seen[c] = true
		}
	}

	if data.BaseRef.BranchProtectionRule != nil {
		for _, c := range data.BaseRef.BranchProtectionRule.RequiredStatusCheckContexts {
			seen[c] = true
		}
	}

	checks := make([]string, 0, len(seen))
	for c := range seen {
		checks = append(checks, c)
	}
	return checks
}

// calculateTestStateFromGraphQL determines test state from check runs.
func (*Platform) calculateTestStateFromGraphQL(data *graphQLPullRequestComplete) string {
	if data.HeadRef.Target.StatusCheckRollup == nil {
		return ""
	}

	var hasFailure, hasRunning, hasQueued bool

	for i := range data.HeadRef.Target.StatusCheckRollup.Contexts.Nodes {
		node := &data.HeadRef.Target.StatusCheckRollup.Contexts.Nodes[i]
		if node.TypeName != "CheckRun" {
			continue
		}

		if !strings.Contains(strings.ToLower(node.Name), "test") &&
			!strings.Contains(strings.ToLower(node.Name), "check") &&
			!strings.Contains(strings.ToLower(node.Name), "ci") {
			continue
		}

		switch strings.ToLower(node.Status) {
		case "queued":
			hasQueued = true
		case "in_progress":
			hasRunning = true
		default:
			// Other statuses don't affect state calculation
		}

		switch strings.ToLower(node.Conclusion) {
		case "failure", "timed_out", "action_required":
			hasFailure = true
		default:
			// Other conclusions don't indicate failure
		}
	}

	if hasFailure {
		return "failing"
	}
	if hasRunning {
		return "running"
	}
	if hasQueued {
		return "queued"
	}
	return "passing"
}

// truncateSHA returns the first 7 characters of a SHA, or the full string if shorter.
func truncateSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// buildReviewersMap constructs a map of reviewer login to their review state.
func buildReviewersMap(data *graphQLPullRequestComplete) map[string]types.ReviewState {
	reviewers := make(map[string]types.ReviewState)

	for _, request := range data.ReviewRequests.Nodes {
		reviewer := request.RequestedReviewer
		if reviewer.Login != "" {
			reviewers[reviewer.Login] = types.ReviewStatePending
		} else if reviewer.Name != "" {
			reviewers[reviewer.Name] = types.ReviewStatePending
		}
	}

	for i := range data.Reviews.Nodes {
		review := &data.Reviews.Nodes[i]
		if review.Author.Login == "" {
			continue
		}

		var state types.ReviewState
		switch strings.ToUpper(review.State) {
		case "APPROVED":
			state = types.ReviewStateApproved
		case "CHANGES_REQUESTED":
			state = types.ReviewStateChangesRequested
		case "COMMENTED":
			state = types.ReviewStateCommented
		default:
			continue
		}

		reviewers[review.Author.Login] = state
	}

	return reviewers
}

// fetchRulesetsREST fetches repository rulesets via REST API.
func (p *Platform) fetchRulesetsREST(ctx context.Context, owner, repo string) ([]string, error) {
	return p.rulesetsCache.Fetch(fmt.Sprintf("%s/%s", owner, repo), func() ([]string, error) {
		path := fmt.Sprintf("/repos/%s/%s/rulesets", owner, repo)
		var rulesets []Ruleset

		if _, err := p.client.Get(ctx, path, &rulesets); err != nil {
			return nil, err
		}

		var required []string
		for _, rs := range rulesets {
			if rs.Target != "branch" {
				continue
			}
			for _, rule := range rs.Rules {
				if rule.Type == "required_status_checks" && rule.Parameters.RequiredStatusChecks != nil {
					for _, chk := range rule.Parameters.RequiredStatusChecks {
						required = append(required, chk.Context)
					}
				}
			}
		}

		p.logger.InfoContext(ctx, "fetched required checks from rulesets",
			"owner", owner, "repo", repo, "count", len(required), "checks", required)

		return required, nil
	})
}

// fetchCheckRunsREST fetches check runs via REST API for a specific commit.
func (p *Platform) fetchCheckRunsREST(ctx context.Context, owner, repo, sha string, refTime time.Time) ([]types.Event, error) {
	if sha == "" {
		return nil, nil
	}

	cacheKey := fmt.Sprintf("%s/%s/%s", owner, repo, sha)

	if cached, ok := p.checkRunsCache.Get(cacheKey); ok {
		if !cached.CachedAt.Before(refTime) {
			p.logger.InfoContext(ctx, "cache hit: check runs",
				"owner", owner, "repo", repo, "sha", truncateSHA(sha), "count", len(cached.Events))
			return cached.Events, nil
		}
		p.logger.InfoContext(ctx, "cache miss: check runs expired",
			"owner", owner, "repo", repo, "sha", truncateSHA(sha),
			"cached_at", cached.CachedAt, "reference_time", refTime)
	}

	path := fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs?per_page=100", owner, repo, sha)
	var checkRuns CheckRuns
	if _, err := p.client.Get(ctx, path, &checkRuns); err != nil {
		return nil, fmt.Errorf("fetching check runs: %w", err)
	}

	var events []types.Event
	for _, run := range checkRuns.CheckRuns {
		if run == nil {
			continue
		}

		var timestamp time.Time
		var outcome string

		switch {
		case !run.CompletedAt.IsZero():
			timestamp = run.CompletedAt
			outcome = strings.ToLower(run.Conclusion)
		case !run.StartedAt.IsZero():
			timestamp = run.StartedAt
			outcome = strings.ToLower(run.Status)
		default:
			continue
		}

		event := types.Event{
			Kind:      types.EventKindCheckRun,
			Timestamp: timestamp,
			Actor:     "github",
			Bot:       true,
			Body:      run.Name,
			Outcome:   outcome,
		}

		switch {
		case run.Output.Title != "" && run.Output.Summary != "":
			event.Description = fmt.Sprintf("%s: %s", run.Output.Title, run.Output.Summary)
		case run.Output.Title != "":
			event.Description = run.Output.Title
		case run.Output.Summary != "":
			event.Description = run.Output.Summary
		default:
			// No description available
		}

		events = append(events, event)
	}

	p.checkRunsCache.Set(cacheKey, cachedCheckRuns{
		Events:   events,
		CachedAt: time.Now(),
	})

	p.logger.InfoContext(ctx, "fetched check runs from API",
		"owner", owner, "repo", repo, "sha", truncateSHA(sha), "count", len(events))

	return events, nil
}

// fetchAllCheckRunsREST fetches check runs for all commits in the PR.
func (p *Platform) fetchAllCheckRunsREST(ctx context.Context, owner, repo string, prData *types.PullRequestData, refTime time.Time) []types.Event {
	shas := make(map[string]bool)

	if prData.PullRequest.HeadSHA != "" {
		shas[prData.PullRequest.HeadSHA] = true
	}

	for i := range prData.Events {
		e := &prData.Events[i]
		if e.Kind == types.EventKindCommit && e.Body != "" {
			shas[e.Body] = true
		}
	}

	var all []types.Event
	seen := make(map[string]bool)

	for sha := range shas {
		events, err := p.fetchCheckRunsREST(ctx, owner, repo, sha, refTime)
		if err != nil {
			p.logger.WarnContext(ctx, "failed to fetch check runs for commit", "sha", sha, "error", err)
			continue
		}

		for i := range events {
			ev := &events[i]
			key := fmt.Sprintf("%s:%s", ev.Body, ev.Timestamp.Format(time.RFC3339Nano))
			if !seen[key] {
				seen[key] = true
				ev.Target = sha
				all = append(all, *ev)
			}
		}
	}

	return all
}

// existingRequiredChecks extracts required checks that were already identified.
func (*Platform) existingRequiredChecks(prData *types.PullRequestData) []string {
	var required []string

	for i := range prData.Events {
		e := &prData.Events[i]
		if e.Required && (e.Kind == types.EventKindCheckRun || e.Kind == types.EventKindStatusCheck) {
			required = append(required, e.Body)
		}
	}

	if prData.PullRequest.CheckSummary != nil {
		for chk := range prData.PullRequest.CheckSummary.Pending {
			if !slices.Contains(required, chk) {
				required = append(required, chk)
			}
		}
	}

	return required
}

// recalculateCheckSummaryWithCheckRuns updates the check summary with REST-fetched check runs.
func (p *Platform) recalculateCheckSummaryWithCheckRuns(_ context.Context, prData *types.PullRequestData, _ []types.Event) {
	var required []string
	if prData.PullRequest.CheckSummary != nil {
		for chk := range prData.PullRequest.CheckSummary.Pending {
			required = append(required, chk)
		}
	}

	prData.PullRequest.CheckSummary = types.CalculateCheckSummary(prData.Events, required)
	prData.PullRequest.TestState = p.calculateTestStateFromCheckSummary(prData.PullRequest.CheckSummary)
}

// calculateTestStateFromCheckSummary determines test state from a CheckSummary.
func (*Platform) calculateTestStateFromCheckSummary(summary *types.CheckSummary) string {
	if summary == nil {
		return types.TestStateNone
	}

	if len(summary.Failing) > 0 {
		return types.TestStateFailing
	}

	if len(summary.Pending) > 0 {
		return types.TestStatePending
	}

	if len(summary.Success) > 0 {
		return types.TestStatePassing
	}

	return types.TestStateNone
}
