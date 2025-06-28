package main

import (
	"reflect"
	"testing"
)

// Test data for sorting strategies
func getTestImagePaths() []ImagePath {
	return []ImagePath{
		{Path: "test/01.png", ArchivePath: "", EntryPath: ""},
		{Path: "test/04.zip", ArchivePath: "", EntryPath: ""},
		{Path: "test/08.png", ArchivePath: "", EntryPath: ""},
		{Path: "test/09.png", ArchivePath: "", EntryPath: ""},
		{Path: "test/2.png", ArchivePath: "", EntryPath: ""},
		{Path: "test/３.png", ArchivePath: "", EntryPath: ""},
	}
}

func getExpectedNaturalOrder() []ImagePath {
	return []ImagePath{
		{Path: "test/01.png", ArchivePath: "", EntryPath: ""},
		{Path: "test/2.png", ArchivePath: "", EntryPath: ""},
		{Path: "test/04.zip", ArchivePath: "", EntryPath: ""},
		{Path: "test/08.png", ArchivePath: "", EntryPath: ""},
		{Path: "test/09.png", ArchivePath: "", EntryPath: ""},
		{Path: "test/３.png", ArchivePath: "", EntryPath: ""},
	}
}

func getExpectedSimpleOrder() []ImagePath {
	return []ImagePath{
		{Path: "test/01.png", ArchivePath: "", EntryPath: ""},
		{Path: "test/04.zip", ArchivePath: "", EntryPath: ""},
		{Path: "test/08.png", ArchivePath: "", EntryPath: ""},
		{Path: "test/09.png", ArchivePath: "", EntryPath: ""},
		{Path: "test/2.png", ArchivePath: "", EntryPath: ""},
		{Path: "test/３.png", ArchivePath: "", EntryPath: ""},
	}
}

func TestNaturalSortStrategy(t *testing.T) {
	strategy := &NaturalSortStrategy{}

	t.Run("Name", func(t *testing.T) {
		if strategy.Name() != "Natural" {
			t.Errorf("Expected 'Natural', got '%s'", strategy.Name())
		}
	})

	t.Run("ID", func(t *testing.T) {
		if strategy.ID() != SortNatural {
			t.Errorf("Expected %d, got %d", SortNatural, strategy.ID())
		}
	})

	t.Run("Sort", func(t *testing.T) {
		input := getTestImagePaths()
		expected := getExpectedNaturalOrder()
		result := strategy.Sort(input)

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Natural sort failed")
			t.Logf("Expected: %v", pathsToStrings(expected))
			t.Logf("Got:      %v", pathsToStrings(result))
		}
	})

	t.Run("ImmutableInput", func(t *testing.T) {
		input := getTestImagePaths()
		original := make([]ImagePath, len(input))
		copy(original, input)

		_ = strategy.Sort(input)

		if !reflect.DeepEqual(input, original) {
			t.Error("Input slice was modified - should be immutable")
		}
	})

	t.Run("EmptySlice", func(t *testing.T) {
		result := strategy.Sort([]ImagePath{})
		if len(result) != 0 {
			t.Errorf("Expected empty slice, got %v", result)
		}
	})
}

func TestSimpleSortStrategy(t *testing.T) {
	strategy := &SimpleSortStrategy{}

	t.Run("Name", func(t *testing.T) {
		if strategy.Name() != "Simple" {
			t.Errorf("Expected 'Simple', got '%s'", strategy.Name())
		}
	})

	t.Run("ID", func(t *testing.T) {
		if strategy.ID() != SortSimple {
			t.Errorf("Expected %d, got %d", SortSimple, strategy.ID())
		}
	})

	t.Run("Sort", func(t *testing.T) {
		input := getTestImagePaths()
		expected := getExpectedSimpleOrder()
		result := strategy.Sort(input)

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Simple sort failed")
			t.Logf("Expected: %v", pathsToStrings(expected))
			t.Logf("Got:      %v", pathsToStrings(result))
		}
	})

	t.Run("ImmutableInput", func(t *testing.T) {
		input := getTestImagePaths()
		original := make([]ImagePath, len(input))
		copy(original, input)

		_ = strategy.Sort(input)

		if !reflect.DeepEqual(input, original) {
			t.Error("Input slice was modified - should be immutable")
		}
	})
}

