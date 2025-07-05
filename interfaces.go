package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"time"
)

// RenderState provides read-only access to game state for the renderer
type RenderState interface {
	// Display modes
	IsBookMode() bool
	IsTempSingleMode() bool
	IsFullscreen() bool

	// Rendering data
	GetCurrentImage() *ebiten.Image
	GetBookModeImages() (*ebiten.Image, *ebiten.Image)
	ShouldUseBookMode(left, right *ebiten.Image) bool

	// Transformation state
	GetRotationAngle() int
	IsFlippedH() bool
	IsFlippedV() bool

	// UI state
	IsShowingHelp() bool
	IsShowingInfo() bool
	IsInPageInputMode() bool
	GetPageInputBuffer() string
	GetOverlayMessage() string
	GetOverlayMessageTime() time.Time

	// Display data
	GetCurrentPageNumber() string
	GetTotalPagesCount() int
	GetHelpFontSize() float64
}

// InputActions provides action methods for the input handler
type InputActions interface {
	// Application control
	Exit()

	// Display toggles
	ToggleHelp()
	ToggleInfo()
	ToggleBookMode()
	ToggleFullscreen()

	// Page input
	EnterPageInputMode()
	ExitPageInputMode()
	ProcessPageInput()
	UpdatePageInputBuffer(buffer string)

	// Settings
	ToggleReadingDirection()
	CycleSortMethod()

	// Navigation
	NavigateNext()
	NavigatePrevious()
	JumpToPage(page int)
	ExpandToDirectory()

	// Transformations
	RotateLeft()
	RotateRight()
	FlipHorizontal()
	FlipVertical()

	// Messages
	ShowOverlayMessage(message string)

	// Common data access
	GetCurrentIndex() int
	GetTotalPagesCount() int
	PreloadAdjacentImages(idx int)
}

// InputState provides read-only access to input-related state
type InputState interface {
	IsInPageInputMode() bool
	GetPageInputBuffer() string
}
