package prx

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
