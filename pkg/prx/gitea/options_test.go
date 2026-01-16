package gitea

import (
	"log/slog"
	"net/http"
	"testing"
)

func TestPlatformOptions(t *testing.T) {
	t.Run("WithLogger option", func(t *testing.T) {
		logger := slog.Default()
		platform := NewPlatform("test-token", WithLogger(logger))
		if platform.logger != logger {
			t.Error("Logger was not set correctly")
		}
	})

	t.Run("WithHTTPClient option", func(t *testing.T) {
		client := &http.Client{}
		platform := NewPlatform("test-token", WithHTTPClient(client))
		if platform.httpClient != client {
			t.Error("HTTP client was not set correctly")
		}
	})

	t.Run("WithBaseURL option", func(t *testing.T) {
		baseURL := "https://codeberg.org"
		platform := NewPlatform("test-token", WithBaseURL(baseURL))
		if platform.baseURL != baseURL {
			t.Error("Base URL was not set correctly")
		}
	})

	t.Run("Multiple options", func(t *testing.T) {
		logger := slog.Default()
		client := &http.Client{}
		baseURL := "https://gitea.example.com"

		platform := NewPlatform("test-token",
			WithLogger(logger),
			WithHTTPClient(client),
			WithBaseURL(baseURL),
		)

		if platform.logger != logger {
			t.Error("Logger was not set correctly")
		}
		if platform.httpClient != client {
			t.Error("HTTP client was not set correctly")
		}
		if platform.baseURL != baseURL {
			t.Error("Base URL was not set correctly")
		}
	})
}
