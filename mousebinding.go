package main

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// MouseSettings contains mouse-specific configuration
type MouseSettings struct {
	WheelSensitivity float64 `json:"wheel_sensitivity"`
	DoubleClickTime  int     `json:"double_click_time"` // milliseconds
	DragThreshold    int     `json:"drag_threshold"`    // pixels
	EnableMouse      bool    `json:"enable_mouse"`
	WheelInverted    bool    `json:"wheel_inverted"`
	EnableDragPan    bool    `json:"enable_drag_pan"`   // Enable drag to pan
	DragSensitivity  float64 `json:"drag_sensitivity"`  // Drag movement sensitivity
	DragPanInverted  bool    `json:"drag_pan_inverted"` // Invert drag pan direction (both X and Y axes)
}

// DoubleClickTracker tracks double-click state
type DoubleClickTracker struct {
	lastClickTime   time.Time
	lastClickButton ebiten.MouseButton
	clickCount      int
}

// MouseCombination represents a mouse action with optional modifiers
type MouseCombination struct {
	Button        ebiten.MouseButton
	IsWheel       bool
	WheelDeltaX   float64
	WheelDeltaY   float64
	IsDoubleClick bool
	Shift         bool
	Ctrl          bool
	Alt           bool
}

// MousebindingManager handles dynamic mouse binding processing
type MousebindingManager struct {
	mousebindings      map[string][]string
	mouseMapping       map[string]ebiten.MouseButton
	settings           MouseSettings
	doubleClickTracker DoubleClickTracker
	parsed             map[string][]MouseCombination
}

// NewMousebindingManager creates a new MousebindingManager
func NewMousebindingManager(mousebindings map[string][]string, settings MouseSettings) *MousebindingManager {
	mm := &MousebindingManager{
		mousebindings: mousebindings,
		mouseMapping:  mouseActionToButton,
		settings:      settings,
		doubleClickTracker: DoubleClickTracker{
			lastClickTime: time.Now(),
			clickCount:    0,
		},
	}
	mm.rebuildParsed()
	return mm
}

// rebuildParsed parses the current mousebindings map into MouseCombinations
// once, so CheckAction doesn't need to re-parse mouse strings on every
// frame. Invalid strings are skipped here exactly as CheckAction skipped
// them before pre-parsing existed.
func (mm *MousebindingManager) rebuildParsed() {
	parsed := make(map[string][]MouseCombination, len(mm.mousebindings))
	for action, mouseStrings := range mm.mousebindings {
		combos := make([]MouseCombination, 0, len(mouseStrings))
		for _, mouseStr := range mouseStrings {
			if combination, valid := mm.parseMouseString(mouseStr); valid {
				combos = append(combos, *combination)
			}
		}
		parsed[action] = combos
	}
	mm.parsed = parsed
}

// parseMouseString parses a mouse string like "Shift+LeftClick" or "WheelUp"
// into a MouseCombination. Unknown modifier tokens are silently ignored (not
// treated as a parse failure); only an unrecognized trailing action name
// fails the parse.
func (mm *MousebindingManager) parseMouseString(mouseStr string) (*MouseCombination, bool) {
	name, shift, ctrl, alt, _ := splitModifiers(mouseStr)

	combination := &MouseCombination{Shift: shift, Ctrl: ctrl, Alt: alt}

	// Handle wheel actions
	if delta, exists := mouseWheelActionDeltas[name]; exists {
		combination.IsWheel = true
		combination.WheelDeltaX = delta.x
		combination.WheelDeltaY = delta.y
	} else if button, exists := mouseDoubleClickToButton[name]; exists {
		combination.IsDoubleClick = true
		combination.Button = button
	} else {
		button, exists := mm.mouseMapping[name]
		if !exists {
			return nil, false
		}
		combination.Button = button
	}

	return combination, true
}

