package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
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

func (g *Game) GetOverlayMessage() string {
	return g.overlayMessage
}

func (g *Game) GetOverlayMessageTime() time.Time {
	return g.overlayMessageTime
}

func (g *Game) GetCurrentPageNumber() string {
	return g.getCurrentPageNumber()
}

func (g *Game) GetTotalPagesCount() int {
	return g.imageManager.GetPathsCount()
}

func (g *Game) GetHelpFontSize() float64 {
	return g.config.HelpFontSize
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
			return
		}
		// shouldUseBookMode = false means single page movement
	}
	// Single page mode or Shift+key
	g.idx++
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
			return
		}
		// shouldUseBookMode = false means single page movement
	}
	// Single page mode or Shift+key
	g.idx--
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
	scale := ebiten.DeviceScaleFactor()
	return int(float64(outsideWidth) * scale), int(float64(outsideHeight) * scale)
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
	flag.Parse()

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

	ebiten.SetWindowTitle("Ebiten Image Viewer")
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
