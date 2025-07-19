package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// Build-time variables (set by ldflags)
var (
	version   = "dev"
	buildDate = "unknown"
)

// Global debug mode flag
var debugMode bool

//go:embed icon/icon_16.png
var icon16 []byte

//go:embed icon/icon_32.png
var icon32 []byte

//go:embed icon/icon_48.png
var icon48 []byte

const (
	// Book mode layout constants
	imageGap = 10 // Gap between images in book mode

	// Aspect ratio thresholds
	minAspectRatio = 0.4 // Extremely tall images
	maxAspectRatio = 2.5 // Extremely wide images
)

// ZoomMode represents the current zoom mode
type ZoomMode int

const (
	ZoomModeFit    ZoomMode = iota // Automatic fit to window (default)
	ZoomModeManual                 // Manual zoom level
)

// ZoomState manages zoom and pan state
type ZoomState struct {
	Mode       ZoomMode // Current zoom mode
	Level      float64  // Zoom level (1.0 = 100%, 2.0 = 200%, etc.)
	PanOffsetX float64  // Pan offset X coordinate
	PanOffsetY float64  // Pan offset Y coordinate
}

// NewZoomState creates a new zoom state with default values
func NewZoomState() *ZoomState {
	return &ZoomState{
		Mode:       ZoomModeFit,
		Level:      1.0,
		PanOffsetX: 0,
		PanOffsetY: 0,
	}
}

// Reset resets zoom state to fit mode (called when switching to fit or changing images)
func (z *ZoomState) Reset() {
	z.Mode = ZoomModeFit
	z.Level = 1.0
	z.PanOffsetX = 0
	z.PanOffsetY = 0
}

func isArchiveExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".zip", ".rar", ".7z":
		return true
	default:
		return false
	}
}

func isSupportedExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif":
		return true
	default:
		return false
	}
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

	// Zoom and pan state
	zoomState *ZoomState

	// Page input mode state
	pageInputMode   bool
	pageInputBuffer string

	// Overlay message state (unified system for boundary, sort, direction messages)
	overlayMessage     string
	overlayMessageTime time.Time

	savedWinW  int
	savedWinH  int
	config     Config
	configPath string // Custom config file path, empty for default

	// Single file expansion mode state
	originalArgs       []string // Original command line arguments
	expandedFromSingle bool     // Whether the current file list was expanded from a single file
	originalFileIndex  int      // Index of the original file in the expanded list

	// Image transformation state
	rotationAngle int  // 0, 90, 180, 270 degrees
	flipH         bool // Horizontal flip
	flipV         bool // Vertical flip

	// Rendering optimization state
	forceRedrawFrames int  // Force redraw for N frames
	wasInputHandled   bool // True if input was processed in this frame

	// Config status for help display
	configStatus ConfigLoadResult
}

func (g *Game) getCurrentImage() *ebiten.Image {
	return g.imageManager.GetCurrentImage(g.idx)
}

func (g *Game) getBookModeImages() (*ebiten.Image, *ebiten.Image) {
	return g.imageManager.GetBookModeImages(g.idx, g.config.RightToLeft)
}

func (g *Game) saveCurrentConfig() {
	if g.configPath != "" {
		saveConfigToPath(g.config, g.configPath)
	} else {
		saveConfig(g.config)
	}
}

func (g *Game) rotateLeft() {
	g.rotationAngle = (g.rotationAngle + 270) % 360
}

func (g *Game) rotateRight() {
	g.rotationAngle = (g.rotationAngle + 90) % 360
}

func (g *Game) flipHorizontal() {
	g.flipH = !g.flipH
}

func (g *Game) flipVertical() {
	g.flipV = !g.flipV
}

func (g *Game) cycleSortMethod() {
	// Cycle through sort methods
	g.config.SortMethod = (g.config.SortMethod + 1) % 3

	// Show message
	g.showOverlayMessage("Sort: " + getSortMethodName(g.config.SortMethod))

	// Re-collect and sort images
	args := flag.Args()
	if len(args) > 0 {
		paths, err := collectImages(args, g.config.SortMethod)
		if err == nil && len(paths) > 0 {
			g.imageManager.SetPaths(paths)
			// Reset to first image
			g.idx = 0
		}
	}
}

