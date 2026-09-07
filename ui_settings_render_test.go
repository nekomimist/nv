package main

import "testing"

type settingsRenderProbe struct {
	RenderState
	pageDraws    int
	overlayDraws int
}

func (p *settingsRenderProbe) GetDisplayContent() *DisplayContent {
	p.pageDraws++
	return p.RenderState.GetDisplayContent()
}

func (p *settingsRenderProbe) IsShowingInfo() bool {
	p.overlayDraws++
	return p.RenderState.IsShowingInfo()
}

func TestGUI_SettingsSkipViewerRenderingAndRestoreOnClose(t *testing.T) {
	g, controller, screen := settingsLayoutTestController(t, 800, 600, 24)
	probe := &settingsRenderProbe{RenderState: g}
	g.renderer = NewRenderer(probe)
	g.Draw(screen)
	if probe.pageDraws != 0 || probe.overlayDraws != 0 {
		t.Fatal("settings drew the underlying viewer")
	}
	if got := controller.settingsOverlay.GetWidget().Rect; got != screen.Bounds() {
		t.Fatalf("settings background does not cover screen: got %v, want %v", got, screen.Bounds())
	}

	// UI activity must also skip the viewer on subsequent settings frames.
	controller.redrawRequested = true
	g.Draw(screen)
	if probe.pageDraws != 0 || probe.overlayDraws != 0 {
		t.Fatal("settings redraw drew the underlying viewer")
	}

	// Exercise the UI transition without wasInputHandled forcing a redraw.
	g.ToggleSettings()
	controller.Update()
	if !controller.NeedsRedraw() {
		t.Fatal("closing settings did not request a redraw")
	}
	g.Draw(screen)
	if probe.pageDraws != 1 || probe.overlayDraws != 1 {
		t.Fatalf("closing settings did not restore viewer rendering: pages=%d overlays=%d", probe.pageDraws, probe.overlayDraws)
	}
}
