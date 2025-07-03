package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// InputHandler handles all keyboard input processing
type InputHandler struct {
	game *Game
}

// NewInputHandler creates a new InputHandler
func NewInputHandler(game *Game) *InputHandler {
	return &InputHandler{
		game: game,
	}
}

// HandleInput processes all input for the current frame
func (h *InputHandler) HandleInput() {
	if h.game.imageManager.GetPathsCount() == 0 {
		return
	}

	h.handleExitKeys()
	h.handleHelpToggle()
	h.handleInfoToggle()
	h.handlePageInputMode()
	h.handleModeToggleKeys()
	h.handleNavigationKeys()
	h.handleFullscreenToggle()
}

func (h *InputHandler) handleExitKeys() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		h.game.saveCurrentWindowSize()
		h.game.Exit()
	}
}

func (h *InputHandler) handleHelpToggle() {
	if inpututil.IsKeyJustPressed(ebiten.KeyH) {
		h.game.showHelp = !h.game.showHelp
	}
}

func (h *InputHandler) handleInfoToggle() {
	if inpututil.IsKeyJustPressed(ebiten.KeyI) {
		h.game.showInfo = !h.game.showInfo
	}
}

func (h *InputHandler) handlePageInputMode() {
	// Check for G key to enter page input mode
	if !h.game.pageInputMode {
		if inpututil.IsKeyJustPressed(ebiten.KeyG) {
			h.game.pageInputMode = true
			h.game.pageInputBuffer = ""
		}
		return
	}

	// Handle page input mode
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		// Cancel page input
		h.game.pageInputMode = false
		h.game.pageInputBuffer = ""
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		// Confirm page input
		h.game.processPageInput()
		h.game.pageInputMode = false
		h.game.pageInputBuffer = ""
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		// Delete last character
		if len(h.game.pageInputBuffer) > 0 {
			h.game.pageInputBuffer = h.game.pageInputBuffer[:len(h.game.pageInputBuffer)-1]
		}
		return
	}

	// Handle digit input (both regular and numpad)
	var digit string
	if digit = h.checkDigitKeys(ebiten.Key0, ebiten.Key9, '0'); digit == "" {
		digit = h.checkDigitKeys(ebiten.KeyNumpad0, ebiten.KeyNumpad9, '0')
	}
	if digit != "" {
		h.game.pageInputBuffer += digit
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
			h.game.config.RightToLeft = !h.game.config.RightToLeft
			h.game.saveCurrentConfig()

			// Show direction change message
			direction := "Left-to-Right"
			if h.game.config.RightToLeft {
				direction = "Right-to-Left"
			}
			h.game.showOverlayMessage("Reading Direction: " + direction)
		} else {
			// B: Toggle book mode
			h.game.toggleBookMode()
			h.game.imageManager.PreloadAdjacentImages(h.game.idx)
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			// SHIFT+S: Cycle sort method
			h.game.cycleSortMethod()
		}
	}
}

func (h *InputHandler) handleNavigationKeys() {
	// Next page
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyN) {
		h.game.navigateNext()
		h.game.imageManager.PreloadAdjacentImages(h.game.idx)
	}
	// Previous page
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || inpututil.IsKeyJustPressed(ebiten.KeyP) {
		h.game.navigatePrevious()
		h.game.imageManager.PreloadAdjacentImages(h.game.idx)
	}
	// Jump to first page
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) || inpututil.IsKeyJustPressed(ebiten.KeyComma) {
		h.game.jumpToPage(1)
	}
	// Jump to last page
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) || inpututil.IsKeyJustPressed(ebiten.KeyPeriod) {
		totalPages := h.game.imageManager.GetPathsCount()
		if totalPages > 0 {
			h.game.jumpToPage(totalPages)
		}
	}
	// Load directory images (L key) - only works for single file launch
	if inpututil.IsKeyJustPressed(ebiten.KeyL) {
		h.game.expandToDirectoryAndJump()
	}
}

func (h *InputHandler) handleFullscreenToggle() {
	if inpututil.IsKeyJustPressed(ebiten.KeyZ) {
		h.game.toggleFullscreen()
	}
}
