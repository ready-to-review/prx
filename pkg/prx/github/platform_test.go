//nolint:errcheck // Test handlers don't need to check w.Write errors
package github

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestWithLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	platform := NewPlatform("test-token", WithLogger(logger))

	if platform.logger != logger {
		t.Error("Expected custom logger to be set")
	}
}

func TestWithHTTPClient(t *testing.T) {
	customClient := &http.Client{
		Timeout: 60,
	}

	platform := NewPlatform("test-token", WithHTTPClient(customClient))

	if platform.client.HTTPClient == nil {
		t.Error("Expected HTTP client to be set")
	}

	// Verify transport was wrapped
	if _, ok := platform.client.HTTPClient.Transport.(*Transport); !ok {
		t.Error("Expected transport to be wrapped with retry Transport")
	}
}

func TestWithHTTPClient_ExistingTransport(t *testing.T) {
	customTransport := &http.Transport{}
	customClient := &http.Client{
		Transport: customTransport,
	}

	platform := NewPlatform("test-token", WithHTTPClient(customClient))

	// Verify existing transport was wrapped
	if transport, ok := platform.client.HTTPClient.Transport.(*Transport); !ok {
		t.Error("Expected transport to be wrapped")
	} else if transport.Base != customTransport {
		t.Error("Expected base transport to be the custom transport")
	}
}

func TestWithHTTPClient_AlreadyWrapped(t *testing.T) {
	wrappedTransport := &Transport{Base: http.DefaultTransport}
	customClient := &http.Client{
		Transport: wrappedTransport,
	}

	platform := NewPlatform("test-token", WithHTTPClient(customClient))

	// Verify transport is not double-wrapped
	if transport, ok := platform.client.HTTPClient.Transport.(*Transport); !ok {
		t.Error("Expected transport to remain as Transport")
	} else if transport != wrappedTransport {
		t.Error("Expected transport to not be double-wrapped")
	}
}

func TestCalculateTestStateFromGraphQL(t *testing.T) {
	tests := []struct {
		name      string
		data      *graphQLPullRequestComplete
		wantState string
	}{
		{
			name:      "no status check rollup",
			data:      &graphQLPullRequestComplete{},
			wantState: "",
		},
		{
			name: "failing test",
			data: &graphQLPullRequestComplete{
				HeadRef: makeHeadRef([]graphQLStatusCheckNode{
					{
						TypeName:   "CheckRun",
						Name:       "unit-tests",
						Status:     "completed",
						Conclusion: "failure",
					},
				}),
			},
			wantState: "failing",
		},
		{
			name: "running test",
			data: &graphQLPullRequestComplete{
				HeadRef: makeHeadRef([]graphQLStatusCheckNode{
					{
						TypeName: "CheckRun",
						Name:     "test-check",
						Status:   "in_progress",
					},
				}),
			},
			wantState: "running",
		},
		{
			name: "queued test",
			data: &graphQLPullRequestComplete{
				HeadRef: makeHeadRef([]graphQLStatusCheckNode{
					{
						TypeName: "CheckRun",
						Name:     "CI-test",
						Status:   "queued",
					},
				}),
			},
			wantState: "queued",
		},
		{
			name: "passing tests",
			data: &graphQLPullRequestComplete{
				HeadRef: makeHeadRef([]graphQLStatusCheckNode{
					{
						TypeName:   "CheckRun",
						Name:       "test-suite",
						Status:     "completed",
						Conclusion: "success",
					},
				}),
			},
			wantState: "passing",
		},
		{
			name: "non-test check run ignored",
			data: &graphQLPullRequestComplete{
				HeadRef: makeHeadRef([]graphQLStatusCheckNode{
					{
						TypeName:   "CheckRun",
						Name:       "lint",
						Status:     "completed",
						Conclusion: "failure",
					},
				}),
			},
			wantState: "passing",
		},
		{
			name: "non-CheckRun type ignored",
			data: &graphQLPullRequestComplete{
				HeadRef: makeHeadRef([]graphQLStatusCheckNode{
					{
						TypeName:   "StatusContext",
						Name:       "test-status",
						Status:     "completed",
						Conclusion: "failure",
					},
				}),
			},
			wantState: "passing",
		},
		{
			name: "timed out test",
			data: &graphQLPullRequestComplete{
				HeadRef: makeHeadRef([]graphQLStatusCheckNode{
					{
						TypeName:   "CheckRun",
						Name:       "test-timeout",
						Status:     "completed",
						Conclusion: "timed_out",
					},
				}),
			},
			wantState: "failing",
		},
		{
			name: "action required test",
			data: &graphQLPullRequestComplete{
				HeadRef: makeHeadRef([]graphQLStatusCheckNode{
					{
						TypeName:   "CheckRun",
						Name:       "check-required",
						Status:     "completed",
						Conclusion: "action_required",
					},
				}),
			},
			wantState: "failing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Platform{}
			state := p.calculateTestStateFromGraphQL(tt.data)
			if state != tt.wantState {
				t.Errorf("Expected state %q, got %q", tt.wantState, state)
			}
		})
	}
}

