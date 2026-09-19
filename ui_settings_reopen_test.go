package main

import (
	"testing"

	"github.com/ebitenui/ebitenui/widget"
)

func TestGUI_SettingsReopenRetainsControlsAndResetsState(t *testing.T) {
	for _, closeSettings := range []struct {
		name string
		run  func(*Game)
	}{
		{"cancel", (*Game).SettingsCancel},
		{"toggle", (*Game).ToggleSettings},
	} {
		t.Run(closeSettings.name, func(t *testing.T) {
			g, c, screen := settingsLayoutTestController(t, 400, 300, 24)
			g.zoomState = NewZoomState()
			input := c.numericInputs["FontSize"]
			checkbox := c.settingCheckboxes["BookMode"]
			combo := c.settingEnums["SortMethod"]
			scroller := c.settingsScroller
			settle := func() {
				for range 3 {
					c.Update()
					c.Draw(screen, nil)
				}
			}

			g.pendingConfig.FontSize = 28
			g.pendingConfig.BookMode = !g.config.BookMode
			setSettingEnumValue(&g.pendingConfig, "SortMethod", "Simple")
			settle()
			combo.SetContentVisible(true)
			settle()
			c.ui.SetFocusedWidget(input)
			settle()
			input.SetText("31.5") // Leave a focused edit uncommitted on close.
			c.settingsSlider.Current = c.settingsSlider.Max
			c.settingsHorizontalSlider.Current = c.settingsHorizontalSlider.Max
			settle()
			if scroller.ScrollTop == 0 || scroller.ScrollLeft == 0 {
				t.Fatal("test did not scroll away from the initial position")
			}

			closeSettings.run(g)
			settle()
			if !editableSettingsEqual(g.pendingConfig, g.config) {
				t.Fatal("deferred UI events changed discarded settings")
			}
			if combo.ContentVisible() || c.HasFocus() {
				t.Fatal("closing settings retained a popup or input focus")
			}

			g.ToggleSettings()
			settle()
			if c.numericInputs["FontSize"] != input || c.settingCheckboxes["BookMode"] != checkbox || c.settingEnums["SortMethod"] != combo || c.settingsScroller != scroller {
				t.Fatal("reopening settings recreated controls or their render buffers")
			}
			if got, want := input.GetText(), formatSettingFloat(g.config.FontSize); got != want {
				t.Fatalf("reopened numeric input = %q, want %q", got, want)
			}
			if (checkbox.State() == widget.WidgetChecked) != g.config.BookMode || combo.SelectedEntry() != getSettingValueStringFromConfig(g.config, "SortMethod") {
				t.Fatal("reopening settings retained discarded boolean or enum values")
			}
			if g.settingsIndex != 0 || c.lastSettingsIndex != 0 {
				t.Fatalf("deferred sync events moved selection: game=%d UI=%d", g.settingsIndex, c.lastSettingsIndex)
			}
			if scroller.ScrollTop != 0 || scroller.ScrollLeft != 0 || c.settingsSlider.Current != 0 || c.settingsHorizontalSlider.Current != 0 {
				t.Fatal("reopening settings did not reset both scroll axes")
			}
			if combo.ContentVisible() || c.HasFocus() {
				t.Fatal("reopening settings restored a popup or input focus")
			}

			// Retained controls must still deliver user changes after reopening.
			state := widget.WidgetChecked
			if g.config.BookMode {
				state = widget.WidgetUnchecked
			}
			checkbox.SetState(state)
			combo.SetSelectedEntry("Entry Order")
			settle()
			if g.pendingConfig.BookMode == g.config.BookMode || getSettingValueStringFromConfig(g.pendingConfig, "SortMethod") != "Entry Order" {
				t.Fatal("retained controls stopped updating pending settings")
			}
		})
	}
}

func TestGUI_SettingsReopenSyncsChangedConfig(t *testing.T) {
	g, c, screen := settingsLayoutTestController(t, 800, 600, 24)
	input := c.numericInputs["FontSize"]
	g.ToggleSettings()
	c.Update()
	c.Draw(screen, nil)

	// Saved settings may change while the panel is closed (for example reload).
	g.config.FontSize = 32
	g.config.BookMode = !g.config.BookMode
	setSettingEnumValue(&g.config, "SortMethod", "Entry Order")
	g.ToggleSettings()
	for range 3 {
		c.Update()
		c.Draw(screen, nil)
	}
	if c.numericInputs["FontSize"] != input || input.GetText() != "32" || c.lastUIFontSize != 32 {
		t.Fatal("retained numeric input or theme did not reflect the saved font size")
	}
	if (c.settingCheckboxes["BookMode"].State() == widget.WidgetChecked) != g.config.BookMode || c.settingEnums["SortMethod"].SelectedEntry() != "Entry Order" {
		t.Fatal("retained controls did not reflect changed saved settings")
	}
	if g.settingsIndex != 0 || !editableSettingsEqual(g.pendingConfig, g.config) {
		t.Fatal("deferred sync events changed the reopened settings state")
	}
}
