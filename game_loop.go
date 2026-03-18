package main

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

func (g *Game) Update() error {
	if !g.wasInputHandled {
		g.wasInputHandled = g.inputHandler.HandleInput()
	}

	if g.overlayMessage != "" && time.Since(g.overlayMessageTime) >= overlayMessageDuration {
		g.overlayMessage = ""
		g.overlayMessageTime = time.Time{}
	}

	if g.imageManager.ConsumeAsyncRefresh() {
		g.calculateDisplayContent()
		g.renderer.lastSnapshot = nil
		debugKV("cache", "async_refresh", "idx", g.idx)
	}

	if g.exitRequested {
		g.shutdown()
		return ebiten.Termination
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	if g.needsInitialZoomUpdate {
		g.updateZoomLevelForFitMode()
		g.needsInitialZoomUpdate = false
		if g.needsInitialPanAlign {
			g.alignPanForCurrentFitModeIfConfigured()
			g.clampPanToLimits()
			g.needsInitialPanAlign = false
		}
	}

	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	currentSnapshot := NewRenderStateSnapshot(g, w, h)
	redrawReason := ""

	if g.wasInputHandled ||
		g.renderer.lastSnapshot == nil ||
		!currentSnapshot.Equals(g.renderer.lastSnapshot) ||
		g.forceRedrawFrames > 0 {
		switch {
		case g.wasInputHandled:
			redrawReason = "input_handled"
		case g.renderer.lastSnapshot == nil:
			redrawReason = "missing_snapshot"
		case !currentSnapshot.Equals(g.renderer.lastSnapshot):
			redrawReason = "snapshot_changed"
		case g.forceRedrawFrames > 0:
			redrawReason = "forced_redraw"
		}
		g.renderer.Draw(screen)
		g.renderer.lastSnapshot = currentSnapshot
		debugKV("renderer", "redraw", "reason", redrawReason, "width", w, "height", h, "force_redraw_frames", g.forceRedrawFrames)

		if g.forceRedrawFrames > 0 {
			g.forceRedrawFrames--
		}
		g.wasInputHandled = false
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	if g.currentLogicalW != outsideWidth || g.currentLogicalH != outsideHeight {
		g.currentLogicalW = outsideWidth
		g.currentLogicalH = outsideHeight
		g.forceRedrawFrames = 1
		debugKV("viewport", "layout_changed",
			"logical_width", outsideWidth,
			"logical_height", outsideHeight,
			"device_scale", ebiten.Monitor().DeviceScaleFactor(),
		)
	}

	if g.savedWinW != outsideWidth || g.savedWinH != outsideHeight {
		if !g.fullscreen {
			g.savedWinW = outsideWidth
			g.savedWinH = outsideHeight
		}
	}

	scale := ebiten.Monitor().DeviceScaleFactor()
	return int(float64(outsideWidth) * scale), int(float64(outsideHeight) * scale)
}
