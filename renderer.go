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
	"golang.org/x/image/font/gofont/goregular"
)

// Common colors used in rendering
var (
	colorWhite     = color.RGBA{255, 255, 255, 255}
	colorGray      = color.RGBA{180, 180, 180, 255}
	colorLightGray = color.RGBA{192, 192, 192, 255}
	colorYellow    = color.RGBA{255, 255, 100, 255}
	colorCyan      = color.RGBA{100, 255, 255, 255}
	colorLightBlue = color.RGBA{200, 200, 255, 255}
	colorGreen     = color.RGBA{100, 255, 100, 255}
	colorOrange    = color.RGBA{255, 200, 100, 255}
	colorLightRed  = color.RGBA{255, 150, 150, 255}

	// Background colors for semi-transparent overlays
	bgColorLight  = color.RGBA{0, 0, 0, 128} // Light semi-transparent
	bgColorMedium = color.RGBA{0, 0, 0, 160} // Medium semi-transparent
	bgColorDark   = color.RGBA{0, 0, 0, 200} // Dark semi-transparent
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

// getActionsList returns a sorted list of all actions that have bindings
func (r *Renderer) getActionsList() []string {
	keybindings := r.renderState.GetKeybindings()
	mousebindings := r.renderState.GetMousebindings()

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
	return actions
}

// Draw renders the entire screen
func (r *Renderer) Draw(screen *ebiten.Image) {
	// Clear the screen since SetScreenClearedEveryFrame(false) is enabled
	screen.Clear()

	// Get display content - all rendering decisions are already made
	content := r.renderState.GetDisplayContent()
	if content == nil || content.LeftImage == nil {
		// No content to display
		return
	}

	// Draw images (unified handling for single and book mode)
	r.drawImagesDirect(screen, content.LeftImage, content.RightImage)

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
	w, h := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())

	// Calculate available space (accounting for padding)
	padding := 40.0
	availableWidth := w - padding*2
	availableHeight := h - padding*2

	// Calculate optimal font size
	optimalFontSize, canFit := r.calculateOptimalFontSize(availableWidth, availableHeight)

	// If cannot fit even with minimum font size, show Fermat's joke
	if !canFit {
		r.drawMarginTooSmallMessage(screen)
		return
	}

	// Get data needed for rendering
	actions := r.getActionsList()
	keybindings := r.renderState.GetKeybindings()
	mousebindings := r.renderState.GetMousebindings()
	configStatus := r.renderState.GetConfigStatus()

	// Semi-transparent black background (lighter for more image transparency)
	DrawFilledRect(screen, 0, 0, w, h, bgColorLight)

	// Help text area with semi-transparent black background
	DrawFilledRect(screen, padding, padding, w-padding*2, h-padding*2, bgColorMedium)

	// Create font with dynamically calculated size
	helpFont := &text.GoTextFace{
		Source: r.helpFontSource,
		Size:   optimalFontSize,
	}

	// Draw title
	titleY := padding + 30
	DrawText(screen, "HELP:", helpFont, padding+20, titleY, colorWhite)

	currentY := titleY + optimalFontSize*2 // Start below title
	lineHeight := optimalFontSize * 1.5

	// Get action descriptions
	actionDescriptions := getActionDescriptions()

	// Draw input bindings title
	DrawText(screen, "Controls (Keyboard | Mouse):", helpFont, padding+20, currentY, colorWhite)
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
	actionColumnX := padding + 40
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
		DrawText(screen, action, helpFont, actionColumnX, currentY, colorLightBlue)

		// Draw arrow
		DrawText(screen, "→", helpFont, arrowColumnX, currentY, colorWhite)

		// Draw combined input bindings with color coding
		currentInputX := inputColumnX

		// Draw keyboard bindings in yellow
		if len(keys) > 0 {
			keysList := strings.Join(keys, ", ")
			DrawText(screen, keysList, helpFont, currentInputX, currentY, colorYellow)

			keysWidth, _ := text.Measure(keysList, helpFont, 0)
			currentInputX += keysWidth
		}

		// Draw separator if both keyboard and mouse bindings exist
		if len(keys) > 0 && len(mouseActions) > 0 {
			DrawText(screen, " | ", helpFont, currentInputX, currentY, colorWhite)

			sepWidth, _ := text.Measure(" | ", helpFont, 0)
			currentInputX += sepWidth
		}

		// Draw mouse bindings in cyan
		if len(mouseActions) > 0 {
			mouseList := strings.Join(mouseActions, ", ")
			DrawText(screen, mouseList, helpFont, currentInputX, currentY, colorCyan)
		}

		// Draw description on same line
		DrawText(screen, description, helpFont, descColumnX, currentY, colorGray)

		currentY += lineHeight
	}

	// Add some spacing before config status
	currentY += lineHeight

	// Draw config status section

	// Draw section title
	DrawText(screen, "System:", helpFont, padding+20, currentY, colorWhite)
	currentY += lineHeight

	// Add config status
	statusText := fmt.Sprintf("Config Status: %s", configStatus.Status)

	statusColor := colorGreen
	if configStatus.Status == "Warning" || configStatus.Status == "Error" {
		statusColor = colorOrange
	}
	DrawText(screen, statusText, helpFont, padding+40, currentY, statusColor)
	currentY += lineHeight

	// Add warnings if any
	if len(configStatus.Warnings) > 0 {
		for i, warning := range configStatus.Warnings {
			if i >= 2 { // Limit to first 2 warnings to avoid clutter
				break
			}
			shortWarning := warning
			if len(shortWarning) > 50 {
				shortWarning = shortWarning[:47] + "..."
			}
			DrawText(screen, "• "+shortWarning, helpFont, padding+40, currentY, colorLightRed)
			currentY += lineHeight
		}
	}

}

