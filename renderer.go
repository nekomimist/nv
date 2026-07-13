package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
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
	lastSnapshot   RenderStateSnapshot // Previous frame's state for comparison
	hasSnapshot    bool                // Whether lastSnapshot holds a valid snapshot
	tilePointCache map[image.Point]tileScreenPoint
}

// NewRenderer creates a new Renderer
func NewRenderer(renderState RenderState) *Renderer {
	// Initialize font source with lightweight goregular
	s, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		fatalKV("renderer", "font_init_failed", "error", err)
	}

	return &Renderer{
		renderState:    renderState,
		helpFontSource: s,
		tilePointCache: make(map[image.Point]tileScreenPoint),
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

	// Draw settings overlay if active (only when base was redrawn)
	if r.renderState.IsShowingSettings() {
		r.drawSettingsOverlay(screen)
	}

	// Draw overlay message if active
	if r.renderState.GetOverlayMessage() != "" && time.Since(r.renderState.GetOverlayMessageTime()) < overlayMessageDuration {
		r.drawOverlayMessage(screen)
	}
}

// drawSettingsOverlay renders the settings panel
func (r *Renderer) drawSettingsOverlay(screen *ebiten.Image) {
	w, h := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())

	// Fonts
	titleFont := &text.GoTextFace{Source: r.helpFontSource, Size: 22}
	itemFont := &text.GoTextFace{Source: r.helpFontSource, Size: 18}
	hintFont := &text.GoTextFace{Source: r.helpFontSource, Size: 14}

	// Dim background and panel
	DrawFilledRect(screen, 0, 0, w, h, bgColorLight)

	items := settingsListOrder()
	panelW := math.Min(700, w*0.9)
	panelH := math.Min(0.9*h, 60+float64(len(items))*28+40)
	panelX := (w - panelW) / 2
	panelY := (h - panelH) / 2
	DrawFilledRect(screen, panelX, panelY, panelW, panelH, bgColorDark)

	// Title and hints
	DrawText(screen, "Settings", titleFont, panelX+16, panelY+20, colorWhite)
	hint := "↑/↓: select  ←/→: change  Enter: toggle  Ctrl+S: save  Esc: cancel"
	hw, hh := text.Measure(hint, hintFont, 0)
	DrawText(screen, hint, hintFont, panelX+panelW-hw-16, panelY+20+(22-hh)/2, colorLightGray)

	// List items
	startY := panelY + 60
	rowH := 26.0
	nameX := panelX + 24
	valX := panelX + panelW - 260
	selColor := color.RGBA{60, 60, 60, 200}

	cfg := r.renderState.GetPendingConfig()
	sel := r.renderState.GetSettingsIndex()
	for i, name := range items {
		y := startY + float64(i)*rowH
		if i == sel {
			DrawFilledRect(screen, panelX+8, y-4, panelW-16, rowH, selColor)
		}
		val := getSettingValueStringFromConfig(cfg, name)
		DrawText(screen, name, itemFont, nameX, y, colorWhite)
		DrawText(screen, val, itemFont, valX, y, colorCyan)
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

func (r *Renderer) buildPageNumberString() string {
	content := r.renderState.GetDisplayContent()
	if content == nil {
		return "0 / 0"
	}

	total := content.Metadata.TotalPages
	leftPage := content.Metadata.LeftPage
	rightPage := content.Metadata.RightPage
	actualImages := content.Metadata.ActualImages

	if actualImages == 2 {
		separator := "→"
		if leftPage > rightPage {
			separator = "←"
		}
		return fmt.Sprintf("%d%s%d / %d", leftPage, separator, rightPage, total)
	}

	return fmt.Sprintf("%d / %d", leftPage, total)
}

type displayLayout struct {
	canvasW      int
	canvasH      int
	transformedW int
	transformedH int
	leftX        int
	leftY        int
	rightX       int
	rightY       int
}

type tileScreenPoint struct {
	x float32
	y float32
}

var tileTriangleIndices = []uint16{0, 1, 2, 1, 2, 3}

// drawImagesDirect draws images (single or book mode) without any mode checking.
func (r *Renderer) drawImagesDirect(screen *ebiten.Image, leftImg, rightImg DisplayImage) {
	if leftImg == nil {
		return
	}

	layout := r.calculateDisplayLayout(leftImg, rightImg)
	scale, offsetX, offsetY := r.calculateDisplayTransform(screen, layout.transformedW, layout.transformedH)
	transform := r.calculateCanvasTransform(layout, scale, offsetX, offsetY)
	clear(r.tilePointCache)
	r.drawDisplayImageTiles(screen, leftImg, layout.leftX, layout.leftY, layout, scale, offsetX, offsetY, transform)
	if rightImg != nil {
		r.drawDisplayImageTiles(screen, rightImg, layout.rightX, layout.rightY, layout, scale, offsetX, offsetY, transform)
	}
}

func (r *Renderer) calculateDisplayLayout(leftImg, rightImg DisplayImage) displayLayout {
	leftBounds := leftImg.Bounds()
	leftW, leftH := leftBounds.Dx(), leftBounds.Dy()

	layout := displayLayout{
		canvasW: leftW,
		canvasH: leftH,
		leftX:   0,
		leftY:   0,
	}

	if rightImg != nil {
		rightBounds := rightImg.Bounds()
		rightW, rightH := rightBounds.Dx(), rightBounds.Dy()
		layout.canvasW = leftW + rightW + imageGap
		layout.canvasH = int(math.Max(float64(leftH), float64(rightH)))
		layout.leftY = layout.canvasH/2 - leftH/2
		layout.rightX = leftW + imageGap
		layout.rightY = layout.canvasH/2 - rightH/2
	}

	if r.renderState.GetRotationAngle() == 90 || r.renderState.GetRotationAngle() == 270 {
		layout.transformedW = layout.canvasH
		layout.transformedH = layout.canvasW
	} else {
		layout.transformedW = layout.canvasW
		layout.transformedH = layout.canvasH
	}

	return layout
}

func (r *Renderer) calculateDisplayTransform(screen *ebiten.Image, imageW, imageH int) (float64, float64, float64) {
	iw, ih := float64(imageW), float64(imageH)
	w, h := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())

	var scale float64
	var offsetX, offsetY float64

	if r.renderState.GetZoomMode() == ZoomModeFitWindow {
		if r.renderState.IsFullscreen() {
			scale = math.Min(w/iw, h/ih)
		} else if iw > w || ih > h {
			scale = math.Min(w/iw, h/ih)
		} else {
			scale = 1
		}
		sw, sh := iw*scale, ih*scale
		offsetX = w/2 - sw/2
		offsetY = h/2 - sh/2
		return scale, offsetX, offsetY
	}

	scale = r.renderState.GetZoomLevel()
	sw, sh := iw*scale, ih*scale
	panX := r.renderState.GetPanOffsetX()
	panY := r.renderState.GetPanOffsetY()

	if sw <= w {
		offsetX = w/2 - sw/2
	} else {
		offsetX = math.Max(w-sw, math.Min(0, w/2-sw/2+panX))
	}

	if sh <= h {
		offsetY = h/2 - sh/2
	} else {
		offsetY = math.Max(h-sh, math.Min(0, h/2-sh/2+panY))
	}

	return scale, offsetX, offsetY
}

