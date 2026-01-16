// Package gitea provides a Gitea/Codeberg platform implementation for fetching
// pull request data from Gitea-based forges.
package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/fido"
	"github.com/codeGROOVE-dev/prx/pkg/prx"
)

// Cache TTL constants.
const (
	prDataCacheTTL = 20 * 24 * time.Hour // 20 days - validity checked against reference time
)

// Cached data types with timestamps for reference time validation.
//
//nolint:govet // fieldalignment: cache structs prioritize readability over memory layout
type cachedReviews struct {
	Data     []review
	CachedAt time.Time
}

//nolint:govet // fieldalignment: cache structs prioritize readability over memory layout
type cachedComments struct {
	Data     []comment
	CachedAt time.Time
}

//nolint:govet // fieldalignment: cache structs prioritize readability over memory layout
type cachedCommits struct {
	Data     []commit
	CachedAt time.Time
}

//nolint:govet // fieldalignment: cache structs prioritize readability over memory layout
type cachedTimeline struct {
	Data     []timelineEvent
	CachedAt time.Time
}

// Platform implements the prx.Platform interface for Gitea-based forges (Codeberg, self-hosted Gitea).
//
//nolint:govet // fieldalignment: struct fields ordered for clarity
type Platform struct {
	logger        *slog.Logger
	httpClient    *http.Client
	token         string
	baseURL       string
	reviewsCache  *fido.Cache[string, cachedReviews]
	commentsCache *fido.Cache[string, cachedComments]
	commitsCache  *fido.Cache[string, cachedCommits]
	timelineCache *fido.Cache[string, cachedTimeline]
}

// Option configures a Platform.
type Option func(*Platform)

// WithLogger sets a custom logger for the Gitea platform.
func WithLogger(logger *slog.Logger) Option {
	return func(p *Platform) {
		p.logger = logger
	}
}

// WithHTTPClient sets a custom HTTP client for the Gitea platform.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Platform) {
		p.httpClient = client
	}
}

// WithBaseURL sets a custom base URL for self-hosted Gitea instances.
func WithBaseURL(baseURL string) Option {
	return func(p *Platform) {
		p.baseURL = strings.TrimSuffix(baseURL, "/")
	}
}

