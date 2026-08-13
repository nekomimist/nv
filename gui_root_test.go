package main

import (
	"image"
	"image/color"
	"math"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"nv/internal/imgdecode"
)

func TestGUI_NavigateSingleUsesActionSemantics(t *testing.T) {
	images := []DisplayImage{
		testDisplayImage(100, 150),
		testDisplayImage(100, 150),
		testDisplayImage(100, 150),
		testDisplayImage(100, 150),
	}
	manager := &stubImageManager{
		paths: []ImagePath{
			{Path: "1.png"},
			{Path: "2.png"},
			{Path: "3.png"},
			{Path: "4.png"},
		},
		images: images,
	}
	g := &Game{
		imageManager:    manager,
		bookMode:        true,
		displayContent:  &DisplayContent{LeftImage: images[0], RightImage: images[1]},
		zoomState:       NewZoomState(),
		config:          Config{AspectRatioThreshold: 1.5},
		currentLogicalW: 800,
		currentLogicalH: 600,
	}

	g.NavigateNextSingle()

	if g.idx != 1 {
		t.Fatalf("NavigateNextSingle moved to %d, want 1", g.idx)
	}
	if len(manager.preloadDirections) != 1 || manager.preloadDirections[0] != NavigationForward {
		t.Fatalf("unexpected preload directions: %v", manager.preloadDirections)
	}
}

func TestGUI_NavigateNextKeepsSpreadBehavior(t *testing.T) {
	images := []DisplayImage{
		testDisplayImage(100, 150),
		testDisplayImage(100, 150),
		testDisplayImage(100, 150),
		testDisplayImage(100, 150),
	}
	manager := &stubImageManager{
		paths: []ImagePath{
			{Path: "1.png"},
			{Path: "2.png"},
			{Path: "3.png"},
			{Path: "4.png"},
		},
		images: images,
	}
	g := &Game{
		imageManager:    manager,
		bookMode:        true,
		displayContent:  &DisplayContent{LeftImage: images[0], RightImage: images[1]},
		zoomState:       NewZoomState(),
		config:          Config{AspectRatioThreshold: 1.5},
		currentLogicalW: 800,
		currentLogicalH: 600,
	}

	g.NavigateNext()

	if g.idx != 2 {
		t.Fatalf("NavigateNext moved to %d, want 2", g.idx)
	}
}

func TestGUI_CalculateDisplayContentUsesNavigationPlan(t *testing.T) {
	tests := []struct {
		name               string
		rightToLeft        bool
		leftW, leftH       int
		rightW, rightH     int
		expectedActual     int
		expectedLeftIndex  int
		expectedRightIndex int
		expectedLeftPage   int
		expectedRightPage  int
	}{
		{"Compatible LTR spread", false, 100, 150, 100, 150, 2, 0, 1, 1, 2},
		{"Compatible RTL spread", true, 100, 150, 100, 150, 2, 1, 0, 2, 1},
		{"Incompatible fallback to single", false, 100, 150, 300, 100, 1, 0, -1, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			images := []DisplayImage{
				testDisplayImage(tt.leftW, tt.leftH),
				testDisplayImage(tt.rightW, tt.rightH),
			}
			manager := &stubImageManager{
				paths: []ImagePath{
					{Path: "left.png"},
					{Path: "right.png"},
				},
				images: images,
			}
			g := &Game{
				imageManager: manager,
				bookMode:     true,
				config: Config{
					AspectRatioThreshold: 1.5,
					RightToLeft:          tt.rightToLeft,
				},
				zoomState: NewZoomState(),
			}

			g.calculateDisplayContent()

			if g.displayContent == nil {
				t.Fatal("expected display content")
			}
			if g.displayContent.Metadata.ActualImages != tt.expectedActual {
				t.Fatalf("actual images = %d, want %d", g.displayContent.Metadata.ActualImages, tt.expectedActual)
			}
			if g.displayContent.Metadata.LeftPage != tt.expectedLeftPage || g.displayContent.Metadata.RightPage != tt.expectedRightPage {
				t.Fatalf("unexpected pages: got left=%d right=%d want left=%d right=%d",
					g.displayContent.Metadata.LeftPage,
					g.displayContent.Metadata.RightPage,
					tt.expectedLeftPage,
					tt.expectedRightPage,
				)
			}

			expectedLeft := manager.images[tt.expectedLeftIndex]
			if g.displayContent.LeftImage != expectedLeft {
				t.Fatalf("unexpected left image index: got %p want %p", g.displayContent.LeftImage, expectedLeft)
			}

			if tt.expectedRightIndex < 0 {
				if g.displayContent.RightImage != nil {
					t.Fatalf("expected nil right image, got %p", g.displayContent.RightImage)
				}
			} else {
				expectedRight := manager.images[tt.expectedRightIndex]
				if g.displayContent.RightImage != expectedRight {
					t.Fatalf("unexpected right image index: got %p want %p", g.displayContent.RightImage, expectedRight)
				}
			}
		})
	}
}