func (r *Renderer) calculateCanvasTransform(layout displayLayout, scale, offsetX, offsetY float64) ebiten.GeoM {
	return r.calculateImageTransform(layout, scale, offsetX, offsetY, 0, 0)
}

func (r *Renderer) calculateImageTransform(layout displayLayout, scale, offsetX, offsetY, imageX, imageY float64) ebiten.GeoM {
	centerX := float64(layout.canvasW) / 2
	centerY := float64(layout.canvasH) / 2
	var transform ebiten.GeoM
	transform.Translate(imageX, imageY)
	transform.Translate(-centerX, -centerY)

	if r.renderState.IsFlippedH() {
		transform.Scale(-1, 1)
	}
	if r.renderState.IsFlippedV() {
		transform.Scale(1, -1)
	}
	if angle := r.renderState.GetRotationAngle(); angle != 0 {
		transform.Rotate(float64(angle) * math.Pi / 180)
	}

	transform.Translate(float64(layout.transformedW)/2, float64(layout.transformedH)/2)
	transform.Scale(scale, scale)
	transform.Translate(offsetX, offsetY)
	return transform
}

func (r *Renderer) drawDisplayImageTiles(screen *ebiten.Image, img DisplayImage, imageX, imageY int, layout displayLayout, scale, offsetX, offsetY float64, transform ebiten.GeoM) {
	tiles := img.Tiles()
	if len(tiles) == 1 && tiles[0].SrcX == 0 && tiles[0].SrcY == 0 {
		tile := tiles[0]
		if tile.Image == nil {
			return
		}

		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		op.GeoM = r.calculateImageTransform(
			layout,
			scale,
			offsetX,
			offsetY,
			float64(imageX+tile.X),
			float64(imageY+tile.Y),
		)
		screen.DrawImage(tile.Image, op)
		return
	}

	op := &ebiten.DrawTrianglesOptions{Filter: ebiten.FilterLinear}
	for _, tile := range tiles {
		if tile.Image == nil {
			continue
		}

		vertices := r.displayTileVertices(tile, imageX, imageY, transform)
		screen.DrawTriangles(vertices[:], tileTriangleIndices, tile.Image, op)
	}
}

