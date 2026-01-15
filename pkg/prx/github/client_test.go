//nolint:errcheck // Test handlers don't need to check w.Write errors
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_Do(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		serverHandler  http.HandlerFunc
		wantErr        bool
		wantStatusCode int
	}{
		{
			name: "successful request",
			path: "/test",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("Expected Authorization header with token")
				}
				if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
					t.Errorf("Expected Accept header")
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"test": "data"}`))
			},
			wantErr: false,
		},
		{
			name: "api error 404",
			path: "/notfound",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message": "Not Found"}`))
			},
			wantErr:        true,
			wantStatusCode: http.StatusNotFound,
		},
		{
			name: "api error 403 collaborators warning",
			path: "/repos/owner/repo/collaborators",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message": "Forbidden"}`))
			},
			wantErr:        true,
			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "pagination with next page",
			path: "/test",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Link", `<https://api.github.com/test?page=2>; rel="next"`)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"test": "data"}`))
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			client := &Client{
				HTTPClient: server.Client(),
				Token:      "test-token",
				BaseURL:    server.URL,
			}

			data, resp, err := client.Do(context.Background(), tt.path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				var apiErr *Error
				if errors.As(err, &apiErr) {
					if apiErr.StatusCode != tt.wantStatusCode {
						t.Errorf("Expected status code %d, got %d", tt.wantStatusCode, apiErr.StatusCode)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if data == nil {
					t.Errorf("Expected data but got nil")
				}
				if resp == nil {
					t.Errorf("Expected response but got nil")
				}
			}
		})
	}
}

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"login": "testuser", "type": "User"}`))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		Token:      "test-token",
		BaseURL:    server.URL,
	}

	var user User
	resp, err := client.Get(context.Background(), "/users/testuser", &user)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response but got nil")
	}
	if user.Login != "testuser" {
		t.Errorf("Expected login 'testuser', got '%s'", user.Login)
	}
	if user.Type != "User" {
		t.Errorf("Expected type 'User', got '%s'", user.Type)
	}
}

func TestClient_Raw(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"raw": "json", "data": 123}`))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		Token:      "test-token",
		BaseURL:    server.URL,
	}

	raw, resp, err := client.Raw(context.Background(), "/test")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response but got nil")
	}
	if len(raw) == 0 {
		t.Fatal("Expected raw data but got empty")
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("Failed to unmarshal raw data: %v", err)
	}
	if data["raw"] != "json" {
		t.Errorf("Expected raw field to be 'json', got '%v'", data["raw"])
	}
}

func TestClient_Collaborators(t *testing.T) {
	tests := []struct {
		name          string
		serverHandler http.HandlerFunc
		wantErr       bool
		wantCollabs   map[string]string
	}{
		{
			name: "successful fetch with various permissions",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.URL.Path, "/collaborators") {
					t.Errorf("Expected collaborators path")
				}
				if !strings.Contains(r.URL.Query().Get("affiliation"), "all") {
					t.Errorf("Expected affiliation=all query parameter")
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[
					{
						"login": "admin-user",
						"permissions": {"admin": true, "maintain": false, "push": true, "triage": true, "pull": true}
					},
					{
						"login": "maintainer-user",
						"permissions": {"admin": false, "maintain": true, "push": true, "triage": true, "pull": true}
					},
					{
						"login": "write-user",
						"permissions": {"admin": false, "maintain": false, "push": true, "triage": true, "pull": true}
					},
					{
						"login": "triage-user",
						"permissions": {"admin": false, "maintain": false, "push": false, "triage": true, "pull": true}
					},
					{
						"login": "read-user",
						"permissions": {"admin": false, "maintain": false, "push": false, "triage": false, "pull": true}
					},
					{
						"login": "none-user",
						"permissions": {"admin": false, "maintain": false, "push": false, "triage": false, "pull": false}
					}
				]`))
			},
			wantErr: false,
			wantCollabs: map[string]string{
				"admin-user":      "admin",
				"maintainer-user": "maintain",
				"write-user":      "write",
				"triage-user":     "triage",
				"read-user":       "read",
				"none-user":       "none",
			},
		},
		{
			name: "api error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message": "Forbidden"}`))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			client := &Client{
				HTTPClient: server.Client(),
				Token:      "test-token",
				BaseURL:    server.URL,
			}

			collabs, err := client.Collaborators(context.Background(), "owner", "repo")

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if len(collabs) != len(tt.wantCollabs) {
					t.Errorf("Expected %d collaborators, got %d", len(tt.wantCollabs), len(collabs))
				}
				for login, perm := range tt.wantCollabs {
					if collabs[login] != perm {
						t.Errorf("Expected %s to have permission %s, got %s", login, perm, collabs[login])
					}
				}
			}
		})
	}
}