// Helper function to create HeadRef with status check nodes
func makeHeadRef(nodes []graphQLStatusCheckNode) struct {
	Target struct {
		StatusCheckRollup *struct {
			Contexts struct {
				Nodes []graphQLStatusCheckNode `json:"nodes"`
			} `json:"contexts"`
			State string `json:"state"`
		} `json:"statusCheckRollup"`
		OID string `json:"oid"`
	} `json:"target"`
	Name string `json:"name"`
} {
	return struct {
		Target struct {
			StatusCheckRollup *struct {
				Contexts struct {
					Nodes []graphQLStatusCheckNode `json:"nodes"`
				} `json:"contexts"`
				State string `json:"state"`
			} `json:"statusCheckRollup"`
			OID string `json:"oid"`
		} `json:"target"`
		Name string `json:"name"`
	}{
		Target: struct {
			StatusCheckRollup *struct {
				Contexts struct {
					Nodes []graphQLStatusCheckNode `json:"nodes"`
				} `json:"contexts"`
				State string `json:"state"`
			} `json:"statusCheckRollup"`
			OID string `json:"oid"`
		}{
			StatusCheckRollup: &struct {
				Contexts struct {
					Nodes []graphQLStatusCheckNode `json:"nodes"`
				} `json:"contexts"`
				State string `json:"state"`
			}{
				Contexts: struct {
					Nodes []graphQLStatusCheckNode `json:"nodes"`
				}{
					Nodes: nodes,
				},
			},
		},
	}
}

func TestExecuteGraphQL_ErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		serverHandler http.HandlerFunc
		wantErr       bool
	}{
		{
			name: "graphql error in response",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"errors": [{"message": "Field 'badField' doesn't exist"}]}`))
			},
			wantErr: true,
		},
		{
			name: "successful response with pr data",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"data": {
						"repository": {
							"pullRequest": {
								"id": "PR_123",
								"number": 1,
								"title": "Test PR",
								"body": "Test body",
								"state": "OPEN",
								"mergeable": "MERGEABLE",
								"mergeStateStatus": "CLEAN",
								"authorAssociation": "OWNER",
								"isDraft": false,
								"additions": 10,
								"deletions": 5,
								"changedFiles": 2,
								"createdAt": "2024-01-01T00:00:00Z",
								"updatedAt": "2024-01-02T00:00:00Z",
								"author": {"login": "testuser"},
								"assignees": {"nodes": []},
								"labels": {"nodes": []},
								"reviewRequests": {"nodes": []},
								"baseRef": {
									"name": "main",
									"target": {"oid": "abc123"}
								},
								"headRef": {
									"name": "feature",
									"target": {"oid": "def456"}
								},
								"reviews": {"nodes": []},
								"timelineItems": {"nodes": [], "pageInfo": {"hasNextPage": false}},
								"latestReviews": {"nodes": []}
							}
						},
						"rateLimit": {
							"cost": 1,
							"remaining": 5000,
							"limit": 5000,
							"resetAt": "2024-01-01T01:00:00Z"
						}
					}
				}`))
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			platform := NewTestPlatform("test-token", server.URL)

			_, err := platform.executeGraphQL(context.Background(), "owner", "repo", 1)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestTruncateSHA(t *testing.T) {
	tests := []struct {
		name    string
		sha     string
		wantSHA string
	}{
		{
			name:    "long sha",
			sha:     "1234567890abcdef",
			wantSHA: "1234567",
		},
		{
			name:    "short sha",
			sha:     "12345",
			wantSHA: "12345",
		},
		{
			name:    "exactly 7 chars",
			sha:     "1234567",
			wantSHA: "1234567",
		},
		{
			name:    "empty sha",
			sha:     "",
			wantSHA: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateSHA(tt.sha)
			if result != tt.wantSHA {
				t.Errorf("Expected %q, got %q", tt.wantSHA, result)
			}
		})
	}
}
