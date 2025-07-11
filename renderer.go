package main

import (
	"bytes"
	"fmt"
	"image/color"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/goregular"
)

// Renderer handles all drawing operations
type Renderer struct {
	renderState    RenderState
	helpFontSource *text.GoTextFaceSource
	lastSnapshot   *RenderStateSnapshot // Previous frame's state for comparison
}

// NewRenderer creates a new Renderer
func NewRenderer(renderState RenderState) *Renderer {
	// Initialize font source with lightweight goregular
	s, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		log.Fatal(err)
	}

	return &Renderer{
		renderState:    renderState,
		helpFontSource: s,
	}
}

// getActionDescriptions returns descriptions for each action
func getActionDescriptions() map[string]string {
	return GetActionDescriptions()
}

// Draw renders the entire screen
func (r *Renderer) Draw(screen *ebiten.Image) {
	// Clear the screen since SetScreenClearedEveryFrame(false) is enabled
	screen.Clear()

	if r.renderState.IsTempSingleMode() || !r.renderState.IsBookMode() {
		// Single page mode or temporary single mode
		r.drawSingleImage(screen)
	} else {
		// Book mode
		r.drawBookMode(screen)
	}

	// Draw info display (page status, etc.) at bottom of screen if enabled
	if r.renderState.IsShowingInfo() {
		r.drawInfoDisplay(screen)
	}

	// Draw help overlay if enabled
	if r.renderState.IsShowingHelp() {
		r.drawHelpOverlay(screen)
	}

	// Draw page input overlay if active
	if r.renderState.IsInPageInputMode() {
		r.drawPageInputOverlay(screen)
	}

	// Draw overlay message if active
	if r.renderState.GetOverlayMessage() != "" && time.Since(r.renderState.GetOverlayMessageTime()) < overlayMessageDuration {
		r.drawOverlayMessage(screen)
	}
}

func (r *Renderer) drawSingleImage(screen *ebiten.Image) {
	img := r.renderState.GetCurrentImage()
	if img == nil {
		return
	}

	// Apply transformations to the image
	transformedImg := r.applyTransformations(img)

	iw, ih := transformedImg.Bounds().Dx(), transformedImg.Bounds().Dy()
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Get device scale factor for hi-DPI support
	deviceScale := ebiten.DeviceScaleFactor()

	var scale float64
	if r.renderState.IsFullscreen() {
		scale = math.Min(float64(w)/float64(iw), float64(h)/float64(ih))
	} else {
		if iw > w || ih > h {
			scale = math.Min(float64(w)/float64(iw), float64(h)/float64(ih))
		} else {
			scale = deviceScale
		}
	}

	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterLinear
	op.GeoM.Scale(scale, scale)
	sw, sh := float64(iw)*scale, float64(ih)*scale
	op.GeoM.Translate(float64(w)/2-sw/2, float64(h)/2-sh/2)

	screen.DrawImage(transformedImg, op)
}

