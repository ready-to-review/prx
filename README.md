# prx

Go library for fetching pull request data from GitHub, GitLab, and Gitea/Codeberg with automatic platform detection, authentication resolution, and caching.

## Quick Start

```go
import "github.com/codeGROOVE-dev/prx/pkg/pr"

data, err := pr.Fetch(ctx, "https://github.com/golang/go/pull/12345")
// Works with: GitHub, GitLab, Codeberg, self-hosted instances
```
Auto-detects platform and resolves authentication from `GITHUB_TOKEN`/`GITLAB_TOKEN`/`GITEA_TOKEN` or CLI tools (`gh`, `glab`, `tea`, `berg`).

## Library Usage

```go
import (
    "github.com/codeGROOVE-dev/prx/pkg/prx"
    "github.com/codeGROOVE-dev/prx/pkg/prx/github"
)

platform := github.NewPlatform(token)
client := prx.NewClient(platform)
data, err := client.PullRequest(ctx, "owner", "repo", 123)
```

GitLab: `gitlab.NewPlatform(token, gitlab.WithBaseURL("https://gitlab.example.com"))` | Gitea: `gitea.NewPlatform(token, gitea.WithBaseURL("https://gitea.example.com"))`

Caching enabled by default. Disable: `prx.WithCacheStore(null.New[string, prx.PullRequestData]())`
