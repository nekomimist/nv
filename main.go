package main

import (
	"bytes"
	"flag"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/goregular"
	"time"
)

const (
	// Book mode layout constants
	imageGap = 10 // Gap between images in book mode

	// Aspect ratio thresholds
	minAspectRatio = 0.4 // Extremely tall images
	maxAspectRatio = 2.5 // Extremely wide images

	// Boundary message display duration
	boundaryMessageDuration = 2 * time.Second
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
	imageManager   ImageManager
	idx            int
	fullscreen     bool
	bookMode       bool // Book/spread view mode
	tempSingleMode bool // Temporary single page mode (return to book mode after navigation)
	showHelp       bool // Help overlay display

	// Page input mode state
	pageInputMode   bool
	pageInputBuffer string

	// Boundary message state
	boundaryMessage     string
	boundaryMessageTime time.Time

	// Sort method message state
	sortMessage     string
	sortMessageTime time.Time

	savedWinW  int
	savedWinH  int
	config     Config
	configPath string // Custom config file path, empty for default
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

func (g *Game) cycleSortMethod() {
	// Cycle through sort methods
	g.config.SortMethod = (g.config.SortMethod + 1) % 3

	// Save config
	g.saveCurrentConfig()

	// Show message
	g.sortMessage = "Sort: " + getSortMethodName(g.config.SortMethod)
	g.sortMessageTime = time.Now()

	// Re-collect and sort images
	args := flag.Args()
	if len(args) > 0 {
		paths, err := collectImages(args, g.config.SortMethod)
		if err == nil && len(paths) > 0 {
			g.imageManager.SetPaths(paths)
			// Reset to first image
			g.idx = 0
			g.imageManager.PreloadAdjacentImages(0)
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

func (g *Game) showBoundaryMessage(message string) {
	g.boundaryMessage = message
	g.boundaryMessageTime = time.Now()
}

func (g *Game) processPageInput() {
	if g.pageInputBuffer == "" {
		return
	}

	pageNum, err := strconv.Atoi(g.pageInputBuffer)
	if err != nil {
		g.showBoundaryMessage("Invalid page number")
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
		g.showBoundaryMessage(fmt.Sprintf("Page %d not found (1-%d)", pageNum, pathsCount))
		return
	}

	if g.bookMode && targetIdx == pathsCount-1 {
		// Special handling for jumping to the final page in book mode
		if targetIdx > 0 {
			// Check if final page can be paired with previous page
			prevImg := g.imageManager.GetCurrentImage(targetIdx - 1)
			finalImg := g.imageManager.GetCurrentImage(targetIdx)

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

	g.imageManager.PreloadAdjacentImages(g.idx)
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
	g.saveCurrentConfig()
}

func (g *Game) Update() error {
	if g.imageManager.GetPathsCount() == 0 {
		return nil
	}

	g.handleExitKeys()
	g.handleHelpToggle()
	g.handlePageInputMode()
	g.handleModeToggleKeys()
	g.handleNavigationKeys()
	g.handleFullscreenToggle()

	return nil
}

func (g *Game) handleExitKeys() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		g.saveCurrentWindowSize()
		os.Exit(0)
	}
}

func (g *Game) handleHelpToggle() {
	if inpututil.IsKeyJustPressed(ebiten.KeyH) {
		g.showHelp = !g.showHelp
	}
}

func (g *Game) handlePageInputMode() {
	// Check for G key to enter page input mode
	if !g.pageInputMode {
		if inpututil.IsKeyJustPressed(ebiten.KeyG) {
			g.pageInputMode = true
			g.pageInputBuffer = ""
		}
		return
	}

	// Handle page input mode
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		// Cancel page input
		g.pageInputMode = false
		g.pageInputBuffer = ""
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
		// Confirm page input
		g.processPageInput()
		g.pageInputMode = false
		g.pageInputBuffer = ""
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		// Delete last character
		if len(g.pageInputBuffer) > 0 {
			g.pageInputBuffer = g.pageInputBuffer[:len(g.pageInputBuffer)-1]
		}
		return
	}

	// Handle digit input (both regular and numpad)
	var digit string
	if digit = checkDigitKeys(ebiten.Key0, ebiten.Key9, '0'); digit == "" {
		digit = checkDigitKeys(ebiten.KeyNumpad0, ebiten.KeyNumpad9, '0')
	}
	if digit != "" {
		g.pageInputBuffer += digit
	}
}

func checkDigitKeys(startKey, endKey ebiten.Key, baseChar rune) string {
	for key := startKey; key <= endKey; key++ {
		if inpututil.IsKeyJustPressed(key) {
			return string(baseChar + rune(key-startKey))
		}
	}
	return ""
}

func (g *Game) handleModeToggleKeys() {
	if inpututil.IsKeyJustPressed(ebiten.KeyB) {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			// SHIFT+B: Toggle reading direction
			g.config.RightToLeft = !g.config.RightToLeft
			g.saveCurrentConfig()
		} else {
			// B: Toggle book mode
			g.bookMode = !g.bookMode
			g.imageManager.PreloadAdjacentImages(g.idx)
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			// SHIFT+S: Cycle sort method
			g.cycleSortMethod()
		}
	}
}

func (g *Game) handleNavigationKeys() {
	// Next page
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyN) {
		g.navigateNext()
		g.imageManager.PreloadAdjacentImages(g.idx)
	}
	// Previous page
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || inpututil.IsKeyJustPressed(ebiten.KeyP) {
		g.navigatePrevious()
		g.imageManager.PreloadAdjacentImages(g.idx)
	}
	// Jump to first page
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) || inpututil.IsKeyJustPressed(ebiten.KeyComma) {
		g.jumpToPage(1)
	}
	// Jump to last page
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) || inpututil.IsKeyJustPressed(ebiten.KeyPeriod) {
		totalPages := g.imageManager.GetPathsCount()
		if totalPages > 0 {
			g.jumpToPage(totalPages)
		}
	}
}

