package navlogic

import "testing"

func lookupFromSlice(metrics []PageMetrics) MetricsLookup {
	return func(idx int) PageMetrics {
		if idx < 0 || idx >= len(metrics) {
			return PageMetrics{}
		}
		return metrics[idx]
	}
}

func TestShouldUseBookMode(t *testing.T) {
	tests := []struct {
		name     string
		left     PageMetrics
		right    PageMetrics
		learned  []float64
		expected bool
	}{
		{"same aspect ratio", PageMetrics{Width: 100, Height: 150}, PageMetrics{Width: 100, Height: 150}, nil, true},
		{"similar aspect ratio", PageMetrics{Width: 100, Height: 150}, PageMetrics{Width: 120, Height: 180}, nil, true},
		{"square pages still pair", PageMetrics{Width: 100, Height: 100}, PageMetrics{Width: 100, Height: 100}, nil, true},
		{"wide single pages still pair by default", PageMetrics{Width: 200, Height: 150}, PageMetrics{Width: 210, Height: 150}, nil, true},
		{"learned spread ratio blocks pairing", PageMetrics{Width: 200, Height: 150}, PageMetrics{Width: 210, Height: 150}, []float64{1.34}, false},
		{"only one side near learned spread ratio still pairs", PageMetrics{Width: 200, Height: 150}, PageMetrics{Width: 160, Height: 150}, []float64{1.34}, true},
		{"very different aspect ratio", PageMetrics{Width: 100, Height: 150}, PageMetrics{Width: 300, Height: 100}, nil, false},
		{"missing page", PageMetrics{Width: 100, Height: 150}, PageMetrics{}, nil, false},
		{"extremely tall image", PageMetrics{Width: 100, Height: 1000}, PageMetrics{Width: 100, Height: 150}, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldUseBookMode(tt.left, tt.right, 1.5, tt.learned); got != tt.expected {
				t.Fatalf("ShouldUseBookMode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPlanDisplay(t *testing.T) {
	metrics := []PageMetrics{
		{Width: 100, Height: 150},
		{Width: 100, Height: 150},
		{Width: 100, Height: 150},
	}

	t.Run("single mode", func(t *testing.T) {
		plan := PlanDisplay(State{Index: 1, PageCount: len(metrics)}, lookupFromSlice(metrics))
		if plan.LeftIndex != 1 || plan.RightIndex != -1 || plan.ActualImages != 1 {
			t.Fatalf("unexpected single plan: %+v", plan)
		}
	})

	t.Run("book mode ltr", func(t *testing.T) {
		plan := PlanDisplay(State{
			Index:                0,
			PageCount:            len(metrics),
			BookMode:             true,
			AspectRatioThreshold: 1.5,
		}, lookupFromSlice(metrics))
		if plan.LeftIndex != 0 || plan.RightIndex != 1 || plan.ActualImages != 2 {
			t.Fatalf("unexpected ltr plan: %+v", plan)
		}
	})

	t.Run("book mode rtl", func(t *testing.T) {
		plan := PlanDisplay(State{
			Index:                0,
			PageCount:            len(metrics),
			BookMode:             true,
			RightToLeft:          true,
			AspectRatioThreshold: 1.5,
		}, lookupFromSlice(metrics))
		if plan.LeftIndex != 1 || plan.RightIndex != 0 || plan.ActualImages != 2 {
			t.Fatalf("unexpected rtl plan: %+v", plan)
		}
	})

	t.Run("incompatible spread falls back to single", func(t *testing.T) {
		metrics[1] = PageMetrics{Width: 300, Height: 100}
		plan := PlanDisplay(State{
			Index:                0,
			PageCount:            len(metrics),
			BookMode:             true,
			AspectRatioThreshold: 1.5,
			LearnedSpreadAspects: []float64{1.34},
		}, lookupFromSlice(metrics))
		if plan.LeftIndex != 0 || plan.RightIndex != -1 || plan.ActualImages != 1 {
			t.Fatalf("unexpected fallback plan: %+v", plan)
		}
	})
}

func TestSetCurrentIndexAndJumpToPage(t *testing.T) {
	metrics := []PageMetrics{
		{Width: 100, Height: 150},
		{Width: 100, Height: 150},
		{Width: 100, Height: 150},
	}
	lookup := lookupFromSlice(metrics)

	state := SetCurrentIndex(State{
		PageCount:            len(metrics),
		BookMode:             true,
		AspectRatioThreshold: 1.5,
	}, 2, lookup)
	if state.Index != 1 || state.TempSingleMode {
		t.Fatalf("expected final page to shift into book pair, got %+v", state)
	}

	incompatible := []PageMetrics{
		{Width: 100, Height: 150},
		{Width: 300, Height: 100},
		{Width: 100, Height: 150},
	}
	state = SetCurrentIndex(State{
		PageCount:            len(incompatible),
		BookMode:             true,
		AspectRatioThreshold: 1.5,
	}, 2, lookupFromSlice(incompatible))
	if state.Index != 2 || !state.TempSingleMode || state.BookMode {
		t.Fatalf("expected incompatible final page temp-single fallback, got %+v", state)
	}

	state, boundary := JumpToPage(State{PageCount: len(metrics)}, 4, lookup)
	if boundary != BoundaryPageNotFound {
		t.Fatalf("expected BoundaryPageNotFound, got %v (%+v)", boundary, state)
	}
}

func TestNavigateNextAndPrevious(t *testing.T) {
	metrics := []PageMetrics{
		{Width: 100, Height: 150},
		{Width: 100, Height: 150},
		{Width: 100, Height: 150},
		{Width: 100, Height: 150},
		{Width: 100, Height: 150},
	}
	lookup := lookupFromSlice(metrics)

	state, boundary := NavigateNext(State{PageCount: len(metrics), BookMode: true, AspectRatioThreshold: 1.5}, lookup, false)
	if boundary != BoundaryNone || state.Index != 2 {
		t.Fatalf("expected spread advance by 2, got boundary=%v state=%+v", boundary, state)
	}

	state, boundary = NavigateNext(State{
		Index:                2,
		PageCount:            len(metrics),
		BookMode:             true,
		AspectRatioThreshold: 1.5,
	}, lookup, false)
	if boundary != BoundaryNone || state.Index != 4 || !state.TempSingleMode || state.BookMode {
		t.Fatalf("expected temp single final page, got boundary=%v state=%+v", boundary, state)
	}

	state, boundary = NavigatePrevious(State{
		Index:          4,
		PageCount:      len(metrics),
		BookMode:       false,
		TempSingleMode: true,
	}, lookup, false)
	if boundary != BoundaryNone || state.Index != 2 || !state.BookMode || state.TempSingleMode {
		t.Fatalf("expected temp-single previous to restore book spread, got boundary=%v state=%+v", boundary, state)
	}

	_, boundary = NavigatePrevious(State{PageCount: len(metrics)}, lookup, false)
	if boundary != BoundaryFirstPage {
		t.Fatalf("expected BoundaryFirstPage, got %v", boundary)
	}
}

func TestToggleBookMode(t *testing.T) {
	metrics := []PageMetrics{
		{Width: 100, Height: 150},
		{Width: 100, Height: 150},
		{Width: 100, Height: 150},
	}
	state := ToggleBookMode(State{
		Index:                2,
		PageCount:            len(metrics),
		AspectRatioThreshold: 1.5,
	}, lookupFromSlice(metrics))
	if !state.BookMode || state.TempSingleMode || state.Index != 1 {
		t.Fatalf("expected final page to pair backward when enabling book mode, got %+v", state)
	}

	singlePage := ToggleBookMode(State{
		PageCount:            1,
		AspectRatioThreshold: 1.5,
	}, lookupFromSlice([]PageMetrics{{Width: 100, Height: 150}}))
	if !singlePage.BookMode || !singlePage.TempSingleMode {
		t.Fatalf("expected single-page temp single mode, got %+v", singlePage)
	}
}
