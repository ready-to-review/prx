// Package main provides the prx command-line tool for analyzing pull requests
// from GitHub, GitLab, and Codeberg/Gitea.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/codeGROOVE-dev/fido/pkg/store/null"
	"github.com/codeGROOVE-dev/prx/pkg/prx"
	"github.com/codeGROOVE-dev/prx/pkg/prx/auth"
	"github.com/codeGROOVE-dev/prx/pkg/prx/gitea"
	"github.com/codeGROOVE-dev/prx/pkg/prx/github"
	"github.com/codeGROOVE-dev/prx/pkg/prx/gitlab"
	"github.com/codeGROOVE-dev/prx/pkg/prx/types"
)

var (
	debug            = flag.Bool("debug", false, "Enable debug logging")
	noCache          = flag.Bool("no-cache", false, "Disable caching")
	referenceTimeStr = flag.String("reference-time", "", "Reference time for cache validation (RFC3339 format)")
)

func main() {
	flag.Parse()

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "prx: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if *debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}

	if flag.NArg() != 1 {
		printUsage()
		return errors.New("expected exactly one argument (pull request URL)")
	}

	// Parse reference time if provided
	referenceTime := time.Now()
	if *referenceTimeStr != "" {
		var err error
		referenceTime, err = time.Parse(time.RFC3339, *referenceTimeStr)
		if err != nil {
			return fmt.Errorf("invalid reference time format (use RFC3339, e.g., 2025-03-16T06:18:08Z): %w", err)
		}
	}

	prURL := flag.Arg(0)

	parsed, err := types.ParseURL(prURL)
	if err != nil {
		return fmt.Errorf("invalid PR URL: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Resolve token based on platform
	resolver := auth.NewResolver()
	platform := auth.DetectPlatform(parsed.Platform)

	token, err := resolver.Resolve(ctx, platform, parsed.Host)
	// Authentication is optional for public repos on GitLab/Gitea/Codeberg
	// Only GitHub strictly requires authentication for most API calls
	tokenOptional := parsed.Platform != types.PlatformGitHub

	if err != nil && !tokenOptional {
		return fmt.Errorf("authentication failed: %w", err)
	}

	var tokenValue string
	if token != nil {
		tokenValue = token.Value
		slog.Debug("Using token", "source", token.Source, "host", token.Host)
	} else {
		slog.Debug("No authentication token found, proceeding without authentication")
	}

	// Create platform-specific client
	var prxPlatform types.Platform
	switch parsed.Platform {
	case types.PlatformGitHub:
		prxPlatform = github.NewPlatform(tokenValue)
	case types.PlatformGitLab:
		prxPlatform = gitlab.NewPlatform(tokenValue, gitlab.WithBaseURL("https://"+parsed.Host))
	case types.PlatformCodeberg:
		prxPlatform = gitea.NewCodebergPlatform(tokenValue)
	default:
		// Self-hosted Gitea
		prxPlatform = gitea.NewPlatform(tokenValue, gitea.WithBaseURL("https://"+parsed.Host))
	}

	// Configure client options
	var opts []prx.Option
	if *debug {
		opts = append(opts, prx.WithLogger(slog.Default()))
	}
	if *noCache {
		opts = append(opts, prx.WithCacheStore(null.New[string, types.PullRequestData]()))
	}

	client := prx.NewClientWithPlatform(prxPlatform, opts...)
	data, err := client.PullRequestWithReferenceTime(ctx, parsed.Owner, parsed.Repo, parsed.Number, referenceTime)
	if err != nil {
		return fmt.Errorf("failed to fetch PR data: %w", err)
	}

	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode pull request: %w", err)
	}

	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] <pull-request-url>\n\n", os.Args[0])
	fmt.Fprint(os.Stderr, "Supported platforms:\n")
	fmt.Fprint(os.Stderr, "  GitHub:   https://github.com/owner/repo/pull/123\n")
	fmt.Fprint(os.Stderr, "  GitLab:   https://gitlab.com/owner/repo/-/merge_requests/123\n")
	fmt.Fprint(os.Stderr, "  Codeberg: https://codeberg.org/owner/repo/pulls/123\n\n")
	fmt.Fprint(os.Stderr, "Authentication:\n")
	fmt.Fprint(os.Stderr, "  GitHub:   GITHUB_TOKEN env or 'gh auth login'\n")
	fmt.Fprint(os.Stderr, "  GitLab:   GITLAB_TOKEN env or 'glab auth login'\n")
	fmt.Fprint(os.Stderr, "  Codeberg: CODEBERG_TOKEN env\n")
	fmt.Fprint(os.Stderr, "  Gitea:    GITEA_TOKEN env\n\n")
	fmt.Fprint(os.Stderr, "Options:\n")
	flag.PrintDefaults()
}
