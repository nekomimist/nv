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
	plan := navlogic.PlanDisplay(g.navigationState(), g.pageMetricsAt)
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
	nextState := navlogic.ToggleBookMode(g.navigationState(), g.pageMetricsAt)
	g.applyNavigationState(nextState)
	if g.bookMode {
		g.showOverlayMessage("Book Mode: ON")
	} else {
		g.showOverlayMessage("Book Mode: OFF")
	}

	g.config.BookMode = g.bookMode
	g.calculateDisplayContent()
}

func (g *Game) toggleReadingDirection() {
	g.config.RightToLeft = !g.config.RightToLeft
	direction := "Left-to-Right"
	if g.config.RightToLeft {
		direction = "Right-to-Left"
	}
	g.showOverlayMessage("Reading Direction: " + direction)
	g.calculateDisplayContent()
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
	nextState, boundary := navlogic.JumpToPage(g.navigationState(), pageNum, g.pageMetricsAt)
	if boundary == navlogic.BoundaryPageNotFound {
		g.showOverlayMessage(fmt.Sprintf("Page %d not found (1-%d)", pageNum, g.imageManager.GetPathsCount()))
		return
	}

	g.applyNavigationState(nextState)
	g.imageManager.StartPreload(g.idx, NavigationJump)
	g.resetZoomToInitial()
	g.calculateDisplayContent()
}

func (g *Game) navigateNext(singleStep bool) {
	nextState, boundary := navlogic.NavigateNext(g.navigationState(), g.pageMetricsAt, singleStep)
	if boundary == navlogic.BoundaryLastPage {
		g.showOverlayMessage("Last page")
		return
	}

	g.applyNavigationState(nextState)
	g.resetZoomToInitial()
	g.calculateDisplayContent()
}

func (g *Game) navigatePrevious(singleStep bool) {
	nextState, boundary := navlogic.NavigatePrevious(g.navigationState(), g.pageMetricsAt, singleStep)
	if boundary == navlogic.BoundaryFirstPage {
		g.showOverlayMessage("First page")
		return
	}

	g.applyNavigationState(nextState)
	g.resetZoomToInitial()
	g.calculateDisplayContent()
}
