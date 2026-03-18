package main

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"nv/navlogic"
)

func (g *Game) navigationState() navlogic.State {
	return navlogic.State{
		Index:                g.idx,
		PageCount:            g.imageManager.GetPathsCount(),
		BookMode:             g.bookMode,
		TempSingleMode:       g.tempSingleMode,
		RightToLeft:          g.config.RightToLeft,
		AspectRatioThreshold: g.config.AspectRatioThreshold,
		LearnedSpreadAspects: append([]float64(nil), g.learnedSpreadAspects...),
	}
}

func (g *Game) applyNavigationState(state navlogic.State) {
	g.idx = state.Index
	g.bookMode = state.BookMode
	g.tempSingleMode = state.TempSingleMode
}

func (g *Game) pageMetricsAt(idx int) navlogic.PageMetrics {
	if idx < 0 || idx >= g.imageManager.GetPathsCount() {
		return navlogic.PageMetrics{}
	}

	img := g.imageManager.GetImage(idx)
	if img == nil {
		return navlogic.PageMetrics{}
	}

	bounds := img.Bounds()
	return navlogic.PageMetrics{
		Width:  bounds.Dx(),
		Height: bounds.Dy(),
	}
}

func (g *Game) displayImageAt(idx int) *ebiten.Image {
	if idx < 0 {
		return nil
	}
	return g.imageManager.GetImage(idx)
}

func (g *Game) pageAspectAt(idx int) float64 {
	metrics := g.pageMetricsAt(idx)
	if metrics.Width <= 0 || metrics.Height <= 0 {
		return 0
	}
	return float64(metrics.Width) / float64(metrics.Height)
}

func (g *Game) logDisplayPlan(context string, state navlogic.State, plan navlogic.DisplayPlan) {
	if plan.TotalPages == 0 {
		debugLog("%s: no pages available", context)
		return
	}

	debugLog(
		"%s: state idx=%d pageCount=%d bookMode=%t tempSingle=%t rtl=%t learned=%d -> plan leftIdx=%d rightIdx=%d actualImages=%d currentPage=%d/%d",
		context,
		state.Index,
		state.PageCount,
		state.BookMode,
		state.TempSingleMode,
		state.RightToLeft,
		len(state.LearnedSpreadAspects),
		plan.LeftIndex,
		plan.RightIndex,
		plan.ActualImages,
		plan.CurrentPage,
		plan.TotalPages,
	)

	if state.TempSingleMode {
		debugLog("%s: bookmode decision skipped because tempSingleMode=true", context)
		return
	}
	if !state.BookMode {
		debugLog("%s: bookmode decision skipped because bookMode=false", context)
		return
	}

	leftIdx, rightIdx := plan.LeftIndex, plan.RightIndex
	if plan.ActualImages != 2 {
		leftIdx, rightIdx = pairedIndicesForLog(state)
	}
	leftMetrics := g.pageMetricsAt(leftIdx)
	rightMetrics := g.pageMetricsAt(rightIdx)
	decision := navlogic.ExplainBookModeDecision(leftMetrics, rightMetrics, state.AspectRatioThreshold, state.LearnedSpreadAspects)
	debugLog(
		"%s: pair candidate leftIdx=%d(%dx%d aspect=%.3f) rightIdx=%d(%dx%d aspect=%.3f) decision=%t reason=%s",
		context,
		leftIdx,
		leftMetrics.Width,
		leftMetrics.Height,
		decision.LeftAspect,
		rightIdx,
		rightMetrics.Width,
		rightMetrics.Height,
		decision.RightAspect,
		decision.UseBookMode,
		decision.Reason,
	)
}

func pairedIndicesForLog(state navlogic.State) (int, int) {
	if state.RightToLeft {
		return state.Index + 1, state.Index
	}
	return state.Index, state.Index + 1
}

func (g *Game) addLearnedSpreadAspect(aspect float64) bool {
	if aspect <= 0 || math.IsNaN(aspect) || math.IsInf(aspect, 0) {
		return false
	}

	for _, existing := range g.learnedSpreadAspects {
		if existing <= 0 {
			continue
		}
		if max(aspect/existing, existing/aspect) <= 1.02 {
			return false
		}
	}

	g.learnedSpreadAspects = append(g.learnedSpreadAspects, aspect)
	return true
}

// calculateDisplayContent determines what should be displayed based on current state.
func (g *Game) calculateDisplayContent() {
	state := g.navigationState()
	plan := navlogic.PlanDisplay(state, g.pageMetricsAt)
	if plan.TotalPages == 0 {
		g.displayContent = nil
		return
	}

	g.displayContent = &DisplayContent{
		LeftImage:  g.displayImageAt(plan.LeftIndex),
		RightImage: g.displayImageAt(plan.RightIndex),
		Metadata: DisplayMetadata{
			LeftPage:     plan.LeftIndex + 1,
			RightPage:    plan.RightIndex + 1,
			TotalPages:   plan.TotalPages,
			ActualImages: plan.ActualImages,
		},
	}

	if g.zoomState.Mode != ZoomModeManual && !g.needsInitialZoomUpdate {
		g.updateZoomLevelForFitMode()
	}

	g.logDisplayPlan("calculateDisplayContent", state, plan)
}

func (g *Game) showOverlayMessage(message string) {
	g.overlayMessage = message
	if message != "" {
		g.overlayMessageTime = time.Now()
	} else {
		g.overlayMessageTime = time.Time{}
	}
}

