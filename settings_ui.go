package main

import (
	"fmt"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type settingControlKind uint8

const (
	settingControlNumber settingControlKind = iota
	settingControlBool
	settingControlEnum
)

type settingNumberKind uint8

const (
	settingNumberInt settingNumberKind = iota
	settingNumberFloat
)

// settingSpec is the single display-order and control-kind catalog used by
// both keyboard adjustment and the retained Settings UI.
type settingSpec struct {
	ID      string
	Label   string
	Control settingControlKind
	Options []string
	Restart bool
	Number  settingNumberKind
	Unit    string
	Hint    string
}

var editableSettingSpecs = []settingSpec{
	{ID: "WindowWidth", Label: "WindowWidth"},
	{ID: "WindowHeight", Label: "WindowHeight"},
	{ID: "DefaultWindowWidth", Label: "DefaultWindowWidth"},
	{ID: "DefaultWindowHeight", Label: "DefaultWindowHeight"},
	{ID: "Fullscreen", Label: "Fullscreen", Control: settingControlBool},
	{ID: "FontSize", Label: "FontSize", Number: settingNumberFloat},
	{ID: "BookMode", Label: "BookMode", Control: settingControlBool},
	{ID: "RightToLeft", Label: "RightToLeft", Control: settingControlBool},
	{ID: "SortMethod", Label: "SortMethod", Control: settingControlEnum, Options: []string{"Natural", "Simple", "Entry Order"}},
	{ID: "AspectRatioThreshold", Label: "AspectRatioThreshold", Number: settingNumberFloat},
	{ID: "InitialZoomMode", Label: "InitialZoomMode", Control: settingControlEnum, Options: []string{"fit_window", "fit_width", "fit_height", "actual_size"}},
	{ID: "FitWidthAlignTop", Label: "FitWidthAlignTop", Control: settingControlBool},
	{ID: "FitHeightAlignLeft", Label: "FitHeightAlignLeft", Control: settingControlBool},
	{ID: "MaxImageDimension", Label: "MaxImageDimension", Hint: "0 = Auto"},
	{ID: "CacheSize (restart)", Label: "CacheSize", Restart: true},
	{ID: "TransitionFrames", Label: "TransitionFrames"},
	{ID: "PreloadEnabled", Label: "PreloadEnabled", Control: settingControlBool},
	{ID: "PreloadCount", Label: "PreloadCount"},
	{ID: "Mouse.EnableMouse", Label: "Mouse.EnableMouse", Control: settingControlBool},
	{ID: "Mouse.WheelSensitivity", Label: "Mouse.WheelSensitivity", Number: settingNumberFloat},
	{ID: "Mouse.WheelInverted", Label: "Mouse.WheelInverted", Control: settingControlBool},
	{ID: "Mouse.EnableDragPan", Label: "Mouse.EnableDragPan", Control: settingControlBool},
	{ID: "Mouse.DragSensitivity", Label: "Mouse.DragSensitivity", Number: settingNumberFloat},
	{ID: "Mouse.DragPanInverted", Label: "Mouse.DragPanInverted", Control: settingControlBool},
	{ID: "Mouse.DoubleClickTime", Label: "Mouse.DoubleClickTime", Unit: "ms"},
	{ID: "Mouse.DragThreshold", Label: "Mouse.DragThreshold", Unit: "px"},
}

// settingsListOrder returns display order of editable items
func settingsListOrder() []string {
	order := make([]string, 0, len(editableSettingSpecs)+2)
	for _, spec := range editableSettingSpecs {
		order = append(order, spec.ID)
	}
	return append(order, "[ Save ]", "[ Cancel ]")
}

// getSettingValueString returns human-friendly value for the named item
func getSettingValueStringFromConfig(c Config, name string) string {
	switch name {
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
	case "MaxImageDimension":
		if c.MaxImageDimension == 0 {
			return fmt.Sprintf("Auto (%d)", defaultMaxImageDimension)
		}
		return fmt.Sprintf("%d", c.MaxImageDimension)
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

// getSettingNumericInputText returns an editable value without display-only
// decorations such as "Auto", "ms", or "px".
func getSettingNumericInputText(c Config, name string) (string, bool) {
	switch name {
	case "WindowWidth":
		return strconv.Itoa(c.WindowWidth), true
	case "WindowHeight":
		return strconv.Itoa(c.WindowHeight), true
	case "DefaultWindowWidth":
		return strconv.Itoa(c.DefaultWindowWidth), true
	case "DefaultWindowHeight":
		return strconv.Itoa(c.DefaultWindowHeight), true
	case "FontSize":
		return formatSettingFloat(c.FontSize), true
	case "AspectRatioThreshold":
		return formatSettingFloat(c.AspectRatioThreshold), true
	case "MaxImageDimension":
		return strconv.Itoa(c.MaxImageDimension), true
	case "CacheSize (restart)":
		return strconv.Itoa(c.CacheSize), true
	case "TransitionFrames":
		return strconv.Itoa(c.TransitionFrames), true
	case "PreloadCount":
		return strconv.Itoa(c.PreloadCount), true
	case "Mouse.WheelSensitivity":
		return formatSettingFloat(c.MouseSettings.WheelSensitivity), true
	case "Mouse.DragSensitivity":
		return formatSettingFloat(c.MouseSettings.DragSensitivity), true
	case "Mouse.DoubleClickTime":
		return strconv.Itoa(c.MouseSettings.DoubleClickTime), true
	case "Mouse.DragThreshold":
		return strconv.Itoa(c.MouseSettings.DragThreshold), true
	default:
		return "", false
	}
}

func formatSettingFloat(value float64) string {
	return strconv.FormatFloat(math.Round(value*100)/100, 'f', -1, 64)
}

func validSettingNumericInput(kind settingNumberKind, value string) bool {
	if value == "" {
		return true
	}
	dotSeen := false
	for _, r := range value {
		if r >= '0' && r <= '9' {
			continue
		}
		if kind == settingNumberFloat && r == '.' && !dotSeen {
			dotSeen = true
			continue
		}
		return false
	}
	return true
}

// applySettingNumericInput parses and clamps a committed editor value using
// the same limits as keyboard adjustment. The returned string is the
// normalized value that should be shown in the editor.
func applySettingNumericInput(c *Config, name, input string) (string, bool) {
	if c == nil || input == "" || input == "." {
		return "", false
	}

	parseInt := func() (int, bool) {
		value, err := strconv.Atoi(input)
		return value, err == nil
	}
	parseFloat := func() (float64, bool) {
		value, err := strconv.ParseFloat(input, 64)
		return value, err == nil && !math.IsInf(value, 0) && !math.IsNaN(value)
	}

	switch name {
	case "WindowWidth":
		value, ok := parseInt()
		if !ok {
			return "", false
		}
		c.WindowWidth = clampInt(value, windowWidthBound.min, windowWidthBound.max)
	case "WindowHeight":
		value, ok := parseInt()
		if !ok {
			return "", false
		}
		c.WindowHeight = clampInt(value, windowHeightBound.min, windowHeightBound.max)
	case "DefaultWindowWidth":
		value, ok := parseInt()
		if !ok {
			return "", false
		}
		c.DefaultWindowWidth = clampInt(value, defaultWindowWidthBound.min, defaultWindowWidthBound.max)
	case "DefaultWindowHeight":
		value, ok := parseInt()
		if !ok {
			return "", false
		}
		c.DefaultWindowHeight = clampInt(value, defaultWindowHeightBound.min, defaultWindowHeightBound.max)
	case "FontSize":
		value, ok := parseFloat()
		if !ok {
			return "", false
		}
		c.FontSize = clampFloat(value, fontSizeBound.min, fontSizeBound.max)
	case "AspectRatioThreshold":
		value, ok := parseFloat()
		if !ok {
			return "", false
		}
		c.AspectRatioThreshold = clampFloat(value, aspectRatioThresholdBound.min, aspectRatioThresholdBound.max)
	case "MaxImageDimension":
		value, ok := parseInt()
		if !ok {
			return "", false
		}
		if value <= 0 {
			c.MaxImageDimension = 0
		} else {
			c.MaxImageDimension = clampInt(value, 512, maxImageDimensionBound.max)
		}
	case "CacheSize (restart)":
		value, ok := parseInt()
		if !ok {
			return "", false
		}
		c.CacheSize = clampInt(value, cacheSizeBound.min, cacheSizeBound.max)
	case "TransitionFrames":
		value, ok := parseInt()
		if !ok {
			return "", false
		}
		c.TransitionFrames = clampInt(value, transitionFramesBound.min, transitionFramesBound.max)
	case "PreloadCount":
		value, ok := parseInt()
		if !ok {
			return "", false
		}
		c.PreloadCount = clampInt(value, preloadCountBound.min, preloadCountBound.max)
	case "Mouse.WheelSensitivity":
		value, ok := parseFloat()
		if !ok {
			return "", false
		}
		c.MouseSettings.WheelSensitivity = clampFloat(value, 0.1, 5.0)
	case "Mouse.DragSensitivity":
		value, ok := parseFloat()
		if !ok {
			return "", false
		}
		c.MouseSettings.DragSensitivity = clampFloat(value, 0.1, 5.0)
	case "Mouse.DoubleClickTime":
		value, ok := parseInt()
		if !ok {
			return "", false
		}
		c.MouseSettings.DoubleClickTime = clampInt(value, 100, 1000)
	case "Mouse.DragThreshold":
		value, ok := parseInt()
		if !ok {
			return "", false
		}
		c.MouseSettings.DragThreshold = clampInt(value, 1, 20)
	default:
		return "", false
	}

	normalized, _ := getSettingNumericInputText(*c, name)
	return normalized, true
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
	order := settingsListOrder()
	name := order[idx]
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

	switch name {
	case "WindowWidth":
		c.WindowWidth = clampInt(c.WindowWidth+stepSign*intStep, windowWidthBound.min, windowWidthBound.max)
	case "WindowHeight":
		c.WindowHeight = clampInt(c.WindowHeight+stepSign*intStep, windowHeightBound.min, windowHeightBound.max)
	case "DefaultWindowWidth":
		c.DefaultWindowWidth = clampInt(c.DefaultWindowWidth+stepSign*intStep, defaultWindowWidthBound.min, defaultWindowWidthBound.max)
	case "DefaultWindowHeight":
		c.DefaultWindowHeight = clampInt(c.DefaultWindowHeight+stepSign*intStep, defaultWindowHeightBound.min, defaultWindowHeightBound.max)
	case "FontSize":
		c.FontSize = clampFloat(c.FontSize+float64(stepSign)*floatStep, fontSizeBound.min, fontSizeBound.max)
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
		c.AspectRatioThreshold = clampFloat(c.AspectRatioThreshold+float64(stepSign)*0.1, aspectRatioThresholdBound.min, aspectRatioThresholdBound.max)
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
	case "MaxImageDimension":
		const minMaxImageDimension = 512 // UI-only "auto" collapse threshold
		if c.MaxImageDimension == 0 {
			if left {
				break
			}
			c.MaxImageDimension = defaultMaxImageDimension
			break
		}
		newValue := c.MaxImageDimension + stepSign*intStep
		if newValue <= minMaxImageDimension {
			c.MaxImageDimension = 0
		} else {
			c.MaxImageDimension = clampInt(newValue, minMaxImageDimension, maxImageDimensionBound.max)
		}
	case "CacheSize (restart)":
		c.CacheSize = clampInt(c.CacheSize+stepSign*1, cacheSizeBound.min, cacheSizeBound.max)
	case "TransitionFrames":
		c.TransitionFrames = clampInt(c.TransitionFrames+stepSign*1, transitionFramesBound.min, transitionFramesBound.max)
	case "Fullscreen":
		c.Fullscreen = !c.Fullscreen
	case "PreloadEnabled":
		c.PreloadEnabled = !c.PreloadEnabled
	case "PreloadCount":
		c.PreloadCount = clampInt(c.PreloadCount+stepSign*1, preloadCountBound.min, preloadCountBound.max)
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
	debugKV("config", "settings_adjust",
		"setting", name,
		"direction", map[bool]string{true: "left", false: "right"}[left],
		"value", getSettingValueStringFromConfig(g.pendingConfig, name),
	)
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
		debugKV("input", "action", "source", "settings", "action", "toggle_settings")
		return true
	}

	// Navigation
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if h.inputState.IsEditingSettingsValue() {
			debugKV("input", "action", "source", "settings", "action", "settings_edit_cancel")
			h.inputActions.CancelSettingsEdit()
			return true
		}
		debugKV("input", "action", "source", "settings", "action", "settings_cancel")
		h.inputActions.SettingsCancel()
		return true
	}
	if ebiten.IsKeyPressed(ebiten.KeyControl) && inpututil.IsKeyJustPressed(ebiten.KeyS) {
		debugKV("input", "action", "source", "settings", "action", "settings_save")
		h.inputActions.SettingsSave()
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		debugKV("input", "action", "source", "settings", "action", "settings_move_up")
		h.inputActions.SettingsMoveUp()
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		debugKV("input", "action", "source", "settings", "action", "settings_move_down")
		h.inputActions.SettingsMoveDown()
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) {
		if h.inputState.IsEditingSettingsValue() {
			return false
		}
		debugKV("input", "action", "source", "settings", "action", "settings_left")
		h.inputActions.SettingsLeft()
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) {
		if h.inputState.IsEditingSettingsValue() {
			return false
		}
		debugKV("input", "action", "source", "settings", "action", "settings_right")
		h.inputActions.SettingsRight()
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		if h.inputState.HasFocusedUIControl() {
			return false
		}
		debugKV("input", "action", "source", "settings", "action", "settings_enter")
		h.inputActions.SettingsEnter()
		return true
	}
	return false
}
