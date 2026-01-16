//nolint:errcheck // Test handlers don't need to check w.Write errors
package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientErrorPaths(t *testing.T) {
	t.Run("Raw method with non-200 status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`Not found`))
		}))
		defer server.Close()

		client := &Client{
			BaseURL:    server.URL,
			HTTPClient: http.DefaultClient,
			Token:      "test-token",
		}

		_, _, err := client.Raw(context.Background(), "/test")
		if err == nil {
			t.Error("Expected error for non-200 status code")
		}
	})

	t.Run("GraphQL with non-200 status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"message": "Bad request"}`))
		}))
		defer server.Close()

		client := &Client{
			BaseURL:    server.URL,
			HTTPClient: http.DefaultClient,
			Token:      "test-token",
		}

		var result map[string]any
		err := client.GraphQL(context.Background(), "query { test }", nil, &result)
		if err == nil {
			t.Error("Expected error for non-200 GraphQL response")
		}
	})

	t.Run("GraphQL with invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{invalid json`))
		}))
		defer server.Close()

		client := &Client{
			BaseURL:    server.URL,
			HTTPClient: http.DefaultClient,
			Token:      "test-token",
		}

		var result map[string]any
		err := client.GraphQL(context.Background(), "query { test }", nil, &result)
		if err == nil {
			t.Error("Expected error for invalid JSON response")
		}
	})

	t.Run("Do with context cancel", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Slow handler
			<-r.Context().Done()
		}))
		defer server.Close()

		client := &Client{
			BaseURL:    server.URL,
			HTTPClient: http.DefaultClient,
			Token:      "test-token",
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, _, err := client.Do(ctx, "/test")
		if err == nil {
			t.Error("Expected error when context is canceled")
		}
	})

	t.Run("Do with non-JSON content type", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`Bad request`))
		}))
		defer server.Close()

		client := &Client{
			BaseURL:    server.URL,
			HTTPClient: http.DefaultClient,
			Token:      "test-token",
		}

		_, _, err := client.Do(context.Background(), "/test")
		if err == nil {
			t.Error("Expected error for non-200 status with non-JSON response")
		}
	})

	t.Run("Do with JSON error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message": "Unauthorized", "documentation_url": "https://docs.github.com"}`))
		}))
		defer server.Close()

		client := &Client{
			BaseURL:    server.URL,
			HTTPClient: http.DefaultClient,
			Token:      "test-token",
		}

		_, _, err := client.Do(context.Background(), "/test")
		if err == nil {
			t.Error("Expected error for unauthorized status")
		}
		// Check that error contains the message
		if err != nil && err.Error() == "" {
			t.Error("Error message should not be empty")
		}
	})
}