// NewPlatform creates a new Gitea platform client.
// For Codeberg, use NewCodebergPlatform instead.
func NewPlatform(token string, opts ...Option) *Platform {
	p := &Platform{
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		token:         token,
		baseURL:       "https://codeberg.org", // Default to Codeberg
		logger:        slog.Default(),
		reviewsCache:  fido.New[string, cachedReviews](fido.TTL(prDataCacheTTL)),
		commentsCache: fido.New[string, cachedComments](fido.TTL(prDataCacheTTL)),
		commitsCache:  fido.New[string, cachedCommits](fido.TTL(prDataCacheTTL)),
		timelineCache: fido.New[string, cachedTimeline](fido.TTL(prDataCacheTTL)),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// NewCodebergPlatform creates a new Codeberg platform client.
func NewCodebergPlatform(token string, opts ...Option) *Platform {
	return NewPlatform(token, opts...)
}

// Name returns the platform identifier.
func (p *Platform) Name() string {
	if strings.Contains(p.baseURL, "codeberg.org") {
		return prx.PlatformCodeberg
	}
	return "gitea"
}

// FetchPR retrieves a pull request with all events and metadata.
func (p *Platform) FetchPR(ctx context.Context, owner, repo string, number int, refTime time.Time) (*prx.PullRequestData, error) {
	p.logger.Info("fetching pull request via Gitea REST API",
		"owner", owner, "repo", repo, "pr", number)

	// Fetch pull request details (not cached - contains updatedAt for reference).
	pr, err := p.fetchPullRequest(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("fetch pull request: %w", err)
	}

	// Fetch reviews (cached with reference time validation).
	reviews, err := p.fetchReviews(ctx, owner, repo, number, refTime)
	if err != nil {
		p.logger.Warn("failed to fetch reviews", "error", err)
	}

	// Fetch comments (cached with reference time validation).
	comments, err := p.fetchComments(ctx, owner, repo, number, refTime)
	if err != nil {
		p.logger.Warn("failed to fetch comments", "error", err)
	}

	// Fetch commits (cached with reference time validation).
	commits, err := p.fetchCommits(ctx, owner, repo, number, refTime)
	if err != nil {
		p.logger.Warn("failed to fetch commits", "error", err)
	}

	// Fetch timeline events (cached with reference time validation).
	timeline, err := p.fetchTimeline(ctx, owner, repo, number, refTime)
	if err != nil {
		p.logger.Warn("failed to fetch timeline", "error", err)
	}

	// Convert to neutral format.
	pullRequest := convertPullRequest(pr, reviews)
	events := convertToEvents(pr, reviews, comments, commits, timeline)

	// Sort events by timestamp.
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	// Finalize with calculated summaries.
	prx.FinalizePullRequest(&pullRequest, events, nil, "")

	return &prx.PullRequestData{
		CachedAt:    time.Now(),
		PullRequest: pullRequest,
		Events:      events,
	}, nil
}

// Gitea API response prx.
// See: https://docs.gitea.com/api/1.20/

//nolint:govet // fieldalignment: struct fields ordered for JSON clarity and API compatibility
type pullRequest struct {
	User               user       `json:"user"`
	Assignee           *user      `json:"assignee"`
	MergedBy           *user      `json:"merged_by"`
	Head               branch     `json:"head"`
	Base               branch     `json:"base"`
	MergedAt           *time.Time `json:"merged_at"`
	ClosedAt           *time.Time `json:"closed_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	Assignees          []user     `json:"assignees"`
	RequestedReviewers []user     `json:"requested_reviewers"`
	Labels             []label    `json:"labels"`
	Title              string     `json:"title"`
	Body               string     `json:"body"`
	State              string     `json:"state"` // "open", "closed"
	HTMLURL            string     `json:"html_url"`
	DiffURL            string     `json:"diff_url"`
	PatchURL           string     `json:"patch_url"`
	MergeBase          string     `json:"merge_base"`
	Mergeable          bool       `json:"mergeable"`
	Merged             bool       `json:"merged"`
	ID                 int64      `json:"id"`
	Number             int        `json:"number"`
	Additions          int        `json:"additions"`
	Deletions          int        `json:"deletions"`
	ChangedFiles       int        `json:"changed_files"`
	Draft              bool       `json:"draft"`
}

type user struct {
	Login     string `json:"login"`
	FullName  string `json:"full_name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
	ID        int64  `json:"id"`
	IsAdmin   bool   `json:"is_admin"`
}

type branch struct {
	Repo *repo  `json:"repo"`
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
}

type repo struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
	ID       int64  `json:"id"`
}

type label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
	ID    int64  `json:"id"`
}

//nolint:govet // fieldalignment: JSON API structs prioritize readability over memory layout
type review struct {
	User        user      `json:"user"`
	SubmittedAt time.Time `json:"submitted_at"`
	Body        string    `json:"body"`
	State       string    `json:"state"` // "APPROVED", "REQUEST_CHANGES", "COMMENT", "PENDING"
	HTMLURL     string    `json:"html_url"`
	ID          int64     `json:"id"`
	Official    bool      `json:"official"`
	Stale       bool      `json:"stale"`
	Dismissed   bool      `json:"dismissed"`
}

//nolint:govet // fieldalignment: JSON API structs prioritize readability over memory layout
type comment struct {
	User      user      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	HTMLURL   string    `json:"html_url"`
	ID        int64     `json:"id"`
}

type commit struct {
	Author  *user      `json:"author"`
	Commit  commitInfo `json:"commit"`
	SHA     string     `json:"sha"`
	HTMLURL string     `json:"html_url"`
}

type commitInfo struct {
	Author    commitAuthor `json:"author"`
	Committer commitAuthor `json:"committer"`
	Message   string       `json:"message"`
}

