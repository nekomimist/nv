package main

import (
	"fmt"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"nv/internal/imgdecode"
)

// ZoomMode represents the current zoom mode.
type ZoomMode int

const (
	ZoomModeFitWindow ZoomMode = iota // Automatic fit to window (width/height smaller)
	ZoomModeFitWidth                  // Fit to window width
	ZoomModeFitHeight                 // Fit to window height
	ZoomModeManual                    // Manual zoom level
)

func (m ZoomMode) String() string {
	switch m {
	case ZoomModeFitWindow:
		return "fit_window"
	case ZoomModeFitWidth:
		return "fit_width"
	case ZoomModeFitHeight:
		return "fit_height"
	case ZoomModeManual:
		return "manual"
	default:
		return "unknown"
	}
}

// ZoomState manages zoom and pan state.
type ZoomState struct {
	Mode       ZoomMode // Current zoom mode
	Level      float64  // Zoom level (1.0 = 100%, 2.0 = 200%, etc.)
	PanOffsetX float64  // Pan offset X coordinate
	PanOffsetY float64  // Pan offset Y coordinate
}

// NewZoomState creates a new zoom state with default values.
func NewZoomState() *ZoomState {
	return &ZoomState{
		Mode:       ZoomModeFitWindow,
		Level:      1.0,
		PanOffsetX: 0,
		PanOffsetY: 0,
	}
}

func (g *Game) zoomIn() {
	if g.zoomState.Mode != ZoomModeManual {
		debugKV("viewport", "zoom_switch_to_manual", "trigger", "zoom_in", "prev_mode", g.zoomState.Mode)
		g.switchToManual100()
		return
	}

	newLevel := g.zoomState.Level * 1.25
	if newLevel > 4.0 {
		g.zoomState.Level = 4.0
		g.checkDecodeBudget()
		g.showOverlayMessage("Maximum zoom 400%")
		return
	}

	g.zoomState.Level = newLevel
	g.checkDecodeBudget()
	g.showOverlayMessage(fmt.Sprintf("%.0f%%", g.zoomState.Level*100))
	debugKV("viewport", "zoom_in", "level", g.zoomState.Level)
}

func (g *Game) zoomOut() {
	if g.zoomState.Mode != ZoomModeManual {
		debugKV("viewport", "zoom_switch_to_manual", "trigger", "zoom_out", "prev_mode", g.zoomState.Mode)
		g.switchToManual100()
		return
	}

	newLevel := g.zoomState.Level / 1.25
	if newLevel < 0.25 {
		g.zoomState.Level = 0.25
		g.checkDecodeBudget()
		g.showOverlayMessage("Minimum zoom 25%")
		return
	}

	g.zoomState.Level = newLevel
	g.checkDecodeBudget()
	g.showOverlayMessage(fmt.Sprintf("%.0f%%", g.zoomState.Level*100))
	debugKV("viewport", "zoom_out", "level", g.zoomState.Level)
}

func (g *Game) zoomReset() {
	g.switchToManual100()
}

func (g *Game) zoomFit() {
	prevMode := g.zoomState.Mode
	switch g.zoomState.Mode {
	case ZoomModeFitWindow:
		g.zoomState.Mode = ZoomModeFitWidth
		g.zoomState.PanOffsetX = 0
		g.zoomState.PanOffsetY = 0
		g.updateZoomLevelForFitMode()
		g.alignPanForCurrentFitModeIfConfigured()
		g.clampPanToLimits()
		g.checkDecodeBudget()
		g.showOverlayMessage("Fit to Width")
	case ZoomModeFitWidth:
		g.zoomState.Mode = ZoomModeFitHeight
		g.zoomState.PanOffsetX = 0
		g.zoomState.PanOffsetY = 0
		g.updateZoomLevelForFitMode()
		g.alignPanForCurrentFitModeIfConfigured()
		g.clampPanToLimits()
		g.checkDecodeBudget()
		g.showOverlayMessage("Fit to Height")
	case ZoomModeFitHeight:
		g.switchToManual100()
	case ZoomModeManual:
		g.zoomState.Mode = ZoomModeFitWindow
		g.zoomState.PanOffsetX = 0
		g.zoomState.PanOffsetY = 0
		g.updateZoomLevelForFitMode()
		g.checkDecodeBudget()
		g.showOverlayMessage("Fit to Window")
	}
	debugKV("viewport", "zoom_fit_cycle", "prev_mode", prevMode, "next_mode", g.zoomState.Mode, "level", g.zoomState.Level)
}

