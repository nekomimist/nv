package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// DragState manages mouse drag state for pan operations
type DragState struct {
	IsDragging  bool    // Whether drag is currently active
	StartX      int     // Drag start X coordinate
	StartY      int     // Drag start Y coordinate
	LastX       int     // Last known X coordinate during drag
	LastY       int     // Last known Y coordinate during drag
	TotalDeltaX float64 // Total accumulated X movement
	TotalDeltaY float64 // Total accumulated Y movement
}

// Reset clears all drag state
func (d *DragState) Reset() {
	d.IsDragging = false
	d.StartX = 0
	d.StartY = 0
	d.LastX = 0
	d.LastY = 0
	d.TotalDeltaX = 0
	d.TotalDeltaY = 0
}

// PendingMouseAction manages delayed mouse action execution to resolve drag/click conflicts
type PendingMouseAction struct {
	HasPending bool   // Whether there's a pending action
	Action     string // The action name to execute
	StartX     int    // Mouse position when action was triggered
	StartY     int    // Mouse position when action was triggered
}

// Reset clears the pending action
func (p *PendingMouseAction) Reset() {
	p.HasPending = false
	p.Action = ""
	p.StartX = 0
	p.StartY = 0
}

// SetPending sets a new pending action
func (p *PendingMouseAction) SetPending(action string, x, y int) {
	p.HasPending = true
	p.Action = action
	p.StartX = x
	p.StartY = y
}

// InputHandler handles all keyboard and mouse input processing
type InputHandler struct {
	inputActions        InputActions
	inputState          InputState
	keybindingManager   *KeybindingManager
	mousebindingManager *MousebindingManager
	dragState           *DragState          // Mouse drag state for pan operations
	pendingMouseAction  *PendingMouseAction // Delayed mouse action to resolve drag/click conflicts
}

// NewInputHandler creates a new InputHandler
func NewInputHandler(inputActions InputActions, inputState InputState, keybindingManager *KeybindingManager, mousebindingManager *MousebindingManager) *InputHandler {
	return &InputHandler{
		inputActions:        inputActions,
		inputState:          inputState,
		keybindingManager:   keybindingManager,
		mousebindingManager: mousebindingManager,
		dragState:           &DragState{},          // Initialize drag state
		pendingMouseAction:  &PendingMouseAction{}, // Initialize pending mouse action
	}
}

// HandleInput processes all input for the current frame
// Returns true if any input was processed, false otherwise
func (h *InputHandler) HandleInput() bool {
	if h.inputActions.GetTotalPagesCount() == 0 {
		return false
	}

	// Process keyboard input first
	if h.handleKeyboardInput() {
		return true
	}

	// Process mouse input if keyboard didn't handle anything
	return h.handleMouseInput()
}

// handleKeyboardInput processes all keyboard input for the current frame
func (h *InputHandler) handleKeyboardInput() bool {
	// Page input mode requires special handling for dynamic digit input
	if h.inputState.IsInPageInputMode() {
		return h.handlePageInputModeKeys()
	}

	// Normal input processing uses the action system
	for _, actionDef := range actionDefinitions {
		if h.keybindingManager.ExecuteAction(actionDef.Name, h.inputActions, h.inputState) {
			return true
		}
	}

	return false
}

// handlePageInputModeKeys handles keyboard input when in page input mode
// This bypasses the normal action system because page input needs to accept
// any digit key dynamically, which doesn't fit the predefined action model
func (h *InputHandler) handlePageInputModeKeys() bool {
	// Handle page input mode special keys
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		// Cancel page input
		h.inputActions.ExitPageInputMode()
		return true
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		// Confirm page input
		h.inputActions.ProcessPageInput()
		h.inputActions.ExitPageInputMode()
		return true
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		// Delete last character
		currentBuffer := h.inputState.GetPageInputBuffer()
		if len(currentBuffer) > 0 {
			newBuffer := currentBuffer[:len(currentBuffer)-1]
			h.inputActions.UpdatePageInputBuffer(newBuffer)
		}
		return true
	}

	// Handle digit input (both regular and numpad)
	var digit string
	if digit = h.checkDigitKeys(ebiten.Key0, ebiten.Key9, '0'); digit == "" {
		digit = h.checkDigitKeys(ebiten.KeyNumpad0, ebiten.KeyNumpad9, '0')
	}
	if digit != "" {
		currentBuffer := h.inputState.GetPageInputBuffer()
		h.inputActions.UpdatePageInputBuffer(currentBuffer + digit)
		return true
	}

	return false
}

func (h *InputHandler) checkDigitKeys(startKey, endKey ebiten.Key, baseChar rune) string {
	for key := startKey; key <= endKey; key++ {
		if inpututil.IsKeyJustPressed(key) {
			return string(baseChar + rune(key-startKey))
		}
	}
	return ""
}

