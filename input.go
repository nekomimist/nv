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
func (h *InputHandler) HandleInput() {
	if h.inputActions.GetTotalPagesCount() == 0 {
		return
	}

	h.handleExitKeys()
	h.handleHelpToggle()
	h.handleInfoToggle()
	h.handlePageInputMode()
	h.handleModeToggleKeys()
	h.handleTransformationKeys()
	h.handleNavigationKeys()
	h.handleFullscreenToggle()
}

func (h *InputHandler) handleExitKeys() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		h.inputActions.Exit()
	}
}

func (h *InputHandler) handleHelpToggle() {
	if inpututil.IsKeyJustPressed(ebiten.KeySlash) && ebiten.IsKeyPressed(ebiten.KeyShift) {
		h.inputActions.ToggleHelp()
	}
}

func (h *InputHandler) handleInfoToggle() {
	if inpututil.IsKeyJustPressed(ebiten.KeyI) {
		h.inputActions.ToggleInfo()
	}
}

func (h *InputHandler) handlePageInputMode() {
	// Check for G key to enter page input mode
	if !h.inputState.IsInPageInputMode() {
		if inpututil.IsKeyJustPressed(ebiten.KeyG) {
			h.inputActions.EnterPageInputMode()
		}
		return
	}

	// Handle page input mode
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		// Cancel page input
		h.inputActions.ExitPageInputMode()
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		// Confirm page input
		h.inputActions.ProcessPageInput()
		h.inputActions.ExitPageInputMode()
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		// Delete last character
		currentBuffer := h.inputState.GetPageInputBuffer()
		if len(currentBuffer) > 0 {
			newBuffer := currentBuffer[:len(currentBuffer)-1]
			h.inputActions.UpdatePageInputBuffer(newBuffer)
		}
		return
	}

	// Handle digit input (both regular and numpad)
	var digit string
	if digit = h.checkDigitKeys(ebiten.Key0, ebiten.Key9, '0'); digit == "" {
		digit = h.checkDigitKeys(ebiten.KeyNumpad0, ebiten.KeyNumpad9, '0')
	}
	if digit != "" {
		currentBuffer := h.inputState.GetPageInputBuffer()
		h.inputActions.UpdatePageInputBuffer(currentBuffer + digit)
	}
}

func (h *InputHandler) checkDigitKeys(startKey, endKey ebiten.Key, baseChar rune) string {
	for key := startKey; key <= endKey; key++ {
		if inpututil.IsKeyJustPressed(key) {
			return string(baseChar + rune(key-startKey))
		}
	}
	return ""
}

func (h *InputHandler) handleModeToggleKeys() {
	if inpututil.IsKeyJustPressed(ebiten.KeyB) {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			// SHIFT+B: Toggle reading direction
			h.inputActions.ToggleReadingDirection()
		} else {
			// B: Toggle book mode
			h.inputActions.ToggleBookMode()
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			// SHIFT+S: Cycle sort method
			h.inputActions.CycleSortMethod()
		} else {
			// S: Scan directory images - only works for single file launch
			h.inputActions.ExpandToDirectory()
		}
	}
}

func (h *InputHandler) handleTransformationKeys() {
	// L: Rotate left 90 degrees
	if inpututil.IsKeyJustPressed(ebiten.KeyL) {
		h.inputActions.RotateLeft()
	}
	// R: Rotate right 90 degrees
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		h.inputActions.RotateRight()
	}
	// H: Flip horizontally
	if inpututil.IsKeyJustPressed(ebiten.KeyH) {
		h.inputActions.FlipHorizontal()
	}
	// V: Flip vertically
	if inpututil.IsKeyJustPressed(ebiten.KeyV) {
		h.inputActions.FlipVertical()
	}
}

func (h *InputHandler) handleNavigationKeys() {
	// Next page
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyN) {
		h.inputActions.NavigateNext()
	}
	// Previous page
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || inpututil.IsKeyJustPressed(ebiten.KeyP) {
		h.inputActions.NavigatePrevious()
	}
	// Jump to first page
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) || inpututil.IsKeyJustPressed(ebiten.KeyComma) {
		h.inputActions.JumpToPage(1)
	}
	// Jump to last page
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) || inpututil.IsKeyJustPressed(ebiten.KeyPeriod) {
		totalPages := h.inputActions.GetTotalPagesCount()
		if totalPages > 0 {
			h.inputActions.JumpToPage(totalPages)
		}
	}
}

func (h *InputHandler) handleFullscreenToggle() {
	if inpututil.IsKeyJustPressed(ebiten.KeyZ) {
		h.inputActions.ToggleFullscreen()
	}
}
