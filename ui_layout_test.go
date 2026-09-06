package main

import (
	"fmt"
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
)

func settingsLayoutTestController(t *testing.T, width, height int, fontSize float64) (*Game, *UIController, *ebiten.Image) {
	t.Helper()
	cfg := defaultConfig()
	cfg.FontSize = fontSize
	g := &Game{config: cfg, pendingConfig: cfg, showSettings: true}
	c := NewUIController(g)
	g.uiController = c
	screen := ebiten.NewImage(width, height)
	t.Cleanup(screen.Deallocate)
	c.Update()
	c.Draw(screen, nil)
	return g, c, screen
}

func TestGUI_SettingsSelectionScrollsIntoView(t *testing.T) {
	for _, size := range []image.Point{{800, 600}, {400, 300}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			g, c, screen := settingsLayoutTestController(t, size.X, size.Y, 32)
			for _, index := range []int{len(editableSettingSpecs) - 1, 0} {
				g.settingsIndex = index
				c.Update()
				c.Draw(screen, nil)
				if !c.NeedsRedraw() {
					t.Fatal("scroll adjustment did not request presentation")
				}
				c.Update()
				c.Draw(screen, nil)
				view := c.settingsScroller.ViewRect()
				row := c.settingsRows[index].editor.GetWidget().Rect
				// The scrollbar uses 1000 discrete positions.
				if row.Min.Y < view.Min.Y-2 || row.Max.Y > view.Max.Y+2 {
					t.Fatalf("selected row %d outside viewport: row=%v view=%v", index, row, view)
				}
			}
			// Manual scrolling must not snap back to the selected row.
			c.settingsScroller.ScrollTop = 0.5
			c.Draw(screen, nil)
			if c.settingsScroller.ScrollTop != 0.5 {
				t.Fatal("manual scrolling was overridden")
			}
		})
	}
}

func TestGUI_SettingsHorizontalScrollReachesEditors(t *testing.T) {
	for _, font := range []float64{20, 32} {
		t.Run(fmt.Sprint(font), func(t *testing.T) {
			_, c, screen := settingsLayoutTestController(t, 400, 300, font)
			area := c.settingsPanel.Children()[1].(*widget.Container)
			slider := area.Children()[2].(*widget.Slider)
			slider.Current = slider.Max
			// EbitenUI detects slider changes during Draw and dispatches
			// their events in the following Update.
			c.Draw(screen, nil)
			c.Update()
			if !c.NeedsRedraw() {
				t.Fatal("scroll event did not request presentation")
			}
			c.Draw(screen, nil)
			view := c.settingsScroller.ViewRect()
			plus := c.settingsRows[0].editor.Children()[2].GetWidget().Rect
			if plus.Min.X < view.Min.X || plus.Max.X > view.Max.X {
				t.Fatalf("rightmost numeric button unreachable: button=%v view=%v", plus, view)
			}
			if !slider.GetWidget().Rect.In(screen.Bounds()) {
				t.Fatalf("horizontal scrollbar outside screen: %v", slider.GetWidget().Rect)
			}
		})
	}
}

func TestGUI_SettingsFooterParticipatesInSelection(t *testing.T) {
	g, c, screen := settingsLayoutTestController(t, 400, 300, 32)
	if len(c.settingsRows) != len(settingsListOrder()) {
		t.Fatal("footer actions missing from selection rows")
	}
	for index := len(editableSettingSpecs); index < len(c.settingsRows); index++ {
		cell := c.settingsRows[index].editor
		button := cell.Children()[0].(*widget.Button)
		c.ui.SetFocusedWidget(button)
		c.Update()
		c.Draw(screen, nil)
		if g.settingsIndex != index || c.lastSettingsIndex != index {
			t.Fatalf("footer focus did not update selection: got %d want %d", g.settingsIndex, index)
		}
		if !cell.GetWidget().Rect.In(screen.Bounds()) {
			t.Fatalf("footer outside screen: %v", cell.GetWidget().Rect)
		}
	}
}