// Zoom and pan implementation methods
func (g *Game) zoomIn() {
	if g.zoomState.Mode == ZoomModeFit {
		// Switch to manual mode and start at 100%
		g.switchToManual100()
	} else {
		// Increase zoom level
		newLevel := g.zoomState.Level * 1.25
		if newLevel > 4.0 { // Max zoom 400%
			// Clamp to exactly 400%
			g.zoomState.Level = 4.0
			g.showOverlayMessage("Maximum zoom 400%")
		} else {
			g.zoomState.Level = newLevel
			g.showOverlayMessage(fmt.Sprintf("%.0f%%", g.zoomState.Level*100))
		}
	}
}

func (g *Game) zoomOut() {
	if g.zoomState.Mode == ZoomModeFit {
		// Switch to manual mode and start at 100%
		g.switchToManual100()
	} else {
		// Decrease zoom level
		newLevel := g.zoomState.Level / 1.25
		if newLevel < 0.25 { // Min zoom 25%
			// Clamp to exactly 25%
			g.zoomState.Level = 0.25
			g.showOverlayMessage("Minimum zoom 25%")
		} else {
			g.zoomState.Level = newLevel
			g.showOverlayMessage(fmt.Sprintf("%.0f%%", g.zoomState.Level*100))
		}
	}
}

func (g *Game) zoomReset() {
	g.switchToManual100()
}

func (g *Game) zoomFit() {
	if g.zoomState.Mode == ZoomModeFit {
		// Currently in fit mode, switch to 100%
		g.switchToManual100()
	} else {
		// Switch to fit mode
		g.zoomState.Reset()
		g.showOverlayMessage("Fit to Window")
	}
}

// switchToManual100 sets zoom mode to Manual at 100% scale
func (g *Game) switchToManual100() {
	g.zoomState.Mode = ZoomModeManual
	g.zoomState.Level = 1.0
	g.zoomState.PanOffsetX = 0
	g.zoomState.PanOffsetY = 0
	g.showOverlayMessage("100%")
}

func (g *Game) panUp() {
	if g.zoomState.Mode == ZoomModeManual {
		_, stepY := g.getPanStep()
		g.zoomState.PanOffsetY += stepY
		g.clampPanToLimits()
	}
}

func (g *Game) panDown() {
	if g.zoomState.Mode == ZoomModeManual {
		_, stepY := g.getPanStep()
		g.zoomState.PanOffsetY -= stepY
		g.clampPanToLimits()
	}
}

func (g *Game) panLeft() {
	if g.zoomState.Mode == ZoomModeManual {
		stepX, _ := g.getPanStep()
		g.zoomState.PanOffsetX += stepX
		g.clampPanToLimits()
	}
}

func (g *Game) panRight() {
	if g.zoomState.Mode == ZoomModeManual {
		stepX, _ := g.getPanStep()
		g.zoomState.PanOffsetX -= stepX
		g.clampPanToLimits()
	}
}

func (g *Game) panByDelta(deltaX, deltaY float64) {
	if g.zoomState.Mode == ZoomModeManual {
		g.zoomState.PanOffsetX += deltaX
		g.zoomState.PanOffsetY += deltaY
		g.clampPanToLimits()
	}
}

// getPanStep calculates dynamic pan step size based on screen size and zoom level
func (g *Game) getPanStep() (float64, float64) {
	// Base step size as 10% of screen dimensions
	stepX := float64(g.savedWinW) * 0.1
	stepY := float64(g.savedWinH) * 0.1

	// Scale by zoom level for more consistent feel
	zoomFactor := g.zoomState.Level
	stepX *= zoomFactor
	stepY *= zoomFactor

	return stepX, stepY
}

