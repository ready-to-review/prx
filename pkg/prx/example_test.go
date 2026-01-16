package prx_test

import (
	"context"
	"fmt"
	"log"

	"github.com/codeGROOVE-dev/prx/pkg/prx/fetch"
)

func Example() {
	// The simplest way: just pass a URL
	// Authentication is automatically resolved from environment or CLI tools
	ctx := context.Background()
	data, err := fetch.PullRequest(ctx, "https://github.com/owner/repo/pull/123")
	if err != nil {
		log.Fatal(err)
	}

	// Show PR metadata
	fmt.Printf("PR #%d: %s\n", data.PullRequest.Number, data.PullRequest.Title)
	fmt.Printf("Author: %s (write access: %d)\n", data.PullRequest.Author, data.PullRequest.AuthorWriteAccess)
	fmt.Printf("Status: %s, Mergeable: %v\n", data.PullRequest.State, data.PullRequest.MergeableState)

	// Process events
	for i := range data.Events {
		event := &data.Events[i]
		fmt.Printf("%s: %s by %s\n",
			event.Timestamp.Format("2006-01-02 15:04:05"),
			event.Kind,
			event.Actor,
		)
	}
}

func ExamplePullRequest() {
	ctx := context.Background()

	// Works with GitHub
	data, err := fetch.PullRequest(ctx, "https://github.com/owner/repo/pull/123")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("GitHub PR #%d\n", data.PullRequest.Number)

	// Works with GitLab
	data, err = fetch.PullRequest(ctx, "https://gitlab.com/owner/repo/-/merge_requests/456")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("GitLab MR #%d\n", data.PullRequest.Number)

	// Works with Codeberg
	data, err = fetch.PullRequest(ctx, "https://codeberg.org/owner/repo/pulls/789")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Codeberg PR #%d\n", data.PullRequest.Number)

	// Works with any Gitea instance
	data, err = fetch.PullRequest(ctx, "https://gitea.example.com/owner/repo/pulls/100")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Gitea PR #%d\n", data.PullRequest.Number)

	// URL fragments and query parameters are automatically stripped
	data, err = fetch.PullRequest(ctx, "https://github.com/owner/repo/pull/123?tab=checks#issuecomment-456")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("PR #%d (cleaned URL)\n", data.PullRequest.Number)
}

func ExampleClient_PullRequest() {
	// For advanced use cases where you need to fetch multiple PRs from the same platform,
	// you can still use the client-based API
	ctx := context.Background()

	// Fetch using the simple API
	data, err := fetch.PullRequest(ctx, "https://github.com/golang/go/pull/123")
	if err != nil {
		log.Fatal(err)
	}

	// Show PR size
	fmt.Printf("PR #%d: +%d -%d in %d files\n",
		data.PullRequest.Number,
		data.PullRequest.Additions,
		data.PullRequest.Deletions,
		data.PullRequest.ChangedFiles)

	// Count events by type
	eventCounts := make(map[string]int)
	for i := range data.Events {
		eventCounts[data.Events[i].Kind]++
	}

	// Print summary
	fmt.Printf("Total events: %d\n", len(data.Events))
	for eventKind, count := range eventCounts {
		fmt.Printf("%s: %d\n", eventKind, count)
	}
}
