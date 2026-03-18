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
		debugKV("nav", "display_plan", "context", context, "reason", "no_pages")
		return
	}

	debugKV("nav", "display_plan",
		"context", context,
		"idx", state.Index,
		"page_count", state.PageCount,
		"book_mode", state.BookMode,
		"temp_single", state.TempSingleMode,
		"rtl", state.RightToLeft,
		"learned_count", len(state.LearnedSpreadAspects),
		"left_idx", plan.LeftIndex,
		"right_idx", plan.RightIndex,
		"actual_images", plan.ActualImages,
		"current_page", plan.CurrentPage,
		"total_pages", plan.TotalPages,
	)

	if state.TempSingleMode {
		debugKV("nav", "book_decision_skipped", "context", context, "reason", "temp_single")
		return
	}
	if !state.BookMode {
		debugKV("nav", "book_decision_skipped", "context", context, "reason", "book_mode_off")
		return
	}

	leftIdx, rightIdx := plan.LeftIndex, plan.RightIndex
	if plan.ActualImages != 2 {
		leftIdx, rightIdx = pairedIndicesForLog(state)
	}
	leftMetrics := g.pageMetricsAt(leftIdx)
	rightMetrics := g.pageMetricsAt(rightIdx)
	decision := navlogic.ExplainBookModeDecision(leftMetrics, rightMetrics, state.AspectRatioThreshold, state.LearnedSpreadAspects)
	debugKV("nav", "book_decision",
		"context", context,
		"left_idx", leftIdx,
		"left_width", leftMetrics.Width,
		"left_height", leftMetrics.Height,
		"left_aspect", decision.LeftAspect,
		"right_idx", rightIdx,
		"right_width", rightMetrics.Width,
		"right_height", rightMetrics.Height,
		"right_aspect", decision.RightAspect,
		"use_book_mode", decision.UseBookMode,
		"threshold", state.AspectRatioThreshold,
		"reason", decision.Reason,
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
		debugKV("nav", "overlay_message",
			"message", message,
			"idx", g.idx,
			"book_mode", g.bookMode,
			"temp_single", g.tempSingleMode,
			"zoom_mode", g.zoomState.Mode,
		)
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
	debugKV("nav", "toggle_book_mode",
		"prev_idx", prevState.Index,
		"prev_book_mode", prevState.BookMode,
		"prev_temp_single", prevState.TempSingleMode,
		"next_idx", nextState.Index,
		"next_book_mode", nextState.BookMode,
		"next_temp_single", nextState.TempSingleMode,
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
	debugKV("nav", "toggle_reading_direction", "rtl", g.config.RightToLeft)
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
		debugKV("nav", "learn_spread_skip", "reason", "already_learned", "idx", g.idx)
		return
	}

	if len(learned) == 1 {
		g.showOverlayMessage(fmt.Sprintf("Learned pre-joined spread ratio: %.2f", learned[0]))
	} else {
		g.showOverlayMessage(fmt.Sprintf("Learned pre-joined spread ratios: %.2f, %.2f", learned[0], learned[1]))
	}

	g.calculateDisplayContent()
	debugKV("nav", "learn_spread",
		"idx", g.idx,
		"learned", learned,
		"learned_count", len(g.learnedSpreadAspects),
	)
}

func (g *Game) processPageInput() {
	if g.pageInputBuffer == "" {
		debugKV("input", "page_input_skip", "reason", "empty_buffer")
		return
	}

	pageNum, err := strconv.Atoi(g.pageInputBuffer)
	if err != nil {
		g.showOverlayMessage("Invalid page number")
		debugKV("input", "page_input_invalid", "buffer", g.pageInputBuffer, "error", err)
		return
	}

	debugKV("input", "page_input_submit", "buffer", g.pageInputBuffer, "page", pageNum)
	g.jumpToPage(pageNum)
}

func (g *Game) jumpToPage(pageNum int) {
	prevState := g.navigationState()
	nextState, boundary := navlogic.JumpToPage(g.navigationState(), pageNum, g.pageMetricsAt)
	if boundary == navlogic.BoundaryPageNotFound {
		g.showOverlayMessage(fmt.Sprintf("Page %d not found (1-%d)", pageNum, g.imageManager.GetPathsCount()))
		debugKV("nav", "jump_to_page", "requested_page", pageNum, "boundary", boundary, "reason", "page_not_found")
		return
	}

	g.applyNavigationState(nextState)
	g.imageManager.StartPreload(g.idx, NavigationJump)
	g.resetZoomToInitial()
	g.calculateDisplayContent()
	debugKV("nav", "jump_to_page",
		"requested_page", pageNum,
		"prev_idx", prevState.Index,
		"next_idx", nextState.Index,
		"boundary", boundary,
		"book_mode", nextState.BookMode,
		"temp_single", nextState.TempSingleMode,
	)
}

func (g *Game) navigateNext(singleStep bool) {
	prevState := g.navigationState()
	nextState, boundary := navlogic.NavigateNext(g.navigationState(), g.pageMetricsAt, singleStep)
	if boundary == navlogic.BoundaryLastPage {
		debugKV("nav", "navigate_next", "single_step", singleStep, "prev_idx", prevState.Index, "boundary", boundary)
		g.showOverlayMessage("Last page")
		return
	}

	g.applyNavigationState(nextState)
	g.resetZoomToInitial()
	g.calculateDisplayContent()
	debugKV("nav", "navigate_next",
		"single_step", singleStep,
		"prev_idx", prevState.Index,
		"prev_book_mode", prevState.BookMode,
		"prev_temp_single", prevState.TempSingleMode,
		"next_idx", nextState.Index,
		"next_book_mode", nextState.BookMode,
		"next_temp_single", nextState.TempSingleMode,
		"boundary", boundary,
	)
}

func (g *Game) navigatePrevious(singleStep bool) {
	prevState := g.navigationState()
	nextState, boundary := navlogic.NavigatePrevious(g.navigationState(), g.pageMetricsAt, singleStep)
	if boundary == navlogic.BoundaryFirstPage {
		debugKV("nav", "navigate_previous", "single_step", singleStep, "prev_idx", prevState.Index, "boundary", boundary)
		g.showOverlayMessage("First page")
		return
	}

	g.applyNavigationState(nextState)
	g.resetZoomToInitial()
	g.calculateDisplayContent()
	debugKV("nav", "navigate_previous",
		"single_step", singleStep,
		"prev_idx", prevState.Index,
		"prev_book_mode", prevState.BookMode,
		"prev_temp_single", prevState.TempSingleMode,
		"next_idx", nextState.Index,
		"next_book_mode", nextState.BookMode,
		"next_temp_single", nextState.TempSingleMode,
		"boundary", boundary,
	)
}