func (g *Game) navigateNext() {
	pathsCount := g.imageManager.GetPathsCount()

	// Common boundary check - cannot proceed to next
	if g.idx+1 >= pathsCount {
		g.showBoundaryMessage("Last page")
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
				g.showBoundaryMessage("Last page")
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
		g.showBoundaryMessage("First page")
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

func (g *Game) handleFullscreenToggle() {
	if inpututil.IsKeyJustPressed(ebiten.KeyZ) {
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
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	if g.tempSingleMode || !g.bookMode {
		// Single page mode or temporary single mode
		g.drawSingleImage(screen)
	} else {
		// Book mode
		g.drawBookMode(screen)
	}

	// Draw page status at bottom of screen
	g.drawPageStatus(screen)

	// Draw help overlay if enabled
	if g.showHelp {
		g.drawHelpOverlay(screen)
	}

	// Draw page input overlay if active
	if g.pageInputMode {
		g.drawPageInputOverlay(screen)
	}

	// Draw boundary message overlay if active
	if g.boundaryMessage != "" && time.Since(g.boundaryMessageTime) < boundaryMessageDuration {
		g.drawBoundaryMessageOverlay(screen)
	}

	// Draw sort method message overlay if active
	if g.sortMessage != "" && time.Since(g.sortMessageTime) < boundaryMessageDuration {
		g.drawSortMessageOverlay(screen)
	}
}

func (g *Game) drawSingleImage(screen *ebiten.Image) {
	img := g.getCurrentImage()
	if img == nil {
		return
	}

	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	var scale float64
	if g.fullscreen {
		scale = math.Min(float64(w)/float64(iw), float64(h)/float64(ih))
	} else {
		if iw > w || ih > h {
			scale = math.Min(float64(w)/float64(iw), float64(h)/float64(ih))
		} else {
			scale = 1
		}
	}

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	sw, sh := float64(iw)*scale, float64(ih)*scale
	op.GeoM.Translate(float64(w)/2-sw/2, float64(h)/2-sh/2)

	screen.DrawImage(img, op)
}

func (g *Game) drawBookMode(screen *ebiten.Image) {
	leftImg, rightImg := g.getBookModeImages()
	if leftImg == nil {
		return
	}

	// Check if images are compatible for book mode display
	if !g.shouldUseBookMode(leftImg, rightImg) {
		// Fall back to single page display
		g.drawSingleImage(screen)
		return
	}

	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Calculate available width for each image
	availableWidth := (w - imageGap) / 2

	// Draw left image (right-aligned within its region)
	g.drawBookImageInRegion(screen, leftImg, 0, 0, availableWidth, h, "right")

	// Draw right image if exists (left-aligned within its region)
	if rightImg != nil {
		rightX := availableWidth + imageGap
		g.drawBookImageInRegion(screen, rightImg, rightX, 0, availableWidth, h, "left")
	}
}

func (g *Game) drawImageInRegion(screen *ebiten.Image, img *ebiten.Image, x, y, maxW, maxH int) {
	g.drawImageInRegionWithAlign(screen, img, x, y, maxW, maxH, "center")
}

func (g *Game) drawBookImageInRegion(screen *ebiten.Image, img *ebiten.Image, x, y, maxW, maxH int, align string) {
	g.drawImageInRegionWithAlign(screen, img, x, y, maxW, maxH, align)
}

func (g *Game) drawImageInRegionWithAlign(screen *ebiten.Image, img *ebiten.Image, x, y, maxW, maxH int, align string) {
	// Calculate scaling
	scale := g.calculateImageScale(img, maxW, maxH)

	// Create draw options
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)

	// Calculate position based on alignment
	scaledW := float64(img.Bounds().Dx()) * scale
	scaledH := float64(img.Bounds().Dy()) * scale

	xPos := g.calculateHorizontalPosition(x, maxW, scaledW, align)
	yPos := float64(y) + float64(maxH)/2 - scaledH/2 // Always center vertically

	op.GeoM.Translate(xPos, yPos)
	screen.DrawImage(img, op)
}

func (g *Game) calculateImageScale(img *ebiten.Image, maxW, maxH int) float64 {
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()

	if g.fullscreen {
		return math.Min(float64(maxW)/float64(iw), float64(maxH)/float64(ih))
	}

	// In windowed mode, don't scale up small images
	if iw > maxW || ih > maxH {
		return math.Min(float64(maxW)/float64(iw), float64(maxH)/float64(ih))
	}
	return 1
}

func (g *Game) calculateHorizontalPosition(x, maxW int, scaledW float64, align string) float64 {
	switch align {
	case "left":
		return float64(x)
	case "right":
		return float64(x+maxW) - scaledW
	default: // "center"
		return float64(x) + float64(maxW)/2 - scaledW/2
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

// Help text data - keys and descriptions separated for better alignment
var helpSections = []struct {
	title string
	items []struct {
		key  string
		desc string
	}
}{
	{
		title: "Navigation:",
		items: []struct {
			key  string
			desc string
		}{
			{"Space/N", "Next image (2 images in book mode)"},
			{"Backspace/P", "Previous image (2 images in book mode)"},
			{"Shift+Space/N", "Single page forward (fine adjustment)"},
			{"Shift+Backspace/P", "Single page backward (fine adjustment)"},
			{"Home/<", "Jump to first page"},
			{"End/>", "Jump to last page"},
		},
	},
	{
		title: "Display Modes:",
		items: []struct {
			key  string
			desc string
		}{
			{"B", "Toggle book mode (dual image view)"},
			{"Shift+B", "Toggle reading direction (LTR ↔ RTL)"},
			{"Shift+S", "Cycle sort method (Natural/Simple/Entry)"},
			{"Z", "Toggle fullscreen"},
		},
	},
	{
		title: "Other:",
		items: []struct {
			key  string
			desc string
		}{
			{"G", "Go to page (enter page number)"},
			{"H", "Show/hide this help"},
			{"Escape/Q", "Quit application"},
		},
	},
}

var (
	helpFontSource *text.GoTextFaceSource
)

func init() {
	// Initialize font source with lightweight goregular
	s, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		log.Fatal(err)
	}
	helpFontSource = s
}

func (g *Game) drawHelpOverlay(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Semi-transparent black background (lighter for more image transparency)
	vector.DrawFilledRect(screen, 0, 0, float32(w), float32(h), color.RGBA{0, 0, 0, 128}, false)

	// Help text area with semi-transparent black background
	padding := 40
	textAreaX := float32(padding)
	textAreaY := float32(padding)
	textAreaW := float32(w - padding*2)
	textAreaH := float32(h - padding*2)

	// Semi-transparent black background for text area
	vector.DrawFilledRect(screen, textAreaX, textAreaY, textAreaW, textAreaH, color.RGBA{0, 0, 0, 160}, false)

	// Create font with size from config
	helpFont := &text.GoTextFace{
		Source: helpFontSource,
		Size:   g.config.HelpFontSize,
	}

	// Draw title
	titleY := float64(padding + 30)
	titleOp := &text.DrawOptions{}
	titleOp.GeoM.Translate(float64(padding+20), titleY)
	titleOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255})
	text.Draw(screen, "CONTROLS:", helpFont, titleOp)

	// Calculate column positions
	keyColumnX := float64(padding + 220)  // Key column (right-aligned)
	descColumnX := float64(padding + 270) // Description column (left-aligned)

	currentY := titleY + g.config.HelpFontSize*2 // Start below title
	lineHeight := g.config.HelpFontSize * 1.5

	// Draw each section
	for _, section := range helpSections {
		// Draw section title
		sectionOp := &text.DrawOptions{}
		sectionOp.GeoM.Translate(float64(padding+20), currentY)
		sectionOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255})
		text.Draw(screen, section.title, helpFont, sectionOp)
		currentY += lineHeight

		// Draw each key-description pair
		for _, item := range section.items {
			// Draw key (right-aligned)
			keyOp := &text.DrawOptions{}
			keyOp.GeoM.Translate(keyColumnX, currentY)
			keyOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255})
			keyOp.PrimaryAlign = text.AlignEnd // Right align keys
			text.Draw(screen, item.key, helpFont, keyOp)

			// Draw description (left-aligned)
			descOp := &text.DrawOptions{}
			descOp.GeoM.Translate(descColumnX, currentY)
			descOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255})
			descOp.PrimaryAlign = text.AlignStart // Left align descriptions
			text.Draw(screen, item.desc, helpFont, descOp)

			currentY += lineHeight
		}

		// Add extra space between sections
		currentY += lineHeight * 0.5
	}
}

