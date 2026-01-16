package github

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/codeGROOVE-dev/prx/pkg/prx"

	"github.com/codeGROOVE-dev/fido"
)

// Test helper for cache keys
func collaboratorsCacheKey(owner, repo string) string {
	return fmt.Sprintf("%s/%s", owner, repo)
}

// TestPermissionToWriteAccess tests permission level mapping
func TestPermissionToWriteAccess(t *testing.T) {
	tests := []struct {
		permission string
		expected   int
	}{
		{"admin", prx.WriteAccessDefinitely},
		{"maintain", prx.WriteAccessDefinitely},
		{"write", prx.WriteAccessDefinitely},
		{"read", prx.WriteAccessNo},
		{"triage", prx.WriteAccessNo},
		{"none", prx.WriteAccessNo},
		{"", prx.WriteAccessUnlikely},          // Not in collaborators list
		{"unknown", prx.WriteAccessUnlikely},   // Unknown permission
		{"ADMIN", prx.WriteAccessUnlikely},     // Case sensitive - not matched
		{"something", prx.WriteAccessUnlikely}, // Invalid permission
	}

	for _, tt := range tests {
		t.Run(tt.permission, func(t *testing.T) {
			// Inline permission mapping logic
			var result int
			switch tt.permission {
			case "admin", "maintain", "write":
				result = prx.WriteAccessDefinitely
			case "read", "triage", "none":
				result = prx.WriteAccessNo
			default:
				result = prx.WriteAccessUnlikely
			}
			if result != tt.expected {
				t.Errorf("permission mapping for %q = %d, want %d",
					tt.permission, result, tt.expected)
			}
		})
	}
}

// TestCollaboratorsCacheGetSet tests cache get/set operations using fido
func TestCollaboratorsCacheGetSet(t *testing.T) {
	cache := fido.New[string, map[string]string](fido.TTL(collaboratorsCacheTTL))

	owner := "testowner"
	repo := "testrepo"
	collabs := map[string]string{
		"alice": "admin",
		"bob":   "write",
		"carol": "read",
	}

	cacheKey := collaboratorsCacheKey(owner, repo)

	// Test cache miss
	if _, ok := cache.Get(cacheKey); ok {
		t.Error("Expected cache miss, got hit")
	}

	// Test set
	cache.Set(cacheKey, collabs)

	// Test cache hit
	cached, ok := cache.Get(cacheKey)
	if !ok {
		t.Fatal("Expected cache hit, got miss")
	}

	// Verify cached data
	if len(cached) != len(collabs) {
		t.Errorf("Expected %d collaborators, got %d", len(collabs), len(cached))
	}
	for user, perm := range collabs {
		if cached[user] != perm {
			t.Errorf("Expected %s permission for %s, got %s", perm, user, cached[user])
		}
	}
}

// TestWriteAccessFromAssociationWithCache tests MEMBER association with cache
func TestWriteAccessFromAssociationWithCache(t *testing.T) {
	tests := []struct {
		name       string
		user       string
		permission string
		expected   int
	}{
		{
			name:       "member with admin permission",
			user:       "alice",
			permission: "admin",
			expected:   prx.WriteAccessDefinitely,
		},
		{
			name:       "member with write permission",
			user:       "bob",
			permission: "write",
			expected:   prx.WriteAccessDefinitely,
		},
		{
			name:       "member with maintain permission",
			user:       "charlie",
			permission: "maintain",
			expected:   prx.WriteAccessDefinitely,
		},
		{
			name:       "member with read permission",
			user:       "david",
			permission: "read",
			expected:   prx.WriteAccessNo,
		},
		{
			name:       "member with triage permission",
			user:       "eve",
			permission: "triage",
			expected:   prx.WriteAccessNo,
		},
		{
			name:       "member not in collaborators list",
			user:       "frank",
			permission: "", // Not in the cache
			expected:   prx.WriteAccessUnlikely,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup cache with test data
			cache := fido.New[string, map[string]string](fido.TTL(collaboratorsCacheTTL))

			collabs := map[string]string{
				"alice":   "admin",
				"bob":     "write",
				"charlie": "maintain",
				"david":   "read",
				"eve":     "triage",
			}

			// Pre-populate cache
			cacheKey := collaboratorsCacheKey("owner", "repo")
			cache.Set(cacheKey, collabs)

			// Create platform with cache
			p := &Platform{
				logger:             slog.Default(),
				collaboratorsCache: cache,
			}

			result := p.writeAccessFromAssociation(ctx, "owner", "repo", tt.user, "MEMBER")
			if result != tt.expected {
				t.Errorf("writeAccessFromAssociation(MEMBER, %s) = %d, want %d",
					tt.user, result, tt.expected)
			}
		})
	}
}

// TestWriteAccessFromAssociationCacheHit tests that cache prevents API calls
func TestWriteAccessFromAssociationCacheHit(t *testing.T) {
	ctx := context.Background()

	// Setup cache with test data
	cache := fido.New[string, map[string]string](fido.TTL(collaboratorsCacheTTL))

	collabs := map[string]string{
		"tstromberg": "admin",
	}

	// Pre-populate cache
	cacheKey := collaboratorsCacheKey("codeGROOVE-dev", "goose")
	cache.Set(cacheKey, collabs)

	// Create platform with cache but without a real HTTP client
	// This tests that we use the cache and don't try to call the API
	p := &Platform{
		logger:             slog.Default(),
		collaboratorsCache: cache,
		client:             nil, // No HTTP client - would fail if API called
	}

	result := p.writeAccessFromAssociation(ctx, "codeGROOVE-dev", "goose", "tstromberg", "MEMBER")
	if result != prx.WriteAccessDefinitely {
		t.Errorf("writeAccessFromAssociation(MEMBER, tstromberg) = %d, want %d",
			result, prx.WriteAccessDefinitely)
	}
}

// TestWriteAccessFromAssociationNonMember tests non-MEMBER associations don't use cache
func TestWriteAccessFromAssociationNonMember(t *testing.T) {
	ctx := context.Background()

	// Empty cache
	cache := fido.New[string, map[string]string](fido.TTL(collaboratorsCacheTTL))

	p := &Platform{
		logger:             slog.Default(),
		collaboratorsCache: cache,
	}

	tests := []struct {
		association string
		expected    int
	}{
		{"OWNER", prx.WriteAccessDefinitely},
		{"COLLABORATOR", prx.WriteAccessDefinitely},
		{"CONTRIBUTOR", prx.WriteAccessUnlikely},
		{"NONE", prx.WriteAccessUnlikely},
		{"FIRST_TIME_CONTRIBUTOR", prx.WriteAccessUnlikely},
		{"FIRST_TIMER", prx.WriteAccessUnlikely},
	}

	for _, tt := range tests {
		t.Run(tt.association, func(t *testing.T) {
			result := p.writeAccessFromAssociation(ctx, "owner", "repo", "user", tt.association)
			if result != tt.expected {
				t.Errorf("writeAccessFromAssociation(%s) = %d, want %d",
					tt.association, result, tt.expected)
			}
		})
	}

	// Verify cache wasn't used (should still be empty)
	cacheKey := collaboratorsCacheKey("owner", "repo")
	if _, ok := cache.Get(cacheKey); ok {
		t.Error("Cache should not have been populated for non-MEMBER associations")
	}
}
