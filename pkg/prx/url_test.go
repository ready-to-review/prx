package prx

import (
	"testing"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    *ParsedURL
		expectError bool
	}{
		// GitHub URLs
		{
			name:  "github PR with https",
			input: "https://github.com/owner/repo/pull/123",
			expected: &ParsedURL{
				Platform: PlatformGitHub,
				Host:     "github.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   123,
			},
		},
		{
			name:  "github PR without https",
			input: "github.com/owner/repo/pull/456",
			expected: &ParsedURL{
				Platform: PlatformGitHub,
				Host:     "github.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   456,
			},
		},
		{
			name:  "github PR with dashes and dots",
			input: "https://github.com/org-name/my.repo/pull/789",
			expected: &ParsedURL{
				Platform: PlatformGitHub,
				Host:     "github.com",
				Owner:    "org-name",
				Repo:     "my.repo",
				Number:   789,
			},
		},
		{
			name:  "github PR with fragment",
			input: "https://github.com/owner/repo/pull/123#issuecomment-123456",
			expected: &ParsedURL{
				Platform: PlatformGitHub,
				Host:     "github.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   123,
			},
		},
		{
			name:  "github PR with query params",
			input: "https://github.com/owner/repo/pull/123?foo=bar",
			expected: &ParsedURL{
				Platform: PlatformGitHub,
				Host:     "github.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   123,
			},
		},
		// GitLab URLs
		{
			name:  "gitlab MR with https",
			input: "https://gitlab.com/owner/repo/-/merge_requests/123",
			expected: &ParsedURL{
				Platform: PlatformGitLab,
				Host:     "gitlab.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   123,
			},
		},
		{
			name:  "gitlab self-hosted MR",
			input: "https://gitlab.example.com/owner/repo/-/merge_requests/456",
			expected: &ParsedURL{
				Platform: PlatformGitLab,
				Host:     "gitlab.example.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   456,
			},
		},
		// Codeberg URLs
		{
			name:  "codeberg PR with https",
			input: "https://codeberg.org/owner/repo/pulls/123",
			expected: &ParsedURL{
				Platform: PlatformCodeberg,
				Host:     "codeberg.org",
				Owner:    "owner",
				Repo:     "repo",
				Number:   123,
			},
		},
		{
			name:  "codeberg PR without https",
			input: "codeberg.org/owner/repo/pulls/456",
			expected: &ParsedURL{
				Platform: PlatformCodeberg,
				Host:     "codeberg.org",
				Owner:    "owner",
				Repo:     "repo",
				Number:   456,
			},
		},
		// Generic Gitea URLs
		{
			name:  "gitea instance PR",
			input: "https://gitea.example.com/owner/repo/pulls/789",
			expected: &ParsedURL{
				Platform: "gitea",
				Host:     "gitea.example.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   789,
			},
		},
		// Error cases
		{
			name:        "empty URL",
			input:       "",
			expectError: true,
		},
		{
			name:        "whitespace only",
			input:       "   ",
			expectError: true,
		},
		{
			name:        "invalid github URL",
			input:       "https://github.com/owner/repo",
			expectError: true,
		},
		{
			name:        "invalid gitlab URL",
			input:       "https://gitlab.com/owner/repo",
			expectError: true,
		},
		{
			name:        "unrecognized format",
			input:       "https://example.com/something",
			expectError: true,
		},
		{
			name:        "invalid PR number",
			input:       "https://github.com/owner/repo/pull/abc",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseURL(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("ParseURL(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseURL(%q) unexpected error: %v", tt.input, err)
				return
			}

			if result.Platform != tt.expected.Platform {
				t.Errorf("Platform = %q, expected %q", result.Platform, tt.expected.Platform)
			}
			if result.Host != tt.expected.Host {
				t.Errorf("Host = %q, expected %q", result.Host, tt.expected.Host)
			}
			if result.Owner != tt.expected.Owner {
				t.Errorf("Owner = %q, expected %q", result.Owner, tt.expected.Owner)
			}
			if result.Repo != tt.expected.Repo {
				t.Errorf("Repo = %q, expected %q", result.Repo, tt.expected.Repo)
			}
			if result.Number != tt.expected.Number {
				t.Errorf("Number = %d, expected %d", result.Number, tt.expected.Number)
			}
		})
	}
}