func (g *Game) drawPageInputOverlay(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Create font for page input
	inputFont := &text.GoTextFace{
		Source: helpFontSource,
		Size:   g.config.HelpFontSize,
	}

	// Create smaller font for range display
	rangeFont := &text.GoTextFace{
		Source: helpFontSource,
		Size:   g.config.HelpFontSize * 0.8,
	}

	// Get total pages for range display
	totalPages := g.imageManager.GetPathsCount()

	// Create display texts
	inputText := fmt.Sprintf("Go to page: %s_", g.pageInputBuffer)
	rangeText := fmt.Sprintf("(1-%d)", totalPages)

	// Measure text dimensions
	inputWidth, inputHeight := text.Measure(inputText, inputFont, 0)
	rangeWidth, rangeHeight := text.Measure(rangeText, rangeFont, 0)

	// Calculate box dimensions (accommodate both lines)
	maxWidth := math.Max(inputWidth, rangeWidth)
	totalHeight := inputHeight + rangeHeight + 10 // 10px gap between lines

	padding := 20
	boxWidth := maxWidth + float64(padding*2)
	boxHeight := totalHeight + float64(padding*2)
	boxX := (float64(w) - boxWidth) / 2
	boxY := (float64(h) - boxHeight) / 2

	// Semi-transparent black background
	vector.DrawFilledRect(screen, float32(boxX), float32(boxY), float32(boxWidth), float32(boxHeight), color.RGBA{0, 0, 0, 200}, false)

	// Draw input text (centered)
	inputTextOp := &text.DrawOptions{}
	inputTextX := boxX + (boxWidth-inputWidth)/2
	inputTextOp.GeoM.Translate(inputTextX, boxY+float64(padding))
	inputTextOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255})
	text.Draw(screen, inputText, inputFont, inputTextOp)

	// Draw range text (centered, below input text)
	rangeTextOp := &text.DrawOptions{}
	rangeTextX := boxX + (boxWidth-rangeWidth)/2
	rangeTextOp.GeoM.Translate(rangeTextX, boxY+float64(padding)+inputHeight+10)
	rangeTextOp.ColorScale.ScaleWithColor(color.RGBA{192, 192, 192, 255}) // Slightly dimmed
	text.Draw(screen, rangeText, rangeFont, rangeTextOp)
}

