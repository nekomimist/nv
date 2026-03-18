package navlogic

const (
	minAspectRatio         = 0.4
	maxAspectRatio         = 2.5
	learnedSpreadTolerance = 1.12
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
	LearnedSpreadAspects []float64
}

type DisplayPlan struct {
	LeftIndex    int
	RightIndex   int
	CurrentPage  int
	TotalPages   int
	ActualImages int
}

type BookModeDecision struct {
	UseBookMode bool
	Reason      string
	LeftAspect  float64
	RightAspect float64
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
	if ShouldUseBookMode(leftMetrics, rightMetrics, state.AspectRatioThreshold, state.LearnedSpreadAspects) {
		plan.LeftIndex = leftIdx
		plan.RightIndex = rightIdx
		plan.ActualImages = 2
		return plan
	}

	currentMetrics := lookup(state.Index)
	switch {
	case isAvailable(currentMetrics):
		plan.LeftIndex = state.Index
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
			if ShouldUseBookMode(lookup(leftIdx), lookup(rightIdx), state.AspectRatioThreshold, state.LearnedSpreadAspects) {
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

	if state.BookMode && !singleStep {
		currentPlan := PlanDisplay(state, lookup)
		prevPage := minDisplayedIndex(currentPlan) - 1
		if prevPage < 0 {
			return state, BoundaryFirstPage
		}

		prevPairAnchor := prevPage - 1
		if prevPairAnchor >= 0 {
			prevState := state
			prevState.Index = prevPairAnchor
			prevPlan := PlanDisplay(prevState, lookup)
			if prevPlan.ActualImages == 2 &&
				minDisplayedIndex(prevPlan) == prevPairAnchor &&
				maxDisplayedIndex(prevPlan) == prevPage {
				state.Index = prevPairAnchor
				state.TempSingleMode = false
				return state, BoundaryNone
			}
		}

		state.Index = prevPage
		state.TempSingleMode = true
		state.BookMode = true
		return state, BoundaryNone
	}

	state.Index--
	return state, BoundaryNone
}

func minDisplayedIndex(plan DisplayPlan) int {
	if plan.ActualImages == 2 && plan.RightIndex >= 0 && plan.RightIndex < plan.LeftIndex {
		return plan.RightIndex
	}
	if plan.LeftIndex >= 0 {
		return plan.LeftIndex
	}
	return -1
}

func maxDisplayedIndex(plan DisplayPlan) int {
	if plan.ActualImages == 2 && plan.RightIndex > plan.LeftIndex {
		return plan.RightIndex
	}
	if plan.LeftIndex >= 0 {
		return plan.LeftIndex
	}
	return -1
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
		if ShouldUseBookMode(lookup(leftIdx), lookup(rightIdx), state.AspectRatioThreshold, state.LearnedSpreadAspects) {
			state.Index--
			state.TempSingleMode = false
			state.BookMode = true
			return state
		}
		state.TempSingleMode = true
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

func ShouldUseBookMode(leftMetrics, rightMetrics PageMetrics, aspectRatioThreshold float64, learnedSpreadAspects []float64) bool {
	return ExplainBookModeDecision(leftMetrics, rightMetrics, aspectRatioThreshold, learnedSpreadAspects).UseBookMode
}

func ExplainBookModeDecision(leftMetrics, rightMetrics PageMetrics, aspectRatioThreshold float64, learnedSpreadAspects []float64) BookModeDecision {
	decision := BookModeDecision{}
	if !isAvailable(leftMetrics) || !isAvailable(rightMetrics) {
		decision.Reason = "missing page metrics"
		return decision
	}

	decision.LeftAspect = aspectRatio(leftMetrics)
	decision.RightAspect = aspectRatio(rightMetrics)
	if decision.LeftAspect < minAspectRatio || decision.LeftAspect > maxAspectRatio ||
		decision.RightAspect < minAspectRatio || decision.RightAspect > maxAspectRatio {
		decision.Reason = "aspect ratio outside supported range"
		return decision
	}

	if aspectDistance(decision.LeftAspect, decision.RightAspect) > aspectRatioThreshold {
		decision.Reason = "page aspect ratios differ too much"
		return decision
	}

	if matchesLearnedSpreadAspect(decision.LeftAspect, learnedSpreadAspects) &&
		matchesLearnedSpreadAspect(decision.RightAspect, learnedSpreadAspects) {
		decision.Reason = "matches learned pre-joined spread ratio"
		return decision
	}

	decision.UseBookMode = true
	decision.Reason = "compatible pair"
	return decision
}

func aspectRatio(metrics PageMetrics) float64 {
	return float64(metrics.Width) / float64(metrics.Height)
}

func aspectDistance(a, b float64) float64 {
	if a < b {
		a, b = b, a
	}
	return a / b
}

func matchesLearnedSpreadAspect(aspect float64, learnedSpreadAspects []float64) bool {
	for _, learnedAspect := range learnedSpreadAspects {
		if learnedAspect <= 0 {
			continue
		}
		if aspectDistance(aspect, learnedAspect) <= learnedSpreadTolerance {
			return true
		}
	}
	return false
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