// isMouseActionTriggered checks if a mouse combination is currently being triggered
func (mm *MousebindingManager) isMouseActionTriggered(combination *MouseCombination) bool {
	if !mm.settings.EnableMouse {
		return false
	}

	// Check modifiers first
	if combination.Shift && !ebiten.IsKeyPressed(ebiten.KeyShift) {
		return false
	}
	if combination.Ctrl && !ebiten.IsKeyPressed(ebiten.KeyControl) {
		return false
	}
	if combination.Alt && !ebiten.IsKeyPressed(ebiten.KeyAlt) {
		return false
	}

	// Check that unwanted modifiers aren't pressed
	if !combination.Shift && ebiten.IsKeyPressed(ebiten.KeyShift) {
		return false
	}
	if !combination.Ctrl && ebiten.IsKeyPressed(ebiten.KeyControl) {
		return false
	}
	if !combination.Alt && ebiten.IsKeyPressed(ebiten.KeyAlt) {
		return false
	}

	// Handle wheel actions
	if combination.IsWheel {
		wheelX, wheelY := ebiten.Wheel()

		// Apply sensitivity and inversion
		if mm.settings.WheelInverted {
			wheelY = -wheelY
		}
		wheelX *= mm.settings.WheelSensitivity
		wheelY *= mm.settings.WheelSensitivity

		// Check if wheel movement matches the expected direction
		if combination.WheelDeltaX != 0 {
			return (combination.WheelDeltaX > 0 && wheelX > 0) || (combination.WheelDeltaX < 0 && wheelX < 0)
		}
		if combination.WheelDeltaY != 0 {
			return (combination.WheelDeltaY > 0 && wheelY > 0) || (combination.WheelDeltaY < 0 && wheelY < 0)
		}
		return false
	}

	// Handle double-click actions
	if combination.IsDoubleClick {
		return mm.checkDoubleClick(combination.Button)
	}

	// Handle regular mouse button actions
	return inpututil.IsMouseButtonJustPressed(combination.Button)
}

// checkDoubleClick checks if a double-click occurred for the given button
func (mm *MousebindingManager) checkDoubleClick(button ebiten.MouseButton) bool {
	if !inpututil.IsMouseButtonJustPressed(button) {
		return false
	}

	now := time.Now()
	timeSinceLastClick := now.Sub(mm.doubleClickTracker.lastClickTime)

	// Check if this is the same button and within double-click time
	if mm.doubleClickTracker.lastClickButton == button &&
		timeSinceLastClick <= time.Duration(mm.settings.DoubleClickTime)*time.Millisecond {
		mm.doubleClickTracker.clickCount++
		if mm.doubleClickTracker.clickCount == 2 {
			// Reset for next potential double-click
			mm.doubleClickTracker.clickCount = 0
			mm.doubleClickTracker.lastClickTime = now
			return true
		}
	} else {
		// First click or different button
		mm.doubleClickTracker.clickCount = 1
		mm.doubleClickTracker.lastClickButton = button
	}

	mm.doubleClickTracker.lastClickTime = now
	return false
}

// CheckAction checks if any mouse binding for the given action is triggered
func (mm *MousebindingManager) CheckAction(action string) bool {
	combos, exists := mm.parsed[action]
	if !exists {
		return false
	}

	for i := range combos {
		if mm.isMouseActionTriggered(&combos[i]) {
			return true
		}
	}

	return false
}

// ExecuteAction executes the given action using the InputActions interface
func (mm *MousebindingManager) ExecuteAction(action string, inputActions InputActions, inputState InputState) bool {
	if !mm.CheckAction(action) {
		return false
	}

	return globalActionExecutor.ExecuteAction(action, inputActions, inputState)
}

// GetMousebindings returns the current mouse bindings map (for display purposes)
func (mm *MousebindingManager) GetMousebindings() map[string][]string {
	return mm.mousebindings
}

// UpdateMousebindings updates the mouse bindings map
func (mm *MousebindingManager) UpdateMousebindings(mousebindings map[string][]string) {
	mm.mousebindings = mousebindings
	mm.rebuildParsed()
}

// UpdateSettings updates the mouse settings
func (mm *MousebindingManager) UpdateSettings(settings MouseSettings) {
	mm.settings = settings
}

// GetSettings returns the current mouse settings
func (mm *MousebindingManager) GetSettings() MouseSettings {
	return mm.settings
}

// GetDefaultMouseSettings returns the default mouse settings
func GetDefaultMouseSettings() MouseSettings {
	return MouseSettings{
		WheelSensitivity: 1.0,
		DoubleClickTime:  300, // milliseconds
		DragThreshold:    5,   // pixels
		EnableMouse:      true,
		WheelInverted:    false,
		EnableDragPan:    true,  // Enable drag to pan by default
		DragSensitivity:  1.0,   // 1:1 mouse movement to pan ratio
		DragPanInverted:  false, // false = mouse/trackball style (drag to move image)
	}
}