func (g *Game) drawPageStatus(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Create smaller font for page status
	statusFont := &text.GoTextFace{
		Source: helpFontSource,
		Size:   g.config.HelpFontSize * 0.7, // Smaller than help text
	}

	// Get page status text
	statusText := g.getCurrentPageNumber()

	// Measure text dimensions
	textWidth, textHeight := text.Measure(statusText, statusFont, 0)

	// Position at bottom right corner
	padding := 10
	textX := float64(w) - textWidth - float64(padding)
	textY := float64(h) - float64(padding)

	// Semi-transparent background
	bgPadding := 5
	bgX := textX - float64(bgPadding)
	bgY := textY - textHeight - float64(bgPadding)
	bgW := textWidth + float64(bgPadding*2)
	bgH := textHeight + float64(bgPadding*2)

	vector.DrawFilledRect(screen, float32(bgX), float32(bgY), float32(bgW), float32(bgH), color.RGBA{0, 0, 0, 128}, false)

	// Draw text
	textOp := &text.DrawOptions{}
	textOp.GeoM.Translate(textX, textY)
	textOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255})
	text.Draw(screen, statusText, statusFont, textOp)
}

func (g *Game) drawBoundaryMessageOverlay(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Create font for boundary message
	messageFont := &text.GoTextFace{
		Source: helpFontSource,
		Size:   g.config.HelpFontSize,
	}

	// Measure text dimensions
	textWidth, textHeight := text.Measure(g.boundaryMessage, messageFont, 0)

	// Calculate position (center of screen)
	padding := 20
	boxWidth := textWidth + float64(padding*2)
	boxHeight := textHeight + float64(padding*2)
	boxX := (float64(w) - boxWidth) / 2
	boxY := (float64(h) - boxHeight) / 2

	// Semi-transparent black background
	vector.DrawFilledRect(screen, float32(boxX), float32(boxY), float32(boxWidth), float32(boxHeight), color.RGBA{0, 0, 0, 200}, false)

	// Draw text
	textOp := &text.DrawOptions{}
	textOp.GeoM.Translate(boxX+float64(padding), boxY+float64(padding))
	textOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255})
	text.Draw(screen, g.boundaryMessage, messageFont, textOp)
}

