package main

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

func (g *Game) Update() error {
	if g.wasInputHandled {
		debugLog("waiting for previous input to complete\n")
	} else {
		g.wasInputHandled = g.inputHandler.HandleInput()
	}

	if g.overlayMessage != "" && time.Since(g.overlayMessageTime) >= overlayMessageDuration {
		g.overlayMessage = ""
		g.overlayMessageTime = time.Time{}
	}

	if g.imageManager.ConsumeAsyncRefresh() {
		g.calculateDisplayContent()
		g.renderer.lastSnapshot = nil
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

	if g.wasInputHandled ||
		g.renderer.lastSnapshot == nil ||
		!currentSnapshot.Equals(g.renderer.lastSnapshot) ||
		g.forceRedrawFrames > 0 {
		g.renderer.Draw(screen)
		g.renderer.lastSnapshot = currentSnapshot

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
