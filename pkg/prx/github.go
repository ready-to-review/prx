package prx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	githubAPI = "https://api.github.com"
	// maxResponseSize limits API response size to prevent memory exhaustion
	maxResponseSize = 10 * 1024 * 1024 // 10MB
)

// GitHubAPIError represents an error response from the GitHub API.
type GitHubAPIError struct {
	StatusCode int
	Status     string
	Body       string
	URL        string
}

func (e *GitHubAPIError) Error() string {
	return fmt.Sprintf("github API error: %s", e.Status)
}


// githubClient is a client for interacting with the GitHub API.
type githubClient struct {
	client *http.Client
	token  string
	api    string
}

// newGithubClient creates a new githubClient.
func newGithubClient(client *http.Client, token string) *githubClient {
	return &githubClient{client: client, token: token, api: githubAPI}
}

// doRequest performs the common HTTP request logic for GitHub API calls
func (c *githubClient) doRequest(ctx context.Context, path string) ([]byte, *githubResponse, error) {
	apiURL := c.api + path
	slog.Info("GitHub API request starting", "method", "GET", "url", apiURL)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	start := time.Now()
	resp, err := c.client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		slog.Error("GitHub API request failed", "url", apiURL, "error", err, "elapsed", elapsed)
		return nil, nil, err
	}
	defer resp.Body.Close()

	slog.Info("GitHub API response received", "status", resp.Status, "url", apiURL, "elapsed", elapsed)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		slog.Error("GitHub API error", "status", resp.Status, "url", apiURL, "body", string(body))
		return nil, nil, &GitHubAPIError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(body),
			URL:        apiURL,
		}
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, nil, err
	}

	// Parse Link header for pagination
	nextPageNum := 0
	linkHeader := resp.Header.Get("Link")
	links := strings.Split(linkHeader, ",")
	for _, link := range links {
		parts := strings.Split(strings.TrimSpace(link), ";")
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == `rel="next"` {
			u, err := url.Parse(strings.Trim(parts[0], "<>"))
			if err == nil {
				page := u.Query().Get("page")
				nextPageNum, _ = strconv.Atoi(page)
			}
			break
		}
	}

	return data, &githubResponse{NextPage: nextPageNum}, nil
}

// get makes a GET request to the GitHub API and decodes the response into v.
func (c *githubClient) get(ctx context.Context, path string, v any) (*githubResponse, error) {
	data, resp, err := c.doRequest(ctx, path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, v); err != nil {
		return nil, err
	}

	return resp, nil
}

// raw makes a GET request to the GitHub API and returns the raw JSON response.
func (c *githubClient) raw(ctx context.Context, path string) (json.RawMessage, *githubResponse, error) {
	data, resp, err := c.doRequest(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	return json.RawMessage(data), resp, nil
}

// userPermission gets the permission level for a user on a repository.
// Returns "admin", "write", "read", or "none".
func (c *githubClient) userPermission(ctx context.Context, owner, repo, username string) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/collaborators/%s/permission", owner, repo, username)

	var permResp struct {
		Permission string `json:"permission"`
	}

	if _, err := c.get(ctx, path, &permResp); err != nil {
		// Return the error so caller can handle it appropriately
		return "", err
	}

	return permResp.Permission, nil
}

// githubResponse wraps a GitHub API response.
type githubResponse struct {
	NextPage int
}

// githubUser represents a GitHub user.
type githubUser struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

// githubCommit represents a GitHub commit.
type githubCommit struct {
	Author struct {
		Date time.Time `json:"date"`
	} `json:"author"`
	Message string `json:"message"`
}

// githubPullRequestCommit represents a commit in a pull request.
type githubPullRequestCommit struct {
	Author *githubUser  `json:"author"`
	Commit githubCommit `json:"commit"`
}

// githubComment represents a GitHub comment.
type githubComment struct {
	User              *githubUser `json:"user"`
	CreatedAt         time.Time   `json:"created_at"`
	Body              string      `json:"body"`
	AuthorAssociation string      `json:"author_association"`
}

// githubReview represents a GitHub review.
type githubReview struct {
	User              *githubUser `json:"user"`
	SubmittedAt       time.Time   `json:"submitted_at"`
	State             string      `json:"state"`
	Body              string      `json:"body"`
	AuthorAssociation string      `json:"author_association"`
}

// githubReviewComment represents a GitHub review comment.
type githubReviewComment struct {
	User              *githubUser `json:"user"`
	CreatedAt         time.Time   `json:"created_at"`
	Body              string      `json:"body"`
	AuthorAssociation string      `json:"author_association"`
}

// githubTimelineEvent represents a GitHub timeline event.
type githubTimelineEvent struct {
	Event             string      `json:"event"`
	Actor             *githubUser `json:"actor"`
	CreatedAt         time.Time   `json:"created_at"`
	AuthorAssociation string      `json:"author_association"`
	Assignee          *githubUser `json:"assignee"`
	Label             struct {
		Name string `json:"name"`
	} `json:"label"`
	Milestone struct {
		Title string `json:"title"`
	} `json:"milestone"`
	RequestedReviewer *githubUser `json:"requested_reviewer"`
	RequestedTeam     struct {
		Name string `json:"name"`
	} `json:"requested_team"`
}

// githubStatus represents a GitHub status.
type githubStatus struct {
	Context     string      `json:"context"`     // The status check name
	Description string      `json:"description"` // Optional description
	Creator     *githubUser `json:"creator"`
	CreatedAt   time.Time   `json:"created_at"`
	State       string      `json:"state"`
	TargetURL   string      `json:"target_url"`
}

// githubCheckRun represents a GitHub check run.
type githubCheckRun struct {
	Name string `json:"name"`
	App  struct {
		Owner *githubUser `json:"owner"`
	} `json:"app"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Conclusion  string    `json:"conclusion"`
	Status      string    `json:"status"`
	HTMLURL     string    `json:"html_url"`
}

// githubCheckRuns represents a list of GitHub check runs.
type githubCheckRuns struct {
	CheckRuns []*githubCheckRun `json:"check_runs"`
}

// githubPullRequest represents a GitHub pull request.
type githubPullRequest struct {
	Number    int         `json:"number"`
	Title     string      `json:"title"`
	Body      string      `json:"body"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	User      *githubUser `json:"user"`
	Merged    bool        `json:"merged"`
	MergedAt  time.Time   `json:"merged_at"`
	MergedBy  *githubUser `json:"merged_by"`
	State     string      `json:"state"`
	ClosedAt  time.Time   `json:"closed_at"`
	Head      struct {
		SHA string `json:"sha"`
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	AuthorAssociation  string        `json:"author_association"`
	Mergeable          *bool         `json:"mergeable"`           // Can be true, false, or null
	MergeableState     string        `json:"mergeable_state"`     // "clean", "dirty", "blocked", "unstable", "unknown"
	Draft              bool          `json:"draft"`               // Whether the PR is a draft
	Additions          int           `json:"additions"`           // Lines added
	Deletions          int           `json:"deletions"`           // Lines removed
	ChangedFiles       int           `json:"changed_files"`       // Number of files changed
	Commits            int           `json:"commits"`             // Number of commits
	ReviewComments     int           `json:"review_comments"`     // Number of review comments
	Comments           int           `json:"comments"`            // Number of issue comments
	Assignees          []*githubUser `json:"assignees"`           // Current assignees
	RequestedReviewers []*githubUser `json:"requested_reviewers"` // Pending reviewers
	Labels             []struct {
		Name string `json:"name"`
	} `json:"labels"` // PR labels
}