// calculateRequiredDimensions calculates the required width and height for help content at a given font size
func (r *Renderer) calculateRequiredDimensions(fontSize float64) (float64, float64) {
	actions := r.getActionsList()
	keybindings := r.renderState.GetKeybindings()
	mousebindings := r.renderState.GetMousebindings()
	configStatus := r.renderState.GetConfigStatus()
	// Create temporary font for measurements
	tempFont := &text.GoTextFace{
		Source: r.helpFontSource,
		Size:   fontSize,
	}

	padding := 40.0
	lineHeight := fontSize * 1.5

	// Calculate height
	height := padding * 2      // Top and bottom padding
	height += fontSize * 2     // Title
	height += lineHeight * 1.5 // Controls title spacing

	// Count lines for actions
	actionLines := 0
	for _, action := range actions {
		keys := keybindings[action]
		mouseActions := mousebindings[action]
		// Skip if no bindings at all
		if len(keys) == 0 && len(mouseActions) == 0 {
			continue
		}
		actionLines++
	}
	height += float64(actionLines) * lineHeight

	// System section
	height += lineHeight // Spacing before system section
	height += lineHeight // "System:" title
	height += lineHeight // Config status line

	// Add warnings if any (limit to 2 like in original code)
	warningLines := len(configStatus.Warnings)
	if warningLines > 2 {
		warningLines = 2
	}
	height += float64(warningLines) * lineHeight

	// Calculate width
	maxWidth := 0.0

	// Check title width
	titleWidth, _ := text.Measure("HELP:", tempFont, 0)
	if titleWidth+padding*2+40 > maxWidth { // 40 for left margin
		maxWidth = titleWidth + padding*2 + 40
	}

	// Check controls title width
	controlsTitleWidth, _ := text.Measure("Controls (Keyboard | Mouse):", tempFont, 0)
	if controlsTitleWidth+padding*2+40 > maxWidth {
		maxWidth = controlsTitleWidth + padding*2 + 40
	}

	// Calculate column widths for actions (similar to original logic)
	maxActionWidth := 0.0
	maxInputWidth := 0.0
	maxDescWidth := 0.0

	actionDescriptions := getActionDescriptions()

	for _, action := range actions {
		keys := keybindings[action]
		mouseActions := mousebindings[action]

		// Skip if no bindings at all
		if len(keys) == 0 && len(mouseActions) == 0 {
			continue
		}

		// Measure action name width
		actionWidth, _ := text.Measure(action, tempFont, 0)
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
		inputWidth, _ := text.Measure(combinedInput, tempFont, 0)
		if inputWidth > maxInputWidth {
			maxInputWidth = inputWidth
		}

		// Measure description width
		description := actionDescriptions[action]
		if description == "" {
			description = "No description available"
		}
		descWidth, _ := text.Measure(description, tempFont, 0)
		if descWidth > maxDescWidth {
			maxDescWidth = descWidth
		}
	}

	// Calculate total width: left margin + action + spacing + arrow + spacing + input + spacing + description + right margin
	actionLineWidth := 40 + maxActionWidth + 20 + 30 + 20 + maxInputWidth + 20 + maxDescWidth + padding
	if actionLineWidth > maxWidth {
		maxWidth = actionLineWidth
	}

	// Check system section width
	systemTitleWidth, _ := text.Measure("System:", tempFont, 0)
	if systemTitleWidth+padding*2+40 > maxWidth {
		maxWidth = systemTitleWidth + padding*2 + 40
	}

	statusText := fmt.Sprintf("Config Status: %s", configStatus.Status)
	statusWidth, _ := text.Measure(statusText, tempFont, 0)
	if statusWidth+padding*2+80 > maxWidth { // 80 for indentation
		maxWidth = statusWidth + padding*2 + 80
	}

	// Check warning widths
	for i, warning := range configStatus.Warnings {
		if i >= 2 {
			break
		}
		shortWarning := warning
		if len(shortWarning) > 50 {
			shortWarning = shortWarning[:47] + "..."
		}
		warningWidth, _ := text.Measure("• "+shortWarning, tempFont, 0)
		if warningWidth+padding*2+80 > maxWidth {
			maxWidth = warningWidth + padding*2 + 80
		}
	}

	return maxWidth, height
}

