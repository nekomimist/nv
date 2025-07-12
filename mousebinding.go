package main

import (
	"strings"
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
	EnableDragPan    bool    `json:"enable_drag_pan"`  // Enable drag to pan
	DragSensitivity  float64 `json:"drag_sensitivity"` // Drag movement sensitivity
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
}

// NewMousebindingManager creates a new MousebindingManager
func NewMousebindingManager(mousebindings map[string][]string, settings MouseSettings) *MousebindingManager {
	mm := &MousebindingManager{
		mousebindings: mousebindings,
		mouseMapping:  getMouseMapping(),
		settings:      settings,
		doubleClickTracker: DoubleClickTracker{
			lastClickTime: time.Now(),
			clickCount:    0,
		},
	}
	return mm
}

// getMouseMapping returns a mapping from string mouse actions to Ebiten mouse buttons
func getMouseMapping() map[string]ebiten.MouseButton {
	return map[string]ebiten.MouseButton{
		"LeftClick":   ebiten.MouseButtonLeft,
		"RightClick":  ebiten.MouseButtonRight,
		"MiddleClick": ebiten.MouseButtonMiddle,
		"Back":        ebiten.MouseButton3, // Back button (side button)
		"Forward":     ebiten.MouseButton4, // Forward button (side button)
	}
}

// parseMouseString parses a mouse string like "Shift+LeftClick" or "WheelUp" into a MouseCombination
func (mm *MousebindingManager) parseMouseString(mouseStr string) (*MouseCombination, bool) {
	parts := strings.Split(mouseStr, "+")
	if len(parts) == 0 {
		return nil, false
	}

	combination := &MouseCombination{}

	// Last part should be the actual mouse action
	actionName := parts[len(parts)-1]

	// Handle wheel actions
	if strings.HasPrefix(actionName, "Wheel") {
		combination.IsWheel = true
		switch actionName {
		case "WheelUp":
			combination.WheelDeltaY = 1.0
		case "WheelDown":
			combination.WheelDeltaY = -1.0
		case "WheelLeft":
			combination.WheelDeltaX = -1.0
		case "WheelRight":
			combination.WheelDeltaX = 1.0
		default:
			return nil, false
		}
	} else if strings.HasPrefix(actionName, "Double") {
		// Handle double-click actions
		combination.IsDoubleClick = true
		baseAction := strings.TrimPrefix(actionName, "Double")
		button, exists := mm.mouseMapping[baseAction]
		if !exists {
			return nil, false
		}
		combination.Button = button
	} else {
		// Handle regular mouse button actions
		button, exists := mm.mouseMapping[actionName]
		if !exists {
			return nil, false
		}
		combination.Button = button
	}

	// Check for modifiers
	for i := 0; i < len(parts)-1; i++ {
		switch strings.ToLower(parts[i]) {
		case "shift":
			combination.Shift = true
		case "ctrl":
			combination.Ctrl = true
		case "alt":
			combination.Alt = true
		}
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
	mouseStrings, exists := mm.mousebindings[action]
	if !exists {
		return false
	}

	for _, mouseStr := range mouseStrings {
		combination, valid := mm.parseMouseString(mouseStr)
		if valid && mm.isMouseActionTriggered(combination) {
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
		EnableDragPan:    true, // Enable drag to pan by default
		DragSensitivity:  1.0,  // 1:1 mouse movement to pan ratio
	}
}
