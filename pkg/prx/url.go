package prx

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParsedURL represents a parsed code hosting URL.
type ParsedURL struct {
	Platform string // "github", "gitlab", "codeberg"
	Host     string // e.g., "github.com", "gitlab.com"
	Owner    string
	Repo     string
	Number   int // PR or MR number
}

// Common platform hosts.
const (
	PlatformGitHub   = "github"
	PlatformGitLab   = "gitlab"
	PlatformCodeberg = "codeberg"
	PlatformGitee    = "gitee"
)

var (
	// URL patterns for different platforms.
	githubPRPattern   = regexp.MustCompile(`^(?:https?://)?github\.com/([\w.-]+)/([\w.-]+)/pull/(\d+)`)
	gitlabMRPattern   = regexp.MustCompile(`^(?:https?://)?([\w.-]+)/([\w.-]+)/([\w.-]+)/-/merge_requests/(\d+)`)
	giteePRPattern    = regexp.MustCompile(`^(?:https?://)?gitee\.com/([\w.-]+)/([\w.-]+)/pulls/(\d+)`)
	codebergPRPattern = regexp.MustCompile(`^(?:https?://)?codeberg\.org/([\w.-]+)/([\w.-]+)/pulls/(\d+)`)
	giteaPRPattern    = regexp.MustCompile(`^(?:https?://)?([\w.-]+)/([\w.-]+)/([\w.-]+)/pulls/(\d+)`)

	// Common parsing errors.
	errEmptyURL         = errors.New("empty URL")
	errInvalidGitHubURL = errors.New("invalid GitHub PR URL format, expected: github.com/owner/repo/pull/123")
	errInvalidGitLabURL = errors.New("invalid GitLab MR URL format, expected: gitlab.com/owner/repo/-/merge_requests/123")
	errInvalidGitee     = errors.New("invalid Gitee PR URL format, expected: gitee.com/owner/repo/pulls/123")
	errInvalidCodeberg  = errors.New("invalid Codeberg PR URL format, expected: codeberg.org/owner/repo/pulls/123")
)

// ParseURL parses a pull request or merge request URL and returns its components.
// It detects the platform based on the URL structure and host.
// URL fragments (#...) and query parameters (?...) are automatically stripped.
func ParseURL(input string) (*ParsedURL, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errEmptyURL
	}

	// Strip fragments and query parameters for permissive parsing
	if idx := strings.Index(input, "#"); idx != -1 {
		input = input[:idx]
	}
	if idx := strings.Index(input, "?"); idx != -1 {
		input = input[:idx]
	}
	input = strings.TrimSpace(input)

	// Try GitHub pattern first (most common).
	if match := githubPRPattern.FindStringSubmatch(input); match != nil {
		number, err := strconv.Atoi(match[3])
		if err != nil {
			return nil, errInvalidGitHubURL
		}
		return &ParsedURL{
			Platform: PlatformGitHub,
			Host:     "github.com",
			Owner:    match[1],
			Repo:     match[2],
			Number:   number,
		}, nil
	}

	// Try Gitee pattern.
	if match := giteePRPattern.FindStringSubmatch(input); match != nil {
		number, err := strconv.Atoi(match[3])
		if err != nil {
			return nil, errInvalidGitee
		}
		return &ParsedURL{
			Platform: PlatformGitee,
			Host:     "gitee.com",
			Owner:    match[1],
			Repo:     match[2],
			Number:   number,
		}, nil
	}

	// Try Codeberg pattern (before GitLab since it's more specific).
	if match := codebergPRPattern.FindStringSubmatch(input); match != nil {
		number, err := strconv.Atoi(match[3])
		if err != nil {
			return nil, errInvalidCodeberg
		}
		return &ParsedURL{
			Platform: PlatformCodeberg,
			Host:     "codeberg.org",
			Owner:    match[1],
			Repo:     match[2],
			Number:   number,
		}, nil
	}

	// Try GitLab pattern (includes self-hosted instances).
	if match := gitlabMRPattern.FindStringSubmatch(input); match != nil {
		number, err := strconv.Atoi(match[4])
		if err != nil {
			return nil, errInvalidGitLabURL
		}
		return &ParsedURL{
			Platform: PlatformGitLab,
			Host:     match[1],
			Owner:    match[2],
			Repo:     match[3],
			Number:   number,
		}, nil
	}

	// Try generic Gitea pattern as fallback (default for unknown hosts).
	if match := giteaPRPattern.FindStringSubmatch(input); match != nil {
		number, err := strconv.Atoi(match[4])
		if err != nil {
			return nil, errors.New("invalid PR number in Gitea URL")
		}

		host := match[1]

		// Detect platform from host if possible
		platform := DetectPlatformFromHost(host)

		return &ParsedURL{
			Platform: platform,
			Host:     host,
			Owner:    match[2],
			Repo:     match[3],
			Number:   number,
		}, nil
	}

	// Try to provide a helpful error message.
	if strings.Contains(input, "github.com") {
		return nil, errInvalidGitHubURL
	}
	if strings.Contains(input, "gitee.com") {
		return nil, errInvalidGitee
	}
	if strings.Contains(input, "gitlab") || strings.Contains(input, "merge_requests") {
		return nil, errInvalidGitLabURL
	}
	if strings.Contains(input, "codeberg.org") {
		return nil, errInvalidCodeberg
	}

	return nil, errors.New("unrecognized URL format")
}

