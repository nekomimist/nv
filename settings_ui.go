package main

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Settings UI model is intentionally simple: a flat list of items with
// index-based dispatch. We edit a copy (pendingConfig) and apply on Save.

// settingsListOrder returns display order of editable items
func settingsListOrder() []string {
	return []string{
		"WindowWidth",
		"WindowHeight",
		"DefaultWindowWidth",
		"DefaultWindowHeight",
		"Fullscreen",
		"FontSize",
		"BookMode",
		"RightToLeft",
		"SortMethod",
		"AspectRatioThreshold",
		"InitialZoomMode",
		"FitWidthAlignTop",
		"FitHeightAlignLeft",
		"CacheSize (restart)",
		"TransitionFrames",
		"PreloadEnabled",
		"PreloadCount",
		"Mouse.EnableMouse",
		"Mouse.WheelSensitivity",
		"Mouse.WheelInverted",
		"Mouse.EnableDragPan",
		"Mouse.DragSensitivity",
		"Mouse.DragPanInverted",
		"Mouse.DoubleClickTime",
		"Mouse.DragThreshold",
		"[ Save ]",
		"[ Cancel ]",
	}
}

// (moved) settings overlay drawing lives in renderer.go

// getSettingValueString returns human-friendly value for the i-th item
func getSettingValueStringFromConfig(c Config, i int) string {
	switch settingsListOrder()[i] {
	case "WindowWidth":
		return fmt.Sprintf("%d", c.WindowWidth)
	case "WindowHeight":
		return fmt.Sprintf("%d", c.WindowHeight)
	case "DefaultWindowWidth":
		return fmt.Sprintf("%d", c.DefaultWindowWidth)
	case "DefaultWindowHeight":
		return fmt.Sprintf("%d", c.DefaultWindowHeight)
	case "Fullscreen":
		if c.Fullscreen {
			return "ON"
		}
		return "OFF"
	case "FontSize":
		return fmt.Sprintf("%.1f", c.FontSize)
	case "BookMode":
		if c.BookMode {
			return "ON"
		}
		return "OFF"
	case "RightToLeft":
		if c.RightToLeft {
			return "RTL"
		}
		return "LTR"
	case "SortMethod":
		return getSortMethodName(c.SortMethod)
	case "AspectRatioThreshold":
		return fmt.Sprintf("%.2f", c.AspectRatioThreshold)
	case "InitialZoomMode":
		return c.InitialZoomMode
	case "FitWidthAlignTop":
		if c.FitWidthAlignTop {
			return "ON"
		}
		return "OFF"
	case "FitHeightAlignLeft":
		if c.FitHeightAlignLeft {
			return "ON"
		}
		return "OFF"
	case "CacheSize (restart)":
		return fmt.Sprintf("%d", c.CacheSize)
	case "TransitionFrames":
		return fmt.Sprintf("%d", c.TransitionFrames)
	case "PreloadEnabled":
		if c.PreloadEnabled {
			return "ON"
		}
		return "OFF"
	case "PreloadCount":
		return fmt.Sprintf("%d", c.PreloadCount)
	case "Mouse.EnableMouse":
		if c.MouseSettings.EnableMouse {
			return "ON"
		}
		return "OFF"
	case "Mouse.WheelSensitivity":
		return fmt.Sprintf("%.1f", c.MouseSettings.WheelSensitivity)
	case "Mouse.WheelInverted":
		if c.MouseSettings.WheelInverted {
			return "ON"
		}
		return "OFF"
	case "Mouse.EnableDragPan":
		if c.MouseSettings.EnableDragPan {
			return "ON"
		}
		return "OFF"
	case "Mouse.DragSensitivity":
		return fmt.Sprintf("%.1f", c.MouseSettings.DragSensitivity)
	case "Mouse.DragPanInverted":
		if c.MouseSettings.DragPanInverted {
			return "ON"
		}
		return "OFF"
	case "Mouse.DoubleClickTime":
		return fmt.Sprintf("%d ms", c.MouseSettings.DoubleClickTime)
	case "Mouse.DragThreshold":
		return fmt.Sprintf("%d px", c.MouseSettings.DragThreshold)
	case "[ Save ]":
		return ""
	case "[ Cancel ]":
		return ""
	default:
		return ""
	}
}

