// Package fetch provides convenience functions for fetching pull requests
// with automatic platform detection and authentication resolution.
package fetch

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/codeGROOVE-dev/prx/pkg/prx"
	"github.com/codeGROOVE-dev/prx/pkg/prx/auth"
	"github.com/codeGROOVE-dev/prx/pkg/prx/gitea"
	"github.com/codeGROOVE-dev/prx/pkg/prx/github"
	"github.com/codeGROOVE-dev/prx/pkg/prx/gitlab"
	"github.com/codeGROOVE-dev/prx/pkg/prx/types"
)

// PullRequest automatically detects the platform from a PR/MR URL, resolves authentication,
// and fetches the pull request data. This is the simplest way to use prx.
//
// Example:
//
//	data, err := fetch.PullRequest(ctx, "https://github.com/owner/repo/pull/123")
//	data, err := fetch.PullRequest(ctx, "https://gitlab.com/owner/repo/-/merge_requests/456")
//	data, err := fetch.PullRequest(ctx, "https://codeberg.org/owner/repo/pulls/789")
//	data, err := fetch.PullRequest(ctx, "https://gitea.example.com/owner/repo/pulls/100")
//
// The function will:
//   - Parse the URL to detect platform, owner, repo, and PR number
//   - Automatically resolve authentication tokens from environment variables or CLI tools
//   - Create the appropriate platform client
//   - Fetch and return the pull request data
//
// Authentication is resolved in this order:
//   - GitHub: GITHUB_TOKEN, GH_TOKEN env vars, or 'gh auth token'
//   - GitLab: GITLAB_TOKEN, GL_TOKEN env vars, or 'glab config get token'
//   - Gitea/Codeberg: CODEBERG_TOKEN, GITEA_TOKEN env vars, tea config, or 'berg auth token'
//
// URL fragments (#...) and query parameters (?...) are automatically stripped.
// Unknown hosts default to Gitea unless the URL indicates GitHub or GitLab.
//
// Use WithOptions to pass additional client configuration options.
func PullRequest(ctx context.Context, url string, opts ...prx.Option) (*types.PullRequestData, error) {
	// Parse URL to get platform, owner, repo, PR number
	parsed, err := types.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	// Resolve authentication token for the platform
	resolver := auth.NewResolver()
	authPlatform := auth.DetectPlatform(parsed.Platform)
	token, err := resolver.Resolve(ctx, authPlatform, parsed.Host)
	if err != nil {
		return nil, fmt.Errorf("resolving authentication for %s: %w", parsed.Platform, err)
	}

	// Create platform-specific client
	var platform types.Platform
	switch parsed.Platform {
	case types.PlatformGitHub:
		platform = github.NewPlatform(token.Value)
	case types.PlatformGitLab:
		platform = gitlab.NewPlatform(token.Value, gitlab.WithBaseURL("https://"+parsed.Host))
	case types.PlatformCodeberg:
		platform = gitea.NewCodebergPlatform(token.Value)
	default:
		// Default to Gitea for unknown platforms
		platform = gitea.NewPlatform(token.Value, gitea.WithBaseURL("https://"+parsed.Host))
	}

	// Create client and fetch PR
	client := prx.NewClientWithPlatform(platform, opts...)
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			slog.WarnContext(ctx, "failed to close client", "error", closeErr)
		}
	}()

	return client.PullRequest(ctx, parsed.Owner, parsed.Repo, parsed.Number)
}