// getTransformedImageSize calculates the size of the currently displayed image after transformations
func (g *Game) getTransformedImageSize() (int, int) {
	var w, h int

	if g.tempSingleMode || !g.bookMode {
		// Single Image Mode
		img := g.getCurrentImage()
		if img == nil {
			return 0, 0
		}
		w, h = img.Bounds().Dx(), img.Bounds().Dy()
	} else {
		// Book Mode
		leftImg, rightImg := g.getBookModeImages()
		if leftImg == nil {
			return 0, 0
		}

		if rightImg == nil {
			w, h = leftImg.Bounds().Dx(), leftImg.Bounds().Dy()
		} else {
			leftW, leftH := leftImg.Bounds().Dx(), leftImg.Bounds().Dy()
			rightW, rightH := rightImg.Bounds().Dx(), rightImg.Bounds().Dy()
			w = leftW + rightW + imageGap
			h = int(math.Max(float64(leftH), float64(rightH)))
		}
	}

	// Apply transformation size calculation (same as applyTransformations)
	if g.rotationAngle == 90 || g.rotationAngle == 270 {
		return h, w // 90°/270° rotation swaps width and height
	}
	return w, h
}

// clampPanToLimits ensures pan offsets stay within valid boundaries
func (g *Game) clampPanToLimits() {
	if g.zoomState.Mode != ZoomModeManual {
		return
	}

	iw, ih := g.getTransformedImageSize()
	if iw == 0 || ih == 0 {
		return
	}

	deviceScale := ebiten.Monitor().DeviceScaleFactor()
	w, h := float64(g.savedWinW)*deviceScale, float64(g.savedWinH)*deviceScale
	scale := g.zoomState.Level
	sw, sh := float64(iw)*scale, float64(ih)*scale

	// Calculate X boundaries
	if sw > w {
		// Image is wider than screen, apply pan limits
		maxPanX := sw/2 - w/2 // Right limit
		minPanX := w/2 - sw/2 // Left limit

		if g.zoomState.PanOffsetX > maxPanX {
			g.zoomState.PanOffsetX = maxPanX
		} else if g.zoomState.PanOffsetX < minPanX {
			g.zoomState.PanOffsetX = minPanX
		}
	} else {
		// Image is narrower than screen, center horizontally
		g.zoomState.PanOffsetX = 0
	}

	// Calculate Y boundaries
	if sh > h {
		// Image is taller than screen, apply pan limits
		maxPanY := sh/2 - h/2 // Bottom limit
		minPanY := h/2 - sh/2 // Top limit

		if g.zoomState.PanOffsetY > maxPanY {
			g.zoomState.PanOffsetY = maxPanY
		} else if g.zoomState.PanOffsetY < minPanY {
			g.zoomState.PanOffsetY = minPanY
		}
	} else {
		// Image is shorter than screen, center vertically
		g.zoomState.PanOffsetY = 0
	}
}

func (g *Game) shouldUseBookMode(leftImg, rightImg *ebiten.Image) bool {
	if leftImg == nil || rightImg == nil {
		return false // Can't do book mode with only one image
	}

	// Calculate aspect ratios
	leftAspect := float64(leftImg.Bounds().Dx()) / float64(leftImg.Bounds().Dy())
	rightAspect := float64(rightImg.Bounds().Dx()) / float64(rightImg.Bounds().Dy())

	// Check for extremely tall or wide images (should be single page)
	if leftAspect < minAspectRatio || leftAspect > maxAspectRatio ||
		rightAspect < minAspectRatio || rightAspect > maxAspectRatio {
		return false
	}

	// Calculate the ratio between the two aspect ratios
	aspectRatio := leftAspect / rightAspect
	if aspectRatio < 1.0 {
		aspectRatio = 1.0 / aspectRatio // Always use the larger ratio
	}

	// Use single page if aspect ratios are too different
	return aspectRatio <= g.config.AspectRatioThreshold
}

