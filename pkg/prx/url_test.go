package prx

import (
	"strings"
	"testing"
)

//nolint:maintidx // Table-driven test with many security test cases
func TestParseURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     *ParsedURL
		wantErr  bool
		errMatch string
	}{
		// GitHub URLs
		{
			name:  "github full URL with https",
			input: "https://github.com/owner/repo/pull/123",
			want: &ParsedURL{
				Platform: PlatformGitHub,
				Host:     "github.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   123,
			},
		},
		{
			name:  "github URL without scheme",
			input: "github.com/owner/repo/pull/456",
			want: &ParsedURL{
				Platform: PlatformGitHub,
				Host:     "github.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   456,
			},
		},
		{
			name:  "github URL with http",
			input: "http://github.com/kubernetes/kubernetes/pull/99999",
			want: &ParsedURL{
				Platform: PlatformGitHub,
				Host:     "github.com",
				Owner:    "kubernetes",
				Repo:     "kubernetes",
				Number:   99999,
			},
		},
		{
			name:  "github URL with dashes in names",
			input: "https://github.com/my-org/my-repo/pull/1",
			want: &ParsedURL{
				Platform: PlatformGitHub,
				Host:     "github.com",
				Owner:    "my-org",
				Repo:     "my-repo",
				Number:   1,
			},
		},

		// GitLab URLs
		{
			name:  "gitlab.com MR URL",
			input: "https://gitlab.com/owner/repo/-/merge_requests/123",
			want: &ParsedURL{
				Platform: PlatformGitLab,
				Host:     "gitlab.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   123,
			},
		},
		{
			name:  "self-hosted gitlab MR URL",
			input: "https://gitlab.example.com/team/project/-/merge_requests/456",
			want: &ParsedURL{
				Platform: PlatformGitLab,
				Host:     "gitlab.example.com",
				Owner:    "team",
				Repo:     "project",
				Number:   456,
			},
		},

		// Codeberg URLs
		{
			name:  "codeberg PR URL",
			input: "https://codeberg.org/owner/repo/pulls/123",
			want: &ParsedURL{
				Platform: PlatformCodeberg,
				Host:     "codeberg.org",
				Owner:    "owner",
				Repo:     "repo",
				Number:   123,
			},
		},
		{
			name:  "codeberg URL without scheme",
			input: "codeberg.org/forgejo/forgejo/pulls/789",
			want: &ParsedURL{
				Platform: PlatformCodeberg,
				Host:     "codeberg.org",
				Owner:    "forgejo",
				Repo:     "forgejo",
				Number:   789,
			},
		},

		// Generic Gitea URLs
		{
			name:  "generic gitea URL",
			input: "https://gitea.example.com/owner/repo/pulls/123",
			want: &ParsedURL{
				Platform: "gitea",
				Host:     "gitea.example.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   123,
			},
		},
		{
			name:  "unknown host defaults to gitea",
			input: "https://code.mycompany.com/team/project/pulls/456",
			want: &ParsedURL{
				Platform: "gitea",
				Host:     "code.mycompany.com",
				Owner:    "team",
				Repo:     "project",
				Number:   456,
			},
		},

		// URLs with fragments and query parameters
		{
			name:  "github URL with fragment",
			input: "https://github.com/owner/repo/pull/123#issuecomment-456",
			want: &ParsedURL{
				Platform: PlatformGitHub,
				Host:     "github.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   123,
			},
		},
		{
			name:  "github URL with query params",
			input: "https://github.com/owner/repo/pull/123?foo=bar&baz=qux",
			want: &ParsedURL{
				Platform: PlatformGitHub,
				Host:     "github.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   123,
			},
		},
		{
			name:  "gitlab URL with fragment and query",
			input: "https://gitlab.com/owner/repo/-/merge_requests/456?tab=notes#note_789",
			want: &ParsedURL{
				Platform: PlatformGitLab,
				Host:     "gitlab.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   456,
			},
		},
		{
			name:  "gitea URL with query params",
			input: "https://gitea.example.com/owner/repo/pulls/100?state=open",
			want: &ParsedURL{
				Platform: "gitea",
				Host:     "gitea.example.com",
				Owner:    "owner",
				Repo:     "repo",
				Number:   100,
			},
		},

		// Error cases
		{
			name:     "empty input",
			input:    "",
			wantErr:  true,
			errMatch: "empty URL",
		},
		{
			name:     "injection attempt with newline in host",
			input:    "https://github.com\n.evil.com/owner/repo/pull/123",
			wantErr:  true,
			errMatch: "invalid GitHub PR URL",
		},
		{
			name:     "injection attempt with @ in host",
			input:    "https://attacker@github.com/owner/repo/pull/123",
			wantErr:  true,
			errMatch: "invalid GitHub PR URL",
		},
		{
			name:     "injection attempt with special chars in owner",
			input:    "https://github.com/owner\x00/repo/pull/123",
			wantErr:  true,
			errMatch: "invalid GitHub PR URL",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			wantErr:  true,
			errMatch: "empty URL",
		},
		{
			name:     "invalid github URL - wrong path",
			input:    "https://github.com/owner/repo/issues/123",
			wantErr:  true,
			errMatch: "invalid GitHub PR URL",
		},
		{
			name:     "invalid gitlab URL - missing merge_requests",
			input:    "https://gitlab.com/owner/repo/123",
			wantErr:  true,
			errMatch: "invalid GitLab MR URL",
		},
		{
			name:     "random URL",
			input:    "https://example.com/foo/bar",
			wantErr:  true,
			errMatch: "unrecognized URL format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseURL(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseURL() expected error containing %q, got nil", tt.errMatch)
					return
				}
				if tt.errMatch != "" && !strings.Contains(err.Error(), tt.errMatch) {
					t.Errorf("ParseURL() error = %q, want error containing %q", err.Error(), tt.errMatch)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseURL() unexpected error: %v", err)
				return
			}

			if got.Platform != tt.want.Platform {
				t.Errorf("Platform = %q, want %q", got.Platform, tt.want.Platform)
			}
			if got.Host != tt.want.Host {
				t.Errorf("Host = %q, want %q", got.Host, tt.want.Host)
			}
			if got.Owner != tt.want.Owner {
				t.Errorf("Owner = %q, want %q", got.Owner, tt.want.Owner)
			}
			if got.Repo != tt.want.Repo {
				t.Errorf("Repo = %q, want %q", got.Repo, tt.want.Repo)
			}
			if got.Number != tt.want.Number {
				t.Errorf("Number = %d, want %d", got.Number, tt.want.Number)
			}
		})
	}
}