type commitAuthor struct {
	Date  time.Time `json:"date"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

type timelineEvent struct {
	User      *user     `json:"user"`
	Assignee  *user     `json:"assignee"`
	Label     *label    `json:"label"`
	OldRef    string    `json:"old_ref"`
	NewRef    string    `json:"new_ref"`
	RefAction string    `json:"ref_action"`
	CreatedAt time.Time `json:"created_at"`
	Type      string    `json:"type"`
	Body      string    `json:"body"`
	ID        int64     `json:"id"`
}

// API fetch methods.

func (p *Platform) fetchPullRequest(ctx context.Context, owner, repoName string, number int) (*pullRequest, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d", p.baseURL, owner, repoName, number)

	var pr pullRequest
	if err := p.doRequest(ctx, url, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (p *Platform) fetchReviews(ctx context.Context, owner, repoName string, number int, refTime time.Time) ([]review, error) {
	cacheKey := fmt.Sprintf("%s/%s/%d/reviews", owner, repoName, number)

	if cached, ok := p.reviewsCache.Get(cacheKey); ok {
		if !cached.CachedAt.Before(refTime) {
			p.logger.DebugContext(ctx, "cache hit: reviews", "owner", owner, "repo", repoName, "pr", number, "count", len(cached.Data))
			return cached.Data, nil
		}
		p.logger.DebugContext(ctx, "cache miss: reviews expired",
			"owner", owner, "repo", repoName, "pr", number,
			"cached_at", cached.CachedAt, "reference_time", refTime)
	}

	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews", p.baseURL, owner, repoName, number)

	var reviews []review
	if err := p.doRequest(ctx, url, &reviews); err != nil {
		return nil, err
	}

	p.reviewsCache.Set(cacheKey, cachedReviews{Data: reviews, CachedAt: time.Now()})
	return reviews, nil
}

func (p *Platform) fetchComments(ctx context.Context, owner, repoName string, number int, refTime time.Time) ([]comment, error) {
	cacheKey := fmt.Sprintf("%s/%s/%d/comments", owner, repoName, number)

	if cached, ok := p.commentsCache.Get(cacheKey); ok {
		if !cached.CachedAt.Before(refTime) {
			p.logger.DebugContext(ctx, "cache hit: comments", "owner", owner, "repo", repoName, "pr", number, "count", len(cached.Data))
			return cached.Data, nil
		}
		p.logger.DebugContext(ctx, "cache miss: comments expired",
			"owner", owner, "repo", repoName, "pr", number,
			"cached_at", cached.CachedAt, "reference_time", refTime)
	}

	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%d/comments", p.baseURL, owner, repoName, number)

	var comments []comment
	if err := p.doRequest(ctx, url, &comments); err != nil {
		return nil, err
	}

	p.commentsCache.Set(cacheKey, cachedComments{Data: comments, CachedAt: time.Now()})
	return comments, nil
}

func (p *Platform) fetchCommits(ctx context.Context, owner, repoName string, number int, refTime time.Time) ([]commit, error) {
	cacheKey := fmt.Sprintf("%s/%s/%d/commits", owner, repoName, number)

	if cached, ok := p.commitsCache.Get(cacheKey); ok {
		if !cached.CachedAt.Before(refTime) {
			p.logger.DebugContext(ctx, "cache hit: commits", "owner", owner, "repo", repoName, "pr", number, "count", len(cached.Data))
			return cached.Data, nil
		}
		p.logger.DebugContext(ctx, "cache miss: commits expired",
			"owner", owner, "repo", repoName, "pr", number,
			"cached_at", cached.CachedAt, "reference_time", refTime)
	}

	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/commits", p.baseURL, owner, repoName, number)

	var commits []commit
	if err := p.doRequest(ctx, url, &commits); err != nil {
		return nil, err
	}

	p.commitsCache.Set(cacheKey, cachedCommits{Data: commits, CachedAt: time.Now()})
	return commits, nil
}

func (p *Platform) fetchTimeline(ctx context.Context, owner, repoName string, number int, refTime time.Time) ([]timelineEvent, error) {
	cacheKey := fmt.Sprintf("%s/%s/%d/timeline", owner, repoName, number)

	if cached, ok := p.timelineCache.Get(cacheKey); ok {
		if !cached.CachedAt.Before(refTime) {
			p.logger.DebugContext(ctx, "cache hit: timeline", "owner", owner, "repo", repoName, "pr", number, "count", len(cached.Data))
			return cached.Data, nil
		}
		p.logger.DebugContext(ctx, "cache miss: timeline expired",
			"owner", owner, "repo", repoName, "pr", number,
			"cached_at", cached.CachedAt, "reference_time", refTime)
	}

	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%d/timeline", p.baseURL, owner, repoName, number)

	var timeline []timelineEvent
	if err := p.doRequest(ctx, url, &timeline); err != nil {
		return nil, err
	}

	p.timelineCache.Set(cacheKey, cachedTimeline{Data: timeline, CachedAt: time.Now()})
	return timeline, nil
}

func (p *Platform) doRequest(ctx context.Context, url string, result any) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if p.token != "" {
		req.Header.Set("Authorization", "token "+p.token)
	}
	req.Header.Set("Accept", "application/json")

	p.logger.Debug("Gitea API request", "url", url)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close response body: %w", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("gitea API error: %d %s (failed to read body: %w)", resp.StatusCode, resp.Status, readErr)
		}
		return fmt.Errorf("gitea API error: %d %s: %s", resp.StatusCode, resp.Status, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

// Conversion methods.

func convertPullRequest(pr *pullRequest, reviews []review) prx.PullRequest {
	result := prx.PullRequest{
		Number:       pr.Number,
		Title:        pr.Title,
		Body:         pr.Body,
		Author:       pr.User.Login,
		State:        pr.State,
		Draft:        pr.Draft,
		CreatedAt:    pr.CreatedAt,
		UpdatedAt:    pr.UpdatedAt,
		ClosedAt:     pr.ClosedAt,
		MergedAt:     pr.MergedAt,
		Merged:       pr.Merged,
		Additions:    pr.Additions,
		Deletions:    pr.Deletions,
		ChangedFiles: pr.ChangedFiles,
	}

	// Set head SHA.
	result.HeadSHA = pr.Head.SHA

	// Set merged by.
	if pr.MergedBy != nil {
		result.MergedBy = pr.MergedBy.Login
	}

	// Set assignees.
	for _, a := range pr.Assignees {
		result.Assignees = append(result.Assignees, a.Login)
	}

	// Set labels.
	for _, l := range pr.Labels {
		result.Labels = append(result.Labels, l.Name)
	}

	// Set reviewers with their states.
	result.Reviewers = make(map[string]prx.ReviewState)
	for _, r := range pr.RequestedReviewers {
		result.Reviewers[r.Login] = prx.ReviewStatePending
	}

	// Update reviewer states from reviews.
	for i := range reviews {
		if reviews[i].Dismissed || reviews[i].Stale {
			continue
		}
		result.Reviewers[reviews[i].User.Login] = convertReviewState(reviews[i].State)
	}

	// Set mergeable state.
	result.Mergeable = &pr.Mergeable
	switch {
	case pr.Mergeable:
		result.MergeableState = "clean"
	case pr.Draft:
		result.MergeableState = "draft"
	default:
		result.MergeableState = "blocked"
	}

	return result
}

func convertReviewState(state string) prx.ReviewState {
	switch strings.ToUpper(state) {
	case "APPROVED":
		return prx.ReviewStateApproved
	case "REQUEST_CHANGES":
		return prx.ReviewStateChangesRequested
	case "COMMENT":
		return prx.ReviewStateCommented
	default:
		return prx.ReviewStatePending
	}
}

func convertToEvents(
	pr *pullRequest,
	reviews []review,
	comments []comment,
	commits []commit,
	timeline []timelineEvent,
) []prx.Event {
	var events []prx.Event

	// Add PR opened event.
	events = append(events, prx.Event{
		Timestamp: pr.CreatedAt,
		Kind:      prx.EventKindPROpened,
		Actor:     pr.User.Login,
	})

	// Add commit events.
	for i := range commits {
		actor := commits[i].Commit.Author.Name
		if commits[i].Author != nil {
			actor = commits[i].Author.Login
		}
		msg := commits[i].Commit.Message
		if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
			msg = msg[:idx]
		}
		events = append(events, prx.Event{
			Timestamp:   commits[i].Commit.Author.Date,
			Kind:        prx.EventKindCommit,
			Actor:       actor,
			Body:        commits[i].SHA[:7],
			Description: msg,
		})
	}

	// Add review events.
	for i := range reviews {
		events = append(events, prx.Event{
			Timestamp: reviews[i].SubmittedAt,
			Kind:      prx.EventKindReview,
			Actor:     reviews[i].User.Login,
			Outcome:   convertReviewOutcome(reviews[i].State),
			Body:      reviews[i].Body,
			Question:  prx.ContainsQuestion(reviews[i].Body),
			Outdated:  reviews[i].Stale || reviews[i].Dismissed,
		})
	}

	// Add comment events.
	for i := range comments {
		events = append(events, prx.Event{
			Timestamp: comments[i].CreatedAt,
			Kind:      prx.EventKindComment,
			Actor:     comments[i].User.Login,
			Body:      comments[i].Body,
			Question:  prx.ContainsQuestion(comments[i].Body),
		})
	}

	// Add timeline events.
	for i := range timeline {
		event := convertTimelineEvent(&timeline[i])
		if event != nil {
			events = append(events, *event)
		}
	}

	// Add closed/merged events.
	if pr.MergedAt != nil {
		actor := ""
		if pr.MergedBy != nil {
			actor = pr.MergedBy.Login
		}
		events = append(events, prx.Event{
			Timestamp: *pr.MergedAt,
			Kind:      prx.EventKindPRMerged,
			Actor:     actor,
		})
	} else if pr.ClosedAt != nil && pr.State == "closed" {
		events = append(events, prx.Event{
			Timestamp: *pr.ClosedAt,
			Kind:      prx.EventKindPRClosed,
		})
	}

	return events
}

func convertReviewOutcome(state string) string {
	switch strings.ToUpper(state) {
	case "APPROVED":
		return "approved"
	case "REQUEST_CHANGES":
		return "changes_requested"
	case "COMMENT":
		return "commented"
	default:
		return "pending"
	}
}

func convertTimelineEvent(event *timelineEvent) *prx.Event {
	actor := ""
	if event.User != nil {
		actor = event.User.Login
	}

	switch event.Type {
	case "label":
		if event.Label == nil {
			return nil
		}
		return &prx.Event{
			Timestamp:   event.CreatedAt,
			Kind:        prx.EventKindLabeled,
			Actor:       actor,
			Description: event.Label.Name,
		}
	case "unlabel":
		if event.Label == nil {
			return nil
		}
		return &prx.Event{
			Timestamp:   event.CreatedAt,
			Kind:        prx.EventKindUnlabeled,
			Actor:       actor,
			Description: event.Label.Name,
		}
	case "assignees":
		if event.Assignee == nil {
			return nil
		}
		return &prx.Event{
			Timestamp: event.CreatedAt,
			Kind:      prx.EventKindAssigned,
			Actor:     actor,
			Target:    event.Assignee.Login,
		}
	case "unassignees":
		if event.Assignee == nil {
			return nil
		}
		return &prx.Event{
			Timestamp: event.CreatedAt,
			Kind:      prx.EventKindUnassigned,
			Actor:     actor,
			Target:    event.Assignee.Login,
		}
	case "review_requested":
		target := ""
		if event.Assignee != nil {
			target = event.Assignee.Login
		}
		return &prx.Event{
			Timestamp: event.CreatedAt,
			Kind:      prx.EventKindReviewRequested,
			Actor:     actor,
			Target:    target,
		}
	case "review_request_removed":
		target := ""
		if event.Assignee != nil {
			target = event.Assignee.Login
		}
		return &prx.Event{
			Timestamp: event.CreatedAt,
			Kind:      prx.EventKindReviewRequestRemoved,
			Actor:     actor,
			Target:    target,
		}
	case "close":
		return &prx.Event{
			Timestamp: event.CreatedAt,
			Kind:      prx.EventKindClosed,
			Actor:     actor,
		}
	case "reopen":
		return &prx.Event{
			Timestamp: event.CreatedAt,
			Kind:      prx.EventKindReopened,
			Actor:     actor,
		}
	case "change_title":
		return &prx.Event{
			Timestamp:   event.CreatedAt,
			Kind:        prx.EventKindRenamedTitle,
			Actor:       actor,
			Description: event.Body,
		}
	case "change_ref":
		return &prx.Event{
			Timestamp:   event.CreatedAt,
			Kind:        prx.EventKindBaseRefChanged,
			Actor:       actor,
			Description: fmt.Sprintf("%s -> %s", event.OldRef, event.NewRef),
		}
	case "merge":
		return &prx.Event{
			Timestamp: event.CreatedAt,
			Kind:      prx.EventKindMerged,
			Actor:     actor,
		}
	case "comment_ref", "issue_ref", "pull_ref":
		return &prx.Event{
			Timestamp:   event.CreatedAt,
			Kind:        prx.EventKindCrossReferenced,
			Actor:       actor,
			Description: event.Body,
		}
	default:
		// Unknown timeline event type - skip it.
		return nil
	}
}

// Helper functions.