// mutate helpers
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// settingsAdjust changes current item by delta (int) or deltaF (float)
func (g *Game) settingsAdjust(left bool) {
	idx := g.settingsIndex
	c := g.pendingConfig
	stepSign := 1
	if left {
		stepSign = -1
	}
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl)

	// variable steps
	intStep := 10
	if shift {
		intStep = 100
	}
	if ctrl {
		intStep = 1
	}
	floatStep := 0.1
	if shift {
		floatStep = 0.5
	}
	if ctrl {
		floatStep = 0.05
	}

	switch settingsListOrder()[idx] {
	case "WindowWidth":
		c.WindowWidth = clampInt(c.WindowWidth+stepSign*intStep, minWidth, 8192)
	case "WindowHeight":
		c.WindowHeight = clampInt(c.WindowHeight+stepSign*intStep, minHeight, 8192)
	case "DefaultWindowWidth":
		c.DefaultWindowWidth = clampInt(c.DefaultWindowWidth+stepSign*intStep, minWidth, 8192)
	case "DefaultWindowHeight":
		c.DefaultWindowHeight = clampInt(c.DefaultWindowHeight+stepSign*intStep, minHeight, 8192)
	case "FontSize":
		c.FontSize = clampFloat(c.FontSize+float64(stepSign)*floatStep, 10.0, 72.0)
	case "BookMode":
		c.BookMode = !c.BookMode
	case "RightToLeft":
		c.RightToLeft = !c.RightToLeft
	case "SortMethod":
		if left {
			c.SortMethod = (c.SortMethod + 3 - 1) % 3
		} else {
			c.SortMethod = (c.SortMethod + 1) % 3
		}
	case "AspectRatioThreshold":
		c.AspectRatioThreshold = clampFloat(c.AspectRatioThreshold+float64(stepSign)*0.1, 1.0, 3.0)
	case "InitialZoomMode":
		modes := []string{"fit_window", "fit_width", "fit_height", "actual_size"}
		cur := 0
		for i, m := range modes {
			if m == c.InitialZoomMode {
				cur = i
				break
			}
		}
		if left {
			cur = (cur + len(modes) - 1) % len(modes)
		} else {
			cur = (cur + 1) % len(modes)
		}
		c.InitialZoomMode = modes[cur]
	case "FitWidthAlignTop":
		c.FitWidthAlignTop = !c.FitWidthAlignTop
	case "FitHeightAlignLeft":
		c.FitHeightAlignLeft = !c.FitHeightAlignLeft
	case "CacheSize (restart)":
		c.CacheSize = clampInt(c.CacheSize+stepSign*1, 1, 64)
	case "TransitionFrames":
		c.TransitionFrames = clampInt(c.TransitionFrames+stepSign*1, 0, 60)
	case "Fullscreen":
		c.Fullscreen = !c.Fullscreen
	case "PreloadEnabled":
		c.PreloadEnabled = !c.PreloadEnabled
	case "PreloadCount":
		c.PreloadCount = clampInt(c.PreloadCount+stepSign*1, 1, 16)
	case "Mouse.EnableMouse":
		c.MouseSettings.EnableMouse = !c.MouseSettings.EnableMouse
	case "Mouse.WheelSensitivity":
		c.MouseSettings.WheelSensitivity = clampFloat(c.MouseSettings.WheelSensitivity+float64(stepSign)*floatStep, 0.1, 5.0)
	case "Mouse.WheelInverted":
		c.MouseSettings.WheelInverted = !c.MouseSettings.WheelInverted
	case "Mouse.EnableDragPan":
		c.MouseSettings.EnableDragPan = !c.MouseSettings.EnableDragPan
	case "Mouse.DragSensitivity":
		c.MouseSettings.DragSensitivity = clampFloat(c.MouseSettings.DragSensitivity+float64(stepSign)*floatStep, 0.1, 5.0)
	case "Mouse.DragPanInverted":
		c.MouseSettings.DragPanInverted = !c.MouseSettings.DragPanInverted
	case "Mouse.DoubleClickTime":
		c.MouseSettings.DoubleClickTime = clampInt(c.MouseSettings.DoubleClickTime+stepSign*50, 100, 1000)
	case "Mouse.DragThreshold":
		c.MouseSettings.DragThreshold = clampInt(c.MouseSettings.DragThreshold+stepSign*1, 1, 20)
	}
	g.pendingConfig = c
}

// settingsToggle toggles bool/enums or triggers save/cancel on Enter
func (g *Game) settingsToggleOrEnter() {
	switch settingsListOrder()[g.settingsIndex] {
	case "[ Save ]":
		g.SettingsSave()
	case "[ Cancel ]":
		g.SettingsCancel()
	default:
		g.settingsAdjust(false) // right as toggle/cycle
	}
}

// handleSettingsModeKeys processes keys when the settings panel is open
func (h *InputHandler) handleSettingsModeKeys() bool {
	// Allow the dedicated action to close the panel
	if h.keybindingManager.ExecuteAction("toggle_settings", h.inputActions, h.inputState) {
		return true
	}

	// Navigation
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		h.inputActions.SettingsCancel()
		return true
	}
	if ebiten.IsKeyPressed(ebiten.KeyControl) && inpututil.IsKeyJustPressed(ebiten.KeyS) {
		h.inputActions.SettingsSave()
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		h.inputActions.SettingsMoveUp()
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		h.inputActions.SettingsMoveDown()
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) {
		h.inputActions.SettingsLeft()
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) {
		h.inputActions.SettingsRight()
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		h.inputActions.SettingsEnter()
		return true
	}
	return false
}
