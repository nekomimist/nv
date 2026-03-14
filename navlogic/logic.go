package navlogic

const (
	minAspectRatio = 0.4
	maxAspectRatio = 2.5
)

type PageMetrics struct {
	Width  int
	Height int
}

type MetricsLookup func(idx int) PageMetrics

type State struct {
	Index                int
	PageCount            int
	BookMode             bool
	TempSingleMode       bool
	RightToLeft          bool
	AspectRatioThreshold float64
}

type DisplayPlan struct {
	LeftIndex    int
	RightIndex   int
	CurrentPage  int
	TotalPages   int
	ActualImages int
}

type Boundary int

const (
	BoundaryNone Boundary = iota
	BoundaryFirstPage
	BoundaryLastPage
	BoundaryPageNotFound
)

func PlanDisplay(state State, lookup MetricsLookup) DisplayPlan {
	state = normalizeState(state)
	if state.PageCount == 0 {
		return DisplayPlan{
			LeftIndex:    -1,
			RightIndex:   -1,
			CurrentPage:  0,
			TotalPages:   0,
			ActualImages: 0,
		}
	}

	plan := DisplayPlan{
		LeftIndex:    state.Index,
		RightIndex:   -1,
		CurrentPage:  state.Index + 1,
		TotalPages:   state.PageCount,
		ActualImages: 1,
	}

	if state.TempSingleMode || !state.BookMode {
		return plan
	}

	leftIdx, rightIdx := pairIndices(state, state.Index)
	leftMetrics := lookup(leftIdx)
	rightMetrics := lookup(rightIdx)
	if ShouldUseBookMode(leftMetrics, rightMetrics, state.AspectRatioThreshold) {
		plan.LeftIndex = leftIdx
		plan.RightIndex = rightIdx
		plan.ActualImages = 2
		return plan
	}

	switch {
	case isAvailable(leftMetrics):
		plan.LeftIndex = leftIdx
	case isAvailable(rightMetrics):
		plan.LeftIndex = rightIdx
	default:
		plan.LeftIndex = -1
	}
	return plan
}

func SetCurrentIndex(state State, targetIdx int, lookup MetricsLookup) State {
	state = normalizeState(state)
	if state.PageCount == 0 {
		state.Index = 0
		state.TempSingleMode = false
		return state
	}

	targetIdx = clampIndex(targetIdx, state.PageCount)
	if state.BookMode && targetIdx == state.PageCount-1 {
		if targetIdx > 0 {
			leftIdx, rightIdx := pairIndices(state, targetIdx-1)
			if ShouldUseBookMode(lookup(leftIdx), lookup(rightIdx), state.AspectRatioThreshold) {
				state.Index = targetIdx - 1
				state.TempSingleMode = false
				return state
			}
		}
		state.Index = targetIdx
		state.BookMode = false
		state.TempSingleMode = true
		return state
	}

	state.Index = targetIdx
	state.TempSingleMode = false
	return state
}

func NavigateNext(state State, lookup MetricsLookup, singleStep bool) (State, Boundary) {
	state = normalizeState(state)
	if state.Index+1 >= state.PageCount {
		return state, BoundaryLastPage
	}

	if state.TempSingleMode {
		state.Index++
		state.TempSingleMode = false
		state.BookMode = true
		return state, BoundaryNone
	}

	currentPlan := PlanDisplay(state, lookup)
	if state.BookMode && !singleStep && currentPlan.ActualImages == 2 {
		if state.Index+2 >= state.PageCount {
			return state, BoundaryLastPage
		}
		if state.Index+3 >= state.PageCount {
			state.Index += 2
			state.BookMode = false
			state.TempSingleMode = true
			return state, BoundaryNone
		}
		state.Index += 2
		return state, BoundaryNone
	}

	state.Index++
	return state, BoundaryNone
}

func NavigatePrevious(state State, lookup MetricsLookup, singleStep bool) (State, Boundary) {
	state = normalizeState(state)
	if state.Index <= 0 {
		return state, BoundaryFirstPage
	}

	if state.TempSingleMode {
		if state.Index < 2 {
			state.Index = 0
		} else {
			state.Index -= 2
		}
		state.TempSingleMode = false
		state.BookMode = true
		return state, BoundaryNone
	}

	currentPlan := PlanDisplay(state, lookup)
	if state.BookMode && !singleStep && currentPlan.ActualImages == 2 {
		if state.Index < 2 {
			state.Index = 0
			state.BookMode = false
			state.TempSingleMode = true
			return state, BoundaryNone
		}
		state.Index -= 2
		return state, BoundaryNone
	}

	state.Index--
	return state, BoundaryNone
}

func ToggleBookMode(state State, lookup MetricsLookup) State {
	state = normalizeState(state)
	if state.TempSingleMode || state.BookMode {
		state.BookMode = false
		state.TempSingleMode = false
		return state
	}

	if state.PageCount == 1 {
		state.BookMode = true
		state.TempSingleMode = true
		return state
	}

	if state.Index == state.PageCount-1 {
		leftIdx, rightIdx := pairIndices(state, state.Index-1)
		if ShouldUseBookMode(lookup(leftIdx), lookup(rightIdx), state.AspectRatioThreshold) {
			state.Index--
			state.TempSingleMode = false
		} else {
			state.TempSingleMode = true
		}
		state.BookMode = true
		return state
	}

	state.BookMode = true
	state.TempSingleMode = false
	return state
}

func JumpToPage(state State, pageNum int, lookup MetricsLookup) (State, Boundary) {
	state = normalizeState(state)
	targetIdx := pageNum - 1
	if targetIdx < 0 || targetIdx >= state.PageCount {
		return state, BoundaryPageNotFound
	}
	return SetCurrentIndex(state, targetIdx, lookup), BoundaryNone
}

func ShouldUseBookMode(leftMetrics, rightMetrics PageMetrics, aspectRatioThreshold float64) bool {
	if !isAvailable(leftMetrics) || !isAvailable(rightMetrics) {
		return false
	}

	leftAspect := float64(leftMetrics.Width) / float64(leftMetrics.Height)
	rightAspect := float64(rightMetrics.Width) / float64(rightMetrics.Height)
	if leftAspect < minAspectRatio || leftAspect > maxAspectRatio ||
		rightAspect < minAspectRatio || rightAspect > maxAspectRatio {
		return false
	}

	aspectRatio := leftAspect / rightAspect
	if aspectRatio < 1.0 {
		aspectRatio = 1.0 / aspectRatio
	}
	return aspectRatio <= aspectRatioThreshold
}

func normalizeState(state State) State {
	if state.PageCount < 0 {
		state.PageCount = 0
	}
	if state.PageCount == 0 {
		state.Index = 0
		return state
	}
	state.Index = clampIndex(state.Index, state.PageCount)
	return state
}

func clampIndex(idx, pageCount int) int {
	if pageCount <= 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= pageCount {
		return pageCount - 1
	}
	return idx
}

func pairIndices(state State, idx int) (int, int) {
	if state.RightToLeft {
		return idx + 1, idx
	}
	return idx, idx + 1
}

func isAvailable(metrics PageMetrics) bool {
	return metrics.Width > 0 && metrics.Height > 0
}
