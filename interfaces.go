package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"time"
)

const (
	// Overlay message display duration
	overlayMessageDuration = 2 * time.Second
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

	// Zoom and pan state
	GetZoomMode() ZoomMode
	GetZoomLevel() float64
	GetPanOffsetX() float64
	GetPanOffsetY() float64

	// Display data
	GetCurrentPageNumber() string
	GetTotalPagesCount() int
	GetFontSize() float64
	GetConfigStatus() ConfigLoadResult
	GetKeybindings() map[string][]string
	GetMousebindings() map[string][]string
	GetMouseSettings() MouseSettings
}

// RenderStateSnapshot captures a snapshot of render state for comparison
// Only tracks fields that can change without key input
type RenderStateSnapshot struct {
	// Overlay message state (auto-expires after 2 seconds)
	OverlayMessage     string
	OverlayMessageTime time.Time

	// Window dimensions for resize detection
	WindowWidth  int
	WindowHeight int
}

// NewRenderStateSnapshot creates a lightweight snapshot of non-key-input state
// Only tracks fields that can change without key input
func NewRenderStateSnapshot(state RenderState, windowWidth, windowHeight int) *RenderStateSnapshot {
	return &RenderStateSnapshot{
		OverlayMessage:     state.GetOverlayMessage(),
		OverlayMessageTime: state.GetOverlayMessageTime(),
		WindowWidth:        windowWidth,
		WindowHeight:       windowHeight,
	}
}

// Equals checks if two snapshots are equal
func (s *RenderStateSnapshot) Equals(other *RenderStateSnapshot) bool {
	if other == nil {
		return false
	}

	// Helper function to check if overlay message is effectively active
	isOverlayActive := func(message string, messageTime time.Time) bool {
		return message != "" && time.Since(messageTime) < overlayMessageDuration
	}

	// Compare overlay states semantically rather than exact time values
	overlayEqual := func() bool {
		sActive := isOverlayActive(s.OverlayMessage, s.OverlayMessageTime)
		otherActive := isOverlayActive(other.OverlayMessage, other.OverlayMessageTime)

		// If both are inactive, check if the messages are the same
		// This ensures we detect transitions from active to inactive
		if !sActive && !otherActive {
			return s.OverlayMessage == other.OverlayMessage
		}

		// If both are active, compare messages and times
		if sActive && otherActive {
			return s.OverlayMessage == other.OverlayMessage &&
				s.OverlayMessageTime == other.OverlayMessageTime
		}

		// One active, one inactive - not equal
		return false
	}

	// Compare only fields that can change without key input
	return overlayEqual() &&
		s.WindowWidth == other.WindowWidth &&
		s.WindowHeight == other.WindowHeight
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
	ResetWindowSize()

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

	// Zoom and pan actions
	ZoomIn()
	ZoomOut()
	ZoomReset()
	ZoomFit()
	PanUp()
	PanDown()
	PanLeft()
	PanRight()
	PanByDelta(deltaX, deltaY float64) // Mouse drag pan

	// Messages
	ShowOverlayMessage(message string)

	// Common data access
	GetCurrentIndex() int
	GetTotalPagesCount() int
}

// InputState provides read-only access to input-related state
type InputState interface {
	IsInPageInputMode() bool
	GetPageInputBuffer() string
	GetZoomMode() ZoomMode // For drag permission checking
}
