//nolint:errcheck // os.Unsetenv errors are not critical in tests
package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectPlatform(t *testing.T) {
	tests := []struct {
		input string
		want  Platform
	}{
		{"github", PlatformGitHub},
		{"gitlab", PlatformGitLab},
		{"codeberg", PlatformCodeberg},
		{"gitea", PlatformGitea},
		{"unknown", PlatformGitea}, // Default to Gitea for unknown
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := DetectPlatform(tt.input)
			if got != tt.want {
				t.Errorf("DetectPlatform(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolver_ResolveGitHub_EnvVar(t *testing.T) {
	tests := []struct {
		name      string
		envVar    string
		envValue  string
		wantToken string
		wantErr   bool
	}{
		{
			name:      "GITHUB_TOKEN set",
			envVar:    "GITHUB_TOKEN",
			envValue:  "ghp_test123",
			wantToken: "ghp_test123",
		},
		{
			name:      "GH_TOKEN set",
			envVar:    "GH_TOKEN",
			envValue:  "ghp_fallback456",
			wantToken: "ghp_fallback456",
		},
		{
			name:      "token with whitespace",
			envVar:    "GITHUB_TOKEN",
			envValue:  "  ghp_trimmed  \n",
			wantToken: "ghp_trimmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("GITHUB_TOKEN")
			os.Unsetenv("GH_TOKEN")
			t.Setenv(tt.envVar, tt.envValue)

			resolver := NewResolver()
			token, err := resolver.ResolveGitHub(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if token.Value != tt.wantToken {
				t.Errorf("Token = %q, want %q", token.Value, tt.wantToken)
			}
			if token.Source != TokenSourceEnv {
				t.Errorf("Source = %v, want %v", token.Source, TokenSourceEnv)
			}
			if token.Host != "github.com" {
				t.Errorf("Host = %q, want %q", token.Host, "github.com")
			}
		})
	}
}

func TestResolver_ResolveGitLab_EnvVar(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		envVar    string
		envValue  string
		wantToken string
		wantHost  string
	}{
		{
			name:      "GITLAB_TOKEN set",
			host:      "",
			envVar:    "GITLAB_TOKEN",
			envValue:  "glpat-test123",
			wantToken: "glpat-test123",
			wantHost:  "gitlab.com",
		},
		{
			name:      "GL_TOKEN set",
			host:      "",
			envVar:    "GL_TOKEN",
			envValue:  "glpat-fallback",
			wantToken: "glpat-fallback",
			wantHost:  "gitlab.com",
		},
		{
			name:      "custom host",
			host:      "gitlab.example.com",
			envVar:    "GITLAB_TOKEN",
			envValue:  "glpat-custom",
			wantToken: "glpat-custom",
			wantHost:  "gitlab.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("GITLAB_TOKEN")
			os.Unsetenv("GL_TOKEN")
			t.Setenv(tt.envVar, tt.envValue)

			resolver := NewResolver()
			token, err := resolver.ResolveGitLab(context.Background(), tt.host)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if token.Value != tt.wantToken {
				t.Errorf("Token = %q, want %q", token.Value, tt.wantToken)
			}
			if token.Source != TokenSourceEnv {
				t.Errorf("Source = %v, want %v", token.Source, TokenSourceEnv)
			}
			if token.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", token.Host, tt.wantHost)
			}
		})
	}
}