func (g *Game) toggleBookMode() {
	prevState := g.navigationState()
	nextState := navlogic.ToggleBookMode(g.navigationState(), g.pageMetricsAt)
	g.applyNavigationState(nextState)
	if g.bookMode {
		g.showOverlayMessage("Book Mode: ON")
	} else {
		g.showOverlayMessage("Book Mode: OFF")
	}

	g.config.BookMode = g.bookMode
	g.calculateDisplayContent()
	debugLog("toggleBookMode: prev idx=%d bookMode=%t tempSingle=%t -> next idx=%d bookMode=%t tempSingle=%t",
		prevState.Index, prevState.BookMode, prevState.TempSingleMode,
		nextState.Index, nextState.BookMode, nextState.TempSingleMode,
	)
}

func (g *Game) toggleReadingDirection() {
	g.config.RightToLeft = !g.config.RightToLeft
	direction := "Left-to-Right"
	if g.config.RightToLeft {
		direction = "Right-to-Left"
	}
	g.showOverlayMessage("Reading Direction: " + direction)
	g.calculateDisplayContent()
	debugLog("toggleReadingDirection: rtl=%t", g.config.RightToLeft)
}

func (g *Game) markCurrentAsPreJoinedSpread() {
	plan := navlogic.PlanDisplay(g.navigationState(), g.pageMetricsAt)
	if plan.TotalPages == 0 || plan.LeftIndex < 0 {
		return
	}

	indices := []int{plan.LeftIndex}
	if plan.RightIndex >= 0 && plan.RightIndex != plan.LeftIndex {
		indices = append(indices, plan.RightIndex)
	}

	learned := make([]float64, 0, len(indices))
	for _, idx := range indices {
		aspect := g.pageAspectAt(idx)
		if g.addLearnedSpreadAspect(aspect) {
			learned = append(learned, aspect)
		}
	}

	if len(learned) == 0 {
		g.showOverlayMessage("Pre-joined spread ratio already learned")
		return
	}

	if len(learned) == 1 {
		g.showOverlayMessage(fmt.Sprintf("Learned pre-joined spread ratio: %.2f", learned[0]))
	} else {
		g.showOverlayMessage(fmt.Sprintf("Learned pre-joined spread ratios: %.2f, %.2f", learned[0], learned[1]))
	}

	g.calculateDisplayContent()
}

func (g *Game) processPageInput() {
	if g.pageInputBuffer == "" {
		return
	}

	pageNum, err := strconv.Atoi(g.pageInputBuffer)
	if err != nil {
		g.showOverlayMessage("Invalid page number")
		return
	}

	g.jumpToPage(pageNum)
}

func (g *Game) jumpToPage(pageNum int) {
	prevState := g.navigationState()
	nextState, boundary := navlogic.JumpToPage(g.navigationState(), pageNum, g.pageMetricsAt)
	if boundary == navlogic.BoundaryPageNotFound {
		g.showOverlayMessage(fmt.Sprintf("Page %d not found (1-%d)", pageNum, g.imageManager.GetPathsCount()))
		return
	}

	g.applyNavigationState(nextState)
	g.imageManager.StartPreload(g.idx, NavigationJump)
	g.resetZoomToInitial()
	g.calculateDisplayContent()
	debugLog("jumpToPage: requested=%d prevIdx=%d -> nextIdx=%d boundary=%v bookMode=%t tempSingle=%t",
		pageNum, prevState.Index, nextState.Index, boundary, nextState.BookMode, nextState.TempSingleMode)
}

func (g *Game) navigateNext(singleStep bool) {
	prevState := g.navigationState()
	nextState, boundary := navlogic.NavigateNext(g.navigationState(), g.pageMetricsAt, singleStep)
	if boundary == navlogic.BoundaryLastPage {
		debugLog("navigateNext: singleStep=%t prevIdx=%d boundary=%v", singleStep, prevState.Index, boundary)
		g.showOverlayMessage("Last page")
		return
	}

	g.applyNavigationState(nextState)
	g.resetZoomToInitial()
	g.calculateDisplayContent()
	debugLog("navigateNext: singleStep=%t prev idx=%d bookMode=%t tempSingle=%t -> next idx=%d bookMode=%t tempSingle=%t boundary=%v",
		singleStep,
		prevState.Index, prevState.BookMode, prevState.TempSingleMode,
		nextState.Index, nextState.BookMode, nextState.TempSingleMode,
		boundary,
	)
}

func (g *Game) navigatePrevious(singleStep bool) {
	prevState := g.navigationState()
	nextState, boundary := navlogic.NavigatePrevious(g.navigationState(), g.pageMetricsAt, singleStep)
	if boundary == navlogic.BoundaryFirstPage {
		debugLog("navigatePrevious: singleStep=%t prevIdx=%d boundary=%v", singleStep, prevState.Index, boundary)
		g.showOverlayMessage("First page")
		return
	}

	g.applyNavigationState(nextState)
	g.resetZoomToInitial()
	g.calculateDisplayContent()
	debugLog("navigatePrevious: singleStep=%t prev idx=%d bookMode=%t tempSingle=%t -> next idx=%d bookMode=%t tempSingle=%t boundary=%v",
		singleStep,
		prevState.Index, prevState.BookMode, prevState.TempSingleMode,
		nextState.Index, nextState.BookMode, nextState.TempSingleMode,
		boundary,
	)
}