func (r *Renderer) drawBookMode(screen *ebiten.Image) {
	leftImg, rightImg := r.renderState.GetBookModeImages()
	if leftImg == nil {
		return
	}

	// Check if images are compatible for book mode display
	if !r.renderState.ShouldUseBookMode(leftImg, rightImg) {
		// Fall back to single page display
		r.drawSingleImage(screen)
		return
	}

	// Create a combined image for book mode
	combinedImg := r.createBookModeImage(leftImg, rightImg)

	// Apply transformations to the combined image
	transformedImg := r.applyTransformations(combinedImg)

	// Draw the transformed combined image
	r.drawTransformedImageCentered(screen, transformedImg)
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

	if r.renderState.IsFullscreen() {
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
		Size:   r.renderState.GetHelpFontSize(),
	}

	// Draw title
	titleY := float64(padding + 30)
	titleOp := &text.DrawOptions{}
	titleOp.GeoM.Translate(float64(padding+20), titleY)
	titleOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255})
	text.Draw(screen, "HELP:", helpFont, titleOp)

	currentY := titleY + r.renderState.GetHelpFontSize()*2 // Start below title
	lineHeight := r.renderState.GetHelpFontSize() * 1.5

	// Dynamic input bindings display
	keybindings := r.renderState.GetKeybindings()
	mousebindings := r.renderState.GetMousebindings()
	actionDescriptions := getActionDescriptions()

	// Get sorted action list for consistent display (union of keyboard and mouse actions)
	actionSet := make(map[string]bool)
	for action := range keybindings {
		actionSet[action] = true
	}
	for action := range mousebindings {
		actionSet[action] = true
	}

	actions := make([]string, 0, len(actionSet))
	for action := range actionSet {
		actions = append(actions, action)
	}
	sort.Strings(actions)

	// Draw input bindings title
	inputTitleOp := &text.DrawOptions{}
	inputTitleOp.GeoM.Translate(float64(padding+20), currentY)
	inputTitleOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255})
	text.Draw(screen, "Controls (Keyboard | Mouse):", helpFont, inputTitleOp)
	currentY += lineHeight * 1.5

	// Calculate column widths using text measurement
	maxActionWidth := 0.0
	maxInputWidth := 0.0

	// First pass: measure text to determine column widths
	for _, action := range actions {
		keys := keybindings[action]
		mouseActions := mousebindings[action]

		// Skip if no bindings at all
		if len(keys) == 0 && len(mouseActions) == 0 {
			continue
		}

		// Measure action name width
		actionWidth, _ := text.Measure(action, helpFont, 0)
		if actionWidth > maxActionWidth {
			maxActionWidth = actionWidth
		}

		// Build combined input string (keyboard | mouse)
		var inputParts []string
		if len(keys) > 0 {
			inputParts = append(inputParts, strings.Join(keys, ", "))
		}
		if len(mouseActions) > 0 {
			inputParts = append(inputParts, strings.Join(mouseActions, ", "))
		}

		combinedInput := strings.Join(inputParts, " | ")
		inputWidth, _ := text.Measure(combinedInput, helpFont, 0)
		if inputWidth > maxInputWidth {
			maxInputWidth = inputWidth
		}
	}

	// Calculate column positions with proper spacing
	actionColumnX := float64(padding + 40)
	arrowColumnX := actionColumnX + maxActionWidth + 20 // 20px spacing
	inputColumnX := arrowColumnX + 30                   // Arrow width + spacing
	descColumnX := inputColumnX + maxInputWidth + 20    // 20px spacing after input

	// Draw each action and its input bindings on single line
	for _, action := range actions {
		keys := keybindings[action]
		mouseActions := mousebindings[action]

		// Skip if no bindings at all
		if len(keys) == 0 && len(mouseActions) == 0 {
			continue
		}

		// Get description
		description := actionDescriptions[action]
		if description == "" {
			description = "No description available"
		}

		// Draw action name (left-aligned)
		actionOp := &text.DrawOptions{}
		actionOp.GeoM.Translate(actionColumnX, currentY)
		actionOp.ColorScale.ScaleWithColor(color.RGBA{200, 200, 255, 255}) // Light blue for action names
		text.Draw(screen, action, helpFont, actionOp)

		// Draw arrow
		arrowOp := &text.DrawOptions{}
		arrowOp.GeoM.Translate(arrowColumnX, currentY)
		arrowOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255})
		text.Draw(screen, "→", helpFont, arrowOp)

		// Draw combined input bindings with color coding
		currentInputX := inputColumnX

		// Draw keyboard bindings in yellow
		if len(keys) > 0 {
			keysList := strings.Join(keys, ", ")
			keysOp := &text.DrawOptions{}
			keysOp.GeoM.Translate(currentInputX, currentY)
			keysOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 100, 255}) // Yellow for keyboard
			text.Draw(screen, keysList, helpFont, keysOp)

			keysWidth, _ := text.Measure(keysList, helpFont, 0)
			currentInputX += keysWidth
		}

		// Draw separator if both keyboard and mouse bindings exist
		if len(keys) > 0 && len(mouseActions) > 0 {
			sepOp := &text.DrawOptions{}
			sepOp.GeoM.Translate(currentInputX, currentY)
			sepOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255}) // White for separator
			text.Draw(screen, " | ", helpFont, sepOp)

			sepWidth, _ := text.Measure(" | ", helpFont, 0)
			currentInputX += sepWidth
		}

		// Draw mouse bindings in cyan
		if len(mouseActions) > 0 {
			mouseList := strings.Join(mouseActions, ", ")
			mouseOp := &text.DrawOptions{}
			mouseOp.GeoM.Translate(currentInputX, currentY)
			mouseOp.ColorScale.ScaleWithColor(color.RGBA{100, 255, 255, 255}) // Cyan for mouse
			text.Draw(screen, mouseList, helpFont, mouseOp)
		}

		// Draw description on same line
		descOp := &text.DrawOptions{}
		descOp.GeoM.Translate(descColumnX, currentY)
		descOp.ColorScale.ScaleWithColor(color.RGBA{180, 180, 180, 255}) // Gray for descriptions
		text.Draw(screen, description, helpFont, descOp)

		currentY += lineHeight
	}

	// Add some spacing before config status
	currentY += lineHeight

	// Draw config status section
	configStatus := r.renderState.GetConfigStatus()

	// Draw section title
	sectionOp := &text.DrawOptions{}
	sectionOp.GeoM.Translate(float64(padding+20), currentY)
	sectionOp.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255})
	text.Draw(screen, "System:", helpFont, sectionOp)
	currentY += lineHeight

	// Add config status
	statusText := fmt.Sprintf("Config Status: %s", configStatus.Status)

	descOp := &text.DrawOptions{}
	descOp.GeoM.Translate(float64(padding+40), currentY)
	if configStatus.Status == "Warning" || configStatus.Status == "Error" {
		descOp.ColorScale.ScaleWithColor(color.RGBA{255, 200, 100, 255}) // Orange for warnings/errors
	} else {
		descOp.ColorScale.ScaleWithColor(color.RGBA{100, 255, 100, 255}) // Green for OK
	}
	text.Draw(screen, statusText, helpFont, descOp)
	currentY += lineHeight

	// Add warnings if any
	if len(configStatus.Warnings) > 0 {
		for i, warning := range configStatus.Warnings {
			if i >= 2 { // Limit to first 2 warnings to avoid clutter
				break
			}
			warningOp := &text.DrawOptions{}
			warningOp.GeoM.Translate(float64(padding+40), currentY)
			warningOp.ColorScale.ScaleWithColor(color.RGBA{255, 150, 150, 255}) // Light red for warnings
			shortWarning := warning
			if len(shortWarning) > 50 {
				shortWarning = shortWarning[:47] + "..."
			}
			text.Draw(screen, "• "+shortWarning, helpFont, warningOp)
			currentY += lineHeight
		}
	}

}

