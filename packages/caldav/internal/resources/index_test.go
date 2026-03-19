package resources

import (
	"sort"
	"testing"

	"github.com/amarbel-llc/bob/packages/caldav/internal/caldav"
)

func TestWordIndex_BuildAndSearch(t *testing.T) {
	idx := NewWordIndex()

	tasks := []caldav.Task{
		{UID: "1", Summary: "Buy groceries", Description: "Milk and bread", Categories: []string{"Shopping"}},
		{UID: "2", Summary: "Fix the kitchen sink", Categories: []string{"Home"}},
		{UID: "3", Summary: "Buy new shoes", Categories: []string{"Shopping"}},
	}

	idx.Build(tasks)

	t.Run("exact match", func(t *testing.T) {
		results := idx.Search("buy")
		sort.Strings(results)
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d: %v", len(results), results)
		}
		if results[0] != "1" || results[1] != "3" {
			t.Errorf("expected [1, 3], got %v", results)
		}
	})

	t.Run("category search", func(t *testing.T) {
		results := idx.Search("shopping")
		sort.Strings(results)
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d: %v", len(results), results)
		}
	})

	t.Run("prefix match", func(t *testing.T) {
		results := idx.Search("groc")
		if len(results) != 1 || results[0] != "1" {
			t.Errorf("expected [1], got %v", results)
		}
	})

	t.Run("no match", func(t *testing.T) {
		results := idx.Search("nonexistent")
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %v", results)
		}
	})

	t.Run("stop words filtered", func(t *testing.T) {
		results := idx.Search("the")
		if len(results) != 0 {
			t.Errorf("stop word 'the' should return 0 results, got %v", results)
		}
	})

	t.Run("short words filtered", func(t *testing.T) {
		results := idx.Search("an")
		if len(results) != 0 {
			t.Errorf("short word 'an' should return 0 results, got %v", results)
		}
	})
}

func TestWordIndex_Empty(t *testing.T) {
	idx := NewWordIndex()
	results := idx.Search("anything")
	if len(results) != 0 {
		t.Errorf("empty index should return 0 results")
	}
}
