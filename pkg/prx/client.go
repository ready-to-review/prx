// Package prx provides a client for fetching pull request events from code hosting platforms.
// It supports GitHub, GitLab, and Codeberg (Gitea), with caching to improve performance
// and reduce API rate limit consumption.
package prx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/fido"
	"github.com/codeGROOVE-dev/fido/pkg/store/localfs"
)

const (
	// Cache TTL constants.
	prCacheTTL            = 20 * 24 * time.Hour // 20 days - validity checked against reference time
	collaboratorsCacheTTL = 3 * time.Hour       // 3 hours - repo-level, simple TTL
	rulesetsCacheTTL      = 3 * time.Hour       // 3 hours - repo-level, simple TTL
)

// PRStore is the interface for PR cache storage backends.
// This is an alias for fido.Store with the appropriate type parameters.
type PRStore = fido.Store[string, PullRequestData]

// Client provides methods to fetch pull request events from various platforms.
type Client struct {
	platform Platform
	logger   *slog.Logger
	prCache  *fido.TieredCache[string, PullRequestData]
}

// Option is a function that configures a Client.
type Option func(*Client)

// WithLogger sets a custom logger for the client.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithCacheStore sets a custom cache store for PR data.
// Use null.New[string, prx.PullRequestData]() to disable persistence.
func WithCacheStore(store PRStore) Option {
	return func(c *Client) {
		prCache, err := fido.NewTiered(store, fido.TTL(prCacheTTL))
		if err != nil {
			c.logger.Warn("failed to create cache from store, using default", "error", err)
			return
		}
		c.prCache = prCache
	}
}

// NewClient creates a new Client with the given platform.
// For GitHub: NewClient(github.NewPlatform(token), opts...)
// For GitLab: NewClient(gitlab.NewPlatform(token), opts...)
// For Gitea:  NewClient(gitea.NewPlatform(token), opts...)
func NewClient(platform Platform, opts ...Option) *Client {
	c := &Client{
		platform: platform,
		logger:   slog.Default(),
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.prCache == nil {
		c.prCache = createDefaultCache(c.logger)
	}

	return c
}

func createDefaultCache(log *slog.Logger) *fido.TieredCache[string, PullRequestData] {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "prx")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Warn("failed to create cache directory, caching disabled", "error", err)
		return nil
	}
	store, err := localfs.New[string, PullRequestData]("prx-pr", dir)
	if err != nil {
		log.Warn("failed to create cache store, caching disabled", "error", err)
		return nil
	}
	cache, err := fido.NewTiered(store, fido.TTL(prCacheTTL))
	if err != nil {
		log.Warn("failed to create cache, caching disabled", "error", err)
		return nil
	}
	return cache
}

// PullRequest fetches a pull request with all its events and metadata.
func (c *Client) PullRequest(ctx context.Context, owner, repo string, prNumber int) (*PullRequestData, error) {
	return c.PullRequestWithReferenceTime(ctx, owner, repo, prNumber, time.Now())
}

// PullRequestWithReferenceTime fetches a pull request using the given reference time for caching decisions.
func (c *Client) PullRequestWithReferenceTime(
	ctx context.Context,
	owner, repo string,
	pr int,
	refTime time.Time,
) (*PullRequestData, error) {
	if c.prCache == nil {
		return c.platform.FetchPR(ctx, owner, repo, pr, refTime)
	}

	key := prCacheKey(c.platform.Name(), owner, repo, pr)

	if cached, found, err := c.prCache.Get(ctx, key); err != nil {
		c.logger.WarnContext(ctx, "cache get error", "error", err)
	} else if found {
		if !cached.CachedAt.Before(refTime) {
			c.logger.InfoContext(ctx, "cache hit: pull request",
				"platform", c.platform.Name(), "owner", owner, "repo", repo, "pr", pr, "cached_at", cached.CachedAt)
			return &cached, nil
		}
		c.logger.InfoContext(ctx, "cache miss: pull request expired",
			"platform", c.platform.Name(), "owner", owner, "repo", repo, "pr", pr,
			"cached_at", cached.CachedAt, "reference_time", refTime)
		if err := c.prCache.Delete(ctx, key); err != nil {
			c.logger.WarnContext(ctx, "failed to delete stale cache entry", "error", err)
		}
	} else {
		c.logger.InfoContext(ctx, "cache miss: pull request not in cache",
			"platform", c.platform.Name(), "owner", owner, "repo", repo, "pr", pr)
	}

	result, err := c.prCache.Fetch(ctx, key, func(ctx context.Context) (PullRequestData, error) {
		data, err := c.platform.FetchPR(ctx, owner, repo, pr, refTime)
		if err != nil {
			return PullRequestData{}, err
		}
		data.CachedAt = time.Now()
		return *data, nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Close releases cache resources.
func (c *Client) Close() error {
	if c.prCache != nil {
		return c.prCache.Close()
	}
	return nil
}

// NewCacheStore creates a cache store backed by the given directory.
// This is a convenience function for use with WithCacheStore.
func NewCacheStore(dir string) (PRStore, error) {
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		return nil, errors.New("cache directory must be absolute path")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}
	store, err := localfs.New[string, PullRequestData]("prx-pr", dir)
	if err != nil {
		return nil, fmt.Errorf("creating PR cache store: %w", err)
	}
	return store, nil
}

// prCacheKey generates a cache key for PR data.
func prCacheKey(platform, owner, repo string, prNumber int) string {
	key := strings.Join([]string{platform, "pr", owner, repo, strconv.Itoa(prNumber)}, "/")
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// collaboratorsCacheKey generates a cache key for collaborators data.
func collaboratorsCacheKey(owner, repo string) string {
	return fmt.Sprintf("%s/%s", owner, repo)
}

// rulesetsCacheKey generates a cache key for rulesets data.
func rulesetsCacheKey(owner, repo string) string {
	return fmt.Sprintf("%s/%s", owner, repo)
}

// isHexString returns true if the string contains only hex characters.
func isHexString(s string) bool {
	for i := range s {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}