func TestDetectPlatformFromHost(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{name: "github.com", host: "github.com", expected: PlatformGitHub},
		{name: "GitHub subdomain", host: "api.github.com", expected: PlatformGitHub},
		{name: "gitlab.com", host: "gitlab.com", expected: PlatformGitLab},
		{name: "GitLab self-hosted", host: "gitlab.example.com", expected: PlatformGitLab},
		{name: "codeberg.org", host: "codeberg.org", expected: PlatformCodeberg},
		{name: "unknown host defaults to gitea", host: "git.example.com", expected: "gitea"},
		{name: "uppercase GitHub", host: "GITHUB.COM", expected: PlatformGitHub},
		{name: "uppercase GitLab", host: "GITLAB.COM", expected: PlatformGitLab},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectPlatformFromHost(tt.host)
			if result != tt.expected {
				t.Errorf("DetectPlatformFromHost(%q) = %q, expected %q", tt.host, result, tt.expected)
			}
		})
	}
}

func TestDetectPlatform(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{name: "github.com", host: "github.com", expected: PlatformGitHub},
		{name: "GitHub subdomain", host: "api.github.com", expected: PlatformGitHub},
		{name: "gitlab.com", host: "gitlab.com", expected: PlatformGitLab},
		{name: "GitLab self-hosted", host: "gitlab.example.com", expected: PlatformGitLab},
		{name: "codeberg.org", host: "codeberg.org", expected: PlatformCodeberg},
		{name: "unknown host returns empty", host: "git.example.com", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectPlatform(tt.host)
			if result != tt.expected {
				t.Errorf("DetectPlatform(%q) = %q, expected %q", tt.host, result, tt.expected)
			}
		})
	}
}

