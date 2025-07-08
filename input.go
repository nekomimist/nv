package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// InputHandler handles all keyboard input processing
type InputHandler struct {
	inputActions InputActions
	inputState   InputState
}

// NewInputHandler creates a new InputHandler
func NewInputHandler(inputActions InputActions, inputState InputState) *InputHandler {
	return &InputHandler{
		inputActions: inputActions,
		inputState:   inputState,
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
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		h.inputActions.Exit()
		return true
	}
	return false
}

func (h *InputHandler) handleHelpToggle() bool {
	if inpututil.IsKeyJustPressed(ebiten.KeySlash) && ebiten.IsKeyPressed(ebiten.KeyShift) {
		h.inputActions.ToggleHelp()
		return true
	}
	return false
}

func (h *InputHandler) handleInfoToggle() bool {
	if inpututil.IsKeyJustPressed(ebiten.KeyI) {
		h.inputActions.ToggleInfo()
		return true
	}
	return false
}

func (h *InputHandler) handlePageInputMode() bool {
	// Check for G key to enter page input mode
	if !h.inputState.IsInPageInputMode() {
		if inpututil.IsKeyJustPressed(ebiten.KeyG) {
			h.inputActions.EnterPageInputMode()
			return true
		}
		return false
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

	if inpututil.IsKeyJustPressed(ebiten.KeyB) {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			// SHIFT+B: Toggle reading direction
			h.inputActions.ToggleReadingDirection()
		} else {
			// B: Toggle book mode
			h.inputActions.ToggleBookMode()
		}
		inputProcessed = true
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			// SHIFT+S: Cycle sort method
			h.inputActions.CycleSortMethod()
		} else {
			// S: Scan directory images - only works for single file launch
			h.inputActions.ExpandToDirectory()
		}
		inputProcessed = true
	}

	return inputProcessed
}

func (h *InputHandler) handleTransformationKeys() bool {
	inputProcessed := false

	// L: Rotate left 90 degrees
	if inpututil.IsKeyJustPressed(ebiten.KeyL) {
		h.inputActions.RotateLeft()
		inputProcessed = true
	}
	// R: Rotate right 90 degrees
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		h.inputActions.RotateRight()
		inputProcessed = true
	}
	// H: Flip horizontally
	if inpututil.IsKeyJustPressed(ebiten.KeyH) {
		h.inputActions.FlipHorizontal()
		inputProcessed = true
	}
	// V: Flip vertically
	if inpututil.IsKeyJustPressed(ebiten.KeyV) {
		h.inputActions.FlipVertical()
		inputProcessed = true
	}

	return inputProcessed
}

func (h *InputHandler) handleNavigationKeys() bool {
	inputProcessed := false

	// Next page
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyN) {
		h.inputActions.NavigateNext()
		inputProcessed = true
	}
	// Previous page
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || inpututil.IsKeyJustPressed(ebiten.KeyP) {
		h.inputActions.NavigatePrevious()
		inputProcessed = true
	}
	// Jump to first page
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) || inpututil.IsKeyJustPressed(ebiten.KeyComma) {
		h.inputActions.JumpToPage(1)
		inputProcessed = true
	}
	// Jump to last page
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) || inpututil.IsKeyJustPressed(ebiten.KeyPeriod) {
		totalPages := h.inputActions.GetTotalPagesCount()
		if totalPages > 0 {
			h.inputActions.JumpToPage(totalPages)
		}
		inputProcessed = true
	}

	return inputProcessed
}

func (h *InputHandler) handleFullscreenToggle() bool {
	if inpututil.IsKeyJustPressed(ebiten.KeyZ) {
		h.inputActions.ToggleFullscreen()
		return true
	}
	return false
}