// calculateOptimalFontSize finds the largest font size that fits within the given dimensions
func (r *Renderer) calculateOptimalFontSize(availableWidth, availableHeight float64) (float64, bool) {
	maxFontSize := r.renderState.GetFontSize()
	minFontSize := 12.0

	// Quick check: can we fit with minimum font size?
	minWidth, minHeight := r.calculateRequiredDimensions(minFontSize)
	if minWidth > availableWidth || minHeight > availableHeight {
		return minFontSize, false // Cannot fit even with minimum size
	}

	// Quick check: can we fit with maximum font size?
	maxWidth, maxHeight := r.calculateRequiredDimensions(maxFontSize)
	if maxWidth <= availableWidth && maxHeight <= availableHeight {
		return maxFontSize, true // Fits perfectly with maximum size
	}

	// Binary search for optimal font size
	low := minFontSize
	high := maxFontSize
	bestSize := minFontSize
	epsilon := 0.5 // Search precision

	for high-low > epsilon {
		mid := (low + high) / 2.0

		reqWidth, reqHeight := r.calculateRequiredDimensions(mid)

		if reqWidth <= availableWidth && reqHeight <= availableHeight {
			// This size fits, try larger
			bestSize = mid
			low = mid
		} else {
			// This size doesn't fit, try smaller
			high = mid
		}
	}

	return bestSize, true
}

// drawMarginTooSmallMessage displays Fermat's margin joke when help cannot fit
func (r *Renderer) drawMarginTooSmallMessage(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Semi-transparent black background
	DrawFilledRect(screen, 0, 0, float64(w), float64(h), bgColorLight)

	// Create font for the joke (16px should be readable)
	jokeFont := &text.GoTextFace{
		Source: r.helpFontSource,
		Size:   16.0,
	}

	// The famous quote from Fermat's Last Theorem margin note
	message := "Hanc marginis exiguitas non caperet."
	subtitle := "(This margin is too small to contain it.)"

	// Measure text for centering
	messageWidth, messageHeight := text.Measure(message, jokeFont, 0)
	subtitleWidth, _ := text.Measure(subtitle, jokeFont, 0)

	// Calculate center positions
	messageX := float64(w)/2 - messageWidth/2
	messageY := float64(h)/2 - messageHeight/2

	subtitleX := float64(w)/2 - subtitleWidth/2
	subtitleY := messageY + messageHeight + 10 // 10px spacing

	// Draw main message
	DrawText(screen, message, jokeFont, messageX, messageY, colorWhite)

	// Draw subtitle in gray
	DrawText(screen, subtitle, jokeFont, subtitleX, subtitleY, colorGray)
}

