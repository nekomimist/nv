package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// InputHandler handles all keyboard input processing
type InputHandler struct {
	inputActions      InputActions
	inputState        InputState
	keybindingManager *KeybindingManager
}

// NewInputHandler creates a new InputHandler
func NewInputHandler(inputActions InputActions, inputState InputState, keybindingManager *KeybindingManager) *InputHandler {
	return &InputHandler{
		inputActions:      inputActions,
		inputState:        inputState,
		keybindingManager: keybindingManager,
	}
}

// HandleInput processes all input for the current frame
// Returns true if any input was processed, false otherwise
func (h *InputHandler) HandleInput() bool {
	if h.inputActions.GetTotalPagesCount() == 0 {
		return false
	}

	inputProcessed := false

	inputProcessed = h.handleExitKeys() || inputProcessed
	inputProcessed = h.handleHelpToggle() || inputProcessed
	inputProcessed = h.handleInfoToggle() || inputProcessed
	inputProcessed = h.handlePageInputMode() || inputProcessed
	inputProcessed = h.handleModeToggleKeys() || inputProcessed
	inputProcessed = h.handleTransformationKeys() || inputProcessed
	inputProcessed = h.handleNavigationKeys() || inputProcessed
	inputProcessed = h.handleFullscreenToggle() || inputProcessed

	return inputProcessed
}

func (h *InputHandler) handleExitKeys() bool {
	return h.keybindingManager.ExecuteAction("exit", h.inputActions, h.inputState)
}

func (h *InputHandler) handleHelpToggle() bool {
	return h.keybindingManager.ExecuteAction("help", h.inputActions, h.inputState)
}

func (h *InputHandler) handleInfoToggle() bool {
	return h.keybindingManager.ExecuteAction("info", h.inputActions, h.inputState)
}

func (h *InputHandler) handlePageInputMode() bool {
	// Check for G key to enter page input mode
	if !h.inputState.IsInPageInputMode() {
		return h.keybindingManager.ExecuteAction("page_input", h.inputActions, h.inputState)
	}

	// Handle page input mode
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

func (h *InputHandler) handleModeToggleKeys() bool {
	inputProcessed := false

	if h.keybindingManager.ExecuteAction("toggle_book_mode", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	if h.keybindingManager.ExecuteAction("toggle_reading_direction", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	if h.keybindingManager.ExecuteAction("cycle_sort", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	if h.keybindingManager.ExecuteAction("expand_directory", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	return inputProcessed
}

func (h *InputHandler) handleTransformationKeys() bool {
	inputProcessed := false

	if h.keybindingManager.ExecuteAction("rotate_left", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	if h.keybindingManager.ExecuteAction("rotate_right", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	if h.keybindingManager.ExecuteAction("flip_horizontal", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	if h.keybindingManager.ExecuteAction("flip_vertical", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	return inputProcessed
}

func (h *InputHandler) handleNavigationKeys() bool {
	inputProcessed := false

	if h.keybindingManager.ExecuteAction("next", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	if h.keybindingManager.ExecuteAction("previous", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	if h.keybindingManager.ExecuteAction("next_single", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	if h.keybindingManager.ExecuteAction("previous_single", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	if h.keybindingManager.ExecuteAction("jump_first", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	if h.keybindingManager.ExecuteAction("jump_last", h.inputActions, h.inputState) {
		inputProcessed = true
	}

	return inputProcessed
}

func (h *InputHandler) handleFullscreenToggle() bool {
	return h.keybindingManager.ExecuteAction("fullscreen", h.inputActions, h.inputState)
}