func (g *Game) switchToManual100() {
	prevMode := g.zoomState.Mode
	g.zoomState.Mode = ZoomModeManual
	g.zoomState.Level = 1.0
	g.zoomState.PanOffsetX = 0
	g.zoomState.PanOffsetY = 0
	g.checkDecodeBudget()
	g.showOverlayMessage("100%")
	debugKV("viewport", "zoom_reset_manual", "prev_mode", prevMode, "level", g.zoomState.Level)
}

func (g *Game) panUp() {
	if g.zoomState.Mode == ZoomModeFitWindow {
		return
	}

	_, stepY := g.getPanStep()
	g.zoomState.PanOffsetY += stepY
	g.clampPanToLimits()
	debugKV("viewport", "pan_up", "pan_y", g.zoomState.PanOffsetY)
}

func (g *Game) panDown() {
	if g.zoomState.Mode == ZoomModeFitWindow {
		return
	}

	_, stepY := g.getPanStep()
	g.zoomState.PanOffsetY -= stepY
	g.clampPanToLimits()
	debugKV("viewport", "pan_down", "pan_y", g.zoomState.PanOffsetY)
}

func (g *Game) panLeft() {
	if g.zoomState.Mode == ZoomModeFitWindow {
		return
	}

	stepX, _ := g.getPanStep()
	g.zoomState.PanOffsetX += stepX
	g.clampPanToLimits()
	debugKV("viewport", "pan_left", "pan_x", g.zoomState.PanOffsetX)
}

func (g *Game) panRight() {
	if g.zoomState.Mode == ZoomModeFitWindow {
		return
	}

	stepX, _ := g.getPanStep()
	g.zoomState.PanOffsetX -= stepX
	g.clampPanToLimits()
	debugKV("viewport", "pan_right", "pan_x", g.zoomState.PanOffsetX)
}

func (g *Game) panByDelta(deltaX, deltaY float64) {
	if g.zoomState.Mode == ZoomModeFitWindow {
		return
	}

	g.zoomState.PanOffsetX += deltaX
	g.zoomState.PanOffsetY += deltaY
	g.clampPanToLimits()
}

// getPanStep calculates dynamic pan step size based on screen size and zoom level.
func (g *Game) getPanStep() (float64, float64) {
	stepX := float64(g.currentLogicalW) * 0.1
	stepY := float64(g.currentLogicalH) * 0.1

	zoomFactor := g.zoomState.Level
	stepX *= zoomFactor
	stepY *= zoomFactor
	return stepX, stepY
}

// updateZoomLevelForFitMode calculates and sets the actual zoom level for fit modes.
func (g *Game) updateZoomLevelForFitMode() {
	iw, ih := g.getTransformedImageSize()
	if iw == 0 || ih == 0 {
		g.zoomState.Level = 1.0
		return
	}

	w := float64(g.currentLogicalW)
	h := float64(g.currentLogicalH)
	fiw := float64(iw)
	fih := float64(ih)

	var scale float64
	switch g.zoomState.Mode {
	case ZoomModeFitWindow:
		if g.fullscreen {
			scale = math.Min(w/fiw, h/fih)
		} else if fiw > w || fih > h {
			scale = math.Min(w/fiw, h/fih)
		} else {
			scale = 1.0
		}
	case ZoomModeFitWidth:
		scale = w / fiw
	case ZoomModeFitHeight:
		scale = h / fih
	default:
		scale = 1.0
	}

	scale *= ebiten.Monitor().DeviceScaleFactor()
	g.zoomState.Level = scale
	debugKV("viewport", "fit_scale_updated",
		"mode", g.zoomState.Mode,
		"image_width", iw,
		"image_height", ih,
		"logical_width", g.currentLogicalW,
		"logical_height", g.currentLogicalH,
		"level", g.zoomState.Level,
	)
}