func (g *Game) drawSortMessageOverlay(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Create font for sort message
	messageFont := &text.GoTextFace{
		Source: helpFontSource,
		Size:   g.config.HelpFontSize,
	}

	// Measure text dimensions
	textWidth, textHeight := text.Measure(g.sortMessage, messageFont, 0)

	// Calculate position (center of screen)
	padding := 20
	boxWidth := textWidth + float64(padding*2)
	boxHeight := textHeight + float64(padding*2)
	boxX := (float64(w) - boxWidth) / 2
	boxY := (float64(h) - boxHeight) / 2

	// Semi-transparent black background
	vector.DrawFilledRect(screen, float32(boxX), float32(boxY), float32(boxWidth), float32(boxHeight), color.RGBA{0, 0, 0, 200}, false)

	// Draw text
	textOp := &text.DrawOptions{}
	textOp.GeoM.Translate(boxX+float64(padding), boxY+float64(padding))
	textOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255})
	text.Draw(screen, g.sortMessage, messageFont, textOp)
}

func main() {
	var configFile = flag.String("c", "", "config file path (default: ~/.nv.json)")
	flag.Parse()

	var config Config
	if *configFile != "" {
		config = loadConfigFromPath(*configFile)
	} else {
		config = loadConfig()
	}
	paths, err := collectImages(flag.Args(), config.SortMethod)
	if err != nil {
		log.Fatal(err)
	}
	if len(paths) == 0 {
		log.Fatal("no image files specified")
	}

	imageManager := NewImageManager()
	imageManager.SetPaths(paths)

	g := &Game{
		imageManager: imageManager,
		idx:          0,
		config:       config,
		configPath:   *configFile,
	}

	// Preload the first image and adjacent ones for faster startup
	g.imageManager.PreloadAdjacentImages(0)

	ebiten.SetWindowTitle("Ebiten Image Viewer")
	ebiten.SetWindowSize(config.WindowWidth, config.WindowHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
