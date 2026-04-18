package navlogic

import (
	"fmt"
	"math/rand"
	"testing"
)

func lookupFromSlice(metrics []PageMetrics) MetricsLookup {
	return func(idx int) PageMetrics {
		if idx < 0 || idx >= len(metrics) {
			return PageMetrics{}
		}
		return metrics[idx]
	}
}

type testPageKind int

const (
	testPagePairable testPageKind = iota
	testPageForcedSingle
)

func (k testPageKind) String() string {
	switch k {
	case testPagePairable:
		return "pairable"
	case testPageForcedSingle:
		return "forcedSingle"
	default:
		return fmt.Sprintf("unknown(%d)", int(k))
	}
}

func metricsFromKinds(kinds []testPageKind) []PageMetrics {
	metrics := make([]PageMetrics, len(kinds))
	for i, kind := range kinds {
		switch kind {
		case testPagePairable:
			metrics[i] = PageMetrics{Width: 100, Height: 150}
		case testPageForcedSingle:
			metrics[i] = PageMetrics{Width: 100, Height: 1000}
		default:
			panic(fmt.Sprintf("unsupported page kind: %d", kind))
		}
	}
	return metrics
}

func kindsString(kinds []testPageKind) string {
	return fmt.Sprint(kinds)
}

func displayedIndicesForTest(t *testing.T, plan DisplayPlan, pageCount int) []int {
	t.Helper()

	displayed := make([]int, 0, 2)
	if plan.LeftIndex >= 0 {
		displayed = append(displayed, plan.LeftIndex)
	}
	if plan.RightIndex >= 0 {
		displayed = append(displayed, plan.RightIndex)
	}

	if len(displayed) != plan.ActualImages {
		t.Fatalf("ActualImages mismatch: plan=%+v displayed=%v", plan, displayed)
	}

	seen := make(map[int]struct{}, len(displayed))
	for _, idx := range displayed {
		if idx < 0 || idx >= pageCount {
			t.Fatalf("displayed index out of range: idx=%d pageCount=%d plan=%+v", idx, pageCount, plan)
		}
		if _, ok := seen[idx]; ok {
			t.Fatalf("displayed duplicate index within one plan: idx=%d plan=%+v", idx, plan)
		}
		seen[idx] = struct{}{}
	}

	return displayed
}

func formatPlanHistory(history []DisplayPlan) string {
	return fmt.Sprint(history)
}

func verifyForwardCoverage(t *testing.T, kinds []testPageKind, rightToLeft bool) State {
	t.Helper()

	metrics := metricsFromKinds(kinds)
	lookup := lookupFromSlice(metrics)
	state := State{
		Index:                0,
		PageCount:            len(metrics),
		BookMode:             true,
		RightToLeft:          rightToLeft,
		AspectRatioThreshold: 1.5,
	}

	counts := make([]int, len(metrics))
	history := make([]DisplayPlan, 0, len(metrics))
	for {
		plan := PlanDisplay(state, lookup)
		history = append(history, plan)
		for _, idx := range displayedIndicesForTest(t, plan, len(metrics)) {
			counts[idx]++
		}

		nextState, boundary := NavigateNext(state, lookup, false)
		if boundary == BoundaryLastPage {
			break
		}
		if boundary != BoundaryNone {
			t.Fatalf("unexpected forward boundary: %v state=%+v history=%s", boundary, state, formatPlanHistory(history))
		}
		state = nextState
	}

	for idx, count := range counts {
		if count != 1 {
			t.Fatalf("forward coverage mismatch: page=%d count=%d kinds=%s rtl=%v history=%s", idx, count, kindsString(kinds), rightToLeft, formatPlanHistory(history))
		}
	}

	return state
}

