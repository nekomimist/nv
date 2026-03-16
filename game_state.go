package main

import (
	"fmt"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	// Book mode layout constants
	imageGap = 10 // Gap between images in book mode
)

// DisplayMetadata contains information about what is being displayed.
type DisplayMetadata struct {
	CurrentPage  int // Current page number (1-based)
	TotalPages   int // Total number of pages
	ActualImages int // Number of images actually being displayed (1 or 2)
}

// DisplayContent represents what should be displayed on screen.
type DisplayContent struct {
	LeftImage  *ebiten.Image   // Primary image (always present for single/left side of book)
	RightImage *ebiten.Image   // Secondary image (only present in book mode, nil for single)
	Metadata   DisplayMetadata // Display metadata for info overlay
}

type Game struct {
	imageManager        ImageManager
	inputHandler        *InputHandler
	renderer            *Renderer
	keybindingManager   *KeybindingManager
	mousebindingManager *MousebindingManager
	idx                 int
	fullscreen          bool
	bookMode            bool // Book/spread view mode
	tempSingleMode      bool // Temporary single page mode (return to book mode after navigation)
	showHelp            bool // Help overlay display
	showInfo            bool // Info display (page numbers, metadata, etc.)

	// Display content state (what should be rendered)
	displayContent *DisplayContent

	// Zoom and pan state
	zoomState              *ZoomState
	needsInitialZoomUpdate bool // Flag for updating zoom level on first draw
	needsInitialPanAlign   bool // Flag for applying initial pan alignment after zoom update

	// Page input mode state
	pageInputMode   bool
	pageInputBuffer string

	// Overlay message state (unified system for boundary, sort, direction messages)
	overlayMessage     string
	overlayMessageTime time.Time

	savedWinW       int // Window mode size for restoration (config save)
	savedWinH       int // Window mode size for restoration (config save)
	currentLogicalW int // Current logical size for zoom/pan calculations
	currentLogicalH int // Current logical size for zoom/pan calculations
	config          Config
	configPath      string // Custom config file path, empty for default

	// Image collection source state
	collectionSource     CollectionSource
	launchSingleFile     string // Original launch file path when started from a single regular image
	learnedSpreadAspects []float64

	// Image transformation state
	rotationAngle int  // 0, 90, 180, 270 degrees
	flipH         bool // Horizontal flip
	flipV         bool // Vertical flip

	// Rendering optimization state
	forceRedrawFrames int  // Force redraw for N frames
	wasInputHandled   bool // True if input was processed in this frame

	// Config status for help display
	configStatus ConfigLoadResult

	// Settings UI state
	showSettings  bool
	settingsIndex int
	pendingConfig Config

	exitRequested bool
	didShutdown   bool
}

func (g *Game) rotateLeft() {
	g.rotationAngle = (g.rotationAngle + 270) % 360
	g.showOverlayMessage(fmt.Sprintf("Rotation: %d°", g.rotationAngle))
}

func (g *Game) rotateRight() {
	g.rotationAngle = (g.rotationAngle + 90) % 360
	g.showOverlayMessage(fmt.Sprintf("Rotation: %d°", g.rotationAngle))
}

func (g *Game) flipHorizontal() {
	g.flipH = !g.flipH
	status := "OFF"
	if g.flipH {
		status = "ON"
	}
	g.showOverlayMessage(fmt.Sprintf("Flip Horizontal: %s", status))
}

func (g *Game) flipVertical() {
	g.flipV = !g.flipV
	status := "OFF"
	if g.flipV {
		status = "ON"
	}
	g.showOverlayMessage(fmt.Sprintf("Flip Vertical: %s", status))
}

func (g *Game) Exit() {
	g.exitRequested = true
}

// RenderState interface implementation
func (g *Game) IsFullscreen() bool {
	return g.fullscreen
}

func (g *Game) GetRotationAngle() int {
	return g.rotationAngle
}

func (g *Game) IsFlippedH() bool {
	return g.flipH
}

