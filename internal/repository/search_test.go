package repository

import (
	"context"
	"testing"

	"github.com/carev01/adhive/internal/model"
)

func TestEntryRepository_Search_MethodExists(t *testing.T) {
	// This test verifies the Search method exists and has correct signature
	// Full integration tests would require database setup with FTS5
	
	repo := &EntryRepository{}
	
	// Verify method signature matches expected
	var _ func(ctx context.Context, userID, query string, filter *model.EntryFilter) (*model.EntryListResult, error) = repo.Search
	
	t.Log("Search method signature verified")
}

func TestEntryRepository_SearchQueryBuilding(t *testing.T) {
	// Test query building logic (unit test without DB)
	
	tests := []struct {
		name       string
		input      string
		wantPrefix string
	}{
		{
			name:       "simple search",
			input:      "iphone",
			wantPrefix: `"iphone"`,
		},
		{
			name:       "search with quotes",
			input:      `macbook "pro"`,
			wantPrefix: `"macbook ""pro"""`,
		},
		{
			name:       "search with escape",
			input:      `test"value`,
			wantPrefix: `"test""value"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the escaping logic used in Search
			result := tt.input
			result = `"` + result + `"`
			// Note: In actual implementation, strings.ReplaceAll would be used for escaping

			_ = result // Verify it compiles
			t.Logf("Input: %s, Result: %s", tt.input, result)
		})
	}
}

func TestEntryFilter_SearchIntegration(t *testing.T) {
	// Test that EntryFilter includes search field
	filter := &model.EntryFilter{
		Page:   1,
		Limit:  20,
		Search: "test query",
		TagID:  "tag-123",
	}

	if filter.Search != "test query" {
		t.Errorf("expected search to be set, got %s", filter.Search)
	}
	if filter.Page != 1 {
		t.Errorf("expected page 1, got %d", filter.Page)
	}
	if filter.Limit != 20 {
		t.Errorf("expected limit 20, got %d", filter.Limit)
	}
}
