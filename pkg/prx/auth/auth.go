// Package auth provides token resolution for different git hosting platforms.
// It supports environment variables and CLI tools for GitHub, GitLab, and Gitea/Codeberg.
package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Platform identifies a git hosting platform.
type Platform string

// Platform constants.
const (
	PlatformGitHub   Platform = "github"
	PlatformGitLab   Platform = "gitlab"
	PlatformCodeberg Platform = "codeberg"
	PlatformGitea    Platform = "gitea"
	PlatformGitee    Platform = "gitee"
)

// TokenSource describes where a token was obtained from.
type TokenSource string

// Token source constants.
const (
	TokenSourceEnv    TokenSource = "env"
	TokenSourceCLI    TokenSource = "cli"
	TokenSourceConfig TokenSource = "config"
)

// Token represents an authentication token with metadata.
type Token struct {
	Value  string
	Source TokenSource
	Host   string // For multi-host platforms like GitLab/Gitea
}

// ErrNoToken is returned when no token could be found.
var ErrNoToken = errors.New("no authentication token found")

// Resolver resolves authentication tokens for git hosting platforms.
type Resolver struct {
	timeout time.Duration
}

// NewResolver creates a new token resolver.
func NewResolver() *Resolver {
	return &Resolver{
		timeout: 10 * time.Second,
	}
}

// ResolveGitHub returns a GitHub token from GITHUB_TOKEN/GH_TOKEN env or gh CLI.
func (r *Resolver) ResolveGitHub(ctx context.Context) (*Token, error) {
	// Check environment variables first
	for _, envVar := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if token := os.Getenv(envVar); token != "" {
			token = strings.TrimSpace(token)
			if token != "" {
				return &Token{Value: token, Source: TokenSourceEnv, Host: "github.com"}, nil
			}
		}
	}

	// Try gh CLI
	token, err := r.runCommand(ctx, "gh", "auth", "token")
	if err != nil {
		return nil, fmt.Errorf("%w: set GITHUB_TOKEN or run 'gh auth login'", ErrNoToken)
	}

	return &Token{Value: token, Source: TokenSourceCLI, Host: "github.com"}, nil
}

// ResolveGitLab returns a GitLab token from GITLAB_TOKEN/GL_TOKEN env or glab CLI.
func (r *Resolver) ResolveGitLab(ctx context.Context, host string) (*Token, error) {
	if host == "" {
		host = "gitlab.com"
	}

	// Check environment variables first
	for _, envVar := range []string{"GITLAB_TOKEN", "GL_TOKEN"} {
		if token := os.Getenv(envVar); token != "" {
			token = strings.TrimSpace(token)
			if token != "" {
				return &Token{Value: token, Source: TokenSourceEnv, Host: host}, nil
			}
		}
	}

	// Try glab config get token (most reliable method)
	token, err := r.runCommand(ctx, "glab", "config", "get", "token", "--host", host)
	if err == nil && token != "" {
		return &Token{Value: token, Source: TokenSourceCLI, Host: host}, nil
	}

	return nil, fmt.Errorf("%w: set GITLAB_TOKEN or run 'glab auth login'", ErrNoToken)
}