// resetZoomToInitial resets zoom state to the configured initial mode.
func (g *Game) resetZoomToInitial() {
	g.zoomState.PanOffsetX = 0
	g.zoomState.PanOffsetY = 0

	switch g.config.InitialZoomMode {
	case "fit_window":
		g.zoomState.Mode = ZoomModeFitWindow
		g.zoomState.Level = 1.0
		g.needsInitialZoomUpdate = false
	case "fit_width":
		g.zoomState.Mode = ZoomModeFitWidth
		g.zoomState.Level = 1.0
		g.needsInitialZoomUpdate = true
		g.needsInitialPanAlign = true
	case "fit_height":
		g.zoomState.Mode = ZoomModeFitHeight
		g.zoomState.Level = 1.0
		g.needsInitialZoomUpdate = true
		g.needsInitialPanAlign = true
	case "actual_size":
		g.zoomState.Mode = ZoomModeManual
		g.zoomState.Level = 1.0
		g.needsInitialZoomUpdate = false
	default:
		g.zoomState.Mode = ZoomModeFitWindow
		g.zoomState.Level = 1.0
		g.needsInitialZoomUpdate = false
	}

	debugKV("viewport", "zoom_reset_initial",
		"initial_zoom_mode", g.config.InitialZoomMode,
		"mode", g.zoomState.Mode,
		"needs_initial_zoom", g.needsInitialZoomUpdate,
		"needs_initial_pan_align", g.needsInitialPanAlign,
	)
}

// alignPanForCurrentFitModeIfConfigured nudges pan offsets to configured edges.
func (g *Game) alignPanForCurrentFitModeIfConfigured() {
	switch g.zoomState.Mode {
	case ZoomModeFitWidth:
		if g.config.FitWidthAlignTop {
			g.zoomState.PanOffsetY = 1e12
			debugKV("viewport", "fit_align_applied", "mode", g.zoomState.Mode, "axis", "y", "direction", "top")
		}
	case ZoomModeFitHeight:
		if g.config.FitHeightAlignLeft {
			g.zoomState.PanOffsetX = 1e12
			debugKV("viewport", "fit_align_applied", "mode", g.zoomState.Mode, "axis", "x", "direction", "left")
		}
	}
}

// getTransformedImageSize calculates the displayed image size after
// transformations, in source pixels (DisplayImage.SourceBounds) rather than
// decoded-texture pixels (DisplayImage.Bounds). ZoomLevel is defined as an
// absolute scale against source pixels (see ZoomState's doc comment), so
// every caller of this function -- the fit-scale calculation and pan
// clamping below -- needs the source-space size to keep that meaning stable
// regardless of whether the currently cached image is a budget-tier
// (reduced) or tierFull decode; only the renderer's actual draw scale needs
// to know about the texture size (see textureToSourceScale in renderer.go).
func (g *Game) getTransformedImageSize() (int, int) {
	content := g.displayContent
	if content == nil || content.LeftImage == nil {
		return 0, 0
	}

	var w, h int
	if content.RightImage == nil {
		w, h = content.LeftImage.SourceBounds().Dx(), content.LeftImage.SourceBounds().Dy()
	} else {
		leftW, leftH := content.LeftImage.SourceBounds().Dx(), content.LeftImage.SourceBounds().Dy()
		rightW, rightH := content.RightImage.SourceBounds().Dx(), content.RightImage.SourceBounds().Dy()
		w = leftW + rightW + imageGap
		h = int(math.Max(float64(leftH), float64(rightH)))
	}

	if g.rotationAngle == 90 || g.rotationAngle == 270 {
		return h, w
	}
	return w, h
}

