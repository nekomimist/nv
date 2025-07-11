package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// InputHandler handles all keyboard and mouse input processing
type InputHandler struct {
	inputActions        InputActions
	inputState          InputState
	keybindingManager   *KeybindingManager
	mousebindingManager *MousebindingManager
}

// NewInputHandler creates a new InputHandler
func NewInputHandler(inputActions InputActions, inputState InputState, keybindingManager *KeybindingManager, mousebindingManager *MousebindingManager) *InputHandler {
	return &InputHandler{
		inputActions:        inputActions,
		inputState:          inputState,
		keybindingManager:   keybindingManager,
		mousebindingManager: mousebindingManager,
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
	// Page input mode has special key handling
	if h.inputState.IsInPageInputMode() {
		return h.handlePageInputModeKeys()
	}

	// Normal input processing - unified with actionDefinitions
	for _, actionDef := range actionDefinitions {
		if h.keybindingManager.ExecuteAction(actionDef.Name, h.inputActions, h.inputState) {
			return true
		}
	}

	return false
}

func (h *InputHandler) handlePageInputMode() bool {
	// Check for G key to enter page input mode
	if !h.inputState.IsInPageInputMode() {
		return h.keybindingManager.ExecuteAction("page_input", h.inputActions, h.inputState)
	}

	// If in page input mode, delegate to specialized handler
	return h.handlePageInputModeKeys()
}

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

// handleMouseInput processes all mouse input for the current frame
func (h *InputHandler) handleMouseInput() bool {
	if h.mousebindingManager == nil {
		return false
	}

	// Process all mouse actions using actionDefinitions to ensure consistency
	// and automatic handling of new actions
	for _, actionDef := range actionDefinitions {
		if h.mousebindingManager.ExecuteAction(actionDef.Name, h.inputActions, h.inputState) {
			return true // Return immediately on first action processed
		}
	}

	return false
}