func (g *Game) IsFlippedV() bool {
	return g.flipV
}

func (g *Game) IsShowingHelp() bool {
	return g.showHelp
}

func (g *Game) IsShowingInfo() bool {
	return g.showInfo
}

func (g *Game) IsInPageInputMode() bool {
	return g.pageInputMode
}

func (g *Game) GetPageInputBuffer() string {
	return g.pageInputBuffer
}

func (g *Game) GetOverlayMessage() string {
	return g.overlayMessage
}

func (g *Game) GetOverlayMessageTime() time.Time {
	return g.overlayMessageTime
}

// Settings/InputState method
func (g *Game) IsInSettingsMode() bool { return g.showSettings }

// RenderState additions for settings overlay
func (g *Game) IsShowingSettings() bool  { return g.showSettings }
func (g *Game) GetPendingConfig() Config { return g.pendingConfig }
func (g *Game) GetSettingsIndex() int    { return g.settingsIndex }

func (g *Game) GetTotalPagesCount() int {
	return g.imageManager.GetPathsCount()
}

func (g *Game) GetFontSize() float64 {
	return g.config.FontSize
}

func (g *Game) GetConfigStatus() ConfigLoadResult {
	return g.configStatus
}

func (g *Game) GetKeybindings() map[string][]string {
	return g.keybindingManager.GetKeybindings()
}

func (g *Game) GetMousebindings() map[string][]string {
	return g.mousebindingManager.GetMousebindings()
}

func (g *Game) GetDisplayContent() *DisplayContent {
	return g.displayContent
}

// InputActions interface implementation
func (g *Game) ToggleHelp() {
	g.showHelp = !g.showHelp
}

func (g *Game) ToggleInfo() {
	g.showInfo = !g.showInfo
}

func (g *Game) ToggleBookMode() {
	g.toggleBookMode()
}

func (g *Game) EnterPageInputMode() {
	g.pageInputMode = true
	g.pageInputBuffer = ""
}

func (g *Game) ExitPageInputMode() {
	g.pageInputMode = false
	g.pageInputBuffer = ""
}

func (g *Game) ProcessPageInput() {
	g.processPageInput()
}

func (g *Game) UpdatePageInputBuffer(buffer string) {
	g.pageInputBuffer = buffer
}

func (g *Game) ToggleReadingDirection() {
	g.config.RightToLeft = !g.config.RightToLeft
	direction := "Left-to-Right"
	if g.config.RightToLeft {
		direction = "Right-to-Left"
	}
	g.showOverlayMessage("Reading Direction: " + direction)
}

func (g *Game) CycleSortMethod() {
	g.cycleSortMethod()
}

func (g *Game) MarkCurrentAsPreJoinedSpread() {
	g.markCurrentAsPreJoinedSpread()
}

func (g *Game) NavigateNext() {
	g.navigateNext(false)
	g.imageManager.StartPreload(g.idx, NavigationForward)
}

func (g *Game) NavigatePrevious() {
	g.navigatePrevious(false)
	g.imageManager.StartPreload(g.idx, NavigationBackward)
}

func (g *Game) NavigateNextSingle() {
	g.navigateNext(true)
	g.imageManager.StartPreload(g.idx, NavigationForward)
}

func (g *Game) NavigatePreviousSingle() {
	g.navigatePrevious(true)
	g.imageManager.StartPreload(g.idx, NavigationBackward)
}

func (g *Game) JumpToPage(page int) {
	g.jumpToPage(page)
}

func (g *Game) ExpandToDirectory() {
	g.expandToDirectoryAndJump()
	g.imageManager.StartPreload(g.idx, NavigationJump)
}

func (g *Game) RotateLeft() {
	g.rotateLeft()
}

func (g *Game) RotateRight() {
	g.rotateRight()
}

func (g *Game) FlipHorizontal() {
	g.flipHorizontal()
}

func (g *Game) FlipVertical() {
	g.flipVertical()
}