// clampPanToLimits ensures pan offsets stay within valid boundaries.
func (g *Game) clampPanToLimits() {
	if g.zoomState.Mode == ZoomModeFitWindow {
		return
	}

	iw, ih := g.getTransformedImageSize()
	if iw == 0 || ih == 0 {
		return
	}

	deviceScale := ebiten.Monitor().DeviceScaleFactor()
	w := float64(g.currentLogicalW) * deviceScale
	h := float64(g.currentLogicalH) * deviceScale
	scale := g.zoomState.Level
	sw := float64(iw) * scale
	sh := float64(ih) * scale
	prevPanX := g.zoomState.PanOffsetX
	prevPanY := g.zoomState.PanOffsetY

	if sw > w {
		maxPanX := sw/2 - w/2
		minPanX := w/2 - sw/2
		if g.zoomState.PanOffsetX > maxPanX {
			g.zoomState.PanOffsetX = maxPanX
		} else if g.zoomState.PanOffsetX < minPanX {
			g.zoomState.PanOffsetX = minPanX
		}
	} else {
		g.zoomState.PanOffsetX = 0
	}

	if sh > h {
		maxPanY := sh/2 - h/2
		minPanY := h/2 - sh/2
		if g.zoomState.PanOffsetY > maxPanY {
			g.zoomState.PanOffsetY = maxPanY
		} else if g.zoomState.PanOffsetY < minPanY {
			g.zoomState.PanOffsetY = minPanY
		}
	} else {
		g.zoomState.PanOffsetY = 0
	}

	if debugMode && (prevPanX != g.zoomState.PanOffsetX || prevPanY != g.zoomState.PanOffsetY) {
		debugKV("viewport", "pan_clamped",
			"mode", g.zoomState.Mode,
			"prev_pan_x", prevPanX,
			"prev_pan_y", prevPanY,
			"next_pan_x", g.zoomState.PanOffsetX,
			"next_pan_y", g.zoomState.PanOffsetY,
			"scaled_width", sw,
			"scaled_height", sh,
			"viewport_width", w,
			"viewport_height", h,
		)
	}
}

// GetZoomMode for InputState interface (drag permission checking).
func (g *Game) GetZoomMode() ZoomMode {
	return g.zoomState.Mode
}

// Zoom and pan state methods for RenderState interface.
func (g *Game) GetZoomLevel() float64 {
	return g.zoomState.Level
}

func (g *Game) GetPanOffsetX() float64 {
	return g.zoomState.PanOffsetX
}

func (g *Game) GetPanOffsetY() float64 {
	return g.zoomState.PanOffsetY
}

// Zoom and pan actions for InputActions interface.
func (g *Game) ZoomIn() {
	g.zoomIn()
}

func (g *Game) ZoomOut() {
	g.zoomOut()
}

func (g *Game) ZoomReset() {
	g.zoomReset()
}

func (g *Game) ZoomFit() {
	g.zoomFit()
}

func (g *Game) PanUp() {
	g.panUp()
}

func (g *Game) PanDown() {
	g.panDown()
}

func (g *Game) PanLeft() {
	g.panLeft()
}

func (g *Game) PanRight() {
	g.panRight()
}

func (g *Game) PanByDelta(deltaX, deltaY float64) {
	g.panByDelta(deltaX, deltaY)
}

// decodeBudgetForSlot computes the load-time decode Hint for one display
// slot -- the box the image will actually be shown in, in real device
// pixels -- so a fresh decode can be requested at (close to) display size
// instead of always at full source resolution. bookSlot is true for either
// side of a two-up book-mode spread.
//
// This mirrors updateZoomLevelForFitMode's own device-pixel scaling
// (g.currentLogicalW/H times ebiten.Monitor().DeviceScaleFactor()); see
// decodeBudgetHint, the pure function that actually computes it, for the
// per-mode math.
func (g *Game) decodeBudgetForSlot(bookSlot bool) imgdecode.Hint {
	w, h := g.currentLogicalW, g.currentLogicalH
	if w <= 0 || h <= 0 {
		// The first images are requested from newGameFromStartup, before
		// RunGame and therefore before any Layout call has set the logical
		// size. Fall back to the size configureWindow is about to open the
		// window at, so the very first page -- the one the user waits on --
		// gets a budget too. Layout corrects this on the first frame.
		w, h = g.config.WindowWidth, g.config.WindowHeight
	}

	scale := 1.0
	if monitor := ebiten.Monitor(); monitor != nil {
		scale = monitor.DeviceScaleFactor()
	}
	return decodeBudgetHint(g.zoomState.Mode, w, h, scale, bookSlot)
}