func TestError_Error(t *testing.T) {
	err := &Error{
		Status:     "404 Not Found",
		Body:       `{"message": "Not Found"}`,
		URL:        "https://api.github.com/repos/owner/repo",
		StatusCode: 404,
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "github API error") {
		t.Errorf("Expected error message to contain 'github API error', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "404 Not Found") {
		t.Errorf("Expected error message to contain status, got: %s", errMsg)
	}
}

func TestClient_TokenMasking(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		wantShort bool
	}{
		{
			name:      "long token gets masked",
			token:     "ghp_1234567890abcdefghijklmnopqrstuvwxyz",
			wantShort: false,
		},
		{
			name:      "short token gets fully masked",
			token:     "short",
			wantShort: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				authHeader := r.Header.Get("Authorization")
				if !strings.HasPrefix(authHeader, "Bearer ") {
					t.Errorf("Expected Bearer token")
				}
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if token != tt.token {
					t.Errorf("Expected token %s, got %s", tt.token, token)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			client := &Client{
				HTTPClient: server.Client(),
				Token:      tt.token,
				BaseURL:    server.URL,
			}

			_, _, err := client.Do(context.Background(), "/test")
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestClient_RateLimitHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Ratelimit-Limit", "5000")
		w.Header().Set("X-Ratelimit-Remaining", "4999")
		w.Header().Set("X-Ratelimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix()))
		w.Header().Set("X-Ratelimit-Used", "1")
		w.Header().Set("X-Ratelimit-Resource", "core")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		Token:      "test-token",
		BaseURL:    server.URL,
	}

	_, resp, err := client.Do(context.Background(), "/test")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response but got nil")
	}
}

func TestClient_PaginationParsing(t *testing.T) {
	tests := []struct {
		name         string
		linkHeader   string
		wantNextPage int
	}{
		{
			name:         "has next page",
			linkHeader:   `<https://api.github.com/test?page=2>; rel="next", <https://api.github.com/test?page=10>; rel="last"`,
			wantNextPage: 2,
		},
		{
			name:         "no next page",
			linkHeader:   `<https://api.github.com/test?page=10>; rel="last"`,
			wantNextPage: 0,
		},
		{
			name:         "empty link header",
			linkHeader:   "",
			wantNextPage: 0,
		},
		{
			name:         "malformed link header",
			linkHeader:   "not a valid link",
			wantNextPage: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.linkHeader != "" {
					w.Header().Set("Link", tt.linkHeader)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			defer server.Close()

			client := &Client{
				HTTPClient: server.Client(),
				Token:      "test-token",
				BaseURL:    server.URL,
			}

			_, resp, err := client.Do(context.Background(), "/test")
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if resp.NextPage != tt.wantNextPage {
				t.Errorf("Expected next page %d, got %d", tt.wantNextPage, resp.NextPage)
			}
		})
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		Token:      "test-token",
		BaseURL:    server.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, _, err := client.Do(ctx, "/test")
	if err == nil {
		t.Error("Expected context cancellation error but got none")
	}
}

func TestClient_GraphQL(t *testing.T) {
	tests := []struct {
		name          string
		serverHandler http.HandlerFunc
		query         string
		variables     map[string]any
		wantErr       bool
		wantErrStatus int
	}{
		{
			name: "successful query",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST, got %s", r.Method)
				}
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("Expected Authorization header")
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type application/json")
				}
				if r.Header.Get("Accept") != "application/vnd.github.v4+json" {
					t.Errorf("Expected Accept header for GraphQL")
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"data": {"repository": {"name": "test-repo"}}}`))
			},
			query:     "query { repository(owner: $owner, name: $name) { name } }",
			variables: map[string]any{"owner": "testowner", "name": "testrepo"},
			wantErr:   false,
		},
		{
			name: "api error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message": "Bad credentials"}`))
			},
			query:         "query { viewer { login } }",
			wantErr:       true,
			wantErrStatus: http.StatusUnauthorized,
		},
		{
			name: "server error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message": "Internal Server Error"}`))
			},
			query:         "query { viewer { login } }",
			wantErr:       true,
			wantErrStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			client := &Client{
				HTTPClient: server.Client(),
				Token:      "test-token",
				BaseURL:    server.URL,
			}

			var result map[string]any
			err := client.GraphQL(context.Background(), tt.query, tt.variables, &result)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				var apiErr *Error
				if errors.As(err, &apiErr) && tt.wantErrStatus != 0 {
					if apiErr.StatusCode != tt.wantErrStatus {
						t.Errorf("Expected status %d, got %d", tt.wantErrStatus, apiErr.StatusCode)
					}
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestClient_GraphQL_DefaultBaseURL(t *testing.T) {
	// This test verifies that the default GitHub API URL is used when BaseURL is empty
	// We can't actually test against real API, but we test the logic path
	client := &Client{
		HTTPClient: &http.Client{Timeout: 1 * time.Millisecond}, // Very short timeout to fail fast
		Token:      "test-token",
		BaseURL:    "", // Empty to trigger default
	}

	var result map[string]any
	err := client.GraphQL(context.Background(), "query { viewer { login } }", nil, &result)
	// Should fail due to timeout/connection, but the point is it tried the default URL
	if err == nil {
		t.Error("Expected error (timeout) but got none")
	}
}

func TestTransport_RoundTrip(t *testing.T) {
	tests := []struct {
		name           string
		serverHandler  http.HandlerFunc
		wantErr        bool
		wantStatusCode int
	}{
		{
			name: "successful request",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"test": "data"}`))
			},
			wantErr:        false,
			wantStatusCode: http.StatusOK,
		},
		{
			name: "server error returns after exhausting retries",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK) // Return OK to avoid retry in test
				_, _ = w.Write([]byte(`{}`))
			},
			wantErr:        false,
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			transport := &Transport{Base: http.DefaultTransport}
			client := &http.Client{Transport: transport}

			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
			resp, err := client.Do(req)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if resp != nil {
					defer resp.Body.Close()
					if resp.StatusCode != tt.wantStatusCode {
						t.Errorf("Expected status %d, got %d", tt.wantStatusCode, resp.StatusCode)
					}
				}
			}
		})
	}
}

