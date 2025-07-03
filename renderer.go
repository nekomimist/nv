package main

import (
	"bytes"
	"fmt"
	"image/color"
	"log"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/goregular"
)

// Renderer handles all drawing operations
type Renderer struct {
	game           *Game
	helpFontSource *text.GoTextFaceSource
	helpSections   []helpSection
}

type helpSection struct {
	title string
	items []helpItem
}

type helpItem struct {
	key  string
	desc string
}

// NewRenderer creates a new Renderer
func NewRenderer(game *Game) *Renderer {
	// Initialize font source with lightweight goregular
	s, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		log.Fatal(err)
	}

	helpSections := []helpSection{
		{
			title: "Navigation:",
			items: []helpItem{
				{"Space/N", "Next image (2 images in book mode)"},
				{"Backspace/P", "Previous image (2 images in book mode)"},
				{"Shift+Space/N", "Single page forward (fine adjustment)"},
				{"Shift+Backspace/P", "Single page backward (fine adjustment)"},
				{"Home/<", "Jump to first page"},
				{"End/>", "Jump to last page"},
				{"L", "Load all images from directory (single file mode only)"},
			},
		},
		{
			title: "Display Modes:",
			items: []helpItem{
				{"B", "Toggle book mode (dual image view)"},
				{"Shift+B", "Toggle reading direction (LTR ↔ RTL)"},
				{"Shift+S", "Cycle sort method (Natural/Simple/Entry)"},
				{"Z", "Toggle fullscreen"},
			},
		},
		{
			title: "Other:",
			items: []helpItem{
				{"G", "Go to page (enter page number)"},
				{"H", "Show/hide this help"},
				{"I", "Show/hide info display (page numbers)"},
				{"Escape/Q", "Quit application"},
			},
		},
	}

	return &Renderer{
		game:           game,
		helpFontSource: s,
		helpSections:   helpSections,
	}
}

// Draw renders the entire screen
func (r *Renderer) Draw(screen *ebiten.Image) {
	if r.game.tempSingleMode || !r.game.bookMode {
		// Single page mode or temporary single mode
		r.drawSingleImage(screen)
	} else {
		// Book mode
		r.drawBookMode(screen)
	}

	// Draw info display (page status, etc.) at bottom of screen if enabled
	if r.game.showInfo {
		r.drawInfoDisplay(screen)
	}

	// Draw help overlay if enabled
	if r.game.showHelp {
		r.drawHelpOverlay(screen)
	}

	// Draw page input overlay if active
	if r.game.pageInputMode {
		r.drawPageInputOverlay(screen)
	}

	// Draw overlay message if active
	if r.game.overlayMessage != "" && time.Since(r.game.overlayMessageTime) < overlayMessageDuration {
		r.drawOverlayMessage(screen)
	}
}

func (r *Renderer) drawSingleImage(screen *ebiten.Image) {
	img := r.game.getCurrentImage()
	if img == nil {
		return
	}

	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	var scale float64
	if r.game.fullscreen {
		scale = math.Min(float64(w)/float64(iw), float64(h)/float64(ih))
	} else {
		if iw > w || ih > h {
			scale = math.Min(float64(w)/float64(iw), float64(h)/float64(ih))
		} else {
			scale = 1
		}
	}

	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterLinear
	op.GeoM.Scale(scale, scale)
	sw, sh := float64(iw)*scale, float64(ih)*scale
	op.GeoM.Translate(float64(w)/2-sw/2, float64(h)/2-sh/2)

	screen.DrawImage(img, op)
}

func (r *Renderer) drawBookMode(screen *ebiten.Image) {
	leftImg, rightImg := r.game.getBookModeImages()
	if leftImg == nil {
		return
	}

	// Check if images are compatible for book mode display
	if !r.game.shouldUseBookMode(leftImg, rightImg) {
		// Fall back to single page display
		r.drawSingleImage(screen)
		return
	}

	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Calculate available width for each image
	availableWidth := (w - imageGap) / 2

	// Draw left image (right-aligned within its region)
	r.drawBookImageInRegion(screen, leftImg, 0, 0, availableWidth, h, "right")

	// Draw right image if exists (left-aligned within its region)
	if rightImg != nil {
		rightX := availableWidth + imageGap
		r.drawBookImageInRegion(screen, rightImg, rightX, 0, availableWidth, h, "left")
	}
}