func (g *Game) showOverlayMessage(message string) {
	g.overlayMessage = message
	if message != "" {
		g.overlayMessageTime = time.Now()
	} else {
		g.overlayMessageTime = time.Time{} // Zero value for empty messages
	}
}

func (g *Game) toggleBookMode() {
	if g.tempSingleMode || g.bookMode {
		// Currently in temp single mode or book mode, switch to single mode
		g.bookMode = false
		g.tempSingleMode = false
	} else {
		// Currently in single mode, switch to book mode
		pathsCount := g.imageManager.GetPathsCount()
		if pathsCount == 1 {
			// Only one page, use temp single mode
			g.bookMode = true
			g.tempSingleMode = true
		} else if g.idx == pathsCount-1 {
			// On final page, check if it can be paired with previous page
			prevImg, finalImg := g.imageManager.GetBookModeImages(g.idx-1, g.config.RightToLeft)

			if g.shouldUseBookMode(prevImg, finalImg) {
				// Move to previous page to display final page in book mode
				g.idx--
				g.tempSingleMode = false
			} else {
				// Keep current position but use temp single mode
				g.tempSingleMode = true
			}
			g.bookMode = true
		} else {
			// Not on final page, normal book mode
			g.bookMode = true
			g.tempSingleMode = false
		}
	}

	// Save the book mode preference (true even if in temp single mode)
	g.config.BookMode = g.bookMode
}

func (g *Game) processPageInput() {
	if g.pageInputBuffer == "" {
		return
	}

	pageNum, err := strconv.Atoi(g.pageInputBuffer)
	if err != nil {
		g.showOverlayMessage("Invalid page number")
		return
	}

	g.jumpToPage(pageNum)
}

func (g *Game) jumpToPage(pageNum int) {
	pathsCount := g.imageManager.GetPathsCount()

	// 1-based -> 0-based conversion
	targetIdx := pageNum - 1

	// Range check
	if targetIdx < 0 || targetIdx >= pathsCount {
		g.showOverlayMessage(fmt.Sprintf("Page %d not found (1-%d)", pageNum, pathsCount))
		return
	}

	if g.bookMode && targetIdx == pathsCount-1 {
		// Special handling for jumping to the final page in book mode
		if targetIdx > 0 {
			// Check if final page can be paired with previous page
			prevImg, finalImg := g.imageManager.GetBookModeImages(targetIdx-1, g.config.RightToLeft)

			if g.shouldUseBookMode(prevImg, finalImg) {
				// Use book mode with previous page and final page
				g.idx = targetIdx - 1
				g.tempSingleMode = false
			} else {
				// Use temp single mode for final page only
				g.idx = targetIdx
				g.bookMode = false
				g.tempSingleMode = true
			}
		} else {
			// Only one page total, use temp single mode
			g.idx = targetIdx
			g.bookMode = false
			g.tempSingleMode = true
		}
	} else {
		// Normal jump logic - let regular book mode logic handle pairing
		g.idx = targetIdx
		g.tempSingleMode = false // Reset temp single mode
	}

	// Start preload after jump (both directions)
	g.imageManager.StartPreload(g.idx, NavigationJump)

	// Reset zoom state when image changes
	g.zoomState.Reset()
}

func (g *Game) expandToDirectoryAndJump() {
	// Only work if not already expanded and original file index is valid
	if g.expandedFromSingle || g.originalFileIndex < 0 || len(g.originalArgs) != 1 {
		return
	}

	originalFilePath := g.originalArgs[0]

	// Collect images from the same directory
	newPaths, err := collectImagesFromSameDirectory(originalFilePath, g.config.SortMethod)
	if err != nil {
		g.showOverlayMessage(fmt.Sprintf("Failed to scan directory: %v", err))
		return
	}

	if len(newPaths) == 0 {
		g.showOverlayMessage("No images found in directory")
		return
	}

	// Find the original file in the new list
	originalFileIndex := -1
	for i, imagePath := range newPaths {
		if imagePath.Path == originalFilePath {
			originalFileIndex = i
			break
		}
	}

	if originalFileIndex == -1 {
		g.showOverlayMessage("Original file not found in directory")
		return
	}

	// Update the image manager with new paths
	g.imageManager.SetPaths(newPaths)

	// Jump to the original file
	g.idx = originalFileIndex
	g.expandedFromSingle = true

	// Show success message
	g.showOverlayMessage(fmt.Sprintf("Loaded %d images from directory", len(newPaths)))
}