// ResolveGitea returns a Gitea/Codeberg token from env, tea config, or berg CLI.
func (r *Resolver) ResolveGitea(ctx context.Context, host string) (*Token, error) {
	if host == "" {
		host = "codeberg.org"
	}

	isCodeberg := strings.Contains(host, "codeberg.org")

	// Check environment variables first
	envVars := []string{"GITEA_TOKEN"}
	if isCodeberg {
		envVars = append([]string{"CODEBERG_TOKEN"}, envVars...)
	}

	for _, envVar := range envVars {
		if token := os.Getenv(envVar); token != "" {
			token = strings.TrimSpace(token)
			if token != "" {
				return &Token{Value: token, Source: TokenSourceEnv, Host: host}, nil
			}
		}
	}

	// Try tea config file (~/.config/tea/config.yml)
	if token, err := r.readTeaConfig(host); err == nil && token != "" {
		return &Token{Value: token, Source: TokenSourceConfig, Host: host}, nil
	}

	// Try berg CLI for Codeberg (codeberg-cli)
	if isCodeberg {
		if token, err := r.runCommand(ctx, "berg", "auth", "token"); err == nil && token != "" {
			return &Token{Value: token, Source: TokenSourceCLI, Host: host}, nil
		}
	}

	// Build helpful error message
	if isCodeberg {
		return nil, fmt.Errorf("%w: set CODEBERG_TOKEN, run 'tea login add', or run 'berg auth login'", ErrNoToken)
	}
	return nil, fmt.Errorf("%w: set GITEA_TOKEN or run 'tea login add'", ErrNoToken)
}

// ResolveGitee returns a Gitee token from GITEE_TOKEN env.
func (*Resolver) ResolveGitee(_ context.Context, host string) (*Token, error) {
	if host == "" {
		host = "gitee.com"
	}

	// Check GITEE_TOKEN environment variable
	if token := os.Getenv("GITEE_TOKEN"); token != "" {
		token = strings.TrimSpace(token)
		if token != "" {
			return &Token{Value: token, Source: TokenSourceEnv, Host: host}, nil
		}
	}

	// Gitee doesn't have a widely-used CLI tool like gh or glab,
	// so we only check the environment variable
	return nil, fmt.Errorf("%w: set GITEE_TOKEN environment variable", ErrNoToken)
}

// teaConfig represents the tea CLI config file structure.
type teaConfig struct {
	Logins []teaLogin `yaml:"logins"`
}

type teaLogin struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url"`
	Token   string `yaml:"token"`
	Default bool   `yaml:"default"`
}

// readTeaConfig reads the tea CLI config file and returns a token for the given host.
// It checks ~/.config/tea/config.yml first (where tea stores its config on all platforms),
// then falls back to the OS-specific config directory.
func (*Resolver) readTeaConfig(host string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// tea uses ~/.config/tea on all platforms
	configPath := filepath.Join(homeDir, ".config", "tea", "config.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	var config teaConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", err
	}

	// Normalize host for comparison
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")

	// Look for matching login
	for i := range config.Logins {
		loginHost := strings.TrimPrefix(config.Logins[i].URL, "https://")
		loginHost = strings.TrimPrefix(loginHost, "http://")
		loginHost = strings.TrimSuffix(loginHost, "/")

		if strings.EqualFold(loginHost, host) && config.Logins[i].Token != "" {
			return config.Logins[i].Token, nil
		}
	}

	// If no exact match, try default login
	for i := range config.Logins {
		if config.Logins[i].Default && config.Logins[i].Token != "" {
			return config.Logins[i].Token, nil
		}
	}

	return "", errors.New("no matching login found in tea config")
}

// Resolve automatically detects the platform and returns the appropriate token.
func (r *Resolver) Resolve(ctx context.Context, platform Platform, host string) (*Token, error) {
	switch platform {
	case PlatformGitHub:
		return r.ResolveGitHub(ctx)
	case PlatformGitLab:
		return r.ResolveGitLab(ctx, host)
	case PlatformGitee:
		return r.ResolveGitee(ctx, host)
	case PlatformCodeberg, PlatformGitea:
		return r.ResolveGitea(ctx, host)
	default:
		return nil, fmt.Errorf("unknown platform: %s", platform)
	}
}

// runCommand executes a command and returns trimmed stdout.
func (r *Resolver) runCommand(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// DetectPlatform converts a prx platform string to an auth Platform.
func DetectPlatform(platform string) Platform {
	switch platform {
	case "github":
		return PlatformGitHub
	case "gitlab":
		return PlatformGitLab
	case "gitee":
		return PlatformGitee
	case "codeberg":
		return PlatformCodeberg
	default:
		return PlatformGitea
	}
}
