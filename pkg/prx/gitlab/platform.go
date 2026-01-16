// Package gitlab provides a GitLab platform implementation for fetching
// merge request data from GitLab instances.
package gitlab

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
	mrDataCacheTTL = 20 * 24 * time.Hour // 20 days - validity checked against reference time
)

// Cached data types with timestamps for reference time validation.
type cachedApprovals struct {
	Data     *approvals
	CachedAt time.Time
}

//nolint:govet // fieldalignment: cache structs prioritize readability over memory layout
type cachedPipelines struct {
	Data     []pipeline
	CachedAt time.Time
}

//nolint:govet // fieldalignment: cache structs prioritize readability over memory layout
type cachedNotes struct {
	Data     []note
	CachedAt time.Time
}

//nolint:govet // fieldalignment: cache structs prioritize readability over memory layout
type cachedDiscussions struct {
	Data     []discussion
	CachedAt time.Time
}

//nolint:govet // fieldalignment: cache structs prioritize readability over memory layout
type cachedCommits struct {
	Data     []commit
	CachedAt time.Time
}

// Platform implements the prx.Platform interface for GitLab.
//
//nolint:govet // fieldalignment: struct fields ordered for clarity
type Platform struct {
	logger           *slog.Logger
	httpClient       *http.Client
	token            string
	baseURL          string
	approvalsCache   *fido.Cache[string, cachedApprovals]
	pipelinesCache   *fido.Cache[string, cachedPipelines]
	notesCache       *fido.Cache[string, cachedNotes]
	discussionsCache *fido.Cache[string, cachedDiscussions]
	commitsCache     *fido.Cache[string, cachedCommits]
}

// Option configures a Platform.
type Option func(*Platform)

// WithLogger sets a custom logger for the GitLab platform.
func WithLogger(logger *slog.Logger) Option {
	return func(p *Platform) {
		p.logger = logger
	}
}

// WithHTTPClient sets a custom HTTP client for the GitLab platform.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Platform) {
		p.httpClient = client
	}
}

// WithBaseURL sets a custom base URL for self-hosted GitLab instances.
func WithBaseURL(baseURL string) Option {
	return func(p *Platform) {
		p.baseURL = strings.TrimSuffix(baseURL, "/")
	}
}

