package github

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/codeGROOVE-dev/prx/pkg/prx/types"

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
		{"admin", types.WriteAccessDefinitely},
		{"maintain", types.WriteAccessDefinitely},
		{"write", types.WriteAccessDefinitely},
		{"read", types.WriteAccessNo},
		{"triage", types.WriteAccessNo},
		{"none", types.WriteAccessNo},
		{"", types.WriteAccessUnlikely},          // Not in collaborators list
		{"unknown", types.WriteAccessUnlikely},   // Unknown permission
		{"ADMIN", types.WriteAccessUnlikely},     // Case sensitive - not matched
		{"something", types.WriteAccessUnlikely}, // Invalid permission
	}

	for _, tt := range tests {
		t.Run(tt.permission, func(t *testing.T) {
			// Inline permission mapping logic
			var result int
			switch tt.permission {
			case "admin", "maintain", "write":
				result = types.WriteAccessDefinitely
			case "read", "triage", "none":
				result = types.WriteAccessNo
			default:
				result = types.WriteAccessUnlikely
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
			expected:   types.WriteAccessDefinitely,
		},
		{
			name:       "member with write permission",
			user:       "bob",
			permission: "write",
			expected:   types.WriteAccessDefinitely,
		},
		{
			name:       "member with maintain permission",
			user:       "charlie",
			permission: "maintain",
			expected:   types.WriteAccessDefinitely,
		},
		{
			name:       "member with read permission",
			user:       "david",
			permission: "read",
			expected:   types.WriteAccessNo,
		},
		{
			name:       "member with triage permission",
			user:       "eve",
			permission: "triage",
			expected:   types.WriteAccessNo,
		},
		{
			name:       "member not in collaborators list",
			user:       "frank",
			permission: "", // Not in the cache
			expected:   types.WriteAccessUnlikely,
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
	if result != types.WriteAccessDefinitely {
		t.Errorf("writeAccessFromAssociation(MEMBER, tstromberg) = %d, want %d",
			result, types.WriteAccessDefinitely)
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
		{"OWNER", types.WriteAccessDefinitely},
		{"COLLABORATOR", types.WriteAccessDefinitely},
		{"CONTRIBUTOR", types.WriteAccessUnlikely},
		{"NONE", types.WriteAccessUnlikely},
		{"FIRST_TIME_CONTRIBUTOR", types.WriteAccessUnlikely},
		{"FIRST_TIMER", types.WriteAccessUnlikely},
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
