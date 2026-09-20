package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"
)

// Common colors used in rendering
var (
	colorWhite     = color.RGBA{255, 255, 255, 255}
	colorLightGray = color.RGBA{192, 192, 192, 255}

	// Background colors for semi-transparent overlays
	bgColorLight = color.RGBA{0, 0, 0, 128} // Light semi-transparent
	bgColorDark  = color.RGBA{0, 0, 0, 200} // Dark semi-transparent
)

// Renderer handles all drawing operations
type Renderer struct {
	renderState    RenderState
	helpFontSource *text.GoTextFaceSource
	lastSnapshot   RenderStateSnapshot // Previous frame's state for comparison
	hasSnapshot    bool                // Whether lastSnapshot holds a valid snapshot
	tilePointCache map[canvasPoint]tileScreenPoint
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
		tilePointCache: make(map[canvasPoint]tileScreenPoint),
	}
}

func (r *Renderer) Draw(screen *ebiten.Image) {
	r.DrawBase(screen)
	r.DrawOverlays(screen)
}

// DrawBase clears the frame and renders only page content. EbitenUI error
// windows are drawn after this and before DrawOverlays.
func (r *Renderer) DrawBase(screen *ebiten.Image) {
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
}

// DrawOverlays renders the existing lightweight HUD overlays. Modal and
// page-error UI is owned by UIController.
func (r *Renderer) DrawOverlays(screen *ebiten.Image) {
	// Draw info display (page status, etc.) at bottom of screen if enabled
	if r.renderState.IsShowingInfo() {
		r.drawInfoDisplay(screen)
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

// canvasPoint is a point in the source-pixel canvas (see
// calculateDisplayLayout), used as the transformedTilePoint cache key. It is
// float64 (rather than image.Point's int) because a reduced-texture tile's
// canvas-space corner is imageX/imageY (integer) plus a texture-pixel
// coordinate scaled by that image's textureToSourceScale, which is not
// generally an integer.
type canvasPoint struct {
	x, y float64
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

// calculateDisplayLayout arranges leftImg/rightImg into a canvas measured in
// source pixels (DisplayImage.SourceBounds), not decoded-texture pixels
// (DisplayImage.Bounds) -- see getTransformedImageSize in game_viewport.go
// for why zoom/fit-scale math needs the source-space size. The actual
// (possibly smaller) texture is placed into this canvas at draw time via
// textureToSourceScale.
func (r *Renderer) calculateDisplayLayout(leftImg, rightImg DisplayImage) displayLayout {
	leftBounds := leftImg.SourceBounds()
	leftW, leftH := leftBounds.Dx(), leftBounds.Dy()

	layout := displayLayout{
		canvasW: leftW,
		canvasH: leftH,
		leftX:   0,
		leftY:   0,
	}

	if rightImg != nil {
		rightBounds := rightImg.SourceBounds()
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
	return r.calculateDisplayTransformForSize(screen.Bounds().Dx(), screen.Bounds().Dy(), imageW, imageH)
}

func (r *Renderer) calculateDisplayTransformForSize(width, height, imageW, imageH int) (float64, float64, float64) {
	iw, ih := float64(imageW), float64(imageH)
	w, h := float64(width), float64(height)

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

// displayImageScreenRect reports the axis-aligned screen rectangle occupied
// by one page in the current spread. Error cards use this to avoid covering
// the successfully rendered page when only one side of a spread failed.
func (r *Renderer) displayImageScreenRect(width, height int, leftImg, rightImg DisplayImage, slot int) image.Rectangle {
	if leftImg == nil || (slot != 0 && slot != 1) {
		return image.Rectangle{}
	}

	layout := r.calculateDisplayLayout(leftImg, rightImg)
	img := leftImg
	imageX, imageY := layout.leftX, layout.leftY
	if slot == 1 {
		if rightImg == nil {
			return image.Rectangle{}
		}
		img = rightImg
		imageX, imageY = layout.rightX, layout.rightY
	}

	scale, offsetX, offsetY := r.calculateDisplayTransformForSize(
		width, height, layout.transformedW, layout.transformedH,
	)
	transform := r.calculateImageTransform(
		layout, scale, offsetX, offsetY, float64(imageX), float64(imageY), 1,
	)
	bounds := img.SourceBounds()
	w, h := float64(bounds.Dx()), float64(bounds.Dy())
	points := [4][2]float64{{0, 0}, {w, 0}, {0, h}, {w, h}}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, point := range points {
		x, y := transform.Apply(point[0], point[1])
		minX, minY = math.Min(minX, x), math.Min(minY, y)
		maxX, maxY = math.Max(maxX, x), math.Max(maxY, y)
	}
	return image.Rect(
		int(math.Floor(minX)), int(math.Floor(minY)),
		int(math.Ceil(maxX)), int(math.Ceil(maxY)),
	)
}

// calculateCanvasTransform builds the shared canvas(source-pixel)->screen
// transform used by the tiled draw path (displayTileVertices), where each
// tile's own texture->source scaling is applied separately in Go arithmetic
// before this transform runs (see drawDisplayImageTiles) rather than baked
// into the matrix, because a single shared transform cannot carry two
// different per-image scales (left and right images can be reduced by
// different amounts). Passing imgScale=1 here is exactly that: "no
// texture->source scaling baked in."
func (r *Renderer) calculateCanvasTransform(layout displayLayout, scale, offsetX, offsetY float64) ebiten.GeoM {
	return r.calculateImageTransform(layout, scale, offsetX, offsetY, 0, 0, 1)
}

// calculateImageTransform builds a texture(pixel)->screen transform for one
// image. imgScale (see textureToSourceScale) converts the image's own
// texture-pixel coordinates into source-pixel-equivalent units *before*
// imageX/imageY -- the image's source-space position within the canvas --
// is added, so a reduced (budget-tier) texture still lands in the same
// on-screen place and size a full-resolution decode of the same source
// image would.
func (r *Renderer) calculateImageTransform(layout displayLayout, scale, offsetX, offsetY, imageX, imageY, imgScale float64) ebiten.GeoM {
	centerX := float64(layout.canvasW) / 2
	centerY := float64(layout.canvasH) / 2
	var transform ebiten.GeoM
	transform.Scale(imgScale, imgScale)
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

// textureToSourceScale reports the scale that converts one of img's own
// texture-pixel coordinates into source-pixel-equivalent units (see
// DisplayImage.SourceBounds). It is 1 unless img was decoded from a reduced
// (budget-tier) hint, in which case SourceBounds() is strictly larger than
// Bounds() and this scales texture pixels up to match. A single scalar
// (rather than separate X/Y factors) is deliberate: ZoomLevel and every fit
// scale in this codebase are isotropic, and imgdecode's contain scaling
// preserves the source aspect ratio, so X and Y ratios are the same value
// (up to the codec's own rounding, which is negligible here).
func textureToSourceScale(img DisplayImage) float64 {
	bounds := img.Bounds()
	if bounds.Dx() <= 0 {
		return 1
	}
	return float64(img.SourceBounds().Dx()) / float64(bounds.Dx())
}

func (r *Renderer) drawDisplayImageTiles(screen *ebiten.Image, img DisplayImage, imageX, imageY int, layout displayLayout, scale, offsetX, offsetY float64, transform ebiten.GeoM) {
	tiles := img.Tiles()
	imgScale := textureToSourceScale(img)
	if len(tiles) == 1 && tiles[0].SrcX == 0 && tiles[0].SrcY == 0 {
		tile := tiles[0]
		if tile.Image == nil {
			return
		}

		// tile.X/tile.Y are always 0 in this single-tile branch, so
		// imageX+tile.X is purely the image's source-space canvas offset;
		// calculateImageTransform applies imgScale to the texture's own raw
		// pixel coordinates before that offset is added.
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		op.GeoM = r.calculateImageTransform(
			layout,
			scale,
			offsetX,
			offsetY,
			float64(imageX+tile.X),
			float64(imageY+tile.Y),
			imgScale,
		)
		screen.DrawImage(tile.Image, op)
		return
	}

	op := &ebiten.DrawTrianglesOptions{Filter: ebiten.FilterLinear}
	for _, tile := range tiles {
		if tile.Image == nil {
			continue
		}

		vertices := r.displayTileVertices(tile, imageX, imageY, imgScale, transform)
		screen.DrawTriangles(vertices[:], tileTriangleIndices, tile.Image, op)
	}
}

// displayTileVertices places tile (in its own texture-pixel space) into the
// shared source-pixel canvas by scaling its corners by imgScale *before*
// adding imageX/imageY (the image's already source-space canvas offset),
// then runs the result through the shared canvas->screen transform. See
// calculateCanvasTransform for why this scaling can't be baked into that
// shared transform instead.
func (r *Renderer) displayTileVertices(tile DisplayTile, imageX, imageY int, imgScale float64, transform ebiten.GeoM) [4]ebiten.Vertex {
	x0 := float64(imageX) + float64(tile.X)*imgScale
	y0 := float64(imageY) + float64(tile.Y)*imgScale
	x1 := float64(imageX) + float64(tile.X+tile.W)*imgScale
	y1 := float64(imageY) + float64(tile.Y+tile.H)*imgScale

	p00 := r.transformedTilePoint(canvasPoint{x0, y0}, transform)
	p10 := r.transformedTilePoint(canvasPoint{x1, y0}, transform)
	p01 := r.transformedTilePoint(canvasPoint{x0, y1}, transform)
	p11 := r.transformedTilePoint(canvasPoint{x1, y1}, transform)

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

func (r *Renderer) transformedTilePoint(point canvasPoint, transform ebiten.GeoM) tileScreenPoint {
	// Sharing the already-rounded float32 result is essential: independently
	// transforming the bottom of one tile and the top of the next can put the
	// two DrawImage quads on opposite sides of Ebitengine's vertex snapping.
	// Adjacent tiles within the same image compute this point from the exact
	// same imageX/tile-boundary/imgScale values (see displayTileVertices),
	// so it is bit-identical across tiles without needing to round to an
	// integer canvas position first.
	if transformed, ok := r.tilePointCache[point]; ok {
		return transformed
	}

	x, y := transform.Apply(point.x, point.y)
	transformed := tileScreenPoint{x: float32(x), y: float32(y)}
	if r.tilePointCache == nil {
		r.tilePointCache = make(map[canvasPoint]tileScreenPoint)
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