func (r *Renderer) drawPageInputOverlay(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Create font for page input
	inputFont := &text.GoTextFace{
		Source: r.helpFontSource,
		Size:   r.renderState.GetFontSize(),
	}

	// Create smaller font for range display
	rangeFont := &text.GoTextFace{
		Source: r.helpFontSource,
		Size:   r.renderState.GetFontSize() * 0.8,
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
	DrawFilledRect(screen, boxX, boxY, boxWidth, boxHeight, bgColorDark)

	// Draw input text (centered)
	inputTextX := boxX + (boxWidth-inputWidth)/2
	DrawText(screen, inputText, inputFont, inputTextX, boxY+float64(padding), colorWhite)

	// Draw range text (centered, below input text)
	rangeTextX := boxX + (boxWidth-rangeWidth)/2
	DrawText(screen, rangeText, rangeFont, rangeTextX, boxY+float64(padding)+inputHeight+10, colorLightGray)
}

func (r *Renderer) drawInfoDisplay(screen *ebiten.Image) {
	// Create font for info display (same size as help text)
	infoFont := &text.GoTextFace{
		Source: r.helpFontSource,
		Size:   r.renderState.GetFontSize(),
	}

	// Get page status text
	infoText := r.buildPageNumberString()

	// Measure text dimensions
	textWidth, textHeight := text.Measure(infoText, infoFont, 0)

	// Position at bottom right corner
	padding := 10.0
	textX := float64(screen.Bounds().Dx()) - textWidth - padding
	textY := float64(screen.Bounds().Dy()) - textHeight - padding

	// Semi-transparent background
	bgPadding := 5.0
	bgX := textX - bgPadding
	bgY := textY - bgPadding
	bgW := textWidth + bgPadding*2
	bgH := textHeight + bgPadding*2

	DrawFilledRect(screen, bgX, bgY, bgW, bgH, bgColorLight)

	// Draw text
	DrawText(screen, infoText, infoFont, textX, textY, colorWhite)
}

func (r *Renderer) drawOverlayMessage(screen *ebiten.Image) {
	// Create font for overlay message
	messageFont := &text.GoTextFace{
		Source: r.helpFontSource,
		Size:   r.renderState.GetFontSize(),
	}

	// Measure text dimensions
	textWidth, textHeight := text.Measure(r.renderState.GetOverlayMessage(), messageFont, 0)

	// Calculate position (center of screen)
	padding := 20.0
	boxWidth := textWidth + padding*2
	boxHeight := textHeight + padding*2
	boxX := (float64(screen.Bounds().Dx()) - boxWidth) / 2
	boxY := (float64(screen.Bounds().Dy()) - boxHeight) / 2

	// Semi-transparent black background
	DrawFilledRect(screen, boxX, boxY, boxWidth, boxHeight, bgColorDark)

	// Draw text
	DrawText(screen, r.renderState.GetOverlayMessage(), messageFont, boxX+padding, boxY+padding, colorWhite)
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

func (r *Renderer) buildPageNumberString() string {
	content := r.renderState.GetDisplayContent()
	if content == nil {
		return "0 / 0"
	}

	total := content.Metadata.TotalPages
	currentPage := content.Metadata.CurrentPage
	actualImages := content.Metadata.ActualImages

	if actualImages == 2 {
		// 2 images displayed = book mode
		rightPage := currentPage + 1
		if rightPage > total {
			rightPage = total
		}
		return fmt.Sprintf("%d-%d / %d", currentPage, rightPage, total)
	} else {
		// 1 image displayed = single mode
		return fmt.Sprintf("%d / %d", currentPage, total)
	}
}

func (r *Renderer) drawTransformedImageCentered(screen *ebiten.Image, img *ebiten.Image) {
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	w, h := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())

	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterLinear

	// Calculate scale and position based on zoom mode
	var scale float64
	var offsetX, offsetY float64

	if r.renderState.GetZoomMode() == ZoomModeFitWindow {
		// Fit to window mode - calculate scale here for centering
		if r.renderState.IsFullscreen() {
			scale = math.Min(w/iw, h/ih)
		} else {
			if iw > w || ih > h {
				scale = math.Min(w/iw, h/ih)
			} else {
				scale = 1
			}
		}
		// Center the image
		sw, sh := iw*scale, ih*scale
		offsetX = w/2 - sw/2
		offsetY = h/2 - sh/2
	} else {
		// All other modes (FitWidth, FitHeight, Manual) - use pre-calculated zoom level
		scale = r.renderState.GetZoomLevel()
		sw, sh := iw*scale, ih*scale

		// Apply pan offset with boundary clamping
		panX := r.renderState.GetPanOffsetX()
		panY := r.renderState.GetPanOffsetY()

		// Calculate boundaries
		minX := w - sw
		maxX := 0.0
		minY := h - sh
		maxY := 0.0

		// Clamp pan offsets to keep image on screen
		if sw <= w {
			// Image is smaller than screen width, center horizontally
			offsetX = w/2 - sw/2
		} else {
			// Image is larger than screen, apply pan with clamping
			offsetX = math.Max(minX, math.Min(maxX, w/2-sw/2+panX))
		}

		if sh <= h {
			// Image is smaller than screen height, center vertically
			offsetY = h/2 - sh/2
		} else {
			// Image is larger than screen, apply pan with clamping
			offsetY = math.Max(minY, math.Min(maxY, h/2-sh/2+panY))
		}
	}

	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(offsetX, offsetY)

	screen.DrawImage(img, op)
}

// drawImagesDirect draws images (single or book mode) without any mode checking
func (r *Renderer) drawImagesDirect(screen *ebiten.Image, leftImg, rightImg *ebiten.Image) {
	if leftImg == nil {
		return
	}

	// createBookModeImage handles both single (rightImg=nil) and book mode cases
	finalImg := r.createBookModeImage(leftImg, rightImg)

	// Apply transformations to the final image
	transformedImg := r.applyTransformations(finalImg)

	// Draw the transformed image
	r.drawTransformedImageCentered(screen, transformedImg)
}