func TestGUI_BuildPageNumberStringUsesScreenOrder(t *testing.T) {
	tests := []struct {
		name     string
		metadata DisplayMetadata
		expected string
	}{
		{
			name: "single page",
			metadata: DisplayMetadata{
				LeftPage:     3,
				TotalPages:   10,
				ActualImages: 1,
			},
			expected: "3 / 10",
		},
		{
			name: "ltr spread",
			metadata: DisplayMetadata{
				LeftPage:     3,
				RightPage:    4,
				TotalPages:   10,
				ActualImages: 2,
			},
			expected: "3→4 / 10",
		},
		{
			name: "rtl spread",
			metadata: DisplayMetadata{
				LeftPage:     4,
				RightPage:    3,
				TotalPages:   10,
				ActualImages: 2,
			},
			expected: "4←3 / 10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Game{
				displayContent: &DisplayContent{Metadata: tt.metadata},
			}
			r := NewRenderer(g)

			if got := r.buildPageNumberString(); got != tt.expected {
				t.Fatalf("buildPageNumberString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGUI_ToggleReadingDirectionRecalculatesDisplayContent(t *testing.T) {
	images := []DisplayImage{
		testDisplayImage(100, 150),
		testDisplayImage(100, 150),
	}
	manager := &stubImageManager{
		paths: []ImagePath{
			{Path: "1.png"},
			{Path: "2.png"},
		},
		images: images,
	}
	g := &Game{
		imageManager: manager,
		bookMode:     true,
		config: Config{
			AspectRatioThreshold: 1.5,
			RightToLeft:          false,
		},
		zoomState: NewZoomState(),
	}

	g.calculateDisplayContent()
	if g.displayContent == nil || g.displayContent.LeftImage != images[0] || g.displayContent.RightImage != images[1] {
		t.Fatalf("unexpected initial display content: %+v", g.displayContent)
	}

	g.ToggleReadingDirection()

	if !g.config.RightToLeft {
		t.Fatal("expected RightToLeft to be enabled")
	}
	if g.displayContent == nil {
		t.Fatal("expected display content after toggling reading direction")
	}
	if g.displayContent.LeftImage != images[1] || g.displayContent.RightImage != images[0] {
		t.Fatalf("expected images to swap after toggling reading direction, got left=%p right=%p", g.displayContent.LeftImage, g.displayContent.RightImage)
	}
	if g.displayContent.Metadata.LeftPage != 2 || g.displayContent.Metadata.RightPage != 1 {
		t.Fatalf("unexpected page order after toggle: %+v", g.displayContent.Metadata)
	}
}

func TestGUI_MarkCurrentAsPreJoinedSpreadBreaksCurrentPair(t *testing.T) {
	images := []DisplayImage{
		testDisplayImage(200, 150),
		testDisplayImage(210, 150),
	}
	manager := &stubImageManager{
		paths: []ImagePath{
			{Path: "left.png"},
			{Path: "right.png"},
		},
		images: images,
	}
	g := &Game{
		imageManager: manager,
		bookMode:     true,
		config: Config{
			AspectRatioThreshold: 1.5,
		},
		zoomState: NewZoomState(),
	}

	g.calculateDisplayContent()
	if g.displayContent == nil || g.displayContent.Metadata.ActualImages != 2 {
		t.Fatalf("expected initial pair, got %+v", g.displayContent)
	}

	g.MarkCurrentAsPreJoinedSpread()

	if len(g.learnedSpreadAspects) != 2 {
		t.Fatalf("expected two learned spread aspects, got %v", g.learnedSpreadAspects)
	}
	if g.displayContent == nil || g.displayContent.Metadata.ActualImages != 1 {
		t.Fatalf("expected single-page fallback after learning spread, got %+v", g.displayContent)
	}
	if g.displayContent.RightImage != nil {
		t.Fatalf("expected no right image after learning spread, got %p", g.displayContent.RightImage)
	}
}

func TestGUI_ImageManager(t *testing.T) {
	paths := []ImagePath{
		{Path: "1.jpg"},
		{Path: "2.jpg"},
		{Path: "3.jpg"},
		{Path: "4.jpg"},
		{Path: "5.jpg"},
	}

	imageManager := NewImageManager(4)
	imageManager.SetPaths(paths)

	if count := imageManager.GetPathsCount(); count != 5 {
		t.Errorf("Expected paths count 5, got %d", count)
	}

	leftImg, rightImg := imageManager.GetBookModeImages(0, false, imgdecode.Hint{}, imgdecode.Hint{})
	if leftImg != nil || rightImg != nil {
		t.Logf("Images are nil as expected (no actual image files)")
	}
}

func TestGUI_ImageManagerPreloadQueueCapacity(t *testing.T) {
	tests := []struct {
		name         string
		preloadCount int
		wantCap      int
	}{
		{name: "below_floor", preloadCount: 1, wantCap: 8},
		{name: "at_floor", preloadCount: 8, wantCap: 8},
		{name: "above_floor", preloadCount: 16, wantCap: 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewImageManagerWithPreload(4, tt.preloadCount, true).(*DefaultImageManager)
			t.Cleanup(func() {
				manager.StopPreload()
			})

			if got := cap(manager.preloadRequests); got != tt.wantCap {
				t.Fatalf("cap(preloadRequests) = %d, want %d", got, tt.wantCap)
			}
		})
	}
}

func TestGUI_ImageManagerPlainConstructorKeepsPreloadQueueFloor(t *testing.T) {
	manager := NewImageManager(4).(*DefaultImageManager)
	t.Cleanup(func() {
		manager.StopPreload()
	})

	if got, want := cap(manager.preloadRequests), 8; got != want {
		t.Fatalf("cap(preloadRequests) = %d, want %d", got, want)
	}
}

// deallocateTrackingDisplayImage wraps a DisplayImage to record whether
// Deallocate was called, so a test can observe the tiered cache's evict
// callback actually running (rather than just the entry disappearing from
// the cache).
type deallocateTrackingDisplayImage struct {
	DisplayImage
	deallocated *bool
}

func (d *deallocateTrackingDisplayImage) Deallocate() {
	*d.deallocated = true
	d.DisplayImage.Deallocate()
}

// reducedDisplayImage wraps a DisplayImage to report a SourceBounds larger
// than Bounds, simulating a genuinely reduced budget-tier decode (as a real
// JPEG/WebP native decode would produce) without needing a native-decode
// build in these tests.
type reducedDisplayImage struct {
	DisplayImage
	sourceBounds image.Rectangle
}

func (r *reducedDisplayImage) SourceBounds() image.Rectangle {
	return r.sourceBounds
}

func TestGUI_GetImageReturnsBudgetTierInsteadOfPlaceholder(t *testing.T) {
	manager := NewImageManager(4).(*DefaultImageManager)
	t.Cleanup(func() {
		manager.StopPreload()
	})

	path := ImagePath{Path: "budget-only.png"}
	manager.SetPaths([]ImagePath{path})

	budgetImg := testDisplayImage(40, 30)
	manager.cache.Add(imgCacheKey{path: path.Path, tier: tierBudget}, budgetImg)

	got := manager.GetImage(0, imgdecode.Hint{})
	if got != budgetImg {
		t.Fatalf("GetImage() = %v, want the cached budget-tier image", got)
	}
	if got == manager.loadingPlaceholder {
		t.Fatal("GetImage() returned the loading placeholder despite a cached budget-tier image with real pixels")
	}
}

func TestGUI_GetImageTierFullWinsOverTierBudget(t *testing.T) {
	manager := NewImageManager(4).(*DefaultImageManager)
	t.Cleanup(func() {
		manager.StopPreload()
	})

	path := ImagePath{Path: "both-tiers.png"}
	manager.SetPaths([]ImagePath{path})

	budgetImg := testDisplayImage(40, 30)
	fullImg := testDisplayImage(400, 300)
	manager.cache.Add(imgCacheKey{path: path.Path, tier: tierBudget}, budgetImg)
	manager.cache.Add(imgCacheKey{path: path.Path, tier: tierFull}, fullImg)

	got := manager.GetImage(0, imgdecode.Hint{})
	if got != fullImg {
		t.Fatalf("GetImage() = %v, want the full-tier image to win over the budget-tier one", got)
	}
}

func TestGUI_TierFullLandingRemovesTierBudgetEntry(t *testing.T) {
	fixture := requireFixture(t, filepath.Join("test_images", "debian-logo.png"))

	manager := NewImageManager(4).(*DefaultImageManager)
	t.Cleanup(func() {
		manager.StopPreload()
	})

	imagePath := ImagePath{Path: fixture}

	deallocated := false
	budgetKey := imgCacheKey{path: imagePath.Path, tier: tierBudget}
	manager.cache.Add(budgetKey, &deallocateTrackingDisplayImage{
		DisplayImage: testDisplayImage(10, 10),
		deallocated:  &deallocated,
	})

	// Simulate a tierFull decode landing, as processLoadRequest does on
	// the async load worker after a real decode succeeds.
	fullKey := imgCacheKey{path: imagePath.Path, tier: tierFull}
	manager.processLoadRequest(loadRequest{path: imagePath, cacheKey: fullKey, source: loadSourceAsync})

	if _, ok := manager.cache.Peek(budgetKey); ok {
		t.Fatal("expected the budget-tier entry to be removed once the full-tier decode landed")
	}
	if !deallocated {
		t.Fatal("expected the removed budget-tier entry to be deallocated")
	}
	if _, ok := manager.cache.Peek(fullKey); !ok {
		t.Fatal("expected the full-tier entry to be present after processLoadRequest")
	}
}

func TestGUI_EnsureResolutionNoopWhenAlreadyMaximal(t *testing.T) {
	manager := NewImageManager(4).(*DefaultImageManager)
	t.Cleanup(func() {
		manager.StopPreload()
	})

	path := ImagePath{Path: "already-maximal.png"}
	manager.SetPaths([]ImagePath{path})

	// SourceBounds() == Bounds() here (testDisplayImage mirrors Bounds),
	// i.e. imgdecode.Info.Reduced == false: no higher-resolution version
	// exists, so EnsureResolution must not escalate even though this
	// image is smaller than the hint below.
	budgetImg := testDisplayImage(40, 30)
	manager.cache.Add(imgCacheKey{path: path.Path, tier: tierBudget}, budgetImg)

	manager.EnsureResolution(0, imgdecode.Hint{MaxWidth: 400, MaxHeight: 300})

	if got := len(manager.refineRequests); got != 0 {
		t.Fatalf("refineRequests queued = %d, want 0 (SourceBounds() == Bounds() must suppress escalation)", got)
	}
	manager.fullRequestedMu.Lock()
	_, requested := manager.fullRequested[path.Path]
	manager.fullRequestedMu.Unlock()
	if requested {
		t.Fatal("expected fullRequested to remain unset when nothing was escalated")
	}
}

func TestGUI_EnsureResolutionAlreadyRequestedGuardSuppressesRepeats(t *testing.T) {
	manager := NewImageManager(4).(*DefaultImageManager)
	t.Cleanup(func() {
		manager.StopPreload()
	})

	// A path that does not exist on disk: any refinement the worker
	// happens to dequeue while this test runs will fail to decode, and a
	// failed refinement must not clear the fullRequested guard (only
	// eviction does) -- see processLoadRequest and clearFullRequested.
	path := ImagePath{Path: "reduced-repeat.png"}
	manager.SetPaths([]ImagePath{path})

	budgetImg := &reducedDisplayImage{
		DisplayImage: testDisplayImage(40, 30),
		sourceBounds: image.Rect(0, 0, 400, 300),
	}
	manager.cache.Add(imgCacheKey{path: path.Path, tier: tierBudget}, budgetImg)

	hint := imgdecode.Hint{MaxWidth: 400, MaxHeight: 300}

	manager.EnsureResolution(0, hint)
	manager.fullRequestedMu.Lock()
	_, requestedAfterFirst := manager.fullRequested[path.Path]
	manager.fullRequestedMu.Unlock()
	if !requestedAfterFirst {
		t.Fatal("expected fullRequested to be set after the first EnsureResolution call")
	}

	// requestFullResolution checks fullRequested before ever touching the
	// refine queue, so repeated calls must not re-arm it.
	manager.EnsureResolution(0, hint)
	manager.EnsureResolution(0, hint)

	manager.fullRequestedMu.Lock()
	_, stillRequested := manager.fullRequested[path.Path]
	manager.fullRequestedMu.Unlock()
	if !stillRequested {
		t.Fatal("expected fullRequested guard to remain set across repeated EnsureResolution calls")
	}
}

func TestGUI_CreateDisplayImageTilesWhenOverLimit(t *testing.T) {
	manager := NewImageManager(1).(*DefaultImageManager)
	manager.SetMaxImageDimension(3)
	t.Cleanup(func() {
		manager.StopPreload()
	})

	src := image.NewNRGBA(image.Rect(0, 0, defaultTileSize+1, 4))
	info := imgdecode.Info{
		Width: defaultTileSize + 1, Height: 4,
		SourceWidth: defaultTileSize + 1, SourceHeight: 4,
	}
	img, err := manager.createEbitenImageFromDecoded(src, "large.png", info)
	if err != nil {
		t.Fatalf("createEbitenImageFromDecoded() error = %v", err)
	}
	t.Cleanup(img.Deallocate)

	if got, want := img.Bounds().Dx(), defaultTileSize+1; got != want {
		t.Fatalf("width = %d, want %d", got, want)
	}
	if got, want := img.Bounds().Dy(), 4; got != want {
		t.Fatalf("height = %d, want %d", got, want)
	}
	if img.TileCount() <= 1 {
		t.Fatalf("expected tiled image, got %d tile(s)", img.TileCount())
	}
}

// TestGUI_CreateTiledDisplayImageTilesHaveDistinctContentAndGutters guards
// both the reusable scratch buffer and the sampling gutters around each tile.
//
// Reading pixels back from a real *ebiten.Image (e.g. via Image.At) requires
// an active Ebiten game loop, which isn't running under `go test` here, so
// this intercepts newUnmanagedEbitenImageFn to capture (a copy of) each
// tile's pixel buffer at the exact point production code would hand it off
// for conversion -- which is what buffer reuse and gutter generation must
// get right.
func TestGUI_CreateTiledDisplayImageTilesHaveDistinctContentAndGutters(t *testing.T) {
	const (
		tileSize = 6
		width    = 10
		height   = 6
	)
	coreTileSize := tileSize - 2*tileGutterSize
	srcBounds := image.Rect(11, 17, 11+width, 17+height)
	src := image.NewNRGBA(srcBounds)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			src.SetNRGBA(srcBounds.Min.X+x, srcBounds.Min.Y+y, color.NRGBA{
				R: uint8(17*x + 3),
				G: uint8(29*y + 5),
				B: uint8(x + 7*y),
				A: 255,
			})
		}
	}

	type capturedTile struct {
		w, h, stride int
		pix          []byte
	}
	var captured []capturedTile

	originalConverter := newUnmanagedEbitenImageFn
	newUnmanagedEbitenImageFn = func(tileSrc image.Image) (*ebiten.Image, error) {
		rgba := tileSrc.(*image.RGBA)
		b := rgba.Bounds()
		captured = append(captured, capturedTile{
			w:      b.Dx(),
			h:      b.Dy(),
			stride: rgba.Stride,
			pix:    append([]byte(nil), rgba.Pix...), // copy: scratch buffer may be reused next iteration
		})
		return originalConverter(tileSrc)
	}
	t.Cleanup(func() {
		newUnmanagedEbitenImageFn = originalConverter
	})

	result, err := createTiledDisplayImage(src, tileSize)
	if err != nil {
		t.Fatalf("createTiledDisplayImage() error = %v", err)
	}
	t.Cleanup(result.Deallocate)

	tiles := result.Tiles()
	wantTileCount := ((width + coreTileSize - 1) / coreTileSize) * ((height + coreTileSize - 1) / coreTileSize)
	if len(tiles) != wantTileCount || len(captured) != wantTileCount {
		t.Fatalf("got %d tile(s) and %d captured buffer(s), want %d each", len(tiles), len(captured), wantTileCount)
	}

	coverage := make([]int, width*height)
	for i, tile := range tiles {
		c := captured[i]

		wantW := min(coreTileSize, width-tile.X)
		wantH := min(coreTileSize, height-tile.Y)
		if tile.W != wantW || tile.H != wantH || tile.SrcX != tileGutterSize || tile.SrcY != tileGutterSize {
			t.Fatalf("tile[%d] = %+v, want W=%d H=%d Src=(%d,%d)", i, tile, wantW, wantH, tileGutterSize, tileGutterSize)
		}
		if c.w != tile.W+2*tileGutterSize || c.h != tile.H+2*tileGutterSize {
			t.Fatalf("captured tile[%d] size = %dx%d, want %dx%d", i, c.w, c.h, tile.W+2*tileGutterSize, tile.H+2*tileGutterSize)
		}
		if c.w > tileSize || c.h > tileSize {
			t.Fatalf("captured tile[%d] exceeds texture limit: %dx%d > %d", i, c.w, c.h, tileSize)
		}

		for y := 0; y < c.h; y++ {
			for x := 0; x < c.w; x++ {
				off := y*c.stride + x*4
				got := color.NRGBA{c.pix[off], c.pix[off+1], c.pix[off+2], c.pix[off+3]}
				sourceX := max(0, min(width-1, tile.X+x-tile.SrcX))
				sourceY := max(0, min(height-1, tile.Y+y-tile.SrcY))
				want := src.NRGBAAt(srcBounds.Min.X+sourceX, srcBounds.Min.Y+sourceY)
				if got != want {
					t.Fatalf("tile[%d] pixel (%d,%d) = %v, want source (%d,%d) = %v", i, x, y, got, sourceX, sourceY, want)
				}
			}
		}

		for y := tile.Y; y < tile.Y+tile.H; y++ {
			for x := tile.X; x < tile.X+tile.W; x++ {
				coverage[y*width+x]++
			}
		}
	}

	for i, count := range coverage {
		if count != 1 {
			t.Fatalf("source pixel %d covered %d times, want exactly once", i, count)
		}
	}
}

