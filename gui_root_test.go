package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestGUI_NavigateSingleUsesActionSemantics(t *testing.T) {
	images := []*ebiten.Image{
		ebiten.NewImage(100, 150),
		ebiten.NewImage(100, 150),
		ebiten.NewImage(100, 150),
		ebiten.NewImage(100, 150),
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
	images := []*ebiten.Image{
		ebiten.NewImage(100, 150),
		ebiten.NewImage(100, 150),
		ebiten.NewImage(100, 150),
		ebiten.NewImage(100, 150),
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

func TestGUI_RendererCachesIntermediateImages(t *testing.T) {
	g := &Game{}
	r := NewRenderer(g)
	left := ebiten.NewImage(100, 150)
	right := ebiten.NewImage(100, 150)

	book1 := r.createBookModeImage(left, right)
	book2 := r.createBookModeImage(left, right)
	if book1 != book2 {
		t.Fatal("expected book composition cache hit")
	}

	g.rotationAngle = 90
	transformed1 := r.applyTransformations(book1)
	transformed2 := r.applyTransformations(book1)
	if transformed1 != transformed2 {
		t.Fatal("expected transformation cache hit")
	}

	g.rotationAngle = 180
	transformed3 := r.applyTransformations(book1)
	if transformed3 == transformed2 {
		t.Fatal("expected cache invalidation when rotation changes")
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
	}{
		{"Compatible LTR spread", false, 100, 150, 100, 150, 2, 0, 1},
		{"Compatible RTL spread", true, 100, 150, 100, 150, 2, 1, 0},
		{"Incompatible fallback to single", false, 100, 150, 300, 100, 1, 0, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			images := []*ebiten.Image{
				ebiten.NewImage(tt.leftW, tt.leftH),
				ebiten.NewImage(tt.rightW, tt.rightH),
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

func TestGUI_MarkCurrentAsPreJoinedSpreadBreaksCurrentPair(t *testing.T) {
	images := []*ebiten.Image{
		ebiten.NewImage(200, 150),
		ebiten.NewImage(210, 150),
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

	leftImg, rightImg := imageManager.GetBookModeImages(0, false)
	if leftImg != nil || rightImg != nil {
		t.Logf("Images are nil as expected (no actual image files)")
	}
}