func TestEntryOrderSortStrategy(t *testing.T) {
	strategy := &EntryOrderSortStrategy{}

	t.Run("Name", func(t *testing.T) {
		if strategy.Name() != "Entry Order" {
			t.Errorf("Expected 'Entry Order', got '%s'", strategy.Name())
		}
	})

	t.Run("ID", func(t *testing.T) {
		if strategy.ID() != SortEntryOrder {
			t.Errorf("Expected %d, got %d", SortEntryOrder, strategy.ID())
		}
	})

	t.Run("Sort", func(t *testing.T) {
		input := getTestImagePaths()
		expected := getTestImagePaths() // Should maintain original order
		result := strategy.Sort(input)

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Entry order sort failed")
			t.Logf("Expected: %v", pathsToStrings(expected))
			t.Logf("Got:      %v", pathsToStrings(result))
		}
	})

	t.Run("ImmutableInput", func(t *testing.T) {
		input := getTestImagePaths()
		original := make([]ImagePath, len(input))
		copy(original, input)

		_ = strategy.Sort(input)

		if !reflect.DeepEqual(input, original) {
			t.Error("Input slice was modified - should be immutable")
		}
	})
}

func TestGetSortStrategy(t *testing.T) {
	tests := []struct {
		sortMethod   int
		expectedID   int
		expectedName string
	}{
		{SortNatural, SortNatural, "Natural"},
		{SortSimple, SortSimple, "Simple"},
		{SortEntryOrder, SortEntryOrder, "Entry Order"},
		{999, SortNatural, "Natural"}, // Default fallback
	}

	for _, tt := range tests {
		t.Run(tt.expectedName, func(t *testing.T) {
			strategy := GetSortStrategy(tt.sortMethod)

			if strategy.ID() != tt.expectedID {
				t.Errorf("Expected ID %d, got %d", tt.expectedID, strategy.ID())
			}

			if strategy.Name() != tt.expectedName {
				t.Errorf("Expected name '%s', got '%s'", tt.expectedName, strategy.Name())
			}
		})
	}
}

func TestGetAllSortStrategies(t *testing.T) {
	strategies := GetAllSortStrategies()

	if len(strategies) != 3 {
		t.Errorf("Expected 3 strategies, got %d", len(strategies))
	}

	// Check that all expected strategies are present
	expectedNames := []string{"Natural", "Simple", "Entry Order"}
	var actualNames []string
	for _, strategy := range strategies {
		actualNames = append(actualNames, strategy.Name())
	}

	for _, expected := range expectedNames {
		found := false
		for _, actual := range actualNames {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected strategy '%s' not found in %v", expected, actualNames)
		}
	}
}

// Test edge cases
func TestSortStrategyEdgeCases(t *testing.T) {
	strategies := GetAllSortStrategies()

	t.Run("SingleElement", func(t *testing.T) {
		single := []ImagePath{{Path: "test/single.png"}}

		for _, strategy := range strategies {
			result := strategy.Sort(single)
			if len(result) != 1 || result[0].Path != "test/single.png" {
				t.Errorf("Strategy %s failed on single element", strategy.Name())
			}
		}
	})

	t.Run("IdenticalPaths", func(t *testing.T) {
		identical := []ImagePath{
			{Path: "test/same.png"},
			{Path: "test/same.png"},
			{Path: "test/same.png"},
		}

		for _, strategy := range strategies {
			result := strategy.Sort(identical)
			if len(result) != 3 {
				t.Errorf("Strategy %s changed length on identical paths", strategy.Name())
			}
			for _, path := range result {
				if path.Path != "test/same.png" {
					t.Errorf("Strategy %s changed identical paths", strategy.Name())
				}
			}
		}
	})
}

// Helper function to convert ImagePath slice to string slice for easier debugging
func pathsToStrings(paths []ImagePath) []string {
	var strings []string
	for _, path := range paths {
		strings = append(strings, path.Path)
	}
	return strings
}