// DetectPlatformFromHost detects platform from hostname, defaulting to Gitea for unknown hosts.
func DetectPlatformFromHost(host string) string {
	host = strings.ToLower(host)

	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return PlatformGitHub
	case host == "gitee.com":
		return PlatformGitee
	case host == "gitlab.com" || strings.Contains(host, "gitlab"):
		return PlatformGitLab
	case host == "codeberg.org":
		return PlatformCodeberg
	default:
		// Default to Gitea for unknown hosts
		return "gitea"
	}
}

// DetectPlatform attempts to detect the platform from a host name.
// Returns the platform identifier or empty string if unknown.
//
// Deprecated: Use detectPlatformFromHost instead, which defaults to Gitea.
func DetectPlatform(host string) string {
	host = strings.ToLower(host)

	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return PlatformGitHub
	case host == "gitee.com":
		return PlatformGitee
	case host == "gitlab.com" || strings.Contains(host, "gitlab"):
		return PlatformGitLab
	case host == "codeberg.org":
		return PlatformCodeberg
	default:
		return ""
	}
}

// BuildGitHubURL constructs a GitHub PR URL from components.
func BuildGitHubURL(owner, repo string, number int) string {
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number)
}

// BuildGiteeURL constructs a Gitee PR URL from components.
func BuildGiteeURL(owner, repo string, number int) string {
	return fmt.Sprintf("https://gitee.com/%s/%s/pulls/%d", owner, repo, number)
}

// BuildGitLabURL constructs a GitLab MR URL from components.
func BuildGitLabURL(host, owner, repo string, number int) string {
	if host == "" {
		host = "gitlab.com"
	}
	return fmt.Sprintf("https://%s/%s/%s/-/merge_requests/%d", host, owner, repo, number)
}

// BuildCodebergURL constructs a Codeberg PR URL from components.
func BuildCodebergURL(owner, repo string, number int) string {
	return fmt.Sprintf("https://codeberg.org/%s/%s/pulls/%d", owner, repo, number)
}

// BuildGiteaURL constructs a Gitea PR URL from components.
// For Codeberg, use BuildCodebergURL instead.
func BuildGiteaURL(host, owner, repo string, number int) string {
	if host == "" {
		host = "codeberg.org"
	}
	return fmt.Sprintf("https://%s/%s/%s/pulls/%d", host, owner, repo, number)
}

// NormalizeURL takes any supported URL format and returns a normalized URL string.
func NormalizeURL(input string) (string, error) {
	parsed, err := ParseURL(input)
	if err != nil {
		return "", err
	}

	switch parsed.Platform {
	case PlatformGitHub:
		return BuildGitHubURL(parsed.Owner, parsed.Repo, parsed.Number), nil
	case PlatformGitee:
		return BuildGiteeURL(parsed.Owner, parsed.Repo, parsed.Number), nil
	case PlatformGitLab:
		return BuildGitLabURL(parsed.Host, parsed.Owner, parsed.Repo, parsed.Number), nil
	case PlatformCodeberg:
		return BuildCodebergURL(parsed.Owner, parsed.Repo, parsed.Number), nil
	default:
		return "", errors.New("unknown platform")
	}
}

// IsValidPRURL returns true if the input appears to be a valid PR/MR URL.
func IsValidPRURL(input string) bool {
	_, err := ParseURL(input)
	return err == nil
}

// ExtractShortRef returns a short reference string like "owner/repo#123".
func ExtractShortRef(input string) (string, error) {
	parsed, err := ParseURL(input)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s#%d", parsed.Owner, parsed.Repo, parsed.Number), nil
}

// ShortRef represents a parsed short reference like "owner/repo#123".
type ShortRef struct {
	Owner  string
	Repo   string
	Number int
}

// ParseShortRef parses a short reference like "owner/repo#123" or "owner/repo/123".
// It does not include platform information - that must be provided separately.
func ParseShortRef(ref string) (*ShortRef, error) {
	ref = strings.TrimSpace(ref)

	// Try "owner/repo#123" format.
	if idx := strings.Index(ref, "#"); idx != -1 {
		parts := strings.Split(ref[:idx], "/")
		if len(parts) != 2 {
			return nil, errors.New("invalid short reference format")
		}
		num, err := strconv.Atoi(ref[idx+1:])
		if err != nil {
			return nil, errors.New("invalid PR number in reference")
		}
		return &ShortRef{Owner: parts[0], Repo: parts[1], Number: num}, nil
	}

	// Try "owner/repo/123" format.
	parts := strings.Split(ref, "/")
	if len(parts) == 3 {
		num, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, errors.New("invalid PR number in reference")
		}
		return &ShortRef{Owner: parts[0], Repo: parts[1], Number: num}, nil
	}

	return nil, errors.New("invalid short reference format")
}

// ParsedPR represents the result of parsing a PR reference in any format.
type ParsedPR struct {
	Owner    string
	Repo     string
	Platform string // Empty if parsed from short ref
	Number   int
}

// ParseOwnerRepoPR is a convenience function that accepts multiple input formats:
// full URL, short ref with hash, or short ref with slash.
func ParseOwnerRepoPR(input string) (*ParsedPR, error) {
	input = strings.TrimSpace(input)

	// Check if it looks like a URL.
	if strings.Contains(input, "://") || strings.Contains(input, ".com") || strings.Contains(input, ".org") {
		parsed, err := ParseURL(input)
		if err != nil {
			return nil, err
		}
		return &ParsedPR{
			Owner:    parsed.Owner,
			Repo:     parsed.Repo,
			Number:   parsed.Number,
			Platform: parsed.Platform,
		}, nil
	}

	// Try short ref format.
	shortRef, err := ParseShortRef(input)
	if err != nil {
		return nil, err
	}
	return &ParsedPR{
		Owner:  shortRef.Owner,
		Repo:   shortRef.Repo,
		Number: shortRef.Number,
	}, nil
}