func (r *Renderer) drawImageInRegion(screen *ebiten.Image, img *ebiten.Image, x, y, maxW, maxH int) {
	r.drawImageInRegionWithAlign(screen, img, x, y, maxW, maxH, "center")
}

func (r *Renderer) drawBookImageInRegion(screen *ebiten.Image, img *ebiten.Image, x, y, maxW, maxH int, align string) {
	r.drawImageInRegionWithAlign(screen, img, x, y, maxW, maxH, align)
}

func (r *Renderer) drawImageInRegionWithAlign(screen *ebiten.Image, img *ebiten.Image, x, y, maxW, maxH int, align string) {
	// Calculate scaling
	scale := r.calculateImageScale(img, maxW, maxH)

	// Create draw options
	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterLinear
	op.GeoM.Scale(scale, scale)

	// Calculate position based on alignment
	scaledW := float64(img.Bounds().Dx()) * scale
	scaledH := float64(img.Bounds().Dy()) * scale

	xPos := r.CalculateHorizontalPosition(x, maxW, scaledW, align)
	yPos := float64(y) + float64(maxH)/2 - scaledH/2 // Always center vertically

	op.GeoM.Translate(xPos, yPos)
	screen.DrawImage(img, op)
}

func (r *Renderer) calculateImageScale(img *ebiten.Image, maxW, maxH int) float64 {
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()

	if r.game.fullscreen {
		return math.Min(float64(maxW)/float64(iw), float64(maxH)/float64(ih))
	}

	// In windowed mode, don't scale up small images
	if iw > maxW || ih > maxH {
		return math.Min(float64(maxW)/float64(iw), float64(maxH)/float64(ih))
	}
	return 1
}

func (r *Renderer) CalculateHorizontalPosition(x, maxW int, scaledW float64, align string) float64 {
	switch align {
	case "left":
		return float64(x)
	case "right":
		return float64(x+maxW) - scaledW
	default: // "center"
		return float64(x) + float64(maxW)/2 - scaledW/2
	}
}

func (r *Renderer) drawHelpOverlay(screen *ebiten.Image) {
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
		Source: r.helpFontSource,
		Size:   r.game.config.HelpFontSize,
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

	currentY := titleY + r.game.config.HelpFontSize*2 // Start below title
	lineHeight := r.game.config.HelpFontSize * 1.5

	// Draw each section
	for _, section := range r.helpSections {
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

func (r *Renderer) drawPageInputOverlay(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Create font for page input
	inputFont := &text.GoTextFace{
		Source: r.helpFontSource,
		Size:   r.game.config.HelpFontSize,
	}

	// Create smaller font for range display
	rangeFont := &text.GoTextFace{
		Source: r.helpFontSource,
		Size:   r.game.config.HelpFontSize * 0.8,
	}

	// Get total pages for range display
	totalPages := r.game.imageManager.GetPathsCount()

	// Create display texts
	inputText := fmt.Sprintf("Go to page: %s_", r.game.pageInputBuffer)
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

func (r *Renderer) drawInfoDisplay(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Create font for info display (same size as help text)
	infoFont := &text.GoTextFace{
		Source: r.helpFontSource,
		Size:   r.game.config.HelpFontSize,
	}

	// Get page status text
	infoText := r.game.getCurrentPageNumber()

	// Measure text dimensions
	textWidth, textHeight := text.Measure(infoText, infoFont, 0)

	// Position at bottom right corner
	padding := 10
	textX := float64(w) - textWidth - float64(padding)
	textY := float64(h) - textHeight - float64(padding)

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
	text.Draw(screen, infoText, infoFont, textOp)
}

func (r *Renderer) drawOverlayMessage(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Create font for overlay message
	messageFont := &text.GoTextFace{
		Source: r.helpFontSource,
		Size:   r.game.config.HelpFontSize,
	}

	// Measure text dimensions
	textWidth, textHeight := text.Measure(r.game.overlayMessage, messageFont, 0)

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
	text.Draw(screen, r.game.overlayMessage, messageFont, textOp)
}