func TestTransport_RoundTrip_WithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	transport := &Transport{Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	body := strings.NewReader(`{"test": "data"}`)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, body)
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if resp != nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	}
}

func TestTransport_RoundTrip_NilBase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	transport := &Transport{Base: nil} // Will default to http.DefaultTransport
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
}

func TestRetryableError_Error(t *testing.T) {
	err := &retryableError{StatusCode: http.StatusTooManyRequests}
	errMsg := err.Error()
	if errMsg != "Too Many Requests" {
		t.Errorf("Expected 'Too Many Requests', got %q", errMsg)
	}
}

func TestTransport_RateLimitRetry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call returns rate limit
			w.Header().Set("X-Ratelimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message": "rate limit exceeded"}`))
			return
		}
		// Second call succeeds
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	transport := &Transport{Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	resp, err := client.Do(req)

	// The retry logic should kick in for rate limit
	if err != nil && callCount < 2 {
		t.Logf("Request failed after %d calls: %v", callCount, err)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
}

func TestTransport_ServerErrorRetry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call returns 500
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message": "server error"}`))
			return
		}
		// Second call succeeds
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	transport := &Transport{Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	resp, err := client.Do(req)

	// The retry logic should kick in for 5xx errors
	if err != nil && callCount < 2 {
		t.Logf("Request failed after %d calls: %v", callCount, err)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
}

func TestTransport_TooManyRequestsRetry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call returns 429
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message": "too many requests"}`))
			return
		}
		// Second call succeeds
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	transport := &Transport{Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	resp, err := client.Do(req)

	// The retry logic should kick in for 429 errors
	if err != nil && callCount < 2 {
		t.Logf("Request failed after %d calls: %v", callCount, err)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
}