func verifyBackwardCoverage(t *testing.T, kinds []testPageKind, start State) {
	t.Helper()

	metrics := metricsFromKinds(kinds)
	lookup := lookupFromSlice(metrics)
	counts := make([]int, len(metrics))
	history := make([]DisplayPlan, 0, len(metrics))
	state := start

	for {
		plan := PlanDisplay(state, lookup)
		history = append(history, plan)
		for _, idx := range displayedIndicesForTest(t, plan, len(metrics)) {
			counts[idx]++
		}

		nextState, boundary := NavigatePrevious(state, lookup, false)
		if boundary == BoundaryFirstPage {
			break
		}
		if boundary != BoundaryNone {
			t.Fatalf("unexpected backward boundary: %v state=%+v history=%s", boundary, state, formatPlanHistory(history))
		}
		state = nextState
	}

	for idx, count := range counts {
		if count != 1 {
			t.Fatalf("backward coverage mismatch: page=%d count=%d kinds=%s rtl=%v history=%s", idx, count, kindsString(kinds), start.RightToLeft, formatPlanHistory(history))
		}
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

func TestExplainBookModeDecision(t *testing.T) {
	t.Run("compatible pair", func(t *testing.T) {
		decision := ExplainBookModeDecision(
			PageMetrics{Width: 100, Height: 150},
			PageMetrics{Width: 100, Height: 150},
			1.5,
			nil,
		)
		if !decision.UseBookMode || decision.Reason != "compatible pair" {
			t.Fatalf("unexpected decision: %+v", decision)
		}
	})

	t.Run("learned spread ratio", func(t *testing.T) {
		decision := ExplainBookModeDecision(
			PageMetrics{Width: 200, Height: 150},
			PageMetrics{Width: 210, Height: 150},
			1.5,
			[]float64{1.34},
		)
		if decision.UseBookMode || decision.Reason != "matches learned pre-joined spread ratio" {
			t.Fatalf("unexpected decision: %+v", decision)
		}
	})
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

	t.Run("rtl incompatible spread falls back to current index", func(t *testing.T) {
		rtlMetrics := []PageMetrics{
			{Width: 3138, Height: 2229},
			{Width: 3229, Height: 4647},
			{Width: 3200, Height: 4633},
		}
		plan := PlanDisplay(State{
			Index:                0,
			PageCount:            len(rtlMetrics),
			BookMode:             true,
			RightToLeft:          true,
			AspectRatioThreshold: 1.5,
		}, lookupFromSlice(rtlMetrics))
		if plan.LeftIndex != 0 || plan.RightIndex != -1 || plan.ActualImages != 1 {
			t.Fatalf("unexpected rtl fallback plan: %+v", plan)
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

	t.Run("rtl fallback does not skip current page", func(t *testing.T) {
		rtlMetrics := []PageMetrics{
			{Width: 3138, Height: 2229},
			{Width: 3229, Height: 4647},
			{Width: 3200, Height: 4633},
			{Width: 3200, Height: 4633},
		}
		lookup := lookupFromSlice(rtlMetrics)

		plan := PlanDisplay(State{
			Index:                0,
			PageCount:            len(rtlMetrics),
			BookMode:             true,
			RightToLeft:          true,
			AspectRatioThreshold: 1.5,
		}, lookup)
		if plan.LeftIndex != 0 || plan.RightIndex != -1 || plan.ActualImages != 1 {
			t.Fatalf("expected current page single fallback, got %+v", plan)
		}

		state, boundary := NavigateNext(State{
			Index:                0,
			PageCount:            len(rtlMetrics),
			BookMode:             true,
			RightToLeft:          true,
			AspectRatioThreshold: 1.5,
		}, lookup, false)
		if boundary != BoundaryNone || state.Index != 1 {
			t.Fatalf("expected single-page advance by 1 after rtl fallback, got boundary=%v state=%+v", boundary, state)
		}

		nextPlan := PlanDisplay(state, lookup)
		if nextPlan.LeftIndex != 2 || nextPlan.RightIndex != 1 || nextPlan.ActualImages != 2 {
			t.Fatalf("expected next rtl spread to be 2/1, got %+v", nextPlan)
		}
	})

	t.Run("rtl learned spread fallback shows current page", func(t *testing.T) {
		rtlMetrics := []PageMetrics{
			{Width: 3258, Height: 4676},
			{Width: 3258, Height: 4660},
			{Width: 3144, Height: 2243},
			{Width: 3138, Height: 2238},
		}
		plan := PlanDisplay(State{
			Index:                2,
			PageCount:            len(rtlMetrics),
			BookMode:             true,
			RightToLeft:          true,
			AspectRatioThreshold: 1.5,
			LearnedSpreadAspects: []float64{1.40},
		}, lookupFromSlice(rtlMetrics))
		if plan.LeftIndex != 2 || plan.RightIndex != -1 || plan.ActualImages != 1 {
			t.Fatalf("expected rtl learned-spread fallback to current page, got %+v", plan)
		}
	})

	t.Run("navigate previous from rtl single fallback restores previous spread", func(t *testing.T) {
		rtlMetrics := []PageMetrics{
			{Width: 3258, Height: 4676},
			{Width: 3258, Height: 4660},
			{Width: 3144, Height: 2243},
			{Width: 3138, Height: 2238},
		}
		lookup := lookupFromSlice(rtlMetrics)

		currentPlan := PlanDisplay(State{
			Index:                2,
			PageCount:            len(rtlMetrics),
			BookMode:             true,
			RightToLeft:          true,
			AspectRatioThreshold: 1.5,
			LearnedSpreadAspects: []float64{1.40},
		}, lookup)
		if currentPlan.LeftIndex != 2 || currentPlan.RightIndex != -1 || currentPlan.ActualImages != 1 {
			t.Fatalf("expected rtl current page single fallback, got %+v", currentPlan)
		}

		state, boundary := NavigatePrevious(State{
			Index:                2,
			PageCount:            len(rtlMetrics),
			BookMode:             true,
			RightToLeft:          true,
			AspectRatioThreshold: 1.5,
			LearnedSpreadAspects: []float64{1.40},
		}, lookup, false)
		if boundary != BoundaryNone || state.Index != 0 {
			t.Fatalf("expected previous navigation to restore prior spread anchor, got boundary=%v state=%+v", boundary, state)
		}

		prevPlan := PlanDisplay(state, lookup)
		if prevPlan.LeftIndex != 1 || prevPlan.RightIndex != 0 || prevPlan.ActualImages != 2 {
			t.Fatalf("expected previous rtl spread to be 1/0, got %+v", prevPlan)
		}
	})

	t.Run("navigate previous from rtl spread enters preceding single page first", func(t *testing.T) {
		rtlMetrics := []PageMetrics{
			{Width: 3139, Height: 2228},
			{Width: 3138, Height: 2229},
			{Width: 3138, Height: 2229},
			{Width: 3229, Height: 4647},
			{Width: 3200, Height: 4633},
		}
		lookup := lookupFromSlice(rtlMetrics)

		currentPlan := PlanDisplay(State{
			Index:                3,
			PageCount:            len(rtlMetrics),
			BookMode:             true,
			RightToLeft:          true,
			AspectRatioThreshold: 1.5,
			LearnedSpreadAspects: []float64{1.40},
		}, lookup)
		if currentPlan.LeftIndex != 4 || currentPlan.RightIndex != 3 || currentPlan.ActualImages != 2 {
			t.Fatalf("expected current rtl spread to be 4/3, got %+v", currentPlan)
		}

		state, boundary := NavigatePrevious(State{
			Index:                3,
			PageCount:            len(rtlMetrics),
			BookMode:             true,
			RightToLeft:          true,
			AspectRatioThreshold: 1.5,
			LearnedSpreadAspects: []float64{1.40},
		}, lookup, false)
		if boundary != BoundaryNone || state.Index != 2 {
			t.Fatalf("expected previous navigation to land on immediate preceding single page, got boundary=%v state=%+v", boundary, state)
		}

		prevPlan := PlanDisplay(state, lookup)
		if prevPlan.LeftIndex != 2 || prevPlan.RightIndex != -1 || prevPlan.ActualImages != 1 {
			t.Fatalf("expected previous rtl page to stay single, got %+v", prevPlan)
		}
	})

	t.Run("navigate previous from ltr spread enters preceding single page first", func(t *testing.T) {
		ltrMetrics := []PageMetrics{
			{Width: 3139, Height: 2228},
			{Width: 3138, Height: 2229},
			{Width: 3138, Height: 2229},
			{Width: 3229, Height: 4647},
			{Width: 3200, Height: 4633},
		}
		lookup := lookupFromSlice(ltrMetrics)

		currentPlan := PlanDisplay(State{
			Index:                3,
			PageCount:            len(ltrMetrics),
			BookMode:             true,
			RightToLeft:          false,
			AspectRatioThreshold: 1.5,
			LearnedSpreadAspects: []float64{1.40},
		}, lookup)
		if currentPlan.LeftIndex != 3 || currentPlan.RightIndex != 4 || currentPlan.ActualImages != 2 {
			t.Fatalf("expected current ltr spread to be 3/4, got %+v", currentPlan)
		}

		state, boundary := NavigatePrevious(State{
			Index:                3,
			PageCount:            len(ltrMetrics),
			BookMode:             true,
			RightToLeft:          false,
			AspectRatioThreshold: 1.5,
			LearnedSpreadAspects: []float64{1.40},
		}, lookup, false)
		if boundary != BoundaryNone || state.Index != 2 {
			t.Fatalf("expected previous navigation to land on immediate preceding single page, got boundary=%v state=%+v", boundary, state)
		}

		prevPlan := PlanDisplay(state, lookup)
		if prevPlan.LeftIndex != 2 || prevPlan.RightIndex != -1 || prevPlan.ActualImages != 1 {
			t.Fatalf("expected previous ltr page to stay single, got %+v", prevPlan)
		}
	})

	t.Run("navigate previous follows no-skip heuristic through mixed rtl sequence", func(t *testing.T) {
		rtlMetrics := []PageMetrics{
			{Width: 3150, Height: 2239},
			{Width: 3263, Height: 4664},
			{Width: 3258, Height: 4676},
			{Width: 3267, Height: 4659},
			{Width: 3230, Height: 4676},
			{Width: 3229, Height: 4669},
			{Width: 3139, Height: 2229},
		}
		lookup := lookupFromSlice(rtlMetrics)
		learned := []float64{1.40}

		state := State{
			Index:                6,
			PageCount:            len(rtlMetrics),
			BookMode:             true,
			RightToLeft:          true,
			AspectRatioThreshold: 1.5,
			LearnedSpreadAspects: learned,
		}
		plan := PlanDisplay(state, lookup)
		if plan.LeftIndex != 6 || plan.RightIndex != -1 || plan.ActualImages != 1 {
			t.Fatalf("expected current page 6 to be single, got %+v", plan)
		}

		expected := []struct {
			index      int
			leftIndex  int
			rightIndex int
			actual     int
		}{
			{4, 5, 4, 2},
			{2, 3, 2, 2},
			{1, 1, -1, 1},
			{0, 0, -1, 1},
		}

		for _, want := range expected {
			var boundary Boundary
			state, boundary = NavigatePrevious(state, lookup, false)
			if boundary != BoundaryNone || state.Index != want.index {
				t.Fatalf("unexpected state after previous: boundary=%v state=%+v wantIndex=%d", boundary, state, want.index)
			}
			plan = PlanDisplay(state, lookup)
			if plan.LeftIndex != want.leftIndex || plan.RightIndex != want.rightIndex || plan.ActualImages != want.actual {
				t.Fatalf("unexpected plan after previous: %+v want left=%d right=%d actual=%d", plan, want.leftIndex, want.rightIndex, want.actual)
			}
		}
	})

	t.Run("navigate previous from rtl single enters preceding spread before preceding single", func(t *testing.T) {
		rtlMetrics := []PageMetrics{
			{Width: 3138, Height: 2229},
			{Width: 3229, Height: 4647},
			{Width: 3200, Height: 4633},
			{Width: 3258, Height: 4676},
			{Width: 3258, Height: 4660},
			{Width: 3144, Height: 2243},
			{Width: 3138, Height: 2238},
		}
		lookup := lookupFromSlice(rtlMetrics)
		learned := []float64{1.40}

		state := State{
			Index:                5,
			PageCount:            len(rtlMetrics),
			BookMode:             true,
			RightToLeft:          true,
			AspectRatioThreshold: 1.5,
			LearnedSpreadAspects: learned,
		}
		currentPlan := PlanDisplay(state, lookup)
		if currentPlan.LeftIndex != 5 || currentPlan.RightIndex != -1 || currentPlan.ActualImages != 1 {
			t.Fatalf("expected current rtl page to stay single, got %+v", currentPlan)
		}

		state, boundary := NavigatePrevious(state, lookup, false)
		if boundary != BoundaryNone || state.Index != 3 {
			t.Fatalf("expected previous navigation to land on preceding spread anchor, got boundary=%v state=%+v", boundary, state)
		}

		prevPlan := PlanDisplay(state, lookup)
		if prevPlan.LeftIndex != 4 || prevPlan.RightIndex != 3 || prevPlan.ActualImages != 2 {
			t.Fatalf("expected previous rtl spread to be 4/3, got %+v", prevPlan)
		}
	})
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

func TestNavigationTraversalCoversEachPageExactlyOncePerDirection(t *testing.T) {
	fixedCases := []struct {
		name  string
		kinds []testPageKind
	}{
		{
			name:  "single page",
			kinds: []testPageKind{testPagePairable},
		},
		{
			name:  "all pairable",
			kinds: []testPageKind{testPagePairable, testPagePairable, testPagePairable, testPagePairable, testPagePairable},
		},
		{
			name:  "all forced single",
			kinds: []testPageKind{testPageForcedSingle, testPageForcedSingle, testPageForcedSingle, testPageForcedSingle, testPageForcedSingle},
		},
		{
			name:  "alternating",
			kinds: []testPageKind{testPagePairable, testPageForcedSingle, testPagePairable, testPageForcedSingle, testPagePairable, testPageForcedSingle},
		},
		{
			name: "odd pairable run between singles",
			kinds: []testPageKind{
				testPageForcedSingle,
				testPagePairable, testPagePairable, testPagePairable,
				testPageForcedSingle,
			},
		},
		{
			name: "even pairable run between singles",
			kinds: []testPageKind{
				testPageForcedSingle,
				testPagePairable, testPagePairable, testPagePairable, testPagePairable,
				testPageForcedSingle,
			},
		},
	}

	for _, rightToLeft := range []bool{false, true} {
		directionName := "ltr"
		if rightToLeft {
			directionName = "rtl"
		}

		t.Run(directionName, func(t *testing.T) {
			for _, tc := range fixedCases {
				t.Run(tc.name, func(t *testing.T) {
					lastState := verifyForwardCoverage(t, tc.kinds, rightToLeft)
					verifyBackwardCoverage(t, tc.kinds, lastState)
				})
			}

			const (
				randomSeed  int64 = 20260418
				randomCases       = 1000
				maxPages          = 40
			)
			rng := rand.New(rand.NewSource(randomSeed))

			for i := 0; i < randomCases; i++ {
				kinds := make([]testPageKind, 1+rng.Intn(maxPages))
				for j := range kinds {
					if rng.Intn(2) == 0 {
						kinds[j] = testPagePairable
					} else {
						kinds[j] = testPageForcedSingle
					}
				}

				t.Run(fmt.Sprintf("random_%03d", i), func(t *testing.T) {
					lastState := verifyForwardCoverage(t, kinds, rightToLeft)
					verifyBackwardCoverage(t, kinds, lastState)
				})
			}
		})
	}
}