func TestResolver_ResolveGitea_EnvVar(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		envVar    string
		envValue  string
		wantToken string
		wantHost  string
	}{
		{
			name:      "CODEBERG_TOKEN for codeberg.org",
			host:      "codeberg.org",
			envVar:    "CODEBERG_TOKEN",
			envValue:  "codeberg123",
			wantToken: "codeberg123",
			wantHost:  "codeberg.org",
		},
		{
			name:      "GITEA_TOKEN for codeberg.org",
			host:      "codeberg.org",
			envVar:    "GITEA_TOKEN",
			envValue:  "gitea456",
			wantToken: "gitea456",
			wantHost:  "codeberg.org",
		},
		{
			name:      "GITEA_TOKEN for self-hosted",
			host:      "gitea.example.com",
			envVar:    "GITEA_TOKEN",
			envValue:  "selfhosted789",
			wantToken: "selfhosted789",
			wantHost:  "gitea.example.com",
		},
		{
			name:      "empty host defaults to codeberg",
			host:      "",
			envVar:    "CODEBERG_TOKEN",
			envValue:  "default123",
			wantToken: "default123",
			wantHost:  "codeberg.org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("CODEBERG_TOKEN")
			os.Unsetenv("GITEA_TOKEN")
			t.Setenv(tt.envVar, tt.envValue)

			resolver := NewResolver()
			token, err := resolver.ResolveGitea(context.Background(), tt.host)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if token.Value != tt.wantToken {
				t.Errorf("Token = %q, want %q", token.Value, tt.wantToken)
			}
			if token.Source != TokenSourceEnv {
				t.Errorf("Source = %v, want %v", token.Source, TokenSourceEnv)
			}
			if token.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", token.Host, tt.wantHost)
			}
		})
	}
}

func TestResolver_ResolveGitea_NoToken(t *testing.T) {
	os.Unsetenv("CODEBERG_TOKEN")
	os.Unsetenv("GITEA_TOKEN")

	resolver := NewResolver()

	t.Run("codeberg without token", func(t *testing.T) {
		_, err := resolver.ResolveGitea(context.Background(), "codeberg.org")
		if err == nil {
			t.Error("Expected error for missing Codeberg token")
		}
	})

	t.Run("gitea without token", func(t *testing.T) {
		_, err := resolver.ResolveGitea(context.Background(), "gitea.example.com")
		if err == nil {
			t.Error("Expected error for missing Gitea token")
		}
	})
}

func TestResolver_Resolve(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "gh_token")
	t.Setenv("GITLAB_TOKEN", "gl_token")
	t.Setenv("CODEBERG_TOKEN", "cb_token")

	resolver := NewResolver()
	ctx := context.Background()

	tests := []struct {
		platform  Platform
		host      string
		wantToken string
	}{
		{PlatformGitHub, "", "gh_token"},
		{PlatformGitLab, "", "gl_token"},
		{PlatformCodeberg, "", "cb_token"},
		{PlatformGitea, "codeberg.org", "cb_token"},
	}

	for _, tt := range tests {
		t.Run(string(tt.platform), func(t *testing.T) {
			token, err := resolver.Resolve(ctx, tt.platform, tt.host)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if token.Value != tt.wantToken {
				t.Errorf("Token = %q, want %q", token.Value, tt.wantToken)
			}
		})
	}
}

func TestResolver_Resolve_UnknownPlatform(t *testing.T) {
	resolver := NewResolver()
	_, err := resolver.Resolve(context.Background(), "unknown_platform", "")
	if err == nil {
		t.Error("Expected error for unknown platform")
	}
}