// NewPlatform creates a new GitLab platform client.
func NewPlatform(token string, opts ...Option) *Platform {
	p := &Platform{
		httpClient:       &http.Client{Timeout: 30 * time.Second},
		token:            token,
		baseURL:          "https://gitlab.com",
		logger:           slog.Default(),
		approvalsCache:   fido.New[string, cachedApprovals](fido.TTL(mrDataCacheTTL)),
		pipelinesCache:   fido.New[string, cachedPipelines](fido.TTL(mrDataCacheTTL)),
		notesCache:       fido.New[string, cachedNotes](fido.TTL(mrDataCacheTTL)),
		discussionsCache: fido.New[string, cachedDiscussions](fido.TTL(mrDataCacheTTL)),
		commitsCache:     fido.New[string, cachedCommits](fido.TTL(mrDataCacheTTL)),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Name returns the platform identifier.
func (*Platform) Name() string {
	return prx.PlatformGitLab
}

// FetchPR retrieves a merge request with all events and metadata.
func (p *Platform) FetchPR(ctx context.Context, owner, repo string, number int, refTime time.Time) (*prx.PullRequestData, error) {
	projectPath := fmt.Sprintf("%s/%s", owner, repo)
	p.logger.Info("fetching merge request via GitLab REST API",
		"project", projectPath, "mr", number)

	// Fetch merge request details (not cached - contains updatedAt for reference).
	mr, err := p.fetchMergeRequest(ctx, projectPath, number)
	if err != nil {
		return nil, fmt.Errorf("fetch merge request: %w", err)
	}

	// Fetch approvals (cached with reference time validation).
	approvals, err := p.fetchApprovals(ctx, projectPath, number, refTime)
	if err != nil {
		p.logger.Warn("failed to fetch approvals", "error", err)
	}

	// Fetch pipelines (cached with reference time validation).
	pipelines, err := p.fetchPipelines(ctx, projectPath, number, refTime)
	if err != nil {
		p.logger.Warn("failed to fetch pipelines", "error", err)
	}

	// Fetch notes (cached with reference time validation).
	notes, err := p.fetchNotes(ctx, projectPath, number, refTime)
	if err != nil {
		p.logger.Warn("failed to fetch notes", "error", err)
	}

	// Fetch discussions (cached with reference time validation).
	discussions, err := p.fetchDiscussions(ctx, projectPath, number, refTime)
	if err != nil {
		p.logger.Warn("failed to fetch discussions", "error", err)
	}

	// Fetch commits (cached with reference time validation).
	commits, err := p.fetchCommits(ctx, projectPath, number, refTime)
	if err != nil {
		p.logger.Warn("failed to fetch commits", "error", err)
	}

	// Convert to our neutral format.
	pr := convertMergeRequest(mr, approvals, commits)
	events := convertToEvents(mr, notes, discussions, pipelines, commits)

	// Sort events by timestamp.
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	// Finalize the pull request with calculated summaries.
	// Pass the TestState we derived from the pipeline so it doesn't get overwritten.
	prx.FinalizePullRequest(&pr, events, nil, pr.TestState)

	return &prx.PullRequestData{
		CachedAt:    time.Now(),
		PullRequest: pr,
		Events:      events,
	}, nil
}

// GitLab API response prx.

//nolint:govet // fieldalignment: JSON API structs prioritize readability over memory layout
type mergeRequest struct {
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
	Author               user            `json:"author"`
	MergedAt             *time.Time      `json:"merged_at"`
	ClosedAt             *time.Time      `json:"closed_at"`
	MergedBy             *user           `json:"merged_by"`
	DiffRefs             *diffRefs       `json:"diff_refs"`
	HeadPipeline         *pipeline       `json:"head_pipeline"`
	Assignees            []user          `json:"assignees"`
	Reviewers            []reviewerState `json:"reviewers"`
	Labels               []string        `json:"labels"`
	Title                string          `json:"title"`
	Description          string          `json:"description"`
	State                string          `json:"state"`
	MergeStatus          string          `json:"merge_status"`
	DetailedMergeStatus  string          `json:"detailed_merge_status"`
	SHA                  string          `json:"sha"`
	ChangesCount         string          `json:"changes_count"`
	SourceBranch         string          `json:"source_branch"`
	TargetBranch         string          `json:"target_branch"`
	WebURL               string          `json:"web_url"`
	ID                   int             `json:"id"`
	IID                  int             `json:"iid"`
	UserNotesCount       int             `json:"user_notes_count"`
	Draft                bool            `json:"draft"`
	WorkInProgress       bool            `json:"work_in_progress"`
	HasConflicts         bool            `json:"has_conflicts"`
	MergeableDiscussions bool            `json:"blocking_discussions_resolved"`
}

type user struct {
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
	ID        int    `json:"id"`
}

//nolint:govet // fieldalignment: JSON API structs prioritize readability over memory layout
type reviewerState struct {
	user

	State     string     `json:"state"`
	CreatedAt *time.Time `json:"created_at"`
}

type diffRefs struct {
	BaseSHA  string `json:"base_sha"`
	HeadSHA  string `json:"head_sha"`
	StartSHA string `json:"start_sha"`
}

type pipeline struct {
	DetailedStatus *detailedStatus `json:"detailed_status"`
	StartedAt      *time.Time      `json:"started_at"`
	FinishedAt     *time.Time      `json:"finished_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Status         string          `json:"status"`
	Source         string          `json:"source"`
	Ref            string          `json:"ref"`
	SHA            string          `json:"sha"`
	WebURL         string          `json:"web_url"`
	ID             int             `json:"id"`
}

type detailedStatus struct {
	Icon        string `json:"icon"`
	Text        string `json:"text"`
	Label       string `json:"label"`
	Group       string `json:"group"`
	Tooltip     string `json:"tooltip"`
	DetailsPath string `json:"details_path"`
	HasDetails  bool   `json:"has_details"`
}

type approvals struct {
	ApprovedBy         []approvalUser `json:"approved_by"`
	SuggestedApprovers []user         `json:"suggested_approvers"`
	Approvers          []user         `json:"approvers"`
	ApprovalsLeft      int            `json:"approvals_left"`
	ApprovalsRequired  int            `json:"approvals_required"`
	Approved           bool           `json:"approved"`
}

type approvalUser struct {
	User user `json:"user"`
}

//nolint:govet // fieldalignment: JSON API structs prioritize readability over memory layout
type note struct {
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Author       user      `json:"author"`
	ResolvedBy   *user     `json:"resolved_by"`
	Body         string    `json:"body"`
	NoteableType string    `json:"noteable_type"` //nolint:misspell // GitLab API uses "noteable"
	ID           int       `json:"id"`
	NoteableID   int       `json:"noteable_id"` //nolint:misspell // GitLab API uses "noteable"
	System       bool      `json:"system"`
	Resolvable   bool      `json:"resolvable"`
	Resolved     bool      `json:"resolved"`
}

//nolint:govet // fieldalignment: JSON API structs prioritize readability over memory layout
type discussion struct {
	Notes          []note `json:"notes"`
	ID             string `json:"id"`
	IndividualNote bool   `json:"individual_note"`
}

type commit struct {
	ID             string    `json:"id"`
	ShortID        string    `json:"short_id"`
	Title          string    `json:"title"`
	Message        string    `json:"message"`
	AuthorName     string    `json:"author_name"`
	AuthorEmail    string    `json:"author_email"`
	AuthoredDate   time.Time `json:"authored_date"`
	CommitterName  string    `json:"committer_name"`
	CommitterEmail string    `json:"committer_email"`
	CommittedDate  time.Time `json:"committed_date"`
	WebURL         string    `json:"web_url"`
}

// API fetch methods.

func (p *Platform) fetchMergeRequest(ctx context.Context, project string, mrIID int) (*mergeRequest, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d",
		p.baseURL, urlEncode(project), mrIID)

	var mr mergeRequest
	if err := p.doRequest(ctx, url, &mr); err != nil {
		return nil, err
	}
	return &mr, nil
}

func (p *Platform) fetchApprovals(ctx context.Context, project string, mrIID int, refTime time.Time) (*approvals, error) {
	cacheKey := fmt.Sprintf("%s/%d/approvals", project, mrIID)

	if cached, ok := p.approvalsCache.Get(cacheKey); ok {
		if !cached.CachedAt.Before(refTime) {
			p.logger.DebugContext(ctx, "cache hit: approvals", "project", project, "mr", mrIID)
			return cached.Data, nil
		}
		p.logger.DebugContext(ctx, "cache miss: approvals expired",
			"project", project, "mr", mrIID,
			"cached_at", cached.CachedAt, "reference_time", refTime)
	}

	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/approvals",
		p.baseURL, urlEncode(project), mrIID)

	var a approvals
	if err := p.doRequest(ctx, url, &a); err != nil {
		return nil, err
	}

	p.approvalsCache.Set(cacheKey, cachedApprovals{Data: &a, CachedAt: time.Now()})
	return &a, nil
}

func (p *Platform) fetchPipelines(ctx context.Context, project string, mrIID int, refTime time.Time) ([]pipeline, error) {
	cacheKey := fmt.Sprintf("%s/%d/pipelines", project, mrIID)

	if cached, ok := p.pipelinesCache.Get(cacheKey); ok {
		if !cached.CachedAt.Before(refTime) {
			p.logger.DebugContext(ctx, "cache hit: pipelines", "project", project, "mr", mrIID, "count", len(cached.Data))
			return cached.Data, nil
		}
		p.logger.DebugContext(ctx, "cache miss: pipelines expired",
			"project", project, "mr", mrIID,
			"cached_at", cached.CachedAt, "reference_time", refTime)
	}

	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/pipelines",
		p.baseURL, urlEncode(project), mrIID)

	var pipelines []pipeline
	if err := p.doRequest(ctx, url, &pipelines); err != nil {
		return nil, err
	}

	p.pipelinesCache.Set(cacheKey, cachedPipelines{Data: pipelines, CachedAt: time.Now()})
	return pipelines, nil
}

func (p *Platform) fetchNotes(ctx context.Context, project string, mrIID int, refTime time.Time) ([]note, error) {
	cacheKey := fmt.Sprintf("%s/%d/notes", project, mrIID)

	if cached, ok := p.notesCache.Get(cacheKey); ok {
		if !cached.CachedAt.Before(refTime) {
			p.logger.DebugContext(ctx, "cache hit: notes", "project", project, "mr", mrIID, "count", len(cached.Data))
			return cached.Data, nil
		}
		p.logger.DebugContext(ctx, "cache miss: notes expired",
			"project", project, "mr", mrIID,
			"cached_at", cached.CachedAt, "reference_time", refTime)
	}

	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes?sort=asc&per_page=100",
		p.baseURL, urlEncode(project), mrIID)

	var notes []note
	if err := p.doRequest(ctx, url, &notes); err != nil {
		return nil, err
	}

	p.notesCache.Set(cacheKey, cachedNotes{Data: notes, CachedAt: time.Now()})
	return notes, nil
}

func (p *Platform) fetchDiscussions(ctx context.Context, project string, mrIID int, refTime time.Time) ([]discussion, error) {
	cacheKey := fmt.Sprintf("%s/%d/discussions", project, mrIID)

	if cached, ok := p.discussionsCache.Get(cacheKey); ok {
		if !cached.CachedAt.Before(refTime) {
			p.logger.DebugContext(ctx, "cache hit: discussions", "project", project, "mr", mrIID, "count", len(cached.Data))
			return cached.Data, nil
		}
		p.logger.DebugContext(ctx, "cache miss: discussions expired",
			"project", project, "mr", mrIID,
			"cached_at", cached.CachedAt, "reference_time", refTime)
	}

	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/discussions?per_page=100",
		p.baseURL, urlEncode(project), mrIID)

	var discussions []discussion
	if err := p.doRequest(ctx, url, &discussions); err != nil {
		return nil, err
	}

	p.discussionsCache.Set(cacheKey, cachedDiscussions{Data: discussions, CachedAt: time.Now()})
	return discussions, nil
}

func (p *Platform) fetchCommits(ctx context.Context, project string, mrIID int, refTime time.Time) ([]commit, error) {
	cacheKey := fmt.Sprintf("%s/%d/commits", project, mrIID)

	if cached, ok := p.commitsCache.Get(cacheKey); ok {
		if !cached.CachedAt.Before(refTime) {
			p.logger.DebugContext(ctx, "cache hit: commits", "project", project, "mr", mrIID, "count", len(cached.Data))
			return cached.Data, nil
		}
		p.logger.DebugContext(ctx, "cache miss: commits expired",
			"project", project, "mr", mrIID,
			"cached_at", cached.CachedAt, "reference_time", refTime)
	}

	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/commits",
		p.baseURL, urlEncode(project), mrIID)

	var commits []commit
	if err := p.doRequest(ctx, url, &commits); err != nil {
		return nil, err
	}

	p.commitsCache.Set(cacheKey, cachedCommits{Data: commits, CachedAt: time.Now()})
	return commits, nil
}

func (p *Platform) doRequest(ctx context.Context, url string, result any) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if p.token != "" {
		// Support both PAT (Private-Token) and OAuth2 (Bearer) tokens
		// OAuth2 tokens from glab are typically longer and don't start with "glpat-"
		if strings.HasPrefix(p.token, "glpat-") {
			req.Header.Set("Private-Token", p.token)
		} else {
			req.Header.Set("Authorization", "Bearer "+p.token)
		}
	}
	req.Header.Set("Accept", "application/json")

	p.logger.Debug("GitLab API request", "url", url)

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
			return fmt.Errorf("gitlab API error: %d %s (failed to read body: %w)", resp.StatusCode, resp.Status, readErr)
		}
		return fmt.Errorf("gitlab API error: %d %s: %s", resp.StatusCode, resp.Status, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

// Conversion methods.

func convertMergeRequest(mr *mergeRequest, approvals *approvals, commits []commit) prx.PullRequest {
	pr := prx.PullRequest{
		Number:    mr.IID,
		Title:     mr.Title,
		Body:      mr.Description,
		Author:    mr.Author.Username,
		State:     convertState(mr.State),
		Draft:     mr.Draft || mr.WorkInProgress,
		CreatedAt: mr.CreatedAt,
		UpdatedAt: mr.UpdatedAt,
		ClosedAt:  mr.ClosedAt,
		MergedAt:  mr.MergedAt,
		Labels:    mr.Labels,
		AuthorBot: isBot(&mr.Author),
	}

	// Populate commits list (oldest to newest).
	for i := range commits {
		pr.Commits = append(pr.Commits, commits[i].ShortID)
	}

	// Set merged status.
	if mr.MergedAt != nil {
		pr.Merged = true
		if mr.MergedBy != nil {
			pr.MergedBy = mr.MergedBy.Username
		}
	}

	// Set head SHA.
	if mr.DiffRefs != nil {
		pr.HeadSHA = mr.DiffRefs.HeadSHA
	} else {
		pr.HeadSHA = mr.SHA
	}

	// Set assignees.
	for _, a := range mr.Assignees {
		pr.Assignees = append(pr.Assignees, a.Username)
	}

	// Set reviewers with their states.
	pr.Reviewers = make(map[string]prx.ReviewState)
	for _, r := range mr.Reviewers {
		pr.Reviewers[r.Username] = convertReviewerState(r.State)
	}

	// Update reviewer states from approvals.
	if approvals != nil {
		for _, ab := range approvals.ApprovedBy {
			pr.Reviewers[ab.User.Username] = prx.ReviewStateApproved
		}
	}

	// Set mergeable state.
	pr.MergeableState = convertMergeStatus(mr)
	mergeable := !mr.HasConflicts && mr.MergeStatus == "can_be_merged"
	pr.Mergeable = &mergeable

	// Derive TestState from head pipeline.
	if mr.HeadPipeline != nil {
		pr.TestState = convertPipelineToTestState(mr.HeadPipeline.Status)
	}

	return pr
}

// isBot returns true if the user appears to be a bot.
func isBot(u *user) bool {
	if u == nil {
		return false
	}
	// GitLab bot users typically have "[bot]" suffix or specific usernames
	return strings.HasSuffix(u.Username, "[bot]") ||
		strings.HasSuffix(u.Username, "-bot") ||
		u.Username == "ghost" // GitLab's placeholder for deleted users
}

// convertPipelineToTestState converts a GitLab pipeline status to TestState.
func convertPipelineToTestState(status string) string {
	switch status {
	case "success":
		return prx.TestStatePassing
	case "failed":
		return prx.TestStateFailing
	case "running":
		return prx.TestStateRunning
	case "pending", "waiting_for_resource", "preparing":
		return prx.TestStatePending
	case "created", "scheduled":
		return prx.TestStateQueued
	default:
		// canceled, skipped, manual, or unknown status
		return prx.TestStateNone
	}
}

func convertState(state string) string {
	switch state {
	case "opened":
		return "open"
	case "closed", "merged":
		return "closed"
	default:
		return state
	}
}

func convertReviewerState(state string) prx.ReviewState {
	switch state {
	case "reviewed":
		return prx.ReviewStateCommented
	default:
		return prx.ReviewStatePending
	}
}

func convertMergeStatus(mr *mergeRequest) string {
	// Map GitLab detailed merge status to our neutral format.
	switch mr.DetailedMergeStatus {
	case "mergeable":
		return "clean"
	case "ci_must_pass", "ci_still_running", "discussions_not_resolved", "not_approved":
		return "blocked"
	case "conflict", "need_rebase":
		return "dirty"
	case "checking":
		return "unknown"
	case "draft_status":
		return "draft"
	default:
		// Fall back to simple merge status.
		switch mr.MergeStatus {
		case "can_be_merged":
			return "clean"
		case "cannot_be_merged":
			return "dirty"
		default:
			return "unknown"
		}
	}
}

func convertToEvents(
	mr *mergeRequest,
	notes []note,
	discussions []discussion,
	pipelines []pipeline,
	commits []commit,
) []prx.Event {
	var events []prx.Event

	// Add MR opened event.
	events = append(events, prx.Event{
		Timestamp: mr.CreatedAt,
		Kind:      prx.EventKindPROpened,
		Actor:     mr.Author.Username,
	})

	// Add commit events.
	for i := range commits {
		events = append(events, prx.Event{
			Timestamp:   commits[i].AuthoredDate,
			Kind:        prx.EventKindCommit,
			Actor:       commits[i].AuthorName,
			Body:        commits[i].ShortID,
			Description: commits[i].Title,
		})
	}

	// Track which notes are part of discussions to avoid duplicates.
	discussionNoteIDs := make(map[int]bool)
	for i := range discussions {
		for j := range discussions[i].Notes {
			discussionNoteIDs[discussions[i].Notes[j].ID] = true
		}
	}

	// Add events from notes (comments and system events).
	for i := range notes {
		// Skip notes that are part of discussions (we'll handle them separately).
		if discussionNoteIDs[notes[i].ID] {
			continue
		}

		event := convertNote(&notes[i])
		if event != nil {
			events = append(events, *event)
		}
	}

	// Add events from discussions.
	for i := range discussions {
		for j := range discussions[i].Notes {
			event := convertNote(&discussions[i].Notes[j])
			if event != nil {
				// Mark resolved discussions.
				if discussions[i].Notes[j].Resolved {
					event.Outdated = true
				}
				events = append(events, *event)
			}
		}
	}

	// Add pipeline events (CI status).
	for i := range pipelines {
		events = append(events, convertPipeline(&pipelines[i])...)
	}

	// Add closed/merged events.
	if mr.MergedAt != nil {
		events = append(events, prx.Event{
			Timestamp: *mr.MergedAt,
			Kind:      prx.EventKindPRMerged,
			Actor:     safeUsername(mr.MergedBy),
		})
	} else if mr.ClosedAt != nil && mr.State == "closed" {
		events = append(events, prx.Event{
			Timestamp: *mr.ClosedAt,
			Kind:      prx.EventKindPRClosed,
		})
	}

	return events
}

func convertNote(n *note) *prx.Event {
	if n.System {
		return convertSystemNote(n)
	}

	// Regular user comment.
	return &prx.Event{
		Timestamp: n.CreatedAt,
		Kind:      prx.EventKindComment,
		Actor:     n.Author.Username,
		Body:      n.Body,
		Question:  prx.ContainsQuestion(n.Body),
		Outdated:  n.Resolved,
	}
}

func convertSystemNote(systemNote *note) *prx.Event {
	body := strings.ToLower(systemNote.Body)

	// Map GitLab system notes to our event kinds.
	switch {
	case strings.HasPrefix(body, "approved this merge request"):
		return &prx.Event{
			Timestamp: systemNote.CreatedAt,
			Kind:      prx.EventKindReview,
			Actor:     systemNote.Author.Username,
			Outcome:   "approved",
		}
	case strings.HasPrefix(body, "unapproved this merge request"):
		return &prx.Event{
			Timestamp: systemNote.CreatedAt,
			Kind:      prx.EventKindReview,
			Actor:     systemNote.Author.Username,
			Outcome:   "dismissed",
		}
	case strings.HasPrefix(body, "requested review from"):
		target := extractMentionFromNote(systemNote.Body)
		return &prx.Event{
			Timestamp: systemNote.CreatedAt,
			Kind:      prx.EventKindReviewRequested,
			Actor:     systemNote.Author.Username,
			Target:    target,
		}
	case strings.HasPrefix(body, "assigned to"):
		target := extractMentionFromNote(systemNote.Body)
		return &prx.Event{
			Timestamp: systemNote.CreatedAt,
			Kind:      prx.EventKindAssigned,
			Actor:     systemNote.Author.Username,
			Target:    target,
		}
	case strings.HasPrefix(body, "unassigned"):
		return &prx.Event{
			Timestamp: systemNote.CreatedAt,
			Kind:      prx.EventKindUnassigned,
			Actor:     systemNote.Author.Username,
		}
	case strings.HasPrefix(body, "added") && strings.Contains(body, "label"):
		return &prx.Event{
			Timestamp:   systemNote.CreatedAt,
			Kind:        prx.EventKindLabeled,
			Actor:       systemNote.Author.Username,
			Description: systemNote.Body,
		}
	case strings.HasPrefix(body, "removed") && strings.Contains(body, "label"):
		return &prx.Event{
			Timestamp:   systemNote.CreatedAt,
			Kind:        prx.EventKindUnlabeled,
			Actor:       systemNote.Author.Username,
			Description: systemNote.Body,
		}
	case strings.HasPrefix(body, "marked as a draft"):
		return &prx.Event{
			Timestamp: systemNote.CreatedAt,
			Kind:      prx.EventKindConvertToDraft,
			Actor:     systemNote.Author.Username,
		}
	case strings.HasPrefix(body, "marked this merge request as ready"):
		return &prx.Event{
			Timestamp: systemNote.CreatedAt,
			Kind:      prx.EventKindReadyForReview,
			Actor:     systemNote.Author.Username,
		}
	case strings.HasPrefix(body, "changed target branch"):
		return &prx.Event{
			Timestamp:   systemNote.CreatedAt,
			Kind:        prx.EventKindBaseRefChanged,
			Actor:       systemNote.Author.Username,
			Description: systemNote.Body,
		}
	case strings.HasPrefix(body, "mentioned in"):
		return &prx.Event{
			Timestamp:   systemNote.CreatedAt,
			Kind:        prx.EventKindCrossReferenced,
			Actor:       systemNote.Author.Username,
			Description: systemNote.Body,
		}
	case strings.HasPrefix(body, "closed"):
		return &prx.Event{
			Timestamp: systemNote.CreatedAt,
			Kind:      prx.EventKindClosed,
			Actor:     systemNote.Author.Username,
		}
	case strings.HasPrefix(body, "reopened"):
		return &prx.Event{
			Timestamp: systemNote.CreatedAt,
			Kind:      prx.EventKindReopened,
			Actor:     systemNote.Author.Username,
		}
	case strings.HasPrefix(body, "changed title"):
		return &prx.Event{
			Timestamp:   systemNote.CreatedAt,
			Kind:        prx.EventKindRenamedTitle,
			Actor:       systemNote.Author.Username,
			Description: systemNote.Body,
		}
	default:
		// Unknown system note - include as a generic comment.
		return &prx.Event{
			Timestamp:   systemNote.CreatedAt,
			Kind:        prx.EventKindComment,
			Actor:       systemNote.Author.Username,
			Body:        systemNote.Body,
			Description: "system",
		}
	}
}

func convertPipeline(p *pipeline) []prx.Event {
	var events []prx.Event

	// Add pipeline started event.
	if p.StartedAt != nil {
		status := "pending"
		if p.Status == "running" {
			status = "running"
		}
		events = append(events, prx.Event{
			Timestamp:   *p.StartedAt,
			Kind:        prx.EventKindCheckRun,
			Body:        fmt.Sprintf("pipeline-%d", p.ID),
			Outcome:     status,
			Bot:         true,
			Description: getDetailedStatusText(p.DetailedStatus),
		})
	}

	// Add pipeline completed event.
	if p.FinishedAt != nil {
		outcome := convertPipelineStatus(p.Status)
		events = append(events, prx.Event{
			Timestamp:   *p.FinishedAt,
			Kind:        prx.EventKindCheckRun,
			Body:        fmt.Sprintf("pipeline-%d", p.ID),
			Outcome:     outcome,
			Bot:         true,
			Description: getDetailedStatusText(p.DetailedStatus),
		})
	}

	return events
}

func convertPipelineStatus(status string) string {
	switch status {
	case "success":
		return "success"
	case "failed":
		return "failure"
	case "canceled", "cancelled":
		return "cancelled"
	case "skipped":
		return "skipped"
	case "running":
		return "running"
	case "pending", "created", "waiting_for_resource", "preparing":
		return "pending"
	case "manual":
		return "action_required"
	default:
		return status
	}
}

// Helper functions.

func urlEncode(s string) string {
	// URL encode the project path (e.g., "group/project" -> "group%2Fproject").
	return strings.ReplaceAll(s, "/", "%2F")
}

func extractMentionFromNote(body string) string {
	// Extract @username from note body.
	for p := range strings.FieldsSeq(body) {
		if username, found := strings.CutPrefix(p, "@"); found {
			return username
		}
	}
	return ""
}

func safeUsername(u *user) string {
	if u == nil {
		return ""
	}
	return u.Username
}

func getDetailedStatusText(ds *detailedStatus) string {
	if ds == nil {
		return ""
	}
	return ds.Text
}