func TestBuildGitHubURL(t *testing.T) {
	tests := []struct {
		name     string
		owner    string
		repo     string
		number   int
		expected string
	}{
		{
			name:     "basic URL",
			owner:    "owner",
			repo:     "repo",
			number:   123,
			expected: "https://github.com/owner/repo/pull/123",
		},
		{
			name:     "with dashes and dots",
			owner:    "my-org",
			repo:     "my.repo",
			number:   456,
			expected: "https://github.com/my-org/my.repo/pull/456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildGitHubURL(tt.owner, tt.repo, tt.number)
			if result != tt.expected {
				t.Errorf("BuildGitHubURL() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestBuildGitLabURL(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		owner    string
		repo     string
		number   int
		expected string
	}{
		{
			name:     "default host",
			host:     "",
			owner:    "owner",
			repo:     "repo",
			number:   123,
			expected: "https://gitlab.com/owner/repo/-/merge_requests/123",
		},
		{
			name:     "gitlab.com explicit",
			host:     "gitlab.com",
			owner:    "owner",
			repo:     "repo",
			number:   456,
			expected: "https://gitlab.com/owner/repo/-/merge_requests/456",
		},
		{
			name:     "self-hosted",
			host:     "gitlab.example.com",
			owner:    "owner",
			repo:     "repo",
			number:   789,
			expected: "https://gitlab.example.com/owner/repo/-/merge_requests/789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildGitLabURL(tt.host, tt.owner, tt.repo, tt.number)
			if result != tt.expected {
				t.Errorf("BuildGitLabURL() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestBuildCodebergURL(t *testing.T) {
	result := BuildCodebergURL("owner", "repo", 123)
	expected := "https://codeberg.org/owner/repo/pulls/123"
	if result != expected {
		t.Errorf("BuildCodebergURL() = %q, expected %q", result, expected)
	}
}

func TestBuildGiteaURL(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		owner    string
		repo     string
		number   int
		expected string
	}{
		{
			name:     "default host",
			host:     "",
			owner:    "owner",
			repo:     "repo",
			number:   123,
			expected: "https://codeberg.org/owner/repo/pulls/123",
		},
		{
			name:     "custom host",
			host:     "gitea.example.com",
			owner:    "owner",
			repo:     "repo",
			number:   456,
			expected: "https://gitea.example.com/owner/repo/pulls/456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildGiteaURL(tt.host, tt.owner, tt.repo, tt.number)
			if result != tt.expected {
				t.Errorf("BuildGiteaURL() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:     "github URL",
			input:    "github.com/owner/repo/pull/123",
			expected: "https://github.com/owner/repo/pull/123",
		},
		{
			name:     "github URL with fragment",
			input:    "https://github.com/owner/repo/pull/123#issuecomment-456",
			expected: "https://github.com/owner/repo/pull/123",
		},
		{
			name:     "gitlab URL",
			input:    "https://gitlab.com/owner/repo/-/merge_requests/123",
			expected: "https://gitlab.com/owner/repo/-/merge_requests/123",
		},
		{
			name:     "codeberg URL",
			input:    "codeberg.org/owner/repo/pulls/123",
			expected: "https://codeberg.org/owner/repo/pulls/123",
		},
		{
			name:        "invalid URL",
			input:       "not-a-url",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NormalizeURL(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("NormalizeURL(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("NormalizeURL(%q) unexpected error: %v", tt.input, err)
				return
			}

			if result != tt.expected {
				t.Errorf("NormalizeURL(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidPRURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "valid github", input: "https://github.com/owner/repo/pull/123", expected: true},
		{name: "valid gitlab", input: "https://gitlab.com/owner/repo/-/merge_requests/123", expected: true},
		{name: "valid codeberg", input: "https://codeberg.org/owner/repo/pulls/123", expected: true},
		{name: "invalid format", input: "not-a-url", expected: false},
		{name: "empty string", input: "", expected: false},
		{name: "incomplete github", input: "https://github.com/owner/repo", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidPRURL(tt.input)
			if result != tt.expected {
				t.Errorf("IsValidPRURL(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractShortRef(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:     "github URL",
			input:    "https://github.com/owner/repo/pull/123",
			expected: "owner/repo#123",
		},
		{
			name:     "gitlab URL",
			input:    "https://gitlab.com/owner/repo/-/merge_requests/456",
			expected: "owner/repo#456",
		},
		{
			name:     "codeberg URL",
			input:    "https://codeberg.org/owner/repo/pulls/789",
			expected: "owner/repo#789",
		},
		{
			name:        "invalid URL",
			input:       "not-a-url",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractShortRef(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("ExtractShortRef(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("ExtractShortRef(%q) unexpected error: %v", tt.input, err)
				return
			}

			if result != tt.expected {
				t.Errorf("ExtractShortRef(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseShortRef(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    *ShortRef
		expectError bool
	}{
		{
			name:  "hash format",
			input: "owner/repo#123",
			expected: &ShortRef{
				Owner:  "owner",
				Repo:   "repo",
				Number: 123,
			},
		},
		{
			name:  "slash format",
			input: "owner/repo/456",
			expected: &ShortRef{
				Owner:  "owner",
				Repo:   "repo",
				Number: 456,
			},
		},
		{
			name:  "with whitespace",
			input: "  owner/repo#789  ",
			expected: &ShortRef{
				Owner:  "owner",
				Repo:   "repo",
				Number: 789,
			},
		},
		{
			name:        "invalid format - too few parts",
			input:       "owner#123",
			expectError: true,
		},
		{
			name:        "invalid format - too many parts",
			input:       "owner/repo/extra#123",
			expectError: true,
		},
		{
			name:        "invalid number",
			input:       "owner/repo#abc",
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseShortRef(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("ParseShortRef(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseShortRef(%q) unexpected error: %v", tt.input, err)
				return
			}

			if result.Owner != tt.expected.Owner {
				t.Errorf("Owner = %q, expected %q", result.Owner, tt.expected.Owner)
			}
			if result.Repo != tt.expected.Repo {
				t.Errorf("Repo = %q, expected %q", result.Repo, tt.expected.Repo)
			}
			if result.Number != tt.expected.Number {
				t.Errorf("Number = %d, expected %d", result.Number, tt.expected.Number)
			}
		})
	}
}

func TestParseOwnerRepoPR(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    *ParsedPR
		expectError bool
	}{
		{
			name:  "full github URL",
			input: "https://github.com/owner/repo/pull/123",
			expected: &ParsedPR{
				Owner:    "owner",
				Repo:     "repo",
				Number:   123,
				Platform: PlatformGitHub,
			},
		},
		{
			name:  "full gitlab URL",
			input: "https://gitlab.com/owner/repo/-/merge_requests/456",
			expected: &ParsedPR{
				Owner:    "owner",
				Repo:     "repo",
				Number:   456,
				Platform: PlatformGitLab,
			},
		},
		{
			name:  "short ref with hash",
			input: "owner/repo#789",
			expected: &ParsedPR{
				Owner:    "owner",
				Repo:     "repo",
				Number:   789,
				Platform: "",
			},
		},
		{
			name:  "short ref with slash",
			input: "owner/repo/101",
			expected: &ParsedPR{
				Owner:    "owner",
				Repo:     "repo",
				Number:   101,
				Platform: "",
			},
		},
		{
			name:        "invalid format",
			input:       "not-valid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseOwnerRepoPR(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("ParseOwnerRepoPR(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseOwnerRepoPR(%q) unexpected error: %v", tt.input, err)
				return
			}

			if result.Owner != tt.expected.Owner {
				t.Errorf("Owner = %q, expected %q", result.Owner, tt.expected.Owner)
			}
			if result.Repo != tt.expected.Repo {
				t.Errorf("Repo = %q, expected %q", result.Repo, tt.expected.Repo)
			}
			if result.Number != tt.expected.Number {
				t.Errorf("Number = %d, expected %d", result.Number, tt.expected.Number)
			}
			if result.Platform != tt.expected.Platform {
				t.Errorf("Platform = %q, expected %q", result.Platform, tt.expected.Platform)
			}
		})
	}
}