func (g *Game) getCurrentPageNumber() string {
	total := g.imageManager.GetPathsCount()
	if total == 0 {
		return "0 / 0"
	}

	if g.bookMode && !g.tempSingleMode {
		// In book mode, show range of pages
		leftPage := g.idx + 1
		rightPage := g.idx + 2
		if rightPage > total {
			rightPage = total
		}
		if leftPage == rightPage {
			return fmt.Sprintf("%d / %d", leftPage, total)
		}
		return fmt.Sprintf("%d-%d / %d", leftPage, rightPage, total)
	}

	// Single page mode or temp single mode
	return fmt.Sprintf("%d / %d", g.idx+1, total)
}

func (g *Game) saveCurrentWindowSize() {
	if g.fullscreen {
		// Save the size from before fullscreen
		if g.savedWinW > 0 && g.savedWinH > 0 {
			g.config.WindowWidth = g.savedWinW
			g.config.WindowHeight = g.savedWinH
		}
	} else {
		// Save current window size
		w, h := ebiten.WindowSize()
		g.config.WindowWidth = w
		g.config.WindowHeight = h
	}
}

func (g *Game) Exit() {
	// Save all current settings before exiting
	g.saveCurrentWindowSize()
	g.saveCurrentConfig()
	// Stop preload manager
	g.imageManager.StopPreload()
	os.Exit(0)
}

// RenderState interface implementation
func (g *Game) IsBookMode() bool {
	return g.bookMode
}

func (g *Game) IsTempSingleMode() bool {
	return g.tempSingleMode
}

func (g *Game) IsFullscreen() bool {
	return g.fullscreen
}

func (g *Game) GetCurrentImage() *ebiten.Image {
	return g.getCurrentImage()
}

func (g *Game) GetBookModeImages() (*ebiten.Image, *ebiten.Image) {
	return g.getBookModeImages()
}