func TestDetectPlatform(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"github.com", PlatformGitHub},
		{"GITHUB.COM", PlatformGitHub},
		{"api.github.com", PlatformGitHub},
		{"gitlab.com", PlatformGitLab},
		{"gitlab.example.com", PlatformGitLab},
		{"my-gitlab.internal", PlatformGitLab},
		{"codeberg.org", PlatformCodeberg},
		{"example.com", ""},
		{"bitbucket.org", ""},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := DetectPlatform(tt.host)
			if got != tt.want {
				t.Errorf("DetectPlatform(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}

func TestDetectPlatformFromHost(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"github.com", PlatformGitHub},
		{"GITHUB.COM", PlatformGitHub},
		{"api.github.com", PlatformGitHub},
		{"gitlab.com", PlatformGitLab},
		{"gitlab.example.com", PlatformGitLab},
		{"my-gitlab.internal", PlatformGitLab},
		{"codeberg.org", PlatformCodeberg},
		{"example.com", "gitea"},       // Unknown defaults to gitea
		{"bitbucket.org", "gitea"},     // Unknown defaults to gitea
		{"gitea.example.com", "gitea"}, // Unknown defaults to gitea
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := detectPlatformFromHost(tt.host)
			if got != tt.want {
				t.Errorf("detectPlatformFromHost(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}

func TestBuildURLs(t *testing.T) {
	t.Run("GitHub", func(t *testing.T) {
		got := BuildGitHubURL("owner", "repo", 123)
		want := "https://github.com/owner/repo/pull/123"
		if got != want {
			t.Errorf("BuildGitHubURL() = %q, want %q", got, want)
		}
	})

	t.Run("GitLab default host", func(t *testing.T) {
		got := BuildGitLabURL("", "owner", "repo", 456)
		want := "https://gitlab.com/owner/repo/-/merge_requests/456"
		if got != want {
			t.Errorf("BuildGitLabURL() = %q, want %q", got, want)
		}
	})

	t.Run("GitLab custom host", func(t *testing.T) {
		got := BuildGitLabURL("gitlab.example.com", "team", "project", 789)
		want := "https://gitlab.example.com/team/project/-/merge_requests/789"
		if got != want {
			t.Errorf("BuildGitLabURL() = %q, want %q", got, want)
		}
	})

	t.Run("Codeberg", func(t *testing.T) {
		got := BuildCodebergURL("forgejo", "forgejo", 999)
		want := "https://codeberg.org/forgejo/forgejo/pulls/999"
		if got != want {
			t.Errorf("BuildCodebergURL() = %q, want %q", got, want)
		}
	})

	t.Run("Gitea default host", func(t *testing.T) {
		got := BuildGiteaURL("", "owner", "repo", 123)
		want := "https://codeberg.org/owner/repo/pulls/123"
		if got != want {
			t.Errorf("BuildGiteaURL() = %q, want %q", got, want)
		}
	})

	t.Run("Gitea custom host", func(t *testing.T) {
		got := BuildGiteaURL("gitea.example.com", "team", "project", 456)
		want := "https://gitea.example.com/team/project/pulls/456"
		if got != want {
			t.Errorf("BuildGiteaURL() = %q, want %q", got, want)
		}
	})
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "github without scheme",
			input: "github.com/owner/repo/pull/123",
			want:  "https://github.com/owner/repo/pull/123",
		},
		{
			name:  "gitlab",
			input: "https://gitlab.com/owner/repo/-/merge_requests/456",
			want:  "https://gitlab.com/owner/repo/-/merge_requests/456",
		},
		{
			name:    "invalid URL",
			input:   "not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("NormalizeURL() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("NormalizeURL() unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("NormalizeURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsValidPRURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://github.com/owner/repo/pull/123", true},
		{"https://gitlab.com/owner/repo/-/merge_requests/456", true},
		{"https://codeberg.org/owner/repo/pulls/789", true},
		{"not-a-url", false},
		{"https://example.com/foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsValidPRURL(tt.input)
			if got != tt.want {
				t.Errorf("IsValidPRURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractShortRef(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"https://github.com/kubernetes/kubernetes/pull/123", "kubernetes/kubernetes#123", false},
		{"https://gitlab.com/owner/repo/-/merge_requests/456", "owner/repo#456", false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ExtractShortRef(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("ExtractShortRef() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("ExtractShortRef() unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractShortRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseShortRef(t *testing.T) {
	tests := []struct {
		name       string
		ref        string
		wantOwner  string
		wantRepo   string
		wantNumber int
		wantErr    bool
	}{
		{
			name:       "hash format",
			ref:        "owner/repo#123",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantNumber: 123,
		},
		{
			name:       "slash format",
			ref:        "owner/repo/456",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantNumber: 456,
		},
		{
			name:       "with whitespace",
			ref:        "  owner/repo#789  ",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantNumber: 789,
		},
		{
			name:    "invalid format - no number",
			ref:     "owner/repo",
			wantErr: true,
		},
		{
			name:    "invalid format - not a number",
			ref:     "owner/repo#abc",
			wantErr: true,
		},
		{
			name:    "invalid format - too many parts",
			ref:     "a/b/c/d",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseShortRef(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Error("ParseShortRef() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("ParseShortRef() unexpected error: %v", err)
				return
			}
			if got.Owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", got.Owner, tt.wantOwner)
			}
			if got.Repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", got.Repo, tt.wantRepo)
			}
			if got.Number != tt.wantNumber {
				t.Errorf("number = %d, want %d", got.Number, tt.wantNumber)
			}
		})
	}
}

func TestParseOwnerRepoPR(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantOwner    string
		wantRepo     string
		wantNumber   int
		wantPlatform string
		wantErr      bool
	}{
		{
			name:         "full github URL",
			input:        "https://github.com/owner/repo/pull/123",
			wantOwner:    "owner",
			wantRepo:     "repo",
			wantNumber:   123,
			wantPlatform: PlatformGitHub,
		},
		{
			name:         "short ref with hash",
			input:        "owner/repo#456",
			wantOwner:    "owner",
			wantRepo:     "repo",
			wantNumber:   456,
			wantPlatform: "", // No platform for short refs
		},
		{
			name:         "short ref with slash",
			input:        "owner/repo/789",
			wantOwner:    "owner",
			wantRepo:     "repo",
			wantNumber:   789,
			wantPlatform: "",
		},
		{
			name:         "gitlab URL",
			input:        "https://gitlab.com/team/project/-/merge_requests/999",
			wantOwner:    "team",
			wantRepo:     "project",
			wantNumber:   999,
			wantPlatform: PlatformGitLab,
		},
		{
			name:    "invalid",
			input:   "not-valid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOwnerRepoPR(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("ParseOwnerRepoPR() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("ParseOwnerRepoPR() unexpected error: %v", err)
				return
			}
			if got.Owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", got.Owner, tt.wantOwner)
			}
			if got.Repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", got.Repo, tt.wantRepo)
			}
			if got.Number != tt.wantNumber {
				t.Errorf("number = %d, want %d", got.Number, tt.wantNumber)
			}
			if got.Platform != tt.wantPlatform {
				t.Errorf("platform = %q, want %q", got.Platform, tt.wantPlatform)
			}
		})
	}
}