func TestResolver_readTeaConfig(t *testing.T) {
	// Create a temp home directory with .config/tea structure
	tempDir := t.TempDir()
	teaDir := filepath.Join(tempDir, ".config", "tea")
	if err := os.MkdirAll(teaDir, 0o755); err != nil {
		t.Fatalf("Failed to create tea config dir: %v", err)
	}

	// Override HOME so UserHomeDir returns our temp dir
	t.Setenv("HOME", tempDir)

	resolver := NewResolver()

	// Write a test config file
	configPath := filepath.Join(teaDir, "config.yml")
	configContent := `logins:
  - name: codeberg
    url: https://codeberg.org
    token: cb_tea_token
    default: true
  - name: gitea-selfhosted
    url: https://gitea.example.com
    token: gitea_tea_token
    default: false
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	t.Run("exact host match", func(t *testing.T) {
		token, err := resolver.readTeaConfig("codeberg.org")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
			return
		}
		if token != "cb_tea_token" {
			t.Errorf("Token = %q, want %q", token, "cb_tea_token")
		}
	})

	t.Run("exact host match with https prefix", func(t *testing.T) {
		token, err := resolver.readTeaConfig("https://codeberg.org")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
			return
		}
		if token != "cb_tea_token" {
			t.Errorf("Token = %q, want %q", token, "cb_tea_token")
		}
	})

	t.Run("self-hosted gitea match", func(t *testing.T) {
		token, err := resolver.readTeaConfig("gitea.example.com")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
			return
		}
		if token != "gitea_tea_token" {
			t.Errorf("Token = %q, want %q", token, "gitea_tea_token")
		}
	})

	t.Run("case insensitive host match", func(t *testing.T) {
		token, err := resolver.readTeaConfig("CODEBERG.ORG")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
			return
		}
		if token != "cb_tea_token" {
			t.Errorf("Token = %q, want %q", token, "cb_tea_token")
		}
	})

	t.Run("no matching host uses default", func(t *testing.T) {
		// The config has a default login, so unknown hosts should fall back to it
		token, err := resolver.readTeaConfig("unknown.example.com")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
			return
		}
		if token != "cb_tea_token" {
			t.Errorf("Token = %q, want %q (default)", token, "cb_tea_token")
		}
	})
}

func TestResolver_readTeaConfig_DefaultFallback(t *testing.T) {
	tempDir := t.TempDir()
	teaDir := filepath.Join(tempDir, ".config", "tea")
	if err := os.MkdirAll(teaDir, 0o755); err != nil {
		t.Fatalf("Failed to create tea config dir: %v", err)
	}
	t.Setenv("HOME", tempDir)

	// Config with only a default login
	configPath := filepath.Join(teaDir, "config.yml")
	configContent := `logins:
  - name: default-instance
    url: https://gitea.default.com
    token: default_token
    default: true
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	resolver := NewResolver()

	// When host doesn't match but there's a default, should return default token
	token, err := resolver.readTeaConfig("unknown.host.com")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
		return
	}
	if token != "default_token" {
		t.Errorf("Token = %q, want %q", token, "default_token")
	}
}

func TestResolver_readTeaConfig_NoDefault(t *testing.T) {
	tempDir := t.TempDir()
	teaDir := filepath.Join(tempDir, ".config", "tea")
	if err := os.MkdirAll(teaDir, 0o755); err != nil {
		t.Fatalf("Failed to create tea config dir: %v", err)
	}
	t.Setenv("HOME", tempDir)

	// Config with NO default login
	configPath := filepath.Join(teaDir, "config.yml")
	configContent := `logins:
  - name: specific-instance
    url: https://gitea.specific.com
    token: specific_token
    default: false
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	resolver := NewResolver()

	// When host doesn't match and there's no default, should return error
	_, err := resolver.readTeaConfig("unknown.host.com")
	if err == nil {
		t.Error("Expected error when no matching host and no default")
	}
}

func TestResolver_readTeaConfig_InvalidYAML(t *testing.T) {
	tempDir := t.TempDir()
	teaDir := filepath.Join(tempDir, ".config", "tea")
	if err := os.MkdirAll(teaDir, 0o755); err != nil {
		t.Fatalf("Failed to create tea config dir: %v", err)
	}
	t.Setenv("HOME", tempDir)

	// Write invalid YAML
	configPath := filepath.Join(teaDir, "config.yml")
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	resolver := NewResolver()
	_, err := resolver.readTeaConfig("codeberg.org")
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestResolver_ResolveGitea_TeaConfig(t *testing.T) {
	// Clear env vars
	os.Unsetenv("CODEBERG_TOKEN")
	os.Unsetenv("GITEA_TOKEN")

	tempDir := t.TempDir()
	teaDir := filepath.Join(tempDir, ".config", "tea")
	if err := os.MkdirAll(teaDir, 0o755); err != nil {
		t.Fatalf("Failed to create tea config dir: %v", err)
	}
	t.Setenv("HOME", tempDir)

	configPath := filepath.Join(teaDir, "config.yml")
	configContent := `logins:
  - name: codeberg
    url: https://codeberg.org
    token: tea_config_token
    default: false
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	resolver := NewResolver()
	token, err := resolver.ResolveGitea(context.Background(), "codeberg.org")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
		return
	}

	if token.Value != "tea_config_token" {
		t.Errorf("Token = %q, want %q", token.Value, "tea_config_token")
	}
	if token.Source != TokenSourceConfig {
		t.Errorf("Source = %v, want %v", token.Source, TokenSourceConfig)
	}
}