func (r *Renderer) displayTileVertices(tile DisplayTile, imageX, imageY int, transform ebiten.GeoM) [4]ebiten.Vertex {
	x0 := imageX + tile.X
	y0 := imageY + tile.Y
	x1 := x0 + tile.W
	y1 := y0 + tile.H

	p00 := r.transformedTilePoint(image.Pt(x0, y0), transform)
	p10 := r.transformedTilePoint(image.Pt(x1, y0), transform)
	p01 := r.transformedTilePoint(image.Pt(x0, y1), transform)
	p11 := r.transformedTilePoint(image.Pt(x1, y1), transform)

	sx0 := float32(tile.SrcX)
	sy0 := float32(tile.SrcY)
	sx1 := sx0 + float32(tile.W)
	sy1 := sy0 + float32(tile.H)

	return [4]ebiten.Vertex{
		newTileVertex(p00, sx0, sy0),
		newTileVertex(p10, sx1, sy0),
		newTileVertex(p01, sx0, sy1),
		newTileVertex(p11, sx1, sy1),
	}
}

func (r *Renderer) transformedTilePoint(point image.Point, transform ebiten.GeoM) tileScreenPoint {
	// Sharing the already-rounded float32 result is essential: independently
	// transforming the bottom of one tile and the top of the next can put the
	// two DrawImage quads on opposite sides of Ebitengine's vertex snapping.
	if transformed, ok := r.tilePointCache[point]; ok {
		return transformed
	}

	x, y := transform.Apply(float64(point.X), float64(point.Y))
	transformed := tileScreenPoint{x: float32(x), y: float32(y)}
	if r.tilePointCache == nil {
		r.tilePointCache = make(map[image.Point]tileScreenPoint)
	}
	r.tilePointCache[point] = transformed
	return transformed
}

func newTileVertex(dst tileScreenPoint, srcX, srcY float32) ebiten.Vertex {
	return ebiten.Vertex{
		DstX:   dst.x,
		DstY:   dst.y,
		SrcX:   srcX,
		SrcY:   srcY,
		ColorR: 1,
		ColorG: 1,
		ColorB: 1,
		ColorA: 1,
	}
}