// handleMouseInput processes all mouse input for the current frame with drag/click conflict resolution
func (h *InputHandler) handleMouseInput() bool {
	if h.mousebindingManager == nil {
		return false
	}

	// Handle pending action resolution first
	if h.handlePendingMouseAction() {
		return true
	}

	// Handle drag operations (with conflict-aware logic)
	if h.handleMouseDragWithConflictResolution() {
		return true
	}

	// Process non-LeftClick mouse actions immediately
	for _, actionDef := range actionDefinitions {
		// Skip LeftClick actions - they are handled by the conflict resolution system
		if h.isLeftClickAction(actionDef.Name) {
			continue
		}

		if h.mousebindingManager.ExecuteAction(actionDef.Name, h.inputActions, h.inputState) {
			return true // Return immediately on first action processed
		}
	}

	return false
}

// shouldAllowDrag determines if dragging should be allowed in the current state
func (h *InputHandler) shouldAllowDrag() bool {
	// Only allow drag in manual zoom mode (not fit mode)
	return h.inputState.GetZoomMode() == ZoomModeManual
}

// isLeftClickAction determines if an action is bound to LeftClick
func (h *InputHandler) isLeftClickAction(actionName string) bool {
	mouseStrings, exists := h.mousebindingManager.GetMousebindings()[actionName]
	if !exists {
		return false
	}

	for _, mouseStr := range mouseStrings {
		if mouseStr == "LeftClick" {
			return true
		}
	}
	return false
}

// handlePendingMouseAction processes pending mouse actions (delayed execution)
func (h *InputHandler) handlePendingMouseAction() bool {
	if !h.pendingMouseAction.HasPending {
		return false
	}

	// Check if left mouse button is still pressed
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		// Still holding button - don't execute yet
		return false
	}

	// Button released - execute the pending action
	action := h.pendingMouseAction.Action
	h.pendingMouseAction.Reset()

	return globalActionExecutor.ExecuteAction(action, h.inputActions, h.inputState)
}

// handleMouseDragWithConflictResolution handles drag with LeftClick conflict resolution
func (h *InputHandler) handleMouseDragWithConflictResolution() bool {
	// Get mouse settings for drag threshold
	mouseSettings := h.mousebindingManager.GetSettings()
	if !mouseSettings.EnableMouse || !mouseSettings.EnableDragPan {
		return false
	}

	// Get current mouse position
	mouseX, mouseY := ebiten.CursorPosition()

	// Check for drag start (left mouse button just pressed)
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		// Always check for LeftClick actions and make them pending (regardless of drag capability)
		h.checkAndSetPendingLeftClickActions(mouseX, mouseY)

		// Initialize drag state only if drag is allowed
		if h.shouldAllowDrag() {
			h.dragState.StartX = mouseX
			h.dragState.StartY = mouseY
			h.dragState.LastX = mouseX
			h.dragState.LastY = mouseY
			h.dragState.TotalDeltaX = 0
			h.dragState.TotalDeltaY = 0
			// Don't set IsDragging yet - wait for threshold
		}
		return false // Allow other non-LeftClick processing
	}

	// Check for drag continuation (left mouse button held down)
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && h.shouldAllowDrag() {
		// Calculate movement from start position
		deltaX := mouseX - h.dragState.StartX
		deltaY := mouseY - h.dragState.StartY

		// Check if we've moved beyond the drag threshold
		if !h.dragState.IsDragging {
			totalMovement := float64(deltaX*deltaX + deltaY*deltaY)
			threshold := float64(mouseSettings.DragThreshold * mouseSettings.DragThreshold)

			if totalMovement > threshold {
				// Start dragging - cancel any pending actions
				h.dragState.IsDragging = true
				h.pendingMouseAction.Reset() // Cancel pending LeftClick
				return true                  // Consume the input
			}
			return false // Still within threshold, allow other processing
		}

		// We're actively dragging - calculate movement since last frame
		frameDeltaX := float64(mouseX - h.dragState.LastX)
		frameDeltaY := float64(mouseY - h.dragState.LastY)

		// Update drag state
		h.dragState.LastX = mouseX
		h.dragState.LastY = mouseY
		h.dragState.TotalDeltaX += frameDeltaX
		h.dragState.TotalDeltaY += frameDeltaY

		// Apply pan movement with configurable inversion for both axes
		panDeltaX := frameDeltaX * mouseSettings.DragSensitivity
		panDeltaY := frameDeltaY * mouseSettings.DragSensitivity
		if mouseSettings.DragPanInverted {
			panDeltaX = -panDeltaX
			panDeltaY = -panDeltaY
		}
		h.inputActions.PanByDelta(panDeltaX, panDeltaY)

		return true // Consume the input
	}

	// Check for drag end (left mouse button just released)
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) && h.dragState.IsDragging {
		h.dragState.Reset()
		return true // Consume the input
	}

	return false
}

// checkAndSetPendingLeftClickActions checks for LeftClick actions and makes them pending
func (h *InputHandler) checkAndSetPendingLeftClickActions(mouseX, mouseY int) {
	for _, actionDef := range actionDefinitions {
		if h.isLeftClickAction(actionDef.Name) {
			if h.mousebindingManager.CheckAction(actionDef.Name) {
				// Found a LeftClick action that would trigger - make it pending
				h.pendingMouseAction.SetPending(actionDef.Name, mouseX, mouseY)
				break // Only one pending action at a time
			}
		}
	}
}