func (r *Renderer) drawPageInputOverlay(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Create font for page input
	inputFont := &text.GoTextFace{
		Source: r.helpFontSource,
		Size:   r.renderState.GetHelpFontSize(),
	}

	// Create smaller font for range display
	rangeFont := &text.GoTextFace{
		Source: r.helpFontSource,
		Size:   r.renderState.GetHelpFontSize() * 0.8,
	}

	// Get total pages for range display
	totalPages := r.renderState.GetTotalPagesCount()

	// Create display texts
	inputText := fmt.Sprintf("Go to page: %s_", r.renderState.GetPageInputBuffer())
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
		Size:   r.renderState.GetHelpFontSize(),
	}

	// Get page status text
	infoText := r.renderState.GetCurrentPageNumber()

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
		Size:   r.renderState.GetHelpFontSize(),
	}

	// Measure text dimensions
	textWidth, textHeight := text.Measure(r.renderState.GetOverlayMessage(), messageFont, 0)

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
	text.Draw(screen, r.renderState.GetOverlayMessage(), messageFont, textOp)
}

func (r *Renderer) applyTransformations(img *ebiten.Image) *ebiten.Image {
	if r.renderState.GetRotationAngle() == 0 && !r.renderState.IsFlippedH() && !r.renderState.IsFlippedV() {
		return img
	}

	w, h := img.Bounds().Dx(), img.Bounds().Dy()

	// Calculate final dimensions after rotation
	var finalW, finalH int
	if r.renderState.GetRotationAngle() == 90 || r.renderState.GetRotationAngle() == 270 {
		finalW, finalH = h, w
	} else {
		finalW, finalH = w, h
	}

	// Create new image with final dimensions
	transformedImg := ebiten.NewImage(finalW, finalH)

	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterLinear

	// Apply transformations in order: flip, then rotate
	centerX, centerY := float64(w)/2, float64(h)/2

	// Reset to origin
	op.GeoM.Translate(-centerX, -centerY)

	// Apply flips
	if r.renderState.IsFlippedH() {
		op.GeoM.Scale(-1, 1)
	}
	if r.renderState.IsFlippedV() {
		op.GeoM.Scale(1, -1)
	}

	// Apply rotation
	if r.renderState.GetRotationAngle() != 0 {
		op.GeoM.Rotate(float64(r.renderState.GetRotationAngle()) * math.Pi / 180)
	}

	// Move to center of new image
	op.GeoM.Translate(float64(finalW)/2, float64(finalH)/2)

	transformedImg.DrawImage(img, op)
	return transformedImg
}