func TestGUI_DisplayTileVerticesShareExactTransformedEdges(t *testing.T) {
	const coreSize = 2046
	topLeft := DisplayTile{X: 0, Y: 0, W: coreSize, H: coreSize, SrcX: 1, SrcY: 1}
	topRight := DisplayTile{X: coreSize, Y: 0, W: 500, H: coreSize, SrcX: 1, SrcY: 1}
	bottomLeft := DisplayTile{X: 0, Y: coreSize, W: coreSize, H: 700, SrcX: 1, SrcY: 1}

	tests := []struct {
		name   string
		scale  float64
		angle  int
		flipH  bool
		flipV  bool
		offset float64
	}{
		{name: "quarter_scale", scale: 0.25, angle: 0, offset: -123.25},
		{name: "identity_scale", scale: 1, angle: 90, flipH: true, offset: 17.5},
		{name: "131_percent_repro", scale: 1.31, angle: 0, offset: 635.5 - coreSize*1.31},
		{name: "rotated_and_flipped", scale: 1.31, angle: 270, flipH: true, flipV: true, offset: -911.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			game := &Game{rotationAngle: tt.angle, flipH: tt.flipH, flipV: tt.flipV}
			renderer := &Renderer{renderState: game, tilePointCache: make(map[image.Point]tileScreenPoint)}
			layout := displayLayout{canvasW: 5000, canvasH: 7000, transformedW: 5000, transformedH: 7000}
			if tt.angle == 90 || tt.angle == 270 {
				layout.transformedW, layout.transformedH = layout.canvasH, layout.canvasW
			}
			transform := renderer.calculateCanvasTransform(layout, tt.scale, 73.25, tt.offset)

			leftVertices := renderer.displayTileVertices(topLeft, 19, 23, transform)
			rightVertices := renderer.displayTileVertices(topRight, 19, 23, transform)
			bottomVertices := renderer.displayTileVertices(bottomLeft, 19, 23, transform)

			assertSameVertexPosition(t, "horizontal top", leftVertices[1], rightVertices[0])
			assertSameVertexPosition(t, "horizontal bottom", leftVertices[3], rightVertices[2])
			assertSameVertexPosition(t, "vertical left", leftVertices[2], bottomVertices[0])
			assertSameVertexPosition(t, "vertical right", leftVertices[3], bottomVertices[1])

			if got, want := leftVertices[0].SrcX, float32(1); got != want {
				t.Fatalf("source x = %v, want %v", got, want)
			}
			if got, want := leftVertices[3].SrcY, float32(coreSize+1); got != want {
				t.Fatalf("source y = %v, want %v", got, want)
			}
		})
	}
}

func assertSameVertexPosition(t *testing.T, name string, a, b ebiten.Vertex) {
	t.Helper()
	if math.Float32bits(a.DstX) != math.Float32bits(b.DstX) || math.Float32bits(a.DstY) != math.Float32bits(b.DstY) {
		t.Fatalf("%s edge differs: (%v,%v) vs (%v,%v)", name, a.DstX, a.DstY, b.DstX, b.DstY)
	}
}