// decodeBudgetHint is decodeBudgetForSlot's pure core, split out so it can
// be exercised with an arbitrary device scale factor without a live Ebiten
// monitor.
//
// Manual zoom always returns the unconstrained Hint{} (full resolution):
// it's a deliberate inspection action, and the refinement path
// (checkDecodeBudget/EnsureResolution) makes full resolution the natural
// target for it anyway, so there is no benefit to ever budget-decoding for
// it.
//
// bookSlot halves the device-pixel width allowance and removes this slot's
// share of the inter-page gap (imageGap, game_state.go), matching how
// calculateDisplayLayout (renderer.go) splits a two-up spread across the
// canvas width. The gap itself is not device-scaled: it is a small,
// constant pixel margin, and being off by a fraction of a device pixel here
// has no visible effect on a decode budget that is normally hundreds to
// thousands of pixels wide.
func decodeBudgetHint(mode ZoomMode, logicalW, logicalH int, deviceScale float64, bookSlot bool) imgdecode.Hint {
	if mode == ZoomModeManual {
		return imgdecode.Hint{}
	}

	w := float64(logicalW) * deviceScale
	h := float64(logicalH) * deviceScale
	if bookSlot {
		w = (w - float64(imageGap)) / 2
	}

	hint := imgdecode.Hint{}
	switch mode {
	case ZoomModeFitWindow:
		hint.MaxWidth = decodeBudgetDimension(w)
		hint.MaxHeight = decodeBudgetDimension(h)
	case ZoomModeFitWidth:
		hint.MaxWidth = decodeBudgetDimension(w)
	case ZoomModeFitHeight:
		hint.MaxHeight = decodeBudgetDimension(h)
	}
	return hint
}

// decodeBudgetDimension converts a device-pixel budget dimension to an
// imgdecode.Hint field, rounding up so the decode is never smaller than the
// display actually needs. A non-positive input (including the startup edge
// case where currentLogicalW/H is still 0, before the first Layout call)
// maps to 0 -- imgdecode.Hint's own "unconstrained" zero value -- rather
// than a bogus negative or zero cap.
func decodeBudgetDimension(devicePixels float64) int {
	if devicePixels <= 0 {
		return 0
	}
	return int(math.Ceil(devicePixels))
}

// checkDecodeBudget re-evaluates whether the currently visible image(s) are
// being drawn beyond their decoded size and, if so, requests a
// full-resolution refinement via ImageManager.EnsureResolution. Call this
// whenever the displayed resolution requirement can change: the zoom entry
// points above (zoomIn, zoomOut, switchToManual100, zoomFit's fit-mode
// switches) and Game.Layout (only when the logical size actually changed).
//
// The refinement condition itself -- ZoomLevel * SourceBounds().Dx() >
// Bounds().Dx(), and a higher-resolution decode actually existing -- is
// entirely EnsureResolution's job (via sufficientForHint and its own
// SourceBounds()==Bounds() check, see image.go). All this has to provide is
// *how large* the image is currently being drawn, in the same device-pixel
// units as SourceBounds/Bounds. EnsureResolution's fullRequested guard
// means calling this every frame (or on every wheel-scrolled zoom step)
// cannot produce a request storm.
func (g *Game) checkDecodeBudget() {
	if g.imageManager == nil {
		return
	}

	// Publish the single-slot budget for the preload goroutine. A book-mode
	// slot is narrower, so this errs towards decoding slightly larger than
	// a spread needs rather than too small to show at all.
	g.imageManager.SetDecodeBudget(g.decodeBudgetForSlot(false))

	content := g.displayContent
	if content == nil {
		return
	}

	g.ensureVisibleResolution(content.Metadata.LeftPage-1, content.LeftImage)
	if content.Metadata.ActualImages == 2 {
		g.ensureVisibleResolution(content.Metadata.RightPage-1, content.RightImage)
	}
}

// ensureVisibleResolution requests a full-resolution refinement for idx if
// img -- the image currently displayed there -- is being drawn larger than
// its own decoded size. See checkDecodeBudget for the full rationale.
func (g *Game) ensureVisibleResolution(idx int, img DisplayImage) {
	if img == nil || idx < 0 {
		return
	}

	src := img.SourceBounds()
	hint := imgdecode.Hint{
		MaxWidth:  decodeBudgetDimension(g.zoomState.Level * float64(src.Dx())),
		MaxHeight: decodeBudgetDimension(g.zoomState.Level * float64(src.Dy())),
	}
	g.imageManager.EnsureResolution(idx, hint)
}