func (g *Game) ShouldUseBookMode(left, right *ebiten.Image) bool {
	return g.shouldUseBookMode(left, right)
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

// GetZoomMode for InputState interface (drag permission checking)
func (g *Game) GetZoomMode() ZoomMode {
	return g.zoomState.Mode
}

func (g *Game) GetOverlayMessage() string {
	return g.overlayMessage
}

func (g *Game) GetOverlayMessageTime() time.Time {
	return g.overlayMessageTime
}

// Zoom and pan state methods for RenderState interface
func (g *Game) GetZoomLevel() float64 {
	return g.zoomState.Level
}

func (g *Game) GetPanOffsetX() float64 {
	return g.zoomState.PanOffsetX
}

func (g *Game) GetPanOffsetY() float64 {
	return g.zoomState.PanOffsetY
}

func (g *Game) GetCurrentPageNumber() string {
	return g.getCurrentPageNumber()
}

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

func (g *Game) GetMouseSettings() MouseSettings {
	return g.mousebindingManager.GetSettings()
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

func (g *Game) ToggleFullscreen() {
	g.toggleFullscreen()
}

func (g *Game) ResetWindowSize() {
	g.resetToDefaultWindowSize()
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

func (g *Game) NavigateNext() {
	g.navigateNext()
	g.imageManager.StartPreload(g.idx, NavigationForward)
}

func (g *Game) NavigatePrevious() {
	g.navigatePrevious()
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

func (g *Game) ShowOverlayMessage(message string) {
	g.showOverlayMessage(message)
}

// Zoom and pan actions for InputActions interface
func (g *Game) ZoomIn() {
	g.zoomIn()
}

func (g *Game) ZoomOut() {
	g.zoomOut()
}

func (g *Game) ZoomReset() {
	g.zoomReset()
}

func (g *Game) ZoomFit() {
	g.zoomFit()
}

func (g *Game) PanUp() {
	g.panUp()
}

func (g *Game) PanDown() {
	g.panDown()
}

func (g *Game) PanLeft() {
	g.panLeft()
}

func (g *Game) PanRight() {
	g.panRight()
}

func (g *Game) PanByDelta(deltaX, deltaY float64) {
	g.panByDelta(deltaX, deltaY)
}

func (g *Game) GetCurrentIndex() int {
	return g.idx
}

func (g *Game) Update() error {
	if g.wasInputHandled {
		debugLog("waiting for previous input to complete\n")
	} else {
		g.wasInputHandled = g.inputHandler.HandleInput()
	}

	// Clear expired overlay messages to avoid unnecessary redraws
	if g.overlayMessage != "" && time.Since(g.overlayMessageTime) >= overlayMessageDuration {
		g.overlayMessage = ""
		g.overlayMessageTime = time.Time{}
	}

	return nil
}

func (g *Game) navigateNext() {
	pathsCount := g.imageManager.GetPathsCount()

	// Common boundary check - cannot proceed to next
	if g.idx+1 >= pathsCount {
		g.showOverlayMessage("Last page")
		return
	}

	// From here on, g.idx + 1 < pathsCount is guaranteed, so g.idx++ is safe
	if g.tempSingleMode {
		g.idx++
		g.tempSingleMode = false
		g.bookMode = true
		g.zoomState.Reset()
		return
	}

	if g.bookMode && !ebiten.IsKeyPressed(ebiten.KeyShift) {
		// Check if we can actually display in book mode
		leftImg, rightImg := g.imageManager.GetBookModeImages(g.idx, g.config.RightToLeft)
		if g.shouldUseBookMode(leftImg, rightImg) {
			if g.idx+2 >= pathsCount {
				// Cannot advance 2 pages = all displayed with current pair
				g.showOverlayMessage("Last page")
			} else if g.idx+2+1 >= pathsCount {
				// Advancing 2 pages would make next pair impossible (=becomes last single page)
				g.idx += 2
				g.bookMode = false
				g.tempSingleMode = true
			} else {
				// Normal 2-page movement
				g.idx += 2
			}
			g.zoomState.Reset()
			return
		}
		// shouldUseBookMode = false means single page movement
	}
	// Single page mode or Shift+key
	g.idx++

	// Reset zoom state when image changes
	g.zoomState.Reset()
}

func (g *Game) navigatePrevious() {
	// Common boundary check - cannot go back
	if g.idx <= 0 {
		g.showOverlayMessage("First page")
		return
	}

	// From here on, g.idx > 0 is guaranteed, so some backward processing is possible
	if g.tempSingleMode {
		if g.idx < 2 {
			// g.idx > 0 is guaranteed, so always move to g.idx = 0
			g.idx = 0
			g.tempSingleMode = false
			g.bookMode = true
		} else {
			g.idx -= 2
			g.tempSingleMode = false
			g.bookMode = true
		}
		g.zoomState.Reset()
		return
	}

	if g.bookMode && !ebiten.IsKeyPressed(ebiten.KeyShift) {
		leftImg, rightImg := g.imageManager.GetBookModeImages(g.idx, g.config.RightToLeft)
		if g.shouldUseBookMode(leftImg, rightImg) {
			if g.idx < 2 {
				// g.idx > 0 is guaranteed, so always move to g.idx = 0
				g.idx = 0
				g.bookMode = false
				g.tempSingleMode = true
			} else {
				g.idx -= 2
			}
			g.zoomState.Reset()
			return
		}
		// shouldUseBookMode = false means single page movement
	}
	// Single page mode or Shift+key
	g.idx--

	// Reset zoom state when image changes
	g.zoomState.Reset()
}

func (g *Game) toggleFullscreen() {
	g.fullscreen = !g.fullscreen
	if g.fullscreen {
		g.savedWinW, g.savedWinH = ebiten.WindowSize()
		ebiten.SetFullscreen(true)
	} else {
		ebiten.SetFullscreen(false)
		if g.savedWinW > 0 && g.savedWinH > 0 {
			ebiten.SetWindowSize(g.savedWinW, g.savedWinH)
		}
	}

	// Save fullscreen state to config
	g.config.Fullscreen = g.fullscreen

	// Force redraw for multiple frames to handle slow fullscreen transitions
	if g.config.TransitionFrames > 0 {
		g.forceRedrawFrames = g.config.TransitionFrames
	}
}

func (g *Game) resetToDefaultWindowSize() {
	currentWidth, currentHeight := ebiten.WindowSize()
	defaultWidth := g.config.DefaultWindowWidth
	defaultHeight := g.config.DefaultWindowHeight

	// Check if current size is already the default size
	if !g.fullscreen && currentWidth == defaultWidth && currentHeight == defaultHeight {
		g.showOverlayMessage("Already at default window size")
		return
	}

	// If in fullscreen, exit fullscreen first
	if g.fullscreen {
		g.fullscreen = false
		ebiten.SetFullscreen(false)
		g.config.Fullscreen = false
		g.showOverlayMessage(fmt.Sprintf("Windowed mode: %dx%d (default)", defaultWidth, defaultHeight))
	} else {
		g.showOverlayMessage(fmt.Sprintf("Window size: %dx%d (default)", defaultWidth, defaultHeight))
	}

	// Set window to default size
	ebiten.SetWindowSize(defaultWidth, defaultHeight)

	// Save the new size
	g.savedWinW = defaultWidth
	g.savedWinH = defaultHeight

	// Force redraw for multiple frames to handle window size transitions
	if g.config.TransitionFrames > 0 {
		g.forceRedrawFrames = g.config.TransitionFrames
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Get current window size
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Create lightweight snapshot of current render state
	currentSnapshot := NewRenderStateSnapshot(g, w, h)

	// Check if we need to redraw: input was handled, state changed, or force flag
	if g.wasInputHandled ||
		g.renderer.lastSnapshot == nil ||
		!currentSnapshot.Equals(g.renderer.lastSnapshot) ||
		g.forceRedrawFrames > 0 {

		// State has changed, perform actual drawing
		g.renderer.Draw(screen)

		// Save current snapshot for next frame
		g.renderer.lastSnapshot = currentSnapshot

		// Clear flags after drawing
		if g.forceRedrawFrames > 0 {
			g.forceRedrawFrames--
		}
		g.wasInputHandled = false
	}
	// If state hasn't changed and no input, skip drawing entirely
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	// Only force redraw when layout actually changes
	if g.savedWinW != outsideWidth || g.savedWinH != outsideHeight {
		// Don't update saved window size during fullscreen
		if !g.fullscreen {
			g.savedWinW = outsideWidth
			g.savedWinH = outsideHeight
			g.forceRedrawFrames = 1
		}
	}

	// Hi-DPI support: multiply by device scale factor for sharper rendering
	scale := ebiten.Monitor().DeviceScaleFactor()
	return int(float64(outsideWidth) * scale), int(float64(outsideHeight) * scale)
}

// getWindowTitle returns the window title with version information
func getWindowTitle() string {
	if version == "dev" {
		return "Nekomimist's Image Viewer (dev)"
	}
	return fmt.Sprintf("Nekomimist's Image Viewer v%s", version)
}

// debugLog outputs debug messages only when debug mode is enabled
func debugLog(format string, args ...interface{}) {
	if debugMode {
		log.Printf(format, args...)
	}
}

func main() {
	var configFile = flag.String("c", "", "config file path (default: ~/.nv.json)")
	var debug = flag.Bool("d", false, "enable debug logging")
	var showVersion = flag.Bool("version", false, "show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("nv version v%s (built on %s)\n", version, buildDate)
		os.Exit(0)
	}

	debugMode = *debug

	args := flag.Args()

	var configResult ConfigLoadResult
	if *configFile != "" {
		configResult = loadConfigFromPath(*configFile)
	} else {
		configResult = loadConfig()
	}
	config := configResult.Config

	// Check if launched with single image file
	isSingleImageFile := len(args) == 1 && isSupportedExt(args[0]) && !isArchiveExt(args[0])

	paths, err := collectImages(args, config.SortMethod)
	if err != nil {
		log.Fatal(err)
	}
	if len(paths) == 0 {
		log.Fatal("no image files specified")
	}

	imageManager := NewImageManagerWithPreload(config.CacheSize, config.PreloadCount, config.PreloadEnabled)
	imageManager.SetPaths(paths)

	g := &Game{
		imageManager:       imageManager,
		idx:                0,
		bookMode:           config.BookMode,
		fullscreen:         config.Fullscreen,
		config:             config,
		configPath:         *configFile,
		showInfo:           false, // Hide info display by default
		originalArgs:       args,
		expandedFromSingle: false,
		originalFileIndex:  -1,
		configStatus:       configResult,
		zoomState:          NewZoomState(),
	}

	// Apply initial zoom mode from config
	if config.InitialZoomMode == "actual_size" {
		g.zoomState.Mode = ZoomModeManual
		g.zoomState.Level = 1.0
		g.zoomState.PanOffsetX = 0
		g.zoomState.PanOffsetY = 0
	}

	// Start initial preload in forward direction
	imageManager.StartPreload(0, NavigationForward)

	// Initialize input handler and renderer
	keybindingManager := NewKeybindingManager(config.Keybindings)
	g.keybindingManager = keybindingManager

	mousebindingManager := NewMousebindingManager(config.Mousebindings, config.MouseSettings)
	g.mousebindingManager = mousebindingManager
	g.inputHandler = NewInputHandler(g, g, keybindingManager, mousebindingManager)
	g.renderer = NewRenderer(g)

	// Show config warnings if any
	if configResult.Status == "Warning" || configResult.Status == "Error" {
		if len(configResult.Warnings) > 0 {
			warningMsg := fmt.Sprintf("Config %s: %s", configResult.Status, configResult.Warnings[0])
			g.showOverlayMessage(warningMsg)
		} else {
			g.showOverlayMessage(fmt.Sprintf("Config %s: Using defaults", configResult.Status))
		}
	}

	// Set up single file expansion mode if applicable
	if isSingleImageFile {
		g.originalFileIndex = 0 // The single file is at index 0
	}

	// Handle book mode initialization for single image or incompatible images
	if config.BookMode && len(paths) > 0 {
		pathsCount := len(paths)
		if pathsCount == 1 {
			// Only one image, use temp single mode
			g.tempSingleMode = true
		} else {
			// Check if current images are compatible for book mode
			leftImg, rightImg := g.imageManager.GetBookModeImages(0, g.config.RightToLeft)
			if !g.shouldUseBookMode(leftImg, rightImg) {
				g.tempSingleMode = true
			}
		}
	}

	ebiten.SetWindowTitle(getWindowTitle())
	ebiten.SetWindowSize(config.WindowWidth, config.WindowHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	// Enable rendering optimization by preserving screen content between frames
	ebiten.SetScreenClearedEveryFrame(false)

	// Set window icon
	setWindowIcon()

	// Apply saved fullscreen setting
	if config.Fullscreen {
		g.savedWinW, g.savedWinH = config.WindowWidth, config.WindowHeight
		ebiten.SetFullscreen(true)
	}

	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}

func setWindowIcon() {
	// Load embedded icon images
	iconData := [][]byte{icon16, icon32, icon48}
	var iconImages []image.Image

	for _, data := range iconData {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			continue
		}
		iconImages = append(iconImages, img)
	}

	// Set the window icon if we have at least one image
	if len(iconImages) > 0 {
		ebiten.SetWindowIcon(iconImages)
	}
}
