package main

import (
	"sort"

	"github.com/maruel/natural"
)

// SortStrategy defines the interface for different sorting strategies
type SortStrategy interface {
	// Sort returns a new sorted slice without modifying the original
	Sort(images []ImagePath) []ImagePath
	// Name returns the human-readable name of the strategy
	Name() string
	// ID returns the numeric identifier for config storage
	ID() int
}

// NaturalSortStrategy implements natural sorting using maruel/natural
type NaturalSortStrategy struct{}

func (s *NaturalSortStrategy) Sort(images []ImagePath) []ImagePath {
	if len(images) == 0 {
		return []ImagePath{}
	}

	// Create a copy to avoid modifying the original
	result := make([]ImagePath, len(images))
	copy(result, images)

	sort.Slice(result, func(i, j int) bool {
		return natural.Less(result[i].Path, result[j].Path)
	})

	return result
}

func (s *NaturalSortStrategy) Name() string {
	return "Natural"
}

func (s *NaturalSortStrategy) ID() int {
	return SortNatural
}

// SimpleSortStrategy implements lexicographical sorting
type SimpleSortStrategy struct{}

func (s *SimpleSortStrategy) Sort(images []ImagePath) []ImagePath {
	if len(images) == 0 {
		return []ImagePath{}
	}

	// Create a copy to avoid modifying the original
	result := make([]ImagePath, len(images))
	copy(result, images)

	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})

	return result
}

func (s *SimpleSortStrategy) Name() string {
	return "Simple"
}

func (s *SimpleSortStrategy) ID() int {
	return SortSimple
}

// EntryOrderSortStrategy preserves the original order
type EntryOrderSortStrategy struct{}

func (s *EntryOrderSortStrategy) Sort(images []ImagePath) []ImagePath {
	if len(images) == 0 {
		return []ImagePath{}
	}

	// Create a copy to avoid modifying the original
	result := make([]ImagePath, len(images))
	copy(result, images)

	return result
}

func (s *EntryOrderSortStrategy) Name() string {
	return "Entry Order"
}

func (s *EntryOrderSortStrategy) ID() int {
	return SortEntryOrder
}

// GetSortStrategy returns the appropriate strategy based on the sort method ID
func GetSortStrategy(sortMethod int) SortStrategy {
	switch sortMethod {
	case SortNatural:
		return &NaturalSortStrategy{}
	case SortSimple:
		return &SimpleSortStrategy{}
	case SortEntryOrder:
		return &EntryOrderSortStrategy{}
	default:
		return &NaturalSortStrategy{} // Default fallback
	}
}

// GetAllSortStrategies returns all available sort strategies
func GetAllSortStrategies() []SortStrategy {
	return []SortStrategy{
		&NaturalSortStrategy{},
		&SimpleSortStrategy{},
		&EntryOrderSortStrategy{},
	}
}