func (r *Renderer) createBookModeImage(leftImg, rightImg *ebiten.Image) *ebiten.Image {
	if rightImg == nil {
		return leftImg
	}

	leftW, leftH := leftImg.Bounds().Dx(), leftImg.Bounds().Dy()
	rightW, rightH := rightImg.Bounds().Dx(), rightImg.Bounds().Dy()

	// Calculate combined dimensions
	combinedW := leftW + rightW + imageGap
	combinedH := int(math.Max(float64(leftH), float64(rightH)))

	// Create combined image
	combinedImg := ebiten.NewImage(combinedW, combinedH)

	// Draw left image (right-aligned in its space)
	leftOp := &ebiten.DrawImageOptions{}
	leftOp.Filter = ebiten.FilterLinear
	leftOp.GeoM.Translate(0, float64(combinedH)/2-float64(leftH)/2)
	combinedImg.DrawImage(leftImg, leftOp)

	// Draw right image (left-aligned in its space)
	rightOp := &ebiten.DrawImageOptions{}
	rightOp.Filter = ebiten.FilterLinear
	rightOp.GeoM.Translate(float64(leftW+imageGap), float64(combinedH)/2-float64(rightH)/2)
	combinedImg.DrawImage(rightImg, rightOp)

	return combinedImg
}

func (r *Renderer) drawTransformedImageCentered(screen *ebiten.Image, img *ebiten.Image) {
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Get device scale factor for hi-DPI support
	deviceScale := ebiten.DeviceScaleFactor()

	var scale float64
	if r.renderState.IsFullscreen() {
		scale = math.Min(float64(w)/float64(iw), float64(h)/float64(ih))
	} else {
		if iw > w || ih > h {
			scale = math.Min(float64(w)/float64(iw), float64(h)/float64(ih))
		} else {
			scale = deviceScale
		}
	}

	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterLinear
	op.GeoM.Scale(scale, scale)
	sw, sh := float64(iw)*scale, float64(ih)*scale
	op.GeoM.Translate(float64(w)/2-sw/2, float64(h)/2-sh/2)

	screen.DrawImage(img, op)
}
